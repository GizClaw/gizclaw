package server

import (
	"testing"
)

func TestCmdMigratorCloseHandlesNilState(t *testing.T) {
	var nilMigrator *CmdMigrator
	if err := nilMigrator.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
	if err := (&CmdMigrator{}).Close(); err != nil {
		t.Fatalf("empty Close() error = %v", err)
	}
}
