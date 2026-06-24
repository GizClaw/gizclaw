package model

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
	modelsRoot           = kv.Key{"by-id"}
	modelsBySourceRoot   = kv.Key{"by-source"}
	modelsByProviderRoot = kv.Key{"by-provider"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store kv.Store
	Now   func() time.Time
}

type ModelAdminService interface {
	CreateModel(context.Context, adminservice.CreateModelRequestObject) (adminservice.CreateModelResponseObject, error)
	ListModels(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error)
	DeleteModel(context.Context, adminservice.DeleteModelRequestObject) (adminservice.DeleteModelResponseObject, error)
	GetModel(context.Context, adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error)
	PutModel(context.Context, adminservice.PutModelRequestObject) (adminservice.PutModelResponseObject, error)
}

var _ ModelAdminService = (*Server)(nil)

func (s *Server) CreateModel(ctx context.Context, request adminservice.CreateModelRequestObject) (adminservice.CreateModelResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.CreateModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", "request body required")), nil
	}
	model, err := normalizeModelUpsert(*request.Body, "")
	if err != nil {
		return adminservice.CreateModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", err.Error())), nil
	}
	if _, err := store.Get(ctx, modelKey(string(model.Id))); err == nil {
		return adminservice.CreateModel409JSONResponse(apitypes.NewErrorResponse("MODEL_ALREADY_EXISTS", fmt.Sprintf("model %q already exists", model.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminservice.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := writeModel(ctx, store, model, nil); err != nil {
		return adminservice.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.CreateModel200JSONResponse(model), nil
}

func (s *Server) ListModels(ctx context.Context, request adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	filters := modelFilters{}
	if request.Params.Source != nil {
		source := strings.TrimSpace(string(*request.Params.Source))
		if source != "" {
			filters.source = &source
		}
	}
	if request.Params.ProviderKind != nil {
		kind := strings.TrimSpace(string(*request.Params.ProviderKind))
		if kind != "" {
			filters.providerKind = &kind
		}
	}
	if request.Params.ProviderName != nil {
		name := strings.TrimSpace(string(*request.Params.ProviderName))
		if name != "" {
			filters.providerName = &name
		}
	}
	items, hasNext, nextCursor, err := listModelsPage(ctx, store, filters, cursor, limit)
	if err != nil {
		return adminservice.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListModels200JSONResponse(adminservice.ModelList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) DeleteModel(ctx context.Context, request adminservice.DeleteModelRequestObject) (adminservice.DeleteModelResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	model, err := getModel(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.DeleteModel404JSONResponse(apitypes.NewErrorResponse("MODEL_NOT_FOUND", fmt.Sprintf("model %q not found", id))), nil
		}
		return adminservice.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := deleteModel(ctx, store, model); err != nil {
		return adminservice.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteModel200JSONResponse(model), nil
}

func (s *Server) GetModel(ctx context.Context, request adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	model, err := getModel(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminservice.GetModel404JSONResponse(apitypes.NewErrorResponse("MODEL_NOT_FOUND", fmt.Sprintf("model %q not found", id))), nil
		}
		return adminservice.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetModel200JSONResponse(model), nil
}

func (s *Server) PutModel(ctx context.Context, request adminservice.PutModelRequestObject) (adminservice.PutModelResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminservice.PutModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", "request body required")), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	model, err := normalizeModelUpsert(*request.Body, id)
	if err != nil {
		return adminservice.PutModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", err.Error())), nil
	}
	previous, err := getModel(ctx, store, id)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminservice.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	model.CreatedAt = now
	model.UpdatedAt = now
	var previousPtr *apitypes.Model
	if err == nil {
		if previous.Source == apitypes.ModelSourceSync {
			return adminservice.PutModel409JSONResponse(apitypes.NewErrorResponse("SYNC_MODEL_READ_ONLY", fmt.Sprintf("model %q has source sync and cannot be modified via API", previous.Id))), nil
		}
		model.CreatedAt = previous.CreatedAt
		model.SyncedAt = cloneTime(previous.SyncedAt)
		previousCopy := previous
		previousPtr = &previousCopy
	}
	if err := writeModel(ctx, store, model, previousPtr); err != nil {
		return adminservice.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.PutModel200JSONResponse(model), nil
}

type modelFilters struct {
	source       *string
	providerKind *string
	providerName *string
}

func normalizeModelUpsert(in adminservice.ModelUpsert, expectedID string) (apitypes.Model, error) {
	id := strings.TrimSpace(string(in.Id))
	if id == "" {
		return apitypes.Model{}, errors.New("id is required")
	}
	if expectedID != "" && id != expectedID {
		return apitypes.Model{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	source := apitypes.ModelSource(strings.TrimSpace(string(in.Source)))
	if source == "" {
		return apitypes.Model{}, errors.New("source is required")
	}
	if !source.Valid() {
		return apitypes.Model{}, fmt.Errorf("unsupported source %q", source)
	}
	if source == apitypes.ModelSourceSync {
		return apitypes.Model{}, errors.New("models with source sync cannot be created or updated via API")
	}
	kind := apitypes.ModelKind(strings.TrimSpace(string(in.Kind)))
	if kind == "" {
		return apitypes.Model{}, errors.New("kind is required")
	}
	if !kind.Valid() {
		return apitypes.Model{}, fmt.Errorf("unsupported kind %q", kind)
	}
	providerKind := strings.TrimSpace(string(in.Provider.Kind))
	if providerKind == "" {
		return apitypes.Model{}, errors.New("provider.kind is required")
	}
	providerName := strings.TrimSpace(string(in.Provider.Name))
	if providerName == "" {
		return apitypes.Model{}, errors.New("provider.name is required")
	}
	model := apitypes.Model{
		Id:   string(id),
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKind(providerKind),
			Name: string(providerName),
		},
		Source: source,
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name != "" {
			model.Name = &name
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			model.Description = &description
		}
	}
	if in.ProviderData != nil {
		model.ProviderData = cloneModelProviderData(in.ProviderData)
	}
	if in.Capabilities != nil {
		model.Capabilities = cloneModelCapabilities(in.Capabilities)
	}
	return model, nil
}

func cloneModelCapabilities(in *apitypes.ModelCapabilities) *apitypes.ModelCapabilities {
	if in == nil {
		return nil
	}
	out := *in
	if in.Thinking != nil {
		thinking := *in.Thinking
		if in.Thinking.Levels != nil {
			levels := append([]string(nil), (*in.Thinking.Levels)...)
			thinking.Levels = &levels
		}
		out.Thinking = &thinking
	}
	return &out
}

func listModelsPage(ctx context.Context, store kv.Store, filters modelFilters, cursor string, limit int) ([]apitypes.Model, bool, *string, error) {
	prefix := modelsRoot
	switch {
	case filters.providerKind != nil && filters.providerName != nil:
		prefix = modelByProviderPrefix(*filters.providerKind, *filters.providerName)
	case filters.source != nil:
		prefix = modelBySourcePrefix(*filters.source)
	}
	items := make([]apitypes.Model, 0, limit+1)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, false, nil, err
		}
		if len(entry.Key) == 0 {
			continue
		}
		lastSegment := entry.Key[len(entry.Key)-1]
		if cursor != "" && lastSegment <= cursor {
			continue
		}
		var model apitypes.Model
		if prefix.String() == modelsRoot.String() {
			if err := json.Unmarshal(entry.Value, &model); err != nil {
				return nil, false, nil, fmt.Errorf("models: decode model list %s: %w", entry.Key.String(), err)
			}
		} else {
			decodedID := unescapeStoreSegment(lastSegment)
			model, err = getModel(ctx, store, decodedID)
			if err != nil {
				if errors.Is(err, kv.ErrNotFound) {
					continue
				}
				return nil, false, nil, err
			}
		}
		if !matchesModelFilters(model, filters) {
			continue
		}
		items = append(items, model)
		if len(items) >= limit+1 {
			break
		}
	}
	if len(items) == 0 {
		return []apitypes.Model{}, false, nil, nil
	}
	hasNext := len(items) > limit
	if !hasNext {
		return items, false, nil, nil
	}
	page := items[:limit]
	next := escapeStoreSegment(string(page[len(page)-1].Id))
	return page, true, &next, nil
}

func matchesModelFilters(model apitypes.Model, filters modelFilters) bool {
	if filters.source != nil && string(model.Source) != *filters.source {
		return false
	}
	if filters.providerKind != nil && string(model.Provider.Kind) != *filters.providerKind {
		return false
	}
	if filters.providerName != nil && string(model.Provider.Name) != *filters.providerName {
		return false
	}
	return true
}

func writeModel(ctx context.Context, store kv.Store, model apitypes.Model, previous *apitypes.Model) error {
	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("models: encode model %s: %w", model.Id, err)
	}
	var deletes []kv.Key
	if previous != nil {
		deletes = staleModelIndexKeys(*previous, model)
	}
	if len(deletes) > 0 {
		if err := store.BatchDelete(ctx, deletes); err != nil {
			return fmt.Errorf("models: delete stale model indexes %s: %w", model.Id, err)
		}
	}
	entries := []kv.Entry{
		{Key: modelKey(string(model.Id)), Value: data},
		{Key: modelBySourceKey(string(model.Source), string(model.Id)), Value: []byte{}},
		{Key: modelByProviderKey(string(model.Provider.Kind), string(model.Provider.Name), string(model.Id)), Value: []byte{}},
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("models: write model %s: %w", model.Id, err)
	}
	return nil
}

func staleModelIndexKeys(previous, next apitypes.Model) []kv.Key {
	var keys []kv.Key
	if previous.Source != next.Source {
		keys = append(keys, modelBySourceKey(string(previous.Source), string(previous.Id)))
	}
	if previous.Provider.Kind != next.Provider.Kind || previous.Provider.Name != next.Provider.Name {
		keys = append(keys, modelByProviderKey(string(previous.Provider.Kind), string(previous.Provider.Name), string(previous.Id)))
	}
	return keys
}

func deleteModel(ctx context.Context, store kv.Store, model apitypes.Model) error {
	keys := []kv.Key{
		modelKey(string(model.Id)),
		modelBySourceKey(string(model.Source), string(model.Id)),
		modelByProviderKey(string(model.Provider.Kind), string(model.Provider.Name), string(model.Id)),
	}
	if err := store.BatchDelete(ctx, keys); err != nil {
		return fmt.Errorf("models: delete model %s: %w", model.Id, err)
	}
	return nil
}

func getModel(ctx context.Context, store kv.Store, id string) (apitypes.Model, error) {
	data, err := store.Get(ctx, modelKey(id))
	if err != nil {
		return apitypes.Model{}, err
	}
	var model apitypes.Model
	if err := json.Unmarshal(data, &model); err != nil {
		return apitypes.Model{}, fmt.Errorf("models: decode model %s: %w", id, err)
	}
	return model, nil
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("models: nil store")
	}
	return s.Store, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func modelKey(id string) kv.Key {
	return append(append(kv.Key{}, modelsRoot...), escapeStoreSegment(id))
}

func modelBySourcePrefix(source string) kv.Key {
	return append(append(kv.Key{}, modelsBySourceRoot...), escapeStoreSegment(source))
}

func modelBySourceKey(source, id string) kv.Key {
	return append(modelBySourcePrefix(source), escapeStoreSegment(id))
}

func modelByProviderPrefix(kind, name string) kv.Key {
	prefix := append(append(kv.Key{}, modelsByProviderRoot...), escapeStoreSegment(kind))
	return append(prefix, escapeStoreSegment(name))
}

func modelByProviderKey(kind, name, id string) kv.Key {
	return append(modelByProviderPrefix(kind, name), escapeStoreSegment(id))
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	normalizedLimit := defaultListLimit
	if limit != nil && int(*limit) > 0 {
		normalizedLimit = int(*limit)
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	normalizedCursor := ""
	if cursor != nil {
		normalizedCursor = strings.TrimSpace(string(*cursor))
	}
	return normalizedCursor, normalizedLimit
}

func escapeStoreSegment(value string) string {
	return url.PathEscape(value)
}

func unescapeStoreSegment(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func cloneModelProviderData(in *apitypes.ModelProviderData) *apitypes.ModelProviderData {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out apitypes.ModelProviderData
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return &out
}

func cloneTime(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
