package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"

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
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load configuration")
	}

	// Initialize structured logger
	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		ServiceName: "media-service",
	})
	logger := pkglog.L()

	logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("starting media-service")

	// Ensure HLS output directory exists
	if err := os.MkdirAll(cfg.HLS.OutputDir, 0755); err != nil {
		logger.Fatal().Err(err).Str("dir", cfg.HLS.OutputDir).Msg("failed to create hls output directory")
	}

	// Initialize PubSub
	ps, err := pubsub.NewPubSub(cfg.PubSub)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize pubsub")
	}
	defer ps.Close()
	logger.Info().Str("driver", cfg.PubSub.Driver).Msg("pubsub connected")

	// Get ICE servers
	iceServers, err := cfg.WebRTC.GetICEServers()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to get ice servers")
		iceServers = nil
	}
	logger.Info().Int("count", len(iceServers)).Msg("ice servers configured")

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
			logger.Warn().Err(err).Msg("failed to initialize s3 storage")
		} else {
			logger.Info().Msg("s3 storage initialized for vod")
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
			logger.Fatal().Err(err).Msg("failed to create redis session store")
		}
		sessionStore = store
		logger.Info().Msg("using redis session store")
	default:
		sessionStore = service.NewMemorySessionStore()
		logger.Info().Msg("using in-memory session store")
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
		logger.Info().Msg("vod manager initialized")
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
		logger.Info().Msg("thumbnail service initialized")
	}

	// Initialize media service
	mediaSvc := service.NewMediaService(peerMgr, transcoder, ps, vodManager, thumbnailService)

	// Start service (subscribes to events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mediaSvc.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to start media service")
	}
	defer mediaSvc.Stop()

	// Setup HTTP server
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	handler := pkglog.HTTPMiddleware(logger)(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("media-service listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down media-service")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("media-service stopped")
}
