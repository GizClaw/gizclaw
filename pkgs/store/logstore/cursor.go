package logstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const sqlCursorLimit = 16 * 1024

type sqlBoundQuery struct {
	Streams    []string
	Kinds      []string
	Severities []string
	Matchers   []AttributeMatcher
	Text       string
	StartMS    int64
	EndMS      int64
	Order      Order
}

type sqlPosition struct {
	TimeUnixNano int64
	Stream       string
	ID           string
}

type sqlCursor struct {
	Version  int
	Query    sqlBoundQuery
	Position sqlPosition
}

func normalizeSQLQuery(query Query) sqlBoundQuery {
	bound := sqlBoundQuery{
		Streams:    append([]string(nil), query.Streams...),
		Kinds:      append([]string(nil), query.Kinds...),
		Severities: append([]string(nil), query.Severities...),
		Matchers:   append([]AttributeMatcher(nil), query.Matchers...),
		Text:       query.Text,
		StartMS:    query.Start.UnixMilli(),
		EndMS:      query.End.UnixMilli(),
		Order:      query.Order,
	}
	slices.Sort(bound.Streams)
	slices.Sort(bound.Kinds)
	slices.Sort(bound.Severities)
	for index := range bound.Matchers {
		if bound.Matchers[index].Op == MatchExists || bound.Matchers[index].Op == MatchNotExists {
			bound.Matchers[index].Value = ""
		}
	}
	slices.SortFunc(bound.Matchers, func(left, right AttributeMatcher) int {
		if value := strings.Compare(left.Name, right.Name); value != 0 {
			return value
		}
		if value := strings.Compare(string(left.Op), string(right.Op)); value != 0 {
			return value
		}
		return strings.Compare(left.Value, right.Value)
	})
	return bound
}

func equalSQLQuery(left, right sqlBoundQuery) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func encodeSQLCursor(cursor sqlCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len(encoded) > sqlCursorLimit {
		return "", errors.New("logstore: SQL cursor is too large")
	}
	return encoded, nil
}

func decodeSQLCursor(value string) (sqlCursor, error) {
	if len(value) > sqlCursorLimit {
		return sqlCursor{}, fmt.Errorf("%w: cursor is too large", ErrCursorMismatch)
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(data) > sqlCursorLimit {
		return sqlCursor{}, fmt.Errorf("%w: cursor is malformed", ErrCursorMismatch)
	}
	var cursor sqlCursor
	if err := json.Unmarshal(data, &cursor); err != nil ||
		cursor.Version != 1 ||
		strings.TrimSpace(cursor.Position.Stream) == "" ||
		strings.TrimSpace(cursor.Position.ID) == "" {
		return sqlCursor{}, fmt.Errorf("%w: cursor is invalid", ErrCursorMismatch)
	}
	return cursor, nil
}

// Compatibility aliases keep the existing ClickHouse tests and query builder
// on the backend-neutral cursor representation.
type clickHouseBoundQuery = sqlBoundQuery
type clickHousePosition = sqlPosition
type clickHouseCursor = sqlCursor

func normalizeClickHouseQuery(query Query) sqlBoundQuery      { return normalizeSQLQuery(query) }
func equalClickHouseQuery(left, right sqlBoundQuery) bool     { return equalSQLQuery(left, right) }
func encodeClickHouseCursor(cursor sqlCursor) (string, error) { return encodeSQLCursor(cursor) }
func decodeClickHouseCursor(value string) (sqlCursor, error)  { return decodeSQLCursor(value) }
