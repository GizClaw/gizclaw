package runtimeprofile

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type testSource struct {
	mu        sync.Mutex
	profiles  []apitypes.RuntimeProfile
	entered   chan struct{}
	release   chan struct{}
	fail      error
	failAfter int
	calls     int
}

func (source *testSource) ForEachProfile(ctx context.Context, consume func(apitypes.RuntimeProfile) error) error {
	source.mu.Lock()
	source.calls++
	items := append([]apitypes.RuntimeProfile(nil), source.profiles...)
	entered, release, fail, failAfter := source.entered, source.release, source.fail, source.failAfter
	source.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for i, item := range items {
		if err := consume(item); err != nil {
			return err
		}
		if fail != nil && i+1 == failAfter {
			return fail
		}
	}
	return fail
}

func TestIndexCloseStopsRotation(t *testing.T) {
	source := &testSource{profiles: []apitypes.RuntimeProfile{{Id: "profile", Revision: "r1"}}}
	index := New(source, 5*time.Millisecond)
	if err := index.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	calls := source.calls
	source.mu.Unlock()
	time.Sleep(25 * time.Millisecond)
	source.mu.Lock()
	after := source.calls
	source.mu.Unlock()
	if after != calls {
		t.Fatalf("source calls after Close = %d, want %d", after, calls)
	}
	if err := index.Refresh(t.Context()); err == nil {
		t.Fatal("closed index accepted Refresh")
	}
}

func TestIndexCloseCancelsBlockedRotation(t *testing.T) {
	source := &testSource{profiles: []apitypes.RuntimeProfile{{Id: "profile", Revision: "r1"}}}
	index := New(source, 5*time.Millisecond)
	if err := index.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	source.mu.Lock()
	source.entered, source.release = entered, make(chan struct{})
	source.mu.Unlock()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		_ = index.Close()
		t.Fatal("timer did not start a rotation")
	}
	closed := make(chan error, 1)
	go func() { closed <- index.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel the blocked rotation")
	}
}

func TestIndexKeepsOldSnapshotDuringBuildAndFailure(t *testing.T) {
	ctx := t.Context()
	source := &testSource{profiles: []apitypes.RuntimeProfile{{Id: "first", Revision: "r1"}}}
	index := New(source, time.Hour)
	if err := index.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	entered, release := make(chan struct{}), make(chan struct{})
	source.mu.Lock()
	source.profiles = append(source.profiles, apitypes.RuntimeProfile{Id: "second", Revision: "r1"})
	source.entered, source.release = entered, release
	source.mu.Unlock()
	finished := make(chan error, 1)
	go func() { finished <- index.Refresh(ctx) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach the source")
	}
	if ids, err := index.ListProfileIDs(ctx); err != nil || len(ids) != 1 || ids[0] != "first" || index.Generation() != 1 {
		t.Fatalf("old snapshot during build = %#v, %v", ids, err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	source.mu.Lock()
	source.entered, source.release = nil, nil
	source.fail, source.failAfter = errors.New("source failed after first row"), 1
	source.mu.Unlock()
	if err := index.Refresh(ctx); err == nil {
		t.Fatal("partial source failure was accepted")
	}
	if ids, err := index.ListProfileIDs(ctx); err != nil || len(ids) != 2 || index.Generation() != 2 {
		t.Fatalf("published snapshot changed after failure = %#v, %v", ids, err)
	}
}

func TestIndexRotatesReadOnlySQLiteInstances(t *testing.T) {
	ctx := t.Context()
	source := &testSource{profiles: []apitypes.RuntimeProfile{{
		Id: "first", Revision: "r1", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			"chat": {ResourceId: "chat", Tags: &[]string{"6-8", "stories"}},
		}},
	}}}
	index := New(source, time.Hour)
	if err := index.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	previous := index.memoryIndex
	if _, err := previous.ExecContext(ctx, `DELETE FROM profiles`); err == nil {
		t.Fatal("published SQLite instance is writable")
	}
	source.mu.Lock()
	source.profiles = append(source.profiles, apitypes.RuntimeProfile{Id: "second", Revision: "r1"})
	source.mu.Unlock()
	if err := index.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if index.memoryIndex == previous || index.Generation() != 2 {
		t.Fatal("refresh did not publish a new SQLite instance")
	}
	if _, err := previous.QueryContext(ctx, `SELECT runtime_profile_id FROM profiles`); err == nil {
		t.Fatal("previous SQLite instance remains open")
	}
	if _, err := index.memoryIndex.ExecContext(ctx, `DELETE FROM profiles`); err == nil {
		t.Fatal("new SQLite instance is writable")
	}
	entries, err := index.ListProfileWorkflowsByTags(ctx, "first", "r1", []string{"6-8", "stories"})
	if err != nil || len(entries) != 1 || entries[0].Name != "chat" {
		t.Fatalf("AND query = %#v, %v", entries, err)
	}
	if duplicate, err := index.ListWorkflowsByTags(ctx, []string{"stories", "stories"}); err != nil || len(duplicate) != 1 {
		t.Fatalf("duplicate selector = %#v, %v", duplicate, err)
	}
	if _, err := index.GetEntry(ctx, "first", "workflow", "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing entry error = %v", err)
	}
	for _, tags := range [][]string{{""}, {strings.Repeat("x", 129)}, make([]string, 33)} {
		if _, err := index.ListWorkflowsByTags(ctx, tags); err == nil {
			t.Fatalf("invalid selector %#v was accepted", tags)
		}
	}
}
