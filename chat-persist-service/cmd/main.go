package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/cassandra"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/config"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/consumer"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load config")
	}

	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		Pretty:      cfg.Log.Level == "debug",
		ServiceName: "chat-persist-service",
	})
	logger := pkglog.L()

	logger.Info().Msg("starting chat persist service")

	// Initialize Cassandra client
	cassandraClient, err := cassandra.NewClient(cfg.Cassandra)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to cassandra")
	}
	defer cassandraClient.Close()
	logger.Info().
		Str("keyspace", cfg.Cassandra.Keyspace).
		Strs("hosts", cfg.Cassandra.Hosts).
		Msg("connected to cassandra")

	// Initialize message repository
	repo := cassandra.NewMessageRepository(cassandraClient)

	// Initialize Kafka consumer
	cons, err := consumer.NewConsumer(cfg.Kafka, repo)
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
		logger.Info().
			Str("host", cfg.Server.Host).
			Int("port", cfg.Server.Port).
			Msg("health server listening")
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
	logger.Info().Msg("shutting down chat persist service")
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
	_ = server.Shutdown(shutdownCtx)

	logger.Info().Msg("chat persist service stopped")
}
