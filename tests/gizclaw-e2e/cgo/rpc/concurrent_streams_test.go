//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestCSDKRPCDataChannelLifecycleLocal(t *testing.T) {
	fixture := cgointernal.NewServerRPCFixture(t)
	listener := fixture.Conn.ListenService(0)
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		for range 2 {
			stream, err := listener.Accept()
			if err != nil {
				serverErr <- err
				return
			}
			request, err := rpcapi.ReadRequest(stream)
			if err == nil {
				err = rpcapi.ReadEOS(stream)
			}
			var result rpcapi.RPCPayload
			if err == nil {
				err = result.FromPingResponse(rpcapi.PingResponse{
					ServerTime: time.Now().UnixMilli(),
				})
			}
			if err == nil {
				err = rpcapi.WriteResponseForMethod(
					stream,
					request.Method,
					&rpcapi.RPCResponse{
						V: rpcapi.RPCVersionV1, Id: request.Id, Result: &result,
					},
				)
			}
			if err == nil {
				err = rpcapi.WriteEOS(stream)
			}
			closeErr := stream.Close()
			if err != nil {
				serverErr <- err
				return
			}
			if closeErr != nil {
				serverErr <- closeErr
				return
			}
		}
		serverErr <- nil
	}()

	baseline := requireTransportSnapshot(t, fixture.Client)
	requireMandatoryCTransports(t, baseline)
	if len(baseline.RPCChannelIDs) != 0 ||
		baseline.ActiveRPCChannelID != 0 {
		t.Fatalf("connect left an idle RPC channel: %+v", baseline)
	}

	for index, id := range []string{"local-rpc-1", "local-rpc-2"} {
		var response rpcpb.PingResponse
		if err := fixture.Client.CallRPC(
			rpcpb.RpcMethod_RPC_METHOD_ALL_PING,
			&rpcpb.PingRequest{ClientSendTime: time.Now().UnixMilli()},
			&response,
		); err != nil {
			t.Fatalf("CallRPC(%s): %v", id, err)
		}
		if response.ServerTime <= 0 {
			t.Fatalf("CallRPC(%s) server_time = %d", id, response.ServerTime)
		}
		after := requireTransportSnapshot(t, fixture.Client)
		requireStableMandatoryCTransports(t, baseline, after)
		if len(after.RPCChannelIDs) != 0 ||
			after.ActiveRPCChannelID != 0 {
			t.Fatalf("CallRPC(%s) left a live RPC channel: %+v", id, after)
		}
		wantNextID := baseline.NextLocalChannelID - index - 1
		if after.NextLocalChannelID != wantNextID {
			t.Fatalf(
				"CallRPC(%s) next channel id = %d, want %d",
				id,
				after.NextLocalChannelID,
				wantNextID,
			)
		}
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("local RPC server: %v", err)
	}

	serviceListener := fixture.Conn.ListenService(48)
	defer serviceListener.Close()
	channels := make([]*cgointernal.ServiceChannel, 0, 15)
	for range 15 {
		channel, err := fixture.Client.OpenServiceChannel(48, 10*time.Second)
		if err != nil {
			t.Fatalf("open channel %d: %v", len(channels)+1, err)
		}
		channels = append(channels, channel)
	}
	defer func() {
		for _, channel := range channels {
			channel.Close()
		}
	}()
	_, err := fixture.Client.OpenServiceChannel(48, 10*time.Second)
	var statusErr *cgointernal.StatusError
	if !errors.As(err, &statusErr) ||
		statusErr.Code != cgointernal.StatusChannelLimit {
		t.Fatalf("sixteenth caller channel error = %v, want channel limit", err)
	}
	if err := channels[14].SendFrame(cgointernal.StreamFrame{
		Type: cgointernal.RPCFrameEOS,
	}); err != nil {
		t.Fatalf("existing channel after limit: %v", err)
	}
	channels[0].Close()
	replacement, err := fixture.Client.OpenServiceChannel(48, 10*time.Second)
	if err != nil {
		t.Fatalf("open replacement channel: %v", err)
	}
	channels[0] = replacement
}

