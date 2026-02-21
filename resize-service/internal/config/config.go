package config

import (
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
	"github.com/weiawesome/wes-io-live/pkg/storage"
)

// StorageConfig mirrors the nested structure used by other services.
type StorageConfig struct {
	Type  string              `mapstructure:"type"`
	S3    storage.S3Config    `mapstructure:"s3"`
	Local storage.LocalConfig `mapstructure:"local"`
}

type Config struct {
	Log       LogConfig       `mapstructure:"log"`
	Kafka     KafkaConfig     `mapstructure:"kafka"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Processor ProcessorConfig `mapstructure:"processor"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

type KafkaConfig struct {
	Brokers         string `mapstructure:"brokers"`
	ConsumerTopic   string `mapstructure:"consumer_topic"`
	ConsumerGroupID string `mapstructure:"consumer_group_id"`
	ProducerTopic   string `mapstructure:"producer_topic"`
}

type ProcessorConfig struct {
	BucketFilter     string          `mapstructure:"bucket_filter"`
	PrefixFilter     string          `mapstructure:"prefix_filter"`
	OutputPrefix     string          `mapstructure:"output_prefix"`
	EventNameFilters []string        `mapstructure:"event_name_filters"`
	Sizes            []SizeConfig    `mapstructure:"sizes"`
	JpegQuality      int             `mapstructure:"jpeg_quality"`
	Lifecycle        LifecycleConfig `mapstructure:"lifecycle"`
}

type SizeConfig struct {
	Name   string `mapstructure:"name"`
	Width  int    `mapstructure:"width"`
	Height int    `mapstructure:"height"`
}

type LifecycleConfig struct {
	TagKey   string `mapstructure:"tag_key"`
	TagValue string `mapstructure:"tag_value"`
}

func Load() (*Config, error) {
	v, err := pkgconfig.Load("./config", "config")
	if err != nil {
		return nil, err
	}

	// Defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.consumer_topic", "minio-events")
	v.SetDefault("kafka.consumer_group_id", "resize-service")
	v.SetDefault("kafka.producer_topic", "avatar-processed")
	v.SetDefault("storage.type", "s3")
	v.SetDefault("storage.s3.region", "us-east-1")
	v.SetDefault("storage.s3.use_path_style", true)
	v.SetDefault("storage.local.base_path", "./data/storage")
	v.SetDefault("processor.bucket_filter", "users")
	v.SetDefault("processor.prefix_filter", "avatars/raw/")
	v.SetDefault("processor.output_prefix", "avatars/processed/")
	v.SetDefault("processor.event_name_filters", []string{"s3:ObjectCreated:Put"})
	v.SetDefault("processor.jpeg_quality", 85)
	v.SetDefault("processor.lifecycle.tag_key", "lifecycle")
	v.SetDefault("processor.lifecycle.tag_value", "delete-pending")

	// Env bindings
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.consumer_topic", "KAFKA_CONSUMER_TOPIC")
	v.BindEnv("kafka.consumer_group_id", "KAFKA_CONSUMER_GROUP_ID")
	v.BindEnv("kafka.producer_topic", "KAFKA_PRODUCER_TOPIC")
	v.BindEnv("storage.type", "STORAGE_TYPE")
	v.BindEnv("storage.s3.endpoint", "S3_ENDPOINT")
	v.BindEnv("storage.s3.region", "S3_REGION")
	v.BindEnv("storage.s3.bucket", "S3_BUCKET")
	v.BindEnv("storage.s3.access_key_id", "S3_ACCESS_KEY_ID")
	v.BindEnv("storage.s3.secret_access_key", "S3_SECRET_ACCESS_KEY")
	v.BindEnv("storage.s3.public_url", "S3_PUBLIC_URL")
	v.BindEnv("processor.bucket_filter", "PROCESSOR_BUCKET_FILTER")
	v.BindEnv("processor.event_name_filters", "PROCESSOR_EVENT_NAME_FILTERS")
	v.BindEnv("processor.lifecycle.tag_key", "LIFECYCLE_TAG_KEY")
	v.BindEnv("processor.lifecycle.tag_value", "LIFECYCLE_TAG_VALUE")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
