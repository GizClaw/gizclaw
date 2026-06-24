package peer

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

const legacyClientPeerRole = apitypes.PeerRole("gear")

// Migration updates persisted peer records from older schema values.
func (s *Server) Migration(ctx context.Context) error {
	peers, err := s.list(ctx)
	if err != nil {
		return err
	}
	for _, item := range peers {
		if item.Role != legacyClientPeerRole {
			continue
		}
		item.Role = apitypes.PeerRoleClient
		if _, err := s.SavePeer(ctx, item); err != nil {
			return fmt.Errorf("peer migration: legacy client role %s: %w", item.PublicKey, err)
		}
	}
	return nil
}
