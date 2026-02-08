package generator

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// ULIDGenerator generates ULID (Universally Unique Lexicographically Sortable Identifier) IDs.
type ULIDGenerator struct{}

// NewULIDGenerator creates a new ULIDGenerator.
func NewULIDGenerator() *ULIDGenerator {
	return &ULIDGenerator{}
}

func (g *ULIDGenerator) Generate() (string, error) {
	id, err := ulid.New(ulid.Timestamp(time.Now()), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate ULID: %w", err)
	}
	return id.String(), nil
}

func (g *ULIDGenerator) GenerateBatch(count int) ([]string, error) {
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

func (g *ULIDGenerator) Validate(id string) (bool, string) {
	if len(id) != 26 {
		return false, fmt.Sprintf("expected length 26, got %d", len(id))
	}
	_, err := ulid.Parse(id)
	if err != nil {
		return false, fmt.Sprintf("invalid ULID format: %v", err)
	}
	return true, ""
}

func (g *ULIDGenerator) Parse(id string) (*ParseResult, error) {
	parsed, err := ulid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid ULID format: %w", err)
	}

	return &ParseResult{
		TimestampMs:   int64(parsed.Time()),
		RandomPayload: hex.EncodeToString(parsed.Entropy()),
	}, nil
}
