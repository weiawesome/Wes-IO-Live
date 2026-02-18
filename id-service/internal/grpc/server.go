package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/rs/zerolog"
	"github.com/weiawesome/wes-io-live/id-service/internal/generator"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/id"
	"google.golang.org/grpc"
)

type idServer struct {
	pb.UnimplementedIDServiceServer
	generators map[pb.IDType]generator.Generator
}

func (s *idServer) getGenerator(t pb.IDType) (generator.Generator, error) {
	gen, ok := s.generators[t]
	if !ok {
		return nil, fmt.Errorf("unknown ID type: %v", t)
	}
	return gen, nil
}

func (s *idServer) GenerateID(ctx context.Context, req *pb.GenerateIDRequest) (*pb.GenerateIDResponse, error) {
	gen, err := s.getGenerator(req.GetType())
	if err != nil {
		return nil, err
	}

	id, err := gen.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ID: %w", err)
	}

	return &pb.GenerateIDResponse{Id: id}, nil
}

func (s *idServer) GenerateBatchIDs(ctx context.Context, req *pb.GenerateBatchIDsRequest) (*pb.GenerateBatchIDsResponse, error) {
	count := int(req.GetCount())
	if count < 1 || count > 1000 {
		return nil, fmt.Errorf("count must be between 1 and 1000, got %d", count)
	}

	gen, err := s.getGenerator(req.GetType())
	if err != nil {
		return nil, err
	}

	ids, err := gen.GenerateBatch(count)
	if err != nil {
		return nil, fmt.Errorf("failed to generate batch IDs: %w", err)
	}

	return &pb.GenerateBatchIDsResponse{Ids: ids}, nil
}

func (s *idServer) ValidateID(ctx context.Context, req *pb.ValidateIDRequest) (*pb.ValidateIDResponse, error) {
	gen, err := s.getGenerator(req.GetType())
	if err != nil {
		return nil, err
	}

	valid, reason := gen.Validate(req.GetId())
	return &pb.ValidateIDResponse{
		Valid:  valid,
		Reason: reason,
	}, nil
}

func (s *idServer) ParseID(ctx context.Context, req *pb.ParseIDRequest) (*pb.ParseIDResponse, error) {
	gen, err := s.getGenerator(req.GetType())
	if err != nil {
		return nil, err
	}

	result, err := gen.Parse(req.GetId())
	if err != nil {
		return &pb.ParseIDResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.ParseIDResponse{
		Valid:         true,
		TimestampMs:   result.TimestampMs,
		MachineId:     result.MachineID,
		Sequence:      result.Sequence,
		UuidVersion:   result.UUIDVersion,
		UuidVariant:   result.UUIDVariant,
		RandomPayload: result.RandomPayload,
		IdLength:      result.IDLength,
		Alphabet:      result.Alphabet,
	}, nil
}

// StartGRPCServer creates and starts the gRPC server in a background goroutine.
func StartGRPCServer(addr string, generators map[pb.IDType]generator.Generator, logger zerolog.Logger) (*grpc.Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(pkglog.UnaryServerInterceptor(logger)),
	)
	pb.RegisterIDServiceServer(s, &idServer{
		generators: generators,
	})

	go func() {
		logger.Info().Str("addr", addr).Msg("grpc server listening")
		if err := s.Serve(lis); err != nil {
			logger.Error().Err(err).Msg("grpc server error")
		}
	}()

	return s, nil
}
