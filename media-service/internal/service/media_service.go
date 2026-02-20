package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/weiawesome/wes-io-live/media-service/internal/domain"
	peerManager "github.com/weiawesome/wes-io-live/media-service/internal/webrtc"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/pubsub"
)

type mediaService struct {
	peerManager       *peerManager.PeerManager
	transcoder        *Transcoder
	pubsub            pubsub.PubSub
	vodManager        *VODManager
	thumbnailService  *ThumbnailService

	streams map[string]*domain.Stream
	mu      sync.RWMutex

	cancel context.CancelFunc
}

// NewMediaService creates a new MediaService instance.
func NewMediaService(
	pm *peerManager.PeerManager,
	transcoder *Transcoder,
	ps pubsub.PubSub,
	vodManager *VODManager,
	thumbnailService *ThumbnailService,
) MediaService {
	return &mediaService{
		peerManager:      pm,
		transcoder:       transcoder,
		pubsub:           ps,
		vodManager:       vodManager,
		thumbnailService: thumbnailService,
		streams:          make(map[string]*domain.Stream),
	}
}

func (s *mediaService) HandleStartBroadcast(ctx context.Context, roomID, userID, offerJSON string) error {
	// Parse the offer JSON to extract SDP
	var offer struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}
	if err := json.Unmarshal([]byte(offerJSON), &offer); err != nil {
		return fmt.Errorf("failed to parse offer JSON: %w", err)
	}
	if offer.SDP == "" {
		return fmt.Errorf("offer SDP is empty")
	}

	s.mu.Lock()

	// Check if stream already exists
	if existing, exists := s.streams[roomID]; exists {
		s.mu.Unlock()
		if existing.GetState() == domain.StreamStateLive {
			return fmt.Errorf("stream already active for room %s", roomID)
		}
		// Clean up existing stream
		s.cleanupStream(roomID)
	}

	// Create new stream
	stream := domain.NewStream(roomID, userID)
	stream.SetState(domain.StreamStateConnecting)
	s.streams[roomID] = stream
	s.mu.Unlock()

	// Create peer connection with handlers
	pc, err := s.peerManager.CreatePeerConnection(
		func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			s.handleTrack(roomID, track)
		},
		func(state webrtc.PeerConnectionState) {
			s.handleConnectionState(roomID, state)
		},
		func(candidate *webrtc.ICECandidate) {
			s.handleICECandidate(roomID, candidate)
		},
	)
	if err != nil {
		s.cleanupStream(roomID)
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	stream.PeerConnection = pc

	// Handle the offer and create answer
	answerSDP, err := s.peerManager.HandleOffer(pc, offer.SDP)
	if err != nil {
		s.cleanupStream(roomID)
		return fmt.Errorf("failed to handle offer: %w", err)
	}

	// Send answer back to Signal Service (as JSON object)
	answerJSON, err := json.Marshal(map[string]string{
		"type": "answer",
		"sdp":  answerSDP,
	})
	if err != nil {
		s.cleanupStream(roomID)
		return fmt.Errorf("failed to marshal answer: %w", err)
	}

	event, err := pubsub.NewEvent(pubsub.EventBroadcastAnswer, roomID, &pubsub.BroadcastAnswerPayload{
		RoomID: roomID,
		Answer: string(answerJSON),
	})
	if err != nil {
		s.cleanupStream(roomID)
		return err
	}

	l := pkglog.L()
	channel := pubsub.MediaToSignalChannel(roomID)
	if err := s.pubsub.Publish(ctx, channel, event); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to publish broadcast answer")
	}

	l.Info().Str("room_id", roomID).Msg("broadcast started, answer sent")
	return nil
}

func (s *mediaService) HandleICECandidate(ctx context.Context, roomID, candidateJSON string) error {
	s.mu.RLock()
	stream, exists := s.streams[roomID]
	s.mu.RUnlock()

	if !exists || stream.PeerConnection == nil {
		return fmt.Errorf("no active stream for room %s", roomID)
	}

	// Parse candidate JSON
	var candidate struct {
		Candidate        string `json:"candidate"`
		SDPMid           string `json:"sdpMid"`
		SDPMLineIndex    uint16 `json:"sdpMLineIndex"`
		UsernameFragment string `json:"usernameFragment"`
	}
	if err := json.Unmarshal([]byte(candidateJSON), &candidate); err != nil {
		return fmt.Errorf("failed to parse ICE candidate: %w", err)
	}

	init := webrtc.ICECandidateInit{
		Candidate:        candidate.Candidate,
		SDPMid:           &candidate.SDPMid,
		SDPMLineIndex:    &candidate.SDPMLineIndex,
		UsernameFragment: &candidate.UsernameFragment,
	}

	return stream.PeerConnection.AddICECandidate(init)
}

