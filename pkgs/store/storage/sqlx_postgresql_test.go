package storage

import (
	"strings"
	"testing"
	"time"
)

func TestPostgreSQLRejectsInvalidPoolBeforeConnecting(t *testing.T) {
	for name, config := range map[string]PostgreSQLConfig{
		"negative capacity":  {DSN: "postgres://secret:password@invalid", MaxOpenConns: -1},
		"negative idle time": {DSN: "postgres://secret:password@invalid", ConnMaxIdleTime: -time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			db, err := newPostgreSQL("test", config)
			if db != nil {
				_ = db.Close()
				t.Fatal("invalid pool opened")
			}
			if err == nil || !strings.Contains(err.Error(), "requires nonnegative") || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
				t.Fatalf("pool validation: %v", err)
			}
		})
	}
}
