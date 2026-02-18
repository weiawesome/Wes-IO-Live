package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
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
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize structured logger
	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		Pretty:      cfg.Log.Level == "debug",
		ServiceName: "playback-service",
	})
	logger := pkglog.L()

	logger.Info().Str("version", version).Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).
		Str("storage_type", cfg.Storage.Type).Str("access_mode", cfg.Playback.AccessMode).
		Msg("starting playback service")

	ctx := context.Background()

	// Initialize storage
	store, err := initStorage(ctx, cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize storage")
	}
	logger.Info().Msg("storage initialized successfully")

	// Initialize session store
	sessionStore, cleanup := initSessionStore(cfg)
	if cleanup != nil {
		defer cleanup()
	}
	logger.Info().Str("type", cfg.Session.Type).Msg("session store initialized")

	// Initialize content provider
	contentProvider := service.NewContentProvider(store, cfg.Playback, cfg.Storage.Type)
	logger.Info().Bool("redirect_mode", contentProvider.IsRedirectMode()).Msg("content provider initialized")

	// Initialize playback service
	playbackSvc := service.NewPlaybackService(contentProvider, sessionStore, cfg.Playback)

	// Initialize handlers
	healthHandler := handler.NewHealthHandler(version)
	liveHandler := handler.NewLiveHandler(playbackSvc)
	vodHandler := handler.NewVODHandler(playbackSvc)
	previewHandler := handler.NewPreviewHandler(playbackSvc)

	// Setup Gin router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(pkglog.GinMiddleware(logger))

	// Register routes
	healthHandler.RegisterRoutes(r)
	liveHandler.RegisterRoutes(r)
	vodHandler.RegisterRoutes(r)
	previewHandler.RegisterRoutes(r)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second, // Longer timeout for streaming
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("addr", server.Addr).Msg("playback service listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down playback service")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("playback service stopped")
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
	l := pkglog.L()
	switch cfg.Session.Type {
	case "redis":
		if cfg.Session.Redis.Address == "" {
			l.Warn().Msg("redis session store configured but address is empty, using no-op store")
			return service.NewNoOpSessionStore(), nil
		}

		store, err := service.NewRedisSessionStore(cfg.Session.Redis)
		if err != nil {
			l.Warn().Err(err).Msg("failed to connect to redis, using no-op store")
			return service.NewNoOpSessionStore(), nil
		}

		return store, func() {
			if err := store.Close(); err != nil {
				l.Error().Err(err).Msg("error closing redis connection")
			}
		}

	case "none", "":
		l.Warn().Msg("no session store configured, using no-op store")
		return service.NewNoOpSessionStore(), nil

	default:
		l.Warn().Str("type", cfg.Session.Type).Msg("unknown session type, using no-op store")
		return service.NewNoOpSessionStore(), nil
	}
}
