package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

var (
	workspacesRoot = kv.Key{"by-name"}
	workflowsRoot  = kv.Key{"by-name"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store         kv.Store
	WorkflowStore kv.Store
	RuntimeStore  RuntimeStore
	Authorizer    Authorizer
}

type WorkspaceAdminService interface {
	ListWorkspaces(context.Context, adminservice.ListWorkspacesRequestObject) (adminservice.ListWorkspacesResponseObject, error)
	CreateWorkspace(context.Context, adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error)
	DeleteWorkspace(context.Context, adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error)
	GetWorkspace(context.Context, adminservice.GetWorkspaceRequestObject) (adminservice.GetWorkspaceResponseObject, error)
	PutWorkspace(context.Context, adminservice.PutWorkspaceRequestObject) (adminservice.PutWorkspaceResponseObject, error)
}

var _ WorkspaceAdminService = (*Server)(nil)

func (s *Server) ListWorkspaces(ctx context.Context, request adminservice.ListWorkspacesRequestObject) (adminservice.ListWorkspacesResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.ListWorkspaces500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listWorkspacePage(ctx, store, workspacesRoot, cursor, limit)
	if err != nil {
		return adminservice.ListWorkspaces500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListWorkspaces200JSONResponse(adminservice.WorkspaceList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateWorkspace(ctx context.Context, request adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	normalized, err := normalizeWorkspaceUpsert(*request.Body, "")
	if err != nil {
		return adminservice.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	workflowStore, err := s.workflowStore()
	if err != nil {
		return adminservice.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateReferences(ctx, workflowStore, normalized); err != nil {
		return adminservice.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	if _, err := store.Get(ctx, workspaceKey(string(normalized.Name))); err == nil {
		return adminservice.CreateWorkspace409JSONResponse(apitypes.NewErrorResponse("WORKSPACE_ALREADY_EXISTS", fmt.Sprintf("workspace %q already exists", normalized.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := time.Now().UTC()
	workspace := apitypes.Workspace{
		CreatedAt:    now,
		LastActiveAt: now,
		Name:         normalized.Name,
		Parameters:   cloneParameters(normalized.Parameters),
		UpdatedAt:    now,
		WorkflowName: normalized.WorkflowName,
	}
	if s.RuntimeStore != nil {
		if _, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Name); err != nil {
			return adminservice.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	}
	if err := writeWorkspace(ctx, store, workspace); err != nil {
		return adminservice.CreateWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateWorkspace200JSONResponse(workspace), nil
}

func (s *Server) DeleteWorkspace(ctx context.Context, request adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	workspace, err := getWorkspace(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			if s.RuntimeStore != nil {
				if err := s.RuntimeStore.DeleteWorkspaceRuntime(ctx, name); err != nil {
					return adminservice.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
				}
			}
			return adminservice.DeleteWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", name))), nil
		}
		return adminservice.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if s.RuntimeStore != nil {
		if err := s.RuntimeStore.DeleteWorkspaceRuntime(ctx, workspace.Name); err != nil {
			return adminservice.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	}
	if err := store.BatchDelete(ctx, []kv.Key{
		workspaceKey(string(workspace.Name)),
	}); err != nil {
		return adminservice.DeleteWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteWorkspace200JSONResponse(workspace), nil
}

func (s *Server) GetWorkspace(ctx context.Context, request adminservice.GetWorkspaceRequestObject) (adminservice.GetWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	workspace, err := getWorkspace(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", fmt.Sprintf("workspace %q not found", name))), nil
		}
		return adminservice.GetWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetWorkspace200JSONResponse(workspace), nil
}

func (s *Server) GetWorkspaceRuntime(ctx context.Context, name string) (Runtime, error) {
	if s == nil || s.RuntimeStore == nil {
		return Runtime{}, nil
	}
	return s.RuntimeStore.GetWorkspaceRuntime(ctx, name)
}

func (s *Server) PutWorkspace(ctx context.Context, request adminservice.PutWorkspaceRequestObject) (adminservice.PutWorkspaceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	normalized, err := normalizeWorkspaceUpsert(*request.Body, name)
	if err != nil {
		return adminservice.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	workflowStore, err := s.workflowStore()
	if err != nil {
		return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateReferences(ctx, workflowStore, normalized); err != nil {
		return adminservice.PutWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", err.Error())), nil
	}
	previous, err := getWorkspace(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := time.Now().UTC()
	workspace := apitypes.Workspace{
		CreatedAt:    now,
		LastActiveAt: now,
		Name:         normalized.Name,
		Parameters:   cloneParameters(normalized.Parameters),
		UpdatedAt:    now,
		WorkflowName: normalized.WorkflowName,
	}
	if err == nil {
		workspace.CreatedAt = previous.CreatedAt
		workspace.LastActiveAt = previous.LastActiveAt
	}
	if s.RuntimeStore != nil {
		if _, err := s.RuntimeStore.PrepareWorkspace(ctx, workspace.Name); err != nil {
			return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	}
	if err := writeWorkspace(ctx, store, workspace); err != nil {
		return adminservice.PutWorkspace500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutWorkspace200JSONResponse(workspace), nil
}

func writeWorkspace(ctx context.Context, store kv.Store, workspace apitypes.Workspace) error {
	data, err := json.Marshal(workspace)
	if err != nil {
		return fmt.Errorf("workspace: encode %s: %w", workspace.Name, err)
	}
	if err := store.Set(ctx, workspaceKey(string(workspace.Name)), data); err != nil {
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
	if workspace.LastActiveAt.IsZero() {
		workspace.LastActiveAt = workspace.CreatedAt
	}
	if workspace.LastActiveAt.IsZero() {
		workspace.LastActiveAt = workspace.UpdatedAt
	}
	return workspace
}

func normalizeWorkspaceUpsert(in adminservice.WorkspaceUpsert, expectedName string) (adminservice.WorkspaceUpsert, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return adminservice.WorkspaceUpsert{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return adminservice.WorkspaceUpsert{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	workflowName := strings.TrimSpace(string(in.WorkflowName))
	if workflowName == "" {
		return adminservice.WorkspaceUpsert{}, errors.New("workflow_name is required")
	}
	return adminservice.WorkspaceUpsert{
		Name:         string(name),
		Parameters:   cloneParameters(in.Parameters),
		WorkflowName: string(workflowName),
	}, nil
}

func validateReferences(ctx context.Context, store kv.Store, workspace adminservice.WorkspaceUpsert) error {
	if _, err := store.Get(ctx, workflowReferenceKey(string(workspace.WorkflowName))); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return fmt.Errorf("workflow %q not found", workspace.WorkflowName)
		}
		return err
	}
	return nil
}

func workspaceKey(name string) kv.Key {
	return append(append(kv.Key{}, workspacesRoot...), escapeStoreSegment(name))
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
