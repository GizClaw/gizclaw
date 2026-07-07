package logging

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  slog.Level
	}{
		{name: "empty defaults info", value: "", want: slog.LevelInfo},
		{name: "debug", value: "debug", want: slog.LevelDebug},
		{name: "info", value: "info", want: slog.LevelInfo},
		{name: "warn", value: "warn", want: slog.LevelWarn},
		{name: "warning", value: "warning", want: slog.LevelWarn},
		{name: "error", value: "error", want: slog.LevelError},
		{name: "numeric", value: "-2", want: slog.Level(-2)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLevel(tc.value)
			if err != nil {
				t.Fatalf("ParseLevel() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseLevel() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel should reject unknown level")
	}
}
