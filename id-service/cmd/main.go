package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/weiawesome/wes-io-live/id-service/internal/config"
	"github.com/weiawesome/wes-io-live/id-service/internal/generator"
	idgrpc "github.com/weiawesome/wes-io-live/id-service/internal/grpc"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/id"
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
		ServiceName: "id-service",
	})
	logger := pkglog.L()

	logger.Info().Msg("starting id-service")

	// Initialize Snowflake generator
	snowflake, err := generator.NewSnowflakeGenerator(cfg.Snowflake.MachineID, cfg.Snowflake.Epoch)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create snowflake generator")
	}
	logger.Info().Int64("machine_id", cfg.Snowflake.MachineID).Int64("epoch", cfg.Snowflake.Epoch).Msg("snowflake generator initialized")

	// Initialize UUID generator
	uuidGen := generator.NewUUIDGenerator()

	// Initialize ULID generator
	ulidGen := generator.NewULIDGenerator()

	// Initialize KSUID generator
	ksuidGen := generator.NewKSUIDGenerator()

	// Initialize NanoID generator
	nanoidGen, err := generator.NewNanoIDGenerator(cfg.NanoID.Size, cfg.NanoID.Alphabet)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create nanoid generator")
	}
	logger.Info().Int("size", cfg.NanoID.Size).Msg("nanoid generator initialized")

	// Initialize CUID2 generator
	cuid2Gen, err := generator.NewCUID2Generator(cfg.CUID2.Length)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create cuid2 generator")
	}
	logger.Info().Int("length", cfg.CUID2.Length).Msg("cuid2 generator initialized")

	// Assemble generator map
	generators := map[pb.IDType]generator.Generator{
		pb.IDType_ID_TYPE_SNOWFLAKE: snowflake,
		pb.IDType_ID_TYPE_UUID:      uuidGen,
		pb.IDType_ID_TYPE_ULID:      ulidGen,
		pb.IDType_ID_TYPE_KSUID:     ksuidGen,
		pb.IDType_ID_TYPE_NANOID:    nanoidGen,
		pb.IDType_ID_TYPE_CUID2:     cuid2Gen,
	}

	// Start gRPC server
	grpcAddr := fmt.Sprintf("%s:%d", cfg.GRPC.Host, cfg.GRPC.Port)
	grpcServer, err := idgrpc.StartGRPCServer(grpcAddr, generators, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to start grpc server")
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down id-service")
	grpcServer.GracefulStop()
	logger.Info().Msg("id-service stopped")
}
