// Package logpressure supplies a bounded, controllable system LogStore for
// local SFU diagnostics. It is linked only by the explicit Go build overlay.
package logpressure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

// Store pauses only Append; control requests and real SFU traffic keep running.
type Store struct {
	mu          sync.Mutex
	gate        chan struct{}
	entered     chan struct{}
	timer       *time.Timer
	written     int
	duplicates  int
	keys        map[logstore.RecordKey]struct{}
	profile     []byte
	server      *http.Server
	done        chan struct{}
	started     time.Time
	entries     atomic.Uint64
	accepted    atomic.Uint64
	rejected    atomic.Uint64
	fill        atomic.Uint64
	handlerNS   atomic.Uint64
	appendNS    atomic.Uint64
	appendMax   atomic.Uint64
	appendCalls int
	maxBatch    int
	categories  [5]atomic.Uint64
}

// New creates an inert fixture. Start explicitly owns the control listener.
func New() *Store { return &Store{keys: make(map[logstore.RecordKey]struct{}), started: time.Now()} }

type observedHandler struct {
	slog.Handler
	store *Store
}

// Observe counts actual handler entries and admission results without retaining
// message bodies. Derived handlers share counters, preserving context and source.
func (s *Store) Observe(handler slog.Handler) slog.Handler {
	return &observedHandler{Handler: handler, store: s}
}

func (h *observedHandler) Handle(ctx context.Context, record slog.Record) error {
	h.store.entries.Add(1)
	category := 4
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "sdk" && attr.Value.String() == "livekit" {
			category = 1
		}
		return true
	})
	switch {
	case strings.HasPrefix(record.Message, "local log pressure"):
		category = 0
	case strings.HasPrefix(record.Message, "pion."):
		category = 1
	case record.Message == "genx: stream":
		category = 2
	case record.Message == "gizclaw: peer stream lifecycle":
		category = 3
	}
	h.store.categories[category].Add(1)
	start := time.Now()
	err := h.Handler.Handle(ctx, record)
	h.store.handlerNS.Add(uint64(time.Since(start)))
	if err == nil {
		h.store.accepted.Add(1)
	} else {
		h.store.rejected.Add(1)
	}
	return err
}

func (h *observedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &observedHandler{Handler: h.Handler.WithAttrs(attrs), store: h.store}
}

func (h *observedHandler) WithGroup(group string) slog.Handler {
	return &observedHandler{Handler: h.Handler.WithGroup(group), store: h.store}
}

// Log resolves the sole synthetic sink without touching a production registry.
func (s *Store) Log(string) (logstore.ImmutableStore, error) { return s, nil }

// Append honors each production worker deadline and records accepted identities.
func (s *Store) Append(ctx context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	start := time.Now()
	defer func() {
		ns := uint64(time.Since(start))
		s.appendNS.Add(ns)
		for old := s.appendMax.Load(); old < ns; old = s.appendMax.Load() {
			if s.appendMax.CompareAndSwap(old, ns) {
				break
			}
		}
	}()
	s.mu.Lock()
	s.appendCalls++
	s.maxBatch = max(s.maxBatch, len(records))
	gate := s.gate
	if gate != nil && s.entered != nil {
		close(s.entered)
		s.entered = nil
	}
	s.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys)+len(records) > 65536 {
		return nil, errors.New("logpressure: fixture record bound exceeded")
	}
	keys := make([]logstore.RecordKey, len(records))
	for i, record := range records {
		keys[i] = record.Key()
		if _, ok := s.keys[keys[i]]; ok {
			s.duplicates++
		}
		s.keys[keys[i]] = struct{}{}
	}
	s.written += len(records)
	return keys, nil
}

// Query deliberately does not claim backend search acceptance.
func (*Store) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, errors.New("logpressure: search is outside this fixture")
}

// Start serves an internal-only diagnostic endpoint until Close. No ports are
// published by the Compose overlay. pending reads the real logger queue size.
func (s *Store) Start(address string, logger *slog.Logger, pending func() int, capacity int) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	status := func(w http.ResponseWriter, _ *http.Request) {
		queued := pending()
		s.mu.Lock()
		value := map[string]any{
			"written": s.written, "duplicates": s.duplicates, "blocked": s.gate != nil,
			"goroutines": runtime.NumGoroutine(), "queued": queued, "capacity": capacity,
			"elapsed_ms": time.Since(s.started).Milliseconds(), "handler_entries": s.entries.Load(),
			"queue_accepted": s.accepted.Load(), "queue_rejected": s.rejected.Load(), "synthetic_fill_entries": s.fill.Load(),
			"handler_total_ms": float64(s.handlerNS.Load()) / 1e6,
			"append_calls":     s.appendCalls, "append_max_batch": s.maxBatch,
			"append_total_ms": float64(s.appendNS.Load()) / 1e6, "append_max_ms": float64(s.appendMax.Load()) / 1e6,
			"categories": map[string]uint64{"fixture": s.categories[0].Load(), "rtc_sdk": s.categories[1].Load(), "genx": s.categories[2].Load(), "peer_lifecycle": s.categories[3].Load(), "other": s.categories[4].Load()},
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("GET /status", status)
	mux.HandleFunc("POST /block", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if s.gate != nil {
			s.mu.Unlock()
			http.Error(w, "already blocked", http.StatusConflict)
			return
		}
		entered := make(chan struct{})
		s.entered, s.gate = entered, make(chan struct{})
		s.timer = time.AfterFunc(4*time.Second, s.release)
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		s.fill.Add(1)
		logger.InfoContext(ctx, "local log pressure worker gate")
		select {
		case <-entered:
		case <-ctx.Done():
			http.Error(w, "worker did not reach gate", 500)
			return
		}
		for i := 0; pending() < capacity && i < 2*capacity && ctx.Err() == nil; i++ {
			s.fill.Add(1)
			logger.InfoContext(ctx, "local log pressure queue fill", "index", i)
		}
		if pending() != capacity {
			http.Error(w, "queue did not saturate", 500)
			return
		}
		status(w, r)
	})
	mux.HandleFunc("POST /release", func(w http.ResponseWriter, r *http.Request) { s.release(); status(w, r) })
	mux.HandleFunc("GET /profile", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		profile := bytes.Clone(s.profile)
		s.mu.Unlock()
		_, _ = w.Write(profile)
	})
	s.server = &http.Server{Addr: listener.Addr().String(), Handler: mux, ReadHeaderTimeout: time.Second}
	s.done = make(chan struct{})
	go func() { defer close(s.done); _ = s.server.Serve(listener) }()
	return nil
}

func (s *Store) release() {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate == nil {
		return
	}
	// Capture while the worker is still gated, without holding store state.
	var profile bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&profile, 2)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gate != gate {
		return // Another release already completed this gate.
	}
	s.profile = profile.Bytes()
	close(s.gate)
	s.gate = nil
	if s.timer != nil {
		s.timer.Stop()
	}
}

// Close releases the gate before stopping and joining the diagnostic listener.
func (s *Store) Close() error {
	s.release()
	if s.server == nil {
		return nil
	}
	if err := s.server.Close(); err != nil {
		return fmt.Errorf("logpressure: close control: %w", err)
	}
	<-s.done
	return nil
}
