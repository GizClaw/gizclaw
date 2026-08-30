package gizedge

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestInstallEdgeLoggingDefaultsAndRestoresLogger(t *testing.T) {
	previous := slog.Default()
	cfg := Config{SystemLog: gizlog.DefaultConfig()}
	closeLogging, err := installEdgeLogging(cfg)
	if err != nil {
		t.Fatalf("installEdgeLogging error = %v", err)
	}
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
