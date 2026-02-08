package config

import (
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	GRPC      GRPCConfig
	Snowflake SnowflakeConfig
	NanoID    NanoIDConfig  `mapstructure:"nanoid"`
	CUID2     CUID2Config   `mapstructure:"cuid2"`
	Log       LogConfig
}

type GRPCConfig struct {
	Host string
	Port int
}

type SnowflakeConfig struct {
	MachineID int64 `mapstructure:"machine_id"`
	Epoch     int64
}

type NanoIDConfig struct {
	Size     int    `mapstructure:"size"`
	Alphabet string `mapstructure:"alphabet"`
}

type CUID2Config struct {
	Length int `mapstructure:"length"`
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
	v.SetDefault("grpc.host", "0.0.0.0")
	v.SetDefault("grpc.port", 50053)
	v.SetDefault("snowflake.machine_id", 1)
	v.SetDefault("snowflake.epoch", 1704067200000)
	v.SetDefault("nanoid.size", 21)
	v.SetDefault("nanoid.alphabet", "_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	v.SetDefault("cuid2.length", 24)
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("grpc.port", "GRPC_PORT")
	v.BindEnv("snowflake.machine_id", "SNOWFLAKE_MACHINE_ID")
	v.BindEnv("nanoid.size", "NANOID_SIZE")
	v.BindEnv("nanoid.alphabet", "NANOID_ALPHABET")
	v.BindEnv("cuid2.length", "CUID2_LENGTH")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
