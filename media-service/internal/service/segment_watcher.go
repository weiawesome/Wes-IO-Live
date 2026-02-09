package service

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SegmentWatcherCallback is called when a new segment is detected.
// The callback receives roomID, sessionID, and segment info.
type SegmentWatcherCallback func(roomID string, sessionID string, seg SegmentInfo)

// watchKey uniquely identifies a watcher for a room+session combination
type watchKey struct {
	RoomID    string
	SessionID string
}

func (k watchKey) String() string {
	return k.RoomID + ":" + k.SessionID
}

// SegmentWatcher monitors HLS output directories for new segments.
type SegmentWatcher struct {
	hlsOutputDir string
	callback     SegmentWatcherCallback
	watchers     map[string]*fsnotify.Watcher // watchKey.String() -> watcher
	known        map[string]map[string]bool   // watchKey.String() -> filename -> seen
	mu           sync.RWMutex
	pollInterval time.Duration
}

// NewSegmentWatcher creates a new segment watcher.
func NewSegmentWatcher(hlsOutputDir string, callback SegmentWatcherCallback) *SegmentWatcher {
	return &SegmentWatcher{
		hlsOutputDir: hlsOutputDir,
		callback:     callback,
		watchers:     make(map[string]*fsnotify.Watcher),
		known:        make(map[string]map[string]bool),
		pollInterval: 500 * time.Millisecond,
	}
}

// StartWatchingSession begins monitoring a room's session HLS output directory.
// Directory structure: {hlsOutputDir}/room_{roomID}/{sessionID}/
func (w *SegmentWatcher) StartWatchingSession(roomID, sessionID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	key := watchKey{RoomID: roomID, SessionID: sessionID}
	keyStr := key.String()

	// Check if already watching
	if _, exists := w.watchers[keyStr]; exists {
		return nil
	}

	sessionDir := filepath.Join(w.hlsOutputDir, "room_"+roomID, sessionID)

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	// Initialize known segments map for this session
	w.known[keyStr] = make(map[string]bool)

	// Start watching
	w.watchers[keyStr] = watcher

	// Start event handler goroutine
	go w.handleSessionEvents(key, watcher, sessionDir)

	// Wait for directory to exist and start watching
	go w.waitAndWatchSession(key, watcher, sessionDir)

	log.Printf("Started watching for segments in room %s session %s", roomID, sessionID)
	return nil
}

// StartWatching begins monitoring a room's HLS output directory (legacy support).
// This method is kept for backward compatibility but uses the default session directory structure.
func (w *SegmentWatcher) StartWatching(roomID string) error {
	// For backward compatibility, use empty sessionID which means direct room directory
	w.mu.Lock()
	defer w.mu.Unlock()

	key := watchKey{RoomID: roomID, SessionID: ""}
	keyStr := key.String()

	// Check if already watching
	if _, exists := w.watchers[keyStr]; exists {
		return nil
	}

	roomDir := filepath.Join(w.hlsOutputDir, "room_"+roomID)

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	// Initialize known segments map for this room
	w.known[keyStr] = make(map[string]bool)

	// Start watching
	w.watchers[keyStr] = watcher

	// Start event handler goroutine
	go w.handleSessionEvents(key, watcher, roomDir)

	// Wait for directory to exist
	go w.waitAndWatchSession(key, watcher, roomDir)

	log.Printf("Started watching for segments in room %s", roomID)
	return nil
}

// waitAndWatchSession waits for the directory to exist and then starts watching.
func (w *SegmentWatcher) waitAndWatchSession(key watchKey, watcher *fsnotify.Watcher, dir string) {
	// Wait for directory to be created (max 30 seconds)
	for i := 0; i < 60; i++ {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Add directory to watcher
	if err := watcher.Add(dir); err != nil {
		log.Printf("Failed to watch directory %s: %v", dir, err)
		return
	}

	// Do initial scan for existing segments
	w.scanSessionPlaylist(key, dir)

	// Start polling for playlist changes (backup mechanism)
	go w.pollSessionPlaylist(key, dir)
}

// handleSessionEvents processes fsnotify events for a session.
func (w *SegmentWatcher) handleSessionEvents(key watchKey, watcher *fsnotify.Watcher, dir string) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// When playlist is modified, scan for new segments
			if strings.HasSuffix(event.Name, ".m3u8") && (event.Op&fsnotify.Write != 0) {
				w.scanSessionPlaylist(key, dir)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error for room %s session %s: %v", key.RoomID, key.SessionID, err)
		}
	}
}

