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
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/config"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/domain"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/handler"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/consumer"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/reconciler"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/repository"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/service"
	"github.com/weiawesome/wes-io-live/social-graph-service/internal/store"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load config")
	}

	// 2. Initialize structured logger
	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		Pretty:      cfg.Log.Level == "debug",
		ServiceName: "social-graph-service",
	})
	logger := pkglog.L()

	// 3. Init DB (GORM, auto-migrate FollowModel)
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

	sqlDB, err := db.DB()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to get underlying sql.DB")
	}
	defer sqlDB.Close()

	if err := database.AutoMigrate(db, &domain.FollowModel{}); err != nil {
		logger.Fatal().Err(err).Msg("failed to auto-migrate")
	}
	logger.Info().Msg("database migration completed")

	// Drop old full unique index (if it exists from a previous run without soft delete)
	if err := db.Exec(`DROP INDEX IF EXISTS uidx_follow_pair`).Error; err != nil {
		logger.Warn().Err(err).Msg("could not drop old unique index")
	}

	// Partial unique index: only one ACTIVE follow per (follower, following) pair.
	// This allows re-follows after unfollow (soft-deleted rows are excluded).
	if err := db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uidx_follow_pair_active
		 ON follows (follower_id, following_id)
		 WHERE deleted_at IS NULL`,
	).Error; err != nil {
		logger.Fatal().Err(err).Msg("failed to create partial unique index uidx_follow_pair_active")
	}
	logger.Info().Msg("partial unique index ensured")

	// Enable FULL replica identity so that hard-delete CDC events ("d" op) carry
	// the complete before-row, allowing the consumer to decrement follower counts reliably.
	if err := db.Exec(`ALTER TABLE follows REPLICA IDENTITY FULL`).Error; err != nil {
		logger.Fatal().Err(err).Msg("failed to set REPLICA IDENTITY FULL on follows table")
	}
	logger.Info().Msg("follows table REPLICA IDENTITY FULL set")

	// 4. Init Redis client
	redisStore, err := store.NewRedisFollowStore(cfg.Redis.Address, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisStore.Close()
	logger.Info().Str("addr", cfg.Redis.Address).Msg("redis connected")

	// 5. Create repo, store, svc
	followRepo := repository.NewGormFollowRepository(db)
	svc := service.NewSocialGraphService(followRepo, redisStore)

	// 6. Create auth gRPC middleware
	authMiddleware, err := middleware.NewAuthMiddleware(cfg.Auth.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create auth middleware")
	}

	// 7. Init Kafka consumer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var kafkaConsumer *consumer.ConfluentConsumer
	if cfg.Kafka.Brokers != "" {
		kc, err := consumer.NewConfluentConsumer(
			cfg.Kafka.Brokers,
			cfg.Kafka.Topic,
			cfg.Kafka.GroupID,
			svc, // service implements CDCEventHandler
		)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to create kafka consumer, CDC updates disabled")
		} else {
			if err := kc.Start(ctx); err != nil {
				logger.Warn().Err(err).Msg("failed to start kafka consumer")
			} else {
				kafkaConsumer = kc
				logger.Info().Str("topic", cfg.Kafka.Topic).Msg("kafka CDC consumer started")
			}
		}
	} else {
		logger.Warn().Msg("KAFKA_BROKERS not configured; CDC consumer disabled")
	}

	// 8. Init reconciler and start
	rec := reconciler.New(redisStore, followRepo, cfg.Reconciler)
	rec.Start(ctx)
	logger.Info().Dur("interval", cfg.Reconciler.Interval).Int("top_n", cfg.Reconciler.TopN).Msg("reconciler started")

	// 9. Setup Gin router + HTTP server
	httpHandler := handler.NewHandler(svc, authMiddleware)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(pkglog.GinMiddleware(logger))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	httpHandler.RegisterRoutes(r)

	// 10. Start server goroutine
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		logger.Info().Str("addr", addr).Msg("social-graph-service starting")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	// 11. Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("shutdown signal received")

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		// 1. cancel() — stop Kafka consumer loop and reconciler ticker
		cancel()

		// 2. kafkaConsumer.Close() — wait for in-flight CDC message
		if kafkaConsumer != nil {
			if err := kafkaConsumer.Close(); err != nil {
				logger.Warn().Err(err).Msg("error closing kafka consumer")
			}
		}

		// 3. reconciler.Stop() — stop ticker; <-reconciler.Done()
		rec.Stop()
		<-rec.Done()

		// 4. server.Shutdown(5s) — drain HTTP
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn().Err(err).Msg("HTTP server forced to shutdown")
		}
	}()

	select {
	case <-shutdownDone:
		logger.Info().Msg("social-graph-service stopped")
	case <-time.After(30 * time.Second):
		logger.Warn().Msg("shutdown timed out after 30s")
	}
}
