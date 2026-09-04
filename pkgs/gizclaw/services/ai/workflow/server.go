package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var workflowsRoot = kv.Key{"by-id"}

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store kv.Store
}

// BuiltinWorkflowError is returned by the Admin surface when a request targets
// a built-in system Workflow that only the Server itself may materialize.
const BuiltinWorkflowCode = "WORKFLOW_BUILTIN"

// IsBuiltinWorkflowID reports whether id names a Server-materialized system
// Workflow. Built-in Workflows never appear in Workflow lists and reject
// Admin create, update, and delete.
func IsBuiltinWorkflowID(id string) bool {
	return strings.TrimSpace(id) == socialutil.SFUWorkflowID
}

// BuiltinSFUWorkflow returns the canonical Workflow document for the
// Server-materialized SFU system Workflow used by Friend and Friend Group
// Workspaces. The payload is intentionally empty.
func BuiltinSFUWorkflow() apitypes.Workflow {
	sfu := apitypes.SFUWorkflowSpec{}
	return apitypes.Workflow{
		Id:   socialutil.SFUWorkflowID,
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu, Sfu: &sfu},
	}
}

// EnsureBuiltinWorkflows idempotently materializes every built-in system
// Workflow in the local catalog. Each Server calls it at startup so the same
// Workflow identity exists on every Server that may activate a Social
// Workspace.
func (s *Server) EnsureBuiltinWorkflows(ctx context.Context) error {
	if s == nil || s.Store == nil {
		return errors.New("workflow: store not configured")
	}
	doc, raw, err := validateWorkflow(BuiltinSFUWorkflow(), "")
	if err != nil {
		return fmt.Errorf("workflow: built-in %q: %w", socialutil.SFUWorkflowID, err)
	}
	if existing, err := s.Store.Get(ctx, workflowKey(doc.Id)); err == nil && bytes.Equal(existing, raw) {
		return nil
	} else if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return fmt.Errorf("workflow: read built-in %q: %w", doc.Id, err)
	}
	if err := s.Store.Set(ctx, workflowKey(doc.Id), raw); err != nil {
		return fmt.Errorf("workflow: write built-in %q: %w", doc.Id, err)
	}
	return nil
}

func builtinWorkflowMessage(id string) string {
	return fmt.Sprintf("workflow %q is a built-in system Workflow", strings.TrimSpace(id))
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
	Spec *json.RawMessage `json:"spec"`
}

func (s *Server) ListWorkflows(ctx context.Context, request adminhttp.ListWorkflowsRequestObject) (adminhttp.ListWorkflowsResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	entries, err := listVisibleWorkflows(ctx, s.Store, cursor, limit+1)
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
	body := *request.Body
	if IsBuiltinWorkflowID(body.Id) {
		return adminhttp.CreateWorkflow409JSONResponse(apitypes.NewErrorResponse(BuiltinWorkflowCode, builtinWorkflowMessage(body.Id))), nil
	}
	doc, raw, err := validateWorkflow(apitypes.Workflow{Id: body.Id, Spec: body.Spec}, "")
	if err != nil {
		return adminhttp.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	_, created, err := kv.CreateIfAbsent(ctx, s.Store, kv.Entry{Key: workflowKey(doc.Id), Value: raw}, nil)
	if err != nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateWorkflow409JSONResponse(apitypes.NewErrorResponse("WORKFLOW_ALREADY_EXISTS", fmt.Sprintf("workflow %q already exists", doc.Id))), nil
	}
	return adminhttp.CreateWorkflow200JSONResponse(doc), nil
}

