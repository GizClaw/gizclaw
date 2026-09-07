package voice

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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

// Server owns the local SQL Voice catalog.
type Server struct {
	DB  *sqlx.DB
	Now func() time.Time
}

type VoiceAdminService interface {
	CreateVoice(context.Context, adminhttp.CreateVoiceRequestObject) (adminhttp.CreateVoiceResponseObject, error)
	ListVoices(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error)
	DeleteVoice(context.Context, adminhttp.DeleteVoiceRequestObject) (adminhttp.DeleteVoiceResponseObject, error)
	GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error)
	PutVoice(context.Context, adminhttp.PutVoiceRequestObject) (adminhttp.PutVoiceResponseObject, error)
}

// ProviderVoiceService owns synchronized provider Voice persistence for
// services that discover voices from an external provider.
type ProviderVoiceService interface {
	ReconcileProviderVoices(context.Context, apitypes.VoiceProviderKind, string, []apitypes.Voice) (created, updated, deleted int32, err error)
	ReconcileProviderVoicesInTransaction(context.Context, *sqlx.DB, *sqlx.Tx, apitypes.VoiceProviderKind, string, []apitypes.Voice) (created, updated, deleted int32, err error)
	DeleteProviderVoices(context.Context, apitypes.VoiceProviderKind, string) error
	DeleteProviderVoicesInTransaction(context.Context, *sqlx.DB, *sqlx.Tx, apitypes.VoiceProviderKind, string) error
}

var _ VoiceAdminService = (*Server)(nil)
var _ ProviderVoiceService = (*Server)(nil)

// ReconcileProviderVoices makes synchronized voices for one provider tenant
// match desired while preserving stable IDs and creation timestamps.
func (s *Server) ReconcileProviderVoices(ctx context.Context, kind apitypes.VoiceProviderKind, providerID string, desired []apitypes.Voice) (int32, int32, int32, error) {
	db, err := s.database()
	if err != nil {
		return 0, 0, 0, err
	}
	return reconcileVoiceSQL(ctx, db, kind, providerID, desired)
}

// ReconcileProviderVoicesInTransaction joins a locked tenant incarnation when
// sharing its pool. With separate databases, the caller holds that tenant row
// through reconciliation so retirement cannot race with the independent commit.
func (s *Server) ReconcileProviderVoicesInTransaction(ctx context.Context, ownerDB *sqlx.DB, tx *sqlx.Tx, kind apitypes.VoiceProviderKind, providerID string, desired []apitypes.Voice) (int32, int32, int32, error) {
	db, err := s.database()
	if err != nil {
		return 0, 0, 0, err
	}
	if ownerDB == nil || tx == nil {
		return 0, 0, 0, errors.New("voice: tenant transaction is required")
	}
	if db.DB == ownerDB.DB {
		return reconcileVoiceTx(ctx, tx, kind, providerID, desired)
	}
	return reconcileVoiceSQL(ctx, db, kind, providerID, desired)
}

// DeleteProviderVoices removes synchronized voices owned by one provider
// tenant while leaving manually managed voices untouched.
func (s *Server) DeleteProviderVoices(ctx context.Context, kind apitypes.VoiceProviderKind, providerID string) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	return deleteProviderVoiceSQL(ctx, db, kind, providerID)
}

// DeleteProviderVoicesInTransaction joins tenant retirement when the services
// share a pool, avoiding a second connection and rolling both changes back
// together. With separate databases, the caller must keep the tenant lifecycle
// locked until the independent, retryable voice cleanup commits.
func (s *Server) DeleteProviderVoicesInTransaction(ctx context.Context, ownerDB *sqlx.DB, tx *sqlx.Tx, kind apitypes.VoiceProviderKind, providerID string) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if ownerDB == nil || tx == nil {
		return errors.New("voice: tenant transaction is required")
	}
	if db.DB == ownerDB.DB {
		return deleteProviderVoiceTx(ctx, tx, kind, providerID)
	}
	return deleteProviderVoiceSQL(ctx, db, kind, providerID)
}

type Filters struct {
	Source       *string
	ProviderKind *string
	ProviderId   *string
}

func (s *Server) CreateVoice(ctx context.Context, request adminhttp.CreateVoiceRequestObject) (adminhttp.CreateVoiceResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.CreateVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", "request body required")), nil
	}
	voice, err := normalizeVoiceUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", err.Error())), nil
	}
	now := s.now()
	voice.CreatedAt = now
	voice.UpdatedAt = now
	created, err := insertVoiceSQL(ctx, db, voice)
	if err != nil {
		return adminhttp.CreateVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateVoice409JSONResponse(apitypes.NewErrorResponse("VOICE_ALREADY_EXISTS", fmt.Sprintf("voice %q already exists", voice.Id))), nil
	}

	return adminhttp.CreateVoice200JSONResponse(voice), nil
}

