package gizedge

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestInstallConfiguredEdgeLoggingPreservesHostLoggerWhenOmitted(t *testing.T) {
	previous := slog.Default()
	closeLogging, err := installConfiguredEdgeLogging(Config{SystemLog: gizlog.DefaultConfig()})
	if err != nil {
		t.Fatalf("installConfiguredEdgeLogging error = %v", err)
	}
	t.Cleanup(func() { _ = closeLogging() })
	if slog.Default() != previous {
		t.Fatal("omitted system-log replaced the host process logger")
	}
	if err := closeLogging(); err != nil {
		t.Fatalf("closeLogging error = %v", err)
	}
}

func TestInstallEdgeLoggingDefaultsAndRestoresLogger(t *testing.T) {
	previous := slog.Default()
	cfg := Config{SystemLog: gizlog.DefaultConfig()}
	closeLogging, err := installEdgeLogging(cfg)
	if err != nil {
		t.Fatalf("installEdgeLogging error = %v", err)
	}
	t.Cleanup(func() { _ = closeLogging() })
	if slog.Default() == previous {
		t.Fatal("installEdgeLogging did not install a process logger")
	}
	if err := closeLogging(); err != nil {
		t.Fatalf("closeLogging error = %v", err)
	}
	if slog.Default() != previous {
		t.Fatal("closeLogging did not restore the previous process logger")
	}
}

func TestInstallEdgeLoggingRejectsInvalidStorageBeforeInstallingLogger(t *testing.T) {
	previous := slog.Default()
	cfg := Config{
		Storage: map[string]storage.Config{
			"volc-logs": storage.VolcTLSConfig{},
		},
		Stores: map[string]store.Config{
			"logs": {Kind: store.KindLogImmutable, Storage: "volc-logs", TopicID: "topic"},
		},
		SystemLog: gizlog.Config{Sinks: []gizlog.SinkConfig{{Kind: gizlog.SinkStore, Store: "logs"}}},
	}

	closeLogging, err := installEdgeLogging(cfg)
	if err == nil || !strings.Contains(err.Error(), "requires endpoint") {
		t.Fatalf("installEdgeLogging error = %v, want missing endpoint", err)
	}
	if closeLogging != nil {
		t.Fatal("installEdgeLogging returned cleanup after failed initialization")
	}
	if slog.Default() != previous {
		t.Fatal("failed installEdgeLogging changed the process logger")
	}
}

func TestInstallEdgeLoggingRejectsOverlappingRuntime(t *testing.T) {
	previous := slog.Default()
	firstCleanup, err := installEdgeLogging(Config{SystemLog: gizlog.DefaultConfig()})
	if err != nil {
		t.Fatalf("first installEdgeLogging error = %v", err)
	}
	t.Cleanup(func() { _ = firstCleanup() })
	first := slog.Default()
	secondCleanup, err := installEdgeLogging(Config{SystemLog: gizlog.DefaultConfig()})
	if !errors.Is(err, gizlog.ErrDefaultLoggerInstalled) {
		t.Fatalf("second installEdgeLogging error = %v, want %v", err, gizlog.ErrDefaultLoggerInstalled)
	}
	if secondCleanup != nil {
		t.Fatal("second installEdgeLogging returned cleanup without owning the logger lease")
	}
	if slog.Default() != first {
		t.Fatal("rejected Edge runtime changed the active process logger")
	}
	if err := firstCleanup(); err != nil {
		t.Fatalf("first cleanup error = %v", err)
	}
	if slog.Default() != previous {
		t.Fatal("first cleanup did not restore the host process logger")
	}
}
