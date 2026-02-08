package config

import (
	"time"

	"github.com/spf13/viper"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server   ServerConfig
	Redis    RedisConfig
	Auth     AuthConfig
	Presence PresenceConfig
	Kafka    KafkaConfig
	Log      LogConfig
}

type KafkaConfig struct {
	Brokers     string
	Topic       string
	GroupID     string `mapstructure:"group_id"`
	GracePeriod time.Duration `mapstructure:"grace_period"`
}

type ServerConfig struct {
	Host       string
	Port       int
	InstanceID string `mapstructure:"instance_id"`
}

type RedisConfig struct {
	Address      string
	Password     string
	DB           int
	PubSubChannel string `mapstructure:"pub_sub_channel"`
}

type AuthConfig struct {
	GRPCAddress string `mapstructure:"grpc_address"`
}

type PresenceConfig struct {
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	HeartbeatTimeout  time.Duration `mapstructure:"heartbeat_timeout"`
	BroadcastInterval time.Duration `mapstructure:"broadcast_interval"`
	PingInterval      time.Duration `mapstructure:"ping_interval"`
	PongWait          time.Duration `mapstructure:"pong_wait"`
	WriteWait         time.Duration `mapstructure:"write_wait"`
	MaxMessageSize    int64         `mapstructure:"max_message_size"`
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
	v.SetDefault("server.port", 8092)
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pub_sub_channel", "presence:room_updates")
	v.SetDefault("auth.grpc_address", "localhost:50051")
	v.SetDefault("presence.heartbeat_interval", "30s")
	v.SetDefault("presence.heartbeat_timeout", "90s")
	v.SetDefault("presence.broadcast_interval", "1s")
	v.SetDefault("presence.ping_interval", "30s")
	v.SetDefault("presence.pong_wait", "60s")
	v.SetDefault("presence.write_wait", "10s")
	v.SetDefault("presence.max_message_size", 4096)
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "broadcast-events")
	v.SetDefault("kafka.group_id", "presence-service")
	v.SetDefault("kafka.grace_period", "60s")
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("redis.address", "REDIS_ADDRESS")
	v.BindEnv("redis.password", "REDIS_PASSWORD")
	v.BindEnv("redis.pub_sub_channel", "REDIS_PUBSUB_CHANNEL")
	v.BindEnv("server.instance_id", "INSTANCE_ID")
	v.BindEnv("auth.grpc_address", "AUTH_GRPC_ADDRESS")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_BROADCAST_TOPIC")
	v.BindEnv("kafka.group_id", "KAFKA_GROUP_ID")
	v.BindEnv("kafka.grace_period", "KAFKA_GRACE_PERIOD")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse durations
	cfg.Presence.HeartbeatInterval = parseDuration(v, "presence.heartbeat_interval", 30*time.Second)
	cfg.Presence.HeartbeatTimeout = parseDuration(v, "presence.heartbeat_timeout", 90*time.Second)
	cfg.Presence.BroadcastInterval = parseDuration(v, "presence.broadcast_interval", 1*time.Second)
	cfg.Presence.PingInterval = parseDuration(v, "presence.ping_interval", 30*time.Second)
	cfg.Presence.PongWait = parseDuration(v, "presence.pong_wait", 60*time.Second)
	cfg.Presence.WriteWait = parseDuration(v, "presence.write_wait", 10*time.Second)
	cfg.Kafka.GracePeriod = parseDuration(v, "kafka.grace_period", 60*time.Second)

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
