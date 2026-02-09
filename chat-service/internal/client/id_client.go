package client

import (
	"context"
	"fmt"

	pb "github.com/weiawesome/wes-io-live/proto/id"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type IDClient struct {
	conn   *grpc.ClientConn
	client pb.IDServiceClient
}

func NewIDClient(address string) (*IDClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to id service: %w", err)
	}

	return &IDClient{
		conn:   conn,
		client: pb.NewIDServiceClient(conn),
	}, nil
}

// GenerateID generates a single Snowflake ID from the id-service.
func (c *IDClient) GenerateID(ctx context.Context) (string, error) {
	resp, err := c.client.GenerateID(ctx, &pb.GenerateIDRequest{
		Type: pb.IDType_ID_TYPE_SNOWFLAKE,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}
	return resp.GetId(), nil
}

func (c *IDClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
