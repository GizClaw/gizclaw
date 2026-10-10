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

func TestDeviceReconnectGiztest(t *testing.T) {
	runDeviceReconnectGiztest(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}

func runDeviceReconnectGiztest(t *testing.T, run func(context.Context, string, string) ([]byte, error)) {
	t.Helper()
	runPeerControlGiztest(t, "server.device.wifi.connect.giztest.yaml", 9, 1, run, func(_ *gizclaw.Server, admin *gizcli.Client) {
		profile := adminhttp.RuntimeProfileUpsert{Id: "peer-sync-giztest", Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{},
			Mhs: &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{
				{Id: "wifi.main", Hwd: "wifi"},
			}}},
		}}
		if _, err := adminapi.PutRuntimeProfile(t.Context(), admin, profile.Id, profile); err != nil {
			t.Fatal(err)
		}
	})
}
