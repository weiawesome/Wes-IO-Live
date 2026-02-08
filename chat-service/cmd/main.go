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

	"github.com/weiawesome/wes-io-live/chat-service/internal/client"
	"github.com/weiawesome/wes-io-live/chat-service/internal/config"
	chatgrpc "github.com/weiawesome/wes-io-live/chat-service/internal/grpc"
	"github.com/weiawesome/wes-io-live/chat-service/internal/handler"
	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
	"github.com/weiawesome/wes-io-live/chat-service/internal/kafka"
	"github.com/weiawesome/wes-io-live/chat-service/internal/registry"
	"github.com/weiawesome/wes-io-live/chat-service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting Chat Service on %s:%d", cfg.Server.Host, cfg.Server.Port)

	// Initialize Redis registry
	reg, err := registry.NewRedisRegistry(cfg.Redis, cfg.GRPC.AdvertiseAddress)
	if err != nil {
		log.Fatalf("Failed to initialize Redis registry: %v", err)
	}
	defer reg.Close()
	log.Printf("Connected to Redis at %s", cfg.Redis.Address)

	// Initialize Kafka producer
	producer, err := kafka.NewConfluentProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.Partitions)
	if err != nil {
		log.Fatalf("Failed to initialize Kafka producer: %v", err)
	}
	log.Printf("Connected to Kafka at %s (topic: %s)", cfg.Kafka.Brokers, cfg.Kafka.Topic)

	// Initialize Auth client
	authClient, err := client.NewAuthClient(cfg.Auth.GRPCAddress)
	if err != nil {
		log.Fatalf("Failed to create auth client: %v", err)
	}
	defer authClient.Close()
	log.Printf("Connected to Auth Service at %s", cfg.Auth.GRPCAddress)

	// Initialize ID client
	idClient, err := client.NewIDClient(cfg.IDService.GRPCAddress)
	if err != nil {
		log.Fatalf("Failed to create ID client: %v", err)
	}
	defer idClient.Close()
	log.Printf("Connected to ID Service at %s", cfg.IDService.GRPCAddress)

	// Initialize Hub
	wsHub := hub.NewHub(cfg.WebSocket)
	go wsHub.Run()

	// Initialize Chat Service
	chatSvc := service.NewChatService(wsHub, authClient, idClient, producer, reg)

	// Start service (heartbeat)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := chatSvc.Start(ctx); err != nil {
		log.Fatalf("Failed to start chat service: %v", err)
	}
	defer chatSvc.Stop()

	// Start gRPC server
	grpcAddr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, cfg.GRPC.Port)
	grpcServer, err := chatgrpc.StartGRPCServer(grpcAddr, wsHub)
	if err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
	defer grpcServer.GracefulStop()

	// Initialize WS handler
	wsHandler := handler.NewWSHandler(wsHub, chatSvc, cfg.WebSocket)

	// Setup HTTP server
	mux := http.NewServeMux()
	wsHandler.RegisterRoutes(mux)

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
		log.Printf("Chat Service listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Chat Service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Chat Service stopped")
}
