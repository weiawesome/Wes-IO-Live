package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/storage"
)

// UploadTask represents a file upload task.
type UploadTask struct {
	RoomID      string
	LocalPath   string
	S3Key       string
	ContentType string
	OnComplete  func(error)
	Retries     int
}

// S3Uploader manages concurrent uploads to S3.
type S3Uploader struct {
	storage    storage.Storage
	workers    int
	queue      chan *UploadTask
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	maxRetries int
	retryDelay time.Duration
	stopOnce   sync.Once
}

// S3UploaderConfig holds configuration for the S3 uploader.
type S3UploaderConfig struct {
	Workers    int
	QueueSize  int
	MaxRetries int
	RetryDelay time.Duration
}

// NewS3Uploader creates a new S3 uploader.
func NewS3Uploader(s storage.Storage, cfg S3UploaderConfig) *S3Uploader {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &S3Uploader{
		storage:    s,
		workers:    cfg.Workers,
		queue:      make(chan *UploadTask, cfg.QueueSize),
		ctx:        ctx,
		cancel:     cancel,
		maxRetries: cfg.MaxRetries,
		retryDelay: cfg.RetryDelay,
	}
}

// Start launches the upload workers.
func (u *S3Uploader) Start() {
	for i := 0; i < u.workers; i++ {
		u.wg.Add(1)
		go u.worker(i)
	}
	l := pkglog.L()
	l.Info().Int("workers", u.workers).Msg("s3 uploader started")
}

// Stop gracefully stops all workers.
func (u *S3Uploader) Stop() {
	u.stopOnce.Do(func() {
		u.cancel()
		close(u.queue)
		u.wg.Wait()
		l := pkglog.L()
		l.Info().Msg("s3 uploader stopped")
	})
}

// Upload queues a file for upload.
func (u *S3Uploader) Upload(task *UploadTask) error {
	select {
	case u.queue <- task:
		return nil
	case <-u.ctx.Done():
		return fmt.Errorf("uploader is stopped")
	default:
		return fmt.Errorf("upload queue is full")
	}
}

// UploadSync uploads a file synchronously (blocking).
func (u *S3Uploader) UploadSync(ctx context.Context, localPath, s3Key, contentType string) error {
	return u.uploadFile(ctx, localPath, s3Key, contentType)
}

// worker processes upload tasks from the queue.
func (u *S3Uploader) worker(id int) {
	defer u.wg.Done()

	for {
		select {
		case task, ok := <-u.queue:
			if !ok {
				return
			}
			u.processTask(task)
		case <-u.ctx.Done():
			return
		}
	}
}

// processTask handles a single upload task with retries.
func (u *S3Uploader) processTask(task *UploadTask) {
	l := pkglog.L()
	var lastErr error

	for attempt := 0; attempt <= u.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(u.retryDelay * time.Duration(attempt))
		}

		err := u.uploadFile(u.ctx, task.LocalPath, task.S3Key, task.ContentType)
		if err == nil {
			if task.OnComplete != nil {
				task.OnComplete(nil)
			}
			return
		}

		lastErr = err
		l.Warn().Err(err).Int("attempt", attempt+1).Str("s3_key", task.S3Key).Msg("upload attempt failed")
	}

	l.Error().Err(lastErr).Int("attempts", u.maxRetries+1).Str("s3_key", task.S3Key).Msg("upload failed after retries")
	if task.OnComplete != nil {
		task.OnComplete(lastErr)
	}
}

// uploadFile performs the actual file upload.
func (u *S3Uploader) uploadFile(ctx context.Context, localPath, s3Key, contentType string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", localPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", localPath, err)
	}

	err = u.storage.Write(ctx, s3Key, file, info.Size(), contentType)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// UploadReader uploads content from an io.Reader.
func (u *S3Uploader) UploadReader(ctx context.Context, r io.Reader, size int64, s3Key, contentType string) error {
	return u.storage.Write(ctx, s3Key, r, size, contentType)
}

// QueueLength returns the current number of pending tasks.
func (u *S3Uploader) QueueLength() int {
	return len(u.queue)
}
