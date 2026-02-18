package delivery

import (
	"context"
	"sync"
	"time"

	"github.com/weiawesome/wes-io-live/chat-dispatcher/internal/config"
	"github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/chat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type connEntry struct {
	conn     *grpc.ClientConn
	client   pb.ChatDeliveryServiceClient
	lastUsed time.Time
	mu       sync.Mutex
}

type GRPCPool struct {
	conns       sync.Map // map[string]*connEntry
	dialTimeout time.Duration
	callTimeout time.Duration
	idleTimeout time.Duration
	stopOnce    sync.Once
	stopCh      chan struct{}
}

func NewGRPCPool(cfg config.GRPCConfig) *GRPCPool {
	p := &GRPCPool{
		dialTimeout: cfg.DialTimeout,
		callTimeout: cfg.CallTimeout,
		idleTimeout: cfg.IdleTimeout,
		stopCh:      make(chan struct{}),
	}

	go p.evictLoop()
	return p
}

func (p *GRPCPool) getOrDial(ctx context.Context, addr string) (*connEntry, error) {
	// Fast path: connection exists
	if val, ok := p.conns.Load(addr); ok {
		entry := val.(*connEntry)
		entry.mu.Lock()
		entry.lastUsed = time.Now()
		entry.mu.Unlock()
		return entry, nil
	}

	// Slow path: create new connection
	dialCtx, cancel := context.WithTimeout(ctx, p.dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	entry := &connEntry{
		conn:     conn,
		client:   pb.NewChatDeliveryServiceClient(conn),
		lastUsed: time.Now(),
	}

	// Race-safe: if another goroutine stored first, use theirs and close ours
	actual, loaded := p.conns.LoadOrStore(addr, entry)
	if loaded {
		conn.Close()
		winner := actual.(*connEntry)
		winner.mu.Lock()
		winner.lastUsed = time.Now()
		winner.mu.Unlock()
		return winner, nil
	}

	return entry, nil
}

func (p *GRPCPool) DeliverMessage(ctx context.Context, addr string, req *pb.DeliverMessageRequest) (*pb.DeliverMessageResponse, error) {
	entry, err := p.getOrDial(ctx, addr)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, p.callTimeout)
	defer cancel()

	resp, err := entry.client.DeliverMessage(callCtx, req)
	if err != nil {
		// Evict failed connection so next call re-dials
		p.removeConn(addr)
		return nil, err
	}

	return resp, nil
}

func (p *GRPCPool) removeConn(addr string) {
	val, loaded := p.conns.LoadAndDelete(addr)
	if loaded {
		entry := val.(*connEntry)
		entry.conn.Close()
	}
}

func (p *GRPCPool) evictLoop() {
	ticker := time.NewTicker(p.idleTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			p.conns.Range(func(key, value any) bool {
				addr := key.(string)
				entry := value.(*connEntry)
				entry.mu.Lock()
				idle := now.Sub(entry.lastUsed) > p.idleTimeout
				entry.mu.Unlock()
				if idle {
					l := log.L()
					l.Info().Str("addr", addr).Msg("evicting idle grpc connection")
					p.removeConn(addr)
				}
				return true
			})
		}
	}
}

func (p *GRPCPool) Close() error {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})

	p.conns.Range(func(key, value any) bool {
		addr := key.(string)
		entry := value.(*connEntry)
		entry.conn.Close()
		p.conns.Delete(addr)
		l := log.L()
		l.Debug().Str("addr", addr).Msg("closed grpc connection")
		return true
	})

	return nil
}
