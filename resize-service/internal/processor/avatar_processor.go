package processor

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	"github.com/weiawesome/wes-io-live/pkg/storage"
	"github.com/weiawesome/wes-io-live/resize-service/internal/config"
	"github.com/weiawesome/wes-io-live/resize-service/internal/mq"
)

// sizeSpec holds the target dimensions for a single avatar variant.
type sizeSpec struct {
	name   string
	width  int
	height int
}

// AvatarImageProcessor implements AvatarProcessor and mq.MinIOEventHandler.
type AvatarImageProcessor struct {
	srcStorage        storage.Storage // reads raw image + tags (source bucket)
	dstStorage        storage.Storage // writes processed images (dest bucket)
	publisher         mq.AvatarEventPublisher
	srcBucket         string // raw uploads bucket name (for event metadata)
	dstBucket         string // processed images bucket name (for event metadata)
	outputPrefix      string
	sizes             []sizeSpec
	jpegQuality       int
	lifecycleTagKey   string
	lifecycleTagValue string
}

// NewAvatarImageProcessor constructs an AvatarImageProcessor from service config.
func NewAvatarImageProcessor(
	srcStorage storage.Storage,
	dstStorage storage.Storage,
	publisher mq.AvatarEventPublisher,
	cfg *config.Config,
) *AvatarImageProcessor {
	sizes := make([]sizeSpec, 0, len(cfg.Processor.Sizes))
	for _, s := range cfg.Processor.Sizes {
		sizes = append(sizes, sizeSpec{name: s.Name, width: s.Width, height: s.Height})
	}

	return &AvatarImageProcessor{
		srcStorage:        srcStorage,
		dstStorage:        dstStorage,
		publisher:         publisher,
		srcBucket:         cfg.Processor.BucketFilter,
		dstBucket:         cfg.Storage.S3.Bucket,
		outputPrefix:      cfg.Processor.OutputPrefix,
		sizes:             sizes,
		jpegQuality:       cfg.Processor.JpegQuality,
		lifecycleTagKey:   cfg.Processor.Lifecycle.TagKey,
		lifecycleTagValue: cfg.Processor.Lifecycle.TagValue,
	}
}

// HandleUploadEvent parses the userID from the raw key and calls Process.
// Key format: "avatars/raw/{userID}/{uuid}.ext" → parts[2] = userID.
func (p *AvatarImageProcessor) HandleUploadEvent(ctx context.Context, event *mq.MinIOUploadEvent) error {
	parts := strings.Split(event.Key, "/")
	if len(parts) < 3 {
		return fmt.Errorf("unexpected key format: %s", event.Key)
	}
	userID := parts[2]
	return p.Process(ctx, event.Key, userID)
}

// Process reads the raw image, resizes it to each configured size, uploads the results,
// deletes the raw file, and publishes an AvatarProcessedEvent.
func (p *AvatarImageProcessor) Process(ctx context.Context, rawKey, userID string) error {
	l := pkglog.L()

	// 1. Read raw image from source bucket.
	rc, err := p.srcStorage.Read(ctx, rawKey)
	if err != nil {
		return fmt.Errorf("read raw: %w", err)
	}
	defer rc.Close()

	// 2. Decode the image (format auto-detected).
	img, err := imaging.Decode(rc)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	// 3. Parse uploadUUID from rawKey: "avatars/raw/{userID}/{uuid}.ext" → "{uuid}"
	base := filepath.Base(rawKey)
	uploadID := strings.TrimSuffix(base, filepath.Ext(base))

	// 4. Resize to each size, encode as JPEG, and upload to destination bucket.
	refs := make(map[string]mq.AvatarObjectRef, len(p.sizes))
	for _, sz := range p.sizes {
		// Square crop centred on the image.
		resized := imaging.Fill(img, sz.width, sz.height, imaging.Center, imaging.Lanczos)

		var buf bytes.Buffer
		if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(p.jpegQuality)); err != nil {
			return fmt.Errorf("encode %s: %w", sz.name, err)
		}

		// Unique output key — includes uploadUUID so old processed images remain distinguishable.
		outKey := fmt.Sprintf("%s%s/%s/%s.jpg", p.outputPrefix, userID, uploadID, sz.name)

		if err := p.dstStorage.Write(ctx, outKey, bytes.NewReader(buf.Bytes()), int64(buf.Len()), "image/jpeg"); err != nil {
			return fmt.Errorf("upload %s: %w", sz.name, err)
		}

		refs[sz.name] = mq.AvatarObjectRef{Bucket: p.dstBucket, Key: outKey}
		l.Info().Str("size", sz.name).Str("key", outKey).Msg("uploaded resized avatar")
	}

	// 5. Tag raw file for lifecycle deletion in source bucket (best-effort).
	if err := p.srcStorage.TagObject(ctx, rawKey, p.lifecycleTagKey, p.lifecycleTagValue); err != nil {
		l.Warn().Err(err).Str("key", rawKey).Msg("failed to tag raw for deletion")
	} else {
		l.Info().Str("key", rawKey).Msg("tagged raw for deletion")
	}

	// 6. Publish avatar-processed event with exact bucket+key for each variant.
	event := &mq.AvatarProcessedEvent{
		UserID: userID,
		Raw:    mq.AvatarObjectRef{Bucket: p.srcBucket, Key: rawKey},
		Processed: mq.AvatarProcessedObjects{
			Sm: refs["sm"],
			Md: refs["md"],
			Lg: refs["lg"],
		},
		Timestamp: time.Now().Unix(),
	}

	if err := p.publisher.PublishAvatarProcessed(ctx, event); err != nil {
		return fmt.Errorf("publish event: %w", err)
	}

	l.Info().Str("user_id", userID).Msg("avatar processing complete")
	return nil
}
