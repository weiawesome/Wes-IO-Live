package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weiawesome/wes-io-live/media-service/internal/config"
	"github.com/weiawesome/wes-io-live/media-service/internal/service"
	"github.com/weiawesome/wes-io-live/media-service/internal/webrtc"
	"github.com/weiawesome/wes-io-live/pkg/pubsub"
	"github.com/weiawesome/wes-io-live/pkg/storage"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Media Service on %s:%d", cfg.Server.Host, cfg.Server.Port)

	// Ensure HLS output directory exists
	if err := os.MkdirAll(cfg.HLS.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create HLS output directory: %v", err)
	}

	// Initialize PubSub
	ps, err := pubsub.NewPubSub(cfg.PubSub)
	if err != nil {
		log.Fatalf("Failed to initialize pubsub: %v", err)
	}
	defer ps.Close()
	log.Println("Connected to Redis PubSub")

	// Get ICE servers
	iceServers, err := cfg.WebRTC.GetICEServers()
	if err != nil {
		log.Printf("Warning: Failed to get ICE servers: %v", err)
		iceServers = nil
	}
	log.Printf("ICE servers configured: %d", len(iceServers))

	// Initialize peer manager
	peerMgr := webrtc.NewPeerManager(iceServers)

	// Initialize transcoder
	transcoder := service.NewTranscoder(cfg.HLS, cfg.FFmpeg)

	// Create context for initialization
	initCtx := context.Background()

	// Initialize S3 storage for VOD if configured
	var s3Storage storage.Storage
	var vodManager *service.VODManager

	if cfg.Storage.VOD.Enabled && cfg.Storage.S3.Bucket != "" {
		var err error
		s3Storage, err = storage.NewS3Storage(initCtx, storage.S3Config{
			Endpoint:        cfg.Storage.S3.Endpoint,
			Region:          cfg.Storage.S3.Region,
			Bucket:          cfg.Storage.S3.Bucket,
			AccessKeyID:     cfg.Storage.S3.AccessKeyID,
			SecretAccessKey: cfg.Storage.S3.SecretAccessKey,
			UsePathStyle:    cfg.Storage.S3.UsePathStyle,
			PublicURL:       cfg.Storage.S3.PublicURL,
		})
		if err != nil {
			log.Printf("Warning: Failed to initialize S3 storage: %v", err)
		} else {
			log.Println("S3 storage initialized for VOD")
		}
	}

	// Initialize session store based on config
	var sessionStore service.SessionStore
	switch cfg.Storage.Session.Type {
	case "redis":
		redisCfg := cfg.Storage.Session.Redis
		// If address is not set, use pubsub redis settings
		if redisCfg.Address == "" {
			redisCfg.Address = cfg.PubSub.Redis.Address
			redisCfg.Password = cfg.PubSub.Redis.Password
		}

		store, err := service.NewRedisSessionStore(redisCfg)
		if err != nil {
			log.Fatalf("Failed to create Redis session store: %v", err)
		}
		sessionStore = store
		log.Println("Using Redis session store")
	default:
		sessionStore = service.NewMemorySessionStore()
		log.Println("Using in-memory session store")
	}

	// Initialize VOD manager if enabled
	if cfg.Storage.VOD.Enabled {
		vodManager = service.NewVODManager(service.VODManagerConfig{
			S3Storage:      s3Storage,
			HLSOutputDir:   cfg.HLS.OutputDir,
			VODConfig:      cfg.Storage.VOD,
			TargetDuration: cfg.HLS.SegmentDuration,
			SessionStore:   sessionStore,
		})
		vodManager.Start()
		defer vodManager.Stop()
		log.Println("VOD manager initialized")
	}

	// Initialize thumbnail service if enabled and VOD manager has uploader
	var thumbnailService *service.ThumbnailService
	if cfg.Preview.Enabled && vodManager != nil && vodManager.GetUploader() != nil {
		thumbnailService = service.NewThumbnailService(service.ThumbnailServiceConfig{
			PreviewConfig: cfg.Preview,
			HLSOutputDir:  cfg.HLS.OutputDir,
			Uploader:      vodManager.GetUploader(),
		})
		defer thumbnailService.Stop()
		log.Println("Thumbnail service initialized")
	}

	// Initialize media service
	mediaSvc := service.NewMediaService(peerMgr, transcoder, ps, vodManager, thumbnailService)

	// Start service (subscribes to events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mediaSvc.Start(ctx); err != nil {
		log.Fatalf("Failed to start media service: %v", err)
	}
	defer mediaSvc.Stop()

	// Setup HTTP server
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Media Service listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Media Service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Media Service stopped")
}
