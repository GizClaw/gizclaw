package rpcapi

import (
	"encoding/json"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"testing"
	"time"
)

func TestKVRuntimeAndStatusCodecRetainJSON(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	for _, raw := range []string{`null`, `false`, `0`, `{"x":[null,false,0]}`, `1e400`} {
		state := apitypes.PeerReportedKV{"x": {Value: json.RawMessage(raw), ObservedAt: at}}
		var payload RPCPayload
		if err := payload.FromServerGetStatusResponse(ServerGetStatusResponse{ReportedKv: &state}); err != nil {
			t.Fatal(err)
		}
		got, err := payload.AsServerGetStatusResponse()
		if err != nil || got.ReportedKv == nil {
			t.Fatal(err)
		}
		if entry := (*got.ReportedKv)["x"]; entry.Value == nil || string(entry.Value) != raw || !entry.ObservedAt.Equal(at) {
			t.Fatalf("status lost JSON %s", raw)
		}
		if err := payload.FromServerGetRuntimeResponse(Runtime{LastSeenAt: at, ReportedKv: &state}); err != nil {
			t.Fatal(err)
		}
		runtime, err := payload.AsServerGetRuntimeResponse()
		if err != nil || runtime.ReportedKv == nil {
			t.Fatal(err)
		}
		if string((*runtime.ReportedKv)["x"].Value) != raw {
			t.Fatal("runtime lost JSON")
		}
	}
}
