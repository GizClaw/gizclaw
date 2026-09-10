package peer

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// PublicProfile is the part of a Peer's DeviceInfo that any registered Peer
// may read: the display name and emoji the Peer chose for itself. It never
// carries runtime, hardware, identifier, status or location data.
type PublicProfile struct {
	DisplayName *string
	Emoji       *string
}

// GetPublicProfile returns the public profile of publicKey. A Peer that does
// not exist, is deleted, or is pending deletion returns ErrPeerNotFound, so
// callers cannot tell those states apart.
func (s *Server) GetPublicProfile(ctx context.Context, publicKey giznet.PublicKey) (PublicProfile, error) {
	store, err := s.store()
	if err != nil {
		return PublicProfile{}, err
	}
	peer, err := s.getByPublicKeyText(ctx, store, publicKey.String())
	if errors.Is(err, ErrPeerDeleted) {
		return PublicProfile{}, ErrPeerNotFound
	}
	if err != nil {
		return PublicProfile{}, err
	}
	pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindPeer, publicKey.String())
	if err != nil {
		return PublicProfile{}, err
	}
	if pending {
		return PublicProfile{}, ErrPeerNotFound
	}
	return PublicProfile{
		DisplayName: nonEmptyProfileField(peer.Device.Name),
		Emoji:       nonEmptyProfileField(peer.Device.Emoji),
	}, nil
}

func nonEmptyProfileField(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return new(*value)
}
