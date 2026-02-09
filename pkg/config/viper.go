package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load reads configuration from file and environment variables.
// configPath is the directory containing config files.
// configName is the name of the config file (without extension).
func Load(configPath, configName string) (*viper.Viper, error) {
	v := viper.New()

	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	// Environment variable support
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return v, nil // Config file not found, rely on env vars
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return v, nil
}

// GetEnv returns environment variable value or default.
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// MustGetEnv returns environment variable or panics if not set.
func MustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return value
}
