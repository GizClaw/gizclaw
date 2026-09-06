package kv_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestMemoryBatchSetDeadlineExpires(t *testing.T) {
	ctx := context.Background()
	s := kv.NewMemory(nil)

	if err := s.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"sessions", "expired"}, Value: []byte("gone"), Deadline: time.Now().Add(20 * time.Millisecond)},
		{Key: kv.Key{"sessions", "kept"}, Value: []byte("kept")},
	}); err != nil {
		t.Fatalf("BatchSet deadline entry: %v", err)
	}
	got, err := s.Get(ctx, kv.Key{"sessions", "expired"})
	if err != nil {
		t.Fatalf("Get before expiration: %v", err)
	}
	if string(got) != "gone" {
		t.Fatalf("Get before expiration = %q, want gone", got)
	}

	time.Sleep(30 * time.Millisecond)
	_, err = s.Get(ctx, kv.Key{"sessions", "expired"})
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Get after expiration err = %v, want ErrNotFound", err)
	}

	if value, err := s.Get(ctx, kv.Key{"sessions", "kept"}); err != nil || string(value) != "kept" {
		t.Fatalf("unexpired value = %q, %v", value, err)
	}
}

func TestMemoryBatchSetRejectsExpiredDeadline(t *testing.T) {
	err := kv.NewMemory(nil).BatchSet(context.Background(), []kv.Entry{
		{Key: kv.Key{"session"}, Value: []byte("value"), Deadline: time.Now().Add(-time.Second)},
	})
	if !errors.Is(err, kv.ErrInvalidDeadline) {
		t.Fatalf("BatchSet expired deadline err = %v, want ErrInvalidDeadline", err)
	}
}

func TestMemoryBatchSetRejectsExpiredDeadlineAtomically(t *testing.T) {
	ctx := context.Background()
	s := kv.NewMemory(nil)

	err := s.BatchSet(ctx, []kv.Entry{
		{Key: kv.Key{"sessions", "valid"}, Value: []byte("value"), Deadline: time.Now().Add(time.Minute)},
		{Key: kv.Key{"sessions", "expired"}, Value: []byte("value"), Deadline: time.Now().Add(-time.Second)},
	})
	if !errors.Is(err, kv.ErrInvalidDeadline) {
		t.Fatalf("BatchSet mixed deadline err = %v, want ErrInvalidDeadline", err)
	}
	if _, err := s.Get(ctx, kv.Key{"sessions", "valid"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("BatchSet wrote partial entry, err = %v", err)
	}
}
