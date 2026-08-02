package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

var (
	workspacesRoot        = kv.Key{"by-id"}
	workspacesByScopeRoot = kv.Key{"by-scope-name"}
	workflowsRoot         = kv.Key{"by-id"}
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
	Voices        VoiceService
	RuntimeStore  RuntimeStore
	Assets        objectstore.ObjectStore
	IconLocks     iconasset.Locker
	NewID         func() string
}

type ModelService interface {
	GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error)
}

type VoiceService interface {
	GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error)
}

type runtimeWorkflowBindingsContextKey struct{}
type runtimeModelBindingsContextKey struct{}
type runtimeVoiceBindingsContextKey struct{}

// WithRuntimeWorkflowBindings attaches the authenticated connection's current
// RuntimeProfile Workflow alias snapshot to Workspace validation.
func WithRuntimeWorkflowBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	maps.Copy(cloned, bindings)
	return context.WithValue(ctx, runtimeWorkflowBindingsContextKey{}, cloned)
}

// WithRuntimeModelBindings attaches the same RuntimeProfile snapshot's Model
// alias bindings so Workspace overrides can be validated before persistence.
func WithRuntimeModelBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	maps.Copy(cloned, bindings)
	return context.WithValue(ctx, runtimeModelBindingsContextKey{}, cloned)
}

// WithRuntimeVoiceBindings attaches the same RuntimeProfile snapshot's Voice
// alias bindings so Workspace overrides can be validated before persistence.
func WithRuntimeVoiceBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	maps.Copy(cloned, bindings)
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
	GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error)
	GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error)
}

// SystemWorkspaceRetirementService is the relationship-owner handoff for
// deferred cleanup. Keeping it separate avoids forcing unrelated system
// workspace consumers to own social retirement behavior.
type SystemWorkspaceRetirementService interface {
	GetRetiredSystemWorkspace(context.Context, string, apitypes.ChatRoomMode, string) (apitypes.Workspace, error)
	RetireSystemWorkspace(context.Context, string, apitypes.ChatRoomMode, string) (apitypes.Workspace, error)
}

type chatroomRetirementDescriptor struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	WorkspaceKind    apitypes.ChatRoomMode `json:"workspace_kind"`
	SocialResourceID string                `json:"social_resource_id"`
	OwnerPublicKey   *string               `json:"owner_public_key,omitempty"`
	HasIcon          bool                  `json:"has_icon"`
}

