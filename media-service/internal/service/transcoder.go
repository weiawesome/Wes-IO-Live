package service

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/ivfwriter"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"github.com/weiawesome/wes-io-live/media-service/internal/config"
)

// Transcoder handles video transcoding to HLS.
type Transcoder struct {
	config    config.HLSConfig
	ffmpegCfg config.FFmpegConfig

	processes map[string]*transcoderProcess
	mu        sync.RWMutex
}

type transcoderProcess struct {
	roomID    string
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser
	outputDir string
	videoPipe string
	audioPipe string
	done      chan struct{}
}

// NewTranscoder creates a new Transcoder.
func NewTranscoder(hlsCfg config.HLSConfig, ffmpegCfg config.FFmpegConfig) *Transcoder {
	return &Transcoder{
		config:    hlsCfg,
		ffmpegCfg: ffmpegCfg,
		processes: make(map[string]*transcoderProcess),
	}
}

// createPipes creates named pipes for video and audio input.
func (t *Transcoder) createPipes(roomID string) (videoPipe, audioPipe string, err error) {
	videoPipe = filepath.Join(os.TempDir(), fmt.Sprintf("webrtc_video_%s.ivf", roomID))
	audioPipe = filepath.Join(os.TempDir(), fmt.Sprintf("webrtc_audio_%s.ogg", roomID))

	// Remove existing pipes
	os.Remove(videoPipe)
	os.Remove(audioPipe)

	if err := syscall.Mkfifo(videoPipe, 0666); err != nil {
		return "", "", fmt.Errorf("failed to create video pipe: %w", err)
	}
	if err := syscall.Mkfifo(audioPipe, 0666); err != nil {
		os.Remove(videoPipe)
		return "", "", fmt.Errorf("failed to create audio pipe: %w", err)
	}

	return videoPipe, audioPipe, nil
}

// cleanupPipes removes named pipes for a process.
func (t *Transcoder) cleanupPipes(process *transcoderProcess) {
	if process.videoPipe != "" {
		os.Remove(process.videoPipe)
	}
	if process.audioPipe != "" {
		os.Remove(process.audioPipe)
	}
}

// buildVideoArgs builds FFmpeg video encoding arguments based on config.
func (t *Transcoder) buildVideoArgs() []string {
	args := []string{
		"-c:v", t.ffmpegCfg.VideoCodec,
		"-preset", t.ffmpegCfg.VideoPreset,
		"-tune", "zerolatency",
		"-profile:v", "baseline",
		"-level", "3.0",
		"-pix_fmt", "yuv420p",
	}

	// Video bitrate or CRF
	if t.ffmpegCfg.VideoBitrate != "" {
		args = append(args, "-b:v", t.ffmpegCfg.VideoBitrate)
	} else if t.ffmpegCfg.VideoCRF > 0 {
		args = append(args, "-crf", fmt.Sprintf("%d", t.ffmpegCfg.VideoCRF))
	}

	// Resolution scaling
	if t.ffmpegCfg.Width > 0 && t.ffmpegCfg.Height > 0 {
		// Scale to exact resolution
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", t.ffmpegCfg.Width, t.ffmpegCfg.Height))
	} else if t.ffmpegCfg.Width > 0 {
		// Scale width, keep aspect ratio
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", t.ffmpegCfg.Width))
	} else if t.ffmpegCfg.Height > 0 {
		// Scale height, keep aspect ratio
		args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", t.ffmpegCfg.Height))
	}

	// Framerate
	if t.ffmpegCfg.Framerate > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", t.ffmpegCfg.Framerate))
	}

	// GOP size (keyframe interval) - use framerate or default to 30
	gop := 30
	if t.ffmpegCfg.Framerate > 0 {
		gop = t.ffmpegCfg.Framerate
	}
	args = append(args, "-g", fmt.Sprintf("%d", gop))

	return args
}

// buildAudioArgs builds FFmpeg audio encoding arguments based on config.
func (t *Transcoder) buildAudioArgs() []string {
	bitrate := t.ffmpegCfg.AudioBitrate
	if bitrate == "" {
		bitrate = "128k"
	}

	sampleRate := t.ffmpegCfg.AudioSample
	if sampleRate == 0 {
		sampleRate = 48000
	}

	return []string{
		"-c:a", t.ffmpegCfg.AudioCodec,
		"-b:a", bitrate,
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "2",
		"-async", "1", // Stretch/squeeze audio to sync with video timeline
	}
}

