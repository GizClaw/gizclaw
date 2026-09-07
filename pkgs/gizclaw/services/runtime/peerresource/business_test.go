package peerresource

import (
	"errors"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
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

func TestBusinessErrorMapsSharedWriteConflictsToAborted(t *testing.T) {
	for _, test := range []struct {
		err    error
		reason string
	}{{friendgroup.ErrGroupChanged, "FRIEND_GROUP_CHANGED"}, {peer.ErrPeerConcurrentUpdate, "PEER_CHANGED"}} {
		response := businessError("update", fmt.Errorf("wrapped: %w", test.err))
		if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted || response.Error.Reason != test.reason {
			t.Fatalf("conflict response=%+v", response)
		}
	}
}

func TestBusinessErrorMapsFriendGroupFullToResourceExhausted(t *testing.T) {
	response := businessError("social", fmt.Errorf("wrapped: %w", friendgroup.ErrFriendGroupFull))
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeResourceExhausted ||
		response.Error.Reason != "FRIEND_GROUP_FULL" || response.Error.Message != "friend group is full" {
		t.Fatalf("businessError(full) = %#v", response)
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