// WorkspaceLifecycleService combines the public administration surface with
// the domain-only system Workspace lifecycle surface.
type WorkspaceLifecycleService interface {
	WorkspaceAdminService
	SystemWorkspaceService
	SystemWorkspaceRetirementService
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
		item, err := getWorkspaceByID(ctx, store, string(entry.Value))
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
	if err := s.validateReferences(ctx, workflowStore, normalized, true); err != nil {
		if isInvalidWorkspaceReference(err) {
			return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
		}
		return adminhttp.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if _, err := getWorkspace(ctx, store, string(normalized.Name)); err == nil {
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
	existingID, err := workspaceIDByName(ctx, store, normalized.Name)
	if err == nil {
		retiring, pendingErr := pendingdeletion.HasLocator(
			ctx,
			store,
			pendingdeletion.KindWorkspace,
			existingID,
		)
		if pendingErr != nil {
			return apitypes.Workspace{}, false, pendingErr
		}
		if retiring {
			return apitypes.Workspace{}, false, fmt.Errorf(
				"workspace %q is pending deletion and cannot be reused",
				normalized.Name,
			)
		}
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, false, err
	}
	workflowStore, err := s.workflowStore()
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	if err := s.validateReferences(ctx, workflowStore, normalized, true); err != nil {
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
		Id:           s.newID(),
		LastActiveAt: now,
		Labels:       cloneLabelsOrEmpty(normalized.Labels),
		Name:         normalized.Name,
		Parameters:   cloneParameters(normalized.Parameters),
		System:       new(system),
		Toolkit:      cloneToolkitPolicy(normalized.Toolkit),
		UpdatedAt:    now,
		WorkflowId:   normalized.WorkflowId,
	}
	if owner, ok := ownership.FromContext(ctx); ok {
		workspace.OwnerPublicKey = &owner
	}
	if s.RuntimeStore != nil {
		if _, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Id); err != nil {
			return apitypes.Workspace{}, err
		}
	}
	cleanupRuntime := func(cause error) error {
		if s.RuntimeStore == nil {
			return cause
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if err := s.RuntimeStore.DeleteWorkspaceRuntime(cleanupCtx, workspace.Id); err != nil {
			return errors.Join(cause, fmt.Errorf("delete prepared Workspace runtime: %w", err))
		}
		return cause
	}
	data, err := json.Marshal(workspace)
	if err != nil {
		return apitypes.Workspace{}, cleanupRuntime(err)
	}
	entries := []kv.Entry{{Key: workspaceKey(workspace.Id), Value: data}}
	if workspace.OwnerPublicKey != nil && !system {
		entries = append(entries, kv.Entry{Key: workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name), Value: []byte(workspace.Id)})
	}
	_, created, err := kv.CreateIfAbsent(ctx, store,
		kv.Entry{Key: workspaceScopeNameKey(workspace.OwnerPublicKey, workspace.Name), Value: []byte(workspace.Id)},
		entries,
	)
	if err != nil {
		return apitypes.Workspace{}, cleanupRuntime(err)
	}
	if !created {
		return apitypes.Workspace{}, cleanupRuntime(fmt.Errorf("workspace %q already exists", workspace.Name))
	}
	return workspace, nil
}

func (s *Server) DeleteWorkspace(ctx context.Context, request adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	unlock := s.IconLocks.LockOwner(id)
	defer unlock()
	workspace, err := getWorkspaceByID(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", id))), nil
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

// DeleteSystemWorkspaceByID removes a not-yet-committed system Workspace by
// its canonical identity. Relationship services use this after a Workspace ID
// has been persisted; name-based deletion is limited to creation rollback.
func (s *Server) DeleteSystemWorkspaceByID(ctx context.Context, id string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	id = strings.TrimSpace(id)
	unlock := s.IconLocks.LockOwner(id)
	defer unlock()
	workspace, err := getWorkspaceByID(ctx, store, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if !workspaceIsSystem(workspace) {
		return apitypes.Workspace{}, fmt.Errorf("workspace %q is not a system Workspace", id)
	}
	if err := s.deleteWorkspaceRecord(ctx, store, workspace); err != nil {
		return apitypes.Workspace{}, err
	}
	return workspace, nil
}

// RetireSystemWorkspace persists the asynchronous cleanup handoff for an
// established Chatroom Workspace without deleting its record or artifacts.
func (s *Server) RetireSystemWorkspace(ctx context.Context, name string, mode apitypes.ChatRoomMode, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	name = strings.TrimSpace(name)
	socialResourceID = strings.TrimSpace(socialResourceID)
	if !mode.Valid() || socialResourceID == "" {
		return apitypes.Workspace{}, errors.New("workspace: Chatroom retirement mode and social resource id are required")
	}
	unlock := s.IconLocks.LockOwner(name)
	defer unlock()
	if item, err := s.getRetiredSystemWorkspace(ctx, store, name, mode, socialResourceID); err == nil {
		return item, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, err
	}
	item, err := getWorkspace(ctx, store, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return s.retireSystemWorkspace(ctx, store, item, mode, socialResourceID)
}

// RetireSystemWorkspaceByID persists cleanup for an established Chatroom
// Workspace using its canonical identity.
func (s *Server) RetireSystemWorkspaceByID(ctx context.Context, id string, mode apitypes.ChatRoomMode, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	id = strings.TrimSpace(id)
	socialResourceID = strings.TrimSpace(socialResourceID)
	if !mode.Valid() || socialResourceID == "" {
		return apitypes.Workspace{}, errors.New("workspace: Chatroom retirement mode and social resource id are required")
	}
	unlock := s.IconLocks.LockOwner(id)
	defer unlock()
	if item, err := s.getRetiredSystemWorkspaceByID(ctx, store, id, mode, socialResourceID); err == nil {
		return item, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, err
	}
	item, err := getWorkspaceByID(ctx, store, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return s.retireSystemWorkspace(ctx, store, item, mode, socialResourceID)
}

func (s *Server) retireSystemWorkspace(ctx context.Context, store kv.Store, item apitypes.Workspace, mode apitypes.ChatRoomMode, socialResourceID string) (apitypes.Workspace, error) {
	name := item.Name
	if !workspaceIsSystem(item) {
		return apitypes.Workspace{}, fmt.Errorf("workspace %q is not a system Workspace", name)
	}
	if item.Parameters == nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace %q has no Chatroom parameters", name)
	}
	parameters, err := item.Parameters.AsChatRoomWorkspaceParameters()
	if err != nil || parameters.Mode == nil || *parameters.Mode != mode {
		return apitypes.Workspace{}, fmt.Errorf("workspace %q is not a %s Chatroom Workspace", name, mode)
	}
	reason := pendingdeletion.ReasonFriendRelationshipDelete
	if mode == apitypes.ChatRoomModeGroup {
		reason = pendingdeletion.ReasonFriendGroupDelete
	}
	descriptor := chatroomRetirementDescriptor{
		ID:               item.Id,
		Name:             item.Name,
		WorkspaceKind:    mode,
		SocialResourceID: socialResourceID,
		OwnerPublicKey:   cloneString(item.OwnerPublicKey),
		HasIcon:          item.Icon != nil,
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace,
		item.Id,
		nil,
		reason,
		descriptor,
		time.Now(),
	)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if _, _, err := pendingdeletion.CreateOrGet(ctx, store, record); err != nil {
		return apitypes.Workspace{}, err
	}
	return item, nil
}

// GetRetiredSystemWorkspace returns an existing Chatroom Workspace retirement
// without creating one. Relationship services use this read-only check to
// authorize idempotent delete retries before writing any additional handoff.
func (s *Server) GetRetiredSystemWorkspace(ctx context.Context, name string, mode apitypes.ChatRoomMode, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	name = strings.TrimSpace(name)
	socialResourceID = strings.TrimSpace(socialResourceID)
	if !mode.Valid() || socialResourceID == "" {
		return apitypes.Workspace{}, errors.New("workspace: Chatroom retirement mode and social resource id are required")
	}
	unlock := s.IconLocks.LockOwner(name)
	defer unlock()
	return s.getRetiredSystemWorkspace(ctx, store, name, mode, socialResourceID)
}

// GetRetiredSystemWorkspaceByID returns an existing retirement by canonical
// Workspace ID without resolving an owner-scoped Peer name.
func (s *Server) GetRetiredSystemWorkspaceByID(ctx context.Context, id string, mode apitypes.ChatRoomMode, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	id = strings.TrimSpace(id)
	socialResourceID = strings.TrimSpace(socialResourceID)
	if !mode.Valid() || socialResourceID == "" {
		return apitypes.Workspace{}, errors.New("workspace: Chatroom retirement mode and social resource id are required")
	}
	unlock := s.IconLocks.LockOwner(id)
	defer unlock()
	return s.getRetiredSystemWorkspaceByID(ctx, store, id, mode, socialResourceID)
}

func (s *Server) getRetiredSystemWorkspace(
	ctx context.Context,
	store kv.Store,
	name string,
	mode apitypes.ChatRoomMode,
	socialResourceID string,
) (apitypes.Workspace, error) {
	id, err := workspaceIDByName(ctx, store, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return s.getRetiredSystemWorkspaceByID(ctx, store, id, mode, socialResourceID)
}

func (s *Server) getRetiredSystemWorkspaceByID(
	ctx context.Context,
	store kv.Store,
	id string,
	mode apitypes.ChatRoomMode,
	socialResourceID string,
) (apitypes.Workspace, error) {
	record, err := pendingdeletion.GetByLocator(
		ctx,
		store,
		pendingdeletion.KindWorkspace,
		id,
	)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	var stored chatroomRetirementDescriptor
	if err := json.Unmarshal(record.Descriptor, &stored); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: decode Chatroom retirement descriptor: %w", err)
	}
	descriptor, err := validateChatroomRetirementRecord(record, stored.Name, mode, socialResourceID)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	item, getErr := getWorkspaceByID(ctx, store, id)
	if getErr == nil {
		return item, nil
	}
	if errors.Is(getErr, kv.ErrNotFound) {
		return apitypes.Workspace{
			Id:             descriptor.ID,
			Name:           descriptor.Name,
			OwnerPublicKey: cloneString(descriptor.OwnerPublicKey),
		}, nil
	}
	return apitypes.Workspace{}, getErr
}

func validateChatroomRetirementRecord(
	record pendingdeletion.Record,
	name string,
	mode apitypes.ChatRoomMode,
	socialResourceID string,
) (chatroomRetirementDescriptor, error) {
	expectedReason := pendingdeletion.ReasonFriendRelationshipDelete
	if mode == apitypes.ChatRoomModeGroup {
		expectedReason = pendingdeletion.ReasonFriendGroupDelete
	}
	if record.Reason != expectedReason {
		return chatroomRetirementDescriptor{}, fmt.Errorf(
			"workspace: PendingDeletion for %q has reason %q, want %q",
			name,
			record.Reason,
			expectedReason,
		)
	}
	var descriptor chatroomRetirementDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil {
		return chatroomRetirementDescriptor{}, fmt.Errorf(
			"workspace: decode Chatroom retirement descriptor for %q: %w",
			name,
			err,
		)
	}
	if strings.TrimSpace(descriptor.ID) != record.ResourceID ||
		strings.TrimSpace(descriptor.Name) != name ||
		descriptor.WorkspaceKind != mode ||
		strings.TrimSpace(descriptor.SocialResourceID) != socialResourceID {
		return chatroomRetirementDescriptor{}, fmt.Errorf(
			"workspace: PendingDeletion for %q does not match %s Chatroom resource %q",
			name,
			mode,
			socialResourceID,
		)
	}
	return descriptor, nil
}

func (s *Server) deleteWorkspaceRecord(ctx context.Context, store kv.Store, workspace apitypes.Workspace) error {
	if workspace.Icon != nil && s.Assets == nil {
		return errors.New("workspace asset store not configured")
	}
	if s.Assets != nil {
		for _, format := range []iconasset.Format{iconasset.FormatPixa, iconasset.FormatPNG} {
			if err := s.Assets.Delete(iconasset.ObjectName(string(workspace.Id), format)); err != nil {
				return errors.New("failed to delete workspace icon")
			}
		}
	}
	if s.RuntimeStore != nil {
		if err := s.RuntimeStore.DeleteWorkspaceRuntime(ctx, workspace.Id); err != nil {
			return err
		}
	}
	keys := []kv.Key{workspaceKey(string(workspace.Id)), workspaceScopeNameKey(workspace.OwnerPublicKey, workspace.Name)}
	if workspace.OwnerPublicKey != nil && !workspaceIsSystem(workspace) {
		keys = append(keys, workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name))
	}
	return store.BatchDelete(ctx, keys)
}

func (s *Server) fastDeleteWorkspaceRecord(ctx context.Context, store kv.Store, workspace apitypes.Workspace) error {
	descriptor := struct {
		ID             string  `json:"id"`
		Name           string  `json:"name"`
		OwnerPublicKey *string `json:"owner_public_key,omitempty"`
		HasIcon        bool    `json:"has_icon"`
	}{
		ID:             workspace.Id,
		Name:           workspace.Name,
		OwnerPublicKey: cloneString(workspace.OwnerPublicKey),
		HasIcon:        workspace.Icon != nil,
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace,
		workspace.Id,
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
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	workspace, err := getWorkspaceByID(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", id))), nil
		}
		return adminhttp.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetWorkspace200JSONResponse(workspace), nil
}

// GetWorkspaceByName resolves the authenticated owner's Peer-visible name to
// the canonical Workspace record. Admin callers use GetWorkspace with an ID.
func (s *Server) GetWorkspaceByName(ctx context.Context, name string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return getWorkspace(ctx, store, strings.TrimSpace(name))
}

// GetWorkspaceRuntimeByID returns runtime state by canonical Workspace ID.
func (s *Server) GetWorkspaceRuntimeByID(ctx context.Context, id string) (Runtime, error) {
	if s == nil || s.RuntimeStore == nil {
		return Runtime{}, nil
	}
	return s.RuntimeStore.GetWorkspaceRuntime(ctx, strings.TrimSpace(id))
}

func (s *Server) PutWorkspace(ctx context.Context, request adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	normalized, err := normalizeWorkspaceUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	store, err := s.store()
	if err != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	unlock := s.IconLocks.LockRecord(id)
	defer unlock()
	previous, previousErr := getWorkspaceByID(ctx, store, id)
	if errors.Is(previousErr, kv.ErrNotFound) {
		return adminhttp.PutWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", id))), nil
	}
	if previousErr != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", previousErr.Error())), nil
	}
	if normalized.Name != previous.Name {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", fmt.Sprintf("name %q must match immutable name %q", normalized.Name, previous.Name))), nil
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
	if err := s.validateReferences(ctx, workflowStore, normalized, true); err != nil {
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
		Id:           previous.Id,
		LastActiveAt: now,
		Labels:       cloneLabelsOrEmpty(normalized.Labels),
		Name:         normalized.Name,
		Parameters:   cloneParameters(normalized.Parameters),
		System:       new(false),
		Toolkit:      cloneToolkitPolicy(normalized.Toolkit),
		UpdatedAt:    now,
		WorkflowId:   normalized.WorkflowId,
		Icon:         previous.Icon,
	}
	workspace.CreatedAt = previous.CreatedAt
	workspace.LastActiveAt = previous.LastActiveAt
	workspace.System = previous.System
	workspace.OwnerPublicKey = cloneString(previous.OwnerPublicKey)
	if normalized.Labels == nil {
		workspace.Labels = cloneLabelsOrEmpty(previous.Labels)
	}
	if s.RuntimeStore != nil {
		if _, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Id); err != nil {
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
	entries := []kv.Entry{
		{Key: workspaceKey(string(workspace.Id)), Value: data},
		{Key: workspaceScopeNameKey(workspace.OwnerPublicKey, workspace.Name), Value: []byte(workspace.Id)},
	}
	if workspace.OwnerPublicKey != nil && !workspaceIsSystem(workspace) {
		entries = append(entries, kv.Entry{Key: workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name), Value: []byte(workspace.Id)})
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("workspace: write %s: %w", workspace.Name, err)
	}
	return nil
}

func systemWorkspaceMatches(existing apitypes.Workspace, desired adminhttp.WorkspaceUpsert, owner string) bool {
	return existing.OwnerPublicKey != nil &&
		strings.TrimSpace(*existing.OwnerPublicKey) == owner &&
		existing.WorkflowId == desired.WorkflowId &&
		reflect.DeepEqual(existing.Labels, cloneLabelsOrEmpty(desired.Labels)) &&
		systemWorkspaceDomainParametersMatch(existing.Parameters, desired.Parameters) &&
		reflect.DeepEqual(existing.Toolkit, cloneToolkitPolicy(desired.Toolkit))
}

func systemWorkspaceAllowsInputUpdate(existing apitypes.Workspace, desired adminhttp.WorkspaceUpsert) bool {
	desiredLabels := cloneLabelsOrEmpty(desired.Labels)
	if desired.Labels == nil {
		desiredLabels = cloneLabelsOrEmpty(existing.Labels)
	}
	return existing.WorkflowId == desired.WorkflowId &&
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
	id, err := workspaceIDByName(ctx, store, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return getWorkspaceByID(ctx, store, id)
}

func workspaceIDByName(ctx context.Context, store kv.Store, name string) (string, error) {
	var owner *string
	if value, ok := ownership.FromContext(ctx); ok && strings.TrimSpace(value) != "" {
		value = strings.TrimSpace(value)
		owner = &value
	}
	id, err := store.Get(ctx, workspaceScopeNameKey(owner, name))
	if err != nil {
		return "", err
	}
	return string(id), nil
}

func getWorkspaceByID(ctx context.Context, store kv.Store, id string) (apitypes.Workspace, error) {
	data, err := store.Get(ctx, workspaceKey(id))
	if err != nil {
		return apitypes.Workspace{}, err
	}
	var workspace apitypes.Workspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: decode %s: %w", id, err)
	}
	return validateStoredWorkspace(workspace)
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
		workspace, err = validateStoredWorkspace(workspace)
		if err != nil {
			return nil, false, nil, fmt.Errorf("workspace: validate list %s: %w", entry.Key.String(), err)
		}
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

func validateStoredWorkspace(workspace apitypes.Workspace) (apitypes.Workspace, error) {
	if strings.TrimSpace(workspace.Id) == "" || strings.TrimSpace(string(workspace.Name)) == "" || strings.TrimSpace(string(workspace.WorkflowId)) == "" {
		return apitypes.Workspace{}, errors.New("stored Workspace requires id, name, and workflow_id")
	}
	if workspace.System == nil {
		return apitypes.Workspace{}, errors.New("stored Workspace requires system")
	}
	if workspace.CreatedAt.IsZero() || workspace.LastActiveAt.IsZero() || workspace.UpdatedAt.IsZero() {
		return apitypes.Workspace{}, errors.New("stored Workspace requires created_at, last_active_at, and updated_at")
	}
	workspace.Labels = cloneLabelsOrEmpty(workspace.Labels)
	return workspace, nil
}

func workspaceIsSystem(workspace apitypes.Workspace) bool {
	return workspace.System != nil && *workspace.System
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
	workflowID := strings.TrimSpace(string(in.WorkflowId))
	if workflowID == "" {
		return adminhttp.WorkspaceUpsert{}, errors.New("workflow_id is required")
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
		Labels:     labels,
		Name:       string(name),
		Parameters: cloneParameters(in.Parameters),
		Toolkit:    policy,
		WorkflowId: workflowID,
	}, nil
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

func (s *Server) validateReferences(ctx context.Context, store kv.Store, workspace adminhttp.WorkspaceUpsert, directWorkflow bool) error {
	workflowName, runtimeAlias, err := resolveWorkflowReference(ctx, workspace, directWorkflow)
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
	if workflow.Spec.Driver == apitypes.WorkflowDriverDashscopeRealtime {
		return s.validateDashScopeRealtimeOverrides(ctx, workflow.Spec.DashscopeRealtime, workspace.Parameters, runtimeAlias)
	}
	if workflow.Spec.Driver == apitypes.WorkflowDriverDoubaoRealtimeDuplex {
		return s.validateDoubaoRealtimeDuplexOverrides(ctx, workflow.Spec.DoubaoRealtimeDuplex, workspace.Parameters, runtimeAlias)
	}
	if workflow.Spec.Driver == apitypes.WorkflowDriverEino {
		return validateEinoOverrides(workspace.Parameters)
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
	if err := requireWorkspaceParametersVariant(workspaceParameters, "doubao-realtime"); err != nil {
		return invalidWorkspaceReference("doubao_realtime parameters are required: %v", err)
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

func (s *Server) validateDashScopeRealtimeOverrides(
	ctx context.Context,
	workflow *apitypes.DashScopeRealtimeWorkflowSpec,
	workspaceParameters *apitypes.WorkspaceParameters,
	runtimeAlias bool,
) error {
	var parameters apitypes.DashScopeRealtimeWorkspaceParameters
	if workspaceParameters != nil {
		if err := requireWorkspaceParametersVariant(workspaceParameters, "dashscope-realtime"); err != nil {
			return invalidWorkspaceReference("dashscope_realtime parameters are required: %v", err)
		}
		var err error
		parameters, err = workspaceParameters.AsDashScopeRealtimeWorkspaceParameters()
		if err != nil {
			return invalidWorkspaceReference("dashscope_realtime parameters are required: %v", err)
		}
		if err := parameters.Validate(); err != nil {
			return invalidWorkspaceReference("dashscope_realtime parameters: %v", err)
		}
	}
	if !runtimeAlias {
		return nil
	}
	if workflow == nil {
		return invalidWorkspaceReference("dashscope_realtime workflow spec is required")
	}
	modelAlias := strings.TrimSpace(workflow.Model)
	if parameters.Model != nil && strings.TrimSpace(*parameters.Model) != "" {
		modelAlias = strings.TrimSpace(*parameters.Model)
	}
	model, err := s.validateRuntimeModelAlias(
		ctx, "dashscope_realtime parameter", "model", modelAlias, apitypes.ModelKindRealtime,
	)
	if err != nil {
		return err
	}
	if model.Provider.Kind != apitypes.ModelProviderKindDashscopeTenant {
		return invalidWorkspaceReference(
			"dashscope_realtime parameter %q Model %q has provider %q, want %q",
			"model", modelAlias, model.Provider.Kind, apitypes.ModelProviderKindDashscopeTenant,
		)
	}
	data, err := model.ProviderData.AsDashScopeTenantModelProviderData()
	if err != nil || data.ApiMode == nil ||
		*data.ApiMode != apitypes.DashScopeTenantModelProviderDataApiModeRealtime {
		return invalidWorkspaceReference(
			"dashscope_realtime parameter %q Model %q must use dashscope-tenant api_mode %q",
			"model", modelAlias, apitypes.DashScopeTenantModelProviderDataApiModeRealtime,
		)
	}
	voiceAlias := stringPointerValue(workflow.Voice)
	if parameters.Voice != nil {
		voiceAlias = strings.TrimSpace(*parameters.Voice)
	}
	return s.validateRuntimeVoiceCompatibility(
		ctx, "dashscope_realtime parameter", "voice", voiceAlias, modelAlias, model,
	)
}

func (s *Server) validateDoubaoRealtimeDuplexOverrides(
	ctx context.Context,
	workflow *apitypes.DoubaoRealtimeDuplexWorkflowSpec,
	workspaceParameters *apitypes.WorkspaceParameters,
	runtimeAlias bool,
) error {
	var parameters apitypes.DoubaoRealtimeDuplexWorkspaceParameters
	if workspaceParameters != nil {
		if err := requireWorkspaceParametersVariant(workspaceParameters, "doubao-realtime-duplex"); err != nil {
			return invalidWorkspaceReference("doubao_realtime_duplex parameters are required: %v", err)
		}
		var err error
		parameters, err = workspaceParameters.AsDoubaoRealtimeDuplexWorkspaceParameters()
		if err != nil {
			return invalidWorkspaceReference("doubao_realtime_duplex parameters are required: %v", err)
		}
		if err := parameters.Validate(); err != nil {
			return invalidWorkspaceReference("doubao_realtime_duplex parameters: %v", err)
		}
	}
	if !runtimeAlias {
		return nil
	}
	if workflow == nil {
		return invalidWorkspaceReference("doubao_realtime_duplex workflow spec is required")
	}
	modelAlias := strings.TrimSpace(workflow.Model)
	if parameters.Model != nil && strings.TrimSpace(*parameters.Model) != "" {
		modelAlias = strings.TrimSpace(*parameters.Model)
	}
	model, err := s.validateRuntimeModelAlias(
		ctx, "doubao_realtime_duplex parameter", "model", modelAlias, apitypes.ModelKindRealtimeDuplex,
	)
	if err != nil {
		return err
	}
	if model.Provider.Kind != apitypes.ModelProviderKindVolcTenant {
		return invalidWorkspaceReference(
			"doubao_realtime_duplex parameter %q Model %q has provider %q, want %q",
			"model", modelAlias, model.Provider.Kind, apitypes.ModelProviderKindVolcTenant,
		)
	}
	data, err := model.ProviderData.AsVolcTenantModelProviderData()
	if err != nil || data.ApiMode != apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex {
		return invalidWorkspaceReference(
			"doubao_realtime_duplex parameter %q Model %q must use volc-tenant api_mode %q",
			"model", modelAlias, apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex,
		)
	}
	voiceAlias := stringPointerValue(workflow.Voice)
	if parameters.Voice != nil {
		voiceAlias = strings.TrimSpace(*parameters.Voice)
	}
	return s.validateRuntimeVoiceCompatibility(
		ctx, "doubao_realtime_duplex parameter", "voice", voiceAlias, modelAlias, model,
	)
}

func validateEinoOverrides(workspaceParameters *apitypes.WorkspaceParameters) error {
	if workspaceParameters == nil {
		return nil
	}
	if err := requireWorkspaceParametersVariant(workspaceParameters, "eino"); err != nil {
		return invalidWorkspaceReference("eino parameters are required: %v", err)
	}
	if _, err := workspaceParameters.AsEinoWorkspaceParameters(); err != nil {
		return invalidWorkspaceReference("eino parameters are required: %v", err)
	}
	return nil
}

func requireWorkspaceParametersVariant(parameters *apitypes.WorkspaceParameters, want string) error {
	discriminator, err := parameters.Discriminator()
	if err != nil {
		return err
	}
	if discriminator != want {
		return fmt.Errorf("agent_type is %q, want %q", discriminator, want)
	}
	return nil
}

func (s *Server) validateRuntimeModelAlias(
	ctx context.Context,
	subject, role, alias string,
	want apitypes.ModelKind,
) (apitypes.Model, error) {
	bindings, present := ctx.Value(runtimeModelBindingsContextKey{}).(map[string]string)
	if !present {
		return apitypes.Model{}, errors.New("runtime model bindings not configured")
	}
	modelID := strings.TrimSpace(bindings[alias])
	if modelID == "" {
		return apitypes.Model{}, invalidWorkspaceReference(
			"%s %q references missing runtime Model alias %q", subject, role, alias,
		)
	}
	if s == nil || s.Models == nil {
		return apitypes.Model{}, errors.New("model service not configured")
	}
	response, err := s.Models.GetModel(ctx, adminhttp.GetModelRequestObject{Id: modelID})
	if err != nil {
		return apitypes.Model{}, err
	}
	model, ok := response.(adminhttp.GetModel200JSONResponse)
	if _, missing := response.(adminhttp.GetModel404JSONResponse); missing {
		return apitypes.Model{}, invalidWorkspaceReference(
			"%s %q references missing Model %q", subject, role, alias,
		)
	}
	if !ok {
		return apitypes.Model{}, fmt.Errorf(
			"validate %s %q Model %q: model service returned %T", subject, role, alias, response,
		)
	}
	if model.Kind != want {
		return apitypes.Model{}, invalidWorkspaceReference(
			"%s %q Model %q has kind %q, want %q", subject, role, alias, model.Kind, want,
		)
	}
	return apitypes.Model(model), nil
}

func (s *Server) validateRuntimeVoiceCompatibility(
	ctx context.Context,
	subject, role, alias, modelAlias string,
	model apitypes.Model,
) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil
	}
	bindings, present := ctx.Value(runtimeVoiceBindingsContextKey{}).(map[string]string)
	if !present {
		return errors.New("runtime voice bindings not configured")
	}
	voiceID := strings.TrimSpace(bindings[alias])
	if voiceID == "" {
		return invalidWorkspaceReference(
			"%s %q references missing runtime Voice alias %q", subject, role, alias,
		)
	}
	if s == nil || s.Voices == nil {
		return errors.New("voice service not configured")
	}
	response, err := s.Voices.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: voiceID})
	if err != nil {
		return err
	}
	voice, ok := response.(adminhttp.GetVoice200JSONResponse)
	if _, missing := response.(adminhttp.GetVoice404JSONResponse); missing {
		return invalidWorkspaceReference(
			"%s %q references missing Voice %q", subject, role, alias,
		)
	}
	if !ok {
		return fmt.Errorf(
			"validate %s %q Voice %q: voice service returned %T", subject, role, alias, response,
		)
	}
	if voice.Provider.Kind != apitypes.VoiceProviderKind(model.Provider.Kind) ||
		voice.Provider.Id != model.Provider.Id {
		return invalidWorkspaceReference(
			"%s %q Voice %q uses provider %q/%q, want %q/%q to match Model %q",
			subject, role, alias,
			voice.Provider.Kind, voice.Provider.Id,
			model.Provider.Kind, model.Provider.Id,
			modelAlias,
		)
	}
	return nil
}

func (s *Server) validateASTTranslateOverrides(ctx context.Context, workspaceParameters *apitypes.WorkspaceParameters) error {
	if workspaceParameters == nil {
		return nil
	}
	if err := requireWorkspaceParametersVariant(workspaceParameters, "ast-translate"); err != nil {
		return invalidWorkspaceReference("ast-translate parameters are required: %v", err)
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

func resolveWorkflowReference(ctx context.Context, workspace adminhttp.WorkspaceUpsert, _ bool) (string, bool, error) {
	name := strings.TrimSpace(string(workspace.WorkflowId))
	_, runtimeBound := ctx.Value(runtimeWorkflowBindingsContextKey{}).(map[string]string)
	return name, runtimeBound, nil
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
		if err := requireWorkspaceParametersVariant(workspaceParameters, "flowcraft"); err != nil {
			return nil, invalidWorkspaceReference("flowcraft parameters are required: %v", err)
		}
		_, err := workspaceParameters.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, invalidWorkspaceReference("flowcraft parameters are required: %v", err)
		}
	}
	if workflow.Spec.Flowcraft == nil {
		return nil, invalidWorkspaceReference("flowcraft workflow config is required")
	}
	configured := *workflow.Spec.Flowcraft
	references := make([]FlowcraftModelReference, 0, len(configured.Graph.Nodes)+3)
	for index, raw := range configured.Graph.Nodes {
		if discriminator, _ := raw.Discriminator(); discriminator == "llm" {
			node, err := raw.AsFlowcraftLLMNode()
			if err != nil {
				return nil, invalidWorkspaceReference("flowcraft graph node %d is invalid: %v", index, err)
			}
			references = append(references, FlowcraftModelReference{
				Role: fmt.Sprintf("graph.nodes[%d].config.model", index), ModelID: node.Config.Model, Kind: apitypes.ModelKindLlm,
			})
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

func workspaceKey(id string) kv.Key {
	return append(append(kv.Key{}, workspacesRoot...), escapeStoreSegment(id))
}

func workspaceScopeNameKey(owner *string, name string) kv.Key {
	scope := "@admin"
	if owner != nil && strings.TrimSpace(*owner) != "" {
		scope = strings.TrimSpace(*owner)
	}
	return append(append(append(kv.Key{}, workspacesByScopeRoot...), escapeStoreSegment(scope)), escapeStoreSegment(name))
}

func workspaceByOwnerKey(owner, name string) kv.Key {
	return append(workspaceByOwnerPrefix(owner), escapeStoreSegment(name))
}

func workspaceByOwnerPrefix(owner string) kv.Key {
	return append(append(kv.Key{}, workspacesByOwnerRoot...), escapeStoreSegment(owner))
}

func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return socialutil.NewID()
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
		maps.Copy(cloned, *labels)
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
