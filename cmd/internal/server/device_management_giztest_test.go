package server

import (
	"bytes"
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type managementGiztest struct {
	name                string
	steps, cleanup      int
	stateful, telemetry bool
}

var managementGiztests = []managementGiztest{
	{"management.control", 26, 1, false, false},
	{"management.rejected", 24, 1, false, false},
	{"management.unsupported", 14, 1, false, false},
	{"management.invalid_response", 24, 1, false, false},
	{"management.validation", 24, 1, false, false},
	{"mhs.modem.control", 8, 1, false, false},
	{"mhs.nfc.control", 8, 1, false, false},
	{"mhs.management.invalid_response", 6, 1, false, false},
	{"mhs.management.instances", 14, 1, true, false},
	{"telemetry.kv", 20, 1, false, true},
	{"telemetry.kv.limits", 21, 1, false, true},
	{"telemetry.kv.bytes", 17, 1, false, true},
	{"telemetry.kv.isolation", 13, 2, false, true},
}

// Every case uses real Server/Edge, WebRTC, reverse RPC, owner-bound HTTP
// and durable runtime storage. Device hardware results are explicit fixtures.
func TestDeviceManagementGiztestsGo(t *testing.T) {
	runDeviceManagementGiztests(t, true, true, func(ctx context.Context, file, report string) ([]byte, error) {
		cmd := giztestcmd.NewCmd()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs([]string{"run", file, "--parallel", "1", "--report", report})
		err := cmd.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}
func runDeviceManagementGiztests(t *testing.T, stateful, telemetry bool, run func(context.Context, string, string) ([]byte, error)) {
	t.Helper()
	for _, test := range managementGiztests {
		if test.stateful && !stateful || test.telemetry && !telemetry {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			runPeerControlGiztestWithChannels(t, "server.device."+test.name+".giztest.yaml", test.steps, test.cleanup, 32, run, func(_ *gizclaw.Server, admin *gizcli.Client) {
				profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{}, Mhs: &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "modem.main", Hwd: "modem"}, {Id: "modem.backup", Hwd: "modem"}, {Id: "nfc.left", Hwd: "nfc"}, {Id: "nfc.right", Hwd: "nfc"}}}}}}
				if _, err := adminapi.PutRuntimeProfile(t.Context(), admin, profile.Id, profile); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
