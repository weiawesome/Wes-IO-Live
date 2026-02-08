package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	pkgconfig "github.com/weiawesome/wes-io-live/pkg/config"
)

type Config struct {
	Server ServerConfig
	WebRTC WebRTCConfig
	Log    LogConfig
}

type ServerConfig struct {
	Host string
	Port int
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
	v.SetDefault("server.port", 8086)
	v.SetDefault("log.level", "info")

	// Override from environment
	v.BindEnv("server.port", "PORT")
	v.BindEnv("webrtc.turn_key_id", "CF_TURN_ID")
	v.BindEnv("webrtc.turn_key", "CF_TURN_KEY")

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

	// Debug: log TURN configuration status
	if cfg.WebRTC.TurnKeyID != "" && cfg.WebRTC.TurnKey != "" {
		log.Printf("TURN configuration loaded: TurnKeyID=%s, TurnKey=%s", cfg.WebRTC.TurnKeyID, maskKey(cfg.WebRTC.TurnKey))
	} else {
		log.Printf("TURN configuration not found: TurnKeyID=%q, TurnKey=%q", cfg.WebRTC.TurnKeyID, maskKey(cfg.WebRTC.TurnKey))
	}

	return &cfg, nil
}

// ICEServer represents an ICE server configuration for clients.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// GetICEServers returns the ICE servers configuration for WebRTC clients.
func (c *WebRTCConfig) GetICEServers() ([]ICEServer, error) {
	servers := make([]ICEServer, 0, len(c.ICEServers))

	for _, s := range c.ICEServers {
		servers = append(servers, ICEServer{
			URLs:       s.URLs,
			Username:   s.Username,
			Credential: s.Credential,
		})
	}

	// Add Cloudflare TURN if configured
	if c.TurnKeyID != "" && c.TurnKey != "" {
		log.Printf("TURN credentials found, attempting to fetch Cloudflare TURN servers...")
		turnServer, err := getCloudflareTURN(c.TurnKeyID, c.TurnKey)
		if err != nil {
			log.Printf("Warning: Failed to get Cloudflare TURN credentials: %v", err)
		} else if turnServer != nil {
			log.Printf("Successfully added Cloudflare TURN server with %d URLs", len(turnServer.URLs))
			servers = append(servers, *turnServer)
		}
	} else {
		log.Printf("TURN credentials not configured (TurnKeyID: %q, TurnKey: %q)", c.TurnKeyID, maskKey(c.TurnKey))
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

func getCloudflareTURN(keyID, key string) (*ICEServer, error) {
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
		return nil, fmt.Errorf("failed to call TURN API: %w", err)
	}
	defer resp.Body.Close()

	// Cloudflare TURN API returns 201 (Created) on success, not 200
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TURN API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var turnResp cloudflareTURNResponse
	if err := json.NewDecoder(resp.Body).Decode(&turnResp); err != nil {
		return nil, fmt.Errorf("failed to decode TURN response: %w", err)
	}

	return &ICEServer{
		URLs:       turnResp.ICEServers.URLs,
		Username:   turnResp.ICEServers.Username,
		Credential: turnResp.ICEServers.Credential,
	}, nil
}

// maskKey masks a key for logging purposes
func maskKey(key string) string {
	if key == "" {
		return "<empty>"
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
