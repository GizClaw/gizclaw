package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
)

func TestWorkspaceRewardKindUsesAuthoritativeAgentType(t *testing.T) {
	t.Parallel()
	astTranslateParameters := func(mode apitypes.ASTTranslateMode) *apitypes.WorkspaceParameters {
		var parameters apitypes.WorkspaceParameters
		if err := parameters.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{Mode: &mode}); err != nil {
			t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
		}
		return &parameters
	}
	flowcraftParameters := func() *apitypes.WorkspaceParameters {
		var parameters apitypes.WorkspaceParameters
		if err := parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{}); err != nil {
			t.Fatalf("FromFlowcraftWorkspaceParameters() error = %v", err)
		}
		return &parameters
	}
	for name, item := range map[string]struct {
		workspace apitypes.Workspace
		want      gameplay.WorkspaceRewardKind
		wantErr   string
	}{
		"workflow": {
			workspace: apitypes.Workspace{Name: "workflow-a"},
			want:      gameplay.WorkspaceRewardKindWorkflow,
		},
		"unreadable discriminator": {
			workspace: apitypes.Workspace{Name: "invalid-a", Parameters: &apitypes.WorkspaceParameters{}},
			want:      gameplay.WorkspaceRewardKindWorkflow,
		},
		"flowcraft": {
			workspace: apitypes.Workspace{Name: "flowcraft-a", Parameters: flowcraftParameters()},
			want:      gameplay.WorkspaceRewardKindWorkflow,
		},
		"ast translate s2t": {
			workspace: apitypes.Workspace{Name: "ast-s2t-a", Parameters: astTranslateParameters(apitypes.ASTTranslateModeS2t)},
			want:      gameplay.WorkspaceRewardKindWorkflow,
		},
		"ast translate s2s": {
			workspace: apitypes.Workspace{Name: "ast-s2s-a", Parameters: astTranslateParameters(apitypes.ASTTranslateModeS2s)},
			want:      gameplay.WorkspaceRewardKindWorkflow,
		},
		"sfu workflow": {
			workspace: apitypes.Workspace{Name: "social-a", WorkflowId: socialutil.SFUWorkflowID, System: new(true)},
			wantErr:   `gizclaw: Workspace is not eligible for Workspace rewards: Workspace "social-a" uses the SFU Workflow`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := workspaceRewardKind(item.workspace)
			if item.wantErr != "" {
				if err == nil || err.Error() != item.wantErr {
					t.Fatalf("workspaceRewardKind() error = %v, want %q", err, item.wantErr)
				}
				return
			}
			if err != nil || got != item.want {
				t.Fatalf("workspaceRewardKind() = %q, %v, want %q", got, err, item.want)
			}
		})
	}
}

func TestWorkspaceRewardNotificationRejectsInvalidBeneficiary(t *testing.T) {
	t.Parallel()
	environment := &workspaceRewardEnvironment{manager: &Manager{}}
	err := environment.NotifyWorkspaceReward(context.Background(), "not-a-public-key", gameplay.WorkspaceRewardUpdate{
		WorkspaceID: "workspace-a", RewardGrantID: "grant-a",
	})
	if err == nil {
		t.Fatal("NotifyWorkspaceReward() succeeded")
	}
}
