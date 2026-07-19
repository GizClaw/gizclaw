package gizclaw

import (
	"context"
	"testing"
)

func TestMigratorMigrateValidation(t *testing.T) {
	if err := (*Migrator)(nil).Migrate(context.Background()); err == nil {
		t.Fatal("nil migrator Migrate() error = nil")
	}
	if err := (&Migrator{}).Migrate(context.Background()); err != nil {
		t.Fatalf("empty migrator Migrate() error = %v", err)
	}
}
