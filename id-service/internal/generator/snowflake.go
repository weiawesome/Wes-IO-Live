package generator

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	timestampBits = 41
	machineIDBits = 10
	sequenceBits  = 12

	maxMachineID = (1 << machineIDBits) - 1 // 1023
	maxSequence  = (1 << sequenceBits) - 1   // 4095

	machineIDShift = sequenceBits
	timestampShift = sequenceBits + machineIDBits
)

// SnowflakeGenerator generates 64-bit snowflake IDs.
type SnowflakeGenerator struct {
	mu        sync.Mutex
	epoch     int64 // custom epoch in ms
	machineID int64 // 10-bit machine ID
	sequence  int64 // 12-bit sequence
	lastTime  int64 // last generation timestamp in ms
}

// NewSnowflakeGenerator creates a new SnowflakeGenerator.
// machineID must be in range [0, 1023].
// epoch is the custom epoch in unix milliseconds.
func NewSnowflakeGenerator(machineID int64, epoch int64) (*SnowflakeGenerator, error) {
	if machineID < 0 || machineID > maxMachineID {
		return nil, fmt.Errorf("machine_id must be between 0 and %d, got %d", maxMachineID, machineID)
	}
	return &SnowflakeGenerator{
		epoch:     epoch,
		machineID: machineID,
	}, nil
}

func (g *SnowflakeGenerator) Generate() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.generateLocked()
}

func (g *SnowflakeGenerator) GenerateBatch(count int) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id, err := g.generateLocked()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// generateLocked must be called with g.mu held.
func (g *SnowflakeGenerator) generateLocked() (string, error) {
	now := time.Now().UnixMilli()
	ts := now - g.epoch

	if ts < 0 {
		return "", fmt.Errorf("current time is before custom epoch")
	}

	if now < g.lastTime {
		return "", fmt.Errorf("clock moved backwards: current=%d, last=%d", now, g.lastTime)
	}

	if now == g.lastTime {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// Sequence exhausted, wait for next millisecond
			for now <= g.lastTime {
				now = time.Now().UnixMilli()
			}
			ts = now - g.epoch
		}
	} else {
		g.sequence = 0
	}

	g.lastTime = now

	id := (ts << timestampShift) | (g.machineID << machineIDShift) | g.sequence
	return strconv.FormatInt(id, 10), nil
}

func (g *SnowflakeGenerator) Validate(id string) (bool, string) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return false, "invalid integer format"
	}
	if n < 0 {
		return false, "id must be a positive integer"
	}

	// Extract and validate timestamp
	ts := (n >> timestampShift) & ((1 << timestampBits) - 1)
	absoluteMs := ts + g.epoch
	now := time.Now().UnixMilli()

	if absoluteMs < g.epoch {
		return false, "timestamp is before epoch"
	}
	if absoluteMs > now {
		return false, "timestamp is in the future"
	}

	// Extract and validate machine ID
	mid := (n >> machineIDShift) & maxMachineID
	if mid < 0 || mid > maxMachineID {
		return false, fmt.Sprintf("machine_id %d out of range [0, %d]", mid, maxMachineID)
	}

	return true, ""
}

func (g *SnowflakeGenerator) Parse(id string) (*ParseResult, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid integer format: %w", err)
	}
	if n < 0 {
		return nil, fmt.Errorf("id must be a positive integer")
	}

	ts := (n >> timestampShift) & ((1 << timestampBits) - 1)
	mid := (n >> machineIDShift) & maxMachineID
	seq := n & maxSequence

	return &ParseResult{
		TimestampMs: ts + g.epoch,
		MachineID:   mid,
		Sequence:    seq,
	}, nil
}
