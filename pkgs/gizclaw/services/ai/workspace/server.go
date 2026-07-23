package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

var (
	workspacesRoot        = kv.Key{"by-name"}
	workflowsRoot         = kv.Key{"by-name"}
	workspacesByOwnerRoot = kv.Key{"by-owner"}
)

const (
	defaultListLimit                   = 50
	maxListLimit                       = 200
	maxWorkspaceLabels                 = 32
	maxWorkspaceLabelKeyBytes          = 63
	maxWorkspaceLabelValueBytes        = 128
	SystemWorkspaceDeleteForbiddenCode = "SYSTEM_WORKSPACE_DELETE_FORBIDDEN"
	SystemWorkspaceUpdateForbiddenCode = "SYSTEM_WORKSPACE_UPDATE_FORBIDDEN"
)

type Server struct {
	Store         kv.Store
	WorkflowStore kv.Store
	Models        ModelService
	RuntimeStore  RuntimeStore
	Assets        objectstore.ObjectStore
	IconLocks     iconasset.Locker
}

type ModelService interface {
	GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error)
}

type runtimeWorkflowBindingsContextKey struct{}
type runtimeModelBindingsContextKey struct{}
type runtimeVoiceBindingsContextKey struct{}

// WithRuntimeWorkflowBindings attaches the authenticated connection's current
// RuntimeProfile Workflow alias snapshot to Workspace validation.
func WithRuntimeWorkflowBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	for alias, name := range bindings {
		cloned[alias] = name
	}
	return context.WithValue(ctx, runtimeWorkflowBindingsContextKey{}, cloned)
}

// WithRuntimeModelBindings attaches the same RuntimeProfile snapshot's Model
// alias bindings so Workspace overrides can be validated before persistence.
func WithRuntimeModelBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	for alias, name := range bindings {
		cloned[alias] = name
	}
	return context.WithValue(ctx, runtimeModelBindingsContextKey{}, cloned)
}

// WithRuntimeVoiceBindings attaches the same RuntimeProfile snapshot's Voice
// alias bindings so Workspace overrides can be validated before persistence.
func WithRuntimeVoiceBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	for alias, name := range bindings {
		cloned[alias] = name
	}
	return context.WithValue(ctx, runtimeVoiceBindingsContextKey{}, cloned)
}

type WorkspaceAdminService interface {
	ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error)
	CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error)
	DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error)
	GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error)
	PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error)
}

// SystemWorkspaceService is the domain-only Workspace lifecycle surface. It is
// intentionally not registered in Admin HTTP, Peer RPC, or resource manager
// operations.
type SystemWorkspaceService interface {
	CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error)
	DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error)
}

// WorkspaceLifecycleService combines the public administration surface with
// the domain-only system Workspace lifecycle surface.
type WorkspaceLifecycleService interface {
	WorkspaceAdminService
	SystemWorkspaceService
}

var _ WorkspaceAdminService = (*Server)(nil)
var _ WorkspaceLifecycleService = (*Server)(nil)

type WorkspaceIconAdminService interface {
	DownloadWorkspaceIcon(context.Context, adminhttp.DownloadWorkspaceIconRequestObject) (adminhttp.DownloadWorkspaceIconResponseObject, error)
	UploadWorkspaceIcon(context.Context, adminhttp.UploadWorkspaceIconRequestObject) (adminhttp.UploadWorkspaceIconResponseObject, error)
	DeleteWorkspaceIcon(context.Context, adminhttp.DeleteWorkspaceIconRequestObject) (adminhttp.DeleteWorkspaceIconResponseObject, error)
}

var _ WorkspaceIconAdminService = (*Server)(nil)

