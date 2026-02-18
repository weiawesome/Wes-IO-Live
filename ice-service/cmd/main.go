package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/ice-service/internal/config"
	"github.com/weiawesome/wes-io-live/ice-service/internal/handler"
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
		ServiceName: "ice-service",
	})
	logger := pkglog.L()

	logger.Info().Str("host", cfg.Server.Host).Int("port", cfg.Server.Port).Msg("starting ice service")

	// Get ICE servers
	logger.Info().Msg("fetching ICE servers")
	iceServers, err := cfg.WebRTC.GetICEServers()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to get ice servers")
		iceServers = nil
	}
	logger.Info().Int("count", len(iceServers)).Msg("ice servers configured")
	for i, server := range iceServers {
		logger.Info().Int("index", i+1).Strs("urls", server.URLs).Msg("ice server")
	}

	// Initialize handler
	iceHandler := handler.NewICEHandler(iceServers)

	// Setup Gin router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(pkglog.GinMiddleware(logger))

	// Register routes
	iceHandler.RegisterRoutes(r)

	// Health check endpoints
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info().Str("addr", server.Addr).Msg("ice service listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down ice service")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("ice service stopped")
}
