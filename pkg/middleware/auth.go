package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	pb "github.com/weiawesome/wes-io-live/proto/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	UserIDKey     = "user_id"
	EmailKey      = "email"
	UsernameKey   = "username"
	RolesKey      = "roles"
	AuthHeaderKey = "Authorization"
	BearerPrefix  = "Bearer "
)

// AuthMiddleware validates JWT tokens via Auth Service.
type AuthMiddleware struct {
	authClient pb.AuthServiceClient
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(authServiceAddr string) (*AuthMiddleware, error) {
	conn, err := grpc.Dial(authServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &AuthMiddleware{
		authClient: pb.NewAuthServiceClient(conn),
	}, nil
}

// RequireAuth returns a Gin middleware that validates JWT tokens.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthHeaderKey)
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			return
		}

		if !strings.HasPrefix(authHeader, BearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization format",
			})
			return
		}

		token := strings.TrimPrefix(authHeader, BearerPrefix)
		resp, err := m.authClient.ValidateToken(context.Background(), &pb.ValidateTokenRequest{
			AccessToken: token,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "failed to validate token",
			})
			return
		}

		if !resp.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": resp.ErrorMessage,
			})
			return
		}

		// Set user info in context
		c.Set(UserIDKey, resp.UserId)
		c.Set(EmailKey, resp.Email)
		c.Set(UsernameKey, resp.Username)
		c.Set(RolesKey, resp.Roles)

		c.Next()
	}
}

// GetUserID extracts user ID from Gin context.
func GetUserID(c *gin.Context) string {
	if id, exists := c.Get(UserIDKey); exists {
		return id.(string)
	}
	return ""
}

// GetUsername extracts username from Gin context.
func GetUsername(c *gin.Context) string {
	if username, exists := c.Get(UsernameKey); exists {
		return username.(string)
	}
	return ""
}

// GetEmail extracts email from Gin context.
func GetEmail(c *gin.Context) string {
	if email, exists := c.Get(EmailKey); exists {
		return email.(string)
	}
	return ""
}

// GetRoles extracts roles from Gin context.
func GetRoles(c *gin.Context) []string {
	if roles, exists := c.Get(RolesKey); exists {
		return roles.([]string)
	}
	return nil
}
