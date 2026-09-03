package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

var (
	workspacesRoot         = kv.Key{"by-id"}
	workspacesByScopeRoot  = kv.Key{"by-scope-name"}
	workflowsRoot          = kv.Key{"by-id"}
	workspacesByOwnerRoot  = kv.Key{"by-owner"}
	errWorkspaceIDExists   = errors.New("workspace id already exists")
	errWorkspaceNameExists = errors.New("workspace name already exists")
)

const (
	defaultListLimit                   = 50
	maxListLimit                       = 200
	maxWorkspaceLabels                 = 32
	maxWorkspaceLabelKeyBytes          = 63
	maxWorkspaceLabelValueBytes        = 128
	SystemWorkspaceDeleteForbiddenCode = "SYSTEM_WORKSPACE_DELETE_FORBIDDEN"
	SystemWorkspaceUpdateForbiddenCode = "SYSTEM_WORKSPACE_UPDATE_FORBIDDEN"
	WorkspacePendingDeletionCode       = "WORKSPACE_PENDING_DELETION"
	PeerPendingDeletionCode            = "PEER_PENDING_DELETION"
	PeerDeletedCode                    = "PEER_DELETED"
)

var (
	ErrWorkspacePendingDeletion = errors.New("workspace: pending deletion")
	ErrWorkspaceDeleted         = errors.New("workspace: deleted")
	ErrPeerPendingDeletion      = errors.New("workspace: owner Peer pending deletion")
	ErrPeerDeleted              = errors.New("workspace: owner Peer deleted")
)

type Server struct {
	Store            kv.Store
	Workflows        WorkflowService
	Models           ModelService
	Voices           VoiceService
	RuntimeStore     RuntimeStore
	Assets           objectstore.ObjectStore
	IconLocks        iconasset.Locker
	NewID            func() string
	PeerAvailability func(context.Context, string) error
	DeletionFencer   WorkspaceDeletionFencer

	ownerCreateLocks keyedlock.Locker[string]
}

// WorkspaceDeletionFencer serializes authoritative PendingDeletion marker
// creation with any not-yet-committed reward settlement for the same
// Workspace. The callback must be invoked while the durable fence is held.
type WorkspaceDeletionFencer interface {
	WithWorkspaceDeletionFence(context.Context, string, func(context.Context) error) error
}

// WorkflowService resolves Workflow resources without exposing the owning
// service's backing Store.
type WorkflowService interface {
	GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error)
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
	DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error)
	GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error)
	PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error)
}

// PeerWorkspaceCreateRequest is the transport-independent input for creating
// one ordinary Workspace owned by the authenticated Peer in ctx.
type PeerWorkspaceCreateRequest struct {
	Name       string
	WorkflowID string
	Labels     map[string]string
	Parameters *apitypes.WorkspaceParameters
	Toolkit    *apitypes.ToolkitPolicy

	// Initialize may persist runtime-scoped state after runtime preparation and
	// before the Workspace record becomes visible. A failure rolls the runtime
	// back and leaves no discoverable Workspace.
	Initialize func(context.Context, Runtime) error
}

// PeerWorkspaceCreateErrorKind classifies errors for transport adapters.
type PeerWorkspaceCreateErrorKind string

const (
	PeerWorkspaceCreateInvalid  PeerWorkspaceCreateErrorKind = "invalid"
	PeerWorkspaceCreateNotFound PeerWorkspaceCreateErrorKind = "not_found"
	PeerWorkspaceCreateConflict PeerWorkspaceCreateErrorKind = "conflict"
	PeerWorkspaceCreateInternal PeerWorkspaceCreateErrorKind = "internal"
)

// PeerWorkspaceCreateError is a typed domain failure mapped by Peer RPC and
// other authenticated Peer-owned transports.
type PeerWorkspaceCreateError struct {
	Kind PeerWorkspaceCreateErrorKind
	Err  error
}

func (e *PeerWorkspaceCreateError) Error() string {
	if e == nil || e.Err == nil {
		return "workspace: create failed"
	}
	return e.Err.Error()
}

