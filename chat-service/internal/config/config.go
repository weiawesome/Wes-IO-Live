package config

import (
	"time"

	"github.com/spf13/viper"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server    ServerConfig
	GRPC      GRPCConfig
	WebSocket WebSocketConfig
	Auth      AuthConfig
	IDService IDServiceConfig `mapstructure:"id_service"`
	Kafka     KafkaConfig
	Redis     RedisConfig
	Log       LogConfig
}

type IDServiceConfig struct {
	GRPCAddress string `mapstructure:"grpc_address"`
}

type ServerConfig struct {
	Host string
	Port int
}

type GRPCConfig struct {
	Host             string
	Port             int
	AdvertiseAddress string `mapstructure:"advertise_address"`
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

type KafkaConfig struct {
	Brokers    string
	Topic      string
	Partitions int
}

type RedisConfig struct {
	Address            string
	Password           string
	DB                 int
	RegistryPrefix     string        `mapstructure:"registry_prefix"`
	HeartbeatInterval  time.Duration `mapstructure:"heartbeat_interval"`
	KeyTTL             time.Duration `mapstructure:"key_ttl"`
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
	v.SetDefault("server.port", 8088)
	v.SetDefault("grpc.host", "0.0.0.0")
	v.SetDefault("grpc.port", 50052)
	v.SetDefault("grpc.advertise_address", "localhost:50052")
	v.SetDefault("websocket.ping_interval", "30s")
	v.SetDefault("websocket.pong_wait", "60s")
	v.SetDefault("websocket.write_wait", "10s")
	v.SetDefault("websocket.max_message_size", 4096)
	v.SetDefault("auth.grpc_address", "localhost:50051")
	v.SetDefault("id_service.grpc_address", "localhost:50053")
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "chat-messages")
	v.SetDefault("kafka.partitions", 8)
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.registry_prefix", "chat:registry")
	v.SetDefault("redis.heartbeat_interval", "10s")
	v.SetDefault("redis.key_ttl", "30s")
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("grpc.port", "GRPC_PORT")
	v.BindEnv("grpc.advertise_address", "GRPC_ADVERTISE_ADDRESS")
	v.BindEnv("auth.grpc_address", "AUTH_GRPC_ADDRESS")
	v.BindEnv("id_service.grpc_address", "ID_SERVICE_GRPC_ADDRESS")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_TOPIC")
	v.BindEnv("kafka.partitions", "KAFKA_PARTITIONS")
	v.BindEnv("redis.address", "REDIS_ADDRESS")
	v.BindEnv("redis.password", "REDIS_PASSWORD")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse durations
	cfg.WebSocket.PingInterval = parseDuration(v, "websocket.ping_interval", 30*time.Second)
	cfg.WebSocket.PongWait = parseDuration(v, "websocket.pong_wait", 60*time.Second)
	cfg.WebSocket.WriteWait = parseDuration(v, "websocket.write_wait", 10*time.Second)
	cfg.Redis.HeartbeatInterval = parseDuration(v, "redis.heartbeat_interval", 10*time.Second)
	cfg.Redis.KeyTTL = parseDuration(v, "redis.key_ttl", 30*time.Second)

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
