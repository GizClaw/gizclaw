package peerresource

import (
	"fmt"
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

func TestPetIDConflictMapsToStableConflict(t *testing.T) {
	response := gameplayBusinessError("pet-adopt", fmt.Errorf("adopt: %w", gameplay.ErrPetIDConflict))
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeConflict || response.Error.Message != "pet id is already reserved" {
		t.Fatalf("pet id conflict response = %#v, want stable conflict", response)
	}
}

func TestInvalidPetIDMapsToStableBadRequest(t *testing.T) {
	response := gameplayBusinessError("pet-adopt", fmt.Errorf("adopt: %w", gameplay.ErrInvalidPetID))
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest || response.Error.Message != "invalid pet id" {
		t.Fatalf("invalid pet id response = %#v, want stable bad request", response)
	}
}
