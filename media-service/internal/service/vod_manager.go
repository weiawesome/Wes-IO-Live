package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/weiawesome/wes-io-live/media-service/internal/config"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/storage"
)

// VODManager manages VOD recording and S3 upload for live streams.
type VODManager struct {
	s3Storage        storage.Storage
	uploader         *S3Uploader
	segmentWatcher   *SegmentWatcher
	sessionStore     SessionStore
	playlistBuilders map[string]*VODPlaylistBuilder // sessionKey -> builder (roomID:sessionID)
	hlsOutputDir     string
	vodConfig        config.VODConfig
	targetDuration   int
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	stopOnce         sync.Once
}

// VODManagerConfig holds configuration for VOD manager.
type VODManagerConfig struct {
	S3Storage      storage.Storage
	HLSOutputDir   string
	VODConfig      config.VODConfig
	TargetDuration int
	SessionStore   SessionStore
}

// NewVODManager creates a new VOD manager.
func NewVODManager(cfg VODManagerConfig) *VODManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Use provided session store or create default in-memory store
	sessionStore := cfg.SessionStore
	if sessionStore == nil {
		sessionStore = NewMemorySessionStore()
	}

	m := &VODManager{
		s3Storage:        cfg.S3Storage,
		hlsOutputDir:     cfg.HLSOutputDir,
		vodConfig:        cfg.VODConfig,
		targetDuration:   cfg.TargetDuration,
		sessionStore:     sessionStore,
		playlistBuilders: make(map[string]*VODPlaylistBuilder),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Create S3 uploader if S3 storage is configured
	if cfg.S3Storage != nil {
		m.uploader = NewS3Uploader(cfg.S3Storage, S3UploaderConfig{
			Workers:    cfg.VODConfig.UploadWorkers,
			QueueSize:  200,
			MaxRetries: 3,
			RetryDelay: time.Second,
		})
	}

	// Create segment watcher with session-aware callback
	m.segmentWatcher = NewSegmentWatcher(cfg.HLSOutputDir, m.onSegmentReady)

	return m
}

// Start starts the VOD manager.
func (m *VODManager) Start() {
	if m.uploader != nil {
		m.uploader.Start()
	}
	l := pkglog.L()
	l.Info().Msg("vod manager started")
}

// Stop stops the VOD manager.
func (m *VODManager) Stop() {
	m.stopOnce.Do(func() {
		m.cancel()

		// Stop all watchers
		m.segmentWatcher.StopAll()

		// Stop uploader
		if m.uploader != nil {
			m.uploader.Stop()
		}

		l := pkglog.L()
		l.Info().Msg("vod manager stopped")
	})
}

// sessionKey returns the key for playlistBuilders map
func sessionKey(roomID, sessionID string) string {
	return roomID + ":" + sessionID
}

// StartRoom begins VOD tracking for a room.
// Returns the created session.
func (m *VODManager) StartRoom(ctx context.Context, roomID string) (*VODSession, error) {
	if !m.vodConfig.Enabled {
		return nil, nil
	}

	// Check if already has an active session
	existing, err := m.sessionStore.Get(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check session: %w", err)
	}
	if existing != nil && existing.IsActive() {
		return nil, fmt.Errorf("room %s already has active stream (session: %s, state: %s)",
			roomID, existing.SessionID, existing.State.String())
	}

	// Generate session ID from current timestamp
	sessionID := time.Now().UTC().Format("2006-01-02T15-04-05Z")

	// Create session
	session := &VODSession{
		RoomID:    roomID,
		SessionID: sessionID,
		StartTime: time.Now().UTC(),
		State:     SessionStarting,
		LocalDir:  filepath.Join(m.hlsOutputDir, "room_"+roomID, sessionID),
		S3Prefix:  fmt.Sprintf("vod/room_%s/%s", roomID, sessionID),
	}

	// Save session
	if err := m.sessionStore.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	m.mu.Lock()
	// Create playlist builder for this session
	key := sessionKey(roomID, sessionID)
	builder := NewVODPlaylistBuilder(roomID, m.targetDuration)
	m.playlistBuilders[key] = builder
	m.mu.Unlock()

	// Start watching for segments
	if err := m.segmentWatcher.StartWatchingSession(roomID, sessionID); err != nil {
		m.mu.Lock()
		delete(m.playlistBuilders, key)
		m.mu.Unlock()
		m.sessionStore.Delete(ctx, roomID)
		return nil, fmt.Errorf("failed to start segment watcher: %w", err)
	}

	// Update state to live
	session.State = SessionLive
	l := pkglog.L()
	if err := m.sessionStore.Save(ctx, session); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to update session state to live")
	}

	l.Info().Str("room_id", roomID).Str("session_id", sessionID).Msg("vod tracking started")
	return session, nil
}