func TestCSDKConcurrentServiceStreams(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-concurrent-service-streams")
	identityDir := cgointernal.SharedIdentityDir(
		t,
		h,
		"GIZCLAW_E2E_PEER_IDENTITY",
		"peer",
	)
	h.SetContextDirAlias("cgo-concurrent-services-peer", identityDir)
	peerPublicKey := h.ContextPublicKey("cgo-concurrent-services-peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	client, err := cgointernal.NewClient(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	eventStream, err := client.OpenEventStream(15 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer eventStream.Close()
	const (
		groupName = "C concurrent service Event probe"
		peerName  = "C concurrent service stream peer"
	)
	var putInfo rpcpb.ServerPutInfoResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_INFO_PUT,
		&rpcpb.ServerPutInfoRequest{
			Value: &rpcpb.DeviceProfile{Name: ptr(peerName)},
		},
		&putInfo,
	); err != nil {
		t.Fatalf("register C Event probe peer: %v", err)
	}
	var groupID string
	defer func() {
		if groupID != "" {
			var groupDelete rpcpb.FriendGroupDeleteResponse
			if err := client.CallRPC(
				rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_DELETE,
				&rpcpb.FriendGroupDeleteRequest{Name: groupID},
				&groupDelete,
			); err != nil {
				t.Errorf("delete C Event probe Friend Group: %v", err)
			}
		}
		deleteCEventProbePeer(t, h, peerPublicKey)
	}()
	registerCDefaultRuntimeProfile(t, h, client)
	var groupCreate rpcpb.FriendGroupCreateResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_CREATE,
		&rpcpb.FriendGroupCreateRequest{Name: groupName},
		&groupCreate,
	); err != nil {
		t.Fatalf("create C Event probe Friend Group: %v", err)
	}
	groupID = groupCreate.GetValue().GetName()
	if groupID == "" {
		t.Fatalf("C Event probe Friend Group has no name: %s", groupCreate.String())
	}

	baseline := requireTransportSnapshot(t, client)
	requireMandatoryCTransports(t, baseline)

	firstRPC, err := client.OpenServiceChannel(0, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer firstRPC.Close()
	afterFirst := requireTransportSnapshot(t, client)
	firstID := requireOneAddedRPCChannel(t, baseline.RPCChannelIDs, afterFirst.RPCChannelIDs)

	secondRPC, err := client.OpenServiceChannel(0, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer secondRPC.Close()
	afterSecond := requireTransportSnapshot(t, client)
	secondID := requireOneAddedRPCChannel(t, afterFirst.RPCChannelIDs, afterSecond.RPCChannelIDs)
	if firstID == secondID {
		t.Fatalf("two live C RPC handles reused backend channel id %d", firstID)
	}
	requireStableMandatoryCTransports(t, baseline, afterSecond)

	sendCServicePing(t, firstRPC, "cgo-concurrent-first")
	sendCServicePing(t, secondRPC, "cgo-concurrent-second")
	requireCServicePing(t, firstRPC, "cgo-concurrent-first")
	firstRPC.Close()
	requireCEventAfterServiceClose(t, client, eventStream, groupID, groupName)

	afterClose := requireTransportSnapshot(t, client)
	if slices.Contains(afterClose.RPCChannelIDs, firstID) {
		t.Fatalf("closed RPC channel id %d remains in use: %+v", firstID, afterClose)
	}
	if !slices.Contains(afterClose.RPCChannelIDs, secondID) {
		t.Fatalf("closing RPC channel id %d also removed sibling %d: %+v", firstID, secondID, afterClose)
	}
	requireStableMandatoryCTransports(t, baseline, afterClose)
	requireCServicePing(t, secondRPC, "cgo-concurrent-second")
	requireStableMandatoryCTransports(
		t,
		baseline,
		requireTransportSnapshot(t, client),
	)
}

func registerCDefaultRuntimeProfile(
	t *testing.T,
	h *clitest.Harness,
	client *cgointernal.Client,
) {
	t.Helper()
	adminDir := cgointernal.SharedIdentityDir(
		t,
		h,
		"GIZCLAW_E2E_ADMIN_IDENTITY",
		"admin",
	)
	h.SetContextDirAlias("cgo-concurrent-services-admin", adminDir)
	admin := h.ConnectClientFromContext("cgo-concurrent-services-admin")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create C concurrent-stream admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tokenName := fmt.Sprintf("e2e-c-concurrent-stream-%d", time.Now().UnixNano())
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
		t.Fatalf("create C concurrent-stream registration token: %v", err)
	}
	if response.JSON200 == nil {
		t.Fatalf(
			"create C concurrent-stream registration token status %d: %s",
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
			t.Errorf("delete C concurrent-stream registration token: %v", err)
		} else if response.StatusCode() != 204 {
			t.Errorf(
				"delete C concurrent-stream registration token status %d: %s",
				response.StatusCode(),
				response.Body,
			)
		}
	}()
	var registered rpcpb.ServerRegisterResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_REGISTER,
		&rpcpb.ServerRegisterRequest{Token: tokenName},
		&registered,
	); err != nil {
		t.Fatalf("register C concurrent-stream peer: %v", err)
	}
	if registered.GetRuntimeProfileName() != profile.Name {
		t.Fatalf(
			"registered C RuntimeProfile = %q, want %q",
			registered.GetRuntimeProfileName(),
			profile.Name,
		)
	}
}

