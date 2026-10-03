package peerusage

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ambiguousWriter struct {
	store            *Store
	fail             bool
	entered, release chan struct{}
}

func (w *ambiguousWriter) Maintain(ctx context.Context) error { return w.store.Maintain(ctx) }
func (w *ambiguousWriter) Write(ctx context.Context, items []Snapshot) error {
	if w.entered != nil {
		close(w.entered)
		select {
		case <-w.release:
		case <-ctx.Done():
			return ctx.Err()
		}
		w.entered = nil
	}
	if err := w.store.Write(ctx, items); err != nil {
		return err
	}
	if w.fail {
		w.fail = false
		return errors.New("commit acknowledgement lost")
	}
	return nil
}

func TestRecorderRetriesAmbiguousCommitWithoutDoubleCounting(t *testing.T) {
	s, _ := newTestStore(t)
	writer := &ambiguousWriter{store: s, fail: true}
	r := newRecorder(writer)
	peer := giznet.PublicKey{5}
	hour := time.Now().UTC().Truncate(time.Hour)
	if err := r.Record(peer, "m", genx.UsageRecord{Input: 3, CachedInput: 2, Output: 5}); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(t.Context()); err == nil {
		t.Fatal("expected ambiguous error")
	}
	if err := r.Record(peer, "m", genx.UsageRecord{Input: 2}); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(t.Context(), peer, "m", hour, hour.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Quantity != 12 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestProviderRecordingProgressesWhileDatabaseWriteIsBlocked(t *testing.T) {
	s, _ := newTestStore(t)
	writer := &ambiguousWriter{store: s, entered: make(chan struct{}), release: make(chan struct{})}
	r := newRecorder(writer)
	peer := giznet.PublicKey{6}
	if err := r.Record(peer, "m", genx.UsageRecord{Input: 1}); err != nil {
		t.Fatal(err)
	}
	flushed := make(chan error, 1)
	go func() { flushed <- r.Flush(t.Context()) }()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("write did not enter")
	}
	progress := make(chan error, 1)
	go func() { progress <- r.Record(peer, "m", genx.UsageRecord{Output: 2}) }()
	select {
	case err := <-progress:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider blocked behind database")
	}
	close(writer.release)
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if err := r.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	rows, err := s.Query(t.Context(), peer, "", hour, hour.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Quantity != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if err := r.Record(peer, "m", genx.UsageRecord{Input: 1}); err == nil {
		t.Fatal("closed recorder accepted report")
	}
}

func TestConcurrentRecorderQuantitiesAndRollover(t *testing.T) {
	s, _ := newTestStore(t)
	r := NewRecorder(s)
	peer := giznet.PublicKey{7}
	now := time.Now().UTC().Truncate(time.Hour)
	r.Now = func() time.Time { return now }
	s.Now = r.Now
	var group sync.WaitGroup
	for range 100 {
		group.Go(func() {
			if err := r.Record(peer, "m", genx.UsageRecord{Input: 1, CachedInput: 1, Output: 1}); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if err := r.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if err := r.Record(peer, "m", genx.UsageRecord{Input: 5}); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A clock returning to an acknowledged hour creates a fresh writer epoch;
	// grouped reads still include the earlier persisted contribution.
	now = now.Add(-time.Hour)
	if err := r.Record(peer, "m", genx.UsageRecord{Input: 2}); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(t.Context(), peer, "m", now, now.Add(2*time.Hour))
	if err != nil || len(rows) != 2 || rows[0].Quantity != 302 || rows[1].Quantity != 5 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestRecorderRejectsQuantityOverflow(t *testing.T) {
	s, _ := newTestStore(t)
	r := NewRecorder(s)
	peer := giznet.PublicKey{8}
	if err := r.Record(peer, "m", genx.UsageRecord{Input: math.MaxInt64, Output: 1}); err == nil {
		t.Fatal("overflow accepted")
	}
	if err := r.Record(peer, "m", genx.UsageRecord{Input: -1}); err == nil {
		t.Fatal("negative quantity accepted")
	}
}