// pollSessionPlaylist periodically scans the playlist as a backup mechanism.
func (w *SegmentWatcher) pollSessionPlaylist(key watchKey, dir string) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	keyStr := key.String()

	for {
		<-ticker.C

		w.mu.RLock()
		_, exists := w.watchers[keyStr]
		w.mu.RUnlock()

		if !exists {
			return
		}

		w.scanSessionPlaylist(key, dir)
	}
}

// scanSessionPlaylist parses the m3u8 file and detects new segments.
func (w *SegmentWatcher) scanSessionPlaylist(key watchKey, dir string) {
	playlistPath := filepath.Join(dir, "stream.m3u8")

	file, err := os.Open(playlistPath)
	if err != nil {
		return // Playlist might not exist yet
	}
	defer file.Close()

	keyStr := key.String()

	w.mu.Lock()
	knownSegments := w.known[keyStr]
	if knownSegments == nil {
		knownSegments = make(map[string]bool)
		w.known[keyStr] = knownSegments
	}
	w.mu.Unlock()

	scanner := bufio.NewScanner(file)
	var currentDuration float64

	// Regex to extract segment index from filename
	segmentRegex := regexp.MustCompile(`segment_(\d+)\.ts`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "#EXTINF:") {
			// Parse duration
			durationStr := strings.TrimPrefix(line, "#EXTINF:")
			durationStr = strings.TrimSuffix(durationStr, ",")
			if d, err := strconv.ParseFloat(durationStr, 64); err == nil {
				currentDuration = d
			}
		} else if strings.HasSuffix(line, ".ts") {
			filename := line

			// Skip if already known
			w.mu.RLock()
			seen := knownSegments[filename]
			w.mu.RUnlock()

			if seen {
				continue
			}

			// Mark as known
			w.mu.Lock()
			knownSegments[filename] = true
			w.mu.Unlock()

			// Extract segment index
			matches := segmentRegex.FindStringSubmatch(filename)
			index := -1
			if len(matches) >= 2 {
				if idx, err := strconv.Atoi(matches[1]); err == nil {
					index = idx
				}
			}

			// Verify file exists and is complete
			segmentPath := filepath.Join(dir, filename)
			if !w.isSegmentComplete(segmentPath) {
				// Remove from known so we retry
				w.mu.Lock()
				delete(knownSegments, filename)
				w.mu.Unlock()
				continue
			}

			// Create segment info
			seg := SegmentInfo{
				Index:    index,
				Filename: filename,
				Duration: currentDuration,
			}

			// Notify callback
			if w.callback != nil {
				go w.callback(key.RoomID, key.SessionID, seg)
			}

			log.Printf("New segment detected for room %s session %s: %s (duration: %.2fs)",
				key.RoomID, key.SessionID, filename, currentDuration)
		}
	}
}

// isSegmentComplete checks if a segment file is complete (not being written).
func (w *SegmentWatcher) isSegmentComplete(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check if file is non-empty
	if info.Size() == 0 {
		return false
	}

	// Wait a short time and check if size changed
	time.Sleep(50 * time.Millisecond)

	info2, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.Size() == info2.Size()
}

// StopWatchingSession stops monitoring a specific session.
func (w *SegmentWatcher) StopWatchingSession(roomID, sessionID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	key := watchKey{RoomID: roomID, SessionID: sessionID}
	keyStr := key.String()

	if watcher, exists := w.watchers[keyStr]; exists {
		watcher.Close()
		delete(w.watchers, keyStr)
		delete(w.known, keyStr)
		log.Printf("Stopped watching segments for room %s session %s", roomID, sessionID)
	}
}

// StopWatching stops monitoring a room (legacy support).
func (w *SegmentWatcher) StopWatching(roomID string) {
	w.StopWatchingSession(roomID, "")
}

// GetKnownSegments returns all known segments for a session.
func (w *SegmentWatcher) GetKnownSegments(roomID, sessionID string) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	key := watchKey{RoomID: roomID, SessionID: sessionID}
	known := w.known[key.String()]
	if known == nil {
		return nil
	}

	segments := make([]string, 0, len(known))
	for filename := range known {
		segments = append(segments, filename)
	}
	return segments
}

// StopAll stops all watchers.
func (w *SegmentWatcher) StopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for keyStr, watcher := range w.watchers {
		watcher.Close()
		delete(w.watchers, keyStr)
		delete(w.known, keyStr)
	}
	log.Println("All segment watchers stopped")
}
