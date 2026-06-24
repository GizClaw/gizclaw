package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
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
	ListWorkflows(context.Context, adminservice.ListWorkflowsRequestObject) (adminservice.ListWorkflowsResponseObject, error)
	CreateWorkflow(context.Context, adminservice.CreateWorkflowRequestObject) (adminservice.CreateWorkflowResponseObject, error)
	DeleteWorkflow(context.Context, adminservice.DeleteWorkflowRequestObject) (adminservice.DeleteWorkflowResponseObject, error)
	GetWorkflow(context.Context, adminservice.GetWorkflowRequestObject) (adminservice.GetWorkflowResponseObject, error)
	PutWorkflow(context.Context, adminservice.PutWorkflowRequestObject) (adminservice.PutWorkflowResponseObject, error)
}

var _ WorkflowAdminService = (*Server)(nil)

type documentEnvelope struct {
	Metadata workflowMetadata `json:"metadata"`
	Spec     *json.RawMessage `json:"spec"`
}

type workflowMetadata struct {
	Name string `json:"name"`
}

func (s *Server) ListWorkflows(ctx context.Context, request adminservice.ListWorkflowsRequestObject) (adminservice.ListWorkflowsResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminservice.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	entries, err := kv.ListAfter(ctx, s.Store, workflowsRoot, cursorAfterKey(workflowsRoot, cursor), limit+1)
	if err != nil {
		return adminservice.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.WorkflowDocument, 0)
	for _, entry := range pageEntries {
		doc, err := decodeDocument(entry.Value)
		if err != nil {
			return adminservice.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
		items = append(items, doc)
	}
	return adminservice.ListWorkflows200JSONResponse(adminservice.WorkflowList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateWorkflow(ctx context.Context, request adminservice.CreateWorkflowRequestObject) (adminservice.CreateWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminservice.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	if request.Body == nil {
		return adminservice.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "request body required")), nil
	}
	doc, env, raw, err := validateDocument(*request.Body, "")
	if err != nil {
		return adminservice.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	key := workflowKey(env.Metadata.Name)
	if _, err := s.Store.Get(ctx, key); err == nil {
		return adminservice.CreateWorkflow409JSONResponse(apitypes.NewErrorResponse("WORKFLOW_ALREADY_EXISTS", fmt.Sprintf("workflow %q already exists", env.Metadata.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.Store.Set(ctx, key, raw); err != nil {
		return adminservice.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateWorkflow200JSONResponse(doc), nil
}

func (s *Server) DeleteWorkflow(ctx context.Context, request adminservice.DeleteWorkflowRequestObject) (adminservice.DeleteWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminservice.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	key := workflowKey(name)
	data, err := s.Store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", name))), nil
		}
		return adminservice.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	doc, err := decodeDocument(data)
	if err != nil {
		return adminservice.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.Store.Delete(ctx, key); err != nil {
		return adminservice.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteWorkflow200JSONResponse(doc), nil
}

func (s *Server) GetWorkflow(ctx context.Context, request adminservice.GetWorkflowRequestObject) (adminservice.GetWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminservice.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	data, err := s.Store.Get(ctx, workflowKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", name))), nil
		}
		return adminservice.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	doc, err := decodeDocument(data)
	if err != nil {
		return adminservice.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetWorkflow200JSONResponse(doc), nil
}

func (s *Server) PutWorkflow(ctx context.Context, request adminservice.PutWorkflowRequestObject) (adminservice.PutWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminservice.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	if request.Body == nil {
		return adminservice.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	doc, env, raw, err := validateDocument(*request.Body, name)
	if err != nil {
		return adminservice.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	if err := s.Store.Set(ctx, workflowKey(env.Metadata.Name), raw); err != nil {
		return adminservice.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutWorkflow200JSONResponse(doc), nil
}

func validateDocument(doc apitypes.WorkflowDocument, expectedName string) (apitypes.WorkflowDocument, documentEnvelope, []byte, error) {
	var env documentEnvelope
	raw, err := json.Marshal(doc)
	if err != nil {
		return apitypes.WorkflowDocument{}, env, nil, err
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return apitypes.WorkflowDocument{}, env, nil, err
	}
	env.Metadata.Name = strings.TrimSpace(env.Metadata.Name)
	if env.Metadata.Name == "" {
		return apitypes.WorkflowDocument{}, env, nil, errors.New("metadata.name is required")
	}
	if env.Spec == nil || bytes.Equal(bytes.TrimSpace(*env.Spec), []byte("null")) {
		return apitypes.WorkflowDocument{}, env, nil, errors.New("spec is required")
	}
	if expectedName != "" && env.Metadata.Name != expectedName {
		return apitypes.WorkflowDocument{}, env, nil, fmt.Errorf("metadata.name %q must match path name %q", env.Metadata.Name, expectedName)
	}
	if strings.TrimSpace(string(doc.Spec.Driver)) == "" {
		return apitypes.WorkflowDocument{}, env, nil, errors.New("spec.driver is required")
	}
	if !doc.Spec.Driver.Valid() {
		return apitypes.WorkflowDocument{}, env, nil, fmt.Errorf("unsupported spec.driver %q", doc.Spec.Driver)
	}
	if err := validateDriverSpec(doc.Spec); err != nil {
		return apitypes.WorkflowDocument{}, env, nil, err
	}

	doc.Metadata.Name = env.Metadata.Name
	raw, err = json.Marshal(doc)
	if err != nil {
		return apitypes.WorkflowDocument{}, env, nil, err
	}
	return doc, env, raw, nil
}

func validateDriverSpec(spec apitypes.WorkflowSpec) error {
	switch spec.Driver {
	case apitypes.WorkflowDriverChatroom:
		if spec.Chatroom == nil {
			return errors.New("spec.chatroom is required")
		}
		return nil
	default:
		return nil
	}
}

func decodeDocument(data []byte) (apitypes.WorkflowDocument, error) {
	var doc apitypes.WorkflowDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return apitypes.WorkflowDocument{}, err
	}
	return doc, nil
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
