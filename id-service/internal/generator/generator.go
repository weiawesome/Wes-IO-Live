package generator

// Generator defines the interface for ID generation, validation, and parsing.
type Generator interface {
	Generate() (string, error)
	GenerateBatch(count int) ([]string, error)
	Validate(id string) (bool, string) // (valid, reason)
	Parse(id string) (*ParseResult, error)
}

// ParseResult holds the parsed fields from an ID.
type ParseResult struct {
	TimestampMs   int64  // Snowflake/ULID/KSUID: absolute unix ms
	MachineID     int64  // Snowflake only
	Sequence      int64  // Snowflake only
	UUIDVersion   int32  // UUID only (4)
	UUIDVariant   string // UUID only ("RFC4122")
	RandomPayload string // ULID/KSUID: hex-encoded random bytes
	IDLength      int32  // NanoID/CUID2: ID string length
	Alphabet      string // NanoID: character set used
}
