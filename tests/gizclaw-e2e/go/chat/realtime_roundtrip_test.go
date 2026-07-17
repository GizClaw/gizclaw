//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeRoundtrip(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCaseRealtimeRoundtrip, continuousWorkspaceConfigPaths(t))
}
