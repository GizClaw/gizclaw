//go:build gizclaw_e2e

package chat

import "testing"

func TestRealtimeInterrupt(t *testing.T) {
	paths := realtimeInterruptWorkspaceConfigPaths(t)
	t.Run("individual", func(t *testing.T) {
		runLiveWorkspaceCase(t, workspaceCaseRealtimeInterrupt, paths)
	})
	t.Run("concurrent", func(t *testing.T) {
		runLiveWorkspaceConcurrentCase(t, workspaceCaseRealtimeInterrupt, paths)
	})
}
