// Package logstore defines a business-neutral append-only searchable record store.
package logstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxLimit is the largest page accepted by the store contract.
	MaxLimit = 1000
	// MaxAttributeNameBytes is the largest UTF-8 encoded attribute path.
	MaxAttributeNameBytes = 128
)

var (
	// ErrInvalidQuery identifies an invalid backend-neutral query.
	ErrInvalidQuery = errors.New("logstore: invalid query")
	// ErrCursorMismatch identifies a cursor that is malformed, unsupported, or
	// bound to different query fields.
	ErrCursorMismatch = errors.New("logstore: cursor mismatch")
)

// Appender appends complete records to a log store.
type Appender interface {
	Append(context.Context, []Record) error
}

// Querier reads one ordered page from a log store.
type Querier interface {
	Query(context.Context, Query) (Page, error)
}

// Store is a complete append, query, and lifecycle capability.
type Store interface {
	Appender
	Querier
	Close() error
}

// Record is one append-only searchable record.
type Record struct {
	ID         string
	Time       time.Time
	Stream     string
	Kind       string
	Severity   string
	Message    string
	Attributes map[string]string
	Payload    json.RawMessage
}

// MatchOp identifies an attribute matcher operation.
type MatchOp string

const (
	MatchEqual     MatchOp = "="
	MatchNotEqual  MatchOp = "!="
	MatchExists    MatchOp = "exists"
	MatchNotExists MatchOp = "not-exists"
)

// AttributeMatcher matches one canonical dotted attribute path.
type AttributeMatcher struct {
	Name  string
	Op    MatchOp
	Value string
}

// Order is the provider-neutral time sort direction.
type Order string

const (
	OrderAsc  Order = "asc"
	OrderDesc Order = "desc"
)

// Query selects records in the half-open UTC interval [Start, End).
type Query struct {
	Streams    []string
	Kinds      []string
	Severities []string
	Matchers   []AttributeMatcher
	Text       string
	Start      time.Time
	End        time.Time
	Limit      int
	Order      Order
	Cursor     string
}

// Page is one ordered result page.
type Page struct {
	Records    []Record
	HasNext    bool
	NextCursor string
}

// ValidateRecord validates an append record without retaining its mutable data.
func ValidateRecord(record Record) error {
	if strings.TrimSpace(record.ID) == "" {
		return errors.New("logstore: record id is required")
	}
	if record.Time.IsZero() {
		return errors.New("logstore: record time is required")
	}
	if strings.TrimSpace(record.Stream) == "" {
		return errors.New("logstore: record stream is required")
	}
	if strings.TrimSpace(record.Kind) == "" {
		return errors.New("logstore: record kind is required")
	}
	if len(record.Payload) != 0 && !json.Valid(record.Payload) {
		return errors.New("logstore: record payload is not valid JSON")
	}
	return validateAttributes(record.Attributes)
}

func validateAttributes(attributes map[string]string) error {
	paths := make(map[string]struct{}, len(attributes))
	for name := range attributes {
		if err := ValidateAttributeName(name); err != nil {
			return err
		}
		if _, exists := paths[name]; exists {
			return fmt.Errorf("logstore: duplicate attribute path %q", name)
		}
		paths[name] = struct{}{}
	}
	for name := range paths {
		for prefix := name; ; {
			index := strings.LastIndexByte(prefix, '.')
			if index < 0 {
				break
			}
			prefix = prefix[:index]
			if _, conflict := paths[prefix]; conflict {
				return fmt.Errorf("logstore: attribute path %q conflicts with %q", name, prefix)
			}
		}
	}
	return nil
}

// ValidateQuery validates a complete backend-neutral query.
func ValidateQuery(query Query) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidQuery, fmt.Sprintf(format, args...))
	}
	if query.Start.IsZero() || query.End.IsZero() {
		return invalid("start and end are required")
	}
	if !query.Start.Equal(query.Start.Truncate(time.Millisecond)) || !query.End.Equal(query.End.Truncate(time.Millisecond)) {
		return invalid("start and end must have millisecond precision")
	}
	if !query.End.After(query.Start) {
		return invalid("end must be later than start")
	}
	if query.Limit <= 0 || query.Limit > MaxLimit {
		return invalid("limit must be between 1 and %d", MaxLimit)
	}
	if query.Order != OrderAsc && query.Order != OrderDesc {
		return invalid("unsupported order %q", query.Order)
	}
	if !utf8.ValidString(query.Text) {
		return invalid("text must be valid UTF-8")
	}
	for _, values := range [][]string{query.Streams, query.Kinds, query.Severities} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return invalid("selector values must not be empty")
			}
		}
	}
	for _, matcher := range query.Matchers {
		if err := ValidateAttributeName(matcher.Name); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidQuery, err)
		}
		switch matcher.Op {
		case MatchEqual, MatchNotEqual:
			if matcher.Value == "" {
				return invalid("matcher %q value is required", matcher.Name)
			}
		case MatchExists, MatchNotExists:
		case "":
			return invalid("matcher %q operator is required", matcher.Name)
		default:
			return invalid("unsupported matcher operator %q", matcher.Op)
		}
	}
	return nil
}

// ValidatePage validates cursor invariants for a driver result.
func ValidatePage(page Page, limit int) error {
	if len(page.Records) > limit {
		return fmt.Errorf("logstore: page contains %d records for limit %d", len(page.Records), limit)
	}
	if page.HasNext && page.NextCursor == "" {
		return errors.New("logstore: next cursor is required when another page exists")
	}
	if !page.HasNext && page.NextCursor != "" {
		return errors.New("logstore: next cursor must be empty on the final page")
	}
	return nil
}

// ValidateAttributeName validates a canonical dotted attribute path.
func ValidateAttributeName(name string) error {
	if name == "" || len(name) > MaxAttributeNameBytes || !utf8.ValidString(name) {
		return fmt.Errorf("logstore: invalid attribute name %q", name)
	}
	for segment := range strings.SplitSeq(name, ".") {
		if segment == "" || !validAttributeFirst(segment[0]) {
			return fmt.Errorf("logstore: invalid attribute name %q", name)
		}
		for i := 1; i < len(segment); i++ {
			if !validAttributeRest(segment[i]) {
				return fmt.Errorf("logstore: invalid attribute name %q", name)
			}
		}
	}
	return nil
}

func validAttributeFirst(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func validAttributeRest(value byte) bool {
	return validAttributeFirst(value) || value == '-' || value >= '0' && value <= '9'
}

func cloneRecord(record Record) Record {
	clone := record
	if record.Attributes != nil {
		clone.Attributes = make(map[string]string, len(record.Attributes))
		maps.Copy(clone.Attributes, record.Attributes)
	}
	clone.Payload = append(json.RawMessage(nil), record.Payload...)
	return clone
}
