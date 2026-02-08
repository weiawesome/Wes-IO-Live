package database

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
)

// StringArray is a custom type for storing string arrays that works across
// different databases (PostgreSQL, MySQL, SQLite).
// - PostgreSQL: stored as TEXT[] native array
// - MySQL/SQLite: stored as JSON string
type StringArray []string

// Scan implements the sql.Scanner interface for reading from the database.
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return a.scanBytes(v)
	case string:
		return a.scanBytes([]byte(v))
	default:
		return errors.New("StringArray: unsupported scan type")
	}
}

func (a *StringArray) scanBytes(data []byte) error {
	str := string(data)

	// Try JSON array first (MySQL/SQLite)
	if strings.HasPrefix(str, "[") {
		return json.Unmarshal(data, a)
	}

	// PostgreSQL array format: {item1,item2,item3}
	if strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}") {
		str = strings.TrimPrefix(str, "{")
		str = strings.TrimSuffix(str, "}")
		if str == "" {
			*a = []string{}
			return nil
		}
		*a = parsePostgresArray(str)
		return nil
	}

	// Fallback: treat as single item
	*a = []string{str}
	return nil
}

// parsePostgresArray parses PostgreSQL array format, handling quoted strings.
func parsePostgresArray(s string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch r {
		case '\\':
			escaped = true
		case '"':
			inQuotes = !inQuotes
		case ',':
			if inQuotes {
				current.WriteRune(r)
			} else {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// Value implements the driver.Valuer interface for writing to the database.
// Returns JSON format as string which works across all databases.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	data, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// GormDataType returns the GORM data type hint.
func (StringArray) GormDataType() string {
	return "text"
}
