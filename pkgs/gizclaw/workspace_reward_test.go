package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestWorkspaceRewardEnvironmentListsMalformedWorkspaceByCanonicalKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	const workspaceID = "workspace-malformed"
	raw := []byte("{")
	if err := store.Set(ctx, kv.Key{"by-id", workspaceID}, raw); err != nil {
		t.Fatalf("seed malformed Workspace: %v", err)
	}
	environment := &workspaceRewardEnvironment{workspaces: &workspace.Server{Store: store}}

	ids, err := environment.ListWorkspaceIDs(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaceIDs() error = %v", err)
	}
	if want := []string{workspaceID}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ListWorkspaceIDs() = %#v, want %#v", ids, want)
	}
	stored, err := store.Get(ctx, kv.Key{"by-id", workspaceID})
	if err != nil {
		t.Fatalf("read malformed Workspace: %v", err)
	}
	if !reflect.DeepEqual(stored, raw) {
		t.Fatalf("stored malformed Workspace = %q, want %q", stored, raw)
	}
}

func TestWorkspaceRewardEnvironmentMapsOnlyExactOwnerMissingIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 8, 12, 10, 15, 0, 0, time.UTC)
	system := true
	owner := "peer-owner"
	item := apitypes.Workspace{
		Id: "workspace-owner-missing", Name: "workspace-owner-missing", WorkflowId: "workflow-valid",
		System: &system, OwnerPublicKey: &owner,
		CreatedAt: now, LastActiveAt: now, UpdatedAt: now,
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := store.Set(ctx, kv.Key{"by-id", item.Id}, data); err != nil {
		t.Fatalf("seed Workspace: %v", err)
	}
	environment := &workspaceRewardEnvironment{workspaces: &workspace.Server{
		Store: store,
		PeerAvailability: func(context.Context, string) error {
			return fmt.Errorf("exact owner lookup: %w", runtimepeer.ErrPeerNotFound)
		},
	}}
	if err := environment.EnsureWorkspaceAvailable(ctx, item.Id); !errors.Is(err, workspace.ErrPeerNotFound) ||
		!workspace.IsExactOwnerNotFound(err) {
		t.Fatalf("EnsureWorkspaceAvailable() error = %v, want exact Workspace owner-missing identity", err)
	}
}

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
	chatroomParametersWithoutMode := func() *apitypes.WorkspaceParameters {
		var parameters apitypes.WorkspaceParameters
		if err := parameters.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{}); err != nil {
			t.Fatalf("FromChatRoomWorkspaceParameters() error = %v", err)
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
		"chatroom without mode": {
			workspace: apitypes.Workspace{Name: "chatroom-a", Parameters: chatroomParametersWithoutMode()},
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
		"unsupported chatroom": {
			workspace: apitypes.Workspace{
				Name:       "unsupported-a",
				Parameters: socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomMode("unsupported")),
			},
			wantErr: `gizclaw: Workspace "unsupported-a" has unsupported Chatroom mode "unsupported"`,
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
