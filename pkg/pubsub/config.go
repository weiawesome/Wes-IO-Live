package pubsub

import "time"

// KafkaConfig holds Kafka-specific configuration.
type KafkaConfig struct {
	Brokers    string `mapstructure:"brokers"`
	GroupID    string `mapstructure:"group_id"`
	Partitions int    `mapstructure:"partitions"`
}

// Config holds the configuration for the pub/sub system.
type Config struct {
	Driver string      `mapstructure:"driver"` // "redis", "kafka"
	Redis  RedisConfig `mapstructure:"redis"`
	Kafka  KafkaConfig `mapstructure:"kafka"`
}

// RedisConfig holds Redis-specific configuration.
type RedisConfig struct {
	Address      string        `mapstructure:"address"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Driver: "redis",
		Redis: RedisConfig{
			Address:      "localhost:6379",
			Password:     "",
			DB:           0,
			PoolSize:     10,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		},
	}
}

// NewPubSub creates a new PubSub instance based on the configuration.
func NewPubSub(cfg Config) (PubSub, error) {
	switch cfg.Driver {
	case "kafka":
		return NewKafkaPubSub(cfg.Kafka)
	case "redis":
		return NewRedisPubSub(cfg.Redis)
	default:
		return NewRedisPubSub(cfg.Redis)
	}
}
