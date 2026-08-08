package metrics

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestClickHouseBorrowedPoolValidationDoesNotClosePool(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := NewClickHouseStoreWithDB(nil, "metrics"); err == nil {
		t.Fatal("nil borrowed pool was accepted")
	}
	if _, err := NewClickHouseStoreWithDB(db, "bad-name"); err == nil {
		t.Fatal("invalid table was accepted")
	}
	if _, err := NewClickHouseStoreWithDB(db, "metrics"); err == nil {
		t.Fatal("SQLite pool unexpectedly passed ClickHouse schema validation")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("borrowed pool was closed after constructor failure: %v", err)
	}
}

func TestClickHouseBorrowedStoreCloseLeavesPoolOpen(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := &ClickHouseStore{db: db, table: "metrics"}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Close() closed borrowed pool: %v", err)
	}
}
