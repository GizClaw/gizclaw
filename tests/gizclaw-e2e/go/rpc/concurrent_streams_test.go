//go:build gizclaw_e2e

package rpc_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestConcurrentServiceStreams(t *testing.T) {
	h := clitest.NewSetupHarness(t, "go-concurrent-service-streams")
	aliasSetupAdminContext(t, h)
	registerSetupPeer(t, h, "peer-a", "go-concurrent-service-streams", true)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { peer.Close() })
	peerPublicKey := h.ContextPublicKey("peer-a")
	registerDefaultRuntimeProfile(t, h, peer)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	groupName := "Concurrent service Event probe"
	group, err := peer.CreateFriendGroup(
		ctx,
		"concurrent-services.group.create",
		rpcapi.FriendGroupCreateRequest{Name: groupName},
	)
	cancel()
	if err != nil {
		t.Fatalf("create Event probe Friend Group: %v", err)
	}
	if group.Name == "" {
		t.Fatalf("Event probe Friend Group has no name: %+v", group)
	}
	groupName = group.Name
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := peer.DeleteFriendGroup(
			cleanupCtx,
			"concurrent-services.group.delete",
			rpcapi.FriendGroupDeleteRequest{Name: groupName},
		); err != nil {
			t.Errorf("delete Event probe Friend Group: %v", err)
		}
		deleteGoEventProbePeer(t, h, peerPublicKey)
	})

	peerConn := peer.PeerConn()
	if peerConn == nil {
		t.Fatal("peer connection is nil")
	}

	eventStream, err := peer.DialPeerEventStream()
	if err != nil {
		t.Fatalf("open mandatory Event stream: %v", err)
	}
	defer eventStream.Close()
	setStreamDeadline(t, eventStream)

	firstRPC := dialServiceStream(t, peerConn.Dial, gizcli.ServicePeerRPC)
	defer firstRPC.Close()
	secondRPC := dialServiceStream(t, peerConn.Dial, gizcli.ServicePeerRPC)
	defer secondRPC.Close()
	peerHTTP := dialServiceStream(t, peerConn.Dial, gizcli.ServicePeerHTTP)
	defer peerHTTP.Close()

	if firstRPC == secondRPC || firstRPC == peerHTTP || secondRPC == peerHTTP {
		t.Fatal("service Dial reused a live channel identity")
	}
	if peer.PeerConn() != peerConn {
		t.Fatal("opening service streams replaced the Peer connection")
	}

	writePingRequest(t, firstRPC, "concurrent-services-first")
	writePingRequest(t, secondRPC, "concurrent-services-second")
	httpRequest, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://gizclaw/server-info",
		nil,
	)
	if err != nil {
		t.Fatalf("create Peer HTTP request: %v", err)
	}
	if err := httpRequest.Write(peerHTTP); err != nil {
		t.Fatalf("write Peer HTTP request: %v", err)
	}

	requirePingResponse(t, firstRPC, "concurrent-services-first")
	if err := firstRPC.Close(); err != nil {
		t.Fatalf("close first RPC stream: %v", err)
	}
	requirePeerEventAfterServiceClose(t, peer, eventStream, groupName)

	requirePingResponse(t, secondRPC, "concurrent-services-second")
	httpResponse, err := http.ReadResponse(bufio.NewReader(peerHTTP), httpRequest)
	if err != nil {
		t.Fatalf("read Peer HTTP response after closing sibling RPC: %v", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResponse.Body)
		t.Fatalf("Peer HTTP status = %d, want 200: %s", httpResponse.StatusCode, body)
	}
	if peer.PeerConn() != peerConn {
		t.Fatal("closing one service stream replaced or closed the Peer connection")
	}
}

