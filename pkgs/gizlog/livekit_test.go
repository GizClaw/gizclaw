package gizlog

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

type admissionObserver struct {
	slog.Handler
	rejected *atomic.Int32
}

func (h *admissionObserver) Handle(ctx context.Context, record slog.Record) error {
	err := h.Handler.Handle(ctx, record)
	if errors.Is(err, ErrStoreQueueFull) {
		h.rejected.Add(1)
	}
	return err
}

func (h *admissionObserver) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &admissionObserver{Handler: h.Handler.WithAttrs(attrs), rejected: h.rejected}
}

func (h *admissionObserver) WithGroup(group string) slog.Handler {
	return &admissionObserver{Handler: h.Handler.WithGroup(group), rejected: h.rejected}
}

// The real SDK logs connection-state changes while holding its own mutex.
// SetDefault routes standard log through this queue, under log.outMu.
func TestFullStoreQueueDoesNotBlockLiveKitDisconnect(t *testing.T) {
	room := lksdk.NewRoom(nil)
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	close(store.release)
	q := newStoreQueue(slog.NewTextHandler(io.Discard, nil))
	handler, err := logstore.NewSlogHandler(queuedStoreAppender{queue: q, store: store, name: "logs"}, "system", "log", slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	var rejected atomic.Int32
	logger := slog.New(&admissionObserver{Handler: handler, rejected: &rejected})
	previous := slog.Default()
	slog.SetDefault(logger)
	var start sync.Once
	startWorkers := func() { start.Do(func() { go q.run() }) }
	t.Cleanup(func() { startWorkers(); _ = q.close(); slog.SetDefault(previous) })
	// Keep the pool stopped so saturation is deterministic with any worker count.
	for range storeQueueCapacity {
		slog.Info("fill queue")
	}
	done := make(chan struct{})
	go func() { room.Disconnect(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		startWorkers()
		<-done
		t.Fatal("LiveKit Disconnect waited for Store capacity")
	}
	if rejected.Load() == 0 {
		t.Fatal("LiveKit Disconnect did not emit a log into the full queue")
	}
	before := rejected.Load()
	standardDone := make(chan struct{})
	go func() { log.Print("unrelated standard log caller"); close(standardDone) }()
	select {
	case <-standardDone:
	case <-time.After(500 * time.Millisecond):
		startWorkers()
		<-standardDone
		t.Fatal("standard log waited for Store capacity")
	}
	if rejected.Load() <= before {
		t.Fatal("standard log did not reach the full queue")
	}
	startWorkers()
	if err := q.close(); !errors.Is(err, ErrStoreQueueFull) {
		t.Fatalf("cleanup did not report rejected SDK logs: %v", err)
	}
}
