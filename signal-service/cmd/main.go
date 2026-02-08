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

	"github.com/weiawesome/wes-io-live/pkg/pubsub"
	"github.com/weiawesome/wes-io-live/signal-service/internal/client"
	"github.com/weiawesome/wes-io-live/signal-service/internal/config"
	"github.com/weiawesome/wes-io-live/signal-service/internal/handler"
	"github.com/weiawesome/wes-io-live/signal-service/internal/hub"
	"github.com/weiawesome/wes-io-live/signal-service/internal/kafka"
	"github.com/weiawesome/wes-io-live/signal-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Signal Service on %s:%d", cfg.Server.Host, cfg.Server.Port)

	// Initialize PubSub
	ps, err := pubsub.NewPubSub(cfg.PubSub)
	if err != nil {
		log.Fatalf("Failed to initialize pubsub: %v", err)
	}
	defer ps.Close()
	log.Println("Connected to Redis PubSub")

	// Initialize clients
	authClient, err := client.NewAuthClient(cfg.Auth.GRPCAddress)
	if err != nil {
		log.Fatalf("Failed to create auth client: %v", err)
	}
	defer authClient.Close()
	log.Printf("Connected to Auth Service at %s", cfg.Auth.GRPCAddress)

	roomClient := client.NewRoomClient(cfg.Room.HTTPAddress, cfg.Room.CacheTTL)
	log.Printf("Room Service client configured for %s", cfg.Room.HTTPAddress)

	// Initialize Kafka producer for broadcast events
	var kafkaProducer kafka.BroadcastEventProducer
	kafkaProducer, err = kafka.NewConfluentProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.Partitions)
	if err != nil {
		log.Printf("Warning: Failed to create Kafka producer: %v (broadcast events will not be sent)", err)
		kafkaProducer = nil // Service will work without Kafka
	} else {
		defer kafkaProducer.Close()
		log.Printf("Connected to Kafka at %s, topic: %s", cfg.Kafka.Brokers, cfg.Kafka.Topic)
	}

	// Initialize hub
	wsHub := hub.NewHub(cfg.WebSocket)
	go wsHub.Run()

	// Initialize service
	signalSvc := service.NewSignalService(wsHub, authClient, roomClient, ps, kafkaProducer)

	// Start service (subscribes to events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := signalSvc.Start(ctx); err != nil {
		log.Fatalf("Failed to start signal service: %v", err)
	}
	defer signalSvc.Stop()

	// Initialize handler
	wsHandler := handler.NewWSHandler(wsHub, signalSvc)

	// Setup HTTP server
	mux := http.NewServeMux()
	wsHandler.RegisterRoutes(mux)

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
		log.Printf("Signal Service listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Signal Service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Signal Service stopped")
}
