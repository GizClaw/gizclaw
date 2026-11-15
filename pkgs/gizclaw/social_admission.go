package gizclaw

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

// peerMutationAvailability binds a mutation-only availability check. Historical
// Peer absence gets one authority refresh; ordinary Peer queries are unchanged.
func peerMutationAvailability(peers *peer.Server) func(context.Context, string) error {
	return func(ctx context.Context, publicKey string) error {
		key, err := parsePeerPublicKey(publicKey)
		if err != nil {
			return err
		}
		err = peers.EnsureAvailableForMutation(ctx, key)
		if errors.Is(err, peer.ErrPeerPendingDeletion) || errors.Is(err, peer.ErrPeerDeleted) {
			// Social creation may already own a SQL Workspace when availability
			// changes. Classify retirement so its existing compensation runs,
			// while preserving the Peer error for RPC and HTTP projections.
			return errors.Join(err, ownership.ErrOwnerRetired)
		}
		return err
	}
}
