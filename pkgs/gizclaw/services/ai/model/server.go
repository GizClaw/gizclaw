package model

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/jmoiron/sqlx"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Server owns the local SQL Model catalog.
type Server struct {
	DB  *sqlx.DB
	Now func() time.Time
}

type ModelAdminService interface {
	CreateModel(context.Context, adminhttp.CreateModelRequestObject) (adminhttp.CreateModelResponseObject, error)
	ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)
	DeleteModel(context.Context, adminhttp.DeleteModelRequestObject) (adminhttp.DeleteModelResponseObject, error)
	GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error)
	PutModel(context.Context, adminhttp.PutModelRequestObject) (adminhttp.PutModelResponseObject, error)
}

var _ ModelAdminService = (*Server)(nil)

func (s *Server) CreateModel(ctx context.Context, request adminhttp.CreateModelRequestObject) (adminhttp.CreateModelResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", "request body required")), nil
	}
	model, err := normalizeModelUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", err.Error())), nil
	}
	now := s.now()
	model.CreatedAt = now
	model.UpdatedAt = now
	created, err := insertModel(ctx, db, model)
	if err != nil {
		return adminhttp.CreateModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateModel409JSONResponse(apitypes.NewErrorResponse("MODEL_ALREADY_EXISTS", fmt.Sprintf("model %q already exists", model.Id))), nil
	}

	return adminhttp.CreateModel200JSONResponse(model), nil
}

