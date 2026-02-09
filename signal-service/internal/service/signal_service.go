package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/weiawesome/wes-io-live/pkg/pubsub"
	"github.com/weiawesome/wes-io-live/signal-service/internal/client"
	"github.com/weiawesome/wes-io-live/signal-service/internal/domain"
	"github.com/weiawesome/wes-io-live/signal-service/internal/hub"
	"github.com/weiawesome/wes-io-live/signal-service/internal/kafka"
)

type signalService struct {
	hub           *hub.Hub
	authClient    *client.AuthClient
	roomClient    *client.RoomClient
	pubsub        pubsub.PubSub
	kafkaProducer kafka.BroadcastEventProducer

	// Track active broadcasts per room
	activeBroadcasts   map[string]string // roomID -> broadcasterClientID
	broadcasterUserIDs map[string]string // roomID -> broadcasterUserID (for Kafka events)
	roomStates         map[string]*domain.RoomState
	mu                 sync.RWMutex

	cancel context.CancelFunc
}

// NewSignalService creates a new SignalService instance.
func NewSignalService(
	h *hub.Hub,
	authClient *client.AuthClient,
	roomClient *client.RoomClient,
	ps pubsub.PubSub,
	kafkaProducer kafka.BroadcastEventProducer,
) SignalService {
	return &signalService{
		hub:                h,
		authClient:         authClient,
		roomClient:         roomClient,
		pubsub:             ps,
		kafkaProducer:      kafkaProducer,
		activeBroadcasts:   make(map[string]string),
		broadcasterUserIDs: make(map[string]string),
		roomStates:         make(map[string]*domain.RoomState),
	}
}

func (s *signalService) HandleAuth(ctx context.Context, c *hub.Client, token string) error {
	result, err := s.authClient.ValidateToken(ctx, token)
	if err != nil {
		c.SendMessage(&domain.AuthResultMessage{
			Type:    domain.MsgTypeAuthResult,
			Success: false,
			Message: "Authentication service unavailable",
		})
		return err
	}

	if !result.Valid {
		c.SendMessage(&domain.AuthResultMessage{
			Type:    domain.MsgTypeAuthResult,
			Success: false,
			Message: result.Error,
		})
		return fmt.Errorf("invalid token: %s", result.Error)
	}

	c.Session.Authenticate(result.UserID, result.Username, result.Email, result.Roles)

	return c.SendMessage(&domain.AuthResultMessage{
		Type:     domain.MsgTypeAuthResult,
		Success:  true,
		UserID:   result.UserID,
		Username: result.Username,
	})
}

func (s *signalService) HandleJoinRoom(ctx context.Context, c *hub.Client, roomID string) error {
	if !c.Session.IsAuthenticated() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeUnauthorized, "Not authenticated"))
	}

	// Verify room exists and is active
	room, err := s.roomClient.GetRoom(ctx, roomID)
	if err != nil {
		if err == client.ErrRoomNotFound {
			return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeNotFound, "Room not found"))
		}
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to get room"))
	}

	if room.Status != "active" {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeNotFound, "Room is not active"))
	}

	// Leave current room if any
	if currentRoom := c.Session.GetCurrentRoom(); currentRoom != "" {
		s.hub.LeaveRoom(c, currentRoom)
		c.Session.LeaveRoom()
	}

	// Join the room
	isOwner := room.OwnerID == c.Session.GetUserID()
	s.hub.JoinRoom(c, roomID)
	c.Session.JoinRoom(roomID, false)

	// Get room state
	s.mu.RLock()
	state := s.roomStates[roomID]
	s.mu.RUnlock()

	isLive := false
	hlsUrl := ""

	if state != nil {
		isLive = state.IsLive
		hlsUrl = state.HLSUrl
	}

	// Note: viewer count is now handled by presence-service
	return c.SendMessage(&domain.RoomJoinedMessage{
		Type:        domain.MsgTypeRoomJoined,
		RoomID:      roomID,
		IsOwner:     isOwner,
		ViewerCount: 0, // Viewer count is now from presence-service
		IsLive:      isLive,
		HLSUrl:      hlsUrl,
	})
}

