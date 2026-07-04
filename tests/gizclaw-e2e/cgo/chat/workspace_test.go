//go:build gizclaw_e2e

package chat_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	gochat "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/go/chat"
)

func TestCSDKChatWorkspaceRPC(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-chat")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	cgointernal.CSDKChatWorkspace(t, identityDir)
}

func TestCSDKChatRoundtrip(t *testing.T) {
	h := clitest.NewSetupHarness(t, "cgo-chat-roundtrip")
	identityDir := cgointernal.SharedIdentityDir(t, h, "GIZCLAW_E2E_PEER_IDENTITY", "peer")
	cgointernal.AssertServerAvailable(t, identityDir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	configPath := filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "workspaces", "doubao-realtime.json")
	contextConfigPath := filepath.Join(identityDir, "config.yaml")
	workspaceName, err := gochat.PrepareCgoPushToTalkWorkspace(ctx, configPath, contextConfigPath)
	if err != nil {
		t.Fatalf("prepare cgo chat workspace: %v", err)
	}
	fixture := filepath.Join(h.RepoRoot, "tests", "genx-e2e", "transformer", "testdata", "doubao_realtime_duplex_prompt.ogg")
	cgointernal.CSDKChatRoundtrip(t, identityDir, workspaceName, fixture)
}
