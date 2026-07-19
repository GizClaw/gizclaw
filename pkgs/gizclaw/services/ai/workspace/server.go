package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
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
	SystemWorkspaceDeleteForbiddenCode = "SYSTEM_WORKSPACE_DELETE_FORBIDDEN"
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

// WithRuntimeWorkflowBindings attaches the authenticated connection's current
// RuntimeProfile Workflow alias snapshot to Workspace validation.
func WithRuntimeWorkflowBindings(ctx context.Context, bindings map[string]string) context.Context {
	cloned := make(map[string]string, len(bindings))
	for alias, name := range bindings {
		cloned[alias] = name
	}
	return context.WithValue(ctx, runtimeWorkflowBindingsContextKey{}, cloned)
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
	items, hasNext, nextCursor, err := listWorkspacePage(ctx, store, workspacesRoot, cursor, limit)
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
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
	normalized, err := normalizeWorkspaceUpsert(body, "")
	if err != nil {
		return apitypes.Workspace{}, false, err
	}
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
		CreatedAt:      now,
		LastActiveAt:   now,
		Name:           normalized.Name,
		Parameters:     cloneParameters(normalized.Parameters),
		System:         boolPointer(system),
		Toolkit:        cloneToolkitPolicy(normalized.Toolkit),
		UpdatedAt:      now,
		WorkflowName:   normalized.WorkflowName,
		WorkflowSource: workspaceSource(normalized.WorkflowSource),
	}
	if owner, ok := ownership.FromContext(ctx); ok && !system {
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
			if s.RuntimeStore != nil {
				if err := s.RuntimeStore.DeleteWorkspaceRuntime(ctx, name); err != nil {
					return adminhttp.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
				}
			}
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
	if err := s.deleteWorkspaceRecord(ctx, store, workspace); err != nil {
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
	if workspace.OwnerPublicKey != nil {
		keys = append(keys, workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name))
	}
	return store.BatchDelete(ctx, keys)
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
	store, err := s.store()
	if err != nil {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	unlock := s.IconLocks.LockRecord(name)
	defer unlock()
	normalized, err := normalizeWorkspaceUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
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
	previous, err := getWorkspace(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := iconasset.ValidateProjection(previous.Icon, request.Body.Icon); err != nil {
		return adminhttp.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	now := time.Now().UTC()
	workspace := apitypes.Workspace{
		CreatedAt:      now,
		LastActiveAt:   now,
		Name:           normalized.Name,
		Parameters:     cloneParameters(normalized.Parameters),
		System:         boolPointer(false),
		Toolkit:        cloneToolkitPolicy(normalized.Toolkit),
		UpdatedAt:      now,
		WorkflowName:   normalized.WorkflowName,
		WorkflowSource: workspaceSource(normalized.WorkflowSource),
		Icon:           previous.Icon,
	}
	if err == nil {
		workspace.CreatedAt = previous.CreatedAt
		workspace.LastActiveAt = previous.LastActiveAt
		workspace.System = previous.System
		workspace.OwnerPublicKey = cloneString(previous.OwnerPublicKey)
	}
	if err != nil {
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
	if workspace.OwnerPublicKey != nil {
		entries = append(entries, kv.Entry{Key: workspaceByOwnerKey(*workspace.OwnerPublicKey, workspace.Name), Value: []byte{}})
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("workspace: write %s: %w", workspace.Name, err)
	}
	return nil
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

func listWorkspacePage(ctx context.Context, store kv.Store, prefix kv.Key, cursor string, limit int) ([]apitypes.Workspace, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.Workspace, 0, len(pageEntries))
	for _, entry := range pageEntries {
		var workspace apitypes.Workspace
		if err := json.Unmarshal(entry.Value, &workspace); err != nil {
			return nil, false, nil, fmt.Errorf("workspace: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, normalizeWorkspaceTimestamps(workspace))
	}
	return items, hasNext, nextCursor, nil
}

func normalizeWorkspaceTimestamps(workspace apitypes.Workspace) apitypes.Workspace {
	if workspace.System == nil {
		workspace.System = boolPointer(false)
	}
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
	if in.WorkflowSource != nil && *in.WorkflowSource == adminhttp.Runtime {
		workflowName = strings.TrimSpace(workflowName)
		if workflowName == "" {
			return adminhttp.WorkspaceUpsert{}, errors.New("workflow_name: runtime alias is required")
		}
	} else if err := customid.ValidateField("workflow_name", workflowName); err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
	policy, err := toolkit.NormalizePolicy(in.Toolkit)
	if err != nil {
		return adminhttp.WorkspaceUpsert{}, err
	}
	return adminhttp.WorkspaceUpsert{
		Name:           string(name),
		Parameters:     cloneParameters(in.Parameters),
		Toolkit:        policy,
		WorkflowName:   string(workflowName),
		WorkflowSource: cloneAdminWorkspaceSource(in.WorkflowSource),
	}, nil
}

func (s *Server) validateReferences(ctx context.Context, store kv.Store, workspace adminhttp.WorkspaceUpsert) error {
	workflowName, err := resolveWorkflowReference(ctx, store, workspace)
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
	if workflow.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		return nil
	}
	references, err := ResolveFlowcraftModelReferences(workflow, workspace.Parameters)
	if err != nil {
		return err
	}
	for _, reference := range references {
		if err := s.validateGeneratorModel(ctx, reference.Role, reference.ModelID); err != nil {
			return err
		}
	}
	return nil
}

func resolveWorkflowReference(ctx context.Context, store kv.Store, workspace adminhttp.WorkspaceUpsert) (string, error) {
	name := string(workspace.WorkflowName)
	if workspace.WorkflowSource == nil {
		return name, nil
	}
	switch *workspace.WorkflowSource {
	case adminhttp.Runtime:
		bindings, _ := ctx.Value(runtimeWorkflowBindingsContextKey{}).(map[string]string)
		resolved := strings.TrimSpace(bindings[name])
		if resolved == "" {
			return "", invalidWorkspaceReference("runtime workflow alias %q not found", name)
		}
		return resolved, nil
	case adminhttp.Owned:
		owner, ok := ownership.FromContext(ctx)
		if !ok {
			return "", invalidWorkspaceReference("owned workflow source requires an authenticated owner")
		}
		data, err := store.Get(ctx, workflowReferenceKey(name))
		if errors.Is(err, kv.ErrNotFound) {
			return "", invalidWorkspaceReference("owned workflow %q not found", name)
		}
		if err != nil {
			return "", err
		}
		var workflow apitypes.Workflow
		if err := json.Unmarshal(data, &workflow); err != nil {
			return "", fmt.Errorf("decode workflow %q: %w", name, err)
		}
		if workflow.OwnerPublicKey == nil || *workflow.OwnerPublicKey != owner {
			return "", invalidWorkspaceReference("owned workflow %q not found", name)
		}
		return name, nil
	default:
		return "", invalidWorkspaceReference("unsupported workflow_source %q", *workspace.WorkflowSource)
	}
}

func workspaceSource(source *adminhttp.WorkspaceUpsertWorkflowSource) *apitypes.WorkspaceWorkflowSource {
	if source == nil {
		return nil
	}
	value := apitypes.WorkspaceWorkflowSource(*source)
	return &value
}

func cloneAdminWorkspaceSource(source *adminhttp.WorkspaceUpsertWorkflowSource) *adminhttp.WorkspaceUpsertWorkflowSource {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

// FlowcraftModelReference is one effective Model selected for a FlowCraft role.
type FlowcraftModelReference struct {
	Role    string
	ModelID string
}

// ResolveFlowcraftModelReferences resolves Workspace overrides and Workflow
// settings into the concrete Models used by a FlowCraft runtime.
func ResolveFlowcraftModelReferences(workflow apitypes.Workflow, workspaceParameters *apitypes.WorkspaceParameters) ([]FlowcraftModelReference, error) {
	if workflow.Spec.Driver != apitypes.WorkflowDriverFlowcraft {
		return nil, nil
	}
	parameters := apitypes.FlowcraftWorkspaceParameters{}
	if workspaceParameters != nil {
		var err error
		parameters, err = workspaceParameters.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, invalidWorkspaceReference("flowcraft parameters are required: %v", err)
		}
	}
	settings := map[string]any{}
	if workflow.Spec.Flowcraft != nil {
		if configured, ok := (*workflow.Spec.Flowcraft)["settings"].(map[string]any); ok {
			settings = configured
		}
	}
	roles := []struct {
		name     string
		value    *string
		required bool
	}{
		{name: "generate_model", value: parameters.GenerateModel, required: true},
		{name: "extract_model", value: parameters.ExtractModel},
		{name: "embedding_model", value: parameters.EmbeddingModel},
	}
	references := make([]FlowcraftModelReference, 0, len(roles))
	for _, role := range roles {
		modelID, required := resolveFlowcraftModel(role.name, role.value, settings, role.required)
		if modelID == "" {
			if required {
				return nil, invalidWorkspaceReference("flowcraft parameter %q requires a concrete Model resource name", role.name)
			}
			continue
		}
		references = append(references, FlowcraftModelReference{Role: role.name, ModelID: modelID})
	}
	return references, nil
}

func resolveFlowcraftModel(name string, workspaceValue *string, settings map[string]any, required bool) (string, bool) {
	if workspaceValue != nil {
		if value := strings.TrimSpace(*workspaceValue); value != "" {
			if value == name {
				return "", true
			}
			return value, true
		}
	}
	configured, _ := settings[name].(string)
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", required
	}
	if configured == name {
		return "", true
	}
	return configured, true
}

func (s *Server) validateGeneratorModel(ctx context.Context, role, modelID string) error {
	if s == nil || s.Models == nil {
		return errors.New("model service not configured")
	}
	response, err := s.Models.GetModel(ctx, adminhttp.GetModelRequestObject{Id: modelID})
	if err != nil {
		return err
	}
	model, ok := response.(adminhttp.GetModel200JSONResponse)
	if _, missing := response.(adminhttp.GetModel404JSONResponse); missing {
		return invalidWorkspaceReference("flowcraft parameter %q references missing Model %q", role, modelID)
	}
	if !ok {
		return fmt.Errorf("validate flowcraft parameter %q Model %q: model service returned %T", role, modelID, response)
	}
	if model.Kind != apitypes.ModelKindLlm {
		return invalidWorkspaceReference("flowcraft parameter %q Model %q has kind %q, want %q", role, modelID, model.Kind, apitypes.ModelKindLlm)
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
