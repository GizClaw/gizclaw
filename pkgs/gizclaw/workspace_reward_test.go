package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
)

func TestWorkspaceRewardKindUsesAuthoritativeChatroomMode(t *testing.T) {
	t.Parallel()
	for name, item := range map[string]struct {
		workspace apitypes.Workspace
		want      gameplay.WorkspaceRewardKind
	}{
		"workflow": {
			workspace: apitypes.Workspace{Name: "workflow-a"},
			want:      gameplay.WorkspaceRewardKindWorkflow,
		},
		"direct chatroom": {
			workspace: apitypes.Workspace{
				Name:       "direct-a",
				Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
			},
			want: gameplay.WorkspaceRewardKindDirectChatroom,
		},
		"group chatroom": {
			workspace: apitypes.Workspace{
				Name:       "group-a",
				Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeGroup),
			},
			want: gameplay.WorkspaceRewardKindGroupChatroom,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := workspaceRewardKind(item.workspace)
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
		WorkspaceName: "workflow-a", RewardGrantID: "grant-a",
	})
	if err == nil {
		t.Fatal("NotifyWorkspaceReward() succeeded")
	}
}
