package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/weiawesome/wes-io-live/playback-service/internal/config"
)

// VODInfo represents information about a VOD recording.
type VODInfo struct {
	SessionID string    `json:"session_id"`
	StartTime time.Time `json:"start_time"`
	URL       string    `json:"url,omitempty"`
}

// PlaybackService handles playback operations for live and VOD content.
type PlaybackService struct {
	provider     *ContentProvider
	sessionStore SessionStore
	cfg          config.PlaybackConfig
}

// NewPlaybackService creates a new playback service.
func NewPlaybackService(provider *ContentProvider, sessionStore SessionStore, cfg config.PlaybackConfig) *PlaybackService {
	return &PlaybackService{
		provider:     provider,
		sessionStore: sessionStore,
		cfg:          cfg,
	}
}

// GetActiveSessionID returns the active session ID for a room.
// Returns empty string if no active session exists.
func (s *PlaybackService) GetActiveSessionID(ctx context.Context, roomID string) (string, error) {
	session, err := s.sessionStore.Get(ctx, roomID)
	if err != nil {
		return "", err
	}
	if session == nil || !session.IsActive() {
		return "", nil
	}
	return session.SessionID, nil
}

// ServeLiveContent serves live HLS content for a room/session.
func (s *PlaybackService) ServeLiveContent(ctx context.Context, w http.ResponseWriter, r *http.Request, roomID, sessionID, filename string) error {
	key := buildStorageKey(s.cfg.LivePrefix, roomID, sessionID, filename)
	return s.provider.ServeContent(ctx, w, r, key)
}

// ServeVODContent serves VOD content for a room/session.
func (s *PlaybackService) ServeVODContent(ctx context.Context, w http.ResponseWriter, r *http.Request, roomID, sessionID, filename string) error {
	key := buildStorageKey(s.cfg.VODPrefix, roomID, sessionID, filename)
	return s.provider.ServeContent(ctx, w, r, key)
}

// ListRoomVODs returns a list of VOD recordings for a room.
func (s *PlaybackService) ListRoomVODs(ctx context.Context, roomID string) ([]VODInfo, error) {
	prefix := buildStoragePrefix(s.cfg.VODPrefix, roomID)
	files, err := s.provider.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list VODs: %w", err)
	}

	// Extract unique session IDs from file keys
	sessions := make(map[string]bool)
	for _, f := range files {
		// Remove prefix and extract sessionID
		relPath := strings.TrimPrefix(f.Key, prefix)
		parts := strings.Split(relPath, "/")
		if len(parts) >= 1 && parts[0] != "" {
			sessions[parts[0]] = true
		}
	}

	// Build VODInfo list
	var result []VODInfo
	for sessionID := range sessions {
		// Parse start time from sessionID (format: 2006-01-02T15-04-05Z)
		startTime, _ := time.Parse("2006-01-02T15-04-05Z", sessionID)

		info := VODInfo{
			SessionID: sessionID,
			StartTime: startTime,
		}

		// Get URL for the playlist
		key := buildStorageKey(s.cfg.VODPrefix, roomID, sessionID, "stream.m3u8")
		if url, err := s.provider.GetURL(ctx, key, time.Duration(s.cfg.PresignExpiry)*time.Second); err == nil {
			info.URL = url
		}

		result = append(result, info)
	}

	// Sort by start time descending (most recent first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime.After(result[j].StartTime)
	})

	return result, nil
}

// GetLatestVODURL returns the URL for the latest VOD session of a room.
func (s *PlaybackService) GetLatestVODURL(ctx context.Context, roomID string) (string, string, error) {
	vods, err := s.ListRoomVODs(ctx, roomID)
	if err != nil {
		return "", "", err
	}
	if len(vods) == 0 {
		return "", "", fmt.Errorf("no VOD recordings found for room %s", roomID)
	}

	// Return the latest session's URL
	return vods[0].SessionID, vods[0].URL, nil
}