func (e *PeerWorkspaceCreateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PeerWorkspaceService is the authenticated Peer-owned create surface. The
// Server generates the canonical ID and never accepts an Admin HTTP DTO.
type PeerWorkspaceService interface {
	CreatePeerWorkspace(context.Context, PeerWorkspaceCreateRequest) (apitypes.Workspace, error)
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
	GetRetiredSystemWorkspace(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error)
	RetireSystemWorkspace(context.Context, string, socialutil.SFUWorkspaceKind, string) (apitypes.Workspace, error)
}

// socialRetirementDescriptor is the durable handoff written when a Friend or
// Friend Group retires its SFU Workspace. The caller declares the Social kind;
// the Workspace record carries no domain parameters.
type socialRetirementDescriptor struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	WorkspaceKind    socialutil.SFUWorkspaceKind `json:"workspace_kind"`
	SocialResourceID string                      `json:"social_resource_id"`
	OwnerPublicKey   *string                     `json:"owner_public_key,omitempty"`
	HasIcon          bool                        `json:"has_icon"`
}

func socialRetirementReason(kind socialutil.SFUWorkspaceKind) (pendingdeletion.Reason, bool) {
	switch kind {
	case socialutil.SFUWorkspaceKindFriend:
		return pendingdeletion.ReasonFriendRelationshipDelete, true
	case socialutil.SFUWorkspaceKindFriendGroup:
		return pendingdeletion.ReasonFriendGroupDelete, true
	default:
		return "", false
	}
}

type workspaceDeletionDescriptor struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	OwnerPublicKey *string `json:"owner_public_key,omitempty"`
	HasIcon        bool    `json:"has_icon"`
	System         bool    `json:"system,omitempty"`
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
		if err := s.ensureWorkspaceAvailable(ctx, item.Id); err != nil {
			return nil, err
		}
		if !workspaceMatchesLabels(item, selector) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) CreatePeerWorkspace(ctx context.Context, request PeerWorkspaceCreateRequest) (apitypes.Workspace, error) {
	return s.createPeerWorkspace(ctx, request, s.newID())
}

func (s *Server) createPeerWorkspace(ctx context.Context, request PeerWorkspaceCreateRequest, canonicalID string) (apitypes.Workspace, error) {
	owner, ok := ownership.FromContext(ctx)
	if !ok || strings.TrimSpace(owner) == "" {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInvalid, errors.New("workspace: Peer owner is required"))
	}
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInternal, err)
	}
	labels := maps.Clone(request.Labels)
	body := adminhttp.WorkspaceUpsert{
		Id: canonicalID, Name: request.Name, WorkflowId: request.WorkflowID,
		Labels: &labels, Parameters: request.Parameters, Toolkit: request.Toolkit,
	}
	normalized, err := normalizeWorkspaceUpsert(body, "")
	if err != nil {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInvalid, err)
	}
	if err := s.ensureContextPeerAvailable(ctx); err != nil {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateConflict, err)
	}
	unlock := s.IconLocks.LockRecord(string(normalized.Name))
	defer unlock()
	if err := s.ensureWorkspaceAvailable(ctx, normalized.Id); err != nil {
		if errors.Is(err, ErrWorkspacePendingDeletion) {
			return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateConflict, err)
		}
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInternal, err)
	}
	if err := s.validateReferences(ctx, normalized, true); err != nil {
		if isInvalidWorkspaceReference(err) {
			return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInvalid, err)
		}
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInternal, err)
	}
	if _, err := getWorkspace(ctx, store, string(normalized.Name)); err == nil {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateConflict, fmt.Errorf("workspace %q already exists", normalized.Name))
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInternal, err)
	}
	if _, err := getWorkspaceByID(ctx, store, normalized.Id); err == nil {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateConflict, fmt.Errorf("workspace id %q already exists", normalized.Id))
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInternal, err)
	}
	workspace, err := s.createWorkspaceRecord(ctx, store, normalized, false, request.Initialize)
	if err != nil {
		if errors.Is(err, errWorkspaceIDExists) || errors.Is(err, errWorkspaceNameExists) {
			return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateConflict, err)
		}
		return apitypes.Workspace{}, peerWorkspaceCreateError(PeerWorkspaceCreateInternal, err)
	}
	return workspace, nil
}

func peerWorkspaceCreateError(kind PeerWorkspaceCreateErrorKind, err error) error {
	return &PeerWorkspaceCreateError{Kind: kind, Err: err}
}