// onSegmentReady handles new segment detection.
func (m *VODManager) onSegmentReady(roomID string, sessionID string, seg SegmentInfo) {
	key := sessionKey(roomID, sessionID)

	m.mu.RLock()
	builder, exists := m.playlistBuilders[key]
	m.mu.RUnlock()

	if !exists {
		return
	}

	// Add segment to playlist builder
	builder.AddSegment(seg)

	// Upload segment to S3 asynchronously
	if m.uploader != nil {
		localPath := filepath.Join(m.hlsOutputDir, "room_"+roomID, sessionID, seg.Filename)
		s3Key := fmt.Sprintf("vod/room_%s/%s/%s", roomID, sessionID, seg.Filename)

		task := &UploadTask{
			RoomID:      roomID,
			LocalPath:   localPath,
			S3Key:       s3Key,
			ContentType: "video/mp2t",
			OnComplete: func(err error) {
				if err != nil {
					l := pkglog.L()
					l.Error().Err(err).Str("segment", seg.Filename).Msg("failed to upload segment")
					return
				}

				// Mark segment as uploaded
				builder.MarkSegmentUploaded(seg.Index, s3Key)

				// Update VOD playlist on S3
				m.uploadVODPlaylist(roomID, sessionID, false)
			},
		}

		if err := m.uploader.Upload(task); err != nil {
			l := pkglog.L()
			l.Error().Err(err).Str("room_id", roomID).Msg("failed to queue segment upload")
		}
	}
}

// uploadVODPlaylist generates and uploads the VOD playlist to S3.
func (m *VODManager) uploadVODPlaylist(roomID, sessionID string, finalized bool) {
	key := sessionKey(roomID, sessionID)

	m.mu.RLock()
	builder, exists := m.playlistBuilders[key]
	m.mu.RUnlock()

	if !exists || m.uploader == nil {
		return
	}

	// Generate playlist content
	content := builder.GenerateM3U8(finalized)

	// Upload to S3
	s3Key := fmt.Sprintf("vod/room_%s/%s/stream.m3u8", roomID, sessionID)
	reader := bytes.NewReader(content)

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	l := pkglog.L()
	if err := m.uploader.UploadReader(ctx, reader, int64(len(content)), s3Key, "application/vnd.apple.mpegurl"); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Str("session_id", sessionID).Msg("failed to upload vod playlist")
		return
	}

	if finalized {
		l.Info().Str("room_id", roomID).Str("session_id", sessionID).Msg("final vod playlist uploaded")
	}
}

