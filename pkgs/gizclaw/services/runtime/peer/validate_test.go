package peer

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestValidatePeer(t *testing.T) {
	roleErr := validatePeer(apitypes.Peer{
		PublicKey: giznet.PublicKey{1}.String(),
		Role:      apitypes.PeerRole("bad"),
		Status:    apitypes.PeerRegistrationStatusActive,
	})
	if roleErr == nil {
		t.Fatal("validatePeer should fail on invalid role")
	}

	statusErr := validatePeer(apitypes.Peer{
		PublicKey: giznet.PublicKey{1}.String(),
		Role:      apitypes.PeerRoleServer,
		Status:    apitypes.PeerRegistrationStatus("bad"),
	})
	if statusErr == nil {
		t.Fatal("validatePeer should fail on invalid status")
	}
}
