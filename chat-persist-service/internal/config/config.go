package config

import (
	"time"

	"github.com/spf13/viper"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server    ServerConfig
	Kafka     KafkaConfig
	Cassandra CassandraConfig
	Log       LogConfig
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

type CassandraConfig struct {
	Hosts           []string
	Keyspace        string
	Username        string
	Password        string
	Consistency     string
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`
	Timeout         time.Duration
	NumConns        int `mapstructure:"num_conns"`
	MaxPreparedStmt int `mapstructure:"max_prepared_stmt"`
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
	v.SetDefault("server.port", 8090)
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "chat-messages")
	v.SetDefault("kafka.group_id", "chat-persist")
	v.SetDefault("kafka.auto_offset_reset", "earliest")
	v.SetDefault("kafka.max_poll_interval_ms", 300000)
	v.SetDefault("kafka.session_timeout_ms", 45000)
	v.SetDefault("kafka.heartbeat_interval_ms", 3000)
	v.SetDefault("kafka.fetch_min_bytes", 1)
	v.SetDefault("kafka.fetch_max_wait_ms", 500)
	v.SetDefault("cassandra.hosts", []string{"localhost:9042"})
	v.SetDefault("cassandra.keyspace", "wes_chat")
	v.SetDefault("cassandra.username", "")
	v.SetDefault("cassandra.password", "")
	v.SetDefault("cassandra.consistency", "LOCAL_QUORUM")
	v.SetDefault("cassandra.connect_timeout", "10s")
	v.SetDefault("cassandra.timeout", "5s")
	v.SetDefault("cassandra.num_conns", 2)
	v.SetDefault("cassandra.max_prepared_stmt", 1000)
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_TOPIC")
	v.BindEnv("kafka.group_id", "KAFKA_GROUP_ID")
	v.BindEnv("cassandra.hosts", "CASSANDRA_HOSTS")
	v.BindEnv("cassandra.keyspace", "CASSANDRA_KEYSPACE")
	v.BindEnv("cassandra.username", "CASSANDRA_USERNAME")
	v.BindEnv("cassandra.password", "CASSANDRA_PASSWORD")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse durations
	cfg.Cassandra.ConnectTimeout = parseDuration(v, "cassandra.connect_timeout", 10*time.Second)
	cfg.Cassandra.Timeout = parseDuration(v, "cassandra.timeout", 5*time.Second)

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
