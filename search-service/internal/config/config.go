package config

import (
	"time"

	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server        ServerConfig
	Elasticsearch ElasticsearchConfig
	Redis         RedisConfig
	Cache         CacheConfig
	Log           LogConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type ElasticsearchConfig struct {
	Addresses  []string `mapstructure:"addresses"`
	IndexUsers string   `mapstructure:"index_users"`
	IndexRooms string   `mapstructure:"index_rooms"`
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
	Level string
}

func Load() (*Config, error) {
	v, err := pkgconfig.Load("./config", "config")
	if err != nil {
		return nil, err
	}

	// Set defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8094)
	v.SetDefault("elasticsearch.addresses", []string{"http://localhost:9200"})
	v.SetDefault("elasticsearch.index_users", "cdc-public-users")
	v.SetDefault("elasticsearch.index_rooms", "cdc-public-rooms")
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("cache.prefix", "search")
	v.SetDefault("cache.ttl", "30s")
	v.SetDefault("log.level", "info")

	// Bind environment variables
	v.BindEnv("server.port", "PORT")
	v.BindEnv("elasticsearch.addresses", "ES_ADDRESSES")
	v.BindEnv("elasticsearch.index_users", "ES_INDEX_USERS")
	v.BindEnv("elasticsearch.index_rooms", "ES_INDEX_ROOMS")
	v.BindEnv("redis.address", "REDIS_ADDRESS")
	v.BindEnv("redis.password", "REDIS_PASSWORD")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
