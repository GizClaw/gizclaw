package gizlog

import (
	"errors"
	"log/slog"
	"os"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

// ErrDefaultLoggerInstalled reports that another configured runtime owns the
// process-wide logger lease.
var ErrDefaultLoggerInstalled = errors.New("gizlog: process logger is already installed")

var defaultLoggerLease struct {
	sync.Mutex
	installed bool
}

type defaultLoggerInstallation struct {
	once     sync.Once
	previous *slog.Logger
	cleanup  func() error
	err      error
}

func (installation *defaultLoggerInstallation) close() error {
	if installation == nil {
		return nil
	}
	installation.once.Do(func() {
		defaultLoggerLease.Lock()
		slog.SetDefault(installation.previous)
		defaultLoggerLease.installed = false
		defaultLoggerLease.Unlock()
		installation.err = installation.cleanup()
	})
	return installation.err
}

// StoreResolver resolves named LogStores without transferring ownership.
type StoreResolver interface {
	Log(string) (logstore.ImmutableStore, error)
}

// NewLogger builds the process logger. Store-backed handlers do not own or
// close registry-owned stores.
func NewLogger(cfg Config, registries ...StoreResolver) (*slog.Logger, func() error, error) {
	if len(registries) > 1 {
		return nil, nil, &StoreResolutionError{Reason: "multiple store registries are not supported"}
	}
	cfg, err := PrepareConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	var registry StoreResolver
	if len(registries) > 0 {
		registry = registries[0]
	}
	failureReporter := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})
	handlers := make([]slog.Handler, 0, len(cfg.Sinks))
	for _, sink := range cfg.Sinks {
		level, err := ParseLevel(sink.Level)
		if err != nil {
			return nil, nil, err
		}
		switch sink.Kind {
		case SinkStderr:
			handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
		case SinkStore:
			if registry == nil {
				return nil, nil, &StoreResolutionError{Name: sink.Store, Reason: "store registry is not available"}
			}
			store, err := registry.Log(sink.Store)
			if err != nil {
				return nil, nil, &StoreResolutionError{Name: sink.Store, Err: err}
			}
			handler, err := logstore.NewSlogHandler(store, "system", "log", level)
			if err != nil {
				return nil, nil, err
			}
			handlers = append(handlers, newStoreFailureReportingHandler(handler, failureReporter, sink.Store))
		}
	}
	return slog.New(NewFanoutHandler(handlers...)), func() error { return nil }, nil
}

// StoreResolutionError reports an invalid store sink without exposing store configuration.
type StoreResolutionError struct {
	Name   string
	Reason string
	Err    error
}

func (e *StoreResolutionError) Error() string {
	prefix := "system_log store"
	if e.Name != "" {
		prefix += " " + e.Name
	}
	if e.Reason != "" {
		return prefix + ": " + e.Reason
	}
	if e.Err != nil {
		return prefix + ": " + e.Err.Error()
	}
	return prefix + ": resolution failed"
}

func (e *StoreResolutionError) Unwrap() error { return e.Err }

// InstallDefault installs the configured process logger and returns cleanup.
func InstallDefault(cfg Config, registries ...StoreResolver) (func() error, error) {
	logger, cleanup, err := NewLogger(cfg, registries...)
	if err != nil {
		return nil, err
	}
	defaultLoggerLease.Lock()
	if defaultLoggerLease.installed {
		defaultLoggerLease.Unlock()
		return nil, errors.Join(ErrDefaultLoggerInstalled, cleanup())
	}
	previous := slog.Default()
	slog.SetDefault(logger)
	defaultLoggerLease.installed = true
	defaultLoggerLease.Unlock()
	installation := &defaultLoggerInstallation{previous: previous, cleanup: cleanup}
	return installation.close, nil
}
