//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestSocialPingCrossServer(t *testing.T) {
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	edgeA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_A"))
	edgeB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_B"))
	// Manager selects the connected trusted Edge with the first public key.
	// Put B behind the other Edge, so a relay cannot deliver on its own ingress.
	relay, ingress := edgeA, edgeB
	if strings.Compare(edgeB.TransportPublicKey.String(), edgeA.TransportPublicKey.String()) < 0 {
		relay, ingress = edgeB, edgeA
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	aKey, a := socialPingDevice(t, ctx, serverA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A", "a")
	_ = a.Close()
	a = connectAndServe(t, aKey, relay, serverA.PublicKey, "a-via-relay-edge")
	t.Cleanup(func() { _ = a.Close() })
	registerSocialPeer(t, ctx, a, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	localKey, local := socialPingDevice(t, ctx, serverA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A", "local")
	bKey, b := socialPingDevice(t, ctx, serverB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B", "b-home")
	_ = b.Close()
	b = connectAndServe(t, bKey, ingress, serverB.PublicKey, "b-other-edge")
	t.Cleanup(func() { _ = b.Close() })
	registerSocialPeer(t, ctx, b, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
	offlineKey, offline := socialPingDevice(t, ctx, serverB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B", "offline")
	timeoutKey, timeout := socialPingDevice(t, ctx, serverB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B", "timeout")
	_ = offline.Close()
	t.Logf("TOPOLOGY source=%s server=%s ingress_edge=%s; local=%s server=%s direct; target=%s server=%s ingress_edge=%s selected_relay_edge=%s",
		aKey.Public, serverA.PublicKey, relay.TransportPublicKey, localKey.Public, serverA.PublicKey, bKey.Public, serverB.PublicKey, ingress.TransportPublicKey, relay.TransportPublicKey)
	if ingress.TransportPublicKey == relay.TransportPublicKey {
		t.Fatal("relay Edge must differ from target ingress Edge")
	}
	if err := b.SendBatteryTelemetry(83, false); err != nil {
		t.Fatal(err)
	}
	waitPeerRunStatus(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE"), bKey.Public, 83)
	assertPeerRunAbsent(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE"), bKey.Public)

	localPings := make(chan rpcapi.ClientSocialPingRequest, 8)
	remotePings := make(chan rpcapi.ClientSocialPingRequest, 8)
	_ = local.HandleSocialPing(func(_ context.Context, req rpcapi.ClientSocialPingRequest) error { localPings <- req; return nil })
	ack := make(chan struct{})
	_ = b.HandleSocialPing(func(ctx context.Context, req rpcapi.ClientSocialPingRequest) error {
		remotePings <- req
		select {
		case <-ack:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	var timeoutCalls atomic.Int32
	timeoutStarted := make(chan time.Time, 8)
	_ = timeout.HandleSocialPing(func(ctx context.Context, req rpcapi.ClientSocialPingRequest) error {
		timeoutCalls.Add(1)
		if req.FriendGroupName == nil {
			timeoutStarted <- time.Now()
		}
		<-ctx.Done()
		return ctx.Err()
	})

	t.Run("friend_waits_for_device_ack_and_shares_window", func(t *testing.T) {
		friend := socialPingFriends(t, ctx, a, b)
		type outcome struct {
			response *rpcapi.FriendPingResponse
			err      error
		}
		done := make(chan outcome, 1)
		go func() {
			response, err := a.PingFriend(ctx, "social-remote-friend", rpcapi.FriendPingRequest{Name: friend})
			done <- outcome{response, err}
		}()
		got := awaitSocialPing(t, remotePings)
		if got.FromPeerPublicKey != aKey.Public.String() || got.FriendGroupName != nil {
			t.Fatalf("remote friend notification = %+v", got)
		}
		select {
		case early := <-done:
			t.Fatalf("relay receipt was treated as device ACK: %+v", early)
		case <-time.After(150 * time.Millisecond):
		}
		close(ack)
		gotOutcome := <-done
		requireFriendPing(t, gotOutcome.response, gotOutcome.err, rpcapi.SocialPingResultDelivered, 1)
		reverse, err := b.PingFriend(ctx, "social-reverse-window", rpcapi.FriendPingRequest{Name: aKey.Public.String()})
		requireFriendPing(t, reverse, err, rpcapi.SocialPingResultRateLimited, 0)
		if reverse.RetryAfterSeconds == nil || *reverse.RetryAfterSeconds < 1 || *reverse.RetryAfterSeconds > 60 {
			t.Fatalf("reverse retry window = %+v", reverse)
		}
		assertNoSocialPing(t, remotePings)
	})

	t.Run("local_direct_and_permissions", func(t *testing.T) {
		friend := socialPingFriends(t, ctx, a, local)
		got, err := a.PingFriend(ctx, "social-local-friend", rpcapi.FriendPingRequest{Name: friend})
		requireFriendPing(t, got, err, rpcapi.SocialPingResultDelivered, 1)
		awaitSocialPing(t, localPings)
		if _, err := a.PingFriend(ctx, "social-stranger", rpcapi.FriendPingRequest{Name: timeoutKey.Public.String()}); err == nil {
			t.Fatal("non-Friend ping accepted")
		}
		request := &rpcpb.SocialPingDeliveryRequest{TargetPeerPublicKey: localKey.Public.String(),
			Ping:           &rpcpb.ClientSocialPingRequest{FromPeerPublicKey: aKey.Public.String()},
			DeadlineUnixMs: time.Now().Add(time.Second).UnixMilli()}
		for _, method := range []rpcapi.RPCMethod{rpcapi.RPCMethodServerSocialPingDeliver, rpcapi.RPCMethodEdgeSocialPingRelay} {
			if err := gizclaw.CallSocialPingDelivery(ctx, a.PeerConn(), method, request); err == nil {
				t.Fatalf("ordinary device called internal %s", method)
			}
			// Decodable on the public service still does not authorize delivery.
			stream, err := a.PeerConn().(giznet.ContextDialer).DialContext(ctx, gizclaw.ServicePeerRPC)
			if err != nil {
				t.Fatal(err)
			}
			_ = stream.SetDeadline(time.Now().Add(3 * time.Second))
			var params rpcapi.RPCPayload
			_ = params.FromSocialPingDeliveryRequest(request)
			_ = rpcapi.WriteRequest(stream, &rpcapi.RPCRequest{V: 1, Id: "untrusted-internal", Method: method, Params: &params})
			_ = rpcapi.WriteEOS(stream)
			response, err := rpcapi.ReadResponseForMethod(stream, method)
			_ = stream.Close()
			if err != nil || response.Error == nil {
				t.Fatalf("public internal RPC response = %+v, %v", response, err)
			}
		}
		assertNoSocialPing(t, localPings)
	})

	t.Run("mixed_group_counts_only_acknowledged_devices", func(t *testing.T) {
		group := socialPingGroup(t, ctx, a, "mixed", map[string]*gizcli.Client{
			"remote-name": b, "local-name": local, "offline-name": offline, "timeout-name": timeout,
		})
		start := time.Now()
		result, err := a.PingFriendGroup(ctx, "social-mixed-group", rpcapi.FriendGroupPingRequest{Name: group})
		if err != nil || result.Result != rpcapi.SocialPingResultDelivered || result.DeliveredCount != 2 {
			t.Fatalf("mixed rally = %+v, %v", result, err)
		}
		if elapsed := time.Since(start); elapsed > 4*time.Second {
			t.Fatalf("rally exceeded ACK bound: %s", elapsed)
		}
		remote, localReq := awaitSocialPing(t, remotePings), awaitSocialPing(t, localPings)
		if remote.FriendGroupName == nil || *remote.FriendGroupName != "remote-name" ||
			localReq.FriendGroupName == nil || *localReq.FriendGroupName != "local-name" {
			t.Fatalf("recipient group names: remote=%+v local=%+v", remote, localReq)
		}
		if timeoutCalls.Load() != 1 {
			t.Fatalf("timeout device calls = %d, want one without retries", timeoutCalls.Load())
		}
		limited, err := b.PingFriendGroup(ctx, "social-group-window", rpcapi.FriendGroupPingRequest{Name: "remote-name"})
		if err != nil || limited.Result != rpcapi.SocialPingResultRateLimited || limited.RetryAfterSeconds == nil {
			t.Fatalf("shared group window = %+v, %v", limited, err)
		}
		if _, err := b.PingFriendGroup(ctx, "social-unknown-group", rpcapi.FriendGroupPingRequest{Name: "unknown"}); err == nil {
			t.Fatal("unknown group rally accepted")
		}
		assertNoSocialPing(t, remotePings)
		assertNoSocialPing(t, localPings)
	})

	t.Run("offline_reconnect_and_replacement", func(t *testing.T) {
		// Establish a relationship through the still-online inviter's token,
		// allowing a real offline target without changing any shared test data.
		invite, err := a.CreateFriendInviteToken(ctx, "social-offline-invite", rpcapi.FriendInviteTokenCreateRequest{})
		if err != nil {
			t.Fatal(err)
		}
		offline = connectAndServe(t, offlineKey, ingress, serverB.PublicKey, "offline-create-relation")
		registerSocialPeer(t, ctx, offline, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
		if _, err := offline.AddFriend(ctx, "social-offline-add", rpcapi.FriendAddRequest{InviteToken: invite.InviteToken}); err != nil {
			t.Fatal(err)
		}
		_ = offline.Close()
		start := time.Now()
		missed, err := a.PingFriend(ctx, "social-offline", rpcapi.FriendPingRequest{Name: offlineKey.Public.String()})
		requireFriendPing(t, missed, err, rpcapi.SocialPingResultNotOnline, 0)
		if time.Since(start) > 4*time.Second {
			t.Fatal("offline ping exceeded bound")
		}
		old := connectAndServe(t, offlineKey, ingress, serverB.PublicKey, "reconnect-old")
		t.Cleanup(func() { _ = old.Close() })
		registerSocialPeer(t, ctx, old, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
		var oldCalls atomic.Int32
		_ = old.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { oldCalls.Add(1); return nil })
		// Replace the live Peer through the other ingress; its old generation
		// must not receive a ping or erase the replacement during cleanup.
		current := connectAndServe(t, offlineKey, relay, serverB.PublicKey, "reconnect-replacement")
		t.Cleanup(func() { _ = current.Close() })
		registerSocialPeer(t, ctx, current, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
		currentPings := make(chan rpcapi.ClientSocialPingRequest, 2)
		_ = current.HandleSocialPing(func(_ context.Context, req rpcapi.ClientSocialPingRequest) error { currentPings <- req; return nil })
		delivered, err := a.PingFriend(ctx, "social-after-reconnect", rpcapi.FriendPingRequest{Name: offlineKey.Public.String()})
		requireFriendPing(t, delivered, err, rpcapi.SocialPingResultDelivered, 1)
		awaitSocialPing(t, currentPings)
		if oldCalls.Load() != 0 {
			t.Fatal("stale generation received a notification")
		}
	})

	t.Run("unacknowledged_friend_releases_window", func(t *testing.T) {
		friend := socialPingFriends(t, ctx, a, timeout)
		missed, err := a.PingFriend(ctx, "social-friend-timeout", rpcapi.FriendPingRequest{Name: friend})
		requireFriendPing(t, missed, err, rpcapi.SocialPingResultNotOnline, 0)
		// Authentication and membership transactions precede delivery. Measure
		// its deadline from the device call, independently of storage latency.
		select {
		case start := <-timeoutStarted:
			if time.Since(start) > 4*time.Second {
				t.Fatal("unacknowledged friend exceeded delivery bound")
			}
		default:
			t.Fatal("unacknowledged friend never reached the device")
		}
		_ = timeout.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { return nil })
		delivered, err := a.PingFriend(ctx, "social-friend-timeout-retry", rpcapi.FriendPingRequest{Name: friend})
		requireFriendPing(t, delivered, err, rpcapi.SocialPingResultDelivered, 1)
	})
}

func TestSocialPingWithoutRelay(t *testing.T) {
	if os.Getenv("GIZCLAW_E2E_EXPECT_NO_RELAY") != "1" {
		t.Skip("harness stops both Edges before this phase")
	}
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	_, a := socialPingDevice(t, ctx, serverA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A", "no-relay-a")
	_, b := socialPingDevice(t, ctx, serverB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B", "no-relay-b")
	localKey, local := socialPingDevice(t, ctx, serverA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A", "no-relay-local")
	var remoteCalls atomic.Int32
	_ = b.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { remoteCalls.Add(1); return nil })
	_ = local.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { return nil })
	friend := socialPingFriends(t, ctx, a, b)
	start := time.Now()
	result, err := a.PingFriend(ctx, "no-relay-remote", rpcapi.FriendPingRequest{Name: friend})
	requireFriendPing(t, result, err, rpcapi.SocialPingResultNotOnline, 0)
	if time.Since(start) > 4*time.Second || remoteCalls.Load() != 0 {
		t.Fatal("unavailable relay was unbounded or delivered")
	}
	socialPingFriends(t, ctx, a, local)
	result, err = a.PingFriend(ctx, "no-relay-local", rpcapi.FriendPingRequest{Name: localKey.Public.String()})
	requireFriendPing(t, result, err, rpcapi.SocialPingResultDelivered, 1)
	t.Log("both Edge processes stopped; remote online target returned NOT_ONLINE within bound; local direct delivery still ACKed")
}

func socialPingDevice(t *testing.T, ctx context.Context, transport, server gizcli.ServerInfoMetadata, token, name string) (*giznet.KeyPair, *gizcli.Client) {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client := connectAndServe(t, key, transport, server.PublicKey, "social-"+name)
	t.Cleanup(func() { _ = client.Close() })
	registerSocialPeer(t, ctx, client, server, token)
	return key, client
}

func socialPingFriends(t *testing.T, ctx context.Context, a, b *gizcli.Client) string {
	t.Helper()
	invite, err := b.CreateFriendInviteToken(ctx, "social-invite", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	friend, err := a.AddFriend(ctx, "social-add", rpcapi.FriendAddRequest{InviteToken: invite.InviteToken})
	if err != nil {
		t.Fatal(err)
	}
	return friend.Name
}

func socialPingGroup(t *testing.T, ctx context.Context, owner *gizcli.Client, name string, members map[string]*gizcli.Client) string {
	t.Helper()
	group, err := owner.CreateFriendGroup(ctx, "social-group-create", rpcapi.FriendGroupCreateRequest{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	for alias, client := range members {
		_, err := owner.AddFriendGroupMember(ctx, "social-group-add", rpcapi.FriendGroupMemberAddRequest{
			FriendGroupName: group.Name, PeerPublicKey: client.KeyPair.Public.String(), MemberName: alias,
			Role: rpcapi.FriendGroupMemberMutableRoleMember,
		})
		if err != nil {
			t.Fatalf("add %s: %v", alias, err)
		}
	}
	return group.Name
}

func requireFriendPing(t *testing.T, response *rpcapi.FriendPingResponse, err error, result rpcapi.SocialPingResult, count int32) {
	t.Helper()
	if err != nil || response == nil || response.Result != result || response.DeliveredCount != count {
		t.Fatalf("friend ping = %+v, %v; want %s count %d", response, err, result, count)
	}
}

func awaitSocialPing(t *testing.T, received <-chan rpcapi.ClientSocialPingRequest) rpcapi.ClientSocialPingRequest {
	t.Helper()
	select {
	case request := <-received:
		return request
	case <-time.After(4 * time.Second):
		t.Fatal("device did not receive client.social.ping")
		return rpcapi.ClientSocialPingRequest{}
	}
}

func assertNoSocialPing(t *testing.T, received <-chan rpcapi.ClientSocialPingRequest) {
	t.Helper()
	select {
	case duplicate := <-received:
		t.Fatalf("unexpected/duplicate device notification: %+v", duplicate)
	case <-time.After(100 * time.Millisecond):
	}
}
