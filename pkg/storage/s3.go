package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Storage implements Storage interface for S3/MinIO.
type S3Storage struct {
	client              *s3.Client
	presignClient       *s3.PresignClient // for GetURL (server-side reads)
	uploadPresignClient *s3.PresignClient // for GetUploadURL; uses public endpoint when publicURL is set
	bucket              string
	publicURL           string
}

// S3Config holds configuration for S3 storage.
type S3Config struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UsePathStyle    bool   `mapstructure:"use_path_style"` // Required for MinIO
	PublicURL       string `mapstructure:"public_url"`     // Optional public URL prefix
}

// NewS3Storage creates a new S3Storage instance.
func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	// Set defaults
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	// Build AWS config options
	var opts []func(*config.LoadOptions) error

	opts = append(opts, config.WithRegion(cfg.Region))

	// Use static credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Build S3 client options
	var s3Opts []func(*s3.Options)

	// Custom endpoint for MinIO
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	// Path-style addressing for MinIO
	if cfg.UsePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	presignClient := s3.NewPresignClient(client)

	// If publicURL is set, build a separate presign client using the public endpoint
	// so that GetUploadURL returns URLs the browser can directly access.
	var uploadPresignClient *s3.PresignClient
	if cfg.PublicURL != "" {
		pubClient := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.PublicURL)
			o.UsePathStyle = cfg.UsePathStyle
		})
		uploadPresignClient = s3.NewPresignClient(pubClient)
	} else {
		uploadPresignClient = presignClient
	}

	return &S3Storage{
		client:              client,
		presignClient:       presignClient,
		uploadPresignClient: uploadPresignClient,
		bucket:              cfg.Bucket,
		publicURL:           cfg.PublicURL,
	}, nil
}

// Write stores content from the reader with the given key.
func (s *S3Storage) Write(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}

	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}

	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// Read retrieves content for the given key.
func (s *S3Storage) Read(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}

	return output.Body, nil
}

// Delete removes the content with the given key.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object from S3: %w", err)
	}

	return nil
}

// DeletePrefix removes all content with keys starting with the given prefix.
func (s *S3Storage) DeletePrefix(ctx context.Context, prefix string) error {
	// List all objects with the prefix
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		if len(page.Contents) == 0 {
			continue
		}

		// Build delete request
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{
				Key: obj.Key,
			})
		}

		// Delete objects in batch
		_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects: %w", err)
		}
	}

	return nil
}

// List returns information about all files with keys starting with the given prefix.
func (s *S3Storage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	var files []FileInfo

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			files = append(files, FileInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}

	return files, nil
}

// Exists checks if content with the given key exists.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// GetURL returns a presigned URL for accessing the content.
func (s *S3Storage) GetURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	// If public URL is configured, return direct URL
	if s.publicURL != "" {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(s.publicURL, "/"), key), nil
	}

	// Generate presigned URL
	presignedReq, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedReq.URL, nil
}

// GetUploadURL returns a presigned PUT URL for direct client upload.
func (s *S3Storage) GetUploadURL(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	presignedReq, err := s.uploadPresignClient.PresignPutObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return presignedReq.URL, nil
}

// GetBucket returns the bucket name.
func (s *S3Storage) GetBucket() string {
	return s.bucket
}

// TagObject sets a single tag on an existing S3/MinIO object.
func (s *S3Storage) TagObject(ctx context.Context, key, tagKey, tagValue string) error {
	_, err := s.client.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Tagging: &types.Tagging{
			TagSet: []types.Tag{{Key: aws.String(tagKey), Value: aws.String(tagValue)}},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tag object %s: %w", key, err)
	}
	return nil
}

// RemoveObjectTagging removes all tags from an existing S3/MinIO object.
func (s *S3Storage) RemoveObjectTagging(ctx context.Context, key string) error {
	_, err := s.client.DeleteObjectTagging(ctx, &s3.DeleteObjectTaggingInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to remove tags from object %s: %w", key, err)
	}
	return nil
}