func (s *Server) ListModels(ctx context.Context, request adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
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
	if request.Params.ProviderId != nil {
		providerID := string(*request.Params.ProviderId)
		if providerID != "" {
			filters.providerID = &providerID
		}
	}
	items, hasNext, nextCursor, err := listModelsPage(ctx, db, filters, cursor, limit)
	if err != nil {
		return adminhttp.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) DeleteModel(ctx context.Context, request adminhttp.DeleteModelRequestObject) (adminhttp.DeleteModelResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	model, err := scanModel(db.QueryRowContext(ctx, db.Rebind(`DELETE FROM models WHERE id=? RETURNING `+modelColumns), id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteModel404JSONResponse(apitypes.NewErrorResponse("MODEL_NOT_FOUND", fmt.Sprintf("model %q not found", id))), nil
		}
		return adminhttp.DeleteModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.DeleteModel200JSONResponse(model), nil
}

func (s *Server) GetModel(ctx context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	model, err := getModel(ctx, db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetModel404JSONResponse(apitypes.NewErrorResponse("MODEL_NOT_FOUND", fmt.Sprintf("model %q not found", id))), nil
		}
		return adminhttp.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetModel200JSONResponse(model), nil
}

func (s *Server) PutModel(ctx context.Context, request adminhttp.PutModelRequestObject) (adminhttp.PutModelResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", "request body required")), nil
	}
	id := string(request.Id)
	model, err := normalizeModelUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutModel400JSONResponse(apitypes.NewErrorResponse("INVALID_MODEL", err.Error())), nil
	}
	data, err := json.Marshal(model.ProviderData)
	if err != nil {
		return adminhttp.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	model, err = scanModel(db.QueryRowContext(ctx, db.Rebind(`UPDATE models SET kind=?,source=?,provider_kind=?,provider_id=?,provider_data_json=?,display_name=?,description=?,updated_at=? WHERE id=? AND source<>'sync' RETURNING `+modelColumns), string(model.Kind), string(model.Source), string(model.Provider.Kind), model.Provider.Id, string(data), model.DisplayName, model.Description, s.now().Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		current, lookupErr := getModel(ctx, db, id)
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return adminhttp.PutModel404JSONResponse(apitypes.NewErrorResponse("MODEL_NOT_FOUND", fmt.Sprintf("model %q not found", id))), nil
		}
		if lookupErr != nil {
			return adminhttp.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", lookupErr.Error())), nil
		}
		if current.Source == apitypes.ModelSourceSync {
			return adminhttp.PutModel409JSONResponse(apitypes.NewErrorResponse("SYNC_MODEL_READ_ONLY", fmt.Sprintf("model %q has source sync and cannot be modified via API", id))), nil
		}
		return adminhttp.PutModel409JSONResponse(apitypes.NewErrorResponse("MODEL_CONFLICT", "model changed concurrently")), nil
	}
	if err != nil {
		return adminhttp.PutModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.PutModel200JSONResponse(model), nil
}

type modelFilters struct {
	source       *string
	providerKind *string
	providerID   *string
}

func normalizeModelUpsert(in adminhttp.ModelUpsert, expectedID string) (apitypes.Model, error) {
	id := string(in.Id)
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Model{}, err
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
	if !apitypes.ModelProviderKind(providerKind).Valid() {
		return apitypes.Model{}, fmt.Errorf("unsupported provider.kind %q", providerKind)
	}
	providerID := string(in.Provider.Id)
	if err := customid.ValidateResourceID(providerID); err != nil {
		return apitypes.Model{}, fmt.Errorf("provider.id: %w", err)
	}
	model := apitypes.Model{
		Id:   id,
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKind(providerKind),
			Id:   providerID,
		},
		Source: source,
	}
	if in.DisplayName != nil {
		displayName := strings.TrimSpace(*in.DisplayName)
		if displayName != "" {
			model.DisplayName = &displayName
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			model.Description = &description
		}
	}
	providerData, err := cloneModelProviderData(in.ProviderData)
	if err != nil {
		return apitypes.Model{}, fmt.Errorf("clone provider_data: %w", err)
	}
	model.ProviderData = providerData
	if err := validateModelProviderData(model.Kind, model.Provider.Kind, model.ProviderData); err != nil {
		return apitypes.Model{}, err
	}
	return model, nil
}

func validateModelProviderData(modelKind apitypes.ModelKind, providerKind apitypes.ModelProviderKind, data apitypes.ModelProviderData) error {
	var upstream string
	switch providerKind {
	case apitypes.ModelProviderKindOpenaiTenant:
		if modelKind != apitypes.ModelKindLlm && modelKind != apitypes.ModelKindEmbedding {
			return fmt.Errorf("provider %q does not support model kind %q", providerKind, modelKind)
		}
		var value apitypes.OpenAITenantModelProviderData
		if err := decodeStrictModelProviderData(data, &value); err != nil {
			return fmt.Errorf("provider_data for %s: %w", providerKind, err)
		}
		if modelKind == apitypes.ModelKindLlm {
			if err := validateLLMThinking(providerKind, value.SupportThinking, value.ThinkingParam, value.ThinkingLevelParam, value.ThinkingLevels, value.DefaultThinkingLevel); err != nil {
				return err
			}
		}
		upstream = strings.TrimSpace(value.UpstreamModel)
	case apitypes.ModelProviderKindGeminiTenant:
		if modelKind != apitypes.ModelKindLlm {
			return fmt.Errorf("provider %q does not support model kind %q", providerKind, modelKind)
		}
		var value apitypes.GeminiTenantModelProviderData
		if err := decodeStrictModelProviderData(data, &value); err != nil {
			return fmt.Errorf("provider_data for %s: %w", providerKind, err)
		}
		if err := validateLLMThinking(providerKind, value.SupportThinking, value.ThinkingParam, value.ThinkingLevelParam, value.ThinkingLevels, value.DefaultThinkingLevel); err != nil {
			return err
		}
		upstream = strings.TrimSpace(value.UpstreamModel)
	case apitypes.ModelProviderKindDashscopeTenant:
		if modelKind != apitypes.ModelKindLlm && modelKind != apitypes.ModelKindEmbedding && modelKind != apitypes.ModelKindRealtime {
			return fmt.Errorf("provider %q does not support model kind %q", providerKind, modelKind)
		}
		var value apitypes.DashScopeTenantModelProviderData
		if err := decodeStrictModelProviderData(data, &value); err != nil {
			return fmt.Errorf("provider_data for %s: %w", providerKind, err)
		}
		if value.ApiMode != nil && !value.ApiMode.Valid() {
			return fmt.Errorf("provider_data for %s has unsupported api_mode %q", providerKind, *value.ApiMode)
		}
		if modelKind == apitypes.ModelKindLlm && (value.ApiMode == nil || *value.ApiMode != apitypes.DashScopeTenantModelProviderDataApiModeChatCompletions) {
			return fmt.Errorf("provider_data for %s/%s requires api_mode %q", providerKind, modelKind, apitypes.DashScopeTenantModelProviderDataApiModeChatCompletions)
		}
		if modelKind == apitypes.ModelKindRealtime && (value.ApiMode == nil || *value.ApiMode != apitypes.DashScopeTenantModelProviderDataApiModeRealtime) {
			return fmt.Errorf("provider_data for %s/%s requires api_mode %q", providerKind, modelKind, apitypes.DashScopeTenantModelProviderDataApiModeRealtime)
		}
		if modelKind == apitypes.ModelKindEmbedding && value.ApiMode != nil {
			return fmt.Errorf("provider_data for %s/%s does not support api_mode %q", providerKind, modelKind, *value.ApiMode)
		}
		if modelKind == apitypes.ModelKindLlm {
			if err := validateLLMThinking(providerKind, value.SupportThinking, value.ThinkingParam, value.ThinkingLevelParam, value.ThinkingLevels, value.DefaultThinkingLevel); err != nil {
				return err
			}
		}
		upstream = stringValue(value.UpstreamModel)
	case apitypes.ModelProviderKindDeepseekTenant:
		if modelKind != apitypes.ModelKindLlm {
			return fmt.Errorf("provider %q does not support model kind %q", providerKind, modelKind)
		}
		var value apitypes.DeepSeekTenantModelProviderData
		if err := decodeStrictModelProviderData(data, &value); err != nil {
			return fmt.Errorf("provider_data for %s: %w", providerKind, err)
		}
		if !value.ApiMode.Valid() {
			return fmt.Errorf("provider_data for %s has unsupported api_mode %q", providerKind, value.ApiMode)
		}
		if err := validateLLMThinking(providerKind, value.SupportThinking, value.ThinkingParam, value.ThinkingLevelParam, value.ThinkingLevels, value.DefaultThinkingLevel); err != nil {
			return err
		}
		upstream = strings.TrimSpace(value.UpstreamModel)
	case apitypes.ModelProviderKindMinimaxTenant:
		if modelKind != apitypes.ModelKindLlm {
			return fmt.Errorf("provider %q does not support model kind %q", providerKind, modelKind)
		}
		var value apitypes.MiniMaxTenantModelProviderData
		if err := decodeStrictModelProviderData(data, &value); err != nil {
			return fmt.Errorf("provider_data for %s: %w", providerKind, err)
		}
		if !value.ApiMode.Valid() {
			return fmt.Errorf("provider_data for %s has unsupported api_mode %q", providerKind, value.ApiMode)
		}
		if err := validateLLMThinking(providerKind, value.SupportThinking, value.ThinkingParam, value.ThinkingLevelParam, value.ThinkingLevels, value.DefaultThinkingLevel); err != nil {
			return err
		}
		upstream = strings.TrimSpace(value.UpstreamModel)
	case apitypes.ModelProviderKindVolcTenant:
		var value apitypes.VolcTenantModelProviderData
		if err := decodeStrictModelProviderData(data, &value); err != nil {
			return fmt.Errorf("provider_data for %s: %w", providerKind, err)
		}
		if !value.ApiMode.Valid() {
			return fmt.Errorf("provider_data for %s has unsupported api_mode %q", providerKind, value.ApiMode)
		}
		expectedMode, ok := volcAPIModeForModelKind(modelKind)
		if !ok {
			return fmt.Errorf("provider %q does not support model kind %q", providerKind, modelKind)
		}
		if value.ApiMode != expectedMode {
			return fmt.Errorf("provider_data for %s/%s requires api_mode %q", providerKind, modelKind, expectedMode)
		}
		if modelKind == apitypes.ModelKindLlm {
			if err := validateLLMThinking(providerKind, value.SupportThinking, value.ThinkingParam, value.ThinkingLevelParam, value.ThinkingLevels, value.DefaultThinkingLevel); err != nil {
				return err
			}
		}
		upstream = stringValue(value.UpstreamModel)
	default:
		return fmt.Errorf("unsupported provider.kind %q", providerKind)
	}
	if (modelKind == apitypes.ModelKindLlm ||
		modelKind == apitypes.ModelKindEmbedding ||
		modelKind == apitypes.ModelKindRealtime ||
		modelKind == apitypes.ModelKindRealtimeDuplex) && upstream == "" {
		return fmt.Errorf("provider_data for %s/%s requires upstream_model", providerKind, modelKind)
	}
	return nil
}

func validateLLMThinking(
	providerKind apitypes.ModelProviderKind,
	supportThinking *bool,
	thinkingParam, thinkingLevelParam *string,
	thinkingLevels *[]string,
	defaultThinkingLevel *string,
) error {
	if supportThinking == nil || !*supportThinking {
		return nil
	}
	if stringValue(thinkingParam) == "" && stringValue(thinkingLevelParam) == "" {
		return fmt.Errorf("provider_data for %s/llm with support_thinking requires thinking_param or thinking_level_param", providerKind)
	}
	defaultLevel := stringValue(defaultThinkingLevel)
	if stringValue(thinkingLevelParam) == "" && defaultLevel == "" {
		return nil
	}
	if thinkingLevels == nil || len(*thinkingLevels) == 0 {
		return fmt.Errorf("provider_data for %s/llm with support_thinking requires thinking_levels", providerKind)
	}
	if defaultLevel == "" {
		return fmt.Errorf("provider_data for %s/llm with support_thinking requires default_thinking_level", providerKind)
	}
	for _, level := range *thinkingLevels {
		if strings.TrimSpace(level) == defaultLevel {
			return nil
		}
	}
	return fmt.Errorf("provider_data for %s/llm default_thinking_level %q is not in thinking_levels", providerKind, defaultLevel)
}

func volcAPIModeForModelKind(modelKind apitypes.ModelKind) (apitypes.VolcTenantModelProviderDataApiMode, bool) {
	switch modelKind {
	case apitypes.ModelKindLlm:
		return apitypes.VolcTenantModelProviderDataApiModeChatCompletions, true
	case apitypes.ModelKindTts:
		return apitypes.VolcTenantModelProviderDataApiModeTts, true
	case apitypes.ModelKindAsr:
		return apitypes.VolcTenantModelProviderDataApiModeAsr, true
	case apitypes.ModelKindRealtime:
		return apitypes.VolcTenantModelProviderDataApiModeRealtime, true
	case apitypes.ModelKindRealtimeDuplex:
		return apitypes.VolcTenantModelProviderDataApiModeRealtimeDuplex, true
	case apitypes.ModelKindTranslation:
		return apitypes.VolcTenantModelProviderDataApiModeTranslation, true
	case apitypes.ModelKindEmbedding:
		return apitypes.VolcTenantModelProviderDataApiModeEmbedding, true
	default:
		return "", false
	}
}

// ValidateProviderData verifies that provider data is complete for the selected
// model role and decodes strictly as the concrete type selected by providerKind.
func ValidateProviderData(modelKind apitypes.ModelKind, providerKind apitypes.ModelProviderKind, data apitypes.ModelProviderData) error {
	return validateModelProviderData(modelKind, providerKind, data)
}

func decodeStrictModelProviderData(data apitypes.ModelProviderData, out any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

const modelColumns = "id,kind,source,provider_kind,provider_id,provider_data_json,display_name,description,created_at,updated_at,synced_at"

// Initialize creates the Model catalog and query indexes at Server startup.
func (s *Server) Initialize(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{`CREATE TABLE IF NOT EXISTS models(id TEXT PRIMARY KEY CHECK(length(id)>0),kind TEXT NOT NULL,source TEXT NOT NULL,provider_kind TEXT NOT NULL,provider_id TEXT NOT NULL,provider_data_json TEXT NOT NULL,display_name TEXT,description TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,synced_at TEXT)`}
	for _, index := range []struct{ name, columns string }{
		{"models_source_id", "source,id"}, {"models_provider_kind_id", "provider_kind,id"}, {"models_provider_id_id", "provider_id,id"},
		{"models_provider_id", "provider_kind,provider_id,id"}, {"models_source_provider_id", "source,provider_kind,provider_id,id"},
	} {
		statements = append(statements, `CREATE INDEX IF NOT EXISTS `+index.name+` ON models(`+index.columns+`)`)
	}
	for _, query := range statements {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func listModelsPage(ctx context.Context, db *sqlx.DB, filters modelFilters, cursor string, limit int) ([]apitypes.Model, bool, *string, error) {
	var query strings.Builder
	query.WriteString(`SELECT ` + modelColumns + ` FROM models WHERE id>?`)
	args := []any{cursor}
	for _, filter := range []struct {
		column string
		value  *string
	}{{"source", filters.source}, {"provider_kind", filters.providerKind}, {"provider_id", filters.providerID}} {
		if filter.value != nil {
			query.WriteString(` AND ` + filter.column + `=?`)
			args = append(args, *filter.value)
		}
	}
	query.WriteString(` ORDER BY id LIMIT ?`)
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, db.Rebind(query.String()), args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.Model, 0, limit+1)
	for rows.Next() {
		item, err := scanModel(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(items) <= limit {
		return items, false, nil, nil
	}
	items = items[:limit]
	next := items[len(items)-1].Id
	return items, true, &next, nil
}
func insertModel(ctx context.Context, db *sqlx.DB, model apitypes.Model) (bool, error) {
	data, err := json.Marshal(model.ProviderData)
	if err != nil {
		return false, err
	}
	var synced *string
	if model.SyncedAt != nil {
		synced = new(model.SyncedAt.UTC().Format(time.RFC3339Nano))
	}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO models(`+modelColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`), model.Id, string(model.Kind), string(model.Source), string(model.Provider.Kind), model.Provider.Id, string(data), model.DisplayName, model.Description, model.CreatedAt.UTC().Format(time.RFC3339Nano), model.UpdatedAt.UTC().Format(time.RFC3339Nano), synced)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getModel(ctx context.Context, db *sqlx.DB, id string) (apitypes.Model, error) {
	return scanModel(db.QueryRowContext(ctx, db.Rebind(`SELECT `+modelColumns+` FROM models WHERE id=?`), id))
}
func scanModel(row interface{ Scan(...any) error }) (apitypes.Model, error) {
	var model apitypes.Model
	var data, created, updated string
	var synced *string
	if err := row.Scan(&model.Id, &model.Kind, &model.Source, &model.Provider.Kind, &model.Provider.Id, &data, &model.DisplayName, &model.Description, &created, &updated, &synced); err != nil {
		return model, err
	}
	if err := json.Unmarshal([]byte(data), &model.ProviderData); err != nil {
		return model, err
	}
	var err error
	model.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return model, err
	}
	model.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return model, err
	}
	if synced != nil {
		value, err := time.Parse(time.RFC3339Nano, *synced)
		if err != nil {
			return model, err
		}
		model.SyncedAt = &value
	}
	return model, nil
}
func (s *Server) database() (*sqlx.DB, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("models: database not configured")
	}
	return s.DB, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
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

func cloneModelProviderData(in apitypes.ModelProviderData) (apitypes.ModelProviderData, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return apitypes.ModelProviderData{}, err
	}
	var out apitypes.ModelProviderData
	if err := json.Unmarshal(data, &out); err != nil {
		return apitypes.ModelProviderData{}, err
	}
	return out, nil
}