func (s *Server) ListWorkspaces(ctx context.Context, request adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListWorkspaces500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	selector, err := parseLabelSelector(request.Params.Label)
	if err != nil {
		return adminhttp.ListWorkspaces400JSONResponse(apitypes.NewErrorResponse("INVALID_PARAMS", err.Error())), nil
	}
	items, hasNext, nextCursor, err := listWorkspacePage(ctx, store, workspacesRoot, cursor, limit, selector)
	if err != nil {
		return adminhttp.ListWorkspaces500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListWorkspaces200JSONResponse(adminhttp.WorkspaceList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

// ListWorkspacesByOwner reads the immutable owner index used by Peer RPC.
// System Workspaces are intentionally absent and are added through their
// Friend, FriendGroup, and Pet domain relationships.
func (s *Server) ListWorkspacesByOwner(ctx context.Context, owner string) ([]apitypes.Workspace, error) {
	return s.ListWorkspacesByOwnerAndLabels(ctx, owner, nil)
}

// ListWorkspacesByOwnerAndLabels returns owner Workspaces whose stored labels
// contain every exact key/value pair in selector.
func (s *Server) ListWorkspacesByOwnerAndLabels(ctx context.Context, owner string, selector map[string]string) ([]apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return []apitypes.Workspace{}, nil
	}
	prefix := workspaceByOwnerPrefix(owner)
	items := make([]apitypes.Workspace, 0)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, fmt.Errorf("workspace: list owner %s: %w", owner, err)
		}
		if len(entry.Key) == 0 {
			continue
		}
		name := unescapeStoreSegment(entry.Key[len(entry.Key)-1])
		item, err := getWorkspace(ctx, store, name)
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !workspaceMatchesLabels(item, selector) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) CreateWorkspace(ctx context.Context, request adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	if request.Body.Icon != nil {
		return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "icon object names are managed by the icon API")), nil
	}
	normalized, err := normalizeWorkspaceUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	unlock := s.IconLocks.LockRecord(string(normalized.Name))
	defer unlock()
	workflowStore, err := s.workflowStore()
	if err != nil {
		return adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.validateReferences(ctx, workflowStore, normalized); err != nil {
		if isInvalidWorkspaceReference(err) {
			return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
		}
		return adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if _, err := store.Get(ctx, workspaceKey(string(normalized.Name))); err == nil {
		return adminhttp.CreateWorkspace409JSONResponse(apitypes.NewErrorResponse("WORKSPACE_ALREADY_EXISTS", fmt.Sprintf("workspace %q already exists", normalized.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	workspace, err := s.createWorkspaceRecord(ctx, store, normalized, false)
	if err != nil {
		return adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateWorkspace200JSONResponse(workspace), nil
}

func (s *Server) CreateSystemWorkspace(ctx context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	owner, ok := ownership.FromContext(ctx)
	if !ok || strings.TrimSpace(owner) == "" {
		return apitypes.Workspace{}, false, errors.New("workspace: system Workspace owner is required")
	}
	owner = strings.TrimSpace(owner)
	ctx = ownership.WithOwner(ctx, owner)
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	normalized, err := normalizeWorkspaceUpsert(body, "")
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	unlock := s.IconLocks.LockRecord(string(normalized.Name))
	defer unlock()
	workflowStore, err := s.workflowStore()
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	if err := s.validateReferences(ctx, workflowStore, normalized); err != nil {
		return apitypes.Workspace{}, false, err
	}
	existing, err := getWorkspace(ctx, store, string(normalized.Name))
	if err == nil {
		if !workspaceIsSystem(existing) {
			return apitypes.Workspace{}, false, fmt.Errorf("workspace %q already exists as a user Workspace", normalized.Name)
		}
		if !systemWorkspaceMatches(existing, normalized, owner) {
			return apitypes.Workspace{}, false, fmt.Errorf("workspace %q already exists with different owner, Workflow, or domain binding", normalized.Name)
		}
		return existing, false, nil
	}
	if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, false, err
	}
	workspace, err := s.createWorkspaceRecord(ctx, store, normalized, true)
	return workspace, err == nil, err
}

func (s *Server) createWorkspaceRecord(ctx context.Context, store kv.Store, normalized adminhttp.WorkspaceUpsert, system bool) (apitypes.Workspace, error) {
	now := time.Now().UTC()
	workspace := apitypes.Workspace{
		CreatedAt:    now,
		LastActiveAt: now,
		Labels:       cloneLabelsOrEmpty(normalized.Labels),
		Name:         normalized.Name,
		Parameters:   cloneParameters(normalized.Parameters),
		System:       boolPointer(system),
		Toolkit:      cloneToolkitPolicy(normalized.Toolkit),
		UpdatedAt:    now,
		WorkflowName: normalized.WorkflowName,
	}
	if owner, ok := ownership.FromContext(ctx); ok {
		workspace.OwnerPublicKey = &owner
	}
	if s.RuntimeStore != nil {
		if _, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Name); err != nil {
			return apitypes.Workspace{}, err
		}
	}
	if err := writeWorkspace(ctx, store, workspace); err != nil {
		return apitypes.Workspace{}, err
	}
	return workspace, nil
}

func (s *Server) DeleteWorkspace(ctx context.Context, request adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	unlock := s.IconLocks.LockOwner(name)
	defer unlock()
	workspace, err := getWorkspace(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", name))), nil
		}
		return adminhttp.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if workspaceIsSystem(workspace) {
		return adminhttp.DeleteWorkspace409JSONResponse(apitypes.NewErrorResponse(
			SystemWorkspaceDeleteForbiddenCode,
			fmt.Sprintf("system workspace %q cannot be deleted through the generic Workspace lifecycle", workspace.Name),
		)), nil
	}
	if err := s.fastDeleteWorkspaceRecord(ctx, store, workspace); err != nil {
		return adminhttp.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteWorkspace200JSONResponse(workspace), nil
}

func (s *Server) DeleteSystemWorkspace(ctx context.Context, name string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	name = strings.TrimSpace(name)
	unlock := s.IconLocks.LockOwner(name)
	defer unlock()
	workspace, err := getWorkspace(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) && s.RuntimeStore != nil {
			if cleanupErr := s.RuntimeStore.DeleteWorkspaceRuntime(ctx, name); cleanupErr != nil {
				return apitypes.Workspace{}, cleanupErr
			}
		}
		return apitypes.Workspace{}, err
	}
	if !workspaceIsSystem(workspace) {
		return apitypes.Workspace{}, fmt.Errorf("workspace %q is not a system Workspace", name)
	}
	if err := s.deleteWorkspaceRecord(ctx, store, workspace); err != nil {
		return apitypes.Workspace{}, err
	}
	return workspace, nil
}

func (s *Server) deleteWorkspaceRecord(ctx context.Context, store kv.Store, workspace apitypes.Workspace) error {
	if workspace.Icon != nil && s.Assets == nil {
		return errors.New("workspace asset store not configured")
	}
	if s.Assets != nil {
		for _, format := range []iconasset.Format{iconasset.FormatPixa, iconasset.FormatPNG} {
			if err := s.Assets.Delete(iconasset.ObjectName(string(workspace.Name), format)); err != nil {
				return errors.New("failed to delete workspace icon")
			}
		}
	}
	if s.RuntimeStore != nil {
		if err := s.RuntimeStore.DeleteWorkspaceRuntime(ctx, workspace.Name); err != nil {
			return err
		}
	}
	keys := []kv.Key{workspaceKey(string(workspace.Name))}
	if workspace.OwnerPublicKey != nil && !workspaceIsSystem(workspace) {
		keys = append(keys, workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name))
	}
	return store.BatchDelete(ctx, keys)
}

func (s *Server) fastDeleteWorkspaceRecord(ctx context.Context, store kv.Store, workspace apitypes.Workspace) error {
	descriptor := struct {
		Name           string  `json:"name"`
		OwnerPublicKey *string `json:"owner_public_key,omitempty"`
		HasIcon        bool    `json:"has_icon"`
	}{
		Name:           workspace.Name,
		OwnerPublicKey: cloneString(workspace.OwnerPublicKey),
		HasIcon:        workspace.Icon != nil,
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace,
		workspace.Name,
		workspace.OwnerPublicKey,
		pendingdeletion.ReasonResourceDelete,
		descriptor,
		time.Now(),
	)
	if err != nil {
		return err
	}
	_, _, err = pendingdeletion.CreateOrGet(ctx, store, record)
	return err
}

func (s *Server) GetWorkspace(ctx context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	workspace, err := getWorkspace(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", name))), nil
		}
		return adminhttp.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetWorkspace200JSONResponse(workspace), nil
}

func (s *Server) GetWorkspaceRuntime(ctx context.Context, name string) (Runtime, error) {
	if s == nil || s.RuntimeStore == nil {
		return Runtime{}, nil
	}
	return s.RuntimeStore.GetWorkspaceRuntime(ctx, name)
}

func (s *Server) PutWorkspace(ctx context.Context, request adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	normalized, err := normalizeWorkspaceUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	store, err := s.store()
	if err != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	unlock := s.IconLocks.LockRecord(name)
	defer unlock()
	previous, previousErr := getWorkspace(ctx, store, name)
	if previousErr != nil && !errors.Is(previousErr, kv.ErrNotFound) {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", previousErr.Error())), nil
	}
	if previousErr == nil && workspaceIsSystem(previous) &&
		!systemWorkspaceAllowsInputUpdate(previous, normalized) {
		return adminhttp.PutWorkspace409JSONResponse(apitypes.NewErrorResponse(
			SystemWorkspaceUpdateForbiddenCode,
			fmt.Sprintf("system workspace %q only permits changing the chat input mode", previous.Name),
		)), nil
	}
	workflowStore, err := s.workflowStore()
	if err != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.validateReferences(ctx, workflowStore, normalized); err != nil {
		if isInvalidWorkspaceReference(err) {
			return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
		}
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := iconasset.ValidateProjection(previous.Icon, request.Body.Icon); err != nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	now := time.Now().UTC()
	workspace := apitypes.Workspace{
		CreatedAt:    now,
		LastActiveAt: now,
		Labels:       cloneLabelsOrEmpty(normalized.Labels),
		Name:         normalized.Name,
		Parameters:   cloneParameters(normalized.Parameters),
		System:       boolPointer(false),
		Toolkit:      cloneToolkitPolicy(normalized.Toolkit),
		UpdatedAt:    now,
		WorkflowName: normalized.WorkflowName,
		Icon:         previous.Icon,
	}
	if previousErr == nil {
		workspace.CreatedAt = previous.CreatedAt
		workspace.LastActiveAt = previous.LastActiveAt
		workspace.System = previous.System
		workspace.OwnerPublicKey = cloneString(previous.OwnerPublicKey)
		if normalized.Labels == nil {
			workspace.Labels = cloneLabelsOrEmpty(previous.Labels)
		}
	}
	if previousErr != nil {
		if owner, ok := ownership.FromContext(ctx); ok {
			workspace.OwnerPublicKey = &owner
		}
	}
	if s.RuntimeStore != nil {
		if _, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Name); err != nil {
			return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	}
	if err := writeWorkspace(ctx, store, workspace); err != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutWorkspace200JSONResponse(workspace), nil
}

func writeWorkspace(ctx context.Context, store kv.Store, workspace apitypes.Workspace) error {
	data, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("workspace: encode %s: %w", workspace.Name, err)
	}
	entries := []kv.Entry{{Key: workspaceKey(string(workspace.Name)), Value: data}}
	if workspace.OwnerPublicKey != nil && !workspaceIsSystem(workspace) {
		entries = append(entries, kv.Entry{Key: workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name), Value: []byte{}})
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("workspace: write %s: %w", workspace.Name, err)
	}
	return nil
}

func systemWorkspaceMatches(existing apitypes.Workspace, desired adminhttp.WorkspaceUpsert, owner string) bool {
	return existing.OwnerPublicKey != nil &&
		strings.TrimSpace(*existing.OwnerPublicKey) == owner &&
		existing.WorkflowName == desired.WorkflowName &&
		reflect.DeepEqual(existing.Labels, cloneLabelsOrEmpty(desired.Labels)) &&
		systemWorkspaceDomainParametersMatch(existing.Parameters, desired.Parameters) &&
		reflect.DeepEqual(existing.Toolkit, cloneToolkitPolicy(desired.Toolkit))
}

func systemWorkspaceAllowsInputUpdate(existing apitypes.Workspace, desired adminhttp.WorkspaceUpsert) bool {
	desiredLabels := cloneLabelsOrEmpty(desired.Labels)
	if desired.Labels == nil {
		desiredLabels = cloneLabelsOrEmpty(existing.Labels)
	}
	return existing.WorkflowName == desired.WorkflowName &&
		reflect.DeepEqual(existing.Labels, desiredLabels) &&
		reflect.DeepEqual(existing.Toolkit, cloneToolkitPolicy(desired.Toolkit)) &&
		systemWorkspaceDomainParametersMatch(existing.Parameters, desired.Parameters)
}

func systemWorkspaceDomainParametersMatch(existing, desired *apitypes.WorkspaceParameters) bool {
	if existing == nil || desired == nil {
		return existing == nil && desired == nil
	}
	existingChatroom, existingErr := existing.AsChatRoomWorkspaceParameters()
	desiredChatroom, desiredErr := desired.AsChatRoomWorkspaceParameters()
	existingIsChatroom := existingErr == nil &&
		existingChatroom.AgentType == apitypes.ChatRoomWorkspaceParametersAgentTypeChatroom
	desiredIsChatroom := desiredErr == nil &&
		desiredChatroom.AgentType == apitypes.ChatRoomWorkspaceParametersAgentTypeChatroom
	if existingIsChatroom || desiredIsChatroom {
		return existingIsChatroom &&
			desiredIsChatroom &&
			reflect.DeepEqual(existingChatroom.Mode, desiredChatroom.Mode) &&
			reflect.DeepEqual(existingChatroom.History, desiredChatroom.History) &&
			reflect.DeepEqual(existingChatroom.Transcript, desiredChatroom.Transcript)
	}
	return reflect.DeepEqual(existing, desired)
}

func getWorkspace(ctx context.Context, store kv.Store, name string) (apitypes.Workspace, error) {
	data, err := store.Get(ctx, workspaceKey(name))
	if err != nil {
		return apitypes.Workspace{}, err
	}
	var workspace apitypes.Workspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: decode %s: %w", name, err)
	}
	return normalizeWorkspaceTimestamps(workspace), nil
}

func listWorkspacePage(ctx context.Context, store kv.Store, prefix kv.Key, cursor string, limit int, selector map[string]string) ([]apitypes.Workspace, bool, *string, error) {
	items := make([]apitypes.Workspace, 0, limit+1)
	keys := make([]string, 0, limit+1)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, false, nil, err
		}
		if len(entry.Key) == 0 {
			continue
		}
		key := entry.Key[len(entry.Key)-1]
		if cursor != "" && key <= cursor {
			continue
		}
		var workspace apitypes.Workspace
		if err := json.Unmarshal(entry.Value, &workspace); err != nil {
			return nil, false, nil, fmt.Errorf("workspace: decode list %s: %w", entry.Key.String(), err)
		}
		workspace = normalizeWorkspaceTimestamps(workspace)
		if !workspaceMatchesLabels(workspace, selector) {
			continue
		}
		items = append(items, workspace)
		keys = append(keys, key)
		if len(items) > limit {
			break
		}
	}
	if len(items) <= limit {
		return items, false, nil, nil
	}
	items = items[:limit]
	nextCursor := keys[limit-1]
	return items, true, &nextCursor, nil
}

