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

	"github.com/weiawesome/wes-io-live/ice-service/internal/config"
	"github.com/weiawesome/wes-io-live/ice-service/internal/handler"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting ICE Service on %s:%d", cfg.Server.Host, cfg.Server.Port)

	// Get ICE servers
	log.Printf("Fetching ICE servers...")
	iceServers, err := cfg.WebRTC.GetICEServers()
	if err != nil {
		log.Printf("Warning: Failed to get ICE servers: %v", err)
		iceServers = nil
	}
	log.Printf("ICE servers configured: %d", len(iceServers))
	for i, server := range iceServers {
		log.Printf("  Server %d: URLs=%v", i+1, server.URLs)
	}

	// Initialize handler
	iceHandler := handler.NewICEHandler(iceServers)

	// Setup HTTP server
	mux := http.NewServeMux()
	iceHandler.RegisterRoutes(mux)

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
		log.Printf("ICE Service listening on %s:%d", cfg.Server.Host, cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down ICE Service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("ICE Service stopped")
}
