package domain

import (
	"time"

	"github.com/weiawesome/wes-io-live/pkg/database"
	"gorm.io/gorm"
)

// UserModel is the GORM model for users table.
type UserModel struct {
	ID           string                 `gorm:"type:varchar(36);primaryKey"`
	Email        string                 `gorm:"type:varchar(255);uniqueIndex;not null"`
	Username     string                 `gorm:"type:varchar(50);uniqueIndex;not null"`
	DisplayName  string                 `gorm:"type:varchar(100)"`
	PasswordHash string                 `gorm:"type:varchar(255);not null"`
	Roles        database.StringArray   `gorm:"type:text"`
	CreatedAt    time.Time              `gorm:"autoCreateTime"`
	UpdatedAt    time.Time              `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt         `gorm:"index"`
}

// TableName specifies the table name for UserModel.
func (UserModel) TableName() string {
	return "users"
}

// ToDomain converts UserModel to domain User.
func (m *UserModel) ToDomain() *User {
	return &User{
		ID:           m.ID,
		Email:        m.Email,
		Username:     m.Username,
		DisplayName:  m.DisplayName,
		PasswordHash: m.PasswordHash,
		Roles:        []string(m.Roles),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// UserToModel converts domain User to UserModel.
func UserToModel(u *User) *UserModel {
	return &UserModel{
		ID:           u.ID,
		Email:        u.Email,
		Username:     u.Username,
		DisplayName:  u.DisplayName,
		PasswordHash: u.PasswordHash,
		Roles:        database.StringArray(u.Roles),
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}
