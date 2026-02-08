package client

import (
	"context"
	"fmt"

	"github.com/weiawesome/wes-io-live/proto/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AuthClient wraps the Auth Service gRPC client.
type AuthClient struct {
	conn   *grpc.ClientConn
	client auth.AuthServiceClient
}

// AuthResult represents the result of token validation.
type AuthResult struct {
	Valid    bool
	UserID   string
	Email    string
	Username string
	Roles    []string
	Error    string
}

// NewAuthClient creates a new Auth Service client.
func NewAuthClient(address string) (*AuthClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to auth service: %w", err)
	}

	return &AuthClient{
		conn:   conn,
		client: auth.NewAuthServiceClient(conn),
	}, nil
}

// ValidateToken validates a JWT token with the Auth Service.
func (c *AuthClient) ValidateToken(ctx context.Context, token string) (*AuthResult, error) {
	resp, err := c.client.ValidateToken(ctx, &auth.ValidateTokenRequest{
		AccessToken: token,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	return &AuthResult{
		Valid:    resp.GetValid(),
		UserID:   resp.GetUserId(),
		Email:    resp.GetEmail(),
		Username: resp.GetUsername(),
		Roles:    resp.GetRoles(),
		Error:    resp.GetErrorMessage(),
	}, nil
}

// Close closes the gRPC connection.
func (c *AuthClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
