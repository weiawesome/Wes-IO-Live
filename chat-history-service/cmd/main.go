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

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/cache"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/handler"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/repository"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/service"
)

func main() {
	configPath := "config/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize structured logger
	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		Pretty:      cfg.Log.Level == "debug",
		ServiceName: "chat-history-service",
	})
	logger := pkglog.L()

	// Initialize Cassandra repository
	repo, err := repository.NewCassandraMessageRepository(cfg.Cassandra)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create cassandra repository")
	}
	defer repo.Close()

	// Initialize Redis cache
	msgCache, err := cache.NewRedisMessageCache(cfg.Redis, cfg.Cache.Prefix)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create redis cache")
	}
	defer msgCache.Close()

	// Initialize service
	chatHistoryService := service.NewChatHistoryService(repo, msgCache, cfg.Cache.TTL)

	// Initialize HTTP handler
	httpHandler := handler.NewHTTPHandler(chatHistoryService)

	// Setup Gin router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkglog.GinMiddleware(logger))

	httpHandler.RegisterRoutes(router)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		logger.Info().Str("addr", addr).Msg("starting chat-history-service")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("server exited")
}
