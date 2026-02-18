package service

import (
	"context"
	"errors"

	"github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/room-service/internal/audit"
	"github.com/weiawesome/wes-io-live/room-service/internal/domain"
	"github.com/weiawesome/wes-io-live/room-service/internal/repository"
)

var (
	ErrRoomNotFound    = errors.New("room not found")
	ErrNotRoomOwner    = errors.New("you are not the owner of this room")
	ErrMaxRoomsReached = errors.New("maximum active rooms limit reached")
)

// roomServiceImpl implements RoomService interface.
type roomServiceImpl struct {
	repo            repository.RoomRepository
	maxRoomsPerUser int
}

// NewRoomService creates a new room service.
func NewRoomService(repo repository.RoomRepository, maxRoomsPerUser int) RoomService {
	return &roomServiceImpl{
		repo:            repo,
		maxRoomsPerUser: maxRoomsPerUser,
	}
}

// CreateRoom creates a new room.
func (s *roomServiceImpl) CreateRoom(ctx context.Context, userID, username string, req *domain.CreateRoomRequest) (*domain.RoomResponse, error) {
	l := log.Ctx(ctx)

	// Check if user has reached max rooms limit
	activeCount, err := s.repo.CountActiveRoomsByUser(ctx, userID)
	if err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to count active rooms")
		return nil, err
	}

	if activeCount >= s.maxRoomsPerUser {
		return nil, ErrMaxRoomsReached
	}

	// Create room
	room := &domain.Room{
		OwnerID:       userID,
		OwnerUsername: username,
		Title:         req.Title,
		Description:   req.Description,
		Tags:          req.Tags,
		ViewerCount:   0,
	}

	if err := s.repo.Create(ctx, room); err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to create room")
		return nil, err
	}

	audit.Log(ctx, audit.ActionCreateRoom, userID, "room created")

	resp := room.ToResponse()
	return &resp, nil
}

// GetRoom retrieves a room by ID.
func (s *roomServiceImpl) GetRoom(ctx context.Context, roomID string) (*domain.RoomResponse, error) {
	l := log.Ctx(ctx)

	room, err := s.repo.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repository.ErrRoomNotFound) {
			return nil, ErrRoomNotFound
		}
		l.Error().Err(err).Str(audit.FieldRoomID, roomID).Msg("failed to get room")
		return nil, err
	}

	resp := room.ToResponse()
	return &resp, nil
}

// ListRooms lists rooms with pagination.
func (s *roomServiceImpl) ListRooms(ctx context.Context, page, pageSize int, status string) (*domain.ListRoomsResponse, error) {
	l := log.Ctx(ctx)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Default to active rooms
	if status == "" {
		status = string(domain.RoomStatusActive)
	}

	rooms, total, err := s.repo.List(ctx, page, pageSize, status)
	if err != nil {
		l.Error().Err(err).Msg("failed to list rooms")
		return nil, err
	}

	roomResponses := make([]domain.RoomResponse, len(rooms))
	for i, room := range rooms {
		roomResponses[i] = room.ToResponse()
	}

	totalPages := (total + pageSize - 1) / pageSize

	return &domain.ListRoomsResponse{
		Rooms:      roomResponses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// SearchRooms searches rooms by query.
func (s *roomServiceImpl) SearchRooms(ctx context.Context, query string, page, pageSize int) (*domain.ListRoomsResponse, error) {
	l := log.Ctx(ctx)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	rooms, total, err := s.repo.Search(ctx, query, page, pageSize)
	if err != nil {
		l.Error().Err(err).Msg("failed to search rooms")
		return nil, err
	}

	roomResponses := make([]domain.RoomResponse, len(rooms))
	for i, room := range rooms {
		roomResponses[i] = room.ToResponse()
	}

	totalPages := (total + pageSize - 1) / pageSize

	return &domain.ListRoomsResponse{
		Rooms:      roomResponses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetMyRooms retrieves rooms owned by a user.
func (s *roomServiceImpl) GetMyRooms(ctx context.Context, userID string) ([]domain.RoomResponse, error) {
	l := log.Ctx(ctx)

	rooms, err := s.repo.GetUserRooms(ctx, userID)
	if err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get user rooms")
		return nil, err
	}

	roomResponses := make([]domain.RoomResponse, len(rooms))
	for i, room := range rooms {
		roomResponses[i] = room.ToResponse()
	}

	return roomResponses, nil
}

// CloseRoom closes a room.
func (s *roomServiceImpl) CloseRoom(ctx context.Context, userID, roomID string) error {
	l := log.Ctx(ctx)

	// Verify ownership
	room, err := s.repo.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repository.ErrRoomNotFound) {
			return ErrRoomNotFound
		}
		l.Error().Err(err).Str(audit.FieldRoomID, roomID).Msg("failed to get room for close")
		return err
	}

	if room.OwnerID != userID {
		return ErrNotRoomOwner
	}

	if err := s.repo.Close(ctx, roomID); err != nil {
		l.Error().Err(err).Str(audit.FieldRoomID, roomID).Msg("failed to close room")
		return err
	}

	audit.Log(ctx, audit.ActionCloseRoom, userID, "room closed")

	return nil
}

// GetRoomStats returns room statistics for a user.
func (s *roomServiceImpl) GetRoomStats(ctx context.Context, userID string) (activeCount, maxAllowed int, err error) {
	l := log.Ctx(ctx)

	activeCount, err = s.repo.CountActiveRoomsByUser(ctx, userID)
	if err != nil {
		l.Error().Err(err).Str(log.FieldUserID, userID).Msg("failed to get room stats")
		return 0, 0, err
	}
	return activeCount, s.maxRoomsPerUser, nil
}
