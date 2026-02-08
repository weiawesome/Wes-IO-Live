package config

import (
	"time"

	"github.com/spf13/viper"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server ServerConfig
	Kafka  KafkaConfig
	Redis  RedisConfig
	GRPC   GRPCConfig
	Log    LogConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type KafkaConfig struct {
	Brokers             string
	Topic               string
	GroupID             string `mapstructure:"group_id"`
	AutoOffsetReset     string `mapstructure:"auto_offset_reset"`
	MaxPollIntervalMs   int    `mapstructure:"max_poll_interval_ms"`
	SessionTimeoutMs    int    `mapstructure:"session_timeout_ms"`
	HeartbeatIntervalMs int    `mapstructure:"heartbeat_interval_ms"`
	FetchMinBytes       int    `mapstructure:"fetch_min_bytes"`
	FetchMaxWaitMs      int    `mapstructure:"fetch_max_wait_ms"`
}

type RedisConfig struct {
	Address        string
	Password       string
	DB             int
	RegistryPrefix string        `mapstructure:"registry_prefix"`
	LookupTimeout  time.Duration `mapstructure:"lookup_timeout"`
}

type GRPCConfig struct {
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
	CallTimeout time.Duration `mapstructure:"call_timeout"`
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
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
	v.SetDefault("server.port", 8089)
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "chat-messages")
	v.SetDefault("kafka.group_id", "chat-dispatcher")
	v.SetDefault("kafka.auto_offset_reset", "latest")
	v.SetDefault("kafka.max_poll_interval_ms", 300000)
	v.SetDefault("kafka.session_timeout_ms", 45000)
	v.SetDefault("kafka.heartbeat_interval_ms", 3000)
	v.SetDefault("kafka.fetch_min_bytes", 1)
	v.SetDefault("kafka.fetch_max_wait_ms", 500)
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.registry_prefix", "chat:registry")
	v.SetDefault("redis.lookup_timeout", "2s")
	v.SetDefault("grpc.dial_timeout", "5s")
	v.SetDefault("grpc.call_timeout", "3s")
	v.SetDefault("grpc.idle_timeout", "5m")
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_TOPIC")
	v.BindEnv("kafka.group_id", "KAFKA_GROUP_ID")
	v.BindEnv("redis.address", "REDIS_ADDRESS")
	v.BindEnv("redis.password", "REDIS_PASSWORD")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse durations
	cfg.Redis.LookupTimeout = parseDuration(v, "redis.lookup_timeout", 2*time.Second)
	cfg.GRPC.DialTimeout = parseDuration(v, "grpc.dial_timeout", 5*time.Second)
	cfg.GRPC.CallTimeout = parseDuration(v, "grpc.call_timeout", 3*time.Second)
	cfg.GRPC.IdleTimeout = parseDuration(v, "grpc.idle_timeout", 5*time.Minute)

	return &cfg, nil
}

func parseDuration(v *viper.Viper, key string, defaultVal time.Duration) time.Duration {
	str := v.GetString(key)
	d, err := time.ParseDuration(str)
	if err != nil {
		return defaultVal
	}
	return d
}
