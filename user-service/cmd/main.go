package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/weiawesome/wes-io-live/pkg/database"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/middleware"
	"github.com/weiawesome/wes-io-live/pkg/storage"
	"github.com/weiawesome/wes-io-live/user-service/internal/cache"
	"github.com/weiawesome/wes-io-live/user-service/internal/config"
	"github.com/weiawesome/wes-io-live/user-service/internal/domain"
	"github.com/weiawesome/wes-io-live/user-service/internal/handler"
	"github.com/weiawesome/wes-io-live/user-service/internal/consumer"
	"github.com/weiawesome/wes-io-live/user-service/internal/repository"
	"github.com/weiawesome/wes-io-live/user-service/internal/service"
)

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
		ServiceName: "user-service",
	})
	logger := pkglog.L()

	// Connect to database using GORM
	dbConfig := &database.Config{
		Driver:          cfg.Database.Driver,
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		DBName:          cfg.Database.DBName,
		SSLMode:         cfg.Database.SSLMode,
		FilePath:        cfg.Database.FilePath,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	}

	db, err := database.New(dbConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}

	// Auto-migrate
	if err := database.AutoMigrate(db, &domain.UserModel{}); err != nil {
		logger.Fatal().Err(err).Msg("failed to auto-migrate")
	}
	logger.Info().Msg("database migration completed")

	// Initialize repository
	userRepo := repository.NewGormUserRepository(db)

	// Initialize Redis cache
	userCache, err := cache.NewRedisUserCache(cfg.Redis, cfg.Cache.Prefix)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer userCache.Close()
	logger.Info().Str("addr", cfg.Redis.Address).Msg("redis connected")

	// Initialize storage clients and build a bucket→storage map for the service.
	storages := make(map[string]storage.Storage)
	switch cfg.Storage.Type {
	case "s3":
		if cfg.Storage.S3.Endpoint != "" && cfg.Storage.S3.Bucket != "" {
			s3Storage, err := storage.NewS3Storage(context.Background(), cfg.Storage.S3)
			if err != nil {
				logger.Fatal().Err(err).Msg("failed to initialize raw-bucket storage")
			}
			storages[cfg.Storage.S3.Bucket] = s3Storage
			logger.Info().
				Str("endpoint", cfg.Storage.S3.Endpoint).
				Str("bucket", cfg.Storage.S3.Bucket).
				Msg("s3 raw-bucket storage initialized")
		} else {
			logger.Warn().Msg("s3 storage not configured; avatar upload will be unavailable")
		}
		// Second S3 client for the processed bucket (tagging and URL generation).
		if cfg.Storage.S3.Endpoint != "" && cfg.Storage.ProcessedBucket != "" {
			processedCfg := cfg.Storage.S3
			processedCfg.Bucket = cfg.Storage.ProcessedBucket
			processedS3, err := storage.NewS3Storage(context.Background(), processedCfg)
			if err != nil {
				logger.Fatal().Err(err).Msg("failed to initialize processed-bucket storage")
			}
			storages[cfg.Storage.ProcessedBucket] = processedS3
			logger.Info().Str("bucket", cfg.Storage.ProcessedBucket).Msg("s3 processed-bucket storage initialized")
		}
	case "local":
		localSt, err := storage.NewLocalStorage(storage.LocalConfig{BasePath: cfg.Storage.Local.BasePath})
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize local storage")
		}
		storages[cfg.Storage.S3.Bucket] = localSt
		logger.Info().Str("path", cfg.Storage.Local.BasePath).Msg("local storage initialized")
	default:
		logger.Warn().Str("type", cfg.Storage.Type).Msg("unknown storage type; avatar upload will be unavailable")
	}

	// Initialize service
	lc := service.NewLifecycleCfg(cfg.Lifecycle.TagKey, cfg.Lifecycle.TagValue)
	userService, err := service.NewUserService(
		userRepo,
		cfg.AuthService.GRPCAddress,
		userCache,
		cfg.Cache.TTL,
		storages,
		cfg.Storage.S3.Bucket,
		cfg.Storage.S3.PublicURL,
		lc,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create user service")
	}

	// Initialize Kafka consumer for avatar-processed events (if brokers configured).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var avatarConsumer *consumer.ConfluentConsumer
	if cfg.Kafka.Brokers != "" {
		avatarHandler := &avatarEventAdapter{svc: userService}
		avatarConsumer, err = consumer.NewConfluentConsumer(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.GroupID, avatarHandler)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to create kafka consumer")
		}
		if err := avatarConsumer.Start(ctx); err != nil {
			logger.Fatal().Err(err).Msg("failed to start kafka consumer")
		}
		logger.Info().Str("topic", cfg.Kafka.Topic).Msg("kafka consumer for avatar-processed started")
	} else {
		logger.Warn().Msg("KAFKA_BROKERS not configured; avatar-processed consumer disabled")
	}

	// Initialize auth middleware
	authMiddleware, err := middleware.NewAuthMiddleware(cfg.AuthService.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create auth middleware")
	}

	// Initialize HTTP handler
	httpHandler := handler.NewHandler(userService, authMiddleware)

	// Setup Gin router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(pkglog.GinMiddleware(logger))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	httpHandler.RegisterRoutes(r)

	// Start HTTP server in background so main goroutine can handle signals.
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		logger.Info().Str("addr", addr).Str("driver", cfg.Database.Driver).Msg("user-service starting")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	// Block until SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("shutdown signal received")

	// 1. Stop Kafka consumer (cancel context → consumeLoop exits → Close() drains doneCh).
	cancel()
	if avatarConsumer != nil {
		if err := avatarConsumer.Close(); err != nil {
			logger.Warn().Err(err).Msg("error closing kafka consumer")
		}
	}

	// 2. Gracefully drain in-flight HTTP requests (5 s budget).
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn().Err(err).Msg("HTTP server forced to shutdown")
	}

	logger.Info().Msg("user-service stopped")
}

// avatarEventAdapter bridges consumer.AvatarProcessedHandler → service.UserService.
type avatarEventAdapter struct {
	svc service.UserService
}

func (a *avatarEventAdapter) HandleAvatarProcessed(ctx context.Context, event *consumer.AvatarProcessedEvent) error {
	raw := domain.AvatarObjectRef{
		Bucket: event.Raw.Bucket,
		Key:    event.Raw.Key,
	}
	processed := domain.AvatarObjects{
		Sm: &domain.AvatarObjectRef{Bucket: event.Processed.Sm.Bucket, Key: event.Processed.Sm.Key},
		Md: &domain.AvatarObjectRef{Bucket: event.Processed.Md.Bucket, Key: event.Processed.Md.Key},
		Lg: &domain.AvatarObjectRef{Bucket: event.Processed.Lg.Bucket, Key: event.Processed.Lg.Key},
	}
	return a.svc.HandleAvatarProcessed(ctx, event.UserID, raw, processed)
}
