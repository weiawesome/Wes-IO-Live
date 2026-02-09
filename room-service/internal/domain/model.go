package domain

import (
	"time"

	"github.com/weiawesome/wes-io-live/pkg/database"
	"gorm.io/gorm"
)

// RoomModel is the GORM model for rooms table.
type RoomModel struct {
	ID            string               `gorm:"type:varchar(36);primaryKey"`
	OwnerID       string               `gorm:"type:varchar(36);index;not null"`
	OwnerUsername string               `gorm:"type:varchar(50);not null"`
	Title         string               `gorm:"type:varchar(200);not null"`
	Description   string               `gorm:"type:text"`
	Status        string               `gorm:"type:varchar(20);index;not null;default:'active'"`
	ViewerCount   int                  `gorm:"default:0"`
	Tags          database.StringArray `gorm:"type:text"`
	CreatedAt     time.Time            `gorm:"autoCreateTime"`
	ClosedAt      *time.Time
	DeletedAt     gorm.DeletedAt       `gorm:"index"`
}

// TableName specifies the table name for RoomModel.
func (RoomModel) TableName() string {
	return "rooms"
}

// ToDomain converts RoomModel to domain Room.
func (m *RoomModel) ToDomain() *Room {
	return &Room{
		ID:            m.ID,
		OwnerID:       m.OwnerID,
		OwnerUsername: m.OwnerUsername,
		Title:         m.Title,
		Description:   m.Description,
		Status:        RoomStatus(m.Status),
		ViewerCount:   m.ViewerCount,
		Tags:          []string(m.Tags),
		CreatedAt:     m.CreatedAt,
		ClosedAt:      m.ClosedAt,
	}
}

// RoomToModel converts domain Room to RoomModel.
func RoomToModel(r *Room) *RoomModel {
	return &RoomModel{
		ID:            r.ID,
		OwnerID:       r.OwnerID,
		OwnerUsername: r.OwnerUsername,
		Title:         r.Title,
		Description:   r.Description,
		Status:        string(r.Status),
		ViewerCount:   r.ViewerCount,
		Tags:          database.StringArray(r.Tags),
		CreatedAt:     r.CreatedAt,
		ClosedAt:      r.ClosedAt,
	}
}
