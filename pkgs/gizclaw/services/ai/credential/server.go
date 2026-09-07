package credential

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Server owns the local SQL Credential catalog.
type Server struct {
	DB  *sqlx.DB
	Now func() time.Time
}

type CredentialAdminService interface {
	ListCredentials(context.Context, adminhttp.ListCredentialsRequestObject) (adminhttp.ListCredentialsResponseObject, error)
	CreateCredential(context.Context, adminhttp.CreateCredentialRequestObject) (adminhttp.CreateCredentialResponseObject, error)
	DeleteCredential(context.Context, adminhttp.DeleteCredentialRequestObject) (adminhttp.DeleteCredentialResponseObject, error)
	GetCredential(context.Context, adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error)
	PutCredential(context.Context, adminhttp.PutCredentialRequestObject) (adminhttp.PutCredentialResponseObject, error)
}

var _ CredentialAdminService = (*Server)(nil)

type credentialRecord struct {
	Revision    int64
	Incarnation string
	Body        apitypes.CredentialBody `json:"body"`
	CreatedAt   time.Time               `json:"created_at"`
	Description *string                 `json:"description,omitempty"`
	ID          string                  `json:"id"`
	Provider    string                  `json:"provider"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type normalizedCredentialUpsert struct {
	Body        apitypes.CredentialBody
	Description *string
	ID          string
	Provider    string
}

func (s *Server) ListCredentials(ctx context.Context, request adminhttp.ListCredentialsRequestObject) (adminhttp.ListCredentialsResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.ListCredentials500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	provider := ""
	if request.Params.Provider != nil {
		provider = strings.TrimSpace(string(*request.Params.Provider))
	}
	items, hasNext, nextCursor, err := listCredentialsPage(ctx, db, provider, cursor, limit)

	if err != nil {
		return adminhttp.ListCredentials500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListCredentials200JSONResponse(adminhttp.CredentialList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateCredential(ctx context.Context, request adminhttp.CreateCredentialRequestObject) (adminhttp.CreateCredentialResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.CreateCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "request body required")), nil
	}
	upsert, err := normalizeCredentialUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
	}
	if isZeroCredentialBody(upsert.Body) {
		return adminhttp.CreateCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "body is required")), nil
	}
	if err := validateCredentialBody(upsert.Provider, upsert.Body); err != nil {
		return adminhttp.CreateCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
	}
	now := s.now()
	record := credentialRecord{
		Body:        cloneBody(upsert.Body),
		CreatedAt:   now,
		Description: cloneString(upsert.Description),
		ID:          upsert.ID,
		Provider:    upsert.Provider,
		UpdatedAt:   now,
	}
	created, err := insertCredential(ctx, db, record)
	if err != nil {
		return adminhttp.CreateCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateCredential409JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_ALREADY_EXISTS", fmt.Sprintf("credential %q already exists", record.ID))), nil
	}

	return adminhttp.CreateCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) DeleteCredential(ctx context.Context, request adminhttp.DeleteCredentialRequestObject) (adminhttp.DeleteCredentialResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	record, err := scanCredential(db.QueryRowContext(ctx, db.Rebind(`DELETE FROM credentials WHERE id=? RETURNING `+credentialColumns), id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.DeleteCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", id))), nil
		}
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}

	return adminhttp.DeleteCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) GetCredential(ctx context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id := string(request.Id)
	record, err := getCredentialRecord(ctx, db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.GetCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", id))), nil
		}
		return adminhttp.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) PutCredential(ctx context.Context, request adminhttp.PutCredentialRequestObject) (adminhttp.PutCredentialResponseObject, error) {
	db, err := s.database()
	if err != nil {
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "request body required")), nil
	}
	id := string(request.Id)
	upsert, err := normalizeCredentialUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
	}
	incarnation := ""
	for range 16 {
		previous, err := getCredentialRecord(ctx, db, id)
		if errors.Is(err, sql.ErrNoRows) {
			return adminhttp.PutCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", id))), nil
		}
		if err != nil {
			return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
		if incarnation == "" {
			incarnation = previous.Incarnation
		} else if incarnation != previous.Incarnation {
			return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_CONFLICT", "credential was recreated")), nil
		}
		now := s.now()
		record := credentialRecord{
			Body:        cloneBody(upsert.Body),
			CreatedAt:   now,
			Description: cloneString(upsert.Description),
			ID:          id,
			Provider:    upsert.Provider,
			UpdatedAt:   now,
		}
		record.CreatedAt = previous.CreatedAt
		if isZeroCredentialBody(record.Body) {
			record.Body = cloneBody(previous.Body)
		}
		if isZeroCredentialBody(record.Body) {
			return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "body is required")), nil
		}
		if err := validateCredentialBody(record.Provider, record.Body); err != nil {
			return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
		}
		changed, err := updateCredential(ctx, db, record, previous)
		if err != nil {
			return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
		if changed {
			return adminhttp.PutCredential200JSONResponse(credentialFromRecord(record)), nil
		}
	}
	return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_CONFLICT", "credential changed concurrently")), nil

}

func credentialFromRecord(record credentialRecord) apitypes.Credential {
	return apitypes.Credential{
		Body:        cloneBody(record.Body),
		CreatedAt:   record.CreatedAt,
		Description: cloneString(record.Description),
		Id:          record.ID,
		Provider:    record.Provider,
		UpdatedAt:   record.UpdatedAt,
	}
}

const credentialColumns = "id,provider,body_json,description,created_at,updated_at,revision,incarnation"

// Initialize creates the Credential catalog and Provider index at startup.
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
	for _, query := range []string{`CREATE TABLE IF NOT EXISTS credentials(id TEXT PRIMARY KEY CHECK(length(id)>0),provider TEXT NOT NULL,body_json TEXT NOT NULL,description TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,revision BIGINT NOT NULL CHECK(revision>0),incarnation TEXT NOT NULL)`, `CREATE INDEX IF NOT EXISTS credentials_provider_id ON credentials(provider,id)`} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func insertCredential(ctx context.Context, db *sqlx.DB, record credentialRecord) (bool, error) {
	body, err := json.Marshal(record.Body)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO credentials(`+credentialColumns+`) VALUES (?,?,?,?,?,?,1,?) ON CONFLICT(id) DO NOTHING`), record.ID, record.Provider, string(body), record.Description, record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano), uuid.NewString())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func updateCredential(ctx context.Context, db *sqlx.DB, record, previous credentialRecord) (bool, error) {
	body, err := json.Marshal(record.Body)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE credentials SET provider=?,body_json=?,description=?,updated_at=?,revision=revision+1 WHERE id=? AND revision=? AND incarnation=?`), record.Provider, string(body), record.Description, record.UpdatedAt.Format(time.RFC3339Nano), record.ID, previous.Revision, previous.Incarnation)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
func getCredentialRecord(ctx context.Context, db *sqlx.DB, id string) (credentialRecord, error) {
	return scanCredential(db.QueryRowContext(ctx, db.Rebind(`SELECT `+credentialColumns+` FROM credentials WHERE id=?`), id))
}
func scanCredential(row interface{ Scan(...any) error }) (credentialRecord, error) {
	var record credentialRecord
	var body, created, updated string
	if err := row.Scan(&record.ID, &record.Provider, &body, &record.Description, &created, &updated, &record.Revision, &record.Incarnation); err != nil {
		return record, err
	}
	if err := json.Unmarshal([]byte(body), &record.Body); err != nil {
		return record, err
	}
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return record, err
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return record, err
}
func listCredentialsPage(ctx context.Context, db *sqlx.DB, provider, cursor string, limit int) ([]apitypes.Credential, bool, *string, error) {
	query := `SELECT ` + credentialColumns + ` FROM credentials WHERE id>?`
	args := []any{cursor}
	if provider != "" {
		query += ` AND provider=?`
		args = append(args, provider)
	}
	query += ` ORDER BY id LIMIT ?`
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, db.Rebind(query), args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	items := make([]apitypes.Credential, 0, limit+1)
	for rows.Next() {
		record, err := scanCredential(rows)
		if err != nil {
			return nil, false, nil, err
		}
		items = append(items, credentialFromRecord(record))
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

func normalizeCredentialUpsert(in adminhttp.CredentialUpsert, expectedID string) (normalizedCredentialUpsert, error) {
	id := string(in.Id)
	if err := customid.ValidateResourceID(id); err != nil {
		return normalizedCredentialUpsert{}, err
	}
	if expectedID != "" && id != expectedID {
		return normalizedCredentialUpsert{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	provider := strings.TrimSpace(string(in.Provider))
	if provider == "" {
		return normalizedCredentialUpsert{}, errors.New("provider is required")
	}
	out := normalizedCredentialUpsert{
		Body:     cloneBody(in.Body),
		ID:       id,
		Provider: string(provider),
	}
	if in.Description != nil {
		text := strings.TrimSpace(*in.Description)
		if text != "" {
			out.Description = &text
		}
	}
	return out, nil
}

func validateCredentialBody(provider string, body apitypes.CredentialBody) error {
	if isZeroCredentialBody(body) {
		return errors.New("body is required")
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		var typed apitypes.OpenAICredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if allEmpty(typed.ApiKey, typed.Token, typed.BaseUrl, typed.Organization, typed.Project) {
			return errors.New("body must include at least one non-empty credential field")
		}
		return nil
	case "gemini":
		var typed apitypes.GeminiCredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if allEmpty(typed.ApiKey, typed.Token, typed.BaseUrl) {
			return errors.New("body must include at least one non-empty credential field")
		}
		return nil
	case "dashscope":
		var typed apitypes.DashScopeCredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if allEmpty(typed.ApiKey, typed.Token, typed.BaseUrl) {
			return errors.New("body must include at least one non-empty credential field")
		}
		return nil
	case "deepseek":
		var typed apitypes.DeepSeekCredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if strings.TrimSpace(typed.ApiKey) == "" {
			return errors.New("body.api_key is required")
		}
		return nil
	case "minimax":
		var typed apitypes.MiniMaxCredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if allEmpty(typed.ApiKey, typed.Token, typed.BaseUrl, typed.VoiceBaseUrl, typed.MinimaxVoiceBaseUrl) {
			return errors.New("body must include at least one non-empty credential field")
		}
		return nil
	case "volc", "volcengine":
		var typed apitypes.VolcCredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if allEmpty(typed.SpeechAppId, typed.SpeechApiKey, typed.ArkApiKey, typed.SearchApiKey, typed.OpenapiAccessKeyId, typed.OpenapiAccessKey, typed.OpenapiSessionToken) {
			return errors.New("body must include at least one non-empty credential field")
		}
		return nil
	case "aliyun":
		var typed apitypes.AliyunCredentialBody
		if err := decodeCredentialBody(body, &typed); err != nil {
			return err
		}
		if allEmpty(typed.AppCode, typed.AccessKeyId, typed.AccessKeySecret, typed.SecurityToken) {
			return errors.New("body must include at least one non-empty credential field")
		}
		return nil
	default:
		return fmt.Errorf("unsupported credential provider %q", provider)
	}
}

func decodeCredentialBody(body apitypes.CredentialBody, out any) error {
	data, err := body.MarshalJSON()
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func allEmpty(values ...*string) bool {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return false
		}
	}
	return true
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

func cloneBody(in apitypes.CredentialBody) apitypes.CredentialBody {
	var out apitypes.CredentialBody
	data, err := in.MarshalJSON()
	if err != nil {
		return out
	}
	_ = out.UnmarshalJSON(data)
	return out
}

func isZeroCredentialBody(body apitypes.CredentialBody) bool {
	data, err := body.MarshalJSON()
	if err != nil {
		return true
	}
	data = bytes.TrimSpace(data)
	return len(data) == 0 || bytes.Equal(data, []byte("null"))
}

func cloneString(in *string) *string {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (s *Server) database() (*sqlx.DB, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("credential database not configured")
	}
	return s.DB, nil
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
