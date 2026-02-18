package service

import (
	"context"
	"sync"
	"time"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/presence-service/internal/client"
	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
	"github.com/weiawesome/wes-io-live/presence-service/internal/hub"
	"github.com/weiawesome/wes-io-live/presence-service/internal/kafka"
	"github.com/weiawesome/wes-io-live/presence-service/internal/store"
)

// Config holds presence service configuration.
type Config struct {
	HeartbeatTimeout time.Duration
	GracePeriod      time.Duration // Grace period for disconnect events before marking room offline
}

type presenceService struct {
	hub        *hub.Hub
	store      store.PresenceStore
	authClient *client.AuthClient
	config     Config

	// Grace period timers for disconnect events
	gracePeriodTimers map[string]*time.Timer // roomID -> timer
	timersMu          sync.Mutex

	cancel context.CancelFunc
}

// NewPresenceService creates a new PresenceService instance.
func NewPresenceService(
	h *hub.Hub,
	s store.PresenceStore,
	authClient *client.AuthClient,
	cfg Config,
) PresenceService {
	return &presenceService{
		hub:               h,
		store:             s,
		authClient:        authClient,
		config:            cfg,
		gracePeriodTimers: make(map[string]*time.Timer),
	}
}

func (s *presenceService) HandleJoin(ctx context.Context, c *hub.Client, roomID, token, deviceHash string) error {
	l := pkglog.L()

	// Determine user identity
	var identity *domain.UserIdentity

	if token != "" {
		// Validate token for authenticated user
		result, err := s.authClient.ValidateToken(ctx, token)
		if err != nil {
			l.Warn().Err(err).Msg("token validation failed")
			return c.SendMessage(domain.NewErrorMessage("authentication service unavailable"))
		}

		if !result.Valid {
			return c.SendMessage(domain.NewErrorMessage("invalid token"))
		}

		identity = &domain.UserIdentity{
			UserID: result.UserID,
			IsAuth: true,
		}
	} else if deviceHash != "" {
		// Anonymous user with device hash
		identity = &domain.UserIdentity{
			DeviceHash: deviceHash,
			IsAuth:     false,
		}
	} else {
		return c.SendMessage(domain.NewErrorMessage("token or device_hash required"))
	}

	// Leave current room if any
	if c.RoomID != "" && c.RoomID != roomID {
		if err := s.HandleLeave(ctx, c, c.RoomID); err != nil {
			l.Error().Err(err).Str("room_id", c.RoomID).Msg("error leaving previous room")
		}
	}

	// Add to Redis store
	ttl := s.config.HeartbeatTimeout
	var err error
	if identity.IsAuth {
		err = s.store.AddUser(ctx, roomID, identity.UserID, ttl)
	} else {
		err = s.store.AddDevice(ctx, roomID, identity.DeviceHash, ttl)
	}
	if err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to add presence to store")
		return c.SendMessage(domain.NewErrorMessage("failed to join room"))
	}

	// Update client state
	c.Identity = identity
	s.hub.JoinRoom(c, roomID)

	// Get current count
	count, err := s.store.GetCount(ctx, roomID)
	if err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to get room count")
		count = domain.PresenceCount{}
	}

	// Send joined confirmation
	if err := c.SendMessage(&domain.JoinedMessage{
		Type:   domain.MsgTypeJoined,
		RoomID: roomID,
		Count:  count,
	}); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to send joined message")
	}

	// Publish to Redis Pub/Sub so all instances (including self) broadcast count
	if err := s.store.PublishRoomUpdate(ctx, roomID, count); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to publish room update")
	}

	return nil
}

func (s *presenceService) HandleLeave(ctx context.Context, c *hub.Client, roomID string) error {
	l := pkglog.L()

	if c.Identity == nil || c.RoomID != roomID {
		return nil
	}

	// Remove from Redis store
	var err error
	if c.Identity.IsAuth {
		err = s.store.RemoveUser(ctx, roomID, c.Identity.UserID)
	} else {
		err = s.store.RemoveDevice(ctx, roomID, c.Identity.DeviceHash)
	}
	if err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to remove presence from store")
	}

	// Update client state
	s.hub.LeaveRoom(c, roomID)

	// Publish count update to Redis Pub/Sub
	count, err := s.store.GetCount(ctx, roomID)
	if err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to get room count after leave")
		count = domain.PresenceCount{}
	}
	if err := s.store.PublishRoomUpdate(ctx, roomID, count); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to publish room update")
	}

	return nil
}

func (s *presenceService) HandleHeartbeat(ctx context.Context, c *hub.Client) error {
	if c.Identity == nil || c.RoomID == "" {
		// Send pong even if not in a room
		return c.SendMessage(&domain.BaseMessage{Type: domain.MsgTypePong})
	}

	// Refresh TTL in Redis
	if err := s.store.RefreshTTL(ctx, c.Identity, c.RoomID, s.config.HeartbeatTimeout); err != nil {
		l := pkglog.L()
		l.Error().Err(err).Str("room_id", c.RoomID).Msg("failed to refresh TTL")
	}

	c.LastPing = time.Now()

	return c.SendMessage(&domain.BaseMessage{Type: domain.MsgTypePong})
}

