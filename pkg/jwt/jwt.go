package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
	ErrRevokedToken = errors.New("token has been revoked")
)

// Claims represents JWT claims.
type Claims struct {
	jwt.RegisteredClaims
	UserID   string   `json:"user_id"`
	Email    string   `json:"email"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	Type     string   `json:"type"` // "access" or "refresh"
}

// Manager handles JWT operations.
type Manager struct {
	privateKey     *rsa.PrivateKey
	publicKey      *rsa.PublicKey
	accessDuration time.Duration
	refreshDuration time.Duration
	issuer         string

	// In-memory revocation store (use Redis in production)
	revokedTokens map[string]time.Time
	mu            sync.RWMutex
}

// NewManager creates a new JWT manager.
func NewManager(accessDuration, refreshDuration time.Duration, issuer string) (*Manager, error) {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	return &Manager{
		privateKey:      privateKey,
		publicKey:       &privateKey.PublicKey,
		accessDuration:  accessDuration,
		refreshDuration: refreshDuration,
		issuer:          issuer,
		revokedTokens:   make(map[string]time.Time),
	}, nil
}

// GenerateTokenPair creates access and refresh tokens.
func (m *Manager) GenerateTokenPair(userID, email, username string, roles []string) (accessToken, refreshToken string, accessExp, refreshExp int64, err error) {
	now := time.Now()

	// Access token
	accessExp = now.Add(m.accessDuration).Unix()
	accessClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessDuration)),
		},
		UserID:   userID,
		Email:    email,
		Username: username,
		Roles:    roles,
		Type:     "access",
	}

	accessToken, err = m.signToken(accessClaims)
	if err != nil {
		return "", "", 0, 0, err
	}

	// Refresh token
	refreshExp = now.Add(m.refreshDuration).Unix()
	refreshClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshDuration)),
		},
		UserID: userID,
		Type:   "refresh",
	}

	refreshToken, err = m.signToken(refreshClaims)
	if err != nil {
		return "", "", 0, 0, err
	}

	return accessToken, refreshToken, accessExp, refreshExp, nil
}

// ValidateToken validates a token and returns claims.
func (m *Manager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, ErrInvalidToken
		}
		return m.publicKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Check if token is revoked
	m.mu.RLock()
	if _, revoked := m.revokedTokens[claims.UserID]; revoked {
		m.mu.RUnlock()
		return nil, ErrRevokedToken
	}
	m.mu.RUnlock()

	return claims, nil
}

// RefreshTokens creates new token pair from a valid refresh token.
func (m *Manager) RefreshTokens(refreshTokenString string) (accessToken, refreshToken string, accessExp, refreshExp int64, err error) {
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return "", "", 0, 0, err
	}

	if claims.Type != "refresh" {
		return "", "", 0, 0, ErrInvalidToken
	}

	return m.GenerateTokenPair(claims.UserID, claims.Email, claims.Username, claims.Roles)
}

// RevokeUserTokens revokes all tokens for a user.
func (m *Manager) RevokeUserTokens(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokedTokens[userID] = time.Now().Add(m.refreshDuration)
}

// IsRevoked checks if user's tokens are revoked.
func (m *Manager) IsRevoked(userID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	expiry, exists := m.revokedTokens[userID]
	if !exists {
		return false
	}
	if time.Now().After(expiry) {
		delete(m.revokedTokens, userID)
		return false
	}
	return true
}

// CleanupExpiredRevocations removes expired revocation entries.
func (m *Manager) CleanupExpiredRevocations() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for userID, expiry := range m.revokedTokens {
		if now.After(expiry) {
			delete(m.revokedTokens, userID)
		}
	}
}

func (m *Manager) signToken(claims *Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.privateKey)
}
