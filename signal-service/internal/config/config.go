package config

import (
	"time"

	"github.com/spf13/viper"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
	"github.com/weiawesome/wes-io-live/pkg/pubsub"
)

type Config struct {
	Server    ServerConfig
	WebSocket WebSocketConfig
	Auth      AuthConfig
	Room      RoomConfig
	PubSub    pubsub.Config
	Kafka     KafkaConfig
	Log       LogConfig
}

type KafkaConfig struct {
	Brokers    string
	Topic      string
	Partitions int
}

type ServerConfig struct {
	Host string
	Port int
}

type WebSocketConfig struct {
	PingInterval   time.Duration `mapstructure:"ping_interval"`
	PongWait       time.Duration `mapstructure:"pong_wait"`
	WriteWait      time.Duration `mapstructure:"write_wait"`
	MaxMessageSize int64         `mapstructure:"max_message_size"`
}

type AuthConfig struct {
	GRPCAddress string `mapstructure:"grpc_address"`
}

type RoomConfig struct {
	HTTPAddress string        `mapstructure:"http_address"`
	CacheTTL    time.Duration `mapstructure:"cache_ttl"`
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
	v.SetDefault("server.port", 8084)
	v.SetDefault("websocket.ping_interval", "30s")
	v.SetDefault("websocket.pong_wait", "60s")
	v.SetDefault("websocket.write_wait", "10s")
	v.SetDefault("websocket.max_message_size", 65536)
	v.SetDefault("auth.grpc_address", "localhost:50051")
	v.SetDefault("room.http_address", "http://localhost:8083")
	v.SetDefault("room.cache_ttl", "5m")
	v.SetDefault("pubsub.driver", "kafka")
	v.SetDefault("pubsub.redis.address", "localhost:6379")
	v.SetDefault("pubsub.redis.password", "")
	v.SetDefault("pubsub.redis.db", 0)
	v.SetDefault("pubsub.kafka.brokers", "localhost:9092")
	v.SetDefault("pubsub.kafka.group_id", "signal-service")
	v.SetDefault("pubsub.kafka.partitions", 4)
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "broadcast-events")
	v.SetDefault("kafka.partitions", 4)
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("auth.grpc_address", "AUTH_GRPC_ADDRESS")
	v.BindEnv("room.http_address", "ROOM_HTTP_ADDRESS")
	v.BindEnv("pubsub.redis.address", "REDIS_ADDRESS")
	v.BindEnv("pubsub.redis.password", "REDIS_PASSWORD")
	v.BindEnv("pubsub.kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("pubsub.kafka.group_id", "KAFKA_PUBSUB_GROUP_ID")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_BROADCAST_TOPIC")
	v.BindEnv("kafka.partitions", "KAFKA_BROADCAST_PARTITIONS")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse durations
	cfg.WebSocket.PingInterval = parseDuration(v, "websocket.ping_interval", 30*time.Second)
	cfg.WebSocket.PongWait = parseDuration(v, "websocket.pong_wait", 60*time.Second)
	cfg.WebSocket.WriteWait = parseDuration(v, "websocket.write_wait", 10*time.Second)
	cfg.Room.CacheTTL = parseDuration(v, "room.cache_ttl", 5*time.Minute)

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