func (s *mediaService) HandleStopBroadcast(ctx context.Context, roomID, reason string) error {
	l := pkglog.L()
	l.Info().Str("room_id", roomID).Str("reason", reason).Msg("stopping broadcast")
	s.cleanupStream(roomID)

	// Notify Signal Service
	event, _ := pubsub.NewEvent(pubsub.EventStreamEnded, roomID, &pubsub.StreamEndedPayload{
		RoomID: roomID,
	})

	channel := pubsub.MediaToSignalChannel(roomID)
	s.pubsub.Publish(ctx, channel, event)

	return nil
}

func (s *mediaService) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Subscribe to events from Signal Service using pattern
	pattern := "signal:room:*:to_media"
	eventCh, err := s.pubsub.SubscribePattern(ctx, pattern)
	if err != nil {
		return fmt.Errorf("failed to subscribe to signal events: %w", err)
	}

	go s.handleSignalEvents(ctx, eventCh)

	l := pkglog.L()
	l.Info().Msg("media service started, subscribed to signal events")
	return nil
}

func (s *mediaService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	// Cleanup all streams
	s.mu.Lock()
	for roomID := range s.streams {
		s.cleanupStreamLocked(roomID)
	}
	s.mu.Unlock()

	return nil
}

func (s *mediaService) handleSignalEvents(ctx context.Context, eventCh <-chan *pubsub.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			s.processSignalEvent(ctx, event)
		}
	}
}

func (s *mediaService) processSignalEvent(ctx context.Context, event *pubsub.Event) {
	l := pkglog.L()
	switch event.Type {
	case pubsub.EventStartBroadcast:
		var payload pubsub.StartBroadcastPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			l.Error().Err(err).Msg("failed to unmarshal start broadcast")
			return
		}
		if err := s.HandleStartBroadcast(ctx, payload.RoomID, payload.UserID, payload.Offer); err != nil {
			l.Error().Err(err).Str("room_id", payload.RoomID).Msg("failed to handle start broadcast")
		}

	case pubsub.EventICECandidate:
		var payload pubsub.ICECandidatePayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			l.Error().Err(err).Msg("failed to unmarshal ice candidate")
			return
		}
		if err := s.HandleICECandidate(ctx, payload.RoomID, payload.Candidate); err != nil {
			l.Error().Err(err).Str("room_id", payload.RoomID).Msg("failed to handle ice candidate")
		}

	case pubsub.EventStopBroadcast:
		var payload pubsub.StopBroadcastPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			l.Error().Err(err).Msg("failed to unmarshal stop broadcast")
			return
		}
		if err := s.HandleStopBroadcast(ctx, payload.RoomID, payload.Reason); err != nil {
			l.Error().Err(err).Str("room_id", payload.RoomID).Msg("failed to handle stop broadcast")
		}
	}
}

func (s *mediaService) handleTrack(roomID string, track *webrtc.TrackRemote) {
	s.mu.Lock()
	stream, exists := s.streams[roomID]
	if !exists {
		s.mu.Unlock()
		return
	}

	if track.Kind() == webrtc.RTPCodecTypeVideo {
		stream.SetVideoTrack(track)
	} else if track.Kind() == webrtc.RTPCodecTypeAudio {
		stream.SetAudioTrack(track)
	}

	// Get both tracks (audioTrack may be nil if not yet received)
	videoTrack := stream.GetVideoTrack()
	audioTrack := stream.GetAudioTrack()
	s.mu.Unlock()

	// Start HLS transcoding when video track is received
	// Pass audioTrack which may be nil (broadcaster might not have mic)
	if track.Kind() == webrtc.RTPCodecTypeVideo {
		go s.startHLSTranscoding(roomID, videoTrack, audioTrack)
	}
}

func (s *mediaService) startHLSTranscoding(roomID string, videoTrack, audioTrack *webrtc.TrackRemote) {
	l := pkglog.L()
	// Start VOD tracking if enabled - this determines the sessionID
	var sessionID string
	if s.vodManager != nil {
		ctx := context.Background()
		session, err := s.vodManager.StartRoom(ctx, roomID)
		if err != nil {
			l.Error().Err(err).Str("room_id", roomID).Msg("failed to start vod tracking")
		} else if session != nil {
			sessionID = session.SessionID
			l.Info().Str("room_id", roomID).Str("session_id", sessionID).Msg("vod session started")
		}
	}

	// Start HLS immediately - don't delay, we don't want to miss keyframes
	hlsUrl, err := s.transcoder.StartHLS(roomID, sessionID, videoTrack, audioTrack)
	if err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to start hls")
		return
	}

	s.mu.Lock()
	if stream, exists := s.streams[roomID]; exists {
		stream.SetState(domain.StreamStateLive)
		stream.SetHLSUrl(hlsUrl)
	}
	s.mu.Unlock()

	// Wait for first HLS segment to be created before notifying stream is ready
	time.Sleep(time.Duration(3) * time.Second)

	// Start thumbnail capture if enabled (captures from HLS stream)
	if s.thumbnailService != nil && sessionID != "" {
		s.thumbnailService.StartCapture(roomID, sessionID)
	}

	// Notify Signal Service that stream is ready
	ctx := context.Background()
	event, _ := pubsub.NewEvent(pubsub.EventStreamReady, roomID, &pubsub.StreamReadyPayload{
		RoomID: roomID,
		HLSUrl: hlsUrl,
	})

	channel := pubsub.MediaToSignalChannel(roomID)
	if err := s.pubsub.Publish(ctx, channel, event); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to publish stream ready")
	}

	l.Info().Str("room_id", roomID).Str("hls_url", hlsUrl).Msg("hls stream ready")
}

