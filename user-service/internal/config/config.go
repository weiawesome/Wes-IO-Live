package config

import (
	"time"

	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
	"github.com/weiawesome/wes-io-live/pkg/storage"
)

// StorageConfig mirrors the nested structure used by playback-service / media-service.
type StorageConfig struct {
	Type            string              `mapstructure:"type"`
	S3              storage.S3Config    `mapstructure:"s3"`
	Local           storage.LocalConfig `mapstructure:"local"`
	ProcessedBucket string              `mapstructure:"processed_bucket"`
}

type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	AuthService AuthServiceConfig `mapstructure:"auth_service"`
	Redis       RedisConfig
	Cache       CacheConfig
	Log         LogConfig
	Storage     StorageConfig   `mapstructure:"storage"`
	Kafka       KafkaConfig     `mapstructure:"kafka"`
	Lifecycle   LifecycleConfig `mapstructure:"lifecycle"`
}

type KafkaConfig struct {
	Brokers string `mapstructure:"brokers"`
	Topic   string `mapstructure:"topic"`
	GroupID string `mapstructure:"group_id"`
}

type LifecycleConfig struct {
	TagKey   string `mapstructure:"tag_key"`
	TagValue string `mapstructure:"tag_value"`
}

type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type CacheConfig struct {
	Prefix string        `mapstructure:"prefix"`
	TTL    time.Duration `mapstructure:"ttl"`
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Driver          string `mapstructure:"driver"`
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	FilePath        string `mapstructure:"file_path"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

type AuthServiceConfig struct {
	GRPCAddress string `mapstructure:"grpc_address"`
}

type LogConfig struct {
	Level string
}

func Load() (*Config, error) {
	v, err := pkgconfig.Load("./config", "config")
	if err != nil {
		return nil, err
	}

	// Set defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8082)
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.dbname", "user_service")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.file_path", "./data/user.db")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 60)
	v.SetDefault("auth_service.grpc_address", "localhost:50051")
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("cache.prefix", "user")
	v.SetDefault("cache.ttl", "30s")
	v.SetDefault("log.level", "info")
	v.SetDefault("storage.type", "s3")
	v.SetDefault("storage.s3.region", "us-east-1")
	v.SetDefault("storage.s3.use_path_style", true)
	v.SetDefault("storage.local.base_path", "./data/storage")
	v.SetDefault("storage.processed_bucket", "users-processed")
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "avatar-processed")
	v.SetDefault("kafka.group_id", "user-service")
	v.SetDefault("lifecycle.tag_key", "lifecycle")
	v.SetDefault("lifecycle.tag_value", "delete-pending")

	// Bind environment variables
	v.BindEnv("server.port", "PORT")
	v.BindEnv("database.driver", "DB_DRIVER")
	v.BindEnv("database.host", "DB_HOST")
	v.BindEnv("database.port", "DB_PORT")
	v.BindEnv("database.user", "DB_USER")
	v.BindEnv("database.password", "DB_PASSWORD")
	v.BindEnv("database.dbname", "DB_NAME")
	v.BindEnv("database.sslmode", "DB_SSLMODE")
	v.BindEnv("database.file_path", "DB_FILE_PATH")
	v.BindEnv("database.max_idle_conns", "DB_MAX_IDLE_CONNS")
	v.BindEnv("database.max_open_conns", "DB_MAX_OPEN_CONNS")
	v.BindEnv("database.conn_max_lifetime", "DB_CONN_MAX_LIFETIME")
	v.BindEnv("auth_service.grpc_address", "AUTH_SERVICE_GRPC")
	v.BindEnv("redis.address", "REDIS_ADDRESS")
	v.BindEnv("redis.password", "REDIS_PASSWORD")
	v.BindEnv("storage.type", "STORAGE_TYPE")
	v.BindEnv("storage.s3.endpoint", "S3_ENDPOINT")
	v.BindEnv("storage.s3.region", "S3_REGION")
	v.BindEnv("storage.s3.bucket", "S3_BUCKET")
	v.BindEnv("storage.s3.access_key_id", "S3_ACCESS_KEY_ID")
	v.BindEnv("storage.s3.secret_access_key", "S3_SECRET_ACCESS_KEY")
	v.BindEnv("storage.s3.public_url", "S3_PUBLIC_URL")
	v.BindEnv("storage.processed_bucket", "S3_PROCESSED_BUCKET")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_TOPIC")
	v.BindEnv("kafka.group_id", "KAFKA_GROUP_ID")
	v.BindEnv("lifecycle.tag_key", "LIFECYCLE_TAG_KEY")
	v.BindEnv("lifecycle.tag_value", "LIFECYCLE_TAG_VALUE")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
