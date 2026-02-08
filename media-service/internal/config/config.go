package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pion/webrtc/v4"
	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
	"github.com/weiawesome/wes-io-live/pkg/pubsub"
)

type Config struct {
	Server  ServerConfig
	HLS     HLSConfig
	WebRTC  WebRTCConfig
	PubSub  pubsub.Config
	FFmpeg  FFmpegConfig
	Log     LogConfig
	Storage StorageConfig
	Preview PreviewConfig
}

type PreviewConfig struct {
	Enabled             bool   `mapstructure:"enabled"`
	IntervalSeconds     int    `mapstructure:"interval_seconds"`
	InitialDelaySeconds int    `mapstructure:"initial_delay_seconds"` // Delay before first capture
	Width               int    `mapstructure:"width"`
	Height              int    `mapstructure:"height"`
	Format              string `mapstructure:"format"`
	Quality             int    `mapstructure:"quality"`
}

type StorageConfig struct {
	Type    string             `mapstructure:"type"` // "local" or "s3"
	Local   LocalStorageConfig `mapstructure:"local"`
	S3      S3StorageConfig    `mapstructure:"s3"`
	VOD     VODConfig          `mapstructure:"vod"`
	Session SessionConfig      `mapstructure:"session"`
}

type SessionConfig struct {
	Type  string             `mapstructure:"type"` // "memory" or "redis"
	Redis SessionRedisConfig `mapstructure:"redis"`
}

type SessionRedisConfig struct {
	Address   string `mapstructure:"address"`    // empty = use pubsub.redis
	Password  string `mapstructure:"password"`
	DB        int    `mapstructure:"db"`
	KeyPrefix string `mapstructure:"key_prefix"`
	TTL       int    `mapstructure:"ttl"`        // seconds
}

type LocalStorageConfig struct {
	BasePath string `mapstructure:"base_path"`
}

type S3StorageConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UsePathStyle    bool   `mapstructure:"use_path_style"` // Required for MinIO
	PublicURL       string `mapstructure:"public_url"`
}

type VODConfig struct {
	Enabled       bool `mapstructure:"enabled"`
	AsyncUpload   bool `mapstructure:"async_upload"`
	UploadWorkers int  `mapstructure:"upload_workers"`
	RetentionDays int  `mapstructure:"retention_days"`
}

type ServerConfig struct {
	Host string
	Port int
}

type HLSConfig struct {
	OutputDir       string `mapstructure:"output_dir"`
	SegmentDuration int    `mapstructure:"segment_duration"`
	PlaylistSize    int    `mapstructure:"playlist_size"`
	DeleteSegments  bool   `mapstructure:"delete_segments"`
}

type WebRTCConfig struct {
	ICEServers []ICEServerConfig `mapstructure:"ice_servers"`
	TurnKeyID  string            `mapstructure:"turn_key_id"`
	TurnKey    string            `mapstructure:"turn_key"`
}

type ICEServerConfig struct {
	URLs       []string `mapstructure:"urls"`
	Username   string   `mapstructure:"username"`
	Credential string   `mapstructure:"credential"`
}

