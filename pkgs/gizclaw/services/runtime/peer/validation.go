package peer

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// ValidatePeerRecord checks that the configured reader returns a decodable,
// nondeleted Peer record without resolving registration firmware projections.
func (s *Server) ValidatePeerRecord(ctx context.Context, publicKey giznet.PublicKey) error {
	_, err := s.getStored(ctx, publicKey)
	return err
}