func normalizeWorkspaceTimestamps(workspace apitypes.Workspace) apitypes.Workspace {
	if workspace.System == nil {
		workspace.System = boolPointer(false)
	}
	workspace.Labels = cloneLabelsOrEmpty(workspace.Labels)
	if workspace.LastActiveAt.IsZero() {
		workspace.LastActiveAt = workspace.CreatedAt
	}
	if workspace.LastActiveAt.IsZero() {
		workspace.LastActiveAt = workspace.UpdatedAt
	}
	return workspace
}

func workspaceIsSystem(workspace apitypes.Workspace) bool {
	return workspace.System != nil && *workspace.System
}

func boolPointer(value bool) *bool {
	return &value
}

func normalizeWorkspaceUpsert(in adminhttp.WorkspaceUpsert, expectedName string) (adminhttp.WorkspaceUpsert, error) {
	name := string(in.Name)
	if err := customid.ValidateField("name", name); err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
	if expectedName != "" {
		if err := customid.ValidateField("path name", expectedName); err != nil {
			return adminhttp.WorkspaceUpsert{}, err
		}
		if name != expectedName {
			return adminhttp.WorkspaceUpsert{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
		}
	}
	workflowName := string(in.WorkflowName)
	if err := validateWorkspaceWorkflowName(workflowName); err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
	policy, err := toolkit.NormalizePolicy(in.Toolkit)
	if err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
	labels, err := normalizeWorkspaceLabels(in.Labels)
	if err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
	return adminhttp.WorkspaceUpsert{
		Labels:       labels,
		Name:         string(name),
		Parameters:   cloneParameters(in.Parameters),
		Toolkit:      policy,
		WorkflowName: string(workflowName),
	}, nil
}

func validateWorkspaceWorkflowName(value string) error {
	if err := customid.ValidateField("workflow_name", value); err == nil {
		return nil
	}
	return runtimeprofile.ValidateAlias("workflow_name", value)
}

func normalizeWorkspaceLabels(labels *map[string]string) (*map[string]string, error) {
	if labels == nil {
		return nil, nil
	}
	if len(*labels) > maxWorkspaceLabels {
		return nil, fmt.Errorf("labels: maximum is %d", maxWorkspaceLabels)
	}
	cloned := make(map[string]string, len(*labels))
	for key, value := range *labels {
		if err := validateWorkspaceLabel(key, value); err != nil {
			return nil, err
		}
		cloned[key] = value
	}
	return &cloned, nil
}

func validateWorkspaceLabel(key, value string) error {
	if len(key) == 0 || len(key) > maxWorkspaceLabelKeyBytes {
		return fmt.Errorf("labels: key length must be 1-%d bytes", maxWorkspaceLabelKeyBytes)
	}
	if key[0] < 'a' || key[0] > 'z' {
		return fmt.Errorf("labels: key %q must start with a lowercase ASCII letter", key)
	}
	for index := range len(key) {
		character := key[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return fmt.Errorf("labels: key %q contains an invalid character", key)
	}
	last := key[len(key)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return fmt.Errorf("labels: key %q must end with a lowercase ASCII letter or digit", key)
	}
	if len(value) == 0 || len(value) > maxWorkspaceLabelValueBytes {
		return fmt.Errorf("labels[%q]: value length must be 1-%d UTF-8 bytes", key, maxWorkspaceLabelValueBytes)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("labels[%q]: value must be valid UTF-8", key)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("labels[%q]: value must not have leading or trailing whitespace", key)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("labels[%q]: value must not contain control characters", key)
		}
	}
	return nil
}

func parseLabelSelector(values *[]string) (map[string]string, error) {
	if values == nil {
		return nil, nil
	}
	selector := make(map[string]string, len(*values))
	for _, expression := range *values {
		key, value, ok := strings.Cut(expression, "=")
		if !ok {
			return nil, fmt.Errorf("label selector %q must use key=value", expression)
		}
		if err := validateWorkspaceLabel(key, value); err != nil {
			return nil, err
		}
		if previous, exists := selector[key]; exists && previous != value {
			return nil, fmt.Errorf("label selector %q has conflicting values", key)
		}
		selector[key] = value
	}
	return selector, nil
}

func workspaceMatchesLabels(workspace apitypes.Workspace, selector map[string]string) bool {
	if len(selector) == 0 {
		return true
	}
	if workspace.Labels == nil {
		return false
	}
	for key, value := range selector {
		if (*workspace.Labels)[key] != value {
			return false
		}
	}
	return true
}

func (s *Server) validateReferences(ctx context.Context, store kv.Store, workspace adminhttp.WorkspaceUpsert) error {
	workflowName, runtimeAlias, err := resolveWorkflowReference(ctx, workspace)
	if err != nil {
		return err
	}
	data, err := store.Get(ctx, workflowReferenceKey(workflowName))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return invalidWorkspaceReference("workflow %q not found", workflowName)
		}
		return err
	}
	var workflow apitypes.Workflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		return fmt.Errorf("decode workflow %q: %w", workflowName, err)
	}
	if workflow.Spec.Driver == apitypes.WorkflowDriverAstTranslate && runtimeAlias {
		return s.validateASTTranslateOverrides(ctx, workspace.Parameters)
	}
	if workflow.Spec.Driver == apitypes.WorkflowDriverDoubaoRealtime {
		return validateDoubaoRealtimeOverrides(workspace.Parameters)
	}
	if workflow.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		return nil
	}
	references, err := ResolveFlowcraftModelReferences(workflow, workspace.Parameters)
	if err != nil {
		return err
	}
	// Flowcraft model fields are RuntimeProfile aliases, including when a
	// Workspace names the Workflow resource directly. A direct Workspace write
	// has no authoritative owner RuntimeProfile in this request, so resource
	// existence and model kinds are validated when that profile resolves the
	// Workflow. Treating aliases as Model IDs here would reject valid Graphs and
	// couple reusable Workflow resources to one deployment's model catalog.
	if !runtimeAlias {
		return nil
	}
	for _, reference := range references {
		modelID := reference.ModelID
		visibleModelID := modelID
		bindings, present := ctx.Value(runtimeModelBindingsContextKey{}).(map[string]string)
		if !present {
			return errors.New("runtime model bindings not configured")
		}
		modelID = strings.TrimSpace(bindings[reference.ModelID])
		if modelID == "" {
			return invalidWorkspaceReference("flowcraft parameter %q references missing runtime Model alias %q", reference.Role, reference.ModelID)
		}
		if err := s.validateModelKind(ctx, "flowcraft parameter", reference.Role, modelID, visibleModelID, reference.Kind); err != nil {
			return err
		}
	}
	return nil
}