// FinalizeRoom completes VOD recording for a room.
// Returns the VOD URL for playback.
func (m *VODManager) FinalizeRoom(ctx context.Context, roomID string) (string, error) {
	if !m.vodConfig.Enabled {
		return "", nil
	}

	// Get current session
	session, err := m.sessionStore.Get(ctx, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return "", fmt.Errorf("no VOD tracking for room %s", roomID)
	}
	if !session.CanFinalize() {
		return "", fmt.Errorf("session cannot be finalized (state: %s)", session.State.String())
	}

	sessionID := session.SessionID

	l := pkglog.L()
	// Update state to finalizing
	session.State = SessionFinalizing
	if err := m.sessionStore.Save(ctx, session); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to update session state to finalizing")
	}

	// Stop watching
	m.segmentWatcher.StopWatchingSession(roomID, sessionID)

	key := sessionKey(roomID, sessionID)
	m.mu.Lock()
	builder, exists := m.playlistBuilders[key]
	if !exists {
		m.mu.Unlock()
		return "", fmt.Errorf("no playlist builder for room %s session %s", roomID, sessionID)
	}
	m.mu.Unlock()

	// Wait for pending uploads to complete
	time.Sleep(2 * time.Second)

	// Upload final playlist with ENDLIST
	m.uploadVODPlaylist(roomID, sessionID, true)

	// Clean up playlist builder
	m.mu.Lock()
	delete(m.playlistBuilders, key)
	m.mu.Unlock()

	// Clean up local HLS files
	if err := os.RemoveAll(session.LocalDir); err != nil {
		l.Error().Err(err).Str("dir", session.LocalDir).Msg("failed to cleanup local dir")
	} else {
		l.Info().Str("dir", session.LocalDir).Msg("cleaned up local hls files")
	}

	// Try to remove parent room directory if empty
	roomDir := filepath.Dir(session.LocalDir)
	if entries, err := os.ReadDir(roomDir); err == nil && len(entries) == 0 {
		os.Remove(roomDir)
	}

	// Delete session from store
	if err := m.sessionStore.Delete(ctx, roomID); err != nil {
		l.Error().Err(err).Str("room_id", roomID).Msg("failed to delete session")
	}

	// Generate VOD URL
	if m.s3Storage != nil {
		s3Key := fmt.Sprintf("vod/room_%s/%s/stream.m3u8", roomID, sessionID)
		url, err := m.s3Storage.GetURL(m.ctx, s3Key, 24*time.Hour)
		if err != nil {
			return "", fmt.Errorf("failed to get VOD URL: %w", err)
		}

		l.Info().Str("room_id", roomID).Str("session_id", sessionID).Str("url", url).Int("segments", builder.SegmentCount()).Msg("vod finalized")
		return url, nil
	}

	return "", nil
}

// GetVODURL returns the VOD URL for the latest session of a room.
func (m *VODManager) GetVODURL(ctx context.Context, roomID string) (string, error) {
	if m.s3Storage == nil {
		return "", fmt.Errorf("S3 storage not configured")
	}

	// List sessions for this room and get the latest
	vods, err := m.ListRoomVODs(ctx, roomID)
	if err != nil {
		return "", err
	}
	if len(vods) == 0 {
		return "", fmt.Errorf("VOD not found for room %s", roomID)
	}

	// Return the latest session's URL
	return vods[0].URL, nil
}

// GetSessionVODURL returns the VOD URL for a specific session.
func (m *VODManager) GetSessionVODURL(ctx context.Context, roomID, sessionID string) (string, error) {
	if m.s3Storage == nil {
		return "", fmt.Errorf("S3 storage not configured")
	}

	s3Key := fmt.Sprintf("vod/room_%s/%s/stream.m3u8", roomID, sessionID)

	// Check if VOD exists
	exists, err := m.s3Storage.Exists(ctx, s3Key)
	if err != nil {
		return "", fmt.Errorf("failed to check VOD existence: %w", err)
	}
	if !exists {
		return "", fmt.Errorf("VOD not found for room %s session %s", roomID, sessionID)
	}

	return m.s3Storage.GetURL(ctx, s3Key, 24*time.Hour)
}

// IsRoomLive returns whether a room has an active live stream.
func (m *VODManager) IsRoomLive(ctx context.Context, roomID string) bool {
	session, err := m.sessionStore.Get(ctx, roomID)
	if err != nil || session == nil {
		return false
	}
	return session.State == SessionLive || session.State == SessionStarting
}