func (s *signalService) HandleStartBroadcast(ctx context.Context, c *hub.Client, roomID string, offer json.RawMessage) error {
	if !c.Session.IsAuthenticated() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeUnauthorized, "Not authenticated"))
	}

	// Verify room ownership
	room, err := s.roomClient.GetRoom(ctx, roomID)
	if err != nil {
		if err == client.ErrRoomNotFound {
			return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeNotFound, "Room not found"))
		}
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to get room"))
	}

	if room.OwnerID != c.Session.GetUserID() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeForbidden, "Only room owner can broadcast"))
	}

	if room.Status != "active" {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeNotFound, "Room is not active"))
	}

	// Check if already broadcasting
	s.mu.Lock()
	if _, exists := s.activeBroadcasts[roomID]; exists {
		s.mu.Unlock()
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeAlreadyStreaming, "Room already has an active broadcast"))
	}
	s.activeBroadcasts[roomID] = c.ID
	s.broadcasterUserIDs[roomID] = c.Session.GetUserID()
	s.mu.Unlock()

	// Update session
	c.Session.JoinRoom(roomID, true)
	s.hub.JoinRoom(c, roomID)

	// Publish start broadcast event to Media Service
	event, err := pubsub.NewEvent(pubsub.EventStartBroadcast, roomID, &pubsub.StartBroadcastPayload{
		RoomID: roomID,
		UserID: c.Session.GetUserID(),
		Offer:  string(offer),
	})
	if err != nil {
		s.mu.Lock()
		delete(s.activeBroadcasts, roomID)
		s.mu.Unlock()
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to start broadcast"))
	}

	channel := pubsub.SignalToMediaChannel(roomID)
	if err := s.pubsub.Publish(ctx, channel, event); err != nil {
		log.Printf("Failed to publish start_broadcast event: %v", err)
		s.mu.Lock()
		delete(s.activeBroadcasts, roomID)
		delete(s.broadcasterUserIDs, roomID)
		s.mu.Unlock()
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeInternalError, "Failed to start broadcast"))
	}

	// Send broadcast_started event to Kafka for presence-service
	if s.kafkaProducer != nil {
		if err := s.kafkaProducer.ProduceBroadcastStarted(ctx, roomID, c.Session.GetUserID()); err != nil {
			log.Printf("Failed to produce broadcast_started event to Kafka: %v", err)
			// Don't fail the broadcast, Kafka is non-critical
		}
	}

	log.Printf("Start broadcast event published for room %s", roomID)
	return nil
}

func (s *signalService) HandleICECandidate(ctx context.Context, c *hub.Client, roomID string, candidate json.RawMessage) error {
	if !c.Session.IsAuthenticated() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeUnauthorized, "Not authenticated"))
	}

	if !c.Session.IsBroadcasting() || c.Session.GetCurrentRoom() != roomID {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeForbidden, "Not broadcasting in this room"))
	}

	// Forward ICE candidate to Media Service
	event, err := pubsub.NewEvent(pubsub.EventICECandidate, roomID, &pubsub.ICECandidatePayload{
		RoomID:    roomID,
		Candidate: string(candidate),
	})
	if err != nil {
		return err
	}

	channel := pubsub.SignalToMediaChannel(roomID)
	return s.pubsub.Publish(ctx, channel, event)
}

func (s *signalService) HandleStopBroadcast(ctx context.Context, c *hub.Client, roomID string) error {
	if !c.Session.IsAuthenticated() {
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeUnauthorized, "Not authenticated"))
	}

	s.mu.Lock()
	broadcasterID, exists := s.activeBroadcasts[roomID]
	if !exists || broadcasterID != c.ID {
		s.mu.Unlock()
		return c.SendMessage(domain.NewErrorMessage(domain.ErrCodeForbidden, "Not broadcasting in this room"))
	}
	broadcasterUserID := s.broadcasterUserIDs[roomID]
	delete(s.activeBroadcasts, roomID)
	delete(s.broadcasterUserIDs, roomID)
	delete(s.roomStates, roomID)
	s.mu.Unlock()

	c.Session.JoinRoom(roomID, false)

	// Publish stop broadcast event
	event, _ := pubsub.NewEvent(pubsub.EventStopBroadcast, roomID, &pubsub.StopBroadcastPayload{
		RoomID: roomID,
		Reason: "manual",
	})

	channel := pubsub.SignalToMediaChannel(roomID)
	s.pubsub.Publish(ctx, channel, event)

	// Send broadcast_stopped event to Kafka for presence-service
	if s.kafkaProducer != nil {
		if err := s.kafkaProducer.ProduceBroadcastStopped(ctx, roomID, broadcasterUserID, kafka.ReasonExplicit); err != nil {
			log.Printf("Failed to produce broadcast_stopped event to Kafka: %v", err)
		}
	}

	// Notify viewers
	s.hub.BroadcastToRoom(roomID, &domain.StreamAvailableMessage{
		Type:   domain.MsgTypeStreamAvailable,
		RoomID: roomID,
		HLSUrl: "", // Empty means stream ended
	}, "")

	return nil
}

func (s *signalService) HandleLeaveRoom(ctx context.Context, c *hub.Client, roomID string) error {
	currentRoom := c.Session.GetCurrentRoom()
	if currentRoom != roomID {
		return nil
	}

	// If broadcasting, stop the broadcast
	if c.Session.IsBroadcasting() {
		s.HandleStopBroadcast(ctx, c, roomID)
	}

	s.hub.LeaveRoom(c, roomID)
	c.Session.LeaveRoom()

	// Note: viewer count is now handled by presence-service
	return nil
}