func (s *presenceService) HandleDisconnect(ctx context.Context, c *hub.Client) error {
	if c.RoomID != "" {
		return s.HandleLeave(ctx, c, c.RoomID)
	}
	return nil
}

func (s *presenceService) GetRoomCount(ctx context.Context, roomID string) (domain.PresenceCount, error) {
	return s.store.GetCount(ctx, roomID)
}

func (s *presenceService) GetRoomInfo(ctx context.Context, roomID string) (*domain.RoomInfo, error) {
	count, err := s.store.GetCount(ctx, roomID)
	if err != nil {
		return nil, err
	}

	liveStatus, err := s.store.GetRoomLiveStatus(ctx, roomID)
	if err != nil {
		return nil, err
	}

	return &domain.RoomInfo{
		RoomID:     roomID,
		Count:      count,
		LiveStatus: *liveStatus,
	}, nil
}

func (s *presenceService) GetAllLiveRooms(ctx context.Context) ([]string, error) {
	return s.store.GetAllLiveRooms(ctx)
}

func (s *presenceService) HandleBroadcastEvent(ctx context.Context, event *kafka.BroadcastEvent) error {
	l := pkglog.L()

	switch event.Type {
	case kafka.EventBroadcastStarted:
		return s.handleBroadcastStarted(ctx, event)
	case kafka.EventBroadcastStopped:
		return s.handleBroadcastStopped(ctx, event)
	default:
		l.Warn().Str("event_type", string(event.Type)).Msg("unknown broadcast event type")
		return nil
	}
}

func (s *presenceService) handleBroadcastStarted(ctx context.Context, event *kafka.BroadcastEvent) error {
	l := pkglog.L()

	// Cancel any pending grace period timer for this room
	s.cancelGracePeriod(event.RoomID)

	// Mark room as live
	if err := s.store.SetRoomLive(ctx, event.RoomID, event.BroadcasterID); err != nil {
		l.Error().Err(err).Str("room_id", event.RoomID).Msg("failed to set room as live")
		return err
	}

	// Publish live status update for multi-instance sync
	if err := s.store.PublishLiveStatusUpdate(ctx, event.RoomID, true); err != nil {
		l.Error().Err(err).Str("room_id", event.RoomID).Msg("failed to publish live status update")
	}

	l.Info().Str("room_id", event.RoomID).Str("broadcaster_id", event.BroadcasterID).Msg("room is now live")
	return nil
}

func (s *presenceService) handleBroadcastStopped(ctx context.Context, event *kafka.BroadcastEvent) error {
	l := pkglog.L()

	if event.Reason == kafka.ReasonDisconnect {
		// Start grace period for disconnect events
		s.startGracePeriod(event.RoomID)
		l.Info().Str("room_id", event.RoomID).Dur("grace_period", s.config.GracePeriod).Msg("started grace period for disconnect")
		return nil
	}

	// For explicit stops, mark offline immediately
	return s.setRoomOffline(ctx, event.RoomID)
}

func (s *presenceService) startGracePeriod(roomID string) {
	s.timersMu.Lock()
	defer s.timersMu.Unlock()

	// Cancel existing timer if any
	if timer, exists := s.gracePeriodTimers[roomID]; exists {
		timer.Stop()
	}

	// Start new grace period timer
	s.gracePeriodTimers[roomID] = time.AfterFunc(s.config.GracePeriod, func() {
		ctx := context.Background()
		s.timersMu.Lock()
		delete(s.gracePeriodTimers, roomID)
		s.timersMu.Unlock()

		l := pkglog.L()
		l.Info().Str("room_id", roomID).Msg("grace period expired, marking offline")
		if err := s.setRoomOffline(ctx, roomID); err != nil {
			l.Error().Err(err).Str("room_id", roomID).Msg("failed to set room offline after grace period")
		}
	})
}

func (s *presenceService) cancelGracePeriod(roomID string) {
	s.timersMu.Lock()
	defer s.timersMu.Unlock()

	if timer, exists := s.gracePeriodTimers[roomID]; exists {
		timer.Stop()
		delete(s.gracePeriodTimers, roomID)
		l := pkglog.L()
		l.Info().Str("room_id", roomID).Msg("cancelled grace period")
	}
}

func (s *presenceService) setRoomOffline(ctx context.Context, roomID string) error {
	l := pkglog.L()

	if err := s.store.SetRoomOffline(ctx, roomID); err != nil {
		return err
	}

	// Publish live status update for multi-instance sync
	if err := s.store.PublishLiveStatusUpdate(ctx, roomID, false); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to publish live status update")
	}

	l.Info().Str("room_id", roomID).Msg("room is now offline")
	return nil
}

func (s *presenceService) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	l := pkglog.L()
	l.Info().Msg("presence service started")
	return nil
}

func (s *presenceService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// Stop all grace period timers
	s.timersMu.Lock()
	for roomID, timer := range s.gracePeriodTimers {
		timer.Stop()
		delete(s.gracePeriodTimers, roomID)
	}
	s.timersMu.Unlock()

	return nil
}
