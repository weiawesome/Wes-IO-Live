package config

import (
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

// Config holds all configuration for the playback service.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Storage  StorageConfig  `mapstructure:"storage"`
	Playback PlaybackConfig `mapstructure:"playback"`
	Session  SessionConfig  `mapstructure:"session"`
	Log      LogConfig      `mapstructure:"log"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// StorageConfig holds storage backend configuration.
type StorageConfig struct {
	Type  string       `mapstructure:"type"` // "local" or "s3"
	Local LocalConfig  `mapstructure:"local"`
	S3    S3Config     `mapstructure:"s3"`
}

// LocalConfig holds local filesystem storage configuration.
type LocalConfig struct {
	BasePath string `mapstructure:"base_path"`
}

// S3Config holds S3/MinIO storage configuration.
type S3Config struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UsePathStyle    bool   `mapstructure:"use_path_style"`
	PublicURL       string `mapstructure:"public_url"`
}

// PlaybackConfig holds playback-specific configuration.
type PlaybackConfig struct {
	AccessMode    string `mapstructure:"access_mode"`    // "redirect" or "proxy"
	PresignExpiry int    `mapstructure:"presign_expiry"` // seconds, for redirect mode
	LivePrefix    string `mapstructure:"live_prefix"`    // S3 prefix for live HLS
	VODPrefix     string `mapstructure:"vod_prefix"`     // S3 prefix for VOD
	StoragePrefix string `mapstructure:"storage_prefix"` // Global S3 prefix (empty for root)
}

// SessionConfig holds session store configuration.
type SessionConfig struct {
	Type  string             `mapstructure:"type"` // "none", "memory", or "redis"
	Redis SessionRedisConfig `mapstructure:"redis"`
}

// SessionRedisConfig holds Redis session store configuration.
type SessionRedisConfig struct {
	Address   string `mapstructure:"address"`
	Password  string `mapstructure:"password"`
	DB        int    `mapstructure:"db"`
	KeyPrefix string `mapstructure:"key_prefix"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Load reads configuration from file and environment variables.
func Load() (*Config, error) {
	v, err := pkgconfig.Load("./config", "config")
	if err != nil {
		return nil, err
	}

	// Set defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8087)
	v.SetDefault("storage.type", "local")
	v.SetDefault("storage.local.base_path", "./hls")
	v.SetDefault("storage.s3.region", "us-east-1")
	v.SetDefault("storage.s3.use_path_style", true)
	v.SetDefault("playback.access_mode", "proxy")
	v.SetDefault("playback.presign_expiry", 3600)
	v.SetDefault("playback.live_prefix", "live")
	v.SetDefault("playback.vod_prefix", "vod")
	v.SetDefault("playback.storage_prefix", "")
	v.SetDefault("session.type", "none")
	v.SetDefault("session.redis.db", 1)
	v.SetDefault("session.redis.key_prefix", "vod:session:")
	v.SetDefault("log.level", "info")

	// Bind environment variables
	v.BindEnv("server.port", "PORT")
	v.BindEnv("storage.type", "STORAGE_TYPE")
	v.BindEnv("storage.local.base_path", "STORAGE_LOCAL_BASE_PATH")
	v.BindEnv("storage.s3.endpoint", "S3_ENDPOINT")
	v.BindEnv("storage.s3.region", "S3_REGION")
	v.BindEnv("storage.s3.bucket", "S3_BUCKET")
	v.BindEnv("storage.s3.access_key_id", "S3_ACCESS_KEY_ID")
	v.BindEnv("storage.s3.secret_access_key", "S3_SECRET_ACCESS_KEY")
	v.BindEnv("storage.s3.public_url", "S3_PUBLIC_URL")
	v.BindEnv("playback.access_mode", "PLAYBACK_ACCESS_MODE")
	v.BindEnv("session.type", "SESSION_TYPE")
	v.BindEnv("session.redis.address", "REDIS_ADDRESS")
	v.BindEnv("session.redis.password", "REDIS_PASSWORD")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// IsRedirectMode returns true if using redirect mode with S3 storage.
func (c *Config) IsRedirectMode() bool {
	return c.Playback.AccessMode == "redirect" && c.Storage.Type == "s3"
}
