//go:build gizclaw_e2e

package chat

import "testing"

func TestFlowcraftRealtimeChatRoundtrip(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCaseFlowcraftRealtimeChat, flowcraftRealtimeChatWorkspaceConfigPaths(t))
}
