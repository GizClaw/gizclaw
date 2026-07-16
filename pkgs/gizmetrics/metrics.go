// Package gizmetrics provides process-wide, best-effort instrumentation backed
// by a metrics store installed by the host process.
package gizmetrics

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	storemetrics "github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

const (
	// DefaultFlushInterval is how often an installed recorder flushes dirty
	// series to its store.
	DefaultFlushInterval = 10 * time.Second
	// DefaultAppendTimeout bounds one periodic store append.
	DefaultAppendTimeout = 5 * time.Second
	// DefaultMaxSeries bounds the number of logical metric series retained by
	// one recorder.
	DefaultMaxSeries = 10_000
)

var (
	// ErrStoreRequired reports an attempt to install a nil metrics store.
	ErrStoreRequired = errors.New("gizmetrics: store is required")
	// ErrAlreadyInstalled reports an attempt to replace a live process
	// recorder without shutting it down first.
	ErrAlreadyInstalled = errors.New("gizmetrics: recorder already installed")
)

// Label is one metric label name and value.
type Label struct {
	Name  string
	Value string
}

// Option configures an installed store recorder.
type Option func(*recorderConfig) error

// WithFlushInterval configures how often dirty series are flushed.
func WithFlushInterval(interval time.Duration) Option {
	return func(config *recorderConfig) error {
		if interval <= 0 {
			return fmt.Errorf("gizmetrics: flush interval must be greater than zero")
		}
		config.flushInterval = interval
		return nil
	}
}

// WithAppendTimeout configures the timeout for one periodic store append.
func WithAppendTimeout(timeout time.Duration) Option {
	return func(config *recorderConfig) error {
		if timeout <= 0 {
			return fmt.Errorf("gizmetrics: append timeout must be greater than zero")
		}
		config.appendTimeout = timeout
		return nil
	}
}

// WithMaxSeries configures the maximum number of logical series retained by
// one recorder.
func WithMaxSeries(maxSeries int) Option {
	return func(config *recorderConfig) error {
		if maxSeries <= 0 {
			return fmt.Errorf("gizmetrics: max series must be greater than zero")
		}
		config.maxSeries = maxSeries
		return nil
	}
}

type recorder interface {
	addCounter(context.Context, string, float64, []Label)
	setGauge(context.Context, string, float64, []Label)
	observeHistogram(context.Context, string, float64, []float64, []Label)
}

type recorderSlot struct {
	recorder recorder
}

var (
	processRecorder atomic.Pointer[recorderSlot]
	installMu       sync.Mutex
)

// AddCounter adds a non-negative delta to a process-local counter. It is a
// no-op until InstallStore succeeds.
func AddCounter(ctx context.Context, name string, delta float64, labels ...Label) {
	slot := processRecorder.Load()
	if slot == nil {
		return
	}
	slot.recorder.addCounter(ctx, name, delta, labels)
}

// SetGauge records the latest value for a process-local gauge. It is a no-op
// until InstallStore succeeds.
func SetGauge(ctx context.Context, name string, value float64, labels ...Label) {
	slot := processRecorder.Load()
	if slot == nil {
		return
	}
	slot.recorder.setGauge(ctx, name, value, labels)
}

// ObserveHistogram records a value in a process-local cumulative histogram.
// Buckets must be finite and strictly increasing. It is a no-op until
// InstallStore succeeds.
func ObserveHistogram(ctx context.Context, name string, value float64, buckets []float64, labels ...Label) {
	slot := processRecorder.Load()
	if slot == nil {
		return
	}
	slot.recorder.observeHistogram(ctx, name, value, buckets, labels)
}

// InstallStore installs one process recorder and starts its periodic flush
// worker. The returned shutdown function is idempotent, restores the no-op
// default, and flushes pending samples without closing store.
func InstallStore(store storemetrics.Store, options ...Option) (func(context.Context) error, error) {
	if nilInterface(store) {
		return nil, ErrStoreRequired
	}
	config := recorderConfig{
		flushInterval: DefaultFlushInterval,
		appendTimeout: DefaultAppendTimeout,
		maxSeries:     DefaultMaxSeries,
	}
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("gizmetrics: option %d is nil", index)
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}

	installed := newStoreRecorder(store, config)
	slot := &recorderSlot{recorder: installed}
	installMu.Lock()
	if processRecorder.Load() != nil {
		installMu.Unlock()
		return nil, ErrAlreadyInstalled
	}
	processRecorder.Store(slot)
	installed.start()
	installMu.Unlock()

	var (
		shutdownOnce sync.Once
		shutdownErr  error
	)
	return func(ctx context.Context) error {
		shutdownOnce.Do(func() {
			installMu.Lock()
			if processRecorder.Load() == slot {
				processRecorder.Store(nil)
			}
			installMu.Unlock()
			shutdownErr = installed.shutdown(ctx)
		})
		return shutdownErr
	}, nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
