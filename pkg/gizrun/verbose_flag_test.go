package gizrun

import (
	"log/slog"
	"testing"
)

func TestVerboseFlag(t *testing.T) {
	var flag verboseFlag

	if got := flag.String(); got != "" {
		t.Fatalf("String() = %q, want empty", got)
	}
	if err := flag.Set("debug"); err != nil {
		t.Fatal(err)
	}
	if got := flag.level; got != slog.LevelDebug {
		t.Fatalf("level = %v, want debug", got)
	}
	if got := flag.String(); got != "DEBUG" {
		t.Fatalf("String() = %q, want DEBUG", got)
	}
	if err := flag.Set("warn"); err != nil {
		t.Fatal(err)
	}
	if got := flag.level; got != slog.LevelWarn {
		t.Fatalf("level = %v, want warn", got)
	}
	if err := flag.Set("invalid"); err == nil {
		t.Fatal("Set(invalid) err = nil, want error")
	}
}

func TestVerboseFlagNilPointer(t *testing.T) {
	var flag *verboseFlag
	if err := flag.Set("debug"); err != nil {
		t.Fatal(err)
	}
}
