package server

import (
	"bytes"
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
)

func TestAPIKeyLimitGiztestGo(t *testing.T) {
	runPeerControlGiztest(t, "server.api_key.limit.giztest.yaml", 20, 2, func(ctx context.Context, file, report string) ([]byte, error) {
		command := giztestcmd.NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs([]string{"run", file, "--parallel", "1", "--output", report})
		err := command.ExecuteContext(ctx)
		return output.Bytes(), err
	})
}
