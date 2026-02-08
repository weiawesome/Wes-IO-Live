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

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
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
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting presence service on %s:%d", cfg.Server.Host, cfg.Server.Port)

	// Create Redis store (used for presence data + Publish)
	redisStore, err := store.NewRedisStore(store.RedisConfig{
		Address:       cfg.Redis.Address,
		Password:      cfg.Redis.Password,
		DB:            cfg.Redis.DB,
		PubSubChannel: cfg.Redis.PubSubChannel,
		InstanceID:    cfg.Server.InstanceID,
	})
	if err != nil {
		log.Fatalf("Failed to create Redis store: %v", err)
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
		log.Fatalf("Failed to create auth client: %v", err)
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
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		log.Fatalf("Failed to start presence service: %v", err)
	}

	// Start Redis Pub/Sub subscriber for multi-instance count sync
	subscriber := pubsub.NewSubscriber(redisPubSub, cfg.Redis.PubSubChannel, h, cfg.Server.InstanceID)
	go subscriber.Run(ctx)

	// Start Kafka consumer for broadcast events
	kafkaConsumer, err := kafka.NewConfluentConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.Topic,
		cfg.Kafka.GroupID,
		svc, // service implements BroadcastEventHandler
	)
	if err != nil {
		log.Printf("Warning: Failed to create Kafka consumer: %v (live room tracking disabled)", err)
	} else {
		if err := kafkaConsumer.Start(ctx); err != nil {
			log.Printf("Warning: Failed to start Kafka consumer: %v", err)
		} else {
			defer kafkaConsumer.Close()
			log.Printf("Kafka consumer started, subscribed to topic: %s", cfg.Kafka.Topic)
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
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Presence service listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down presence service...")

	// Stop service
	svc.Stop()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Presence service stopped")
}
