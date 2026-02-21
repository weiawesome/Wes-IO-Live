package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkglog "github.com/weiawesome/wes-io-live/pkg/log"
	pkgstorage "github.com/weiawesome/wes-io-live/pkg/storage"
	"github.com/weiawesome/wes-io-live/resize-service/internal/config"
	"github.com/weiawesome/wes-io-live/resize-service/internal/mq"
	"github.com/weiawesome/wes-io-live/resize-service/internal/processor"
)

func main() {
	// Load configuration.
	cfg, err := config.Load()
	if err != nil {
		l := pkglog.L()
		l.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialise structured logger.
	pkglog.Init(pkglog.Config{
		Level:       cfg.Log.Level,
		Pretty:      cfg.Log.Level == "debug",
		ServiceName: "resize-service",
	})
	l := pkglog.L()
	l.Info().Msg("resize-service starting")

	// Initialise storage backends (src = source/raw bucket, dst = destination/processed bucket).
	var srcStorage, dstStorage pkgstorage.Storage
	switch cfg.Storage.Type {
	case "s3":
		// dstStorage: config bucket (processed image destination)
		dstStorage, err = pkgstorage.NewS3Storage(context.Background(), cfg.Storage.S3)
		if err != nil {
			l.Fatal().Err(err).Msg("failed to init dst s3 storage")
		}
		// srcStorage: source bucket = bucket_filter (raw image source)
		srcS3Cfg := cfg.Storage.S3
		srcS3Cfg.Bucket = cfg.Processor.BucketFilter
		srcStorage, err = pkgstorage.NewS3Storage(context.Background(), srcS3Cfg)
		if err != nil {
			l.Fatal().Err(err).Msg("failed to init src s3 storage")
		}
		l.Info().
			Str("endpoint", cfg.Storage.S3.Endpoint).
			Str("src_bucket", cfg.Processor.BucketFilter).
			Str("dst_bucket", cfg.Storage.S3.Bucket).
			Msg("s3 storage initialised")
	default:
		localSt, lerr := pkgstorage.NewLocalStorage(cfg.Storage.Local)
		if lerr != nil {
			l.Fatal().Err(lerr).Msg("failed to init local storage")
		}
		srcStorage = localSt
		dstStorage = localSt
		l.Info().Str("path", cfg.Storage.Local.BasePath).Msg("local storage initialised")
	}

	// Initialise Kafka publisher.
	publisher, err := mq.NewKafkaPublisher(cfg.Kafka.Brokers, cfg.Kafka.ProducerTopic)
	if err != nil {
		l.Fatal().Err(err).Msg("failed to init kafka publisher")
	}

	// Initialise avatar processor (implements mq.MinIOEventHandler).
	proc := processor.NewAvatarImageProcessor(srcStorage, dstStorage, publisher, cfg)

	// Initialise Kafka consumer.
	consumer, err := mq.NewKafkaConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.ConsumerTopic,
		cfg.Kafka.ConsumerGroupID,
		cfg.Processor.BucketFilter,
		cfg.Processor.PrefixFilter,
		cfg.Processor.EventNameFilters,
		proc,
	)
	if err != nil {
		l.Fatal().Err(err).Msg("failed to init kafka consumer")
	}

	ctx, cancel := context.WithCancel(context.Background())

	if err := consumer.Start(ctx); err != nil {
		l.Fatal().Err(err).Msg("failed to start consumer")
	}

	// Block until SIGINT / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	l.Info().Msg("shutting down: waiting for in-flight processing to complete")
	cancel() // signal consumeLoop to stop accepting new messages

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		consumer.Close()  // waits for in-flight processMessage to finish
		publisher.Close() // then close publisher
	}()

	select {
	case <-shutdownDone:
		l.Info().Msg("shutdown complete")
	case <-time.After(30 * time.Second):
		l.Warn().Msg("shutdown timed out after 30s")
	}
}
