package peer

import (
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	// PeerIdentifierLimit bounds the distinct SN and IMEI indexes of one Peer.
	PeerIdentifierLimit = 10
	// IdentifierIndexPeerLimit bounds the Peers associated with one SN or IMEI.
	IdentifierIndexPeerLimit = 10
)

var (
	// ErrPeerIdentifierLimit reports more than ten distinct SN/IMEI indexes.
	ErrPeerIdentifierLimit = errors.New("peer: SN and IMEI identifier limit reached")
	// ErrIdentifierIndexPeerLimit reports a full SN or IMEI reverse index.
	ErrIdentifierIndexPeerLimit = errors.New("peer: SN or IMEI index Peer limit reached")
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
	return nil
}

func publicKeyFromText(publicKey string) (giznet.PublicKey, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(publicKey)); err != nil {
		return giznet.PublicKey{}, fmt.Errorf("peer: invalid public key: %w", err)
	}
	return key, nil
}
