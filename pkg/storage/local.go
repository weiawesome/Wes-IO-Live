package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage implements Storage interface for local filesystem.
type LocalStorage struct {
	basePath string
}

// LocalConfig holds configuration for local storage.
type LocalConfig struct {
	BasePath string `mapstructure:"base_path"`
}

// NewLocalStorage creates a new LocalStorage instance.
func NewLocalStorage(cfg LocalConfig) (*LocalStorage, error) {
	// Ensure base path exists
	if err := os.MkdirAll(cfg.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}

	absPath, err := filepath.Abs(cfg.BasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return &LocalStorage{
		basePath: absPath,
	}, nil
}

// fullPath returns the full filesystem path for a key.
func (s *LocalStorage) fullPath(key string) string {
	// Clean the key to prevent directory traversal
	cleanKey := filepath.Clean(key)
	// Prevent directory traversal: reject keys that would escape basePath
	if cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(os.PathSeparator)) {
		cleanKey = ""
	}
	return filepath.Join(s.basePath, cleanKey)
}

// Write stores content from the reader with the given key.
func (s *LocalStorage) Write(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	path := s.fullPath(key)

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temporary file in the same directory
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Copy content to temp file
	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write content: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// Read retrieves content for the given key.
func (s *LocalStorage) Read(ctx context.Context, key string) (io.ReadCloser, error) {
	path := s.fullPath(key)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", key)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// Delete removes the content with the given key.
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path := s.fullPath(key)

	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// DeletePrefix removes all content with keys starting with the given prefix.
func (s *LocalStorage) DeletePrefix(ctx context.Context, prefix string) error {
	path := s.fullPath(prefix)

	// Check if it's a directory
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to delete
		}
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return os.RemoveAll(path)
	}

	// If prefix is a file pattern, walk and delete matching files
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	return filepath.Walk(dir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), base) {
			if removeErr := os.Remove(filePath); removeErr != nil {
				return fmt.Errorf("failed to remove %s: %w", filePath, removeErr)
			}
		}
		return nil
	})
}

// List returns information about all files with keys starting with the given prefix.
func (s *LocalStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	path := s.fullPath(prefix)

	// Check if prefix is a directory
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileInfo{}, nil
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	var files []FileInfo

	if info.IsDir() {
		// List all files in directory
		err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, _ := filepath.Rel(s.basePath, filePath)
				files = append(files, FileInfo{
					Key:          relPath,
					Size:         info.Size(),
					LastModified: info.ModTime(),
				})
			}
			return nil
		})
	} else {
		// Single file
		relPath, _ := filepath.Rel(s.basePath, path)
		files = append(files, FileInfo{
			Key:          relPath,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return files, nil
}

// Exists checks if content with the given key exists.
func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	path := s.fullPath(key)

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file: %w", err)
	}

	return true, nil
}

// GetURL returns the file path for local storage.
// For local storage, this simply returns the file:// URL or relative path.
func (s *LocalStorage) GetURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	path := s.fullPath(key)

	// Check if file exists
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", key)
		}
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	// Return relative path from base for HTTP serving
	return "/" + key, nil
}

// GetUploadURL is not supported for local storage.
func (s *LocalStorage) GetUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	return "", fmt.Errorf("presigned upload not supported for local storage")
}

// TagObject is a no-op for local storage (no tagging concept).
func (s *LocalStorage) TagObject(_ context.Context, _, _, _ string) error { return nil }

// RemoveObjectTagging is a no-op for local storage (no tagging concept).
func (s *LocalStorage) RemoveObjectTagging(_ context.Context, _ string) error { return nil }

// GetBasePath returns the base path for the storage.
func (s *LocalStorage) GetBasePath() string {
	return s.basePath
}
