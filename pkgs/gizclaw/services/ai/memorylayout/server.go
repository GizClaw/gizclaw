// Package memorylayout owns the persisted, connection-free MemoryLayout
// resources consumed by RuntimeProfile memory bindings.
package memorylayout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
	"github.com/jmoiron/sqlx"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Server owns SQL MemoryLayout declarations; it does not store Memory content.
type Server struct{ DB *sqlx.DB }

type MemoryLayoutAdminService interface {
	ListMemoryLayouts(context.Context, adminhttp.ListMemoryLayoutsRequestObject) (adminhttp.ListMemoryLayoutsResponseObject, error)
	CreateMemoryLayout(context.Context, adminhttp.CreateMemoryLayoutRequestObject) (adminhttp.CreateMemoryLayoutResponseObject, error)
	DeleteMemoryLayout(context.Context, adminhttp.DeleteMemoryLayoutRequestObject) (adminhttp.DeleteMemoryLayoutResponseObject, error)
	GetMemoryLayout(context.Context, adminhttp.GetMemoryLayoutRequestObject) (adminhttp.GetMemoryLayoutResponseObject, error)
	PutMemoryLayout(context.Context, adminhttp.PutMemoryLayoutRequestObject) (adminhttp.PutMemoryLayoutResponseObject, error)
}

var _ MemoryLayoutAdminService = (*Server)(nil)

