package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
)

// GormRoomRepository implements RoomRepository using GORM.
type GormRoomRepository struct {
	db *gorm.DB
}

// NewGormRoomRepository creates a new GORM-based room repository.
func NewGormRoomRepository(db *gorm.DB) *GormRoomRepository {
	return &GormRoomRepository{db: db}
}

// Create creates a new room.
func (r *GormRoomRepository) Create(ctx context.Context, room *domain.Room) error {
	l := log.Ctx(ctx)

	room.ID = uuid.New().String()
	room.Status = domain.RoomStatusActive

	model := domain.RoomToModel(room)
	result := r.db.WithContext(ctx).Create(model)
	if result.Error != nil {
		l.Error().Err(result.Error).Msg("failed to create room in db")
		return result.Error
	}

	// Update the domain object with generated timestamps
	room.CreatedAt = model.CreatedAt
	l.Debug().Str("room_id", room.ID).Msg("room created in db")
	return nil
}

// GetByID retrieves a room by ID.
func (r *GormRoomRepository) GetByID(ctx context.Context, id string) (*domain.Room, error) {
	l := log.Ctx(ctx)

	var model domain.RoomModel
	result := r.db.WithContext(ctx).First(&model, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRoomNotFound
		}
		l.Error().Err(result.Error).Str("room_id", id).Msg("failed to get room by id")
		return nil, result.Error
	}
	return model.ToDomain(), nil
}

// List retrieves rooms with pagination.
func (r *GormRoomRepository) List(ctx context.Context, page, pageSize int, status string) ([]domain.Room, int, error) {
	l := log.Ctx(ctx)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Build query
	query := r.db.WithContext(ctx).Model(&domain.RoomModel{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		l.Error().Err(err).Msg("failed to count rooms")
		return nil, 0, err
	}

	// Get rooms
	var models []domain.RoomModel
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&models).Error; err != nil {
		l.Error().Err(err).Msg("failed to list rooms from db")
		return nil, 0, err
	}

	rooms := make([]domain.Room, len(models))
	for i, model := range models {
		rooms[i] = *model.ToDomain()
	}

	return rooms, int(total), nil
}

// Search searches rooms by title or description.
func (r *GormRoomRepository) Search(ctx context.Context, queryStr string, page, pageSize int) ([]domain.Room, int, error) {
	l := log.Ctx(ctx)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	searchPattern := "%" + queryStr + "%"

	// Build query - only search active rooms
	query := r.db.WithContext(ctx).Model(&domain.RoomModel{}).
		Where("status = ?", string(domain.RoomStatusActive)).
		Where("title LIKE ? OR description LIKE ?", searchPattern, searchPattern)

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		l.Error().Err(err).Msg("failed to count search results")
		return nil, 0, err
	}

	// Get rooms
	var models []domain.RoomModel
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&models).Error; err != nil {
		l.Error().Err(err).Msg("failed to search rooms from db")
		return nil, 0, err
	}

	rooms := make([]domain.Room, len(models))
	for i, model := range models {
		rooms[i] = *model.ToDomain()
	}

	return rooms, int(total), nil
}

// GetUserRooms retrieves rooms owned by a user.
func (r *GormRoomRepository) GetUserRooms(ctx context.Context, userID string) ([]domain.Room, error) {
	l := log.Ctx(ctx)

	var models []domain.RoomModel
	result := r.db.WithContext(ctx).
		Where("owner_id = ?", userID).
		Order("created_at DESC").
		Find(&models)
	if result.Error != nil {
		l.Error().Err(result.Error).Str(log.FieldUserID, userID).Msg("failed to get user rooms from db")
		return nil, result.Error
	}

	rooms := make([]domain.Room, len(models))
	for i, model := range models {
		rooms[i] = *model.ToDomain()
	}

	return rooms, nil
}

// CountActiveRoomsByUser counts active rooms owned by a user.
func (r *GormRoomRepository) CountActiveRoomsByUser(ctx context.Context, userID string) (int, error) {
	l := log.Ctx(ctx)

	var count int64
	result := r.db.WithContext(ctx).Model(&domain.RoomModel{}).
		Where("owner_id = ? AND status = ?", userID, string(domain.RoomStatusActive)).
		Count(&count)
	if result.Error != nil {
		l.Error().Err(result.Error).Str(log.FieldUserID, userID).Msg("failed to count active rooms")
	}
	return int(count), result.Error
}

// Close closes a room.
func (r *GormRoomRepository) Close(ctx context.Context, id string) error {
	l := log.Ctx(ctx)

	now := time.Now()
	result := r.db.WithContext(ctx).Model(&domain.RoomModel{}).
		Where("id = ? AND status = ?", id, string(domain.RoomStatusActive)).
		Updates(map[string]interface{}{
			"status":    string(domain.RoomStatusClosed),
			"closed_at": now,
		})
	if result.Error != nil {
		l.Error().Err(result.Error).Str("room_id", id).Msg("failed to close room in db")
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRoomNotFound
	}
	l.Debug().Str("room_id", id).Msg("room closed in db")
	return nil
}
