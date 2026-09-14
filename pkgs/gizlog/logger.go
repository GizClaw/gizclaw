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
		defaultLoggerLease.Unlock()
		installation.err = installation.cleanup()
		defaultLoggerLease.Lock()
		defaultLoggerLease.installed = false
		defaultLoggerLease.Unlock()
	})
	return installation.err
}

// StoreResolver resolves named LogStores without transferring ownership.
type StoreResolver interface {
	Log(string) (logstore.ImmutableStore, error)
}

// NewLogger builds an independent logger. Store-backed handlers borrow registry
// stores. Each logger with Store sinks owns one bounded queue and worker shared
// by those sinks; cleanup drains only that logger. Callers must clean up every
// borrowing logger before closing the registry. Stderr-only loggers need no worker.
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
	failureReporter := slog.Handler(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: slog.LevelError}))
	fixed := make([]slog.Attr, 0, 1)
	if cfg.NodeID != "" {
		fixed = append(fixed, slog.String("node_id", cfg.NodeID))
		failureReporter = failureReporter.WithAttrs(fixed)
	}
	queue := newStoreQueue(failureReporter)
	started := false
	defer func() {
		if !started {
			queue.cancel()
		}
	}()
	handlers := make([]slog.Handler, 0, len(cfg.Sinks))
	for _, sink := range cfg.Sinks {
		level, err := ParseLevel(sink.Level)
		if err != nil {
			return nil, nil, err
		}
		switch sink.Kind {
		case SinkStderr:
			handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: level}))
		case SinkStore:
			if registry == nil {
				return nil, nil, &StoreResolutionError{Name: sink.Store, Reason: "store registry is not available"}
			}
			store, err := registry.Log(sink.Store)
			if err != nil {
				return nil, nil, &StoreResolutionError{Name: sink.Store, Err: err}
			}
			handler, err := logstore.NewSlogHandler(queuedStoreAppender{queue: queue, store: store, name: sink.Store}, "system", "log", level)
			if err != nil {
				return nil, nil, err
			}
			handlers = append(handlers, newStoreFailureReportingHandler(handler, failureReporter, sink.Store))
		}
	}
	logger := slog.New(newContextHandler(NewFanoutHandler(handlers...), fixed))
	if len(cfg.Sinks) == 1 && cfg.Sinks[0].Kind == SinkStderr {
		return logger, func() error { return nil }, nil
	}
	go queue.run()
	started = true
	return logger, queue.close, nil
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
