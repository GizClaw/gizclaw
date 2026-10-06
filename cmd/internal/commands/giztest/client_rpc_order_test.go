package giztestcmd

import (
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/proto"
)

func TestDeviceReceiptsPreserveCrossProcedureOrderAndPeerIsolation(t *testing.T) {
	log := &inboundMutationLog{}
	first, second := &inboundCounter{mutations: log}, &inboundCounter{mutations: log}
	first.recordRequests.Store(true)
	second.recordRequests.Store(true)
	payload, err := proto.Marshal(&rpcpb.LedHwdWriteRequest{BrightnessPercent: new(uint32(30))})
	if err != nil {
		t.Fatal(err)
	}
	first.observeRequest(rpcapi.RPCMethodClientMhsV0Write, 0, &rpcpb.ClientMhsV0WriteRequest{Id: "led.status", Hwd: rpcpb.ClientHwd_CLIENT_HWD_LED, Payload: payload})
	second.observeRequest(rpcapi.RPCMethodClientToolV0Invoke, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_GET, &rpcpb.ClientDeviceAudioPlayerGetRequest{})
	second.observeRequest(rpcapi.RPCMethodClientToolV0Invoke, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY, &rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: new(uint32(2))})
	first.observeRequest(rpcapi.RPCMethodClientMhsV0Read, 0, &rpcpb.ClientMhsV0ReadRequest{Id: "led.status", Hwd: rpcpb.ClientHwd_CLIENT_HWD_LED})
	got := log.snapshot()
	if len(got) != 2 || got[0].(map[string]any)["id"] != "led.status" || got[1].(map[string]any)["tool"] != "audioplayer.play" {
		t.Fatalf("cross-procedure receipts = %#v", got)
	}
	got[0] = nil
	if log.snapshot()[0] == nil {
		t.Fatal("snapshot slice aliases the log")
	}
	other := &inboundCounter{mutations: &inboundMutationLog{}}
	other.recordRequests.Store(true)
	other.observeRequest(rpcapi.RPCMethodClientToolV0Invoke, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_STOP, &rpcpb.ClientDeviceAudioPlayerStopRequest{})
	if len(log.snapshot()) != 2 || len(other.mutations.snapshot()) != 1 {
		t.Fatal("another Peer's receipts crossed the log")
	}
}

func TestDeviceMutationLogRemainsBoundedUnderConcurrentObservation(t *testing.T) {
	log := &inboundMutationLog{}
	var work sync.WaitGroup
	for range 8 {
		work.Go(func() {
			for range 200 {
				log.append(map[string]any{"tool": "audioplayer.stop"})
				_ = log.snapshot()
			}
		})
	}
	work.Wait()
	if len(log.snapshot()) != 1024 {
		t.Fatalf("bounded receipt count = %d", len(log.snapshot()))
	}
}

func TestReconfiguredDeviceCountersKeepTheirOrderedReceipts(t *testing.T) {
	steps := []giztest.Step{
		{ID: "play", Client: "peer", ClientRPC: &giztest.ClientRPCOperation{Method: "client.tool.v0.invoke", Tool: "audioplayer.play", ObserveOnly: true}},
		{ID: "write", Client: "peer", ClientRPC: &giztest.ClientRPCOperation{Method: "client.mhs.v0.write", ObserveOnly: true}},
	}
	counts := map[string]*inboundCounter{}
	if err := configureClientRPC(&gizcli.Client{}, "peer", steps, mustVariables(t, nil), counts); err != nil {
		t.Fatal(err)
	}
	play := counts["peer:client.tool.v0.invoke:audioplayer.play"]
	write := counts["peer:client.mhs.v0.write"]
	log := play.mutations
	log.append(map[string]any{"tool": "audioplayer.play"})
	if err := configureClientRPC(&gizcli.Client{}, "peer", steps, mustVariables(t, nil), counts); err != nil {
		t.Fatal(err)
	}
	if play.mutations != log || write.mutations != log || len(log.snapshot()) != 1 {
		t.Fatal("reconnection reset cross-procedure receipts")
	}
}
