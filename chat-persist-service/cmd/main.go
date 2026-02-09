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

	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/cassandra"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/consumer"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Chat Persist Service")

	// Initialize Cassandra client
	cassandraClient, err := cassandra.NewClient(cfg.Cassandra)
	if err != nil {
		log.Fatalf("Failed to connect to Cassandra: %v", err)
	}
	defer cassandraClient.Close()
	log.Printf("Connected to Cassandra (keyspace=%s, hosts=%v)",
		cfg.Cassandra.Keyspace, cfg.Cassandra.Hosts)

	// Initialize message repository
	repo := cassandra.NewMessageRepository(cassandraClient)

	// Initialize Kafka consumer
	cons, err := consumer.NewConsumer(cfg.Kafka, repo)
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
	log.Println("Shutting down Chat Persist Service...")
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

	log.Println("Chat Persist Service stopped")
}
