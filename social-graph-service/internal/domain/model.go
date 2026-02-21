package domain

import (
	"time"

	"gorm.io/gorm"
)

// FollowModel is the GORM model for the follows table.
type FollowModel struct {
	ID          uint           `gorm:"primaryKey;autoIncrement"`
	FollowerID  string         `gorm:"column:follower_id;type:varchar(36);not null"`
	FollowingID string         `gorm:"column:following_id;type:varchar(36);not null"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (FollowModel) TableName() string { return "follows" }

// Follow is the domain representation of a follow relationship.
type Follow struct {
	FollowerID  string
	FollowingID string
	CreatedAt   time.Time
}
