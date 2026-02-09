package generator

import (
	"encoding/hex"
	"fmt"

	"github.com/segmentio/ksuid"
)

// KSUIDGenerator generates KSUID (K-Sortable Unique IDentifier) IDs.
type KSUIDGenerator struct{}

// NewKSUIDGenerator creates a new KSUIDGenerator.
func NewKSUIDGenerator() *KSUIDGenerator {
	return &KSUIDGenerator{}
}

func (g *KSUIDGenerator) Generate() (string, error) {
	id, err := ksuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("failed to generate KSUID: %w", err)
	}
	return id.String(), nil
}

func (g *KSUIDGenerator) GenerateBatch(count int) ([]string, error) {
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id, err := g.Generate()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (g *KSUIDGenerator) Validate(id string) (bool, string) {
	if len(id) != 27 {
		return false, fmt.Sprintf("expected length 27, got %d", len(id))
	}
	_, err := ksuid.Parse(id)
	if err != nil {
		return false, fmt.Sprintf("invalid KSUID format: %v", err)
	}
	return true, ""
}

func (g *KSUIDGenerator) Parse(id string) (*ParseResult, error) {
	parsed, err := ksuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid KSUID format: %w", err)
	}

	return &ParseResult{
		TimestampMs:   parsed.Time().UnixMilli(),
		RandomPayload: hex.EncodeToString(parsed.Payload()),
	}, nil
}
