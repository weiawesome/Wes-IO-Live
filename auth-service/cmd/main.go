package main

import (
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/weiawesome/wes-io-live/auth-service/internal/config"
	"github.com/weiawesome/wes-io-live/auth-service/internal/service"
	"github.com/weiawesome/wes-io-live/pkg/jwt"
	pb "github.com/weiawesome/wes-io-live/proto/auth"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize JWT manager
	jwtManager, err := jwt.NewManager(
		cfg.JWT.AccessDuration,
		cfg.JWT.RefreshDuration,
		cfg.JWT.Issuer,
	)
	if err != nil {
		log.Fatalf("Failed to create JWT manager: %v", err)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()
	authService := service.NewAuthService(jwtManager)
	pb.RegisterAuthServiceServer(grpcServer, authService)

	// Start listening
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.GRPCPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	log.Printf("Auth Service starting on %s", addr)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
