//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestDashScopeRealtimeWorkflowRoundtrip(t *testing.T) {
	runRequiredLiveWorkspaceCase(t, workspaceCaseRealtimeRoundtrip,
		selectedWorkspaceConfigPaths(t, "dashscope-realtime.json"))
}

func TestDoubaoRealtimeDuplexWorkflowRoundtrip(t *testing.T) {
	runRequiredLiveWorkspaceCase(t, workspaceCaseRealtimeRoundtrip,
		selectedWorkspaceConfigPaths(t, "doubao-realtime-duplex.json"))
}

func TestEinoWorkflowRoundtrip(t *testing.T) {
	runConfiguredMemoryRoundtrip(t, "eino-memory.json", workspaceCaseTextRoundtrip, "flowcraft")
}

func TestFlowcraftConfiguredMemoryStoreRoundtrip(t *testing.T) {
	runConfiguredMemoryRoundtrip(t, "flowcraft-configured-memory.json", workspaceCaseTextRoundtrip, "flowcraft")
}

func TestFlowcraftVoiceAssistantMemoryObserveRoundtrip(t *testing.T) {
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	const configName = "flowcraft-basic.json"
	path := filepath.Join("..", "..", "testdata", "workspaces", configName)
	cfg, err := loadConfig(path, clientContextConfigPath())
	if err != nil {
		t.Fatalf("load %s: %v", configName, err)
	}
	token := createChatRegistrationToken(t, workspaceCaseTextRoundtrip)
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN", token)
	runID := time.Now().UnixNano()
	cfg.workspaceSuffix = fmt.Sprintf("memory-observe-%x", runID)
	uniqueToken := fmt.Sprintf("GIZCLAWVOICE%d", runID)
	cfg.Utterances = []string{fmt.Sprintf("我的唯一语音助手测试代号是 %s，请长期记住。", uniqueToken)}
	if _, err := runLoadedConfigWithResultAndInspect(cfg, workspaceCaseTextRoundtrip, func(ctx context.Context, client *gizcli.Client, applied config) error {
		return waitConfiguredMemoryRecall(ctx, client, applied, uniqueToken, "flowcraft")
	}); err != nil {
		t.Fatalf("%s observe turn: %v", configName, err)
	}

	reuseWorkspace := false
	cfg.Ensure = &reuseWorkspace
	cfg.Utterances = []string{"请简单回复收到。"}
	if _, err := runLoadedConfigWithResultAndInspect(cfg, workspaceCaseTextRoundtrip, func(ctx context.Context, client *gizcli.Client, applied config) error {
		return waitConfiguredMemoryRecall(ctx, client, applied, uniqueToken, "flowcraft")
	}); err != nil {
		t.Fatalf("%s post-reload memory check: %v", configName, err)
	}
}

func runConfiguredMemoryRoundtrip(t *testing.T, configName string, selectedCase workspaceCase, expectedBackend string) {
	t.Helper()
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	path := filepath.Join("..", "..", "testdata", "workspaces", configName)
	cfg, err := loadConfig(path, clientContextConfigPath())
	if err != nil {
		t.Fatalf("load %s: %v", configName, err)
	}
	token := createChatRegistrationToken(t, selectedCase)
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN", token)
	runID := time.Now().UnixNano()
	cfg.workspaceSuffix = fmt.Sprintf("memory-%x", runID)
	uniqueToken := fmt.Sprintf("GIZCLAWMEMORY%d", runID)
	uniqueFact := fmt.Sprintf("我的唯一测试代号是 %s，请记住这个完整代号。", uniqueToken)
	cfg.Utterances = []string{uniqueFact}
	if _, err := runLoadedConfigWithResultAndInspect(cfg, selectedCase, func(ctx context.Context, client *gizcli.Client, applied config) error {
		return waitConfiguredMemoryRecall(ctx, client, applied, uniqueToken, expectedBackend)
	}); err != nil {
		t.Fatalf("%s observe turn: %v", configName, err)
	}
	reuseWorkspace := false
	cfg.Ensure = &reuseWorkspace
	cfg.Utterances = []string{"我的唯一测试代号是什么？请只回答完整代号。"}
	result, err := runLoadedConfigWithResultAndInspect(cfg, selectedCase, func(ctx context.Context, client *gizcli.Client, applied config) error {
		return waitConfiguredMemoryRecall(ctx, client, applied, uniqueToken, expectedBackend)
	})
	if err != nil {
		t.Fatalf("%s post-reload recall turn: %v", configName, err)
	}
	if len(result.Rounds) != 1 || !strings.Contains(result.Rounds[0].AssistantText, uniqueToken) {
		t.Fatalf("%s recalled assistant response = %#v, want token %q", configName, result.Rounds, uniqueToken)
	}
}

func waitConfiguredMemoryRecall(ctx context.Context, client *gizcli.Client, cfg config, query, expectedBackend string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		stats, err := client.GetServerRunWorkspaceMemoryStats(ctx, "workspacetest.memory.stats", rpcapi.ServerGetRunWorkspaceMemoryStatsRequest{})
		if err != nil {
			return fmt.Errorf("memory stats: %w", err)
		}
		if !stats.Available || !stats.Enabled {
			return fmt.Errorf("configured memory is unavailable: %+v", stats)
		}
		if stats.Backend == nil || *stats.Backend != expectedBackend {
			return fmt.Errorf("memory backend = %v, want %s", stats.Backend, expectedBackend)
		}
		recall, err := client.ServerRunWorkspaceRecall(ctx, "workspacetest.memory.recall", rpcapi.ServerRunWorkspaceRecallRequest{
			Query: query,
		})
		if err != nil {
			return fmt.Errorf("memory recall: %w", err)
		}
		if recall.Available {
			for _, hit := range recall.Hits {
				if strings.Contains(hit.Snippet, query) ||
					(hit.Metadata != nil && strings.Contains(fmt.Sprint(*hit.Metadata), query)) {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait configured memory recall: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("configured memory returned no hits for workspace %q", cfg.Workspace)
		case <-time.After(time.Second):
		}
	}
}
