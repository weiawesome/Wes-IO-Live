package generator

import (
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

const (
	DefaultNanoIDSize     = 21
	DefaultNanoIDAlphabet = "_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// NanoIDGenerator generates NanoID identifiers with configurable size and alphabet.
type NanoIDGenerator struct {
	size     int
	alphabet string
}

// NewNanoIDGenerator creates a new NanoIDGenerator.
// size must be between 1 and 256. alphabet must have at least 2 characters.
func NewNanoIDGenerator(size int, alphabet string) (*NanoIDGenerator, error) {
	if size < 1 || size > 256 {
		return nil, fmt.Errorf("nanoid size must be between 1 and 256, got %d", size)
	}
	if len(alphabet) < 2 {
		return nil, fmt.Errorf("nanoid alphabet must have at least 2 characters, got %d", len(alphabet))
	}
	return &NanoIDGenerator{
		size:     size,
		alphabet: alphabet,
	}, nil
}

func (g *NanoIDGenerator) Generate() (string, error) {
	id, err := gonanoid.Generate(g.alphabet, g.size)
	if err != nil {
		return "", fmt.Errorf("failed to generate NanoID: %w", err)
	}
	return id, nil
}

func (g *NanoIDGenerator) GenerateBatch(count int) ([]string, error) {
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

func (g *NanoIDGenerator) Validate(id string) (bool, string) {
	if len(id) != g.size {
		return false, fmt.Sprintf("expected length %d, got %d", g.size, len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune(g.alphabet, c) {
			return false, fmt.Sprintf("character '%c' not in alphabet", c)
		}
	}
	return true, ""
}

func (g *NanoIDGenerator) Parse(id string) (*ParseResult, error) {
	valid, reason := g.Validate(id)
	if !valid {
		return nil, fmt.Errorf("invalid NanoID: %s", reason)
	}

	return &ParseResult{
		IDLength: int32(len(id)),
		Alphabet: g.alphabet,
	}, nil
}