func (s *Server) ListVoices(ctx context.Context, request adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	filters := Filters{}
	if request.Params.Source != nil {
		source := strings.TrimSpace(string(*request.Params.Source))
		if source != "" {
			filters.Source = &source
		}
	}
	if request.Params.ProviderKind != nil {
		kind := strings.TrimSpace(string(*request.Params.ProviderKind))
		if kind != "" {
			filters.ProviderKind = &kind
		}
	}
	if request.Params.ProviderId != nil {
		providerID := string(*request.Params.ProviderId)
		if providerID != "" {
			filters.ProviderId = &providerID
		}
	}
	items, hasNext, nextCursor, err := listVoiceSQL(ctx, db, filters, cursor, limit)
	if err != nil {
		return adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListVoices200JSONResponse(adminhttp.VoiceList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) DeleteVoice(ctx context.Context, request adminhttp.DeleteVoiceRequestObject) (adminhttp.DeleteVoiceResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.DeleteVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	voice, err := deleteVoiceSQL(ctx, db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", fmt.Sprintf("voice %q not found", id))), nil
		}
		return adminhttp.DeleteVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.DeleteVoice200JSONResponse(voice), nil
}

func (s *Server) GetVoice(ctx context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	voice, err := Get(ctx, db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", fmt.Sprintf("voice %q not found", id))), nil
		}
		return adminhttp.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetVoice200JSONResponse(voice), nil
}

func (s *Server) PutVoice(ctx context.Context, request adminhttp.PutVoiceRequestObject) (adminhttp.PutVoiceResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", "request body required")), nil
	}
	id := string(request.Id)
	voice, err := normalizeVoiceUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", err.Error())), nil
	}
	data, err := json.Marshal(voice.ProviderData)
	if err != nil {
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	var providerVoiceID *string
	if value := ProviderDataString(voice, "voice_id"); value != "" {
		providerVoiceID = &value
	}
	voice, err = scanVoice(db.QueryRowContext(ctx, db.Rebind(`UPDATE voices SET source=?,provider_kind=?,provider_id=?,provider_voice_id=?,provider_data_json=?,display_name=?,description=?,updated_at=? WHERE id=? AND source<>'sync' RETURNING `+voiceSQLColumns), string(voice.Source), string(voice.Provider.Kind), voice.Provider.Id, providerVoiceID, string(data), voice.DisplayName, voice.Description, s.now().Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		current, lookupErr := Get(ctx, db, id)
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return adminhttp.PutVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", fmt.Sprintf("voice %q not found", id))), nil
		}
		if lookupErr != nil {
			return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", lookupErr.Error())), nil
		}
		if current.Source == apitypes.VoiceSourceSync {
			return adminhttp.PutVoice409JSONResponse(apitypes.NewErrorResponse("SYNC_VOICE_READ_ONLY", fmt.Sprintf("voice %q has source sync and cannot be modified via API", id))), nil
		}
		return adminhttp.PutVoice409JSONResponse(apitypes.NewErrorResponse("VOICE_CONFLICT", "voice changed concurrently")), nil
	}
	if err != nil {
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.PutVoice200JSONResponse(voice), nil
}

func ProviderData(kind apitypes.VoiceProviderKind, values map[string]any) *apitypes.VoiceProviderData {
	clean := make(map[string]any, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return nil
	}
	raw := rawMapFromProviderData(clean)
	out := apitypes.VoiceProviderData{}
	var err error
	switch kind {
	case apitypes.VoiceProviderKindOpenaiTenant:
		err = out.FromOpenAITenantVoiceProviderData(apitypes.OpenAITenantVoiceProviderData{
			VoiceId: stringPtrFromProviderData(clean, "voice_id"),
			Raw:     raw,
		})
	case apitypes.VoiceProviderKindGeminiTenant:
		err = out.FromGeminiTenantVoiceProviderData(apitypes.GeminiTenantVoiceProviderData{
			VoiceId: stringPtrFromProviderData(clean, "voice_id"),
			Raw:     raw,
		})
	case apitypes.VoiceProviderKindDashscopeTenant:
		err = out.FromDashScopeTenantVoiceProviderData(apitypes.DashScopeTenantVoiceProviderData{
			VoiceId: stringPtrFromProviderData(clean, "voice_id"),
			Raw:     raw,
		})
	case apitypes.VoiceProviderKindMinimaxTenant:
		err = out.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{
			VoiceId:    stringPtrFromProviderData(clean, "voice_id"),
			VoiceType:  stringPtrFromProviderData(clean, "voice_type"),
			Model:      stringPtrFromProviderData(clean, "model"),
			Format:     stringPtrFromProviderData(clean, "format"),
			SampleRate: intPtrFromProviderData(clean, "sample_rate"),
			Raw:        raw,
		})
	case apitypes.VoiceProviderKindVolcTenant:
		err = out.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{
			ResourceId: stringPtrFromProviderData(clean, "resource_id"),
			VoiceId:    stringPtrFromProviderData(clean, "voice_id"),
			State:      stringPtrFromProviderData(clean, "state"),
			Status:     stringPtrFromProviderData(clean, "status"),
			Raw:        raw,
		})
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	return &out
}

func ProviderDataString(voice apitypes.Voice, key string) string {
	if voice.ProviderData == nil {
		return ""
	}
	switch voice.Provider.Kind {
	case apitypes.VoiceProviderKindOpenaiTenant:
		data, err := voice.ProviderData.AsOpenAITenantVoiceProviderData()
		if err != nil {
			return ""
		}
		if key == "voice_id" {
			return stringPtrValue(data.VoiceId)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindGeminiTenant:
		data, err := voice.ProviderData.AsGeminiTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		if key == "voice_id" {
			return stringPtrValue(data.VoiceId)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindDashscopeTenant:
		data, err := voice.ProviderData.AsDashScopeTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		if key == "voice_id" {
			return stringPtrValue(data.VoiceId)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindMinimaxTenant:
		data, err := voice.ProviderData.AsMiniMaxTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		switch key {
		case "voice_id":
			return stringPtrValue(data.VoiceId)
		case "voice_type":
			return stringPtrValue(data.VoiceType)
		case "model":
			return stringPtrValue(data.Model)
		case "format":
			return stringPtrValue(data.Format)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindVolcTenant:
		data, err := voice.ProviderData.AsVolcTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		switch key {
		case "resource_id":
			return stringPtrValue(data.ResourceId)
		case "voice_id":
			return stringPtrValue(data.VoiceId)
		case "state":
			return stringPtrValue(data.State)
		case "status":
			return stringPtrValue(data.Status)
		}
		return rawProviderDataString(data.Raw, key)
	default:
		return ""
	}
}

func RawMapValue(in *map[string]any) any {
	if in == nil {
		return nil
	}
	return *in
}

func StableID(kind apitypes.VoiceProviderKind, providerID string, providerVoiceID string) string {
	hash := sha256.New()
	for _, component := range []string{string(kind), providerID, providerVoiceID} {
		encodedLength := make([]byte, 8)
		binary.BigEndian.PutUint64(encodedLength, uint64(len(component)))
		_, _ = hash.Write(encodedLength)
		_, _ = hash.Write([]byte(component))
	}
	return "voice-sha256-" + hex.EncodeToString(hash.Sum(nil))
}

func SemanticEqual(left, right apitypes.Voice) bool {
	return equalStringPtr(left.Description, right.Description) &&
		equalStringPtr(left.DisplayName, right.DisplayName) &&
		left.Provider.Kind == right.Provider.Kind &&
		left.Provider.Id == right.Provider.Id &&
		left.Source == right.Source &&
		providerDataEqual(left.ProviderData, right.ProviderData)
}

// ListProvider reads records belonging to one Provider from its SQL index.
func ListProvider(ctx context.Context, db *sqlx.DB, kind apitypes.VoiceProviderKind, providerID string) ([]apitypes.Voice, error) {
	rows, err := db.QueryContext(ctx, db.Rebind(`SELECT `+voiceSQLColumns+` FROM voices WHERE provider_kind=? AND provider_id=? ORDER BY id`), string(kind), providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]apitypes.Voice, 0)
	for rows.Next() {
		item, err := scanVoice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Write creates a record, or replaces an existing record when previous is supplied.
func Write(ctx context.Context, db *sqlx.DB, item apitypes.Voice, previous *apitypes.Voice) error {
	if previous == nil {
		created, err := insertVoiceSQL(ctx, db, item)
		if err != nil {
			return err
		}
		if !created {
			return fmt.Errorf("voice: id %q already exists", item.Id)
		}
		return nil
	}
	values, err := voiceSQLValues(item)
	if err != nil {
		return err
	}
	args := append(values[1:], item.Id)
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE voices SET source=?,provider_kind=?,provider_id=?,provider_voice_id=?,provider_data_json=?,display_name=?,description=?,created_at=?,updated_at=?,synced_at=? WHERE id=?`), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes a Voice by its ID.
func Delete(ctx context.Context, db *sqlx.DB, item apitypes.Voice) error {
	_, err := deleteVoiceSQL(ctx, db, item.Id)
	return err
}

// Get looks up one Voice by its exact ID.
func Get(ctx context.Context, db *sqlx.DB, id string) (apitypes.Voice, error) {
	return getVoiceSQL(ctx, db, id)
}
func (s *Server) database() (*sqlx.DB, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("voice: database not configured")
	}
	return s.DB, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeVoiceUpsert(in adminhttp.VoiceUpsert, expectedID string) (apitypes.Voice, error) {
	id := string(in.Id)
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.Voice{}, err
	}
	if expectedID != "" && id != expectedID {
		return apitypes.Voice{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	source := apitypes.VoiceSource(strings.TrimSpace(string(in.Source)))
	if source == "" {
		return apitypes.Voice{}, errors.New("source is required")
	}
	if !source.Valid() {
		return apitypes.Voice{}, fmt.Errorf("unsupported source %q", source)
	}
	if source == apitypes.VoiceSourceSync {
		return apitypes.Voice{}, errors.New("voices with source sync cannot be created or updated via API")
	}
	providerKind := strings.TrimSpace(string(in.Provider.Kind))
	if providerKind == "" {
		return apitypes.Voice{}, errors.New("provider.kind is required")
	}
	providerID := string(in.Provider.Id)
	if err := customid.ValidateResourceID(providerID); err != nil {
		return apitypes.Voice{}, fmt.Errorf("provider.id: %w", err)
	}
	voice := apitypes.Voice{
		Id: id,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKind(providerKind),
			Id:   providerID,
		},
		Source: source,
	}
	if in.DisplayName != nil {
		displayName := strings.TrimSpace(*in.DisplayName)
		if displayName != "" {
			voice.DisplayName = &displayName
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			voice.Description = &description
		}
	}
	if in.ProviderData != nil {
		voice.ProviderData = cloneProviderData(in.ProviderData)
	}
	return voice, nil
}

func providerDataEqual(left, right *apitypes.VoiceProviderData) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	leftJSON, err := left.MarshalJSON()
	if err != nil {
		return false
	}
	rightJSON, err := right.MarshalJSON()
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func providerDataString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func rawProviderDataString(raw *map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	return providerDataString((*raw)[key])
}

func rawMapFromProviderData(values map[string]any) *map[string]any {
	rawValue, ok := values["raw"]
	if !ok || rawValue == nil {
		return nil
	}
	switch typed := rawValue.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		maps.Copy(out, typed)
		if len(out) == 0 {
			return nil
		}
		return &out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		if len(out) == 0 {
			return nil
		}
		return &out
	default:
		return nil
	}
}

func stringPtrFromProviderData(values map[string]any, key string) *string {
	value := providerDataString(values[key])
	if value == "" {
		return nil
	}
	return &value
}

func intPtrFromProviderData(values map[string]any, key string) *int {
	switch typed := values[key].(type) {
	case int:
		return &typed
	case int8:
		value := int(typed)
		return &value
	case int16:
		value := int(typed)
		return &value
	case int32:
		value := int(typed)
		return &value
	case int64:
		value := int(typed)
		return &value
	case uint:
		value := int(typed)
		return &value
	case uint8:
		value := int(typed)
		return &value
	case uint16:
		value := int(typed)
		return &value
	case uint32:
		value := int(typed)
		return &value
	case uint64:
		value := int(typed)
		return &value
	case float64:
		value := int(typed)
		if typed == float64(value) {
			return &value
		}
	case float32:
		value := int(typed)
		if typed == float32(value) {
			return &value
		}
	}
	return nil
}

func stringPtrValue(in *string) string {
	if in == nil {
		return ""
	}
	return strings.TrimSpace(*in)
}

func cloneProviderData(in *apitypes.VoiceProviderData) *apitypes.VoiceProviderData {
	if in == nil {
		return nil
	}
	data, err := in.MarshalJSON()
	if err != nil {
		return nil
	}
	var out apitypes.VoiceProviderData
	if err := out.UnmarshalJSON(data); err != nil {
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

func equalStringPtr(left, right *string) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
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
