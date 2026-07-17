//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeAutoSplitHistory(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCaseRealtimeAutoSplit, realtimeAutoSplitWorkspaceConfigPaths(t))
}