func deleteGoEventProbePeer(
	t *testing.T,
	h *clitest.Harness,
	peerPublicKey string,
) {
	t.Helper()
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Errorf("create Go cleanup admin client: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	response, err := api.DeletePeerWithResponse(ctx, peerPublicKey)
	if err != nil {
		t.Errorf("delete Go Event probe peer: %v", err)
		return
	}
	if response.JSON200 == nil {
		t.Errorf(
			"delete Go Event probe peer status %d: %s",
			response.StatusCode(),
			response.Body,
		)
	}
}

func registerDefaultRuntimeProfile(
	t *testing.T,
	h *clitest.Harness,
	peer *gizcli.Client,
) {
	t.Helper()
	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create concurrent-stream admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tokenName := fmt.Sprintf("e2e-gocs-%d", time.Now().UnixNano())
	profile, found, err := clitest.RuntimeProfileByName(ctx, api, "default-gameplay")
	if err != nil || !found {
		t.Fatalf("resolve default gameplay RuntimeProfile: found=%v err=%v", found, err)
	}
	response, err := api.CreateRegistrationTokenWithResponse(
		ctx,
		adminhttp.RegistrationTokenUpsert{
			Name:             tokenName,
			Token:            tokenName,
			RuntimeProfileId: profile.Id,
		},
	)
	if err != nil {
		t.Fatalf("create concurrent-stream registration token: %v", err)
	}
	if response.JSON200 == nil {
		t.Fatalf(
			"create concurrent-stream registration token status %d: %s",
			response.StatusCode(),
			response.Body,
		)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		response, err := api.DeleteRegistrationTokenWithResponse(
			cleanupCtx,
			response.JSON200.Id,
		)
		if err != nil {
			t.Errorf("delete concurrent-stream registration token: %v", err)
		} else if response.StatusCode() != 204 {
			t.Errorf(
				"delete concurrent-stream registration token status %d: %s",
				response.StatusCode(),
				response.Body,
			)
		}
	}()
	registered, err := peer.Register(ctx, "concurrent-services.register", tokenName)
	if err != nil {
		t.Fatalf("register concurrent-stream peer: %v", err)
	}
	if registered.RuntimeProfileName != profile.Name {
		t.Fatalf("registered RuntimeProfile = %q, want %q", registered.RuntimeProfileName, profile.Name)
	}
}

func requirePeerEventAfterServiceClose(
	t *testing.T,
	peer *gizcli.Client,
	stream net.Conn,
	groupName string,
) {
	t.Helper()
	updatedGroupName := groupName + " updated"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	groupPut, err := peer.PutFriendGroup(
		ctx,
		"concurrent-services.group.put",
		rpcapi.FriendGroupPutRequest{Name: groupName, DisplayName: &updatedGroupName},
	)
	if err != nil {
		t.Fatalf("update Event probe Friend Group: %v", err)
	}
	if groupPut.UpdatedAt == nil {
		t.Fatalf("updated Event probe Friend Group has no server updated_at: %+v", groupPut)
	}
	revisionFloor := groupPut.UpdatedAt.UnixMilli()
	if err := stream.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatalf("set Peer Event read deadline: %v", err)
	}
	unmatched := make([]string, 0, 5)
	for {
		event, err := gizcli.ReadPeerStreamEvent(stream)
		if err != nil {
			t.Fatalf(
				"read Peer Event after closing sibling RPC: expected group=%q change=%s revision>=%d; unmatched=%v: %v",
				groupName,
				eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_METADATA_UPDATED,
				revisionFloor,
				unmatched,
				err,
			)
		}
		update := event.GetFriendGroupUpdated()
		if event.GetType() != eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED ||
			update.GetFriendGroupName() != groupName ||
			update.GetChange() != eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_METADATA_UPDATED ||
			update.GetRevisionUnixMs() < revisionFloor {
			if len(unmatched) < cap(unmatched) {
				unmatched = append(unmatched, fmt.Sprintf(
					"type=%s group=%q change=%s revision=%d",
					event.GetType(),
					update.GetFriendGroupName(),
					update.GetChange(),
					update.GetRevisionUnixMs(),
				))
			}
			continue
		}
		return
	}
}

func dialServiceStream(
	t *testing.T,
	dial func(uint64) (net.Conn, error),
	service uint64,
) net.Conn {
	t.Helper()
	stream, err := dial(service)
	if err != nil {
		t.Fatalf("dial service %d: %v", service, err)
	}
	setStreamDeadline(t, stream)
	return stream
}

func setStreamDeadline(t *testing.T, stream net.Conn) {
	t.Helper()
	if err := stream.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatalf("set stream deadline: %v", err)
	}
}

func writePingRequest(t *testing.T, stream io.Writer, id string) {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()}); err != nil {
		t.Fatalf("encode ping params: %v", err)
	}
	request := &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Method: rpcapi.RPCMethodAllPing,
		Params: &params,
	}
	if err := rpcapi.WriteRequest(stream, request); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		t.Fatalf("write %s EOS: %v", id, err)
	}
}

func requirePingResponse(t *testing.T, stream io.Reader, id string) {
	t.Helper()
	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodAllPing)
	if err != nil {
		t.Fatalf("read %s response: %v", id, err)
	}
	if response.Id != id || response.Error != nil || response.Result == nil {
		t.Fatalf("unexpected %s response: %+v", id, response)
	}
	ping, err := response.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("decode %s response: %v", id, err)
	}
	if ping.ServerTime <= 0 {
		t.Fatalf("%s server_time = %d, want positive", id, ping.ServerTime)
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		t.Fatalf("read %s EOS: %v", id, err)
	}
}
