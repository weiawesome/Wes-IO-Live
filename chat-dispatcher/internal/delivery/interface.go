package delivery

import (
	"context"

	pb "github.com/weiawesome/wes-io-live/proto/chat"
)

// DeliveryClient sends messages to chat-service instances via gRPC.
type DeliveryClient interface {
	DeliverMessage(ctx context.Context, addr string, req *pb.DeliverMessageRequest) (*pb.DeliverMessageResponse, error)
	Close() error
}
