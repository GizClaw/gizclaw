package peerresource

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
)

func TestBusinessErrorMapsMissingPeerProfileToNotFound(t *testing.T) {
	response := businessError("friend-info", peer.ErrPeerNotFound)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("businessError() = %#v, want not found", response)
	}
}

func TestPetDeadMapsToStableConflict(t *testing.T) {
	response := gameplayBusinessError("pet-drive", gameplay.ErrPetDead)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeConflict || response.Error.Message != "pet is dead" {
		t.Fatalf("pet dead response = %#v, want stable conflict", response)
	}
}
