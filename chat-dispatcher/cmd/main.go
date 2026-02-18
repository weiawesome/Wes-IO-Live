package main

import (
	"context"
	"fmt"
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
		ServiceName: "chat-dispatcher",
	})
	logger := pkglog.L()

	logger.Info().Msg("starting chat dispatcher")

	// Initialize Redis registry (read-only)
	reg, err := registry.NewRedisLookupClient(cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize redis registry")
	}
	logger.Info().Str("address", cfg.Redis.Address).Msg("connected to redis")

	// Initialize gRPC connection pool
	grpcPool := delivery.NewGRPCPool(cfg.GRPC)
	logger.Info().
		Dur("dial_timeout", cfg.GRPC.DialTimeout).
		Dur("call_timeout", cfg.GRPC.CallTimeout).
		Dur("idle_timeout", cfg.GRPC.IdleTimeout).
		Msg("grpc pool initialized")

	// Initialize dispatcher
	d := dispatcher.NewDispatcher(reg, grpcPool)

	// Initialize Kafka consumer
	cons, err := consumer.NewConsumer(cfg.Kafka, d)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create kafka consumer")
	}
	logger.Info().
		Str("brokers", cfg.Kafka.Brokers).
		Str("topic", cfg.Kafka.Topic).
		Str("group", cfg.Kafka.GroupID).
		Msg("kafka consumer created")

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
		Handler:      pkglog.HTTPMiddleware(logger)(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("health server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("health server error")
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
		logger.Info().Msg("received shutdown signal")
	case err := <-consumerDone:
		if err != nil {
			logger.Error().Err(err).Msg("consumer exited with error")
		}
	}

	// Graceful shutdown
	logger.Info().Msg("shutting down chat dispatcher")
	cancel()

	// Wait for consumer to stop
	select {
	case <-consumerDone:
	case <-time.After(10 * time.Second):
		logger.Warn().Msg("consumer shutdown timed out")
	}

	cons.Close()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)

	grpcPool.Close()
	reg.Close()

	logger.Info().Msg("chat dispatcher stopped")
}