func validateDoubaoRealtimeOverrides(workspaceParameters *apitypes.WorkspaceParameters) error {
	if workspaceParameters == nil {
		return nil
	}
	parameters, err := workspaceParameters.AsDoubaoRealtimeWorkspaceParameters()
	if err != nil {
		return invalidWorkspaceReference("doubao_realtime parameters are required: %v", err)
	}
	if parameters.Tools != nil && len(*parameters.Tools) != 0 {
		return invalidWorkspaceReference("doubao_realtime parameters.tools are unsupported until ToolCall is implemented")
	}
	return nil
}

func (s *Server) validateASTTranslateOverrides(ctx context.Context, workspaceParameters *apitypes.WorkspaceParameters) error {
	if workspaceParameters == nil {
		return nil
	}
	parameters, err := workspaceParameters.AsASTTranslateWorkspaceParameters()
	if err != nil {
		return invalidWorkspaceReference("ast-translate parameters are required: %v", err)
	}
	if parameters.TranslationModel != nil {
		alias := strings.TrimSpace(*parameters.TranslationModel)
		if alias != "" {
			bindings, present := ctx.Value(runtimeModelBindingsContextKey{}).(map[string]string)
			if !present {
				return errors.New("runtime model bindings not configured")
			}
			modelID := strings.TrimSpace(bindings[alias])
			if modelID == "" {
				return invalidWorkspaceReference("ast-translate parameter %q references missing runtime Model alias %q", "translation_model", alias)
			}
			if err := s.validateModelKind(ctx, "ast-translate parameter", "translation_model", modelID, alias, apitypes.ModelKindTranslation); err != nil {
				return err
			}
		}
	}
	if parameters.Voice == nil {
		return nil
	}
	external, err := parameters.Voice.AsASTTranslateExternalVoiceParameters()
	if err != nil {
		return invalidWorkspaceReference("ast-translate voice parameters are invalid: %v", err)
	}
	alias := strings.TrimSpace(external.TtsVoice)
	if alias == "" {
		return nil
	}
	bindings, present := ctx.Value(runtimeVoiceBindingsContextKey{}).(map[string]string)
	if !present {
		return errors.New("runtime voice bindings not configured")
	}
	if strings.TrimSpace(bindings[alias]) == "" {
		return invalidWorkspaceReference("ast-translate parameter %q references missing runtime Voice alias %q", "voice.tts_voice", alias)
	}
	return nil
}

