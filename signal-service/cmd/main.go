package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
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
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load configuration")
	}

	pkglog.Init(pkglog.Config{Level: cfg.Log.Level, ServiceName: "signal-service"})
	logger := pkglog.L()

	logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("starting signal-service")

	// Initialize PubSub
	ps, err := pubsub.NewPubSub(cfg.PubSub)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize pubsub")
	}
	defer ps.Close()
	logger.Info().Msg("connected to redis pubsub")

	// Initialize clients
	authClient, err := client.NewAuthClient(cfg.Auth.GRPCAddress)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create auth client")
	}
	defer authClient.Close()
	logger.Info().Str("address", cfg.Auth.GRPCAddress).Msg("connected to auth service")

	roomClient := client.NewRoomClient(cfg.Room.HTTPAddress, cfg.Room.CacheTTL)
	logger.Info().Str("address", cfg.Room.HTTPAddress).Msg("room service client configured")

	// Initialize Kafka producer for broadcast events
	var kafkaProducer kafka.BroadcastEventProducer
	kafkaProducer, err = kafka.NewConfluentProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.Partitions)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to create kafka producer, broadcast events disabled")
		kafkaProducer = nil // Service will work without Kafka
	} else {
		defer kafkaProducer.Close()
		logger.Info().Str("brokers", cfg.Kafka.Brokers).Str("topic", cfg.Kafka.Topic).Msg("connected to kafka")
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
		logger.Fatal().Err(err).Msg("failed to start signal service")
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
		Handler:      pkglog.HTTPMiddleware(logger)(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("signal-service listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down signal-service")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("signal-service stopped")
}
