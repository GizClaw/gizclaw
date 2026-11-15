package socialutil

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// PeerRetirementFenceKey addresses the permanent admission fence in the owning
// Social store's namespace. Public keys are encoded as a single safe segment.
func PeerRetirementFenceKey(owner string) kv.Key {
	return kv.Key{"peer-retirement-fences", EscapeStoreSegment(owner)}
}

// PeerRetirementCondition requires that the owner's local-domain retirement
// fence is absent at the same atomic boundary as resource admission.
func PeerRetirementCondition(owner string) kv.Condition {
	return kv.Condition{Key: PeerRetirementFenceKey(owner)}
}

// FencePeerRetirement permanently closes resource admission for owner and
// returns a context whose reader has observed that barrier. Ordinary reads are
// retained when the configured reader sees the committed fence. A rejected
// mutation or a known postwrite mismatch permits a read from the authority.
func FencePeerRetirement(ctx context.Context, store kv.Store, owner string) (context.Context, error) {
	if store == nil || owner == "" || owner != strings.TrimSpace(owner) {
		return ctx, errors.New("social: retirement store and canonical Peer owner are required")
	}
	key := PeerRetirementFenceKey(owner)
	value := []byte(owner)
	applied, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{PeerRetirementCondition(owner)},
		Entries:    []kv.Entry{{Key: key, Value: value}},
	})
	if err != nil {
		return ctx, err
	}
	if !applied {
		ctx = kv.WithPrimaryReads(ctx)
	}
	observed, err := store.Get(ctx, key)
	if applied && (errors.Is(err, kv.ErrNotFound) || err == nil && !bytes.Equal(observed, value)) {
		ctx = kv.WithPrimaryReads(ctx)
		observed, err = store.Get(ctx, key)
	}
	if err != nil {
		return ctx, err
	}
	if !bytes.Equal(observed, value) {
		return ctx, errors.New("social: Peer retirement fence identity mismatch")
	}
	return ctx, nil
}
