package peer

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func validatePeer(peer apitypes.Peer) error {
	if key, err := publicKeyFromText(peer.PublicKey); err != nil {
		return err
	} else if key.IsZero() {
		return fmt.Errorf("peer: empty public key")
	}
	if !peer.Role.Valid() {
		return fmt.Errorf("peer: invalid role %q", peer.Role)
	}
	if !peer.Status.Valid() {
		return fmt.Errorf("peer: invalid status %q", peer.Status)
	}
	return validateConfiguration(peer.Configuration)
}

func validateConfiguration(apitypes.Configuration) error {
	return nil
}

func publicKeyFromText(publicKey string) (giznet.PublicKey, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(publicKey)); err != nil {
		return giznet.PublicKey{}, fmt.Errorf("peer: invalid public key: %w", err)
	}
	return key, nil
}
