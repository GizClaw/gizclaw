//go:build gizclaw_e2e

package chat

import "testing"

func TestHistoryReplay(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCaseHistoryReplay, historyReplayWorkspaceConfigPaths(t))
}
