//go:build gizclaw_e2e

package chat

import "testing"

func TestPushToTalkInterrupt(t *testing.T) {
	paths := interruptWorkspaceConfigPaths(t)
	t.Run("individual", func(t *testing.T) {
		runLiveWorkspaceCase(t, workspaceCasePushToTalkInterrupt, paths)
	})
	t.Run("concurrent", func(t *testing.T) {
		runLiveWorkspaceConcurrentCase(t, workspaceCasePushToTalkInterrupt, paths)
	})
}
