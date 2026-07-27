// Package memorylayout owns the persisted, connection-free MemoryLayout
// resources consumed by RuntimeProfile memory bindings.
package memorylayout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	layoutsRoot         = kv.Key{"by-name"}
	runtimeAliasPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store      kv.Store
	mutationMu sync.Mutex
}

type MemoryLayoutAdminService interface {
	ListMemoryLayouts(context.Context, adminhttp.ListMemoryLayoutsRequestObject) (adminhttp.ListMemoryLayoutsResponseObject, error)
	CreateMemoryLayout(context.Context, adminhttp.CreateMemoryLayoutRequestObject) (adminhttp.CreateMemoryLayoutResponseObject, error)
	DeleteMemoryLayout(context.Context, adminhttp.DeleteMemoryLayoutRequestObject) (adminhttp.DeleteMemoryLayoutResponseObject, error)
	GetMemoryLayout(context.Context, adminhttp.GetMemoryLayoutRequestObject) (adminhttp.GetMemoryLayoutResponseObject, error)
	PutMemoryLayout(context.Context, adminhttp.PutMemoryLayoutRequestObject) (adminhttp.PutMemoryLayoutResponseObject, error)
}

var _ MemoryLayoutAdminService = (*Server)(nil)

func (s *Server) ListMemoryLayouts(ctx context.Context, request adminhttp.ListMemoryLayoutsRequestObject) (adminhttp.ListMemoryLayoutsResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.ListMemoryLayouts500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout store not configured")), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	entries, err := kv.ListAfter(ctx, s.Store, layoutsRoot, cursorAfterKey(cursor), limit+1)
	if err != nil {
		return adminhttp.ListMemoryLayouts500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	hasNext := len(entries) > limit
	if hasNext {
		entries = entries[:limit]
	}
	items := make([]apitypes.MemoryLayout, 0, len(entries))
	for _, entry := range entries {
		item, err := decode(entry.Value)
		if err != nil {
			return adminhttp.ListMemoryLayouts500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
		items = append(items, item)
	}
	var nextCursor *string
	if hasNext && len(entries) > 0 {
		value := entries[len(entries)-1].Key[len(entries[len(entries)-1].Key)-1]
		nextCursor = &value
	}
	return adminhttp.ListMemoryLayouts200JSONResponse(adminhttp.MemoryLayoutList{
		HasNext: hasNext, Items: items, NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateMemoryLayout(ctx context.Context, request adminhttp.CreateMemoryLayoutRequestObject) (adminhttp.CreateMemoryLayoutResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout store not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", "request body required")), nil
	}
	item, raw, err := validate(*request.Body, "")
	if err != nil {
		return adminhttp.CreateMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", err.Error())), nil
	}
	key := layoutKey(item.Name)
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := s.Store.Get(ctx, key); err == nil {
		return adminhttp.CreateMemoryLayout409JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_ALREADY_EXISTS", fmt.Sprintf("memory layout %q already exists", item.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.Store.Set(ctx, key, raw); err != nil {
		return adminhttp.CreateMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateMemoryLayout200JSONResponse(item), nil
}

func (s *Server) GetMemoryLayout(ctx context.Context, request adminhttp.GetMemoryLayoutRequestObject) (adminhttp.GetMemoryLayoutResponseObject, error) {
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Store == nil {
		return adminhttp.GetMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout store not configured")), nil
	}
	raw, err := s.Store.Get(ctx, layoutKey(name))
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.GetMemoryLayout404JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_NOT_FOUND", fmt.Sprintf("memory layout %q not found", name))), nil
	}
	if err != nil {
		return adminhttp.GetMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err := decode(raw)
	if err != nil {
		return adminhttp.GetMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetMemoryLayout200JSONResponse(item), nil
}

func (s *Server) PutMemoryLayout(ctx context.Context, request adminhttp.PutMemoryLayoutRequestObject) (adminhttp.PutMemoryLayoutResponseObject, error) {
	if s == nil || s.Store == nil {
		return adminhttp.PutMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout store not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", "request body required")), nil
	}
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	item, raw, err := validate(*request.Body, name)
	if err != nil {
		return adminhttp.PutMemoryLayout400JSONResponse(apitypes.NewErrorResponse("INVALID_MEMORY_LAYOUT", err.Error())), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.Store.Set(ctx, layoutKey(name), raw); err != nil {
		return adminhttp.PutMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutMemoryLayout200JSONResponse(item), nil
}

func (s *Server) DeleteMemoryLayout(ctx context.Context, request adminhttp.DeleteMemoryLayoutRequestObject) (adminhttp.DeleteMemoryLayoutResponseObject, error) {
	name, err := pathName(request.Name)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Store == nil {
		return adminhttp.DeleteMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "memory layout store not configured")), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	raw, err := s.Store.Get(ctx, layoutKey(name))
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.DeleteMemoryLayout404JSONResponse(apitypes.NewErrorResponse("MEMORY_LAYOUT_NOT_FOUND", fmt.Sprintf("memory layout %q not found", name))), nil
	}
	if err != nil {
		return adminhttp.DeleteMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	item, err := decode(raw)
	if err != nil {
		return adminhttp.DeleteMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := s.Store.Delete(ctx, layoutKey(name)); err != nil {
		return adminhttp.DeleteMemoryLayout500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteMemoryLayout200JSONResponse(item), nil
}

func validate(item apitypes.MemoryLayout, expectedName string) (apitypes.MemoryLayout, []byte, error) {
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
	if err := customid.ValidateField("name", item.Name); err != nil {
		return apitypes.MemoryLayout{}, nil, err
	}
	if expectedName != "" && item.Name != expectedName {
		return apitypes.MemoryLayout{}, nil, fmt.Errorf("name %q must match path name %q", item.Name, expectedName)
	}
	item.Spec.Flowcraft.Extraction.Model = strings.TrimSpace(item.Spec.Flowcraft.Extraction.Model)
	if item.Spec.Flowcraft.Extraction.Model == "" {
		return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.extraction.model is required")
	}
	if err := validateRuntimeAlias("spec.flowcraft.extraction.model", item.Spec.Flowcraft.Extraction.Model); err != nil {
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
			if err := validateRuntimeAlias(path+".model", model.Model); err != nil {
				return apitypes.MemoryLayout{}, nil, err
			}
		}
	}
	if item.Spec.Flowcraft.Bbh.SearchOverfetch != nil && *item.Spec.Flowcraft.Bbh.SearchOverfetch < 1 {
		return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.bbh.search_overfetch must be at least 1")
	}
	if bleve := item.Spec.Flowcraft.Bbh.Bleve; bleve != nil {
		if bleve.Analyzer != nil && !bleve.Analyzer.Valid() {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.bbh.bleve.analyzer %q is invalid", *bleve.Analyzer)
		}
		if bleve.Gojieba != nil && bleve.Gojieba.Mode != nil && !bleve.Gojieba.Mode.Valid() {
			return apitypes.MemoryLayout{}, nil, fmt.Errorf("spec.flowcraft.bbh.bleve.gojieba.mode %q is invalid", *bleve.Gojieba.Mode)
		}
	}
	if hnsw := item.Spec.Flowcraft.Bbh.Hnsw; hnsw != nil && hnsw.FlushInterval != nil {
		normalized := strings.TrimSpace(*hnsw.FlushInterval)
		if duration, err := time.ParseDuration(normalized); err != nil || duration <= 0 {
			return apitypes.MemoryLayout{}, nil, errors.New("spec.flowcraft.bbh.hnsw.flush_interval must be a positive duration")
		}
		hnsw.FlushInterval = &normalized
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

func validateRuntimeAlias(path, value string) error {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 63 || !runtimeAliasPattern.MatchString(value) {
		return fmt.Errorf("%s %q must be 1-63 characters of lowercase kebab-case", path, value)
	}
	return nil
}

func decode(raw []byte) (apitypes.MemoryLayout, error) {
	var item apitypes.MemoryLayout
	if err := json.Unmarshal(raw, &item); err != nil {
		return apitypes.MemoryLayout{}, err
	}
	_, _, err := validate(item, "")
	return item, err
}

func layoutKey(name string) kv.Key {
	return append(append(kv.Key{}, layoutsRoot...), name)
}

func pathName(value string) (string, error) {
	name, err := url.PathUnescape(value)
	if err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}
	if err := customid.ValidateField("path name", name); err != nil {
		return "", err
	}
	return name, nil
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

func cursorAfterKey(cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	return append(append(kv.Key{}, layoutsRoot...), cursor)
}
