package service

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/weiawesome/wes-io-live/media-service/internal/config"
)

// ThumbnailService manages periodic thumbnail capture for live streams.
type ThumbnailService struct {
	cfg          config.PreviewConfig
	hlsOutputDir string
	uploader     *S3Uploader

	// Active capture sessions: roomID:sessionID -> captureSession
	sessions map[string]*captureSession
	mu       sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// captureSession holds the state for a single thumbnail capture session.
type captureSession struct {
	roomID    string
	sessionID string
	cancel    context.CancelFunc
}

// ThumbnailServiceConfig holds configuration for thumbnail service.
type ThumbnailServiceConfig struct {
	PreviewConfig config.PreviewConfig
	HLSOutputDir  string      // HLS output directory for capturing thumbnails
	Uploader      *S3Uploader // Shared uploader from VODManager
}

// NewThumbnailService creates a new thumbnail service.
func NewThumbnailService(cfg ThumbnailServiceConfig) *ThumbnailService {
	ctx, cancel := context.WithCancel(context.Background())

	return &ThumbnailService{
		cfg:          cfg.PreviewConfig,
		hlsOutputDir: cfg.HLSOutputDir,
		uploader:     cfg.Uploader,
		sessions:     make(map[string]*captureSession),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// StartCapture begins periodic thumbnail capture for a room/session from HLS stream.
func (s *ThumbnailService) StartCapture(roomID, sessionID string) {
	if !s.cfg.Enabled || s.uploader == nil {
		return
	}

	key := sessionKey(roomID, sessionID)

	s.mu.Lock()
	if _, exists := s.sessions[key]; exists {
		s.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	session := &captureSession{
		roomID:    roomID,
		sessionID: sessionID,
		cancel:    cancel,
	}

	s.sessions[key] = session
	s.mu.Unlock()

	go s.captureLoop(ctx, session)
	log.Printf("Thumbnail capture started for room %s session %s (from HLS)", roomID, sessionID)
}

// StopCapture stops thumbnail capture for a room/session.
func (s *ThumbnailService) StopCapture(roomID, sessionID string) {
	key := sessionKey(roomID, sessionID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if session, exists := s.sessions[key]; exists {
		session.cancel()
		delete(s.sessions, key)
		log.Printf("Thumbnail capture stopped for room %s session %s", roomID, sessionID)
	}
}

// Stop stops all capture sessions.
func (s *ThumbnailService) Stop() {
	s.cancel()

	s.mu.Lock()
	for key, session := range s.sessions {
		session.cancel()
		delete(s.sessions, key)
	}
	s.mu.Unlock()

	log.Println("Thumbnail service stopped")
}

// captureLoop runs periodic thumbnail capture from HLS stream.
func (s *ThumbnailService) captureLoop(ctx context.Context, session *captureSession) {
	// Wait for initial delay before first capture (allows HLS buffer to fill for better quality)
	initialDelay := s.cfg.InitialDelaySeconds
	if initialDelay <= 0 {
		initialDelay = 10 // Default 10 seconds
	}

	log.Printf("Waiting %d seconds before first thumbnail capture for room %s", initialDelay, session.roomID)

	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Duration(initialDelay) * time.Second):
	}

	// Capture first thumbnail
	s.captureThumbnailFromHLS(ctx, session)

	// Then capture every interval_seconds
	ticker := time.NewTicker(time.Duration(s.cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.captureThumbnailFromHLS(ctx, session)
		}
	}
}

// captureThumbnailFromHLS captures a single thumbnail from HLS stream.
// Returns true if successful, false otherwise.
func (s *ThumbnailService) captureThumbnailFromHLS(ctx context.Context, session *captureSession) bool {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return false
	}

	hlsDir := filepath.Join(s.hlsOutputDir, "room_"+session.roomID, session.sessionID)
	m3u8Path := filepath.Join(hlsDir, "stream.m3u8")

	// Check if HLS stream exists
	if _, err := os.Stat(m3u8Path); os.IsNotExist(err) {
		return false
	}

	// Capture frame from HLS using FFmpeg
	jpegData, err := s.captureFromHLS(ctx, m3u8Path)
	if err != nil {
		// Don't log error for first attempts (HLS might not be ready)
		return false
	}

	// Upload to S3
	s3Key := fmt.Sprintf("preview/room_%s/%s/thumbnail.jpg", session.roomID, session.sessionID)

	uploadCtx, uploadCancel := context.WithTimeout(ctx, 30*time.Second)
	defer uploadCancel()

	err = s.uploader.UploadReader(uploadCtx, bytes.NewReader(jpegData), int64(len(jpegData)), s3Key, "image/jpeg")
	if err != nil {
		log.Printf("Failed to upload thumbnail for room %s: %v", session.roomID, err)
		return false
	}

	log.Printf("Thumbnail uploaded for room %s session %s", session.roomID, session.sessionID)
	return true
}

// captureFromHLS captures a frame from HLS stream using FFmpeg.
func (s *ThumbnailService) captureFromHLS(ctx context.Context, m3u8Path string) ([]byte, error) {
	// Calculate quality for JPEG output
	// FFmpeg mjpeg uses -q:v (2-31, lower is better)
	// Config quality is 0-100 (higher is better)
	quality := 2 + ((100 - s.cfg.Quality) * 29 / 100)
	if quality < 2 {
		quality = 2
	}
	if quality > 31 {
		quality = 31
	}

	// Build FFmpeg command to capture from HLS
	// Use scale filter that preserves aspect ratio
	scaleFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		s.cfg.Width, s.cfg.Height, s.cfg.Width, s.cfg.Height)

	args := []string{
		"-y",
		"-i", m3u8Path,
		"-vframes", "1",
		"-vf", scaleFilter,
		"-q:v", fmt.Sprintf("%d", quality),
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "ffmpeg", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w (stderr: %s)", err, stderr.String())
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output")
	}

	return stdout.Bytes(), nil
}

// IsEnabled returns whether thumbnail service is enabled.
func (s *ThumbnailService) IsEnabled() bool {
	return s.cfg.Enabled && s.uploader != nil
}
