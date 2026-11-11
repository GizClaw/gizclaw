package gizclaw

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
)

type missingAdmissionPeerReader struct {
	kv.Store
	missing map[string]bool
	failure error
}

func (s missingAdmissionPeerReader) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if len(key) == 2 && key[0] == "by-pubkey" && s.missing[key[1]] {
		if s.failure != nil {
			return nil, s.failure
		}
		return nil, kv.ErrNotFound
	}
	return s.Store.Get(ctx, key)
}

func TestFriendCreationRefreshesIndependentMissingPeerInputs(t *testing.T) {
	for _, missingNames := range [][]string{{"owner"}, {"member"}, {"owner", "member"}} {
		t.Run(strings.Join(missingNames, "_and_")+"_missing", func(t *testing.T) {
			ctx := t.Context()
			f := newSocialHTTPFixture(t)
			authority := f.peers.Store
			peers := map[string]socialHTTPPeer{"owner": f.a, "member": f.b}
			missing := make(map[string]bool, len(missingNames))
			for _, name := range missingNames {
				missing[peers[name].key.String()] = true
			}
			var ordinary, primary atomic.Int32
			f.peers.Store = kvfixture.ReadView(authority, missingAdmissionPeerReader{Store: authority, missing: missing}, &ordinary, &primary)
			if err := f.peers.EnsureAvailable(ctx, peers[missingNames[0]].key); !errors.Is(err, peer.ErrPeerNotFound) || primary.Load() != 0 {
				t.Fatalf("ordinary query refreshed missing input: %v primary=%d", err, primary.Load())
			}
			if _, err := f.public.Friends.AdminCreateFriend(ctx, f.a.key.String(), f.b.key.String()); err != nil {
				t.Fatalf("Friend mutation rejected a committed Peer: %v", err)
			}
			if primary.Load() == 0 || primary.Load() > int32(8*len(missingNames)) {
				t.Fatalf("unbounded or absent mutation refresh: %d", primary.Load())
			}
			if _, err := f.public.Friends.GetFriendRelation(ctx, f.a.key.String(), f.b.key.String()); err != nil {
				t.Fatalf("admitted Friend absent: %v", err)
			}
		})
	}
}

func TestExistingGroupJoinRefreshesIndependentMissingPeerInputs(t *testing.T) {
	for _, test := range []struct {
		name       string
		missing    []string
		writer     string
		readerFail bool
	}{
		{name: "owner_missing", missing: []string{"owner"}},
		{name: "member_missing", missing: []string{"member"}},
		{name: "both_missing", missing: []string{"owner", "member"}},
		{name: "writer_absent", missing: []string{"owner"}, writer: "absent"},
		{name: "writer_retiring", missing: []string{"owner"}, writer: "retiring"},
		{name: "reader_failure", missing: []string{"owner"}, readerFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			f := newSocialHTTPFixture(t)
			groups := f.public.FriendGroups
			if _, err := groups.CreateFriendGroup(ctx, f.a.key.String(), rpcapi.FriendGroupCreateRequest{Name: "owner-room"}); err != nil {
				t.Fatal(err)
			}
			invitation, err := groups.CreateFriendGroupInviteToken(ctx, f.a.key.String(), rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: "owner-room"})
			if err != nil || invitation.InviteToken == "" {
				t.Fatalf("invitation=%+v, %v", invitation, err)
			}
			request := rpcapi.FriendGroupJoinRequest{Name: "member-room", InviteToken: invitation.InviteToken}
			existing, err := groups.JoinFriendGroup(ctx, f.b.key.String(), request)
			if err != nil {
				t.Fatal(err)
			}
			authority := f.peers.Store
			switch test.writer {
			case "absent":
				if err := authority.Delete(ctx, kv.Key{"by-pubkey", f.a.key.String()}); err != nil {
					t.Fatal(err)
				}
			case "retiring":
				if err := f.peers.DeleteSelf(kv.WithPrimaryReads(ctx), f.a.key); err != nil {
					t.Fatal(err)
				}
			}
			peers := map[string]socialHTTPPeer{"owner": f.a, "member": f.b}
			missing := make(map[string]bool, len(test.missing))
			for _, name := range test.missing {
				missing[peers[name].key.String()] = true
			}
			readFailure := errors.New("Peer reader unavailable")
			reader := missingAdmissionPeerReader{Store: authority, missing: missing}
			if test.readerFail {
				reader.failure = readFailure
			}
			var ordinary, primary atomic.Int32
			f.peers.Store = kvfixture.ReadView(authority, reader, &ordinary, &primary)
			if err := f.peers.EnsureAvailable(ctx, peers[test.missing[0]].key); err == nil || primary.Load() != 0 {
				t.Fatalf("ordinary query refreshed a missing Peer: %v, primary=%d", err, primary.Load())
			}
			rejoined, err := groups.JoinFriendGroup(ctx, f.b.key.String(), request)
			if test.readerFail {
				if !errors.Is(err, readFailure) || primary.Load() != 0 {
					t.Fatalf("unavailable reader was hidden: %v, primary=%d", err, primary.Load())
				}
				return
			}
			if primary.Load() == 0 || primary.Load() > int32(4*len(test.missing)) {
				t.Fatalf("unbounded or absent input refresh: ordinary=%d primary=%d", ordinary.Load(), primary.Load())
			}
			switch test.writer {
			case "absent":
				if !errors.Is(err, peer.ErrPeerNotFound) {
					t.Fatalf("unknown authority owner accepted: %v", err)
				}
			case "retiring":
				if !errors.Is(err, peer.ErrPeerPendingDeletion) {
					t.Fatalf("retiring authority owner accepted: %v", err)
				}
			default:
				if err != nil || !reflect.DeepEqual(rejoined.Member, existing.Member) {
					t.Fatalf("valid existing join changed membership: %+v, %v; want %+v", rejoined, err, existing.Member)
				}
			}
		})
	}
}