func resolveWorkflowReference(ctx context.Context, workspace adminhttp.WorkspaceUpsert) (string, bool, error) {
	name := string(workspace.WorkflowName)
	if bindings, present := ctx.Value(runtimeWorkflowBindingsContextKey{}).(map[string]string); present {
		resolved := strings.TrimSpace(bindings[name])
		if resolved == "" {
			return "", false, invalidWorkspaceReference("runtime workflow alias %q not found", name)
		}
		return resolved, true, nil
	}
	return name, false, nil
}

// FlowcraftModelReference is one effective Model selected for a FlowCraft role.
type FlowcraftModelReference struct {
	Role    string
	ModelID string
	Kind    apitypes.ModelKind
}

// ResolveFlowcraftModelReferences resolves Workspace overrides and Workflow
// settings into the concrete Models used by a FlowCraft runtime.
func ResolveFlowcraftModelReferences(workflow apitypes.Workflow, workspaceParameters *apitypes.WorkspaceParameters) ([]FlowcraftModelReference, error) {
	if workflow.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		return nil, nil
	}
	if workspaceParameters != nil {
		_, err := workspaceParameters.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, invalidWorkspaceReference("flowcraft parameters are required: %v", err)
		}
	}
	if workflow.Spec.Flowcraft == nil {
		return nil, invalidWorkspaceReference("flowcraft workflow config is required")
	}
	configured := *workflow.Spec.Flowcraft
	references := make([]FlowcraftModelReference, 0, len(configured.Agent.Graph.Nodes)+3)
	for index, raw := range configured.Agent.Graph.Nodes {
		if discriminator, _ := raw.Discriminator(); discriminator == "llm" {
			node, err := raw.AsFlowcraftLLMNode()
			if err != nil {
				return nil, invalidWorkspaceReference("flowcraft graph node %d is invalid: %v", index, err)
			}
			references = append(references, FlowcraftModelReference{
				Role: fmt.Sprintf("agent.graph.nodes[%d].config.model", index), ModelID: node.Config.Model, Kind: apitypes.ModelKindLlm,
			})
		}
	}
	if configured.Memory != nil && configured.Memory.Enabled {
		if configured.Memory.Extract != nil && (configured.Memory.Extract.Enabled == nil || *configured.Memory.Extract.Enabled) {
			if alias := stringPointerValue(configured.Memory.Extract.Model); alias != "" {
				references = append(references, FlowcraftModelReference{Role: "memory.extract.model", ModelID: alias, Kind: apitypes.ModelKindLlm})
			}
		}
		if configured.Memory.Embedding != nil && configured.Memory.Embedding.Enabled != nil && *configured.Memory.Embedding.Enabled {
			if alias := stringPointerValue(configured.Memory.Embedding.Model); alias != "" {
				references = append(references, FlowcraftModelReference{Role: "memory.embedding.model", ModelID: alias, Kind: apitypes.ModelKindEmbedding})
			}
		}
		if configured.Memory.Rerank != nil && configured.Memory.Rerank.Enabled != nil && *configured.Memory.Rerank.Enabled {
			if alias := stringPointerValue(configured.Memory.Rerank.Model); alias != "" {
				references = append(references, FlowcraftModelReference{Role: "memory.rerank.model", ModelID: alias, Kind: apitypes.ModelKindLlm})
			}
		}
	}
	if configured.VoiceAdapter != nil {
		if alias := stringPointerValue(configured.VoiceAdapter.AsrModel); alias != "" {
			references = append(references, FlowcraftModelReference{Role: "voice_adapter.asr_model", ModelID: alias, Kind: apitypes.ModelKindAsr})
		}
	}
	return references, nil
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (s *Server) validateModelKind(ctx context.Context, subject, role, modelID, visibleModelID string, want apitypes.ModelKind) error {
	if s == nil || s.Models == nil {
		return errors.New("model service not configured")
	}
	response, err := s.Models.GetModel(ctx, adminhttp.GetModelRequestObject{Id: modelID})
	if err != nil {
		return err
	}
	model, ok := response.(adminhttp.GetModel200JSONResponse)
	if _, missing := response.(adminhttp.GetModel404JSONResponse); missing {
		return invalidWorkspaceReference("%s %q references missing Model %q", subject, role, visibleModelID)
	}
	if !ok {
		return fmt.Errorf("validate %s %q Model %q: model service returned %T", subject, role, visibleModelID, response)
	}
	if model.Kind != want {
		return invalidWorkspaceReference("%s %q Model %q has kind %q, want %q", subject, role, visibleModelID, model.Kind, want)
	}
	return nil
}

