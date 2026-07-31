//go:build gizclaw_e2e

package rpc_test

import (
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestCSDKConcurrentServiceStreams(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-concurrent-service-streams")
	identityDir := cgointernal.SharedIdentityDir(
		t,
		h,
		"GIZCLAW_E2E_PEER_IDENTITY",
		"peer",
	)
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
