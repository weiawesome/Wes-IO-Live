package log

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const metadataKeyRequestID = "x-request-id"

// UnaryServerInterceptor returns a gRPC unary server interceptor that
// creates a child logger with request metadata and injects it into context.
func UnaryServerInterceptor(logger zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		reqID := requestIDFromMD(ctx)
		child := logger.With().
			Str(FieldRequestID, reqID).
			Str(FieldGRPCMethod, info.FullMethod).
			Logger()

		ctx = WithLogger(ctx, child)

		resp, err := handler(ctx, req)

		code := status.Code(err)
		child.Info().
			Str(FieldGRPCCode, code.String()).
			Float64(FieldLatency, float64(time.Since(start).Milliseconds())).
			Err(err).
			Msg("unary call completed")

		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that
// creates a child logger with request metadata and injects it into context.
func StreamServerInterceptor(logger zerolog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		ctx := ss.Context()
		reqID := requestIDFromMD(ctx)
		child := logger.With().
			Str(FieldRequestID, reqID).
			Str(FieldGRPCMethod, info.FullMethod).
			Logger()

		wrapped := &wrappedStream{
			ServerStream: ss,
			ctx:          WithLogger(ctx, child),
		}

		err := handler(srv, wrapped)

		code := status.Code(err)
		child.Info().
			Str(FieldGRPCCode, code.String()).
			Float64(FieldLatency, float64(time.Since(start).Milliseconds())).
			Err(err).
			Msg("stream call completed")

		return err
	}
}

// wrappedStream overrides Context() to inject the child logger.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

func requestIDFromMD(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		vals := md.Get(metadataKeyRequestID)
		if len(vals) > 0 && vals[0] != "" {
			return vals[0]
		}
	}
	return uuid.New().String()
}
