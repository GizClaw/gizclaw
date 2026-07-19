package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	workflowsRoot        = kv.Key{"by-name"}
	workflowsByOwnerRoot = kv.Key{"by-owner"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store kv.Store
}

type WorkflowAdminService interface {
	ListWorkflows(context.Context, adminhttp.ListWorkflowsRequestObject) (adminhttp.ListWorkflowsResponseObject, error)
	CreateWorkflow(context.Context, adminhttp.CreateWorkflowRequestObject) (adminhttp.CreateWorkflowResponseObject, error)
	DeleteWorkflow(context.Context, adminhttp.DeleteWorkflowRequestObject) (adminhttp.DeleteWorkflowResponseObject, error)
	GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error)
	PutWorkflow(context.Context, adminhttp.PutWorkflowRequestObject) (adminhttp.PutWorkflowResponseObject, error)
}

var _ WorkflowAdminService = (*Server)(nil)

type workflowEnvelope struct {
	Name string           `json:"name"`
	Spec *json.RawMessage `json:"spec"`
}

func (s *Server) ListWorkflows(ctx context.Context, request adminhttp.ListWorkflowsRequestObject) (adminhttp.ListWorkflowsResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	entries, err := kv.ListAfter(ctx, s.Store, workflowsRoot, cursorAfterKey(workflowsRoot, cursor), limit+1)
	if err != nil {
		return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.Workflow, 0)
	for _, entry := range pageEntries {
		doc, err := decodeWorkflow(entry.Value)
		if err != nil {
			return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
		items = append(items, doc)
	}
	return adminhttp.ListWorkflows200JSONResponse(adminhttp.WorkflowList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateWorkflow(ctx context.Context, request adminhttp.CreateWorkflowRequestObject) (adminhttp.CreateWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "request body required")), nil
	}
	doc, raw, err := validateWorkflow(*request.Body, "")
	if err != nil {
		return adminhttp.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	key := workflowKey(doc.Name)
	if _, err := s.Store.Get(ctx, key); err == nil {
		return adminhttp.CreateWorkflow409JSONResponse(apitypes.NewErrorResponse("WORKFLOW_ALREADY_EXISTS", fmt.Sprintf("workflow %q already exists", doc.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	doc.OwnerPublicKey = nil
	if owner, ok := ownership.FromContext(ctx); ok {
		doc.OwnerPublicKey = &owner
	}
	raw, err = json.Marshal(doc)
	if err != nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	entries := []kv.Entry{{Key: key, Value: raw}}
	if doc.OwnerPublicKey != nil {
		entries = append(entries, kv.Entry{Key: workflowByOwnerKey(*doc.OwnerPublicKey, doc.Name), Value: []byte{}})
	}
	if err := s.Store.BatchSet(ctx, entries); err != nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateWorkflow200JSONResponse(doc), nil
}

func (s *Server) DeleteWorkflow(ctx context.Context, request adminhttp.DeleteWorkflowRequestObject) (adminhttp.DeleteWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	key := workflowKey(name)
	data, err := s.Store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", name))), nil
		}
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	doc, err := decodeWorkflow(data)
	if err != nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	keys := []kv.Key{key}
	if doc.OwnerPublicKey != nil {
		keys = append(keys, workflowByOwnerKey(*doc.OwnerPublicKey, doc.Name))
	}
	if err := s.Store.BatchDelete(ctx, keys); err != nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteWorkflow200JSONResponse(doc), nil
}

// ListWorkflowsByOwner reads the owner index used by the public RPC owned source.
func (s *Server) ListWorkflowsByOwner(ctx context.Context, owner string) ([]apitypes.Workflow, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("workflow store not configured")
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return []apitypes.Workflow{}, nil
	}
	items := make([]apitypes.Workflow, 0)
	for entry, err := range s.Store.List(ctx, workflowByOwnerPrefix(owner)) {
		if err != nil {
			return nil, fmt.Errorf("workflows: list owner %s: %w", owner, err)
		}
		if len(entry.Key) == 0 {
			continue
		}
		name := unescapeStoreSegment(entry.Key[len(entry.Key)-1])
		data, err := s.Store.Get(ctx, workflowKey(name))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		item, err := decodeWorkflow(data)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) GetWorkflow(ctx context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	data, err := s.Store.Get(ctx, workflowKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", name))), nil
		}
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	doc, err := decodeWorkflow(data)
	if err != nil {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetWorkflow200JSONResponse(doc), nil
}

func (s *Server) PutWorkflow(ctx context.Context, request adminhttp.PutWorkflowRequestObject) (adminhttp.PutWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	previousData, getErr := s.Store.Get(ctx, workflowKey(name))
	var previous apitypes.Workflow
	if getErr == nil {
		previous, err = decodeWorkflow(previousData)
		if err != nil {
			return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	} else if !errors.Is(getErr, kv.ErrNotFound) {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", getErr.Error())), nil
	}
	body := *request.Body
	doc, raw, err := validateWorkflow(body, name)
	if err != nil {
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	doc.OwnerPublicKey = nil
	if getErr == nil {
		doc.OwnerPublicKey = previous.OwnerPublicKey
	} else if owner, ok := ownership.FromContext(ctx); ok {
		doc.OwnerPublicKey = &owner
	}
	raw, err = json.Marshal(doc)
	if err != nil {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	entries := []kv.Entry{{Key: workflowKey(doc.Name), Value: raw}}
	if doc.OwnerPublicKey != nil {
		entries = append(entries, kv.Entry{Key: workflowByOwnerKey(*doc.OwnerPublicKey, doc.Name), Value: []byte{}})
	}
	if err := s.Store.BatchSet(ctx, entries); err != nil {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutWorkflow200JSONResponse(doc), nil
}

func validateWorkflow(item apitypes.Workflow, expectedName string) (apitypes.Workflow, []byte, error) {
	var env workflowEnvelope
	raw, err := json.Marshal(item)
	if err != nil {
		return apitypes.Workflow{}, nil, err
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return apitypes.Workflow{}, nil, err
	}
	if err := customid.ValidateField("name", env.Name); err != nil {
		return apitypes.Workflow{}, nil, err
	}
	if env.Spec == nil || bytes.Equal(bytes.TrimSpace(*env.Spec), []byte("null")) {
		return apitypes.Workflow{}, nil, errors.New("spec is required")
	}
	if expectedName != "" {
		if err := customid.ValidateField("path name", expectedName); err != nil {
			return apitypes.Workflow{}, nil, err
		}
		if env.Name != expectedName {
			return apitypes.Workflow{}, nil, fmt.Errorf("name %q must match path name %q", env.Name, expectedName)
		}
	}
	if strings.TrimSpace(string(item.Spec.Driver)) == "" {
		return apitypes.Workflow{}, nil, errors.New("spec.driver is required")
	}
	if !item.Spec.Driver.Valid() {
		return apitypes.Workflow{}, nil, fmt.Errorf("unsupported spec.driver %q", item.Spec.Driver)
	}
	if err := validateDriverSpec(item.Spec); err != nil {
		return apitypes.Workflow{}, nil, err
	}
	policy, err := toolkit.NormalizePolicy(item.Spec.Toolkit)
	if err != nil {
		return apitypes.Workflow{}, nil, fmt.Errorf("spec.toolkit: %w", err)
	}

	item.Name = env.Name
	item.Spec.Toolkit = policy
	raw, err = json.Marshal(item)
	if err != nil {
		return apitypes.Workflow{}, nil, err
	}
	return item, raw, nil
}

func validateDriverSpec(spec apitypes.WorkflowSpec) error {
	switch spec.Driver {
	case apitypes.WorkflowDriverChatroom:
		if spec.Chatroom == nil {
			return errors.New("spec.chatroom is required")
		}
		return nil
	case apitypes.WorkflowDriverPet:
		if spec.Pet == nil {
			return errors.New("spec.pet is required")
		}
		if len(*spec.Pet) != 0 {
			return errors.New("spec.pet does not accept Flowcraft graph or memory configuration")
		}
		return nil
	case apitypes.WorkflowDriverDoubaoRealtime:
		if spec.DoubaoRealtime == nil {
			return errors.New("spec.doubao_realtime is required")
		}
		if strings.TrimSpace(spec.DoubaoRealtime.Model) == "" {
			return errors.New("spec.doubao_realtime.model is required")
		}
		return nil
	case apitypes.WorkflowDriverDashscopeRealtime:
		if spec.DashscopeRealtime == nil {
			return errors.New("spec.dashscope_realtime is required")
		}
		if strings.TrimSpace(spec.DashscopeRealtime.Model) == "" {
			return errors.New("spec.dashscope_realtime.model is required")
		}
		if spec.DashscopeRealtime.MaxToolCalls != nil && *spec.DashscopeRealtime.MaxToolCalls < 0 {
			return errors.New("spec.dashscope_realtime.max_tool_calls cannot be negative")
		}
		return nil
	case apitypes.WorkflowDriverEino:
		if spec.Eino == nil {
			return errors.New("spec.eino is required")
		}
		if strings.TrimSpace(spec.Eino.Model) == "" {
			return errors.New("spec.eino.model is required")
		}
		if spec.Eino.MaxSteps != nil && *spec.Eino.MaxSteps <= 0 {
			return errors.New("spec.eino.max_steps must be positive")
		}
		if spec.Eino.MaxToolCalls != nil && *spec.Eino.MaxToolCalls < 0 {
			return errors.New("spec.eino.max_tool_calls cannot be negative")
		}
		return nil
	default:
		return nil
	}
}

func decodeWorkflow(data []byte) (apitypes.Workflow, error) {
	var item apitypes.Workflow
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.Workflow{}, err
	}
	validated, _, err := validateWorkflow(item, "")
	if err != nil {
		return apitypes.Workflow{}, err
	}
	return validated, nil
}

func workflowKey(name string) kv.Key {
	return append(append(kv.Key{}, workflowsRoot...), escapeStoreSegment(name))
}

func workflowByOwnerPrefix(owner string) kv.Key {
	return append(append(kv.Key{}, workflowsByOwnerRoot...), escapeStoreSegment(owner))
}

func workflowByOwnerKey(owner, name string) kv.Key {
	return append(workflowByOwnerPrefix(owner), escapeStoreSegment(name))
}

func unescapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%3A", ":")
	return strings.ReplaceAll(value, "%25", "%")
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
