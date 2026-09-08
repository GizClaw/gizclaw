package workflow

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/jmoiron/sqlx"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Server owns the local SQL Workflow catalog.
type Server struct {
	DB *sqlx.DB
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

// Initialize creates the Workflow table once during Server startup.
func (s *Server) Initialize(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return errors.New("workflow: database not configured")
	}
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY CHECK(length(id)>0),driver TEXT NOT NULL,config_json TEXT NOT NULL)`)
	return err
}

// EnsureBuiltinWorkflows idempotently materializes every built-in system
// Workflow in the local catalog. Each Server calls it at startup so the same
// Workflow identity exists on every Server that may activate a Social
// Workspace.
func (s *Server) EnsureBuiltinWorkflows(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return errors.New("workflow: database not configured")
	}
	doc, _, err := validateWorkflow(BuiltinSFUWorkflow(), "")
	if err != nil {
		return err
	}
	config, err := workflowConfig(doc)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, s.DB.Rebind(`INSERT INTO workflows(id,driver,config_json) VALUES (?,?,?) ON CONFLICT(id) DO UPDATE SET driver=excluded.driver,config_json=excluded.config_json WHERE workflows.driver<>excluded.driver OR workflows.config_json<>excluded.config_json`), doc.Id, string(doc.Spec.Driver), config)
	return err
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
	if s == nil || s.DB == nil {
		return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow database not configured")), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	rows, err := s.DB.QueryContext(ctx, s.DB.Rebind(`SELECT id,driver,config_json FROM workflows WHERE id>? AND id<>? ORDER BY id LIMIT ?`), cursor, socialutil.SFUWorkflowID, limit+1)
	if err != nil {
		return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	defer rows.Close()
	items := make([]apitypes.Workflow, 0, limit+1)
	for rows.Next() {
		doc, err := scanWorkflow(rows)
		if err != nil {
			return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
		items = append(items, doc)
	}
	if err := rows.Err(); err != nil {
		return adminhttp.ListWorkflows500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	hasNext := len(items) > limit
	var nextCursor *string
	if hasNext {
		items = items[:limit]
		nextCursor = new(items[len(items)-1].Id)
	}

	return adminhttp.ListWorkflows200JSONResponse(adminhttp.WorkflowList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateWorkflow(ctx context.Context, request adminhttp.CreateWorkflowRequestObject) (adminhttp.CreateWorkflowResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow database not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "request body required")), nil
	}
	body := *request.Body
	if IsBuiltinWorkflowID(body.Id) {
		return adminhttp.CreateWorkflow409JSONResponse(apitypes.NewErrorResponse(BuiltinWorkflowCode, builtinWorkflowMessage(body.Id))), nil
	}
	doc, _, err := validateWorkflow(apitypes.Workflow{Id: body.Id, Spec: body.Spec}, "")
	if err != nil {
		return adminhttp.CreateWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	config, err := workflowConfig(doc)
	if err != nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	result, err := s.DB.ExecContext(ctx, s.DB.Rebind(`INSERT INTO workflows(id,driver,config_json) VALUES (?,?,?) ON CONFLICT(id) DO NOTHING`), doc.Id, string(doc.Spec.Driver), config)
	if err != nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	count, err := result.RowsAffected()
	if err != nil {
		return adminhttp.CreateWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if count == 0 {
		return adminhttp.CreateWorkflow409JSONResponse(apitypes.NewErrorResponse("WORKFLOW_ALREADY_EXISTS", fmt.Sprintf("workflow %q already exists", doc.Id))), nil
	}

	return adminhttp.CreateWorkflow200JSONResponse(doc), nil
}

func (s *Server) DeleteWorkflow(ctx context.Context, request adminhttp.DeleteWorkflowRequestObject) (adminhttp.DeleteWorkflowResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow database not configured")), nil
	}
	id := string(request.Id)
	if IsBuiltinWorkflowID(id) {
		return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse(BuiltinWorkflowCode, builtinWorkflowMessage(id))), nil
	}
	doc, err := scanWorkflow(s.DB.QueryRowContext(ctx, s.DB.Rebind(`DELETE FROM workflows WHERE id=? RETURNING id,driver,config_json`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.DeleteWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.DeleteWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.DeleteWorkflow200JSONResponse(doc), nil
}

func (s *Server) GetWorkflow(ctx context.Context, request adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow database not configured")), nil
	}
	id := string(request.Id)
	doc, err := scanWorkflow(s.DB.QueryRowContext(ctx, s.DB.Rebind(`SELECT id,driver,config_json FROM workflows WHERE id=?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.GetWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.GetWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.GetWorkflow200JSONResponse(doc), nil
}

func (s *Server) PutWorkflow(ctx context.Context, request adminhttp.PutWorkflowRequestObject) (adminhttp.PutWorkflowResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "workflow database not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", "request body required")), nil
	}
	id := string(request.Id)
	if IsBuiltinWorkflowID(id) || (request.Body != nil && IsBuiltinWorkflowID(request.Body.Id)) {
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse(BuiltinWorkflowCode, builtinWorkflowMessage(id))), nil
	}
	body := *request.Body
	doc, _, err := validateWorkflow(apitypes.Workflow{Id: body.Id, Spec: body.Spec}, id)
	if err != nil {
		var exists bool
		if lookupErr := s.DB.QueryRowContext(ctx, s.DB.Rebind(`SELECT EXISTS(SELECT 1 FROM workflows WHERE id=?)`), id).Scan(&exists); lookupErr != nil {
			return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", lookupErr.Error())), nil
		}
		if !exists {
			return adminhttp.PutWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
		}
		return adminhttp.PutWorkflow400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKFLOW", err.Error())), nil
	}
	config, err := workflowConfig(doc)
	if err != nil {
		return adminhttp.PutWorkflow500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	doc, err = scanWorkflow(s.DB.QueryRowContext(ctx, s.DB.Rebind(`UPDATE workflows SET driver=?,config_json=? WHERE id=? RETURNING id,driver,config_json`), string(doc.Spec.Driver), config, id))
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.PutWorkflow404JSONResponse(apitypes.NewErrorResponse("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow %q not found", id))), nil
	}
	if err != nil {
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

func validateDriverPayloads(driver apitypes.WorkflowDriver, flowcraft, doubaoRealtime, dashscopeRealtime, doubaoRealtimeDuplex, eino, astTranslate, sfu bool) error {
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

func workflowConfig(doc apitypes.Workflow) (string, error) {
	raw, err := json.Marshal(doc.Spec)
	if err != nil {
		return "", err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", err
	}
	delete(config, "driver")
	raw, err = json.Marshal(config)
	return string(raw), err
}
func scanWorkflow(row interface{ Scan(...any) error }) (apitypes.Workflow, error) {
	var doc apitypes.Workflow
	var driver, config string
	if err := row.Scan(&doc.Id, &driver, &config); err != nil {
		return doc, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(config), &fields); err != nil {
		return doc, err
	}
	if fields == nil {
		return doc, errors.New("workflow: null configuration")
	}
	encodedDriver, err := json.Marshal(driver)
	if err != nil {
		return doc, err
	}
	fields["driver"] = encodedDriver
	raw, err := json.Marshal(fields)
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(raw, &doc.Spec); err != nil {
		return doc, err
	}
	doc, _, err = validateWorkflow(doc, "")
	return doc, err
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