func deleteCEventProbePeer(
	t *testing.T,
	h *clitest.Harness,
	peerPublicKey string,
) {
	t.Helper()
	admin := h.ConnectClientFromContext("cgo-concurrent-services-admin")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Errorf("create C cleanup admin client: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	response, err := api.DeletePeerWithResponse(ctx, peerPublicKey)
	if err != nil {
		t.Errorf("delete C Event probe peer: %v", err)
		return
	}
	if response.JSON200 == nil {
		t.Errorf(
			"delete C Event probe peer status %d: %s",
			response.StatusCode(),
			response.Body,
		)
	}
}

func requireCEventAfterServiceClose(
	t *testing.T,
	client *cgointernal.Client,
	eventStream *cgointernal.EventStream,
	groupID string,
	groupName string,
) {
	t.Helper()
	updatedGroupName := groupName + " updated"
	var groupPut rpcpb.FriendGroupPutResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_FRIEND_GROUP_PUT,
		&rpcpb.FriendGroupPutRequest{Name: groupID, DisplayName: &updatedGroupName},
		&groupPut,
	); err != nil {
		t.Fatalf("update C Event probe Friend Group: %v", err)
	}
	updatedAt := groupPut.GetValue().GetUpdatedAt()
	if updatedAt == "" {
		t.Fatal("updated C Event probe Friend Group has no server updated_at")
	}
	serverUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		t.Fatalf("parse C Event probe Friend Group server updated_at %q: %v", updatedAt, err)
	}
	revisionFloor := serverUpdatedAt.UnixMilli()
	unmatched := make([]string, 0, 5)
	for {
		event, err := eventStream.ReadEvent(15 * time.Second)
		if err != nil {
			t.Fatalf(
				"read C Event after closing sibling RPC: expected group=%q change=%s revision>=%d; unmatched=%v: %v",
				groupID,
				eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_METADATA_UPDATED,
				revisionFloor,
				unmatched,
				err,
			)
		}
		update := event.GetFriendGroupUpdated()
		if event.GetType() != eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED ||
			update.GetFriendGroupName() != groupID ||
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

func sendCServicePing(
	t *testing.T,
	channel *cgointernal.ServiceChannel,
	id string,
) {
	t.Helper()
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{
		ClientSendTime: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("encode %s params: %v", id, err)
	}
	frame, err := rpcapi.NewRequestFrame(&rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Method: rpcapi.RPCMethodAllPing,
		Params: &params,
	})
	if err != nil {
		t.Fatalf("encode %s request: %v", id, err)
	}
	if err := channel.SendFrame(cgointernal.StreamFrame{
		Type: int(frame.Type),
		Data: frame.Payload,
	}); err != nil {
		t.Fatalf("send %s request: %v", id, err)
	}
	if err := channel.SendFrame(cgointernal.StreamFrame{
		Type: cgointernal.RPCFrameEOS,
	}); err != nil {
		t.Fatalf("send %s EOS: %v", id, err)
	}
}

func requireCServicePing(
	t *testing.T,
	channel *cgointernal.ServiceChannel,
	id string,
) {
	t.Helper()
	frame, err := channel.ReadFrame(15 * time.Second)
	if err != nil {
		t.Fatalf("read %s response: %v", id, err)
	}
	response, err := rpcapi.DecodeResponseFrameForMethod(
		rpcapi.RPCMethodAllPing,
		rpcapi.Frame{
			Type:    rpcapi.FrameType(frame.Type),
			Payload: frame.Data,
		},
	)
	if err != nil {
		t.Fatalf("decode %s response: %v", id, err)
	}
	if response.Id != id || response.Error != nil || response.Result == nil {
		t.Fatalf("unexpected %s response: %+v", id, response)
	}
	result, err := response.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("decode %s result: %v", id, err)
	}
	if result.ServerTime <= 0 {
		t.Fatalf("%s server_time = %d, want positive", id, result.ServerTime)
	}
	eos, err := channel.ReadFrame(15 * time.Second)
	if err != nil {
		t.Fatalf("read %s EOS: %v", id, err)
	}
	if eos.Type != cgointernal.RPCFrameEOS || len(eos.Data) != 0 {
		t.Fatalf("unexpected %s EOS: %+v", id, eos)
	}
}

func requireTransportSnapshot(
	t *testing.T,
	client *cgointernal.Client,
) cgointernal.TransportSnapshot {
	t.Helper()
	snapshot, err := client.TransportSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func requireMandatoryCTransports(
	t *testing.T,
	snapshot cgointernal.TransportSnapshot,
) {
	t.Helper()
	if snapshot.BackendHandle == 0 ||
		!snapshot.PacketReady ||
		snapshot.EventChannelID == 0 ||
		!snapshot.MediaReady {
		t.Fatalf("mandatory C transports are not ready: %+v", snapshot)
	}
}

func requireStableMandatoryCTransports(
	t *testing.T,
	want cgointernal.TransportSnapshot,
	got cgointernal.TransportSnapshot,
) {
	t.Helper()
	if got.BackendHandle != want.BackendHandle ||
		got.PacketChannelID != want.PacketChannelID ||
		got.PacketReady != want.PacketReady ||
		got.EventChannelID != want.EventChannelID ||
		got.MediaReady != want.MediaReady {
		t.Fatalf("mandatory C transport identities changed: before=%+v after=%+v", want, got)
	}
}

func requireOneAddedRPCChannel(
	t *testing.T,
	before []int,
	after []int,
) int {
	t.Helper()
	var added []int
	for _, id := range after {
		if !slices.Contains(before, id) {
			added = append(added, id)
		}
	}
	if len(added) != 1 {
		t.Fatalf("RPC channel delta = %v, want one; before=%v after=%v", added, before, after)
	}
	return added[0]
}

func ptr[T any](value T) *T { return &value }
