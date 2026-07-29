package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type workspaceRewardEnvironment struct {
	manager    *Manager
	workspaces *workspace.Server
}

func (environment *workspaceRewardEnvironment) ListWorkspaceNames(ctx context.Context) ([]string, error) {
	if environment == nil || environment.workspaces == nil {
		return nil, errors.New("gizclaw: Workspace reward source is not configured")
	}
	limit := int32(200)
	var cursor *string
	var names []string
	for {
		response, err := environment.workspaces.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
			Params: adminhttp.ListWorkspacesParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return nil, err
		}
		page, ok := response.(adminhttp.ListWorkspaces200JSONResponse)
		if !ok {
			return nil, fmt.Errorf("gizclaw: list Workspaces returned %T", response)
		}
		for _, item := range page.Items {
			if name := strings.TrimSpace(item.Name); name != "" {
				names = append(names, name)
			}
		}
		if !page.HasNext || page.NextCursor == nil {
			return names, nil
		}
		cursor = page.NextCursor
	}
}

func (environment *workspaceRewardEnvironment) LatestHistoryEntry(
	ctx context.Context,
	workspaceName string,
) (workspace.HistoryEntry, bool, error) {
	return environment.workspaces.LatestWorkspaceHistoryEntry(ctx, workspaceName)
}

func (environment *workspaceRewardEnvironment) LatestHistoryEntryBefore(
	ctx context.Context,
	workspaceName string,
	before time.Time,
) (workspace.HistoryEntry, bool, error) {
	return environment.workspaces.LatestWorkspaceHistoryEntryBefore(ctx, workspaceName, before)
}

func (environment *workspaceRewardEnvironment) ListHistoryEntries(
	ctx context.Context,
	workspaceName, after, through string,
	limit int,
) (workspace.HistoryEntryPage, error) {
	return environment.workspaces.ListWorkspaceHistoryEntries(ctx, workspaceName, after, through, limit)
}

func (environment *workspaceRewardEnvironment) GetHistoryEntry(
	ctx context.Context,
	workspaceName, historyID string,
) (workspace.HistoryEntry, error) {
	return environment.workspaces.GetWorkspaceHistory(ctx, workspaceName, historyID)
}

func (environment *workspaceRewardEnvironment) ResolveWorkspaceRewardPolicy(
	ctx context.Context,
	workspaceName, beneficiary string,
) (gameplay.WorkspaceRewardKind, *gameplay.WorkspaceRewardPolicySnapshot, error) {
	if environment == nil || environment.manager == nil || environment.manager.Gameplay == nil ||
		environment.manager.RuntimeProfiles == nil || environment.workspaces == nil {
		return "", nil, errors.New("gizclaw: Workspace reward resolver is not configured")
	}
	response, err := environment.workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: workspaceName})
	if err != nil {
		return "", nil, err
	}
	value, ok := response.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		return "", nil, fmt.Errorf("gizclaw: get Workspace %q returned %T", workspaceName, response)
	}
	kind, err := workspaceRewardKind(apitypes.Workspace(value))
	if err != nil {
		return "", nil, err
	}
	profile, err := environment.manager.RuntimeProfiles.ResolveOwnerProfile(ctx, beneficiary)
	if err != nil {
		return "", nil, fmt.Errorf("gizclaw: resolve reward beneficiary RuntimeProfile: %w", err)
	}
	policy, err := environment.manager.Gameplay.SnapshotWorkspaceRewardPolicy(ctx, profile, kind)
	return kind, policy, err
}