// StartHLS starts HLS transcoding for a room with optional audio.
// If sessionID is provided, outputs to room_{roomID}/{sessionID}/ directory.
func (t *Transcoder) StartHLS(roomID, sessionID string, videoTrack, audioTrack *webrtc.TrackRemote) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create process key (includes sessionID if provided)
	processKey := roomID
	if sessionID != "" {
		processKey = roomID + ":" + sessionID
	}

	// Check if already running
	if _, exists := t.processes[processKey]; exists {
		return "", fmt.Errorf("transcoder already running for room %s", roomID)
	}

	// Create output directory for this room/session
	var outputDir string
	if sessionID != "" {
		outputDir = filepath.Join(t.config.OutputDir, "room_"+roomID, sessionID)
	} else {
		outputDir = filepath.Join(t.config.OutputDir, "room_"+roomID)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Clean existing files
	t.cleanDir(outputDir)

	outputPath := filepath.Join(outputDir, "stream.m3u8")
	segmentPath := filepath.Join(outputDir, "segment_%03d.ts")

	hlsFlags := "delete_segments+append_list"
	if !t.config.DeleteSegments {
		hlsFlags = "append_list"
	}

	var process *transcoderProcess
	var cmd *exec.Cmd

	// Build common HLS arguments
	hlsArgs := []string{
		"-vsync", "cfr", // Constant frame rate output for sync
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", t.config.SegmentDuration),
		"-hls_list_size", fmt.Sprintf("%d", t.config.PlaylistSize),
		"-hls_flags", hlsFlags,
		"-hls_segment_filename", segmentPath,
		outputPath,
	}

	if audioTrack != nil {
		// With audio: use named pipes for both video and audio
		videoPipe, audioPipe, err := t.createPipes(roomID)
		if err != nil {
			return "", err
		}

		// Build FFmpeg arguments with audio
		// Use wallclock timestamps and generate PTS for proper A/V sync
		args := []string{
			"-use_wallclock_as_timestamps", "1",
			"-fflags", "+genpts",
			"-f", "ivf",
			"-i", videoPipe,
			"-use_wallclock_as_timestamps", "1",
			"-fflags", "+genpts",
			"-f", "ogg",
			"-i", audioPipe,
		}
		args = append(args, t.buildVideoArgs()...)
		args = append(args, t.buildAudioArgs()...)
		args = append(args, hlsArgs...)

		cmd = exec.Command("ffmpeg", args...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout

		if err := cmd.Start(); err != nil {
			t.cleanupPipes(&transcoderProcess{videoPipe: videoPipe, audioPipe: audioPipe})
			return "", fmt.Errorf("failed to start ffmpeg: %w", err)
		}

		process = &transcoderProcess{
			roomID:    roomID,
			cmd:       cmd,
			outputDir: outputDir,
			videoPipe: videoPipe,
			audioPipe: audioPipe,
			done:      make(chan struct{}),
		}

		t.processes[processKey] = process

		// Start goroutines to write video and audio to pipes
		// Use channel to synchronize startup for better A/V sync
		startSignal := make(chan struct{})

		go func() {
			<-startSignal
			t.writeVideoToPipe(roomID, videoTrack, videoPipe, process.done)
		}()

		go func() {
			<-startSignal
			t.writeAudioToPipe(roomID, audioTrack, audioPipe, process.done)
		}()

		close(startSignal) // Start both goroutines simultaneously

		log.Printf("FFmpeg HLS transcoding started for room %s session %s (with audio)", roomID, sessionID)
	} else {
		// Without audio: use stdin pipe for video only
		args := []string{
			"-f", "ivf",
			"-i", "pipe:0",
		}
		args = append(args, t.buildVideoArgs()...)
		args = append(args, "-an")
		args = append(args, hlsArgs...)

		cmd = exec.Command("ffmpeg", args...)

		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			return "", fmt.Errorf("failed to get stdin pipe: %w", err)
		}

		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout

		if err := cmd.Start(); err != nil {
			return "", fmt.Errorf("failed to start ffmpeg: %w", err)
		}

		process = &transcoderProcess{
			roomID:    roomID,
			cmd:       cmd,
			stdinPipe: stdinPipe,
			outputDir: outputDir,
			done:      make(chan struct{}),
		}

		t.processes[processKey] = process

		// Start goroutine to write video frames to FFmpeg
		go t.writeVideoToFFmpeg(roomID, videoTrack, stdinPipe, process.done)

		log.Printf("FFmpeg HLS transcoding started for room %s session %s (video only)", roomID, sessionID)
	}

	// Monitor FFmpeg process
	go func() {
		cmd.Wait()
		t.mu.Lock()
		proc, exists := t.processes[processKey]
		if exists {
			delete(t.processes, processKey)
			t.cleanupPipes(proc)
		}
		t.mu.Unlock()
		close(process.done)
		log.Printf("FFmpeg process ended for room %s session %s", roomID, sessionID)
	}()

	// Return the HLS URL path (new format: /live/{roomID}/{sessionID}/...)
	var hlsUrl string
	if sessionID != "" {
		hlsUrl = fmt.Sprintf("/live/%s/%s/stream.m3u8", roomID, sessionID)
	} else {
		hlsUrl = fmt.Sprintf("/live/%s/stream.m3u8", roomID)
	}
	return hlsUrl, nil
}

