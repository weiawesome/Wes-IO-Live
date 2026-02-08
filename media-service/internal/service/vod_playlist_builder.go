package service

import (
	"bytes"
	"fmt"
	"sync"
)

// SegmentInfo represents information about a single HLS segment.
type SegmentInfo struct {
	Index    int
	Filename string
	Duration float64
	S3Key    string
	Uploaded bool
}

// VODPlaylistBuilder builds and manages VOD m3u8 playlists.
type VODPlaylistBuilder struct {
	roomID         string
	targetDuration int
	segments       []SegmentInfo
	mu             sync.RWMutex
}

// NewVODPlaylistBuilder creates a new VOD playlist builder.
func NewVODPlaylistBuilder(roomID string, targetDuration int) *VODPlaylistBuilder {
	if targetDuration <= 0 {
		targetDuration = 2
	}
	return &VODPlaylistBuilder{
		roomID:         roomID,
		targetDuration: targetDuration,
		segments:       make([]SegmentInfo, 0),
	}
}

// AddSegment adds a new segment to the playlist.
func (b *VODPlaylistBuilder) AddSegment(seg SegmentInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if segment already exists (by index)
	for i, existing := range b.segments {
		if existing.Index == seg.Index {
			// Update existing segment
			b.segments[i] = seg
			return
		}
	}

	b.segments = append(b.segments, seg)
}

// MarkSegmentUploaded marks a segment as uploaded to S3.
func (b *VODPlaylistBuilder) MarkSegmentUploaded(index int, s3Key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, seg := range b.segments {
		if seg.Index == index {
			b.segments[i].Uploaded = true
			b.segments[i].S3Key = s3Key
			return
		}
	}
}

// GetSegments returns a copy of all segments.
func (b *VODPlaylistBuilder) GetSegments() []SegmentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	segments := make([]SegmentInfo, len(b.segments))
	copy(segments, b.segments)
	return segments
}

// GetUploadedSegments returns only uploaded segments.
func (b *VODPlaylistBuilder) GetUploadedSegments() []SegmentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var uploaded []SegmentInfo
	for _, seg := range b.segments {
		if seg.Uploaded {
			uploaded = append(uploaded, seg)
		}
	}
	return uploaded
}

// GetPendingSegments returns segments that haven't been uploaded yet.
func (b *VODPlaylistBuilder) GetPendingSegments() []SegmentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var pending []SegmentInfo
	for _, seg := range b.segments {
		if !seg.Uploaded {
			pending = append(pending, seg)
		}
	}
	return pending
}

// SegmentCount returns the total number of segments.
func (b *VODPlaylistBuilder) SegmentCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.segments)
}

// GenerateM3U8 generates the m3u8 playlist content.
// If finalized is true, includes #EXT-X-ENDLIST tag.
// Only includes segments that have been uploaded.
func (b *VODPlaylistBuilder) GenerateM3U8(finalized bool) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var buf bytes.Buffer

	// Header
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:3\n")
	buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", b.targetDuration))
	buf.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	if finalized {
		buf.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	}

	// Segments - only include uploaded ones
	for _, seg := range b.segments {
		if !seg.Uploaded {
			continue
		}
		buf.WriteString(fmt.Sprintf("#EXTINF:%.6f,\n", seg.Duration))
		buf.WriteString(seg.Filename + "\n")
	}

	// End tag for finalized playlists
	if finalized {
		buf.WriteString("#EXT-X-ENDLIST\n")
	}

	return buf.Bytes()
}

// GenerateM3U8WithAllSegments generates playlist with all segments (for preview).
func (b *VODPlaylistBuilder) GenerateM3U8WithAllSegments(finalized bool) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var buf bytes.Buffer

	// Header
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:3\n")
	buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", b.targetDuration))
	buf.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	if finalized {
		buf.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	}

	// All segments
	for _, seg := range b.segments {
		buf.WriteString(fmt.Sprintf("#EXTINF:%.6f,\n", seg.Duration))
		buf.WriteString(seg.Filename + "\n")
	}

	// End tag for finalized playlists
	if finalized {
		buf.WriteString("#EXT-X-ENDLIST\n")
	}

	return buf.Bytes()
}

// Clear removes all segments.
func (b *VODPlaylistBuilder) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.segments = make([]SegmentInfo, 0)
}

// GetRoomID returns the room ID associated with this builder.
func (b *VODPlaylistBuilder) GetRoomID() string {
	return b.roomID
}
