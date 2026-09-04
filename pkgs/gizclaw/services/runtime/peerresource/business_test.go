package peerresource

import (
	"errors"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
)

func TestBusinessErrorMapsMissingPeerProfileToNotFound(t *testing.T) {
	response := businessError("friend-info", peer.ErrPeerNotFound)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("businessError() = %#v, want not found", response)
	}
}

func TestBusinessErrorMapsCrossServerSocialConflicts(t *testing.T) {
	for _, err := range []error{friend.ErrCrossServerFriendCreation, friendgroup.ErrCrossServerFriendGroupMembership} {
		response := businessError("social", fmt.Errorf("wrapped: %w", err))
		if response.Error == nil || response.Error.Code != rpcapi.StatusCodeFailedPrecondition || response.Error.Message != err.Error() {
			t.Fatalf("businessError(%v) = %#v", err, response)
		}
	}
}

func TestBusinessErrorMapsFriendInviteTokenErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    rpcapi.StatusCode
		message string
	}{
		{name: "required", err: fmt.Errorf("domain: %w", friend.ErrInviteTokenRequired), want: rpcapi.StatusCodeInvalidArgument, message: "friend invite token is required"},
		{name: "unavailable", err: fmt.Errorf("domain: %w", friend.ErrInviteTokenUnavailable), want: rpcapi.StatusCodeNotFound, message: "friend invite token not found"},
		{name: "self owned", err: fmt.Errorf("domain: %w", friend.ErrInviteTokenSelfOwned), want: rpcapi.StatusCodeInvalidArgument, message: "cannot add self as friend"},
		{name: "lookup failed", err: fmt.Errorf("lookup: %w: %w", friend.ErrInviteTokenLookupFailed, errors.New("secret backend detail")), want: rpcapi.StatusCodeInternal, message: "friend invite lookup failed"},
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
	if response.Id != "generic" || response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal || response.Error.Message != "generic failure" {
		t.Fatalf("businessError() = %#v, want unchanged generic internal error", response)
	}
}

func TestPetDeadMapsToStableFailedPrecondition(t *testing.T) {
	response := gameplayBusinessError("pet-drive", gameplay.ErrPetDead)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeFailedPrecondition || response.Error.Message != "pet is dead" {
		t.Fatalf("pet dead response = %#v, want stable failed precondition", response)
	}
}

func TestPetIDConflictMapsToStableAlreadyExists(t *testing.T) {
	response := gameplayBusinessError("pet-adopt", fmt.Errorf("adopt: %w", gameplay.ErrPetIDConflict))
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAlreadyExists || response.Error.Message != "pet id is already reserved" {
		t.Fatalf("pet id conflict response = %#v, want stable already-exists", response)
	}
}

func TestInvalidPetIDMapsToStableBadRequest(t *testing.T) {
	response := gameplayBusinessError("pet-adopt", fmt.Errorf("adopt: %w", gameplay.ErrInvalidPetID))
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument || response.Error.Message != "invalid pet id" {
		t.Fatalf("invalid pet id response = %#v, want stable bad request", response)
	}
}
