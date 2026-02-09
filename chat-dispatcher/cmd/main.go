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

	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/config"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/consumer"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/delivery"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/dispatcher"
	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/registry"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Chat Dispatcher")

	// Initialize Redis registry (read-only)
	reg, err := registry.NewRedisLookupClient(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to initialize Redis registry: %v", err)
	}
	log.Printf("Connected to Redis at %s", cfg.Redis.Address)

	// Initialize gRPC connection pool
	grpcPool := delivery.NewGRPCPool(cfg.GRPC)
	log.Printf("gRPC pool initialized (dial_timeout=%v, call_timeout=%v, idle_timeout=%v)",
		cfg.GRPC.DialTimeout, cfg.GRPC.CallTimeout, cfg.GRPC.IdleTimeout)

	// Initialize dispatcher
	d := dispatcher.NewDispatcher(reg, grpcPool)

	// Initialize Kafka consumer
	cons, err := consumer.NewConsumer(cfg.Kafka, d)
	if err != nil {
		log.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	log.Printf("Kafka consumer created (brokers=%s, topic=%s, group=%s)",
		cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.GroupID)

	// Start health HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		log.Printf("Health server listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Health server error: %v", err)
		}
	}()

	// Start consumer in background
	ctx, cancel := context.WithCancel(context.Background())

	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- cons.Run(ctx)
	}()

	// Wait for interrupt signal or fatal consumer error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Println("Received shutdown signal")
	case err := <-consumerDone:
		if err != nil {
			log.Printf("Consumer exited with error: %v", err)
		}
	}

	// Graceful shutdown
	log.Println("Shutting down Chat Dispatcher...")
	cancel()

	// Wait for consumer to stop
	select {
	case <-consumerDone:
	case <-time.After(10 * time.Second):
		log.Println("Consumer shutdown timed out")
	}

	cons.Close()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)

	grpcPool.Close()
	reg.Close()

	log.Println("Chat Dispatcher stopped")
}