// StopHLS stops HLS transcoding for a room/session.
func (t *Transcoder) StopHLS(roomID, sessionID string) error {
	t.mu.Lock()

	// Create process key
	processKey := roomID
	if sessionID != "" {
		processKey = roomID + ":" + sessionID
	}

	process, exists := t.processes[processKey]
	if !exists {
		t.mu.Unlock()
		return nil
	}
	delete(t.processes, processKey)
	t.mu.Unlock()

	// Close stdin to signal FFmpeg to finish
	if process.stdinPipe != nil {
		process.stdinPipe.Close()
	}

	// Kill the process if still running
	if process.cmd != nil && process.cmd.Process != nil {
		process.cmd.Process.Kill()
	}

	// Cleanup named pipes
	t.cleanupPipes(process)

	log.Printf("FFmpeg stopped for room %s session %s", roomID, sessionID)
	return nil
}

// CleanupRoom removes HLS files for a room (all sessions).
func (t *Transcoder) CleanupRoom(roomID string) error {
	outputDir := filepath.Join(t.config.OutputDir, "room_"+roomID)
	return os.RemoveAll(outputDir)
}

// CleanupSession removes HLS files for a specific session.
func (t *Transcoder) CleanupSession(roomID, sessionID string) error {
	if sessionID == "" {
		return t.CleanupRoom(roomID)
	}
	outputDir := filepath.Join(t.config.OutputDir, "room_"+roomID, sessionID)
	return os.RemoveAll(outputDir)
}

func (t *Transcoder) writeVideoToFFmpeg(roomID string, track *webrtc.TrackRemote, w io.WriteCloser, done chan struct{}) {
	defer w.Close()

	codec := track.Codec()
	log.Printf("Video codec for room %s: %s", roomID, codec.MimeType)

	// Create IVF writer based on codec
	switch codec.MimeType {
	case webrtc.MimeTypeVP8, webrtc.MimeTypeVP9:
		t.writeIVF(roomID, track, w, done)
	case webrtc.MimeTypeH264:
		t.writeH264(roomID, track, w, done)
	default:
		log.Printf("Unsupported codec for room %s: %s", roomID, codec.MimeType)
	}
}

func (t *Transcoder) writeIVF(roomID string, track *webrtc.TrackRemote, w io.WriteCloser, done chan struct{}) {
	ivf, err := ivfwriter.NewWith(w, ivfwriter.WithCodec(track.Codec().MimeType))
	if err != nil {
		log.Printf("Failed to create IVF writer for room %s: %v", roomID, err)
		return
	}
	defer ivf.Close()

	log.Printf("Writing video frames to FFmpeg for room %s...", roomID)

	// Simple loop without select - ReadRTP will return error when connection closes
	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("RTP read error for room %s: %v", roomID, err)
			return
		}

		if err := ivf.WriteRTP(rtpPacket); err != nil {
			log.Printf("IVF write error for room %s: %v", roomID, err)
			return
		}
	}
}

