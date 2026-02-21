package config

import (
	"time"

	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	Kafka      KafkaConfig
	Reconciler ReconcilerConfig
	Auth       AuthServiceConfig `mapstructure:"auth_service"`
	Log        LogConfig
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

type RedisConfig struct {
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type KafkaConfig struct {
	Brokers string `mapstructure:"brokers"`
	Topic   string `mapstructure:"topic"`
	GroupID string `mapstructure:"group_id"`
}

type ReconcilerConfig struct {
	Interval time.Duration `mapstructure:"interval"`
	TopN     int           `mapstructure:"top_n"`
}

type AuthServiceConfig struct {
	GRPCAddress string `mapstructure:"grpc_address"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func Load() (*Config, error) {
	v, err := pkgconfig.Load("./config", "config")
	if err != nil {
		return nil, err
	}

	// Set defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8095)
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.dbname", "postgres")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.file_path", "./data/social-graph.db")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 60)
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("auth_service.grpc_address", "localhost:50051")
	v.SetDefault("kafka.brokers", "localhost:9092")
	v.SetDefault("kafka.topic", "dbserver1.public.follows")
	v.SetDefault("kafka.group_id", "social-graph-service")
	v.SetDefault("reconciler.interval", "60s")
	v.SetDefault("reconciler.top_n", 100)
	v.SetDefault("log.level", "info")

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
	v.BindEnv("redis.address", "REDIS_ADDRESS")
	v.BindEnv("redis.password", "REDIS_PASSWORD")
	v.BindEnv("redis.db", "REDIS_DB")
	v.BindEnv("auth_service.grpc_address", "AUTH_SERVICE_GRPC")
	v.BindEnv("kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("kafka.topic", "KAFKA_TOPIC")
	v.BindEnv("kafka.group_id", "KAFKA_GROUP_ID")
	v.BindEnv("reconciler.interval", "RECONCILER_INTERVAL")
	v.BindEnv("reconciler.top_n", "RECONCILER_TOP_N")
	v.BindEnv("log.level", "LOG_LEVEL")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