func (s *Server) DeleteWorkflow(ctx context.Context, request adminhttp.DeleteWorkflowRequestObject) (adminhttp.DeleteWorkflowResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow store not configured")), nil
	}
	id := string(request.Id)
	if IsBuiltinWorkflowID(id) {
		return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse(BuiltinWorkflowCode, builtinWorkflowMessage(id))), nil
	}
	key := workflowKey(id)
	data, err := s.Store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
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
	id := string(request.Id)
	data, err := s.Store.Get(ctx, workflowKey(id))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
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
	id := string(request.Id)
	if IsBuiltinWorkflowID(id) || (request.Body != nil && IsBuiltinWorkflowID(request.Body.Id)) {
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse(BuiltinWorkflowCode, builtinWorkflowMessage(id))), nil
	}
	previousData, getErr := s.Store.Get(ctx, workflowKey(id))
	if getErr == nil {
		if _, err := decodeWorkflow(previousData); err != nil {
			return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	} else if errors.Is(getErr, kv.ErrNotFound) {
		return adminhttp.PutWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
	} else {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", getErr.Error())), nil
	}
	body := *request.Body
	doc, raw, err := validateWorkflow(apitypes.Workflow{Id: body.Id, Spec: body.Spec}, id)
	if err != nil {
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	if err := s.Store.Set(ctx, workflowKey(id), raw); err != nil {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutWorkflow200JSONResponse(doc), nil
}

func validateWorkflow(item apitypes.Workflow, expectedID string) (apitypes.Workflow, []byte, error) {
	var env workflowEnvelope
	raw, err := json.Marshal(item)
	if err != nil {
		return apitypes.Workflow{}, nil, err
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return apitypes.Workflow{}, nil, err
	}
	if err := customid.ValidateResourceID(item.Id); err != nil {
		return apitypes.Workflow{}, nil, err
	}
	if env.Spec == nil || bytes.Equal(bytes.TrimSpace(*env.Spec), []byte("null")) {
		return apitypes.Workflow{}, nil, errors.New("spec is required")
	}
	if expectedID != "" && item.Id != expectedID {
		return apitypes.Workflow{}, nil, fmt.Errorf("id %q must match path id %q", item.Id, expectedID)
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
	if item.Spec.Pet != nil {
		nestedPolicy, err := toolkit.NormalizePolicy(item.Spec.Pet.Toolkit)
		if err != nil {
			return apitypes.Workflow{}, nil, fmt.Errorf("spec.pet.toolkit: %w", err)
		}
		item.Spec.Pet.Toolkit = nestedPolicy
	}

	item.Spec.Toolkit = policy
	raw, err = json.Marshal(item)
	if err != nil {
		return apitypes.Workflow{}, nil, err
	}
	return item, raw, nil
}

func validateDriverSpec(spec apitypes.WorkflowSpec) error {
	if err := validateDriverPayloads(
		spec.Driver,
		spec.Flowcraft != nil,
		spec.DoubaoRealtime != nil,
		spec.DashscopeRealtime != nil,
		spec.DoubaoRealtimeDuplex != nil,
		spec.Eino != nil,
		spec.AstTranslate != nil,
		spec.Sfu != nil,
		spec.Pet != nil,
	); err != nil {
		return err
	}
	switch spec.Driver {
	case apitypes.WorkflowDriverFlowcraft:
		if err := spec.Flowcraft.Validate(); err != nil {
			return fmt.Errorf("spec.flowcraft: %w", err)
		}
		return nil
	case apitypes.WorkflowDriverSfu:
		if len(*spec.Sfu) != 0 {
			return errors.New("spec.sfu must be an empty object")
		}
		return nil
	case apitypes.WorkflowDriverPet:
		return validateNestedPetWorkflow(*spec.Pet)
	case apitypes.WorkflowDriverDoubaoRealtime:
		if strings.TrimSpace(spec.DoubaoRealtime.Model) == "" {
			return errors.New("spec.doubao_realtime.model is required")
		}
		if spec.DoubaoRealtime.Tools != nil && len(*spec.DoubaoRealtime.Tools) != 0 {
			return errors.New("spec.doubao_realtime.tools are unsupported until ToolCall is implemented")
		}
		return nil
	case apitypes.WorkflowDriverDashscopeRealtime:
		if err := spec.DashscopeRealtime.Validate(); err != nil {
			return fmt.Errorf("spec.dashscope_realtime: %w", err)
		}
		return nil
	case apitypes.WorkflowDriverDoubaoRealtimeDuplex:
		if err := spec.DoubaoRealtimeDuplex.Validate(); err != nil {
			return fmt.Errorf("spec.doubao_realtime_duplex: %w", err)
		}
		return nil
	case apitypes.WorkflowDriverEino:
		if err := einoconfig.Validate(*spec.Eino); err != nil {
			return fmt.Errorf("spec.eino: %w", err)
		}
		return nil
	case apitypes.WorkflowDriverAstTranslate:
		return nil
	default:
		return fmt.Errorf("unsupported spec.driver %q", spec.Driver)
	}
}

func validateNestedPetWorkflow(spec apitypes.PetWorkflowSpec) error {
	if apitypes.WorkflowDriver(spec.Driver) == apitypes.WorkflowDriverSfu {
		return errors.New("spec.pet.driver \"sfu\" cannot be nested in a Pet Workflow")
	}
	if !spec.Driver.Valid() {
		return fmt.Errorf("spec.pet.driver %q is not a reusable Workflow driver", spec.Driver)
	}
	nested := apitypes.WorkflowSpec{
		Driver:               apitypes.WorkflowDriver(spec.Driver),
		Toolkit:              spec.Toolkit,
		Flowcraft:            spec.Flowcraft,
		DoubaoRealtime:       spec.DoubaoRealtime,
		DashscopeRealtime:    spec.DashscopeRealtime,
		DoubaoRealtimeDuplex: spec.DoubaoRealtimeDuplex,
		Eino:                 spec.Eino,
		AstTranslate:         spec.AstTranslate,
	}
	if err := validateDriverSpec(nested); err != nil {
		return fmt.Errorf("spec.pet: %w", err)
	}
	return nil
}

func validateDriverPayloads(driver apitypes.WorkflowDriver, flowcraft, doubaoRealtime, dashscopeRealtime, doubaoRealtimeDuplex, eino, astTranslate, sfu, pet bool) error {
	payloads := []struct {
		driver  apitypes.WorkflowDriver
		field   string
		present bool
	}{
		{apitypes.WorkflowDriverFlowcraft, "flowcraft", flowcraft},
		{apitypes.WorkflowDriverDoubaoRealtime, "doubao_realtime", doubaoRealtime},
		{apitypes.WorkflowDriverDashscopeRealtime, "dashscope_realtime", dashscopeRealtime},
		{apitypes.WorkflowDriverDoubaoRealtimeDuplex, "doubao_realtime_duplex", doubaoRealtimeDuplex},
		{apitypes.WorkflowDriverEino, "eino", eino},
		{apitypes.WorkflowDriverAstTranslate, "ast_translate", astTranslate},
		{apitypes.WorkflowDriverSfu, "sfu", sfu},
		{apitypes.WorkflowDriverPet, "pet", pet},
	}
	for _, payload := range payloads {
		if payload.driver == driver {
			if !payload.present {
				return fmt.Errorf("spec.%s is required", payload.field)
			}
			continue
		}
		if payload.present {
			return fmt.Errorf("spec.%s does not match driver %q", payload.field, driver)
		}
	}
	return nil
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

// listVisibleWorkflows reads up to limit Admin-visible Workflows after cursor.
// Built-in system Workflows are skipped, so the scan continues until limit
// visible entries are collected or the catalog ends.
func listVisibleWorkflows(ctx context.Context, store kv.Store, cursor string, limit int) ([]kv.Entry, error) {
	out := make([]kv.Entry, 0, limit)
	after := cursorAfterKey(workflowsRoot, cursor)
	for len(out) < limit {
		want := limit - len(out)
		entries, err := kv.ListAfter(ctx, store, workflowsRoot, after, want)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if len(entry.Key) > 0 && IsBuiltinWorkflowID(unescapeStoreSegment(entry.Key[len(entry.Key)-1])) {
				continue
			}
			out = append(out, entry)
		}
		if len(entries) < want {
			break
		}
		after = entries[len(entries)-1].Key
	}
	return out, nil
}

func workflowKey(id string) kv.Key {
	return append(append(kv.Key{}, workflowsRoot...), escapeStoreSegment(id))
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
