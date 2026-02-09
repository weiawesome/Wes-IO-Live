package cassandra

import (
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/weiawesome/wes-io-live/chat-persist-service/internal/config"
)

// Client wraps the Cassandra session.
type Client struct {
	session *gocql.Session
}

// NewClient creates a new Cassandra client and establishes a connection.
func NewClient(cfg config.CassandraConfig) (*Client, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Keyspace = cfg.Keyspace
	cluster.Consistency = parseConsistency(cfg.Consistency)
	cluster.ConnectTimeout = cfg.ConnectTimeout
	cluster.Timeout = cfg.Timeout
	cluster.NumConns = cfg.NumConns
	cluster.MaxPreparedStmts = cfg.MaxPreparedStmt

	if cfg.Username != "" && cfg.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}

	// Retry policy for resilience
	cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
		NumRetries: 3,
		Min:        100 * time.Millisecond,
		Max:        2 * time.Second,
	}

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create Cassandra session: %w", err)
	}

	return &Client{session: session}, nil
}

// Session returns the underlying gocql session.
func (c *Client) Session() *gocql.Session {
	return c.session
}

// Close closes the Cassandra session.
func (c *Client) Close() {
	if c.session != nil {
		c.session.Close()
	}
}

// parseConsistency converts a string consistency level to gocql.Consistency.
func parseConsistency(s string) gocql.Consistency {
	switch strings.ToUpper(s) {
	case "ANY":
		return gocql.Any
	case "ONE":
		return gocql.One
	case "TWO":
		return gocql.Two
	case "THREE":
		return gocql.Three
	case "QUORUM":
		return gocql.Quorum
	case "ALL":
		return gocql.All
	case "LOCAL_QUORUM":
		return gocql.LocalQuorum
	case "EACH_QUORUM":
		return gocql.EachQuorum
	case "LOCAL_ONE":
		return gocql.LocalOne
	default:
		return gocql.LocalQuorum
	}
}
