//go:build gizclaw_e2e

package chat

import "testing"

func TestPushToTalkInterrupt(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCasePushToTalkInterrupt, interruptWorkspaceConfigPaths(t))
}
