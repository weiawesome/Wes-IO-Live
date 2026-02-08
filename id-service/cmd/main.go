package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/weiawesome/wes-io-live/id-service/internal/config"
	"github.com/weiawesome/wes-io-live/id-service/internal/generator"
	idgrpc "github.com/weiawesome/wes-io-live/id-service/internal/grpc"
	pb "github.com/weiawesome/wes-io-live/proto/id"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting ID Service")

	// Initialize Snowflake generator
	snowflake, err := generator.NewSnowflakeGenerator(cfg.Snowflake.MachineID, cfg.Snowflake.Epoch)
	if err != nil {
		log.Fatalf("Failed to create Snowflake generator: %v", err)
	}
	log.Printf("Snowflake generator initialized (machine_id=%d, epoch=%d)", cfg.Snowflake.MachineID, cfg.Snowflake.Epoch)

	// Initialize UUID generator
	uuidGen := generator.NewUUIDGenerator()

	// Initialize ULID generator
	ulidGen := generator.NewULIDGenerator()

	// Initialize KSUID generator
	ksuidGen := generator.NewKSUIDGenerator()

	// Initialize NanoID generator
	nanoidGen, err := generator.NewNanoIDGenerator(cfg.NanoID.Size, cfg.NanoID.Alphabet)
	if err != nil {
		log.Fatalf("Failed to create NanoID generator: %v", err)
	}
	log.Printf("NanoID generator initialized (size=%d)", cfg.NanoID.Size)

	// Initialize CUID2 generator
	cuid2Gen, err := generator.NewCUID2Generator(cfg.CUID2.Length)
	if err != nil {
		log.Fatalf("Failed to create CUID2 generator: %v", err)
	}
	log.Printf("CUID2 generator initialized (length=%d)", cfg.CUID2.Length)

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
	grpcServer, err := idgrpc.StartGRPCServer(grpcAddr, generators)
	if err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down ID Service...")
	grpcServer.GracefulStop()
	log.Println("ID Service stopped")
}