func (t *Transcoder) writeH264(roomID string, track *webrtc.TrackRemote, w io.WriteCloser, done chan struct{}) {
	depacketizer := &codecs.H264Packet{}

	// Simple loop without select - ReadRTP will return error when connection closes
	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("RTP read error for room %s: %v", roomID, err)
			return
		}

		// Depacketize H264
		payload, err := depacketizer.Unmarshal(rtpPacket.Payload)
		if err != nil || len(payload) == 0 {
			continue
		}

		// Write NAL unit with start code
		startCode := []byte{0x00, 0x00, 0x00, 0x01}
		if _, err := w.Write(startCode); err != nil {
			log.Printf("Write error for room %s: %v", roomID, err)
			return
		}
		if _, err := w.Write(payload); err != nil {
			log.Printf("Write error for room %s: %v", roomID, err)
			return
		}
	}
}

func (t *Transcoder) writeVideoToPipe(roomID string, track *webrtc.TrackRemote, pipePath string, done chan struct{}) {
	f, err := os.OpenFile(pipePath, os.O_WRONLY, os.ModeNamedPipe)
	if err != nil {
		log.Printf("Failed to open video pipe for room %s: %v", roomID, err)
		return
	}
	defer f.Close()

	codec := track.Codec()
	log.Printf("Video codec for room %s: %s (via pipe)", roomID, codec.MimeType)

	switch codec.MimeType {
	case webrtc.MimeTypeVP8, webrtc.MimeTypeVP9:
		t.writeIVFToPipe(roomID, track, f, done)
	case webrtc.MimeTypeH264:
		t.writeH264(roomID, track, f, done)
	default:
		log.Printf("Unsupported codec for room %s: %s", roomID, codec.MimeType)
	}
}

func (t *Transcoder) writeIVFToPipe(roomID string, track *webrtc.TrackRemote, w io.WriteCloser, done chan struct{}) {
	ivf, err := ivfwriter.NewWith(w, ivfwriter.WithCodec(track.Codec().MimeType))
	if err != nil {
		log.Printf("Failed to create IVF writer for room %s: %v", roomID, err)
		return
	}
	defer ivf.Close()

	log.Printf("Writing video frames to pipe for room %s...", roomID)

	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("Video RTP read error for room %s: %v", roomID, err)
			return
		}

		if err := ivf.WriteRTP(rtpPacket); err != nil {
			log.Printf("Video IVF write error for room %s: %v", roomID, err)
			return
		}
	}
}

func (t *Transcoder) writeAudioToPipe(roomID string, track *webrtc.TrackRemote, pipePath string, done chan struct{}) {
	f, err := os.OpenFile(pipePath, os.O_WRONLY, os.ModeNamedPipe)
	if err != nil {
		log.Printf("Failed to open audio pipe for room %s: %v", roomID, err)
		return
	}
	defer f.Close()

	ogg, err := oggwriter.NewWith(f, 48000, 2)
	if err != nil {
		log.Printf("Failed to create OGG writer for room %s: %v", roomID, err)
		return
	}
	defer ogg.Close()

	log.Printf("Writing audio frames to pipe for room %s...", roomID)

	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("Audio RTP read error for room %s: %v", roomID, err)
			return
		}

		if err := ogg.WriteRTP(rtpPacket); err != nil {
			log.Printf("Audio OGG write error for room %s: %v", roomID, err)
			return
		}
	}
}

func (t *Transcoder) cleanDir(dir string) {
	patterns := []string{"*.ts", "*.m3u8"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range files {
			os.Remove(f)
		}
	}
}

// IsRunning checks if transcoder is running for a room/session.
func (t *Transcoder) IsRunning(roomID, sessionID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	processKey := roomID
	if sessionID != "" {
		processKey = roomID + ":" + sessionID
	}

	_, exists := t.processes[processKey]
	return exists
}

// GetSessionDir returns the HLS output directory for a room/session.
func (t *Transcoder) GetSessionDir(roomID, sessionID string) string {
	if sessionID != "" {
		return filepath.Join(t.config.OutputDir, "room_"+roomID, sessionID)
	}
	return filepath.Join(t.config.OutputDir, "room_"+roomID)
}