func (s *Server) CreateSystemWorkspace(ctx context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	owner, ok := ownership.FromContext(ctx)
	if !ok || strings.TrimSpace(owner) == "" {
		return apitypes.Workspace{}, false, errors.New("workspace: system Workspace owner is required")
	}
	owner = strings.TrimSpace(owner)
	ctx = ownership.WithOwner(ctx, owner)
	if s.PeerAvailability != nil {
		if err := s.PeerAvailability(ctx, owner); err != nil {
			return apitypes.Workspace{}, false, err
		}
	}
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	if strings.TrimSpace(body.Id) == "" {
		body.Id = s.newID()
	}
	normalized, err := normalizeWorkspaceUpsert(body, "")
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	unlock := s.IconLocks.LockRecord(string(normalized.Name))
	defer unlock()
	if err := s.ensureWorkspaceAvailable(ctx, normalized.Id); err != nil {
		return apitypes.Workspace{}, false, err
	}
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
	if err := s.validateReferences(ctx, normalized, true); err != nil {
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
	workspace, err := s.createWorkspaceRecord(ctx, store, normalized, true, nil)
	return workspace, err == nil, err
}

func (s *Server) createWorkspaceRecord(
	ctx context.Context,
	store kv.Store,
	normalized adminhttp.WorkspaceUpsert,
	system bool,
	initialize func(context.Context, Runtime) error,
) (apitypes.Workspace, error) {
	owner := optionalWorkspaceOwner(ctx)
	ownerKey := ""
	if owner != nil {
		ownerKey = *owner
	}
	release, err := s.ownerCreateLocks.Acquire(ctx, ownerKey)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	defer release()
	if err := s.ensureContextPeerAvailable(ctx); err != nil {
		return apitypes.Workspace{}, err
	}
	if _, err := getWorkspaceByID(ctx, store, normalized.Id); err == nil {
		return apitypes.Workspace{}, fmt.Errorf("%w: %q", errWorkspaceIDExists, normalized.Id)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, err
	}
	nameKey := workspaceScopeNameKey(owner, normalized.Name)
	if _, err := store.Get(ctx, nameKey); err == nil {
		return apitypes.Workspace{}, fmt.Errorf("%w: %q", errWorkspaceNameExists, normalized.Name)
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, err
	}
	now := time.Now().UTC()
	workspace := apitypes.Workspace{
		CreatedAt:    now,
		Id:           normalized.Id,
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
	var runtime Runtime
	if s.RuntimeStore != nil {
		prepared, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Id)
		if err != nil {
			return apitypes.Workspace{}, err
		}
		runtime = prepared
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
	if initialize != nil {
		if s.RuntimeStore == nil {
			return apitypes.Workspace{}, errors.New("workspace: runtime store is required for initialization")
		}
		if err := initialize(ctx, runtime); err != nil {
			return apitypes.Workspace{}, cleanupRuntime(fmt.Errorf("initialize Workspace runtime: %w", err))
		}
	}
	data, err := json.Marshal(workspace)
	if err != nil {
		return apitypes.Workspace{}, cleanupRuntime(err)
	}
	guards := []kv.Entry{
		{Key: workspaceKey(workspace.Id), Value: data},
		{Key: nameKey, Value: []byte(workspace.Id)},
	}
	var entries []kv.Entry
	if workspace.OwnerPublicKey != nil && !system {
		entries = append(entries, kv.Entry{Key: workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name), Value: []byte(workspace.Id)})
	}
	conflict, _, created, err := kv.CreateIfAllAbsent(ctx, store, guards, entries)
	if err != nil {
		return apitypes.Workspace{}, cleanupRuntime(err)
	}
	if !created {
		if reflect.DeepEqual(conflict, workspaceKey(workspace.Id)) {
			return apitypes.Workspace{}, cleanupRuntime(fmt.Errorf("%w: %q", errWorkspaceIDExists, workspace.Id))
		}
		return apitypes.Workspace{}, cleanupRuntime(fmt.Errorf("%w: %q", errWorkspaceNameExists, workspace.Name))
	}
	return workspace, nil
}

func optionalWorkspaceOwner(ctx context.Context) *string {
	owner, ok := ownership.FromContext(ctx)
	if !ok {
		return nil
	}
	return &owner
}

func (s *Server) DeleteWorkspace(ctx context.Context, request adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
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
	if err := s.ensureWorkspaceOwnerAvailable(ctx, workspace); err != nil {
		return adminhttp.DeleteWorkspace409JSONResponse(apitypes.NewErrorResponse(peerAvailabilityCode(err), err.Error())), nil
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
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: invalid id: %w", err)
	}
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
// established Social SFU Workspace without deleting its record or artifacts.
// The caller declares which Social resource kind owns the Workspace.
func (s *Server) RetireSystemWorkspace(ctx context.Context, name string, kind socialutil.SFUWorkspaceKind, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	name = strings.TrimSpace(name)
	if err := validateSocialRetirementRequest(kind, socialResourceID); err != nil {
		return apitypes.Workspace{}, err
	}
	unlock := s.IconLocks.LockOwner(name)
	defer unlock()
	if item, err := s.getRetiredSystemWorkspace(ctx, store, name, kind, socialResourceID); err == nil {
		return item, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, err
	}
	item, err := getWorkspace(ctx, store, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return s.retireSystemWorkspace(ctx, store, item, kind, socialResourceID)
}

// RetireSystemWorkspaceByID persists cleanup for an established Social SFU
// Workspace using its canonical identity.
func (s *Server) RetireSystemWorkspaceByID(ctx context.Context, id string, kind socialutil.SFUWorkspaceKind, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: invalid id: %w", err)
	}
	if err := validateSocialRetirementRequest(kind, socialResourceID); err != nil {
		return apitypes.Workspace{}, err
	}
	unlock := s.IconLocks.LockOwner(id)
	defer unlock()
	if item, err := s.getRetiredSystemWorkspaceByID(ctx, store, id, kind, socialResourceID); err == nil {
		return item, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Workspace{}, err
	}
	item, err := getWorkspaceByID(ctx, store, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return s.retireSystemWorkspace(ctx, store, item, kind, socialResourceID)
}

func validateSocialRetirementRequest(kind socialutil.SFUWorkspaceKind, socialResourceID string) error {
	if err := customid.ValidateResourceID(socialResourceID); err != nil {
		return fmt.Errorf("workspace: invalid social resource id: %w", err)
	}
	if _, ok := socialRetirementReason(kind); !ok {
		return fmt.Errorf("workspace: unsupported Social Workspace kind %q", kind)
	}
	return nil
}

func (s *Server) retireSystemWorkspace(ctx context.Context, store kv.Store, item apitypes.Workspace, kind socialutil.SFUWorkspaceKind, socialResourceID string) (apitypes.Workspace, error) {
	name := item.Name
	if !workspaceIsSystem(item) {
		return apitypes.Workspace{}, fmt.Errorf("workspace %q is not a system Workspace", name)
	}
	reason, ok := socialRetirementReason(kind)
	if !ok {
		return apitypes.Workspace{}, fmt.Errorf("workspace: unsupported Social Workspace kind %q", kind)
	}
	descriptor := socialRetirementDescriptor{
		ID:               item.Id,
		Name:             item.Name,
		WorkspaceKind:    kind,
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
	// Social SFU Workspaces are never reward eligible, so there is no reward
	// settlement to fence against. Their marker is written directly: the
	// reward fence opens a transaction on the gameplay SQL storage, and a
	// Workspace KV store sharing that single-connection SQLite handle would
	// deadlock behind it. Any other system Workspace reaching this path -- a
	// stale or malformed Social retirement record naming it -- keeps the
	// fence, because being a system Workspace alone does not prove that no
	// reward settlement is in flight.
	if !workspaceIsSocialSFU(item) {
		if err := s.createPendingDeletion(ctx, store, record); err != nil {
			return apitypes.Workspace{}, err
		}
		return item, nil
	}
	if _, _, err := pendingdeletion.CreateOrGet(ctx, store, record); err != nil {
		return apitypes.Workspace{}, err
	}
	return item, nil
}

// workspaceIsSocialSFU reports the exact shape a Friend or Friend Group SFU
// Workspace is materialized with: a system Workspace bound to the built-in SFU
// Workflow and carrying no agent parameters.
func workspaceIsSocialSFU(item apitypes.Workspace) bool {
	return workspaceIsSystem(item) &&
		strings.TrimSpace(item.WorkflowId) == socialutil.SFUWorkflowID &&
		item.Parameters == nil
}

// GetRetiredSystemWorkspace returns an existing Social SFU Workspace
// retirement without creating one. Relationship services use this read-only
// check to authorize idempotent delete retries before writing any additional
// handoff.
func (s *Server) GetRetiredSystemWorkspace(ctx context.Context, name string, kind socialutil.SFUWorkspaceKind, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	name = strings.TrimSpace(name)
	if err := validateSocialRetirementRequest(kind, socialResourceID); err != nil {
		return apitypes.Workspace{}, err
	}
	unlock := s.IconLocks.LockOwner(name)
	defer unlock()
	return s.getRetiredSystemWorkspace(ctx, store, name, kind, socialResourceID)
}

// GetRetiredSystemWorkspaceByID returns an existing retirement by canonical
// Workspace ID without resolving an owner-scoped Peer name.
func (s *Server) GetRetiredSystemWorkspaceByID(ctx context.Context, id string, kind socialutil.SFUWorkspaceKind, socialResourceID string) (apitypes.Workspace, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: invalid id: %w", err)
	}
	if err := validateSocialRetirementRequest(kind, socialResourceID); err != nil {
		return apitypes.Workspace{}, err
	}
	unlock := s.IconLocks.LockOwner(id)
	defer unlock()
	return s.getRetiredSystemWorkspaceByID(ctx, store, id, kind, socialResourceID)
}

func (s *Server) getRetiredSystemWorkspace(
	ctx context.Context,
	store kv.Store,
	name string,
	kind socialutil.SFUWorkspaceKind,
	socialResourceID string,
) (apitypes.Workspace, error) {
	id, err := workspaceIDByName(ctx, store, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	return s.getRetiredSystemWorkspaceByID(ctx, store, id, kind, socialResourceID)
}

func (s *Server) getRetiredSystemWorkspaceByID(
	ctx context.Context,
	store kv.Store,
	id string,
	kind socialutil.SFUWorkspaceKind,
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
	var stored socialRetirementDescriptor
	if err := json.Unmarshal(record.Descriptor, &stored); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: decode Social retirement descriptor: %w", err)
	}
	descriptor, err := validateSocialRetirementRecord(record, stored.Name, kind, socialResourceID)
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

func validateSocialRetirementRecord(
	record pendingdeletion.Record,
	name string,
	kind socialutil.SFUWorkspaceKind,
	socialResourceID string,
) (socialRetirementDescriptor, error) {
	expectedReason, ok := socialRetirementReason(kind)
	if !ok {
		return socialRetirementDescriptor{}, fmt.Errorf("workspace: unsupported Social Workspace kind %q", kind)
	}
	if record.Reason != expectedReason {
		return socialRetirementDescriptor{}, fmt.Errorf(
			"workspace: PendingDeletion for %q has reason %q, want %q",
			name,
			record.Reason,
			expectedReason,
		)
	}
	var descriptor socialRetirementDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil {
		return socialRetirementDescriptor{}, fmt.Errorf(
			"workspace: decode Social retirement descriptor for %q: %w",
			name,
			err,
		)
	}
	if descriptor.ID != record.ResourceID ||
		strings.TrimSpace(descriptor.Name) != name ||
		descriptor.WorkspaceKind != kind ||
		descriptor.SocialResourceID != socialResourceID {
		return socialRetirementDescriptor{}, fmt.Errorf(
			"workspace: PendingDeletion for %q does not match %s resource %q",
			name,
			kind,
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
	descriptor := workspaceDeletionDescriptor{
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
	return s.createPendingDeletion(ctx, store, record)
}

func (s *Server) retirePeerPetWorkspaceRecord(ctx context.Context, store kv.Store, item apitypes.Workspace, owner string) error {
	if item.OwnerPublicKey == nil || *item.OwnerPublicKey != owner || !workspaceIsSystem(item) {
		return errors.New("workspace: invalid Peer-owned Pet system Workspace")
	}
	descriptor := workspaceDeletionDescriptor{
		ID: item.Id, Name: item.Name, OwnerPublicKey: cloneString(item.OwnerPublicKey),
		HasIcon: item.Icon != nil, System: true,
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace,
		item.Id,
		item.OwnerPublicKey,
		pendingdeletion.ReasonPeerDelete,
		descriptor,
		time.Now(),
	)
	if err != nil {
		return err
	}
	return s.createPendingDeletion(ctx, store, record)
}

func (s *Server) createPendingDeletion(ctx context.Context, store kv.Store, record pendingdeletion.Record) error {
	create := func(ctx context.Context) error {
		_, _, err := pendingdeletion.CreateOrGet(ctx, store, record)
		return err
	}
	if s.DeletionFencer == nil {
		return create(ctx)
	}
	return s.DeletionFencer.WithWorkspaceDeletionFence(ctx, record.ResourceID, create)
}

func (s *Server) GetWorkspace(ctx context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
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
	item, err := getWorkspace(ctx, store, strings.TrimSpace(name))
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if err := s.ensureWorkspaceAvailable(ctx, item.Id); err != nil {
		return apitypes.Workspace{}, err
	}
	if err := s.ensureWorkspaceOwnerAvailable(ctx, item); err != nil {
		return apitypes.Workspace{}, err
	}
	return item, nil
}

// GetAvailableWorkspaceByID returns the canonical Workspace record only while
// both the Workspace and its owner Peer remain available for runtime and
// background work. Admin GetWorkspace intentionally retains its diagnostic
// projection while deletion is pending.
func (s *Server) GetAvailableWorkspaceByID(ctx context.Context, id string) (apitypes.Workspace, error) {
	if s == nil {
		return apitypes.Workspace{}, errors.New("workspace: nil server")
	}
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Workspace{}, fmt.Errorf("workspace: invalid id: %w", err)
	}
	if err := s.ensureWorkspaceAvailable(ctx, id); err != nil {
		return apitypes.Workspace{}, err
	}
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	item, err := getWorkspaceByID(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Workspace{}, fmt.Errorf("%w: Workspace %q no longer exists", ErrWorkspaceDeleted, id)
		}
		return apitypes.Workspace{}, err
	}
	if err := s.ensureWorkspaceOwnerAvailable(ctx, item); err != nil {
		return apitypes.Workspace{}, err
	}
	return item, nil
}

// GetWorkspaceRuntimeByID returns runtime state by canonical Workspace ID.
func (s *Server) GetWorkspaceRuntimeByID(ctx context.Context, id string) (Runtime, error) {
	if s == nil {
		return Runtime{}, nil
	}
	if _, err := s.GetAvailableWorkspaceByID(ctx, id); err != nil {
		return Runtime{}, err
	}
	if s == nil || s.RuntimeStore == nil {
		return Runtime{}, nil
	}
	return s.RuntimeStore.GetWorkspaceRuntime(ctx, id)
}

func (s *Server) PutWorkspace(ctx context.Context, request adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	if request.Body == nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	id := string(request.Id)
	normalized, err := normalizeWorkspaceUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	if normalized.Id != id {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("RESOURCE_ID_MISMATCH", "body id must match path id")), nil
	}
	return s.putWorkspaceRecord(ctx, id, request.Body.Icon, func(apitypes.Workspace) (adminhttp.WorkspaceUpsert, error) {
		return normalized, nil
	})
}

// putWorkspaceRecord updates one stored Workspace under its record lock. build
// receives the record that lock protects, so a caller that patches individual
// fields never writes values read before the lock was held. icon is the icon
// projection the caller requested, which normalizeWorkspaceUpsert drops.
func (s *Server) putWorkspaceRecord(
	ctx context.Context,
	id string,
	icon *apitypes.Icon,
	build func(previous apitypes.Workspace) (adminhttp.WorkspaceUpsert, error),
) (adminhttp.PutWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	unlock := s.IconLocks.LockRecord(id)
	defer unlock()
	if err := s.ensureWorkspaceAvailable(ctx, id); err != nil {
		if errors.Is(err, ErrWorkspacePendingDeletion) {
			return adminhttp.PutWorkspace409JSONResponse(apitypes.NewErrorResponse(WorkspacePendingDeletionCode, err.Error())), nil
		}
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	previous, previousErr := getWorkspaceByID(ctx, store, id)
	if errors.Is(previousErr, kv.ErrNotFound) {
		return adminhttp.PutWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", id))), nil
	}
	if previousErr != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", previousErr.Error())), nil
	}
	if err := s.ensureWorkspaceOwnerAvailable(ctx, previous); err != nil {
		return adminhttp.PutWorkspace409JSONResponse(apitypes.NewErrorResponse(peerAvailabilityCode(err), err.Error())), nil
	}
	normalized, err := build(previous)
	if err != nil {
		if isInvalidWorkspaceReference(err) {
			return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
		}
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if normalized.Name != previous.Name {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", fmt.Sprintf("name %q must match immutable name %q", normalized.Name, previous.Name))), nil
	}
	if previousErr == nil && workspaceIsSystem(previous) &&
		!systemWorkspaceAllowsInputUpdate(previous, normalized) {
		return adminhttp.PutWorkspace409JSONResponse(apitypes.NewErrorResponse(
			SystemWorkspaceUpdateForbiddenCode,
			fmt.Sprintf("system workspace %q only permits changing the chat or pet input mode", previous.Name),
		)), nil
	}
	if err := s.validateReferences(ctx, normalized, true); err != nil {
		if isInvalidWorkspaceReference(err) {
			return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
		}
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := iconasset.ValidateProjection(previous.Icon, icon); err != nil {
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

func (s *Server) ensureWorkspaceOwnerAvailable(ctx context.Context, value apitypes.Workspace) error {
	if s == nil || s.PeerAvailability == nil || value.OwnerPublicKey == nil {
		return nil
	}
	return s.PeerAvailability(ctx, *value.OwnerPublicKey)
}

func (s *Server) ensureContextPeerAvailable(ctx context.Context) error {
	if s == nil || s.PeerAvailability == nil {
		return nil
	}
	owner, ok := ownership.FromContext(ctx)
	if !ok || strings.TrimSpace(owner) == "" {
		return nil
	}
	return s.PeerAvailability(ctx, owner)
}

func peerAvailabilityCode(err error) string {
	if errors.Is(err, ErrPeerDeleted) {
		return PeerDeletedCode
	}
	return PeerPendingDeletionCode
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
		(systemWorkspaceDomainParametersMatch(existing.Parameters, desired.Parameters) ||
			systemPetWorkspaceInputUpdate(existing.Parameters, desired.Parameters)) &&
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
		(systemWorkspaceDomainParametersMatch(existing.Parameters, desired.Parameters) ||
			systemPetWorkspaceInputUpdate(existing.Parameters, desired.Parameters))
}

func systemPetWorkspaceInputUpdate(existing, desired *apitypes.WorkspaceParameters) bool {
	existingPet, existingIsPet := petWorkspaceInput(existing)
	desiredPet, desiredIsPet := petWorkspaceInput(desired)
	if !existingIsPet && !desiredIsPet {
		return false
	}
	return (existing == nil || existingIsPet) &&
		(desired == nil || desiredIsPet) &&
		(existingPet == nil || existingPet.Valid()) &&
		(desiredPet == nil || desiredPet.Valid())
}

func petWorkspaceInput(parameters *apitypes.WorkspaceParameters) (*apitypes.WorkspaceInputMode, bool) {
	if parameters == nil {
		return nil, false
	}
	value, err := parameters.AsPetWorkspaceParameters()
	if err != nil || !value.AgentType.Valid() {
		return nil, false
	}
	return value.Input, true
}

func systemWorkspaceDomainParametersMatch(existing, desired *apitypes.WorkspaceParameters) bool {
	if existing == nil || desired == nil {
		return existing == nil && desired == nil
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

func (s *Server) ensureWorkspaceAvailable(ctx context.Context, id string) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindWorkspace, id)
	if err != nil {
		return err
	}
	if pending {
		return fmt.Errorf("%w: Workspace %q cannot be used", ErrWorkspacePendingDeletion, id)
	}
	return nil
}

func normalizeWorkspaceUpsert(in adminhttp.WorkspaceUpsert, expectedName string) (adminhttp.WorkspaceUpsert, error) {
	id := in.Id
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
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
	workflowID := string(in.WorkflowId)
	if err := customid.ValidateResourceID(workflowID); err != nil {
		return adminhttp.WorkspaceUpsert{}, fmt.Errorf("workflow_id: %w", err)
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
		Id:         id,
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

func (s *Server) validateReferences(ctx context.Context, workspace adminhttp.WorkspaceUpsert, directWorkflow bool) error {
	workflowName, runtimeAlias, err := resolveWorkflowReference(ctx, workspace, directWorkflow)
	if err != nil {
		return err
	}
	workflow, err := s.getWorkflow(ctx, workflowName)
	if err != nil {
		return err
	}
	if workflow.Spec.Driver == apitypes.WorkflowDriverPet {
		return validatePetOverrides(workflow.Spec.Pet, workspace.Parameters)
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
		modelID = bindings[reference.ModelID]
		if modelID == "" {
			return invalidWorkspaceReference("flowcraft parameter %q references missing runtime Model alias %q", reference.Role, reference.ModelID)
		}
		if err := s.validateModelKind(ctx, "flowcraft parameter", reference.Role, modelID, visibleModelID, reference.Kind); err != nil {
			return err
		}
	}
	return nil
}

func validatePetOverrides(workflow *apitypes.PetWorkflowSpec, workspaceParameters *apitypes.WorkspaceParameters) error {
	if workspaceParameters == nil {
		return nil
	}
	if err := requireWorkspaceParametersVariant(workspaceParameters, "pet"); err != nil {
		return invalidWorkspaceReference("pet parameters are required: %v", err)
	}
	parameters, err := workspaceParameters.AsPetWorkspaceParameters()
	if err != nil {
		return invalidWorkspaceReference("pet parameters are required: %v", err)
	}
	if !parameters.AgentType.Valid() {
		return invalidWorkspaceReference("pet parameters.agent_type %q is unsupported", parameters.AgentType)
	}
	if parameters.Input == nil {
		return nil
	}
	if !parameters.Input.Valid() {
		return invalidWorkspaceReference("pet parameters.input %q is unsupported", *parameters.Input)
	}
	if workflow == nil {
		return invalidWorkspaceReference("pet workflow spec is required")
	}
	switch workflow.Driver {
	case apitypes.ReusableWorkflowDriverFlowcraft,
		apitypes.ReusableWorkflowDriverDoubaoRealtime,
		apitypes.ReusableWorkflowDriverEino,
		apitypes.ReusableWorkflowDriverAstTranslate:
		return nil
	default:
		return invalidWorkspaceReference("pet nested workflow driver %q does not support workspace input switching", workflow.Driver)
	}
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
	parameters, err := workspaceParameters.AsEinoWorkspaceParameters()
	if err != nil {
		return invalidWorkspaceReference("eino parameters are required: %v", err)
	}
	if parameters.Input != nil && !parameters.Input.Valid() {
		return invalidWorkspaceReference("eino parameter %q input %q is unsupported", "input", *parameters.Input)
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
	modelID := bindings[alias]
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
	voiceID := bindings[alias]
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
			modelID := bindings[alias]
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
	if bindings[alias] == "" {
		return invalidWorkspaceReference("ast-translate parameter %q references missing runtime Voice alias %q", "voice.tts_voice", alias)
	}
	return nil
}

func resolveWorkflowReference(ctx context.Context, workspace adminhttp.WorkspaceUpsert, _ bool) (string, bool, error) {
	workflowID := string(workspace.WorkflowId)
	_, runtimeBound := ctx.Value(runtimeWorkflowBindingsContextKey{}).(map[string]string)
	return workflowID, runtimeBound, nil
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

func (s *Server) getWorkflow(ctx context.Context, id string) (apitypes.Workflow, error) {
	if s == nil || s.Workflows == nil {
		return apitypes.Workflow{}, errors.New("workflow service not configured")
	}
	response, err := s.Workflows.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: id})
	if err != nil {
		return apitypes.Workflow{}, err
	}
	switch response := response.(type) {
	case adminhttp.GetWorkflow200JSONResponse:
		return apitypes.Workflow(response), nil
	case adminhttp.GetWorkflow404JSONResponse:
		return apitypes.Workflow{}, invalidWorkspaceReference("workflow %q not found", id)
	case adminhttp.GetWorkflow500JSONResponse:
		return apitypes.Workflow{}, fmt.Errorf("get workflow %q: %s", id, response.Error.Message)
	default:
		return apitypes.Workflow{}, fmt.Errorf("get workflow %q: unexpected response %T", id, response)
	}
}
