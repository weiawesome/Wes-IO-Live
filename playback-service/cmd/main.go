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

	"github.com/weiawesome/wes-io-live/pkg/storage"
	"github.com/weiawesome/wes-io-live/playback-service/internal/config"
	"github.com/weiawesome/wes-io-live/playback-service/internal/handler"
	"github.com/weiawesome/wes-io-live/playback-service/internal/service"
)

const version = "1.0.0"

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Playback Service v%s on %s:%d", version, cfg.Server.Host, cfg.Server.Port)
	log.Printf("Storage type: %s, Access mode: %s", cfg.Storage.Type, cfg.Playback.AccessMode)

	ctx := context.Background()

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	log.Printf("Storage initialized successfully")

	// Initialize session store
	sessionStore, cleanup := initSessionStore(cfg)
	if cleanup != nil {
		defer cleanup()
	}
	log.Printf("Session store type: %s", cfg.Session.Type)

	// Initialize content provider
	contentProvider := service.NewContentProvider(store, cfg.Playback, cfg.Storage.Type)
	log.Printf("Content provider initialized (redirect mode: %v)", contentProvider.IsRedirectMode())

	// Initialize playback service
	playbackSvc := service.NewPlaybackService(contentProvider, sessionStore, cfg.Playback)

	// Initialize handlers
	healthHandler := handler.NewHealthHandler(version)
	liveHandler := handler.NewLiveHandler(playbackSvc)
	vodHandler := handler.NewVODHandler(playbackSvc)
	previewHandler := handler.NewPreviewHandler(playbackSvc)

	// Setup HTTP server
	mux := http.NewServeMux()
	healthHandler.RegisterRoutes(mux)
	liveHandler.RegisterRoutes(mux)
	vodHandler.RegisterRoutes(mux)
	previewHandler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second, // Longer timeout for streaming
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Playback Service listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
		log.Printf("Endpoints:")
		log.Printf("  GET /health - Health check")
		log.Printf("  GET /live/{roomID}/stream.m3u8 - Live stream (auto-detect session)")
		log.Printf("  GET /live/{roomID}/{sessionID}/{file} - Live stream (explicit session)")
		log.Printf("  GET /vod/{roomID} - List VOD sessions")
		log.Printf("  GET /vod/{roomID}/latest - Get latest VOD URL")
		log.Printf("  GET /vod/{roomID}/{sessionID}/{file} - VOD content")
		log.Printf("  GET /preview/{roomID}/latest/thumbnail.jpg - Latest preview")
		log.Printf("  GET /preview/{roomID}/{sessionID}/thumbnail.jpg - Session preview")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Playback Service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Playback Service stopped")
}

// initStorage initializes the storage backend based on configuration.
func initStorage(ctx context.Context, cfg *config.Config) (storage.Storage, error) {
	switch cfg.Storage.Type {
	case "s3":
		return storage.NewS3Storage(ctx, storage.S3Config{
			Endpoint:        cfg.Storage.S3.Endpoint,
			Region:          cfg.Storage.S3.Region,
			Bucket:          cfg.Storage.S3.Bucket,
			AccessKeyID:     cfg.Storage.S3.AccessKeyID,
			SecretAccessKey: cfg.Storage.S3.SecretAccessKey,
			UsePathStyle:    cfg.Storage.S3.UsePathStyle,
			PublicURL:       cfg.Storage.S3.PublicURL,
		})
	case "local":
		return storage.NewLocalStorage(storage.LocalConfig{
			BasePath: cfg.Storage.Local.BasePath,
		})
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
}

// initSessionStore initializes the session store based on configuration.
// Returns the session store and a cleanup function.
func initSessionStore(cfg *config.Config) (service.SessionStore, func()) {
	switch cfg.Session.Type {
	case "redis":
		if cfg.Session.Redis.Address == "" {
			log.Printf("Warning: Redis session store configured but address is empty, using no-op store")
			return service.NewNoOpSessionStore(), nil
		}

		store, err := service.NewRedisSessionStore(cfg.Session.Redis)
		if err != nil {
			log.Printf("Warning: Failed to connect to Redis: %v, using no-op store", err)
			return service.NewNoOpSessionStore(), nil
		}

		return store, func() {
			if err := store.Close(); err != nil {
				log.Printf("Error closing Redis connection: %v", err)
			}
		}

	case "none", "":
		return service.NewNoOpSessionStore(), nil

	default:
		log.Printf("Warning: Unknown session type '%s', using no-op store", cfg.Session.Type)
		return service.NewNoOpSessionStore(), nil
	}
}