func (s *signalService) HandleDisconnect(ctx context.Context, c *hub.Client) error {
	roomID := c.Session.GetCurrentRoom()
	if roomID == "" {
		return nil
	}

	// If broadcasting, stop the broadcast
	if c.Session.IsBroadcasting() {
		s.mu.Lock()
		broadcasterUserID := s.broadcasterUserIDs[roomID]
		delete(s.activeBroadcasts, roomID)
		delete(s.broadcasterUserIDs, roomID)
		delete(s.roomStates, roomID)
		s.mu.Unlock()

		// Publish stop broadcast event
		event, _ := pubsub.NewEvent(pubsub.EventStopBroadcast, roomID, &pubsub.StopBroadcastPayload{
			RoomID: roomID,
			Reason: "disconnect",
		})

		channel := pubsub.SignalToMediaChannel(roomID)
		s.pubsub.Publish(ctx, channel, event)

		// Send broadcast_stopped event to Kafka for presence-service with reason "disconnect"
		// This will trigger grace period handling in presence-service
		if s.kafkaProducer != nil {
			if err := s.kafkaProducer.ProduceBroadcastStopped(ctx, roomID, broadcasterUserID, kafka.ReasonDisconnect); err != nil {
				log.Printf("Failed to produce broadcast_stopped (disconnect) event to Kafka: %v", err)
			}
		}
	}

	// Note: viewer count is now handled by presence-service
	return nil
}

func (s *signalService) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Subscribe to events from Media Service using pattern
	pattern := "media:room:*:to_signal"
	eventCh, err := s.pubsub.SubscribePattern(ctx, pattern)
	if err != nil {
		return fmt.Errorf("failed to subscribe to media events: %w", err)
	}

	go s.handleMediaEvents(ctx, eventCh)

	log.Println("Signal service started, subscribed to media events")
	return nil
}

func (s *signalService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *signalService) handleMediaEvents(ctx context.Context, eventCh <-chan *pubsub.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			s.processMediaEvent(event)
		}
	}
}

func (s *signalService) processMediaEvent(event *pubsub.Event) {
	switch event.Type {
	case pubsub.EventBroadcastAnswer:
		var payload pubsub.BroadcastAnswerPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			log.Printf("Failed to unmarshal broadcast answer: %v", err)
			return
		}
		s.handleBroadcastAnswer(payload)

	case pubsub.EventServerICECandidate:
		var payload pubsub.ServerICECandidatePayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			log.Printf("Failed to unmarshal server ICE candidate: %v", err)
			return
		}
		s.handleServerICECandidate(payload)

	case pubsub.EventStreamReady:
		var payload pubsub.StreamReadyPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			log.Printf("Failed to unmarshal stream ready: %v", err)
			return
		}
		s.handleStreamReady(payload)

	case pubsub.EventStreamEnded:
		var payload pubsub.StreamEndedPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			log.Printf("Failed to unmarshal stream ended: %v", err)
			return
		}
		s.handleStreamEnded(payload)
	}
}

func (s *signalService) handleBroadcastAnswer(payload pubsub.BroadcastAnswerPayload) {
	s.mu.RLock()
	broadcasterID, exists := s.activeBroadcasts[payload.RoomID]
	s.mu.RUnlock()

	if !exists {
		log.Printf("No active broadcast for room %s", payload.RoomID)
		return
	}

	s.hub.SendToClient(broadcasterID, &domain.BroadcastStartedMessage{
		Type:   domain.MsgTypeBroadcastStarted,
		RoomID: payload.RoomID,
		Answer: json.RawMessage(payload.Answer),
	})

	log.Printf("Sent broadcast answer to client %s for room %s", broadcasterID, payload.RoomID)
}

func (s *signalService) handleServerICECandidate(payload pubsub.ServerICECandidatePayload) {
	s.mu.RLock()
	broadcasterID, exists := s.activeBroadcasts[payload.RoomID]
	s.mu.RUnlock()

	if !exists {
		return
	}

	s.hub.SendToClient(broadcasterID, &domain.ICECandidateMessage{
		Type:      domain.MsgTypeICECandidate,
		RoomID:    payload.RoomID,
		Candidate: json.RawMessage(payload.Candidate),
	})
}

func (s *signalService) handleStreamReady(payload pubsub.StreamReadyPayload) {
	s.mu.Lock()
	s.roomStates[payload.RoomID] = &domain.RoomState{
		RoomID: payload.RoomID,
		IsLive: true,
		HLSUrl: payload.HLSUrl,
	}
	s.mu.Unlock()

	// Broadcast to all viewers in the room
	s.hub.BroadcastToRoom(payload.RoomID, &domain.StreamAvailableMessage{
		Type:   domain.MsgTypeStreamAvailable,
		RoomID: payload.RoomID,
		HLSUrl: payload.HLSUrl,
	}, "")

	log.Printf("Stream ready for room %s: %s", payload.RoomID, payload.HLSUrl)
}

func (s *signalService) handleStreamEnded(payload pubsub.StreamEndedPayload) {
	s.mu.Lock()
	delete(s.roomStates, payload.RoomID)
	delete(s.activeBroadcasts, payload.RoomID)
	delete(s.broadcasterUserIDs, payload.RoomID)
	s.mu.Unlock()

	// Notify viewers
	s.hub.BroadcastToRoom(payload.RoomID, &domain.StreamAvailableMessage{
		Type:   domain.MsgTypeStreamAvailable,
		RoomID: payload.RoomID,
		HLSUrl: "",
	}, "")

	log.Printf("Stream ended for room %s", payload.RoomID)
}
