package config

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Cassandra CassandraConfig `mapstructure:"cassandra"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Cache     CacheConfig     `mapstructure:"cache"`
	Log       LogConfig       `mapstructure:"log"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type CassandraConfig struct {
	Hosts          []string      `mapstructure:"hosts"`
	Keyspace       string        `mapstructure:"keyspace"`
	Consistency    string        `mapstructure:"consistency"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	Timeout        time.Duration `mapstructure:"timeout"`
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

type LogConfig struct {
	Level string `mapstructure:"level"`
}

func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8091)
	viper.SetDefault("cassandra.consistency", "LOCAL_ONE")
	viper.SetDefault("cassandra.connect_timeout", "10s")
	viper.SetDefault("cassandra.timeout", "5s")
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("cache.prefix", "chat:history")
	viper.SetDefault("cache.ttl", "30s")
	viper.SetDefault("log.level", "info")

	// Env overrides (for Docker)
	_ = viper.BindEnv("server.port", "PORT")
	_ = viper.BindEnv("cassandra.hosts", "CASSANDRA_HOSTS")
	_ = viper.BindEnv("cassandra.keyspace", "CASSANDRA_KEYSPACE")
	_ = viper.BindEnv("redis.address", "REDIS_ADDRESS")
	_ = viper.BindEnv("redis.password", "REDIS_PASSWORD")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// CASSANDRA_HOSTS: comma-separated, e.g. "cassandra:9042" or "host1:9042,host2:9042"
	if v := os.Getenv("CASSANDRA_HOSTS"); v != "" {
		cfg.Cassandra.Hosts = strings.Split(strings.TrimSpace(v), ",")
		for i, h := range cfg.Cassandra.Hosts {
			cfg.Cassandra.Hosts[i] = strings.TrimSpace(h)
		}
	}

	return &cfg, nil
}
