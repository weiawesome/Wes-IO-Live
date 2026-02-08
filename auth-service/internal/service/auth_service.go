package service

import (
	"context"
	"log"

	"github.com/weiawesome/wes-io-live/pkg/jwt"
	pb "github.com/weiawesome/wes-io-live/proto/auth"
)

type AuthService struct {
	pb.UnimplementedAuthServiceServer
	jwtManager *jwt.Manager
}

func NewAuthService(jwtManager *jwt.Manager) *AuthService {
	return &AuthService{
		jwtManager: jwtManager,
	}
}

func (s *AuthService) GenerateTokens(ctx context.Context, req *pb.GenerateTokensRequest) (*pb.GenerateTokensResponse, error) {
	log.Printf("GenerateTokens called for user: %s", req.UserId)

	accessToken, refreshToken, accessExp, refreshExp, err := s.jwtManager.GenerateTokenPair(
		req.UserId,
		req.Email,
		req.Username,
		req.Roles,
	)
	if err != nil {
		log.Printf("Failed to generate tokens: %v", err)
		return nil, err
	}

	return &pb.GenerateTokensResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func (s *AuthService) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	claims, err := s.jwtManager.ValidateToken(req.AccessToken)
	if err != nil {
		log.Printf("Token validation failed: %v", err)
		return &pb.ValidateTokenResponse{
			Valid:        false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.ValidateTokenResponse{
		Valid:    true,
		UserId:   claims.UserID,
		Email:    claims.Email,
		Username: claims.Username,
		Roles:    claims.Roles,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	log.Printf("RefreshToken called")

	accessToken, refreshToken, accessExp, refreshExp, err := s.jwtManager.RefreshTokens(req.RefreshToken)
	if err != nil {
		log.Printf("Token refresh failed: %v", err)
		return &pb.RefreshTokenResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.RefreshTokenResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func (s *AuthService) RevokeTokens(ctx context.Context, req *pb.RevokeTokensRequest) (*pb.RevokeTokensResponse, error) {
	log.Printf("RevokeTokens called for user: %s", req.UserId)

	s.jwtManager.RevokeUserTokens(req.UserId)

	return &pb.RevokeTokensResponse{
		Success: true,
	}, nil
}
