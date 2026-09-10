package peerresource

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type publicProfileStub struct {
	profiles map[giznet.PublicKey]peer.PublicProfile
	failing  giznet.PublicKey
	calls    int
}

func (s *publicProfileStub) GetPublicProfile(_ context.Context, key giznet.PublicKey) (peer.PublicProfile, error) {
	s.calls++
	if key == s.failing {
		return peer.PublicProfile{}, errors.New("store down")
	}
	profile, ok := s.profiles[key]
	if !ok {
		return peer.PublicProfile{}, peer.ErrPeerNotFound
	}
	return profile, nil
}

func profileGet(t *testing.T, server *Server, keys []string) *rpcapi.RPCResponse {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromProfileGetRequest(rpcapi.ProfileGetRequest{PeerPublicKeys: keys}); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(t.Context(), &rpcapi.RPCRequest{Id: "profile", Method: rpcapi.RPCMethodServerProfileGet, Params: &params})
	if err != nil || !handled {
		t.Fatalf("Dispatch(profile.get) handled=%v err=%v", handled, err)
	}
	return response
}

func TestProfileGetReturnsOnlyPublicFieldsForAnyPeerInRequestOrder(t *testing.T) {
	alice, bob, unknown := giznet.PublicKey{1}, giznet.PublicKey{2}, giznet.PublicKey{3}
	name, emoji := "Bob", "🐻"
	profiles := &publicProfileStub{profiles: map[giznet.PublicKey]peer.PublicProfile{
		alice: {},
		bob:   {DisplayName: &name, Emoji: &emoji},
	}}
	server := &Server{Caller: giznet.PublicKey{9}, Profiles: profiles}

	response := profileGet(t, server, []string{bob.String(), unknown.String(), alice.String(), bob.String()})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("profile.get response = %#v", response)
	}
	got, err := response.Result.AsProfileGetResponse()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 3 || profiles.calls != 3 {
		t.Fatalf("items = %+v after %d lookups, want one per distinct key", got.Items, profiles.calls)
	}
	if item := got.Items[0]; item.PeerPublicKey != bob.String() || *item.DisplayName != name || *item.Emoji != emoji {
		t.Fatalf("bob = %+v", item)
	}
	for i, key := range []giznet.PublicKey{unknown, alice} {
		if item := got.Items[i+1]; item.PeerPublicKey != key.String() || item.DisplayName != nil || item.Emoji != nil {
			t.Fatalf("item %d = %+v, want the key alone", i+1, item)
		}
	}
}

func TestProfileGetValidatesTheBatch(t *testing.T) {
	server := &Server{Profiles: &publicProfileStub{}}
	tooMany := make([]string, rpcapi.MaxProfileGetKeys+1)
	for i := range tooMany {
		tooMany[i] = giznet.PublicKey{byte(i + 1)}.String()
	}
	for name, keys := range map[string][]string{
		"empty":         nil,
		"too many":      tooMany,
		"not a key":     {"alice"},
		"padded key":    {" " + giznet.PublicKey{1}.String()},
		"zero key":      {giznet.PublicKey{}.String()},
		"one bad entry": {giznet.PublicKey{1}.String(), "bad"},
	} {
		if response := profileGet(t, server, keys); response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument {
			t.Fatalf("%s: response = %#v, want INVALID_ARGUMENT", name, response)
		}
	}
	full := profileGet(t, server, tooMany[:rpcapi.MaxProfileGetKeys])
	if full.Error != nil {
		t.Fatalf("a full batch of %d keys failed: %#v", rpcapi.MaxProfileGetKeys, full.Error)
	}
}

func TestProfileGetHidesStoreErrors(t *testing.T) {
	failing := giznet.PublicKey{4}
	server := &Server{Profiles: &publicProfileStub{failing: failing}}
	response := profileGet(t, server, []string{failing.String()})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal || response.Error.Message != "profile lookup failed" {
		t.Fatalf("response = %#v", response)
	}
}

func TestSocialPingDispatchValidatesNamesAndMapsErrors(t *testing.T) {
	server := &Server{
		Caller:       giznet.PublicKey{1},
		Friends:      &friend.Server{Friends: kv.NewMemory(nil)},
		FriendGroups: &friendgroup.Server{},
	}
	for _, method := range []rpcapi.RPCMethod{rpcapi.RPCMethodServerFriendPing, rpcapi.RPCMethodServerFriendGroupPing} {
		if !IsMethod(method) {
			t.Fatalf("%s is not dispatched", method)
		}
		for _, name := range []string{"", " padded"} {
			var params rpcapi.RPCPayload
			var err error
			if method == rpcapi.RPCMethodServerFriendPing {
				err = params.FromFriendPingRequest(rpcapi.FriendPingRequest{Name: name})
			} else {
				err = params.FromFriendGroupPingRequest(rpcapi.FriendGroupPingRequest{Name: name})
			}
			if err != nil {
				t.Fatal(err)
			}
			response, _, _ := server.Dispatch(t.Context(), &rpcapi.RPCRequest{Id: "ping", Method: method, Params: &params})
			if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument {
				t.Fatalf("%s name %q: response = %#v", method, name, response)
			}
		}
	}
	var params rpcapi.RPCPayload
	if err := params.FromFriendPingRequest(rpcapi.FriendPingRequest{Name: giznet.PublicKey{2}.String()}); err != nil {
		t.Fatal(err)
	}
	response, _, _ := server.Dispatch(t.Context(), &rpcapi.RPCRequest{Id: "ping", Method: rpcapi.RPCMethodServerFriendPing, Params: &params})
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnavailable {
		t.Fatalf("ping without delivery: response = %#v, want UNAVAILABLE", response)
	}
}
