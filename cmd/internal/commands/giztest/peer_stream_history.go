package giztestcmd

import (
	"context"
	"fmt"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func workspaceHistoryNames(result map[string]any) map[string]bool {
	names := make(map[string]bool)
	items, _ := result["items"].([]any)
	for _, value := range items {
		item, _ := value.(map[string]any)
		name, _ := item["name"].(string)
		if name != "" {
			names[name] = true
		}
	}
	return names
}

func peerStreamHistoryRequiresAgent(terminalLabel string, responseCount int) bool {
	return terminalLabel != "transcript" && responseCount > 0
}

func waitForCurrentWorkspaceHistory(ctx context.Context, query func(context.Context) (map[string]any, error), before map[string]bool, requireAgent bool) (string, error) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, err := query(ctx)
		if err != nil {
			return "", fmt.Errorf("query Workspace history: %w", err)
		}
		if name := firstCurrentHistoryName(result, before, requireAgent); name != "" {
			return name, nil
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			return "", fmt.Errorf("timed out waiting for Workspace history persistence")
		case <-ctx.Done():
			return "", context.Cause(ctx)
		}
	}
}

// Only an entry absent before input can be this turn's persistence receipt.
// A turn with assistant output also requires an agent entry: its user transcript
// alone cannot satisfy completion while the reply is still being stored.
func firstCurrentHistoryName(result map[string]any, before map[string]bool, requireAgent bool) string {
	items, _ := result["items"].([]any)
	for _, value := range items {
		item, _ := value.(map[string]any)
		name, _ := item["name"].(string)
		if name == "" || before[name] {
			continue
		}
		if requireAgent && item["type"] != rpcpb.PeerRunHistoryEntryType_PEER_RUN_HISTORY_ENTRY_TYPE_AGENT.String() {
			continue
		}
		return name
	}
	return ""
}
