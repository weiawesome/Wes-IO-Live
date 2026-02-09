package config

import (
	"time"

	"github.com/spf13/viper"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server ServerConfig
	JWT    JWTConfig
	Log    LogConfig
}

type ServerConfig struct {
	Host     string
	GRPCPort int `mapstructure:"grpc_port"`
}

type JWTConfig struct {
	AccessDuration  time.Duration `mapstructure:"access_duration"`
	RefreshDuration time.Duration `mapstructure:"refresh_duration"`
	Issuer          string
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
	v.SetDefault("server.grpc_port", 50051)
	v.SetDefault("jwt.access_duration", "15m")
	v.SetDefault("jwt.refresh_duration", "168h")
	v.SetDefault("jwt.issuer", "wes-io-live")
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.grpc_port", "GRPC_PORT")
	v.BindEnv("jwt.access_duration", "JWT_ACCESS_DURATION")
	v.BindEnv("jwt.refresh_duration", "JWT_REFRESH_DURATION")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Parse durations from strings
	cfg.JWT.AccessDuration = parseDuration(v, "jwt.access_duration", 15*time.Minute)
	cfg.JWT.RefreshDuration = parseDuration(v, "jwt.refresh_duration", 168*time.Hour)

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