func (s *Server) ListMemoryLayouts(ctx context.Context, request adminhttp.ListMemoryLayoutsRequestObject) (adminhttp.ListMemoryLayoutsResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.ListMemoryLayouts500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout database not configured")), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, err := s.listPage(ctx, cursor, limit+1)
	if err != nil {
		return adminhttp.ListMemoryLayouts500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	hasNext := len(items) > limit
	var nextCursor *string
	if hasNext {
		items = items[:limit]
		nextCursor = &items[len(items)-1].Id
	}
	return adminhttp.ListMemoryLayouts200JSONResponse(adminhttp.MemoryLayoutList{
		HasNext: hasNext, Items: items, NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateMemoryLayout(ctx context.Context, request adminhttp.CreateMemoryLayoutRequestObject) (adminhttp.CreateMemoryLayoutResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout database not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", "request body required")), nil
	}
	item, _, err := validate(upsertToLayout(*request.Body), "")
	if err != nil {
		return adminhttp.CreateMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", err.Error())), nil
	}
	values, err := layoutPolicyValues(item.Spec)
	if err != nil {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	args := append([]any{item.Id}, values...)
	result, err := s.DB.ExecContext(ctx, s.DB.Rebind(`INSERT INTO memory_layouts(id,flowcraft_json,mem0_json,volc_mem0_json) VALUES (?,?,?,?) ON CONFLICT(id) DO NOTHING`), args...)
	if err != nil {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	count, err := result.RowsAffected()
	if err != nil {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	created := count == 1
	if !created {
		return adminhttp.CreateMemoryLayout409JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_ALREADY_EXISTS", fmt.Sprintf("memory layout %q already exists", item.Id))), nil
	}
	return adminhttp.CreateMemoryLayout200JSONResponse(item), nil
}

func (s *Server) GetMemoryLayout(ctx context.Context, request adminhttp.GetMemoryLayoutRequestObject) (adminhttp.GetMemoryLayoutResponseObject, error) {
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	if s == nil || s.DB == nil {
		return adminhttp.GetMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout database not configured")), nil
	}
	item, err := scanLayout(s.DB.QueryRowContext(ctx, s.DB.Rebind(`SELECT `+layoutColumns+` FROM memory_layouts WHERE id=?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.GetMemoryLayout404JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_NOT_FOUND", fmt.Sprintf("memory layout %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.GetMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetMemoryLayout200JSONResponse(item), nil
}

func (s *Server) PutMemoryLayout(ctx context.Context, request adminhttp.PutMemoryLayoutRequestObject) (adminhttp.PutMemoryLayoutResponseObject, error) {
	if s == nil || s.DB == nil {
		return adminhttp.PutMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout database not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", "request body required")), nil
	}
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	item, _, err := validate(upsertToLayout(*request.Body), id)
	if err != nil {
		return adminhttp.PutMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", err.Error())), nil
	}
	values, err := layoutPolicyValues(item.Spec)
	if err != nil {
		return adminhttp.PutMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	result, err := s.DB.ExecContext(ctx, s.DB.Rebind(`UPDATE memory_layouts SET flowcraft_json=?,mem0_json=?,volc_mem0_json=? WHERE id=?`), append(values, id)...)
	if err != nil {
		return adminhttp.PutMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	count, err := result.RowsAffected()
	if err != nil {
		return adminhttp.PutMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if count == 0 {
		return adminhttp.PutMemoryLayout404JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_NOT_FOUND", fmt.Sprintf("memory layout %q not found", id))), nil
	}
	return adminhttp.PutMemoryLayout200JSONResponse(item), nil
}

func (s *Server) DeleteMemoryLayout(ctx context.Context, request adminhttp.DeleteMemoryLayoutRequestObject) (adminhttp.DeleteMemoryLayoutResponseObject, error) {
	id, err := pathID(request.Id)
	if err != nil {
		return nil, err
	}
	if s == nil || s.DB == nil {
		return adminhttp.DeleteMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout database not configured")), nil
	}
	item, err := scanLayout(s.DB.QueryRowContext(ctx, s.DB.Rebind(`DELETE FROM memory_layouts WHERE id=? RETURNING `+layoutColumns), id))
	if errors.Is(err, sql.ErrNoRows) {
		return adminhttp.DeleteMemoryLayout404JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_NOT_FOUND", fmt.Sprintf("memory layout %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.DeleteMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteMemoryLayout200JSONResponse(item), nil
}

func validate(item apitypes.MemoryLayout, expectedID string) (apitypes.MemoryLayout, []byte, error) {
	// Generated resource values contain pointer, slice, and map fields. Clone
	// before normalization so validation never mutates a caller-owned request.
	cloned, err := json.Marshal(item)
	if err != nil {
		return apitypes.MemoryLayout{}, nil, err
	}
	var normalized apitypes.MemoryLayout
	if err := json.Unmarshal(cloned, &normalized); err != nil {
		return apitypes.MemoryLayout{}, nil, err
	}
	item = normalized
	if err := customid.ValidateResourceID(item.Id); err != nil {
		return apitypes.MemoryLayout{}, nil, err
	}
	if expectedID != "" && item.Id != expectedID {
		return apitypes.MemoryLayout{}, nil, fmt.Errorf("id %q must match path id %q", item.Id, expectedID)
	}
	item.Spec.Flowcraft.Extraction.Model = strings.TrimSpace(item.Spec.Flowcraft.Extraction.Model)
	if item.Spec.Flowcraft.Extraction.Model == "" {
		return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.extraction.model is required")
	}
	if err := runtimealias.Validate("spec.flowcraft.extraction.model", item.Spec.Flowcraft.Extraction.Model); err != nil {
		return apitypes.MemoryLayout{}, nil, err
	}
	if !item.Spec.Flowcraft.Extraction.Mode.Valid() {
		return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.extraction.mode %q is invalid", item.Spec.Flowcraft.Extraction.Mode)
	}
	if timeout := item.Spec.Flowcraft.Extraction.StageTimeout; timeout != nil {
		normalized := strings.TrimSpace(*timeout)
		if duration, err := time.ParseDuration(normalized); err != nil || duration <= 0 {
			return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.extraction.stage_timeout must be a positive duration")
		}
		item.Spec.Flowcraft.Extraction.StageTimeout = &normalized
	}
	for path, model := range map[string]*apitypes.FlowcraftMemoryModelPolicy{
		"spec.flowcraft.embedding": item.Spec.Flowcraft.Embedding,
		"spec.flowcraft.rerank":    item.Spec.Flowcraft.Rerank,
	} {
		if model != nil {
			model.Model = strings.TrimSpace(model.Model)
			if err := runtimealias.Validate(path+".model", model.Model); err != nil {
				return apitypes.MemoryLayout{}, nil, err
			}
		}
	}
	if bbh := item.Spec.Flowcraft.Bbh; bbh != nil {
		if bbh.SearchOverfetch != nil && *bbh.SearchOverfetch < 1 {
			return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.bbh.search_overfetch must be at least 1")
		}
		if bleve := bbh.Bleve; bleve != nil {
			if bleve.Analyzer != nil && !bleve.Analyzer.Valid() {
				return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.bbh.bleve.analyzer %q is invalid", *bleve.Analyzer)
			}
			if bleve.Gojieba != nil && bleve.Gojieba.Mode != nil && !bleve.Gojieba.Mode.Valid() {
				return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.bbh.bleve.gojieba.mode %q is invalid", *bleve.Gojieba.Mode)
			}
		}
		if hnsw := bbh.Hnsw; hnsw != nil && hnsw.FlushInterval != nil {
			normalized := strings.TrimSpace(*hnsw.FlushInterval)
			if duration, err := time.ParseDuration(normalized); err != nil || duration <= 0 {
				return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.bbh.hnsw.flush_interval must be a positive duration")
			}
			hnsw.FlushInterval = &normalized
		}
	}
	if !item.Spec.Flowcraft.Write.Mode.Valid() {
		return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.write.mode %q is invalid", item.Spec.Flowcraft.Write.Mode)
	}
	if !item.Spec.Flowcraft.Write.Tier.Valid() {
		return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.write.tier %q is invalid", item.Spec.Flowcraft.Write.Tier)
	}
	if len(item.Spec.Flowcraft.Lanes) == 0 {
		return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.lanes must not be empty")
	}
	laneNames := make(map[string]struct{}, len(item.Spec.Flowcraft.Lanes))
	for index, lane := range item.Spec.Flowcraft.Lanes {
		lane.Name = strings.TrimSpace(lane.Name)
		if lane.Name == "" || len(lane.Name) > 63 {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.lanes[%d].name must be 1-63 characters", index)
		}
		if !lane.Kind.Valid() {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.lanes[%d].kind %q is invalid", index, lane.Kind)
		}
		if _, duplicate := laneNames[lane.Name]; duplicate {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.lanes contains duplicate name %q", lane.Name)
		}
		laneNames[lane.Name] = struct{}{}
		item.Spec.Flowcraft.Lanes[index] = lane
	}
	if item.Spec.Mem0.CustomInstructions == nil &&
		item.Spec.Mem0.CustomCategories == nil &&
		item.Spec.Mem0.Multilingual == nil &&
		item.Spec.Mem0.Decay == nil {
		return apitypes.MemoryLayout{}, nil, errors.New("spec.mem0 must define at least one policy")
	}
	if item.Spec.Mem0.CustomInstructions != nil {
		normalized := strings.TrimSpace(*item.Spec.Mem0.CustomInstructions)
		if normalized == "" {
			return apitypes.MemoryLayout{}, nil, errors.New("spec.mem0.custom_instructions must not be empty")
		}
		item.Spec.Mem0.CustomInstructions = &normalized
	}
	if item.Spec.Mem0.CustomCategories != nil {
		normalized := make(map[string]string, len(*item.Spec.Mem0.CustomCategories))
		for name, description := range *item.Spec.Mem0.CustomCategories {
			name = strings.TrimSpace(name)
			description = strings.TrimSpace(description)
			if name == "" || description == "" {
				return apitypes.MemoryLayout{}, nil, errors.New("spec.mem0.custom_categories names and descriptions must not be empty")
			}
			if _, duplicate := normalized[name]; duplicate {
				return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.mem0.custom_categories contains duplicate name %q", name)
			}
			normalized[name] = description
		}
		item.Spec.Mem0.CustomCategories = &normalized
	}
	if len(item.Spec.VolcMem0.Strategies) < 1 || len(item.Spec.VolcMem0.Strategies) > 50 {
		return apitypes.MemoryLayout{}, nil, errors.New("spec.volc_mem0.strategies must contain between 1 and 50 entries")
	}
	strategyNames := make(map[string]struct{}, len(item.Spec.VolcMem0.Strategies))
	for index, strategy := range item.Spec.VolcMem0.Strategies {
		strategy.Name = strings.TrimSpace(strategy.Name)
		if strategy.Name == "" || len(strategy.Name) > 63 {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.volc_mem0.strategies[%d].name must be 1-63 characters", index)
		}
		if !strategy.Type.Valid() {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.volc_mem0.strategies[%d].type %q is invalid", index, strategy.Type)
		}
		if _, duplicate := strategyNames[strategy.Name]; duplicate {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.volc_mem0.strategies contains duplicate name %q", strategy.Name)
		}
		strategyNames[strategy.Name] = struct{}{}
		if strategy.CustomInstructions != nil {
			normalized := strings.TrimSpace(*strategy.CustomInstructions)
			if utf8.RuneCountInString(normalized) > 2000 {
				return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.volc_mem0.strategies[%d].custom_instructions exceeds 2000 characters", index)
			}
			strategy.CustomInstructions = &normalized
		}
		item.Spec.VolcMem0.Strategies[index] = strategy
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return apitypes.MemoryLayout{}, nil, err
	}
	return item, raw, nil
}

// NormalizeSpec applies the same validation and canonicalization as the
// MemoryLayout service before callers compare desired and stored specs.
func NormalizeSpec(id string, spec apitypes.MemoryLayoutSpec) (apitypes.MemoryLayoutSpec, error) {
	item, _, err := validate(apitypes.MemoryLayout{Id: id, Spec: spec}, id)
	if err != nil {
		return apitypes.MemoryLayoutSpec{}, err
	}
	return item.Spec, nil
}

func upsertToLayout(in adminhttp.MemoryLayoutUpsert) apitypes.MemoryLayout {
	return apitypes.MemoryLayout{Id: in.Id, Spec: in.Spec}
}

func pathID(value string) (string, error) {
	if err := customid.ValidateResourceID(value); err != nil {
		return "", fmt.Errorf("invalid path id: %w", err)
	}
	return value, nil
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	cursorValue := ""
	if cursor != nil {
		cursorValue = *cursor
	}
	limitValue := defaultListLimit
	if limit != nil {
		limitValue = int(*limit)
	}
	if limitValue <= 0 {
		limitValue = defaultListLimit
	}
	if limitValue > maxListLimit {
		limitValue = maxListLimit
	}
	return cursorValue, limitValue
}

const layoutColumns = "id,flowcraft_json,mem0_json,volc_mem0_json"

// Initialize creates the layout schema at Server startup using the shared pool.
func (s *Server) Initialize(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return errors.New("memory layout database not configured")
	}
	switch s.DB.DriverName() {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("memorylayout: unsupported SQL driver %q", s.DB.DriverName())
	}
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS memory_layouts (
 id TEXT PRIMARY KEY CHECK(length(id)>0),
 flowcraft_json TEXT NOT NULL,
 mem0_json TEXT NOT NULL,
 volc_mem0_json TEXT NOT NULL
 )`)
	return err
}

func layoutPolicyValues(spec apitypes.MemoryLayoutSpec) ([]any, error) {
	values := make([]any, 0, 3)
	for _, policy := range []any{spec.Flowcraft, spec.Mem0, spec.VolcMem0} {
		data, err := json.Marshal(policy)
		if err != nil {
			return nil, err
		}
		values = append(values, string(data))
	}
	return values, nil
}

func scanLayout(row interface{ Scan(...any) error }) (apitypes.MemoryLayout, error) {
	var item apitypes.MemoryLayout
	var flowcraft, mem0, volc string
	if err := row.Scan(&item.Id, &flowcraft, &mem0, &volc); err != nil {
		return item, err
	}
	for _, policy := range []struct {
		raw    string
		target any
	}{{flowcraft, &item.Spec.Flowcraft}, {mem0, &item.Spec.Mem0}, {volc, &item.Spec.VolcMem0}} {
		if err := json.Unmarshal([]byte(policy.raw), policy.target); err != nil {
			return item, err
		}
	}
	_, _, err := validate(item, "")
	return item, err
}

func (s *Server) listPage(ctx context.Context, cursor string, limit int) ([]apitypes.MemoryLayout, error) {
	rows, err := s.DB.QueryContext(ctx, s.DB.Rebind(`SELECT `+layoutColumns+` FROM memory_layouts WHERE id>? ORDER BY id LIMIT ?`), cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]apitypes.MemoryLayout, 0, limit)
	for rows.Next() {
		item, err := scanLayout(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
