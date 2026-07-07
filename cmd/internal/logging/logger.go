package logging

import (
	"errors"
	"io"
	"log/slog"
	"os"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging/volclog"
)

// NewLogger builds the process logger and a cleanup function for closeable
// sinks.
func NewLogger(cfg Config) (*slog.Logger, func() error, error) {
	cfg, err := PrepareConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	level, err := ParseLevel(cfg.Level)
	if err != nil {
		return nil, nil, err
	}
	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}),
	}
	var closers []io.Closer
	if cfg.Volc.Enabled {
		handler, err := volclog.NewHandler(volclog.Config{
			Endpoint:         cfg.Volc.Endpoint,
			Region:           cfg.Volc.Region,
			TopicID:          cfg.Volc.TopicID,
			AccessKeyID:      cfg.Volc.AccessKeyID,
			AccessKeySecret:  cfg.Volc.AccessKeySecret,
			Level:            level,
			EnableNanosecond: true,
		})
		if err != nil {
			return nil, nil, err
		}
		handlers = append(handlers, handler)
		closers = append(closers, handler)
	}
	cleanup := func() error {
		var errs []error
		for _, closer := range closers {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	return slog.New(NewFanoutHandler(handlers...)), cleanup, nil
}

// InstallDefault installs the configured process logger and returns cleanup.
func InstallDefault(cfg Config) (func() error, error) {
	logger, cleanup, err := NewLogger(cfg)
	if err != nil {
		return nil, err
	}
	previous := slog.Default()
	slog.SetDefault(logger)
	return func() error {
		slog.SetDefault(previous)
		return cleanup()
	}, nil
}
