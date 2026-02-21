package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/presence-service/internal/client"
	"github.com/weiawesome/wes-io-live/presence-service/internal/config"
	"github.com/weiawesome/wes-io-live/presence-service/internal/handler"
	"github.com/weiawesome/wes-io-live/presence-service/internal/hub"
	"github.com/weiawesome/wes-io-live/presence-service/internal/kafka"
	"github.com/weiawesome/wes-io-live/presence-service/internal/pubsub"
	"github.com/weiawesome/wes-io-live/presence-service/internal/service"
	"github.com/weiawesome/wes-io-live/presence-service/internal/store"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load configuration")
	}

	// Initialize structured logger
	pkglog.Init(pkglog.Config{Level: cfg.Log.Level, ServiceName: "presence-service"})
	logger := pkglog.L()

	logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("starting presence-service")

	// Create Redis store (used for presence data + Publish)
	redisStore, err := store.NewRedisStore(store.RedisConfig{
		Address:       cfg.Redis.Address,
		Password:      cfg.Redis.Password,
		DB:            cfg.Redis.DB,
		PubSubChannel: cfg.Redis.PubSubChannel,
		InstanceID:    cfg.Server.InstanceID,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create redis store")
	}
	defer redisStore.Close()

	// Second Redis client for Subscribe (connection in subscriber mode cannot run other commands)
	redisPubSub := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisPubSub.Close()

	// Create auth client
	authClient, err := client.NewAuthClient(cfg.Auth.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create auth client")
	}
	defer authClient.Close()

	// Create hub
	hubConfig := hub.Config{
		PingInterval:   cfg.Presence.PingInterval,
		PongWait:       cfg.Presence.PongWait,
		WriteWait:      cfg.Presence.WriteWait,
		MaxMessageSize: cfg.Presence.MaxMessageSize,
	}
	h := hub.NewHub(hubConfig)
	go h.Run()

	// Create service
	svcConfig := service.Config{
		HeartbeatTimeout: cfg.Presence.HeartbeatTimeout,
		GracePeriod:      cfg.Kafka.GracePeriod,
	}
	svc := service.NewPresenceService(h, redisStore, authClient, svcConfig)

	// Start service background tasks
	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		cancel()
		logger.Fatal().Err(err).Msg("failed to start presence service")
	}

	// Start Redis Pub/Sub subscriber for multi-instance count sync
	subscriber := pubsub.NewSubscriber(redisPubSub, cfg.Redis.PubSubChannel, h, cfg.Server.InstanceID)
	go subscriber.Run(ctx)

	// Start Kafka consumer for broadcast events
	var kafkaConsumer *kafka.ConfluentConsumer
	if kc, err := kafka.NewConfluentConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.Topic,
		cfg.Kafka.GroupID,
		svc, // service implements BroadcastEventHandler
	); err != nil {
		logger.Warn().Err(err).Msg("failed to create kafka consumer, live room tracking disabled")
	} else {
		if err := kc.Start(ctx); err != nil {
			logger.Warn().Err(err).Msg("failed to start kafka consumer")
		} else {
			kafkaConsumer = kc
			logger.Info().Str("topic", cfg.Kafka.Topic).Msg("kafka consumer started")
		}
	}

	// Create handlers
	wsHandler := handler.NewWSHandler(h, svc)
	httpHandler := handler.NewHTTPHandler(svc)

	// Setup routes
	router := mux.NewRouter()

	// WebSocket endpoint
	router.HandleFunc("/presence", wsHandler.HandleWebSocket)

	// HTTP API endpoints
	router.HandleFunc("/api/v1/rooms/{room_id}/presence", httpHandler.GetPresence).Methods("GET")
	router.HandleFunc("/api/v1/rooms/{room_id}", httpHandler.GetRoomInfo).Methods("GET")
	router.HandleFunc("/api/v1/live-rooms", httpHandler.GetLiveRooms).Methods("GET")
	router.HandleFunc("/health", httpHandler.HealthCheck).Methods("GET")

	// Create server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      pkglog.HTTPMiddleware(logger)(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("addr", addr).Msg("presence-service listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down presence-service")

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		cancel() // 1. stop Kafka consumer + pubsub subscriber

		if kafkaConsumer != nil {
			kafkaConsumer.Close() // 2. wait for in-flight Kafka event
		}
		<-subscriber.Done() // 3. wait for pub/sub goroutine to exit

		h.Stop() // 4. close all WS clients, stop Hub.Run()

		svc.Stop() // 5. cancel grace period timers

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("server shutdown error")
		}
	}()

	select {
	case <-shutdownDone:
		logger.Info().Msg("presence-service stopped")
	case <-time.After(30 * time.Second):
		logger.Warn().Msg("shutdown timed out after 30s")
	}
}
