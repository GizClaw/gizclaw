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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// errWorkspaceRewardNotEligible reports a Workspace whose kind has no reward
// definition, such as a Social SFU Workspace without History.
var errWorkspaceRewardNotEligible = errors.New("gizclaw: Workspace is not eligible for Workspace rewards")

type workspaceRewardEnvironment struct {
	manager    *Manager
	workspaces *workspace.Server
}

func (environment *workspaceRewardEnvironment) EnsureWorkspaceAvailable(ctx context.Context, workspaceID string) error {
	if environment == nil || environment.workspaces == nil {
		return errors.New("gizclaw: Workspace reward source is not configured")
	}
	_, err := environment.workspaces.GetAvailableWorkspaceByID(ctx, workspaceID)
	return err
}

func (environment *workspaceRewardEnvironment) LatestHistoryEntry(
	ctx context.Context,
	workspaceID string,
) (workspace.HistoryEntry, bool, error) {
	return environment.workspaces.LatestWorkspaceHistoryEntryByID(ctx, workspaceID)
}

func (environment *workspaceRewardEnvironment) LatestHistoryEntryBefore(
	ctx context.Context,
	workspaceID string,
	before time.Time,
) (workspace.HistoryEntry, bool, error) {
	return environment.workspaces.LatestWorkspaceHistoryEntryBeforeByID(ctx, workspaceID, before)
}

func (environment *workspaceRewardEnvironment) ListHistoryEntries(
	ctx context.Context,
	workspaceID, after, through string,
	limit int,
) (workspace.HistoryEntryPage, error) {
	return environment.workspaces.ListWorkspaceHistoryEntriesByID(ctx, workspaceID, after, through, limit)
}

func (environment *workspaceRewardEnvironment) GetHistoryEntry(
	ctx context.Context,
	workspaceID, historyID string,
) (workspace.HistoryEntry, error) {
	return environment.workspaces.GetWorkspaceHistoryByID(ctx, workspaceID, historyID)
}

func (environment *workspaceRewardEnvironment) ResolveWorkspaceRewardPolicy(
	ctx context.Context,
	workspaceID, beneficiary string,
) (gameplay.WorkspaceRewardKind, *gameplay.WorkspaceRewardPolicySnapshot, error) {
	if environment == nil || environment.manager == nil || environment.manager.Gameplay == nil ||
		environment.manager.RuntimeProfiles == nil || environment.workspaces == nil {
		return "", nil, errors.New("gizclaw: Workspace reward resolver is not configured")
	}
	response, err := environment.workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: workspaceID})
	if err != nil {
		return "", nil, err
	}
	value, ok := response.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		return "", nil, fmt.Errorf("gizclaw: get Workspace %q returned %T", workspaceID, response)
	}
	item := apitypes.Workspace(value)
	kind, err := workspaceRewardKind(item)
	if errors.Is(err, errWorkspaceRewardNotEligible) {
		return "", nil, nil
	}
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

// workspaceRewardKind classifies a Workspace for reward evaluation. Social
// SFU Workspaces are bound to the built-in SFU Workflow, have no History, and
// therefore report errWorkspaceRewardNotEligible.
func workspaceRewardKind(item apitypes.Workspace) (gameplay.WorkspaceRewardKind, error) {
	if item.WorkflowId == socialutil.SFUWorkflowID {
		return "", fmt.Errorf("%w: Workspace %q uses the SFU Workflow", errWorkspaceRewardNotEligible, item.Name)
	}
	return gameplay.WorkspaceRewardKindWorkflow, nil
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
		Id: policy.RuntimeProfileId, Revision: policy.RuntimeProfileRevision,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &models}},
	}
	service, err := environment.manager.ownerGenXForProfile(ctx, beneficiary, profile)
	if err != nil {
		return nil, err
	}
	return service.Generator(), nil
}

func (environment *workspaceRewardEnvironment) NotifyWorkspaceReward(
	ctx context.Context,
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
	response, err := environment.workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: update.WorkspaceID})
	if err != nil {
		return err
	}
	value, ok := response.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		return fmt.Errorf("gizclaw: get Workspace %q returned %T", update.WorkspaceID, response)
	}
	return environment.manager.BroadcastPeerEvent(publicKey, &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_GAMEPLAY_REWARD_UPDATED,
		Payload: &eventpb.PeerEvent_GameplayRewardUpdated{
			GameplayRewardUpdated: &eventpb.GameplayRewardUpdated{
				WorkspaceName:   value.Name,
				RewardGrantName: update.RewardGrantID,
				RevisionUnixMs:  update.Revision.UnixMilli(),
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
	workspaceID string,
	entry workspace.HistoryEntry,
) {
	if m == nil {
		return
	}
	if m.Gameplay != nil {
		workspace, resolveErr := resolveWorkspaceByID(ctx, m.Workspaces, workspaceID)
		if resolveErr == nil {
			resolveErr = m.Gameplay.EnqueueWorkspaceRewardActivity(workspace.Id, entry)
		}
		if resolveErr != nil {
			slog.Error("enqueue Workspace reward",
				"workspace", workspaceID,
				"history_id", entry.ID,
				"error_class", "enqueue",
				"error", resolveErr,
			)
		}
	}
	m.broadcastWorkspaceHistoryUpdated(ctx, workspaceID, entry.CreatedAt)
}

// handleWorkspaceActivated registers a freshly activated Workspace as a
// reward source. The caller resolves the canonical record through the
// activating Peer's own access path; Social SFU Workspaces are not reward
// eligible and return nil without touching Gameplay.
func (m *Manager) handleWorkspaceActivated(ctx context.Context, item apitypes.Workspace) error {
	if m == nil || m.Gameplay == nil {
		return nil
	}
	if _, err := workspaceRewardKind(item); err != nil {
		if errors.Is(err, errWorkspaceRewardNotEligible) {
			return nil
		}
		return err
	}
	return m.Gameplay.EnqueueWorkspaceRewardActivation(ctx, item.Id)
}
