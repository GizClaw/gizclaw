package gizclaw

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
)

// peerMutationAvailability binds a mutation-only availability check. Historical
// Peer absence gets one authority refresh; ordinary Peer queries are unchanged.
func peerMutationAvailability(peers *peer.Server) func(context.Context, string) error {
	return func(ctx context.Context, publicKey string) error {
		key, err := parsePeerPublicKey(publicKey)
		if err != nil {
			return err
		}
		return peers.EnsureAvailableForMutation(ctx, key)
	}
}
