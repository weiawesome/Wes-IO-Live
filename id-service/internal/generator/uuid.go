package generator

import (
	"fmt"

	"github.com/google/uuid"
)

// UUIDGenerator generates UUID v4 IDs.
type UUIDGenerator struct{}

// NewUUIDGenerator creates a new UUIDGenerator.
func NewUUIDGenerator() *UUIDGenerator {
	return &UUIDGenerator{}
}

func (g *UUIDGenerator) Generate() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID: %w", err)
	}
	return id.String(), nil
}

func (g *UUIDGenerator) GenerateBatch(count int) ([]string, error) {
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id, err := uuid.NewRandom()
		if err != nil {
			return nil, fmt.Errorf("failed to generate UUID: %w", err)
		}
		ids = append(ids, id.String())
	}
	return ids, nil
}

func (g *UUIDGenerator) Validate(id string) (bool, string) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return false, fmt.Sprintf("invalid UUID format: %v", err)
	}
	if parsed.Version() != 4 {
		return false, fmt.Sprintf("expected UUID v4, got v%d", parsed.Version())
	}
	return true, ""
}

func (g *UUIDGenerator) Parse(id string) (*ParseResult, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID format: %w", err)
	}

	var variantStr string
	switch parsed.Variant() {
	case uuid.RFC4122:
		variantStr = "RFC4122"
	case uuid.Reserved:
		variantStr = "Reserved"
	case uuid.Microsoft:
		variantStr = "Microsoft"
	case uuid.Future:
		variantStr = "Future"
	default:
		variantStr = "Unknown"
	}

	return &ParseResult{
		UUIDVersion: int32(parsed.Version()),
		UUIDVariant: variantStr,
	}, nil
}