func (s *mediaService) handleConnectionState(roomID string, state webrtc.PeerConnectionState) {
	if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
		l := pkglog.L()
		l.Info().Str("room_id", roomID).Str("state", state.String()).Msg("connection state changed")
		ctx := context.Background()
		s.HandleStopBroadcast(ctx, roomID, "connection_"+state.String())
	}
}

func (s *mediaService) handleICECandidate(roomID string, candidate *webrtc.ICECandidate) {
	candidateJSON := candidate.ToJSON()
	data, err := json.Marshal(candidateJSON)
	if err != nil {
		return
	}

	ctx := context.Background()
	event, _ := pubsub.NewEvent(pubsub.EventServerICECandidate, roomID, &pubsub.ServerICECandidatePayload{
		RoomID:    roomID,
		Candidate: string(data),
	})

	channel := pubsub.MediaToSignalChannel(roomID)
	s.pubsub.Publish(ctx, channel, event)
}

func (s *mediaService) cleanupStream(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupStreamLocked(roomID)
}

func (s *mediaService) cleanupStreamLocked(roomID string) {
	stream, exists := s.streams[roomID]
	if !exists {
		return
	}

	// Get active session for sessionID
	ctx := context.Background()
	var sessionID string
	if s.vodManager != nil {
		session, _ := s.vodManager.GetActiveSession(ctx, roomID)
		if session != nil {
			sessionID = session.SessionID
		}
	}

	// Stop thumbnail capture
	if s.thumbnailService != nil && sessionID != "" {
		s.thumbnailService.StopCapture(roomID, sessionID)
	}

	// Stop transcoder
	s.transcoder.StopHLS(roomID, sessionID)

	// Finalize VOD if enabled
	if s.vodManager != nil && s.vodManager.IsRoomTracking(roomID) {
		go func() {
			l := pkglog.L()
			vodURL, err := s.vodManager.FinalizeRoom(ctx, roomID)
			if err != nil {
				l.Error().Err(err).Str("room_id", roomID).Msg("failed to finalize vod")
			} else if vodURL != "" {
				l.Info().Str("room_id", roomID).Str("vod_url", vodURL).Msg("vod available")
			}
		}()
	}

	// Close peer connection
	if stream.PeerConnection != nil {
		stream.PeerConnection.Close()
	}

	delete(s.streams, roomID)
	l := pkglog.L()
	l.Info().Str("room_id", roomID).Msg("stream cleaned up")
}

// GetVODURL returns the VOD URL for the latest session of a room.
func (s *mediaService) GetVODURL(ctx context.Context, roomID string) (string, error) {
	if s.vodManager == nil {
		return "", fmt.Errorf("VOD not configured")
	}
	return s.vodManager.GetVODURL(ctx, roomID)
}

// GetSessionVODURL returns the VOD URL for a specific session.
func (s *mediaService) GetSessionVODURL(ctx context.Context, roomID, sessionID string) (string, error) {
	if s.vodManager == nil {
		return "", fmt.Errorf("VOD not configured")
	}
	return s.vodManager.GetSessionVODURL(ctx, roomID, sessionID)
}

// ListRoomVODs returns a list of VOD recordings for a specific room.
func (s *mediaService) ListRoomVODs(ctx context.Context, roomID string) ([]VODInfo, error) {
	if s.vodManager == nil {
		return nil, fmt.Errorf("VOD not configured")
	}
	return s.vodManager.ListRoomVODs(ctx, roomID)
}

// IsRoomLive returns whether a room has an active live stream.
func (s *mediaService) IsRoomLive(ctx context.Context, roomID string) bool {
	if s.vodManager == nil {
		return false
	}
	return s.vodManager.IsRoomLive(ctx, roomID)
}

// GetActiveSession returns the active session for a room.
func (s *mediaService) GetActiveSession(ctx context.Context, roomID string) (*VODSession, error) {
	if s.vodManager == nil {
		return nil, fmt.Errorf("VOD not configured")
	}
	return s.vodManager.GetActiveSession(ctx, roomID)
}

// IsVODEnabled returns whether VOD recording is enabled.
func (s *mediaService) IsVODEnabled() bool {
	return s.vodManager != nil && s.vodManager.IsEnabled()
}
