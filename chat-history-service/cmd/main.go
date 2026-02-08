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

	"github.com/weiawesome/wes-io-live/chat-history-service/internal/cache"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/handler"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/repository"
	"github.com/weiawesome/wes-io-live/chat-history-service/internal/service"
	"github.com/gin-gonic/gin"
)

func main() {
	configPath := "config/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize Cassandra repository
	repo, err := repository.NewCassandraMessageRepository(cfg.Cassandra)
	if err != nil {
		log.Fatalf("Failed to create cassandra repository: %v", err)
	}
	defer repo.Close()

	// Initialize Redis cache
	msgCache, err := cache.NewRedisMessageCache(cfg.Redis, cfg.Cache.Prefix)
	if err != nil {
		log.Fatalf("Failed to create redis cache: %v", err)
	}
	defer msgCache.Close()

	// Initialize service
	chatHistoryService := service.NewChatHistoryService(repo, msgCache, cfg.Cache.TTL)

	// Initialize HTTP handler
	httpHandler := handler.NewHTTPHandler(chatHistoryService)

	// Setup Gin router
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	httpHandler.RegisterRoutes(router)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.Printf("Starting chat-history-service on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}