type invalidWorkspaceReferenceError struct {
	error
}

func invalidWorkspaceReference(format string, args ...any) error {
	return invalidWorkspaceReferenceError{error: fmt.Errorf(format, args...)}
}

func isInvalidWorkspaceReference(err error) bool {
	var invalid invalidWorkspaceReferenceError
	return errors.As(err, &invalid)
}

func workspaceKey(name string) kv.Key {
	return append(append(kv.Key{}, workspacesRoot...), escapeStoreSegment(name))
}

func workspaceByOwnerKey(owner, name string) kv.Key {
	return append(workspaceByOwnerPrefix(owner), escapeStoreSegment(name))
}

func workspaceByOwnerPrefix(owner string) kv.Key {
	return append(append(kv.Key{}, workspacesByOwnerRoot...), escapeStoreSegment(owner))
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneLabelsOrEmpty(labels *map[string]string) *map[string]string {
	cloned := make(map[string]string)
	if labels != nil {
		cloned = make(map[string]string, len(*labels))
		for key, value := range *labels {
			cloned[key] = value
		}
	}
	return &cloned
}

func workflowReferenceKey(name string) kv.Key {
	return append(append(kv.Key{}, workflowsRoot...), escapeStoreSegment(name))
}

func escapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func unescapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%3A", ":")
	return strings.ReplaceAll(value, "%25", "%")
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	nextCursor := ""
	if cursor != nil {
		nextCursor = string(*cursor)
	}
	nextLimit := defaultListLimit
	if limit != nil {
		nextLimit = int(*limit)
	}
	if nextLimit <= 0 {
		nextLimit = defaultListLimit
	}
	if nextLimit > maxListLimit {
		nextLimit = maxListLimit
	}
	return nextCursor, nextLimit
}

func cursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	after := append(kv.Key{}, prefix...)
	return append(after, cursor)
}

func paginateEntries(entries []kv.Entry, limit int) ([]kv.Entry, bool, *string) {
	if len(entries) == 0 {
		return nil, false, nil
	}
	hasNext := len(entries) > limit
	if !hasNext {
		return entries, false, nil
	}
	page := entries[:limit]
	if len(page) == 0 || len(page[len(page)-1].Key) == 0 {
		return page, true, nil
	}
	nextCursor := page[len(page)-1].Key[len(page[len(page)-1].Key)-1]
	return page, true, &nextCursor
}

func cloneParameters(parameters *apitypes.WorkspaceParameters) *apitypes.WorkspaceParameters {
	if parameters == nil {
		return nil
	}
	data, err := parameters.MarshalJSON()
	if err != nil {
		return nil
	}
	var cloned apitypes.WorkspaceParameters
	if err := cloned.UnmarshalJSON(data); err != nil {
		return nil
	}
	return &cloned
}

func cloneToolkitPolicy(policy *apitypes.ToolkitPolicy) *apitypes.ToolkitPolicy {
	if policy == nil {
		return nil
	}
	cloned := *policy
	if policy.ToolIds != nil {
		ids := append([]string(nil), (*policy.ToolIds)...)
		cloned.ToolIds = &ids
	}
	return &cloned
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("workspace store not configured")
	}
	return s.Store, nil
}

func (s *Server) workflowStore() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("workflow store not configured")
	}
	if s.WorkflowStore != nil {
		return s.WorkflowStore, nil
	}
	if s.Store == nil {
		return nil, errors.New("workflow store not configured")
	}
	return s.Store, nil
}
