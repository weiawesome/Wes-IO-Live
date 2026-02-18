package main

import (
	"fmt"
	"net"

	"google.golang.org/grpc"

	"github.com/weiawesome/wes-io-live/auth-service/internal/config"
	"github.com/weiawesome/wes-io-live/auth-service/internal/service"
	"github.com/weiawesome/wes-io-live/pkg/jwt"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/auth"
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
		ServiceName: "auth-service",
	})
	logger := pkglog.L()

	// Initialize JWT manager
	jwtManager, err := jwt.NewManager(
		cfg.JWT.AccessDuration,
		cfg.JWT.RefreshDuration,
		cfg.JWT.Issuer,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create JWT manager")
	}

	// Create gRPC server with logging interceptors
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(pkglog.UnaryServerInterceptor(logger)),
		grpc.StreamInterceptor(pkglog.StreamServerInterceptor(logger)),
	)
	authService := service.NewAuthService(jwtManager)
	pb.RegisterAuthServiceServer(grpcServer, authService)

	// Start listening
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.GRPCPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal().Str("addr", addr).Err(err).Msg("failed to listen")
	}

	logger.Info().Str("addr", addr).Msg("auth service starting")
	if err := grpcServer.Serve(listener); err != nil {
		logger.Fatal().Err(err).Msg("failed to serve")
	}
}
