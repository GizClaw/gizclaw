package socialutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// PingWindow is how long one friend pair or one Friend Group waits between
// pings. It is fixed, like the social limits, and not configurable.
const PingWindow = time.Minute

var (
	FriendPingWindowsRoot = kv.Key{"friend-ping-windows"}
	GroupPingWindowsRoot  = kv.Key{"friend-group-ping-windows"}
)

// FriendPingWindowKey names the ping window shared by both sides of one
// Friend relationship.
func FriendPingWindowKey(relationID string) kv.Key {
	return append(append(kv.Key{}, FriendPingWindowsRoot...), EscapeStoreSegment(relationID))
}

// GroupPingWindowKey names the rally window shared by every member of one
// Friend Group.
func GroupPingWindowKey(friendGroupID string) kv.Key {
	return append(append(kv.Key{}, GroupPingWindowsRoot...), EscapeStoreSegment(friendGroupID))
}

// pingWindowRecord is the value of one open ping window. Nonce makes each
// claim distinct so a release can only delete the claim that made it.
type pingWindowRecord struct {
	ExpiresAt time.Time `json:"expires_at"`
	Nonce     string    `json:"nonce"`
}

// PingWindowClaim is one successfully opened ping window.
type PingWindowClaim struct {
	key   kv.Key
	value []byte
}

// PingWindowRemaining reports how long the window at key stays closed after
// now, or zero when no window is open.
func PingWindowRemaining(ctx context.Context, store kv.Store, key kv.Key, now time.Time) (time.Duration, error) {
	data, err := store.Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return pingWindowRemaining(data, now)
}

// ClaimPingWindow atomically opens the window at key for PingWindow. The
// window lives in the shared Social KV with a store deadline, so every Server
// sees it and the backend removes it when it closes. When another ping already
// holds the window it returns claimed=false and the time left.
func ClaimPingWindow(ctx context.Context, store kv.Store, key kv.Key, now time.Time) (PingWindowClaim, time.Duration, bool, error) {
	expiresAt := now.Add(PingWindow)
	value, err := json.Marshal(pingWindowRecord{ExpiresAt: expiresAt, Nonce: NewID()})
	if err != nil {
		return PingWindowClaim{}, 0, false, err
	}
	existing, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: key, Value: value, Deadline: expiresAt}, nil)
	if err != nil {
		return PingWindowClaim{}, 0, false, err
	}
	if !created {
		remaining, err := pingWindowRemaining(existing, now)
		if err != nil {
			return PingWindowClaim{}, 0, false, err
		}
		// A window the store still returns is still closed, even when the
		// clocks disagree about the last fraction of a second.
		return PingWindowClaim{}, max(remaining, time.Second), false, nil
	}
	return PingWindowClaim{key: key, value: value}, 0, true, nil
}

// ReleasePingWindow closes a window that delivered nothing, so an offline
// target does not cost the caller a minute. It deletes the window only while
// it still holds this claim.
func ReleasePingWindow(ctx context.Context, store kv.Store, claim PingWindowClaim) error {
	if len(claim.key) == 0 {
		return nil
	}
	_, err := kv.CompareAndMutate(ctx, store, claim.key, claim.value, nil, []kv.Key{claim.key})
	return err
}

func pingWindowRemaining(data []byte, now time.Time) (time.Duration, error) {
	var record pingWindowRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return 0, fmt.Errorf("social: decode ping window: %w", err)
	}
	if record.ExpiresAt.IsZero() {
		return 0, errors.New("social: ping window has no expiry")
	}
	return max(record.ExpiresAt.Sub(now), 0), nil
}

// RetryAfterSeconds rounds a closed window up to whole seconds, never below 1.
func RetryAfterSeconds(remaining time.Duration) *int32 {
	seconds := int32((remaining + time.Second - 1) / time.Second)
	return new(max(seconds, 1))
}

// PingTarget is one device a ping or rally is pushed to.
type PingTarget struct {
	PeerPublicKey string
	Request       rpcapi.ClientSocialPingRequest
}

// DeliverPings pushes every target concurrently and returns how many devices
// acknowledged. deliver owns its own timeout; a failed push is not retried.
func DeliverPings(ctx context.Context, delivery PingDelivery, targets []PingTarget) int {
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		delivered int
	)
	for _, target := range targets {
		wg.Go(func() {
			if err := delivery.DeliverSocialPing(ctx, target.PeerPublicKey, target.Request); err != nil {
				return
			}
			mu.Lock()
			delivered++
			mu.Unlock()
		})
	}
	wg.Wait()
	return delivered
}

// PingDisplayName returns the sender's self-chosen name, or nil when unset.
func PingDisplayName(name *string) *string {
	if name == nil || strings.TrimSpace(*name) == "" {
		return nil
	}
	return new(*name)
}

// PingDelivery reaches the devices connected to this Server. Online reads the
// same connection state as Runtime.online; Deliver pushes one
// client.social.ping and returns once the device acknowledged it.
type PingDelivery interface {
	PeerOnline(peerPublicKey string) bool
	DeliverSocialPing(ctx context.Context, peerPublicKey string, request rpcapi.ClientSocialPingRequest) error
}
