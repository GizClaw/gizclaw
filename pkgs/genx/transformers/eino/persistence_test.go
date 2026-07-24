package eino

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestStateCommitPrecedesPrimaryEOS(t *testing.T) {
	t.Parallel()
	store := &recordingStateStore{
		snapshot: StateSnapshot{
			Version: "version-1",
			Fields:  map[string]any{"answer": "previous", "undeclared": "ignored"},
		},
		compareStarted: make(chan struct{}),
		compareRelease: make(chan struct{}),
	}
	config := textConfig()
	config.State = &StatePersistenceConfig{
		Store: store, Scope: "conversation/one", Fields: []string{"answer"},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("current"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() before data error = %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && string(text) == "current" {
			break
		}
	}
	select {
	case <-store.compareStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("State CompareAndSwap did not start after primary data delivery")
	}
	store.mu.Lock()
	if store.loadScope != "conversation/one" || store.compareScope != "conversation/one" {
		t.Fatalf("State scopes load=%q compare=%q", store.loadScope, store.compareScope)
	}
	if store.compareVersion != "version-1" {
		t.Fatalf("CompareAndSwap version = %q", store.compareVersion)
	}
	if !maps.Equal(store.compareFields, map[string]any{"answer": "current"}) {
		t.Fatalf("CompareAndSwap fields = %#v", store.compareFields)
	}
	store.mu.Unlock()
	close(store.compareRelease)

	chunks := drain(t, output)
	if len(chunks) == 0 || !chunks[len(chunks)-1].IsEndOfStream() {
		t.Fatal("primary EOS was not returned after State commit")
	}
}

func TestStateConflictFailsPrimaryEOS(t *testing.T) {
	t.Parallel()
	store := &recordingStateStore{
		snapshot:   StateSnapshot{Version: "stale", Fields: map[string]any{}},
		compareErr: errors.New("version conflict"),
	}
	config := textConfig()
	config.State = &StatePersistenceConfig{Store: store, Scope: "same", Fields: []string{"answer"}}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("answer"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "answer" {
		t.Fatalf("delivered text = %q", got)
	}
	last := chunks[len(chunks)-1]
	if last.Ctrl == nil || !last.IsEndOfStream() || last.Ctrl.Error == "" {
		t.Fatalf("terminal chunk = %#v, want State conflict error", last)
	}
}

type recordingStateStore struct {
	mu sync.Mutex

	snapshot       StateSnapshot
	loadScope      string
	compareScope   string
	compareVersion string
	compareFields  map[string]any
	compareErr     error
	compareStarted chan struct{}
	compareRelease chan struct{}
}

func (store *recordingStateStore) Load(_ context.Context, scope string) (StateSnapshot, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.loadScope = scope
	return StateSnapshot{
		Version: store.snapshot.Version,
		Fields:  maps.Clone(store.snapshot.Fields),
	}, nil
}

func (store *recordingStateStore) CompareAndSwap(
	ctx context.Context,
	scope string,
	version string,
	fields map[string]any,
) (StateSnapshot, error) {
	store.mu.Lock()
	store.compareScope = scope
	store.compareVersion = version
	store.compareFields = maps.Clone(fields)
	started := store.compareStarted
	release := store.compareRelease
	err := store.compareErr
	store.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return StateSnapshot{}, context.Cause(ctx)
		}
	}
	if err != nil {
		return StateSnapshot{}, err
	}
	return StateSnapshot{Version: version + "-next", Fields: maps.Clone(fields)}, nil
}
