package service

import (
	"context"

	"github.com/weiawesome/wes-io-live/pkg/jwt"
	"github.com/weiawesome/wes-io-live/pkg/log"
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
	l := log.Ctx(ctx)
	l.Info().Str(log.FieldUserID, req.UserId).Msg("generating tokens")

	accessToken, refreshToken, accessExp, refreshExp, err := s.jwtManager.GenerateTokenPair(
		req.UserId,
		req.Email,
		req.Username,
		req.Roles,
	)
	if err != nil {
		l.Error().Err(err).Str(log.FieldUserID, req.UserId).Msg("failed to generate tokens")
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
	l := log.Ctx(ctx)

	claims, err := s.jwtManager.ValidateToken(req.AccessToken)
	if err != nil {
		l.Warn().Err(err).Msg("token validation failed")
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
	l := log.Ctx(ctx)
	l.Info().Msg("refreshing token")

	accessToken, refreshToken, accessExp, refreshExp, err := s.jwtManager.RefreshTokens(req.RefreshToken)
	if err != nil {
		l.Warn().Err(err).Msg("token refresh failed")
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
	l := log.Ctx(ctx)
	l.Info().Str(log.FieldUserID, req.UserId).Msg("revoking tokens")

	s.jwtManager.RevokeUserTokens(req.UserId)

	return &pb.RevokeTokensResponse{
		Success: true,
	}, nil
}
