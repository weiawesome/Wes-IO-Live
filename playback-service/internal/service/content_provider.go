package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/weiawesome/wes-io-live/pkg/storage"
	"github.com/weiawesome/wes-io-live/playback-service/internal/config"
)

// ContentProvider handles content delivery from storage.
// It supports both redirect mode (presigned URLs) and proxy mode (direct streaming).
type ContentProvider struct {
	storage      storage.Storage
	redirectMode bool
	presignTTL   time.Duration
}

// NewContentProvider creates a new content provider.
func NewContentProvider(store storage.Storage, cfg config.PlaybackConfig, storageType string) *ContentProvider {
	// Redirect mode only works with S3 storage
	redirectMode := cfg.AccessMode == "redirect" && storageType == "s3"

	return &ContentProvider{
		storage:      store,
		redirectMode: redirectMode,
		presignTTL:   time.Duration(cfg.PresignExpiry) * time.Second,
	}
}

// ServeContent serves content from storage to the HTTP response.
// In redirect mode, it returns a 302 redirect to a presigned URL.
// In proxy mode, it streams the content directly.
func (p *ContentProvider) ServeContent(ctx context.Context, w http.ResponseWriter, r *http.Request, key string) error {
	// Security: validate key to prevent directory traversal
	cleanKey := filepath.Clean(key)
	if strings.Contains(cleanKey, "..") {
		return fmt.Errorf("invalid key: directory traversal attempt")
	}

	if p.redirectMode {
		return p.redirectToPresignedURL(ctx, w, r, key)
	}
	return p.proxyContent(ctx, w, r, key)
}

// redirectToPresignedURL redirects the client to a presigned URL.
func (p *ContentProvider) redirectToPresignedURL(ctx context.Context, w http.ResponseWriter, r *http.Request, key string) error {
	url, err := p.storage.GetURL(ctx, key, p.presignTTL)
	if err != nil {
		return fmt.Errorf("failed to get presigned URL: %w", err)
	}

	http.Redirect(w, r, url, http.StatusFound)
	return nil
}

// proxyContent streams content from storage to the client.
func (p *ContentProvider) proxyContent(ctx context.Context, w http.ResponseWriter, r *http.Request, key string) error {
	// Check if content exists
	exists, err := p.storage.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check content existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("content not found: %s", key)
	}

	// Read content from storage
	reader, err := p.storage.Read(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to read content: %w", err)
	}
	defer reader.Close()

	// Set content type and caching headers based on file extension
	ext := filepath.Ext(key)
	setContentHeaders(w, ext)

	// Stream content to client
	_, err = io.Copy(w, reader)
	if err != nil {
		// Client may have disconnected, log but don't return error
		return nil
	}

	return nil
}

// GetURL returns a URL for accessing content.
// In redirect mode with S3, returns a presigned URL.
// In proxy mode or with local storage, returns a relative path.
func (p *ContentProvider) GetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return p.storage.GetURL(ctx, key, ttl)
}

// Exists checks if content exists in storage.
func (p *ContentProvider) Exists(ctx context.Context, key string) (bool, error) {
	return p.storage.Exists(ctx, key)
}

// List returns a list of files with the given prefix.
func (p *ContentProvider) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	return p.storage.List(ctx, prefix)
}

// IsRedirectMode returns true if using redirect mode.
func (p *ContentProvider) IsRedirectMode() bool {
	return p.redirectMode
}

// setContentHeaders sets appropriate content type and caching headers.
func setContentHeaders(w http.ResponseWriter, ext string) {
	switch ext {
	case ".m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "public, max-age=10")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=5") // Short cache for live previews
	case ".png":
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=5")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Cache-Control", "public, max-age=5")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
}
