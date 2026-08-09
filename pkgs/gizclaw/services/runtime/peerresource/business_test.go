package peerresource

import (
	"errors"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
)

func TestBusinessErrorMapsMissingPeerProfileToNotFound(t *testing.T) {
	response := businessError("friend-info", peer.ErrPeerNotFound)
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("businessError() = %#v, want not found", response)
	}
}

func TestBusinessErrorMapsFriendInviteTokenErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    rpcapi.RPCErrorCode
		message string
	}{
		{name: "required", err: fmt.Errorf("domain: %w", friend.ErrInviteTokenRequired), want: rpcapi.RPCErrorCodeBadRequest, message: "friend invite token is required"},
		{name: "unavailable", err: fmt.Errorf("domain: %w", friend.ErrInviteTokenUnavailable), want: rpcapi.RPCErrorCodeNotFound, message: "friend invite token not found"},
		{name: "self owned", err: fmt.Errorf("domain: %w", friend.ErrInviteTokenSelfOwned), want: rpcapi.RPCErrorCodeConflict, message: "cannot add self as friend"},
		{name: "lookup failed", err: fmt.Errorf("lookup: %w: %w", friend.ErrInviteTokenLookupFailed, errors.New("secret backend detail")), want: rpcapi.RPCErrorCodeInternalError, message: "friend invite lookup failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := businessError("friend.add.case", test.err)
			if response.Id != "friend.add.case" || response.Error == nil || response.Error.Code != test.want || response.Error.Message != test.message {
				t.Fatalf("businessError() = %#v, want id/code/message friend.add.case/%d/%q", response, test.want, test.message)
			}
		})
	}
}

func TestBusinessErrorRetainsGenericInternalFallback(t *testing.T) {
	response := businessError("generic", errors.New("generic failure"))
	if response.Id != "generic" || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError || response.Error.Message != "generic failure" {
		t.Fatalf("businessError() = %#v, want unchanged generic internal error", response)
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
