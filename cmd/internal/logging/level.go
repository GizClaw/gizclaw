package logging

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// ParseLevel parses the server logging level config.
func ParseLevel(value string) (slog.Level, error) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		n, err := strconv.Atoi(value)
		if err == nil {
			return slog.Level(n), nil
		}
		return slog.LevelInfo, fmt.Errorf("unknown level %q", value)
	}
}
