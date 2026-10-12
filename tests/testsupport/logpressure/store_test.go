package logpressure

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

func TestStatusCallsQueueReaderOutsideStoreLock(t *testing.T) {
	s := New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pending := func() int {
		if !s.mu.TryLock() {
			return -1
		}
		s.mu.Unlock()
		return 0
	}
	if err := s.Start("127.0.0.1:0", logger, pending, 1024); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + s.server.Addr + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var status struct {
		Queued int `json:"queued"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Queued != 0 {
		t.Fatal("queue callback was called under the store lock")
	}
}

func TestConcurrentReleasePublishesOneGateSnapshot(t *testing.T) {
	s := New()
	s.gate, s.entered = make(chan struct{}), make(chan struct{})
	entered := s.entered
	done := make(chan error, 1)
	go func() { _, err := s.Append(t.Context(), []logstore.Record{{Stream: "system", ID: "one"}}); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Append did not enter gate")
	}
	var releasers sync.WaitGroup
	for range 8 {
		releasers.Go(s.release)
	}
	releasers.Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("release left Append waiting")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gate != nil || len(s.profile) == 0 || s.written != 1 {
		t.Fatalf("gate=%v profile=%d written=%d", s.gate, len(s.profile), s.written)
	}
}

func TestObserverCountsDerivedHandlerAdmissionErrors(t *testing.T) {
	s := New()
	handler, err := logstore.NewSlogHandler(s, "system", "log", slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(s.Observe(handler)).With("node_id", "local").WithGroup("request")
	logger.Info("genx: stream")
	s.gate, s.entered = make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	logger.InfoContext(ctx, "local log pressure worker gate")
	s.release()
	if s.entries.Load() != 2 || s.accepted.Load() != 1 || s.rejected.Load() != 1 || s.categories[0].Load() != 1 || s.categories[2].Load() != 1 {
		t.Fatalf("entries=%d accepted=%d rejected=%d", s.entries.Load(), s.accepted.Load(), s.rejected.Load())
	}
}

func TestGateHonorsDeadlineAndRecoversAcceptedKeys(t *testing.T) {
	s := New()
	s.gate, s.entered = make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	records := []logstore.Record{{Stream: "system", ID: "one"}, {Stream: "system", ID: "two"}}
	if keys, err := s.Append(ctx, records); !errors.Is(err, context.Canceled) || len(keys) != 0 {
		t.Fatalf("canceled append: keys=%v err=%v", keys, err)
	}
	s.release()
	keys, err := s.Append(t.Context(), records)
	if err != nil || len(keys) != 2 || keys[0] != records[0].Key() || keys[1] != records[1].Key() {
		t.Fatalf("recovery: keys=%v err=%v", keys, err)
	}
	if s.written != 2 || s.duplicates != 0 || len(s.profile) == 0 {
		t.Fatalf("invalid recovery receipt: written=%d duplicates=%d profile=%d", s.written, s.duplicates, len(s.profile))
	}
}

func TestCloseReleasesWaitingAppender(t *testing.T) {
	s := New()
	s.gate, s.entered = make(chan struct{}), make(chan struct{})
	entered := s.entered
	done := make(chan error, 1)
	go func() { _, err := s.Append(t.Context(), []logstore.Record{{Stream: "system", ID: "one"}}); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Append did not enter gate")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close left Append blocked")
	}
}
