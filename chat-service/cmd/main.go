package main

import (
	"context"
	"fmt"
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
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
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
		ServiceName: "chat-service",
	})
	logger := pkglog.L()

	logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("starting chat service")

	// Initialize Redis registry
	reg, err := registry.NewRedisRegistry(cfg.Redis, cfg.GRPC.AdvertiseAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize redis registry")
	}
	defer reg.Close()
	logger.Info().Str("address", cfg.Redis.Address).Msg("connected to redis")

	// Initialize Kafka producer
	producer, err := kafka.NewConfluentProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.Partitions)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize kafka producer")
	}
	logger.Info().Str("brokers", cfg.Kafka.Brokers).Str("topic", cfg.Kafka.Topic).Msg("connected to kafka")

	// Initialize Auth client
	authClient, err := client.NewAuthClient(cfg.Auth.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create auth client")
	}
	defer authClient.Close()
	logger.Info().Str("address", cfg.Auth.GRPCAddress).Msg("connected to auth service")

	// Initialize ID client
	idClient, err := client.NewIDClient(cfg.IDService.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create id client")
	}
	defer idClient.Close()
	logger.Info().Str("address", cfg.IDService.GRPCAddress).Msg("connected to id service")

	// Initialize Hub
	wsHub := hub.NewHub(cfg.WebSocket)
	go wsHub.Run()

	// Initialize Chat Service
	chatSvc := service.NewChatService(wsHub, authClient, idClient, producer, reg)

	// Start service (heartbeat)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := chatSvc.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("failed to start chat service")
	}
	defer chatSvc.Stop()

	// Start gRPC server
	grpcAddr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, cfg.GRPC.Port)
	grpcServer, err := chatgrpc.StartGRPCServer(grpcAddr, wsHub, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to start grpc server")
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
		Handler:      pkglog.HTTPMiddleware(logger)(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("chat service listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down chat service")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("chat service stopped")
}
