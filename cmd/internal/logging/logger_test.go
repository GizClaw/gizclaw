package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestConfigIsZero(t *testing.T) {
	if !((Config{}).IsZero()) {
		t.Fatal("empty config should be zero")
	}
	if !((Config{Level: "  "}).IsZero()) {
		t.Fatal("blank level should be zero")
	}
	if (Config{Level: "info"}).IsZero() {
		t.Fatal("level should make config non-zero")
	}
	if (Config{Volc: VolcConfig{Enabled: true}}).IsZero() {
		t.Fatal("volc config should make config non-zero")
	}
}

func TestNewLoggerDefaultStderrOnly(t *testing.T) {
	logger, cleanup, err := NewLogger(Config{})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	if logger == nil {
		t.Fatal("NewLogger returned nil logger")
	}
	if cleanup == nil {
		t.Fatal("NewLogger returned nil cleanup")
	}
	if !logger.Handler().Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("default logger should enable info")
	}
	if logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("default logger should not enable debug")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
}

func TestNewLoggerRejectsInvalidConfig(t *testing.T) {
	if _, _, err := NewLogger(Config{Level: "verbose"}); err == nil {
		t.Fatal("NewLogger should reject invalid config")
	}
}

func TestInstallDefaultRestoresPreviousLogger(t *testing.T) {
	previous := slog.Default()
	cleanup, err := InstallDefault(Config{Level: "debug"})
	if err != nil {
		t.Fatalf("InstallDefault() error = %v", err)
	}
	if slog.Default() == previous {
		t.Fatal("InstallDefault did not replace default logger")
	}
	if !slog.Default().Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("installed debug logger should enable debug")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if slog.Default() != previous {
		t.Fatal("cleanup did not restore previous default logger")
	}
}
