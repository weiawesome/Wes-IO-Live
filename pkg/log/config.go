package log

import (
	"io"
	stdlog "log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Config holds logger configuration.
type Config struct {
	Level       string `mapstructure:"level"`
	Pretty      bool   `mapstructure:"pretty"`
	ServiceName string `mapstructure:"service_name"`
}

var (
	global zerolog.Logger
	once   sync.Once
)

func init() {
	// Safe default before Init() is called.
	global = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

// New creates a configured zerolog.Logger.
func New(cfg Config) zerolog.Logger {
	var w io.Writer = os.Stdout
	if cfg.Pretty {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Kitchen}
	}

	lvl := parseLevel(cfg.Level)
	logger := zerolog.New(w).Level(lvl).With().Timestamp().Logger()

	if cfg.ServiceName != "" {
		logger = logger.With().Str(FieldService, cfg.ServiceName).Logger()
	}

	return logger
}

// Init initialises the global logger. Call once at service startup.
// It also bridges stdlib log to zerolog so existing log.Printf calls
// automatically produce structured JSON output.
func Init(cfg Config) {
	once.Do(func() {
		global = New(cfg)

		// Bridge stdlib log → zerolog.
		stdlog.SetFlags(0)
		stdlog.SetOutput(global.With().Str("source", "stdlog").Logger())
	})
}

// L returns the global logger.
func L() zerolog.Logger {
	return global
}

func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}