type FFmpegConfig struct {
	VideoCodec   string `mapstructure:"video_codec"`
	VideoPreset  string `mapstructure:"video_preset"`
	VideoBitrate string `mapstructure:"video_bitrate"` // e.g., "2M", "1500k", empty for CRF mode
	VideoCRF     int    `mapstructure:"video_crf"`     // 0-51, 0 means use bitrate instead
	Width        int    `mapstructure:"width"`         // 0 for original resolution
	Height       int    `mapstructure:"height"`        // 0 for original resolution
	Framerate    int    `mapstructure:"framerate"`     // 0 for original framerate
	AudioCodec   string `mapstructure:"audio_codec"`
	AudioBitrate string `mapstructure:"audio_bitrate"` // e.g., "128k"
	AudioSample  int    `mapstructure:"audio_sample"`  // e.g., 48000
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
	v.SetDefault("server.port", 8085)
	v.SetDefault("hls.output_dir", "./hls")
	v.SetDefault("hls.segment_duration", 2)
	v.SetDefault("hls.playlist_size", 5)
	v.SetDefault("hls.delete_segments", true)
	v.SetDefault("ffmpeg.video_codec", "libx264")
	v.SetDefault("ffmpeg.video_preset", "ultrafast")
	v.SetDefault("ffmpeg.video_bitrate", "")    // empty = use CRF
	v.SetDefault("ffmpeg.video_crf", 23)        // default CRF (0 = disabled, use bitrate)
	v.SetDefault("ffmpeg.width", 0)             // 0 = original
	v.SetDefault("ffmpeg.height", 0)            // 0 = original
	v.SetDefault("ffmpeg.framerate", 0)         // 0 = original
	v.SetDefault("ffmpeg.audio_codec", "aac")
	v.SetDefault("ffmpeg.audio_bitrate", "128k")
	v.SetDefault("ffmpeg.audio_sample", 48000)
	v.SetDefault("preview.enabled", true)
	v.SetDefault("preview.interval_seconds", 60)
	v.SetDefault("preview.initial_delay_seconds", 10) // Wait before first capture for better quality
	v.SetDefault("preview.width", 1280)
	v.SetDefault("preview.height", 720)
	v.SetDefault("preview.format", "jpeg")
	v.SetDefault("preview.quality", 80)
	v.SetDefault("pubsub.driver", "kafka")
	v.SetDefault("pubsub.redis.address", "localhost:6379")
	v.SetDefault("pubsub.kafka.brokers", "localhost:9092")
	v.SetDefault("pubsub.kafka.group_id", "media-service")
	v.SetDefault("pubsub.kafka.partitions", 4)
	v.SetDefault("log.level", "info")

	// Storage defaults
	v.SetDefault("storage.type", "local")
	v.SetDefault("storage.local.base_path", "./hls")
	v.SetDefault("storage.s3.region", "us-east-1")
	v.SetDefault("storage.s3.use_path_style", true) // Default to MinIO-compatible
	v.SetDefault("storage.vod.enabled", false)
	v.SetDefault("storage.vod.async_upload", true)
	v.SetDefault("storage.vod.upload_workers", 4)
	v.SetDefault("storage.vod.retention_days", 30)

	// Session store defaults
	v.SetDefault("storage.session.type", "memory")
	v.SetDefault("storage.session.redis.address", "")  // empty = use pubsub.redis
	v.SetDefault("storage.session.redis.db", 1)
	v.SetDefault("storage.session.redis.key_prefix", "vod:session:")
	v.SetDefault("storage.session.redis.ttl", 86400)  // 24 hours

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("hls.output_dir", "HLS_OUTPUT_DIR")
	v.BindEnv("webrtc.turn_key_id", "CF_TURN_ID")
	v.BindEnv("webrtc.turn_key", "CF_TURN_KEY")
	v.BindEnv("pubsub.redis.address", "REDIS_ADDRESS")
	v.BindEnv("pubsub.redis.password", "REDIS_PASSWORD")
	v.BindEnv("pubsub.kafka.brokers", "KAFKA_BROKERS")
	v.BindEnv("pubsub.kafka.group_id", "KAFKA_PUBSUB_GROUP_ID")

	// S3/MinIO environment bindings
	v.BindEnv("storage.s3.endpoint", "S3_ENDPOINT")
	v.BindEnv("storage.s3.region", "S3_REGION")
	v.BindEnv("storage.s3.bucket", "S3_BUCKET")
	v.BindEnv("storage.s3.access_key_id", "S3_ACCESS_KEY_ID")
	v.BindEnv("storage.s3.secret_access_key", "S3_SECRET_ACCESS_KEY")
	v.BindEnv("storage.s3.public_url", "S3_PUBLIC_URL")
	v.BindEnv("storage.vod.enabled", "VOD_ENABLED")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Load TURN credentials from environment if available
	if cfg.WebRTC.TurnKeyID == "" {
		cfg.WebRTC.TurnKeyID = os.Getenv("CF_TURN_ID")
	}
	if cfg.WebRTC.TurnKey == "" {
		cfg.WebRTC.TurnKey = os.Getenv("CF_TURN_KEY")
	}

	return &cfg, nil
}

// GetICEServers returns the ICE servers configuration for WebRTC.
func (c *WebRTCConfig) GetICEServers() ([]webrtc.ICEServer, error) {
	servers := make([]webrtc.ICEServer, 0, len(c.ICEServers))

	for _, s := range c.ICEServers {
		servers = append(servers, webrtc.ICEServer{
			URLs:       s.URLs,
			Username:   s.Username,
			Credential: s.Credential,
		})
	}

	// Add Cloudflare TURN if configured
	if c.TurnKeyID != "" && c.TurnKey != "" {
		turnServers, err := getCloudfareTURN(c.TurnKeyID, c.TurnKey)
		if err == nil && turnServers != nil {
			servers = append(servers, *turnServers)
		}
	}

	return servers, nil
}

type cloudflareTURNResponse struct {
	ICEServers struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username"`
		Credential string   `json:"credential"`
	} `json:"iceServers"`
}

func getCloudfareTURN(keyID, key string) (*webrtc.ICEServer, error) {
	url := fmt.Sprintf("https://rtc.live.cloudflare.com/v1/turn/keys/%s/credentials/generate", keyID)
	reqBody := []byte(`{"ttl": 86400}`)

	req, err := http.NewRequest("POST", url, io.NopCloser(bytes.NewReader(reqBody)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(reqBody))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TURN API returned status: %d", resp.StatusCode)
	}

	var turnResp cloudflareTURNResponse
	if err := json.NewDecoder(resp.Body).Decode(&turnResp); err != nil {
		return nil, err
	}

	return &webrtc.ICEServer{
		URLs:       turnResp.ICEServers.URLs,
		Username:   turnResp.ICEServers.Username,
		Credential: turnResp.ICEServers.Credential,
	}, nil
}
