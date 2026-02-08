package generator

import (
	"fmt"

	"github.com/nrednav/cuid2"
)

const DefaultCUID2Length = 24

// CUID2Generator generates CUID2 (Collision-resistant Unique IDentifier) IDs.
type CUID2Generator struct {
	length   int
	generate func() string
}

// NewCUID2Generator creates a new CUID2Generator with the given length.
// length must be between 2 and 32.
func NewCUID2Generator(length int) (*CUID2Generator, error) {
	if length < 2 || length > 32 {
		return nil, fmt.Errorf("cuid2 length must be between 2 and 32, got %d", length)
	}
	gen, err := cuid2.Init(cuid2.WithLength(length))
	if err != nil {
		return nil, fmt.Errorf("failed to init CUID2 generator: %w", err)
	}
	return &CUID2Generator{
		length:   length,
		generate: gen,
	}, nil
}

func (g *CUID2Generator) Generate() (string, error) {
	return g.generate(), nil
}

func (g *CUID2Generator) GenerateBatch(count int) ([]string, error) {
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

func (g *CUID2Generator) Validate(id string) (bool, string) {
	if len(id) != g.length {
		return false, fmt.Sprintf("expected length %d, got %d", g.length, len(id))
	}
	if !cuid2.IsCuid(id) {
		return false, "invalid CUID2 format"
	}
	return true, ""
}

func (g *CUID2Generator) Parse(id string) (*ParseResult, error) {
	valid, reason := g.Validate(id)
	if !valid {
		return nil, fmt.Errorf("invalid CUID2: %s", reason)
	}

	return &ParseResult{
		IDLength: int32(len(id)),
	}, nil
}
