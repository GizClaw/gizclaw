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
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var workflowsRoot = kv.Key{"by-name"}

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
	if err := s.Store.Set(ctx, key, raw); err != nil {
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
	if err := s.Store.Delete(ctx, key); err != nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteWorkflow200JSONResponse(doc), nil
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
	if getErr == nil {
		_, err = decodeWorkflow(previousData)
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
	if err := s.Store.Set(ctx, workflowKey(doc.Name), raw); err != nil {
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
	case apitypes.WorkflowDriverFlowcraft:
		if spec.Flowcraft == nil {
			return errors.New("spec.flowcraft is required")
		}
		if err := spec.Flowcraft.Validate(); err != nil {
			return fmt.Errorf("spec.flowcraft: %w", err)
		}
		return nil
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
		if spec.DoubaoRealtime.Tools != nil && len(*spec.DoubaoRealtime.Tools) != 0 {
			return errors.New("spec.doubao_realtime.tools are unsupported until ToolCall is implemented")
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
