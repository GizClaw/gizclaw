package socialutil

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPingWindowClaimReleaseAndExpiry(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	key := FriendPingWindowKey("a:b")
	now := time.Now().UTC()

	if remaining, err := PingWindowRemaining(ctx, store, key, now); err != nil || remaining != 0 {
		t.Fatalf("remaining before claim = %v, %v", remaining, err)
	}
	claim, _, claimed, err := ClaimPingWindow(ctx, store, key, now)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	later := now.Add(20 * time.Second)
	if _, remaining, claimed, err := ClaimPingWindow(ctx, store, key, later); err != nil || claimed || remaining != 40*time.Second {
		t.Fatalf("second claim = %v, %v, %v", claimed, remaining, err)
	}
	if remaining, err := PingWindowRemaining(ctx, store, key, later); err != nil || remaining != 40*time.Second {
		t.Fatalf("remaining = %v, %v", remaining, err)
	}

	// Releasing a claim that no longer holds the window leaves the newer one.
	if err := ReleasePingWindow(ctx, store, claim); err != nil {
		t.Fatal(err)
	}
	if remaining, _ := PingWindowRemaining(ctx, store, key, later); remaining != 0 {
		t.Fatalf("release left the window open for %v", remaining)
	}
	if _, _, claimed, err := ClaimPingWindow(ctx, store, key, later); err != nil || !claimed {
		t.Fatalf("claim after release = %v, %v", claimed, err)
	}
	if err := ReleasePingWindow(ctx, store, claim); err != nil {
		t.Fatal(err)
	}
	if remaining, _ := PingWindowRemaining(ctx, store, key, later); remaining != PingWindow {
		t.Fatalf("a stale release closed the newer window: remaining = %v", remaining)
	}

	// The store deadline removes the window once it ends.
	short := GroupPingWindowKey("group")
	if err := store.BatchSet(ctx, []kv.Entry{{Key: short, Value: []byte(`{"expires_at":"2000-01-01T00:00:00Z","nonce":"x"}`), Deadline: time.Now().Add(50 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, _, claimed, err := ClaimPingWindow(ctx, store, short, time.Now()); err != nil || !claimed {
		t.Fatalf("claim after the store expired the window = %v, %v", claimed, err)
	}
}

func TestRetryAfterSecondsRoundsUp(t *testing.T) {
	for remaining, want := range map[time.Duration]int32{
		0: 1, time.Millisecond: 1, time.Second: 1, time.Second + time.Millisecond: 2, 59*time.Second + 999*time.Millisecond: 60,
	} {
		if got := *RetryAfterSeconds(remaining); got != want {
			t.Fatalf("RetryAfterSeconds(%v) = %d, want %d", remaining, got, want)
		}
	}
}