// GetSessionVODURL returns the URL for a specific VOD session.
func (s *PlaybackService) GetSessionVODURL(ctx context.Context, roomID, sessionID string) (string, error) {
	key := buildStorageKey(s.cfg.VODPrefix, roomID, sessionID, "stream.m3u8")

	// Check if VOD exists
	exists, err := s.provider.Exists(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to check VOD existence: %w", err)
	}
	if !exists {
		return "", fmt.Errorf("VOD not found for room %s session %s", roomID, sessionID)
	}

	return s.provider.GetURL(ctx, key, time.Duration(s.cfg.PresignExpiry)*time.Second)
}

// IsRedirectMode returns true if using redirect mode.
func (s *PlaybackService) IsRedirectMode() bool {
	return s.provider.IsRedirectMode()
}

// ServeLatestPreview serves the latest session's preview image.
func (s *PlaybackService) ServeLatestPreview(ctx context.Context, w http.ResponseWriter, r *http.Request, roomID, filename string) error {
	// Try to get active session first
	var sessionID string
	session, err := s.sessionStore.Get(ctx, roomID)
	if err == nil && session != nil && session.IsActive() {
		sessionID = session.SessionID
	}

	if sessionID == "" {
		// No active session, try to find any preview in storage
		sessionID, err = s.findLatestPreviewSession(ctx, roomID)
		if err != nil {
			return fmt.Errorf("no preview found for room %s: %w", roomID, err)
		}
	}

	return s.ServePreviewContent(ctx, w, r, roomID, sessionID, filename)
}

// ServePreviewContent serves preview content for a specific session.
func (s *PlaybackService) ServePreviewContent(ctx context.Context, w http.ResponseWriter, r *http.Request, roomID, sessionID, filename string) error {
	key := buildPreviewKey(s.cfg.StoragePrefix, roomID, sessionID, filename)
	return s.provider.ServeContent(ctx, w, r, key)
}

// findLatestPreviewSession finds the most recent session with a preview.
func (s *PlaybackService) findLatestPreviewSession(ctx context.Context, roomID string) (string, error) {
	prefix := buildPreviewPrefix(s.cfg.StoragePrefix, roomID)
	files, err := s.provider.List(ctx, prefix)
	if err != nil {
		return "", err
	}

	// Extract session IDs and find the latest
	sessions := make(map[string]bool)
	for _, f := range files {
		// Key format: preview/room_{roomID}/{sessionID}/thumbnail.jpg
		relPath := strings.TrimPrefix(f.Key, prefix)
		parts := strings.Split(relPath, "/")
		if len(parts) >= 1 && parts[0] != "" {
			sessions[parts[0]] = true
		}
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no preview sessions found")
	}

	// Find latest session (sessions are timestamp-based: 2006-01-02T15-04-05Z)
	var latest string
	for sessionID := range sessions {
		if latest == "" || sessionID > latest {
			latest = sessionID
		}
	}

	return latest, nil
}

// buildStorageKey builds a storage key from prefix, roomID, sessionID, and filename.
// Handles empty prefix correctly (no leading slash).
func buildStorageKey(prefix, roomID, sessionID, filename string) string {
	if prefix == "" {
		return fmt.Sprintf("room_%s/%s/%s", roomID, sessionID, filename)
	}
	return fmt.Sprintf("%s/room_%s/%s/%s", prefix, roomID, sessionID, filename)
}

// buildStoragePrefix builds a storage prefix for listing files.
// Handles empty prefix correctly (no leading slash).
func buildStoragePrefix(prefix, roomID string) string {
	if prefix == "" {
		return fmt.Sprintf("room_%s/", roomID)
	}
	return fmt.Sprintf("%s/room_%s/", prefix, roomID)
}

// buildPreviewKey builds a storage key for preview images.
func buildPreviewKey(storagePrefix, roomID, sessionID, filename string) string {
	if storagePrefix == "" {
		return fmt.Sprintf("preview/room_%s/%s/%s", roomID, sessionID, filename)
	}
	return fmt.Sprintf("%s/preview/room_%s/%s/%s", storagePrefix, roomID, sessionID, filename)
}

// buildPreviewPrefix builds a storage prefix for listing preview files.
func buildPreviewPrefix(storagePrefix, roomID string) string {
	if storagePrefix == "" {
		return fmt.Sprintf("preview/room_%s/", roomID)
	}
	return fmt.Sprintf("%s/preview/room_%s/", storagePrefix, roomID)
}
