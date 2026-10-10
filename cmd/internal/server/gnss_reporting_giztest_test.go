package server

import (
	"bytes"
	"context"
	"testing"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
)

// TestGNSSReportingGiztestGo runs the shared scenarios over real Server/Edge
// WebRTC and HTTP, without a model or firmware dependency.
func TestGNSSReportingGiztestGo(t *testing.T) {
	runGNSSReportingGiztests(t, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}

func runGNSSReportingGiztests(t *testing.T, run func(context.Context, string, string) ([]byte, error)) {
	t.Helper()
	for _, scenario := range []struct {
		name           string
		steps, cleanup int
	}{
		{"control", 18, 2},
		{"invalid", 9, 1},
		{"unsupported", 5, 1},
		{"rejected", 6, 1},
		{"invalid_response", 6, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Native SDK DataChannel closure is asynchronous. This contract lane
			// has headroom for consecutive reverse RPCs; it is not a capacity test.
			runPeerControlGiztestWithChannels(t, "server.device.gnss.reporting."+scenario.name+".giztest.yaml", scenario.steps, scenario.cleanup, 32, run)
		})
	}
}
