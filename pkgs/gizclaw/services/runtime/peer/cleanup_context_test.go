package peer

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestChildDeletionPendingAcceptsNonComparablePrimaryContext(t *testing.T) {
	// Context implementations need not be comparable, including decorators of
	// an already authoritative view. A negative child lookup must not panic.
	ctx := struct {
		context.Context
		values []string
	}{Context: kv.WithPrimaryReads(t.Context())}
	pending, err := childDeletionPending(ctx, emptyPeerLookup{}, pendingdeletion.KindFriendGroup, []string{"group"})
	if err != nil || pending {
		t.Fatalf("absent known child = %v, %v", pending, err)
	}
}
