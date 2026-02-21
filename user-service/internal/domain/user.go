package domain

import (
	"time"
)

// AvatarObjectRef identifies a stored object by its bucket and key.
type AvatarObjectRef struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// AvatarObjects holds bucket+key refs for each avatar variant (raw + all processed sizes).
type AvatarObjects struct {
	Raw *AvatarObjectRef `json:"raw,omitempty"`
	Sm  *AvatarObjectRef `json:"sm,omitempty"`
	Md  *AvatarObjectRef `json:"md,omitempty"`
	Lg  *AvatarObjectRef `json:"lg,omitempty"`
}

// AvatarURLs holds generated URLs for the different avatar sizes (response DTO only).
type AvatarURLs struct {
	Sm string `json:"sm,omitempty"` // ~48px
	Md string `json:"md,omitempty"` // ~128px
	Lg string `json:"lg,omitempty"` // ~512px
}

// User represents a user entity.
type User struct {
	ID            string         `json:"id"`
	Email         string         `json:"email"`
	Username      string         `json:"username"`
	DisplayName   string         `json:"display_name,omitempty"`
	PasswordHash  string         `json:"-"`
	Roles         []string       `json:"roles"`
	AvatarObjects *AvatarObjects `json:"-"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// RegisterRequest represents a registration request.
type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Password    string `json:"password" binding:"required,min=6"`
	DisplayName string `json:"display_name"`
}

// LoginRequest represents a login request.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest represents a refresh token request.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UpdateUserRequest represents an update user request.
type UpdateUserRequest struct {
	DisplayName *string `json:"display_name"`
}

// ChangePasswordRequest represents a change password request.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
}

// AuthResponse represents authentication response with tokens.
type AuthResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresAt    int64        `json:"expires_at"`
}

// UserResponse represents a user in API responses.
type UserResponse struct {
	ID          string      `json:"id"`
	Email       string      `json:"email"`
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name,omitempty"`
	Roles       []string    `json:"roles"`
	AvatarURLs  *AvatarURLs `json:"avatar_urls,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// ToResponse converts User to UserResponse without avatar URLs.
// The service layer is responsible for populating AvatarURLs from AvatarObjects.
func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:          u.ID,
		Email:       u.Email,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Roles:       u.Roles,
		CreatedAt:   u.CreatedAt,
	}
}

// AvatarPresignRequest is the request body for presigning an avatar upload.
type AvatarPresignRequest struct {
	ContentType string `json:"content_type" binding:"required"`
}

// AvatarPresignResponse is returned when a presigned upload URL is generated.
type AvatarPresignResponse struct {
	UploadURL string `json:"upload_url"`
	Key       string `json:"key"`
	ExpiresIn int    `json:"expires_in"` // seconds
}
