package config

import (
	"time"

	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server      ServerConfig
	Database    DatabaseConfig
	AuthService AuthServiceConfig `mapstructure:"auth_service"`
	Redis       RedisConfig
	Cache       CacheConfig
	Log         LogConfig
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

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
