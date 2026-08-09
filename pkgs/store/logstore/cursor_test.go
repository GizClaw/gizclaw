package logstore

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSQLCursorBindsNormalizedQueryAndAllowsLimitChange(t *testing.T) {
	query := validQuery()
	query.Streams = []string{"z", "a"}
	query.Limit = 2
	cursor, err := encodeSQLCursor(sqlCursor{
		Version: 1,
		Query:   normalizeSQLQuery(query),
		Position: sqlPosition{
			TimeUnixNano: time.UnixMilli(1500).UnixNano(), Stream: "a", ID: "one",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSQLCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	query.Limit = 100
	query.Streams = []string{"a", "z"}
	if !equalSQLQuery(decoded.Query, normalizeSQLQuery(query)) {
		t.Fatal("limit or selector order changed cursor identity")
	}
	query.Text = "changed"
	if equalSQLQuery(decoded.Query, normalizeSQLQuery(query)) {
		t.Fatal("changed query matched cursor")
	}
	if _, err := decodeSQLCursor("not-base64"); !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("decode error = %v", err)
	}
	if _, err := decodeSQLCursor(strings.Repeat("x", sqlCursorLimit+1)); !errors.Is(err, ErrCursorMismatch) {
		t.Fatalf("oversized error = %v", err)
	}
}
