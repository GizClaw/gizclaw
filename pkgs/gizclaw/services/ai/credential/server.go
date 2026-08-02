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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	credentialsRoot           = kv.Key{"by-id"}
	credentialsByNameRoot     = kv.Key{"by-name"}
	credentialsByProviderRoot = kv.Key{"by-provider"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store kv.Store
	NewID func() string
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
	ID          string                  `json:"id"`
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
	now := time.Now().UTC()
	record := credentialRecord{
		Body:        cloneBody(upsert.Body),
		CreatedAt:   now,
		Description: cloneString(upsert.Description),
		ID:          s.newID(),
		Name:        upsert.Name,
		Provider:    upsert.Provider,
		UpdatedAt:   now,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return adminhttp.CreateCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	_, created, err := kv.CreateIfAbsent(ctx, store,
		kv.Entry{Key: credentialNameKey(record.Name), Value: []byte(record.ID)},
		[]kv.Entry{
			{Key: credentialKey(record.ID), Value: data},
			{Key: credentialByProviderKey(record.Provider, record.ID), Value: []byte{}},
		},
	)
	if err != nil {
		return adminhttp.CreateCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if !created {
		return adminhttp.CreateCredential409JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_ALREADY_EXISTS", fmt.Sprintf("credential %q already exists", upsert.Name))), nil
	}
	return adminhttp.CreateCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) DeleteCredential(ctx context.Context, request adminhttp.DeleteCredentialRequestObject) (adminhttp.DeleteCredentialResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	record, err := getCredentialRecord(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", id))), nil
		}
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	keys := []kv.Key{
		credentialKey(record.ID),
		credentialNameKey(record.Name),
		credentialByProviderKey(record.Provider, record.ID),
	}
	if err := store.BatchDelete(ctx, keys); err != nil {
		return adminhttp.DeleteCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteCredential200JSONResponse(credentialFromRecord(record)), nil
}

func (s *Server) GetCredential(ctx context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	record, err := getCredentialRecord(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", id))), nil
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
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	upsert, err := normalizeCredentialUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", err.Error())), nil
	}
	previous, err := getCredentialRecord(ctx, store, id)
	if errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutCredential404JSONResponse(apitypes.NewErrorResponse("CREDENTIAL_NOT_FOUND", fmt.Sprintf("credential %q not found", id))), nil
	}
	if err != nil {
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := time.Now().UTC()
	record := credentialRecord{
		Body:        cloneBody(upsert.Body),
		CreatedAt:   now,
		Description: cloneString(upsert.Description),
		ID:          id,
		Name:        upsert.Name,
		Provider:    upsert.Provider,
		UpdatedAt:   now,
	}
	if record.Name != previous.Name {
		return adminhttp.PutCredential400JSONResponse(apitypes.NewErrorResponse("INVALID_CREDENTIAL", fmt.Sprintf("name %q must match immutable name %q", record.Name, previous.Name))), nil
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
	if err := writeCredential(ctx, store, record, &previous); err != nil {
		return adminhttp.PutCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutCredential200JSONResponse(credentialFromRecord(record)), nil
}

func credentialFromRecord(record credentialRecord) apitypes.Credential {
	return apitypes.Credential{
		Body:        cloneBody(record.Body),
		CreatedAt:   record.CreatedAt,
		Description: cloneString(record.Description),
		Id:          record.ID,
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
	var deletes []kv.Key
	if previous != nil && previous.Provider != record.Provider {
		deletes = append(deletes, credentialByProviderKey(previous.Provider, previous.ID))
	}
	entries := []kv.Entry{
		{Key: credentialKey(record.ID), Value: data},
		{Key: credentialByProviderKey(record.Provider, record.ID), Value: []byte{}},
	}
	if err := store.BatchMutate(ctx, entries, deletes); err != nil {
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
		id := unescapeStoreSegment(entry.Key[len(entry.Key)-1])
		record, err := getCredentialRecord(ctx, store, id)
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

func credentialKey(id string) kv.Key {
	return append(append(kv.Key{}, credentialsRoot...), escapeStoreSegment(id))
}

func credentialNameKey(name string) kv.Key {
	return append(append(kv.Key{}, credentialsByNameRoot...), escapeStoreSegment(name))
}

func credentialByProviderPrefix(provider string) kv.Key {
	return append(append(kv.Key{}, credentialsByProviderRoot...), escapeStoreSegment(provider))
}

func credentialByProviderKey(provider, id string) kv.Key {
	return append(credentialByProviderPrefix(provider), escapeStoreSegment(id))
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

func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return socialutil.NewID()
}