func workspaceRewardKind(item apitypes.Workspace) (gameplay.WorkspaceRewardKind, error) {
	if item.Parameters == nil {
		return gameplay.WorkspaceRewardKindWorkflow, nil
	}
	parameters, err := item.Parameters.AsChatRoomWorkspaceParameters()
	if err != nil || parameters.Mode == nil {
		return gameplay.WorkspaceRewardKindWorkflow, nil
	}
	switch *parameters.Mode {
	case apitypes.ChatRoomModeDirect:
		return gameplay.WorkspaceRewardKindDirectChatroom, nil
	case apitypes.ChatRoomModeGroup:
		return gameplay.WorkspaceRewardKindGroupChatroom, nil
	default:
		return "", fmt.Errorf("gizclaw: Workspace %q has unsupported Chatroom mode %q", item.Name, *parameters.Mode)
	}
}

func (environment *workspaceRewardEnvironment) WorkspaceRewardGenerator(
	ctx context.Context,
	beneficiary string,
	policy gameplay.WorkspaceRewardPolicySnapshot,
) (genx.Generator, error) {
	if environment == nil || environment.manager == nil {
		return nil, errors.New("gizclaw: Workspace reward generator is not configured")
	}
	models := map[string]apitypes.RuntimeProfileBinding{
		policy.ModelAlias: {ResourceId: policy.ModelResourceID},
	}
	profile := apitypes.RuntimeProfile{
		Name: policy.RuntimeProfileName, Revision: policy.RuntimeProfileRevision,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &models}},
	}
	service, err := environment.manager.ownerGenXForProfile(ctx, beneficiary, profile)
	if err != nil {
		return nil, err
	}
	return service.Generator(), nil
}

func (environment *workspaceRewardEnvironment) NotifyWorkspaceReward(
	_ context.Context,
	beneficiary string,
	update gameplay.WorkspaceRewardUpdate,
) error {
	if environment == nil || environment.manager == nil {
		return nil
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(strings.TrimSpace(beneficiary))); err != nil || publicKey.IsZero() {
		return fmt.Errorf("gizclaw: invalid reward beneficiary public key %q", beneficiary)
	}
	return environment.manager.BroadcastPeerEvent(publicKey, &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_GAMEPLAY_REWARD_UPDATED,
		Payload: &eventpb.PeerEvent_GameplayRewardUpdated{
			GameplayRewardUpdated: &eventpb.GameplayRewardUpdated{
				WorkspaceName:  update.WorkspaceName,
				RewardGrantId:  update.RewardGrantID,
				RevisionUnixMs: update.Revision.UnixMilli(),
			},
		},
	})
}

func (m *Manager) ownerGenXForProfile(
	_ context.Context,
	owner string,
	profile apitypes.RuntimeProfile,
) (*peergenx.Service, error) {
	if m == nil {
		return nil, errors.New("gizclaw: manager is not configured")
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(strings.TrimSpace(owner))); err != nil || publicKey.IsZero() {
		return nil, fmt.Errorf("gizclaw: invalid workspace owner public key %q", owner)
	}
	resources := &peerresource.Server{
		Caller: publicKey, Peers: m.Peers, Firmwares: m.Firmwares,
		Workspaces: m.Workspaces, Workflows: m.Workflows, Models: m.Models,
		Voices: m.Voices, Contacts: m.Contacts, Friends: m.Friends,
		FriendGroups: m.FriendGroups, Gameplay: m.Gameplay, Tools: m.Tools,
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	return peergenx.New(peergenx.Service{
		Models: resources, Voices: resources, Credentials: m.Credentials,
		ProviderTenants: m.ProviderTenants,
	}), nil
}

func (m *Manager) handleWorkspaceHistoryUpdated(
	ctx context.Context,
	workspaceName string,
	entry workspace.HistoryEntry,
) {
	if m == nil {
		return
	}
	if m.Gameplay != nil {
		if err := m.Gameplay.ScheduleWorkspaceRewardActivity(ctx, workspaceName, entry); err != nil {
			slog.Error("schedule Workspace reward",
				"workspace", workspaceName,
				"history_id", entry.ID,
				"error_class", "schedule",
				"error", err,
			)
		}
	}
	m.broadcastWorkspaceHistoryUpdated(ctx, workspaceName, entry.CreatedAt)
}