// GetActiveSession returns the active session for a room if any.
func (m *VODManager) GetActiveSession(ctx context.Context, roomID string) (*VODSession, error) {
	session, err := m.sessionStore.Get(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if session == nil || !session.IsActive() {
		return nil, nil
	}
	return session, nil
}

// IsRoomTracking returns whether a room is being tracked for VOD.
func (m *VODManager) IsRoomTracking(roomID string) bool {
	session, err := m.sessionStore.Get(m.ctx, roomID)
	if err != nil || session == nil {
		return false
	}
	return session.IsActive()
}

// GetRoomStats returns VOD statistics for a room's active session.
func (m *VODManager) GetRoomStats(roomID string) (totalSegments, uploadedSegments int) {
	session, err := m.sessionStore.Get(m.ctx, roomID)
	if err != nil || session == nil {
		return 0, 0
	}

	key := sessionKey(roomID, session.SessionID)
	m.mu.RLock()
	builder, exists := m.playlistBuilders[key]
	m.mu.RUnlock()

	if !exists {
		return 0, 0
	}

	segments := builder.GetSegments()
	uploaded := builder.GetUploadedSegments()
	return len(segments), len(uploaded)
}

// ListRoomVODs returns a list of VOD recordings for a specific room.
func (m *VODManager) ListRoomVODs(ctx context.Context, roomID string) ([]VODInfo, error) {
	if m.s3Storage == nil {
		return nil, fmt.Errorf("S3 storage not configured")
	}

	prefix := fmt.Sprintf("vod/room_%s/", roomID)
	files, err := m.s3Storage.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list VODs: %w", err)
	}

	// Extract unique session IDs from file keys
	// Key format: vod/room_{roomID}/{sessionID}/stream.m3u8
	sessions := make(map[string]bool)
	for _, f := range files {
		// Remove prefix and extract sessionID
		relPath := strings.TrimPrefix(f.Key, prefix)
		parts := strings.Split(relPath, "/")
		if len(parts) >= 1 && parts[0] != "" {
			sessions[parts[0]] = true
		}
	}

	// Build VODInfo list with presigned URLs
	var result []VODInfo
	for sessionID := range sessions {
		s3Key := fmt.Sprintf("vod/room_%s/%s/stream.m3u8", roomID, sessionID)
		url, err := m.s3Storage.GetURL(ctx, s3Key, 24*time.Hour)
		if err != nil {
			l := pkglog.L()
			l.Error().Err(err).Str("session_id", sessionID).Msg("failed to get url for session")
			continue
		}

		// Parse start time from sessionID
		startTime, _ := time.Parse("2006-01-02T15-04-05Z", sessionID)

		result = append(result, VODInfo{
			SessionID: sessionID,
			StartTime: startTime,
			URL:       url,
		})
	}

	// Sort by start time descending (most recent first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.After(result[j].StartTime)
	})

	return result, nil
}

// ListVODs returns a list of all rooms with VOD recordings.
func (m *VODManager) ListVODs(ctx context.Context) ([]string, error) {
	if m.s3Storage == nil {
		return nil, fmt.Errorf("S3 storage not configured")
	}

	files, err := m.s3Storage.List(ctx, "vod/")
	if err != nil {
		return nil, fmt.Errorf("failed to list VODs: %w", err)
	}

	// Extract unique room IDs from file keys
	rooms := make(map[string]bool)
	for _, f := range files {
		// Key format: vod/room_{roomID}/{sessionID}/...
		parts := strings.Split(f.Key, "/")
		if len(parts) >= 2 {
			roomDir := parts[1]
			if strings.HasPrefix(roomDir, "room_") {
				rooms[strings.TrimPrefix(roomDir, "room_")] = true
			}
		}
	}

	result := make([]string, 0, len(rooms))
	for roomID := range rooms {
		result = append(result, roomID)
	}
	return result, nil
}

// DeleteVOD deletes a VOD recording for a specific session.
func (m *VODManager) DeleteVOD(ctx context.Context, roomID, sessionID string) error {
	if m.s3Storage == nil {
		return fmt.Errorf("S3 storage not configured")
	}

	prefix := fmt.Sprintf("vod/room_%s/%s/", roomID, sessionID)
	return m.s3Storage.DeletePrefix(ctx, prefix)
}

// DeleteAllRoomVODs deletes all VOD recordings for a room.
func (m *VODManager) DeleteAllRoomVODs(ctx context.Context, roomID string) error {
	if m.s3Storage == nil {
		return fmt.Errorf("S3 storage not configured")
	}

	prefix := fmt.Sprintf("vod/room_%s/", roomID)
	return m.s3Storage.DeletePrefix(ctx, prefix)
}

// IsEnabled returns whether VOD is enabled.
func (m *VODManager) IsEnabled() bool {
	return m.vodConfig.Enabled
}

// GetUploader returns the S3 uploader instance.
// This allows sharing the uploader with other services like ThumbnailService.
func (m *VODManager) GetUploader() *S3Uploader {
	return m.uploader
}
