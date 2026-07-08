package credential

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	credentialsRoot           = kv.Key{"by-name"}
	credentialsByProviderRoot = kv.Key{"by-provider"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store kv.Store
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
	Body        apitypes.CredentialBody `json:"body"`
	CreatedAt   time.Time               `json:"created_at"`
	Description *string                 `json:"description,omitempty"`
	Name        string                  `json:"name"`
	Provider    string                  `json:"provider"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type normalizedCredentialUpsert struct {
	Body        apitypes.CredentialBody
	Description *string
	Name        string
	Provider    string
}

func (s *Server) ListCredentials(ctx context.Context, request adminhttp.ListCredentialsRequestObject) (adminhttp.ListCredentialsResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListCredentials500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	provider := ""
	if request.Params.Provider != nil {
		provider = strings.TrimSpace(string(*request.Params.Provider))
	}
	var (
		items      []apitypes.Credential
		hasNext    bool
		nextCursor *string
	)
	if provider == "" {
		items, hasNext, nextCursor, err = listCredentialRecordsPage(ctx, store, credentialsRoot, cursor, limit)
	} else {
		items, hasNext, nextCursor, err = listCredentialsByProviderPage(ctx, store, provider, cursor, limit)
	}
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
	store, err := s.store()
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
	if _, err := store.Get(ctx, credentialKey(string(upsert.Name))); err == nil {
		return adminhttp.CreateCredential409JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_ALREADY_EXISTS", fmt.Sprintf("credential %q already exists", upsert.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := time.Now().UTC()
	record := credentialRecord{
		Body:        cloneBody(upsert.Body),
		CreatedAt:   now,
		Description: cloneString(upsert.Description),
		Name:        upsert.Name,
		Provider:    upsert.Provider,
		UpdatedAt:   now,
	}
	if err := writeCredential(ctx, store, record, nil); err != nil {
		return adminhttp.CreateCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) DeleteCredential(ctx context.Context, request adminhttp.DeleteCredentialRequestObject) (adminhttp.DeleteCredentialResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	record, err := getCredentialRecord(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", name))), nil
		}
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.BatchDelete(ctx, []kv.Key{
		credentialKey(string(record.Name)),
		credentialByProviderKey(string(record.Provider), string(record.Name)),
	}); err != nil {
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) GetCredential(ctx context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	record, err := getCredentialRecord(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", name))), nil
		}
		return adminhttp.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) PutCredential(ctx context.Context, request adminhttp.PutCredentialRequestObject) (adminhttp.PutCredentialResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	upsert, err := normalizeCredentialUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
	}
	previous, err := getCredentialRecord(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := time.Now().UTC()
	record := credentialRecord{
		Body:        cloneBody(upsert.Body),
		CreatedAt:   now,
		Description: cloneString(upsert.Description),
		Name:        upsert.Name,
		Provider:    upsert.Provider,
		UpdatedAt:   now,
	}
	var previousPtr *credentialRecord
	if err == nil {
		record.CreatedAt = previous.CreatedAt
		if isZeroCredentialBody(record.Body) {
			record.Body = cloneBody(previous.Body)
		}
		previousCopy := previous
		previousPtr = &previousCopy
	}
	if isZeroCredentialBody(record.Body) {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", "body is required")), nil
	}
	if err := validateCredentialBody(record.Provider, record.Body); err != nil {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
	}
	if err := writeCredential(ctx, store, record, previousPtr); err != nil {
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutCredential200JSONResponse(credentialFromRecord(record)), nil
}

func credentialFromRecord(record credentialRecord) apitypes.Credential {
	return apitypes.Credential{
		Body:        cloneBody(record.Body),
		CreatedAt:   record.CreatedAt,
		Description: cloneString(record.Description),
		Name:        record.Name,
		Provider:    record.Provider,
		UpdatedAt:   record.UpdatedAt,
	}
}

func writeCredential(ctx context.Context, store kv.Store, record credentialRecord, previous *credentialRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("credential: encode %s: %w", record.Name, err)
	}
	if previous != nil && previous.Provider != record.Provider {
		if err := store.BatchDelete(ctx, []kv.Key{credentialByProviderKey(string(previous.Provider), string(previous.Name))}); err != nil {
			return fmt.Errorf("credential: delete stale provider index %s: %w", previous.Name, err)
		}
	}
	entries := []kv.Entry{
		{Key: credentialKey(string(record.Name)), Value: data},
		{Key: credentialByProviderKey(string(record.Provider), string(record.Name)), Value: []byte{}},
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("credential: write %s: %w", record.Name, err)
	}
	return nil
}

func getCredentialRecord(ctx context.Context, store kv.Store, name string) (credentialRecord, error) {
	data, err := store.Get(ctx, credentialKey(name))
	if err != nil {
		return credentialRecord{}, err
	}
	return decodeCredentialRecord(data)
}

func decodeCredentialRecord(data []byte) (credentialRecord, error) {
	var record credentialRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return credentialRecord{}, err
	}
	return record, nil
}

func listCredentialRecordsPage(ctx context.Context, store kv.Store, prefix kv.Key, cursor string, limit int) ([]apitypes.Credential, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.Credential, 0, len(pageEntries))
	for _, entry := range pageEntries {
		record, err := decodeCredentialRecord(entry.Value)
		if err != nil {
			return nil, false, nil, fmt.Errorf("credential: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, credentialFromRecord(record))
	}
	return items, hasNext, nextCursor, nil
}

func listCredentialsByProviderPage(ctx context.Context, store kv.Store, provider, cursor string, limit int) ([]apitypes.Credential, bool, *string, error) {
	prefix := credentialByProviderPrefix(provider)
	entries, err := kv.ListAfter(ctx, store, prefix, cursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.Credential, 0, len(pageEntries))
	for _, entry := range pageEntries {
		if len(entry.Key) == 0 {
			continue
		}
		name := unescapeStoreSegment(entry.Key[len(entry.Key)-1])
		record, err := getCredentialRecord(ctx, store, name)
		if err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return nil, false, nil, err
		}
		items = append(items, credentialFromRecord(record))
	}
	return items, hasNext, nextCursor, nil
}

func normalizeCredentialUpsert(in adminhttp.CredentialUpsert, expectedName string) (normalizedCredentialUpsert, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return normalizedCredentialUpsert{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return normalizedCredentialUpsert{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	provider := strings.TrimSpace(string(in.Provider))
	if provider == "" {
		return normalizedCredentialUpsert{}, errors.New("provider is required")
	}
	out := normalizedCredentialUpsert{
		Body:     cloneBody(in.Body),
		Name:     string(name),
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
		if allEmpty(typed.AppId, typed.ApiKey, typed.OpenapiAccessKeyId, typed.OpenapiAccessKey) {
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

func credentialKey(name string) kv.Key {
	return append(append(kv.Key{}, credentialsRoot...), escapeStoreSegment(name))
}

func credentialByProviderPrefix(provider string) kv.Key {
	return append(append(kv.Key{}, credentialsByProviderRoot...), escapeStoreSegment(provider))
}

func credentialByProviderKey(provider, name string) kv.Key {
	return append(credentialByProviderPrefix(provider), escapeStoreSegment(name))
}

func escapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func unescapeStoreSegment(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return unescaped
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

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("credential store not configured")
	}
	return s.Store, nil
}
