package storage

import (
	"context"
	"io"
	"time"
)

// FileInfo represents metadata about a stored file.
type FileInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ContentType  string
}

// Storage defines the interface for file storage operations.
type Storage interface {
	// Write stores content from the reader with the given key.
	// The size parameter is the expected content size (-1 if unknown).
	// The contentType parameter specifies the MIME type of the content.
	Write(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// Read retrieves content for the given key.
	// The caller is responsible for closing the returned ReadCloser.
	Read(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the content with the given key.
	Delete(ctx context.Context, key string) error

	// DeletePrefix removes all content with keys starting with the given prefix.
	DeletePrefix(ctx context.Context, prefix string) error

	// List returns information about all files with keys starting with the given prefix.
	List(ctx context.Context, prefix string) ([]FileInfo, error)

	// Exists checks if content with the given key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// GetURL returns a URL for accessing the content.
	// For local storage, this returns the file path.
	// For S3, this returns a presigned URL valid for the specified duration.
	GetURL(ctx context.Context, key string, expires time.Duration) (string, error)

	// GetUploadURL returns a presigned PUT URL for direct client upload.
	// The client can use this URL to upload content directly to the storage backend.
	// Returns an error for backends that do not support presigned uploads (e.g. local storage).
	GetUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, error)

	// TagObject sets a single tag on an existing object.
	// For backends that do not support tagging (e.g. local storage) this is a no-op.
	TagObject(ctx context.Context, key, tagKey, tagValue string) error

	// RemoveObjectTagging removes all tags from an existing object.
	// For backends that do not support tagging (e.g. local storage) this is a no-op.
	RemoveObjectTagging(ctx context.Context, key string) error
}
