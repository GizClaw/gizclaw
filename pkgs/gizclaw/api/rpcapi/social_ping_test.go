package rpcapi

import (
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestSocialPingAndProfileMethodRegistry(t *testing.T) {
	want := map[RPCMethod]struct {
		id                int32
		request, response string
	}{
		RPCMethodServerFriendPing:      {123, "FriendPingRequest", "FriendPingResponse"},
		RPCMethodServerFriendGroupPing: {124, "FriendGroupPingRequest", "FriendGroupPingResponse"},
		RPCMethodServerProfileGet:      {125, "ProfileGetRequest", "ProfileGetResponse"},
		RPCMethodClientDeviceFind:      {126, "ClientDeviceFindRequest", "ClientDeviceFindResponse"},
		RPCMethodClientSocialPing:      {127, "ClientSocialPingRequest", "ClientSocialPingResponse"},
	}
	for method, tc := range want {
		if !method.Valid() {
			t.Fatalf("%s is not a valid method", method)
		}
		if got := int32(rpcMethodToProto[method]); got != tc.id {
			t.Fatalf("%s id = %d, want %d", method, got, tc.id)
		}
		if got := rpcRequestPayloadMessages[method]; got != tc.request {
			t.Fatalf("%s request message = %q, want %q", method, got, tc.request)
		}
		if got := rpcResponsePayloadMessages[method]; got != tc.response {
			t.Fatalf("%s response message = %q, want %q", method, got, tc.response)
		}
	}
}

func TestFriendPingResponseRoundTrip(t *testing.T) {
	var payload RPCPayload
	retry := int32(42)
	if err := payload.FromFriendPingResponse(FriendPingResponse{Result: SocialPingResultRateLimited, RetryAfterSeconds: &retry}); err != nil {
		t.Fatal(err)
	}
	var wire rpcpb.FriendPingResponse
	if err := proto.Unmarshal(payload.payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.GetResult() != rpcpb.SocialPingResult_SOCIAL_PING_RESULT_RATE_LIMITED || wire.GetRetryAfterSeconds() != 42 || wire.GetDeliveredCount() != 0 {
		t.Fatalf("wire ping response = %+v", &wire)
	}
	got, err := payload.AsFriendPingResponse()
	if err != nil || got.Result != SocialPingResultRateLimited || got.RetryAfterSeconds == nil || *got.RetryAfterSeconds != 42 {
		t.Fatalf("ping response round trip = %+v, %v", got, err)
	}

	if err := payload.FromFriendGroupPingResponse(FriendGroupPingResponse{Result: SocialPingResultDelivered, DeliveredCount: 3}); err != nil {
		t.Fatal(err)
	}
	group, err := payload.AsFriendGroupPingResponse()
	if err != nil || group.Result != SocialPingResultDelivered || group.DeliveredCount != 3 || group.RetryAfterSeconds != nil {
		t.Fatalf("group ping response round trip = %+v, %v", group, err)
	}
}

func TestClientSocialPingRequestRoundTrip(t *testing.T) {
	var payload RPCPayload
	display := "Alice"
	group := "class-3"
	if err := payload.FromClientSocialPingRequest(ClientSocialPingRequest{FromPeerPublicKey: "key", FromDisplayName: &display, FriendGroupName: &group}); err != nil {
		t.Fatal(err)
	}
	var wire rpcpb.ClientSocialPingRequest
	if err := proto.Unmarshal(payload.payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.GetFromPeerPublicKey() != "key" || wire.GetFromDisplayName() != "Alice" || wire.GetFriendGroupName() != "class-3" {
		t.Fatalf("wire social ping = %+v", &wire)
	}
	got, err := payload.AsClientSocialPingRequest()
	if err != nil || got.FromPeerPublicKey != "key" || *got.FromDisplayName != "Alice" || *got.FriendGroupName != "class-3" {
		t.Fatalf("social ping round trip = %+v, %v", got, err)
	}
}

func TestProfileGetRoundTrip(t *testing.T) {
	var payload RPCPayload
	if err := payload.FromProfileGetRequest(ProfileGetRequest{PeerPublicKeys: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	request, err := payload.AsProfileGetRequest()
	if err != nil || len(request.PeerPublicKeys) != 2 || request.PeerPublicKeys[1] != "b" {
		t.Fatalf("profile request round trip = %+v, %v", request, err)
	}
	name := "Bob"
	if err := payload.FromProfileGetResponse(ProfileGetResponse{Items: []PublicProfile{{PeerPublicKey: "a", DisplayName: &name}, {PeerPublicKey: "b"}}}); err != nil {
		t.Fatal(err)
	}
	response, err := payload.AsProfileGetResponse()
	if err != nil || len(response.Items) != 2 || *response.Items[0].DisplayName != "Bob" || response.Items[1].DisplayName != nil || response.Items[1].Emoji != nil {
		t.Fatalf("profile response round trip = %+v, %v", response, err)
	}
}

func TestClientDeviceFindRoundTrip(t *testing.T) {
	var payload RPCPayload
	duration := int64(8000)
	if err := payload.FromClientDeviceFindRequest(ClientDeviceFindRequest{DurationMs: &duration}); err != nil {
		t.Fatal(err)
	}
	got, err := payload.AsClientDeviceFindRequest()
	if err != nil || got.DurationMs == nil || *got.DurationMs != 8000 {
		t.Fatalf("find request round trip = %+v, %v", got, err)
	}
	if err := payload.FromClientDeviceFindRequest(ClientDeviceFindRequest{}); err != nil {
		t.Fatal(err)
	}
	if got, err := payload.AsClientDeviceFindRequest(); err != nil || got.DurationMs != nil {
		t.Fatalf("empty find request round trip = %+v, %v", got, err)
	}
}
