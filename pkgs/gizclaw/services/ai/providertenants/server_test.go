package providertenants

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/volcengine/volcengine-go-sdk/service/speechsaasprod"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerMiniMaxTenantsCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("tok-main"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})

	createBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main",
		"base_url": "https://api.minimax.chat",
		"description": "primary tenant"
	}`)
	createResp, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateMiniMaxTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateMiniMaxTenant() response = %#v", createResp)
	}
	if created.Name != "tenant-a" || created.CredentialName != "cred-main" {
		t.Fatalf("CreateMiniMaxTenant() tenant = %#v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("CreateMiniMaxTenant() timestamps = %#v", created)
	}

	getResp, err := srv.GetMiniMaxTenant(ctx, adminhttp.GetMiniMaxTenantRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("GetMiniMaxTenant() error = %v", err)
	}
	got, ok := getResp.(adminhttp.GetMiniMaxTenant200JSONResponse)
	if !ok {
		t.Fatalf("GetMiniMaxTenant() response = %#v", getResp)
	}
	if got.AppId != "app-1" || got.GroupId != "group-1" {
		t.Fatalf("GetMiniMaxTenant() tenant = %#v", got)
	}

	updateBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-2",
		"group_id": "group-2",
		"credential_name": "cred-main",
		"description": "updated tenant"
	}`)
	putResp, err := srv.PutMiniMaxTenant(ctx, adminhttp.PutMiniMaxTenantRequestObject{
		Name: "tenant-a",
		Body: &updateBody,
	})
	if err != nil {
		t.Fatalf("PutMiniMaxTenant() error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutMiniMaxTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutMiniMaxTenant() response = %#v", putResp)
	}
	if updated.CreatedAt != created.CreatedAt {
		t.Fatalf("PutMiniMaxTenant() created_at = %v, want %v", updated.CreatedAt, created.CreatedAt)
	}
	if updated.AppId != "app-2" || updated.GroupId != "group-2" {
		t.Fatalf("PutMiniMaxTenant() tenant = %#v", updated)
	}

	listResp, err := srv.ListMiniMaxTenants(ctx, adminhttp.ListMiniMaxTenantsRequestObject{})
	if err != nil {
		t.Fatalf("ListMiniMaxTenants() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListMiniMaxTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListMiniMaxTenants() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].Name != "tenant-a" {
		t.Fatalf("ListMiniMaxTenants() = %#v", listed)
	}

	voice := apitypes.Voice{
		CreatedAt: created.CreatedAt,
		Id:        "minimax-tenant:tenant-a:voice-1",
		Provider: apitypes.VoiceProvider{
			Kind: miniMaxProviderKind,
			Name: string("tenant-a"),
		},
		ProviderData: providerData(miniMaxProviderKind, map[string]interface{}{
			"voice_id": "voice-1",
		}),
		Source:    apitypes.VoiceSourceSync,
		SyncedAt:  timePtr(created.CreatedAt),
		UpdatedAt: created.CreatedAt,
	}
	voiceStore, err := srv.voiceStore()
	if err != nil {
		t.Fatalf("voiceStore() error = %v", err)
	}
	if err := writeVoice(ctx, voiceStore, voice, nil); err != nil {
		t.Fatalf("writeVoice() error = %v", err)
	}
	manualVoice := apitypes.Voice{
		CreatedAt: created.CreatedAt,
		Id:        "manual:tenant-a:voice-2",
		Provider: apitypes.VoiceProvider{
			Kind: miniMaxProviderKind,
			Name: string("tenant-a"),
		},
		Source:    apitypes.VoiceSourceManual,
		UpdatedAt: created.CreatedAt,
	}
	if err := writeVoice(ctx, voiceStore, manualVoice, nil); err != nil {
		t.Fatalf("writeVoice(manual) error = %v", err)
	}

	deleteResp, err := srv.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("DeleteMiniMaxTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteMiniMaxTenant200JSONResponse); !ok {
		t.Fatalf("DeleteMiniMaxTenant() response = %#v", deleteResp)
	}
	if _, err := getVoice(ctx, voiceStore, string(voice.Id)); err != kv.ErrNotFound {
		t.Fatalf("getVoice() after tenant delete err = %v, want kv.ErrNotFound", err)
	}
	if _, err := getVoice(ctx, voiceStore, string(manualVoice.Id)); err != nil {
		t.Fatalf("manual voice after tenant delete err = %v, want nil", err)
	}
}

func TestServerMiniMaxTenantsPaginationAndValidation(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("tok-main"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})

	for _, body := range []adminhttp.MiniMaxTenantUpsert{
		{Name: "alpha", AppId: "app-a", GroupId: "group-a", CredentialName: "cred-main"},
		{Name: "beta", AppId: "app-b", GroupId: "group-b", CredentialName: "cred-main"},
		{Name: "gamma", AppId: "app-c", GroupId: "group-c", CredentialName: "cred-main"},
	} {
		if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateMiniMaxTenant(%q) error = %v", body.Name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListMiniMaxTenants(ctx, adminhttp.ListMiniMaxTenantsRequestObject{
		Params: adminhttp.ListMiniMaxTenantsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListMiniMaxTenants(first page) error = %v", err)
	}
	first, ok := firstResp.(adminhttp.ListMiniMaxTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListMiniMaxTenants(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListMiniMaxTenants(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListMiniMaxTenants(ctx, adminhttp.ListMiniMaxTenantsRequestObject{
		Params: adminhttp.ListMiniMaxTenantsParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListMiniMaxTenants(second page) error = %v", err)
	}
	second, ok := secondResp.(adminhttp.ListMiniMaxTenants200JSONResponse)
	if !ok {
		t.Fatalf("ListMiniMaxTenants(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 || second.Items[0].Name == first.Items[0].Name {
		t.Fatalf("ListMiniMaxTenants(second page) = %#v", second)
	}

	invalidBody := adminhttp.MiniMaxTenantUpsert{
		Name:           "missing-cred",
		GroupId:        "group-x",
		CredentialName: "not-found",
	}
	invalidResp, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &invalidBody})
	if err != nil {
		t.Fatalf("CreateMiniMaxTenant(missing cred) error = %v", err)
	}
	if _, ok := invalidResp.(adminhttp.CreateMiniMaxTenant400JSONResponse); !ok {
		t.Fatalf("CreateMiniMaxTenant(missing cred) response = %#v", invalidResp)
	}

	nilCreateResp, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{})
	if err != nil {
		t.Fatalf("CreateMiniMaxTenant(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminhttp.CreateMiniMaxTenant400JSONResponse); !ok {
		t.Fatalf("CreateMiniMaxTenant(nil body) response = %#v", nilCreateResp)
	}

	nilPutResp, err := srv.PutMiniMaxTenant(ctx, adminhttp.PutMiniMaxTenantRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("PutMiniMaxTenant(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminhttp.PutMiniMaxTenant400JSONResponse); !ok {
		t.Fatalf("PutMiniMaxTenant(nil body) response = %#v", nilPutResp)
	}

	getMissingResp, err := srv.GetMiniMaxTenant(ctx, adminhttp.GetMiniMaxTenantRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("GetMiniMaxTenant(missing) error = %v", err)
	}
	if _, ok := getMissingResp.(adminhttp.GetMiniMaxTenant404JSONResponse); !ok {
		t.Fatalf("GetMiniMaxTenant(missing) response = %#v", getMissingResp)
	}

	deleteMissingResp, err := srv.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteMiniMaxTenant(missing) error = %v", err)
	}
	if _, ok := deleteMissingResp.(adminhttp.DeleteMiniMaxTenant404JSONResponse); !ok {
		t.Fatalf("DeleteMiniMaxTenant(missing) response = %#v", deleteMissingResp)
	}
}

func TestServerMiniMaxCredentialValidation(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	tenant := apitypes.MiniMaxTenant{
		CredentialName: "cred-main",
		GroupId:        "group-1",
		Name:           "tenant-a",
	}

	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "openai",
		Body:      testOpenAICredentialBody("sk-test"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	credentialStore, err := srv.credentialStore()
	if err != nil {
		t.Fatalf("credentialStore() error = %v", err)
	}
	if _, err := srv.miniMaxClientForTenant(ctx, credentialStore, tenant); err == nil {
		t.Fatalf("miniMaxClientForTenant(openai provider) error = nil, want error")
	}

	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      apitypes.CredentialBody{},
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	if _, err := srv.miniMaxClientForTenant(ctx, credentialStore, tenant); err == nil {
		t.Fatalf("miniMaxClientForTenant(missing api key) error = nil, want error")
	}

	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("mmx-key"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	client, err := srv.miniMaxClientForTenant(ctx, credentialStore, tenant)
	if err != nil {
		t.Fatalf("miniMaxClientForTenant() error = %v", err)
	}
	if client == nil {
		t.Fatal("miniMaxClientForTenant() returned nil client")
	}
	missingTenant := tenant
	missingTenant.CredentialName = "missing-cred"
	if _, err := srv.miniMaxCredentialForTenant(ctx, credentialStore, missingTenant); err == nil || !strings.Contains(err.Error(), `credential "missing-cred" not found`) {
		t.Fatalf("miniMaxCredentialForTenant(missing) error = %v", err)
	}

	missingTenantResp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("SyncMiniMaxTenantVoices(missing tenant) error = %v", err)
	}
	if _, ok := missingTenantResp.(adminhttp.SyncMiniMaxTenantVoices404JSONResponse); !ok {
		t.Fatalf("SyncMiniMaxTenantVoices(missing tenant) response = %#v", missingTenantResp)
	}
}

func TestServerMiniMaxHelpers(t *testing.T) {
	t.Parallel()

	if miniMaxCredentialRejected(nil) {
		t.Fatal("miniMaxCredentialRejected(nil) = true")
	}
	for _, err := range []error{
		errors.New("invalid api key"),
		errors.New("login fail"),
		errors.New("status_code=2049"),
		errors.New("status=403"),
		errors.New("unauthorized"),
	} {
		if !miniMaxCredentialRejected(err) {
			t.Fatalf("miniMaxCredentialRejected(%v) = false", err)
		}
	}
	if miniMaxCredentialRejected(errors.New("temporary upstream error")) {
		t.Fatal("miniMaxCredentialRejected(temporary) = true")
	}

	if got := miniMaxBaseURL(apitypes.MiniMaxTenant{}); got != defaultMiniMaxBaseURL {
		t.Fatalf("miniMaxBaseURL(default) = %q, want %q", got, defaultMiniMaxBaseURL)
	}
	baseURL := "https://voice.example.test"
	if got := miniMaxBaseURL(apitypes.MiniMaxTenant{BaseUrl: &baseURL}); got != baseURL {
		t.Fatalf("miniMaxBaseURL(tenant) = %q, want %q", got, baseURL)
	}
	credential := apitypes.Credential{
		Body: testMiniMaxCredentialBodyFromStrings(map[string]string{
			"voice_base_url":         " https://voice.example.test ",
			"minimax_voice_base_url": "https://voice-backup.example.test",
			"base_url":               "https://api.example.test",
		}),
	}
	candidates := miniMaxBaseURLCandidates(
		apitypes.MiniMaxTenant{BaseUrl: stringPtr("https://tenant.example.test")},
		credential,
		[]string{"https://fallback.example.test", "https://voice-backup.example.test"},
	)
	if !slices.Equal(candidates, []string{
		"https://tenant.example.test",
		"https://voice.example.test",
		"https://voice-backup.example.test",
		"https://fallback.example.test",
		"https://api.example.test",
	}) {
		t.Fatalf("miniMaxBaseURLCandidates() = %#v", candidates)
	}
	for _, tt := range []struct {
		raw  string
		want string
	}{
		{raw: "", want: ""},
		{raw: " https://api.minimax.chat/v1/ ", want: "https://api.minimax.chat"},
		{raw: "https://api.minimax.chat/anthropic", want: "https://api.minimax.chat"},
		{raw: "https://api.minimax.chat/v1/anthropic", want: "https://api.minimax.chat"},
	} {
		if got := normalizeMiniMaxVoiceBaseURL(tt.raw); got != tt.want {
			t.Fatalf("normalizeMiniMaxVoiceBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
	apiKey, err := miniMaxAPIKey(apitypes.Credential{Name: "mmx", Body: testMiniMaxCredentialBodyFromStrings(map[string]string{"token": " token-key "})})
	if err != nil {
		t.Fatalf("miniMaxAPIKey(token fallback) error = %v", err)
	}
	if apiKey != "token-key" {
		t.Fatalf("miniMaxAPIKey(token fallback) = %q, want token-key", apiKey)
	}
	if _, err := miniMaxAPIKey(apitypes.Credential{Name: "mmx", Body: testMiniMaxCredentialBodyFromStrings(nil)}); err == nil {
		t.Fatal("miniMaxAPIKey(missing) error = nil")
	}
	if got := testCredentialBodyString(testOpenAICredentialBody("12345"), "api_key"); got != "12345" {
		t.Fatalf("testCredentialBodyString(api key) = %q, want 12345", got)
	}
	left := map[string]interface{}{"a": float64(1), "b": "text"}
	right := map[string]interface{}{"b": "text", "a": float64(1)}
	if !rawEqual(&left, &right) {
		t.Fatalf("rawEqual() = false, want true")
	}
	different := map[string]interface{}{"a": float64(2)}
	if rawEqual(&left, &different) {
		t.Fatalf("rawEqual(different) = true, want false")
	}
	if rawEqual(nil, &right) || rawEqual(&left, nil) {
		t.Fatalf("rawEqual(nil) = true, want false")
	}
	if matchesVoiceFilters(apitypes.Voice{Source: apitypes.VoiceSourceManual}, voiceFilters{source: stringPtr("sync")}) {
		t.Fatalf("matchesVoiceFilters(source mismatch) = true, want false")
	}
}

type testStringer string

func (s testStringer) String() string {
	return string(s)
}

func TestVoiceHelperEdgeCases(t *testing.T) {
	t.Parallel()

	if !mapEqual(nil, nil) {
		t.Fatal("mapEqual(nil, nil) = false")
	}
	empty := map[string]interface{}{}
	if mapEqual(nil, &empty) {
		t.Fatal("mapEqual(nil, empty) = true")
	}
	unmarshalable := map[string]interface{}{"ch": make(chan struct{})}
	if mapEqual(&unmarshalable, &empty) {
		t.Fatal("mapEqual(unmarshalable, empty) = true")
	}
	voice := apitypes.Voice{
		Provider: apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindMinimaxTenant, Name: "tenant"},
	}
	providerData := apitypes.VoiceProviderData{}
	voiceID := " voice-1 "
	if err := providerData.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{VoiceId: &voiceID}); err != nil {
		t.Fatalf("FromMiniMaxTenantVoiceProviderData() error = %v", err)
	}
	voice.ProviderData = &providerData
	if got := voiceProviderDataString(voice, "voice_id"); got != "voice-1" {
		t.Fatalf("voiceProviderDataString(map[string]string) = %q", got)
	}
	if got := providerDataString(testStringer(" value ")); got != "value" {
		t.Fatalf("providerDataString(Stringer) = %q", got)
	}
	if got := unescapeStoreSegment("%zz"); got != "%zz" {
		t.Fatalf("unescapeStoreSegment(invalid) = %q", got)
	}
	now := time.Now().UTC()
	cloned := cloneTime(&now)
	if cloned == nil || !cloned.Equal(now) || cloned == &now {
		t.Fatalf("cloneTime() = %#v", cloned)
	}
	raw := rawMessagesToMap(map[string]json.RawMessage{"bad": json.RawMessage(`{`)})
	if raw == nil || (*raw)["bad"] != "{" {
		t.Fatalf("rawMessagesToMap(invalid json) = %#v", raw)
	}
}

func TestDecodeVoiceMigratesLegacyProviderFields(t *testing.T) {
	t.Parallel()

	var voice apitypes.Voice
	if err := decodeVoice([]byte(`{
		"id": "minimax-tenant:tenant-a:voice-1",
		"source": "sync",
		"provider": {"kind": "minimax-tenant", "name": "tenant-a"},
		"provider_voice_id": "voice-1",
		"provider_voice_type": "system",
		"raw": {"gender": "female"},
		"created_at": "2026-05-06T03:48:50Z",
		"updated_at": "2026-05-06T03:48:50Z"
	}`), &voice); err != nil {
		t.Fatalf("decodeVoice() error = %v", err)
	}
	if voiceProviderDataString(voice, "voice_id") != "voice-1" || voiceProviderDataString(voice, "voice_type") != "system" {
		t.Fatalf("provider data = %#v", voice.ProviderData)
	}
	providerData, err := voice.ProviderData.AsMiniMaxTenantVoiceProviderData()
	if err != nil {
		t.Fatalf("provider data = %#v", voice.ProviderData)
	}
	if providerData.Raw == nil || (*providerData.Raw)["gender"] != "female" {
		t.Fatalf("raw provider data = %#v", providerData.Raw)
	}
}

func TestServerMiniMaxStoreNotConfigured(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	ctx := context.Background()
	listResp, err := srv.ListMiniMaxTenants(ctx, adminhttp.ListMiniMaxTenantsRequestObject{})
	if err != nil {
		t.Fatalf("ListMiniMaxTenants() error = %v", err)
	}
	if _, ok := listResp.(adminhttp.ListMiniMaxTenants500JSONResponse); !ok {
		t.Fatalf("ListMiniMaxTenants() response = %#v", listResp)
	}
}

func TestServerMiniMaxStoreHelpers(t *testing.T) {
	t.Parallel()

	var nilServer *Server
	if _, err := nilServer.tenantStore(); err == nil {
		t.Fatal("nil server tenantStore() error = nil")
	}
	if _, err := nilServer.voiceStore(); err == nil {
		t.Fatal("nil server voiceStore() error = nil")
	}
	if _, err := nilServer.credentialStore(); err == nil {
		t.Fatal("nil server credentialStore() error = nil")
	}
	if _, err := nilServer.volcTenantStore(); err == nil {
		t.Fatal("nil server volcTenantStore() error = nil")
	}
	if _, err := (&Server{}).tenantStore(); err == nil {
		t.Fatal("empty server tenantStore() error = nil")
	}
	if _, err := (&Server{}).voiceStore(); err == nil {
		t.Fatal("empty server voiceStore() error = nil")
	}
	if _, err := (&Server{}).credentialStore(); err == nil {
		t.Fatal("empty server credentialStore() error = nil")
	}
	if _, err := (&Server{}).volcTenantStore(); err == nil {
		t.Fatal("empty server volcTenantStore() error = nil")
	}

	base := kv.NewMemory(nil)
	srv := &Server{Store: base}
	if got, err := srv.tenantStore(); err != nil || got != base {
		t.Fatalf("tenantStore fallback = %v, %v", got, err)
	}
	if got, err := srv.voiceStore(); err != nil || got != base {
		t.Fatalf("voiceStore fallback = %v, %v", got, err)
	}
	if got, err := srv.credentialStore(); err != nil || got != base {
		t.Fatalf("credentialStore fallback = %v, %v", got, err)
	}
	if got, err := srv.volcTenantStore(); err != nil || got != base {
		t.Fatalf("volcTenantStore fallback = %v, %v", got, err)
	}

	tenantStore := kv.NewMemory(nil)
	volcTenantStore := kv.NewMemory(nil)
	voiceStore := kv.NewMemory(nil)
	credentialStore := kv.NewMemory(nil)
	srv.TenantStore = tenantStore
	srv.VolcTenantStore = volcTenantStore
	srv.VoiceStore = voiceStore
	srv.CredentialStore = credentialStore
	if got, err := srv.tenantStore(); err != nil || got != tenantStore {
		t.Fatalf("tenantStore explicit = %v, %v", got, err)
	}
	if got, err := srv.voiceStore(); err != nil || got != voiceStore {
		t.Fatalf("voiceStore explicit = %v, %v", got, err)
	}
	if got, err := srv.credentialStore(); err != nil || got != credentialStore {
		t.Fatalf("credentialStore explicit = %v, %v", got, err)
	}
	if got, err := srv.volcTenantStore(); err != nil || got != volcTenantStore {
		t.Fatalf("volcTenantStore explicit = %v, %v", got, err)
	}

	srv.VolcTenantStore = nil
	if got, err := srv.volcTenantStore(); err != nil || got != tenantStore {
		t.Fatalf("volcTenantStore tenant fallback = %v, %v", got, err)
	}
}

func TestServerMiniMaxTenantValidationAndConflictPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("tok-main"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})

	body := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateMiniMaxTenant(seed) error = %v", err)
	}

	conflictResp, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateMiniMaxTenant(conflict) error = %v", err)
	}
	if _, ok := conflictResp.(adminhttp.CreateMiniMaxTenant409JSONResponse); !ok {
		t.Fatalf("CreateMiniMaxTenant(conflict) response = %#v", conflictResp)
	}

	pathMismatch := mustMiniMaxTenantUpsert(t, `{
		"name": "other-name",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	pathMismatchResp, err := srv.PutMiniMaxTenant(ctx, adminhttp.PutMiniMaxTenantRequestObject{
		Name: "tenant-a",
		Body: &pathMismatch,
	})
	if err != nil {
		t.Fatalf("PutMiniMaxTenant(path mismatch) error = %v", err)
	}
	if _, ok := pathMismatchResp.(adminhttp.PutMiniMaxTenant400JSONResponse); !ok {
		t.Fatalf("PutMiniMaxTenant(path mismatch) response = %#v", pathMismatchResp)
	}

	invalidBaseURL := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-b",
		"app_id": "app-2",
		"group_id": "group-2",
		"credential_name": "cred-main",
		"base_url": "not-a-url"
	}`)
	invalidBaseURLResp, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &invalidBaseURL})
	if err != nil {
		t.Fatalf("CreateMiniMaxTenant(invalid base_url) error = %v", err)
	}
	if _, ok := invalidBaseURLResp.(adminhttp.CreateMiniMaxTenant400JSONResponse); !ok {
		t.Fatalf("CreateMiniMaxTenant(invalid base_url) response = %#v", invalidBaseURLResp)
	}

	deleteMissingResp, err := srv.DeleteMiniMaxTenant(ctx, adminhttp.DeleteMiniMaxTenantRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteMiniMaxTenant(missing) error = %v", err)
	}
	if _, ok := deleteMissingResp.(adminhttp.DeleteMiniMaxTenant404JSONResponse); !ok {
		t.Fatalf("DeleteMiniMaxTenant(missing) response = %#v", deleteMissingResp)
	}
}

func TestServerSyncMiniMaxTenantVoicesUsesTenantBaseURL(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.URL.Path; got != "/v1/get_voice" {
			t.Fatalf("path = %s, want /v1/get_voice", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mmx-key" {
			t.Fatalf("authorization = %q, want Bearer mmx-key", got)
		}
		switch got := r.URL.Query().Get("voice_type"); got {
		case "all", "system", "voice_cloning", "voice_generation":
		default:
			t.Fatalf("query.voice_type = %q, want supported voice type", got)
		}
		_, _ = w.Write([]byte(`{
			"base_resp":{"status_code":0,"status_msg":"ok"},
			"voices":[
				{"voice_id":"voice-1","voice_name":"calm narrator","description":["calm"],"voice_type":"system","gender":"female"},
				{"voice_id":"voice-2","voice_name":"fast narrator","description":["fast"],"voice_type":"voice_cloning","gender":"male"}
			],
			"has_more":false
		}`))
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	srv.MiniMaxBaseURLs = []string{upstream.URL}
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBodyFromStrings(map[string]string{"api_key": "mmx-key", "base_url": "https://models.example.invalid"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	tenantBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	tenantBody.BaseUrl = stringPtr(upstream.URL)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &tenantBody}); err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}

	resp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncMiniMaxTenantVoices() error = %v", err)
	}
	syncResp, ok := resp.(adminhttp.SyncMiniMaxTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncMiniMaxTenantVoices() response = %#v", resp)
	}
	if syncResp.CreatedCount != 2 || syncResp.UpdatedCount != 0 || syncResp.DeletedCount != 0 {
		t.Fatalf("SyncMiniMaxTenantVoices() result = %#v", syncResp)
	}
	if callCount.Load() != 4 {
		t.Fatalf("upstream call count = %d, want 4", callCount.Load())
	}

	voice := requireStoredVoice(t, srv, ctx, "minimax-tenant:tenant-a:voice-1")
	if voice.Source != apitypes.VoiceSourceSync || voiceProviderDataString(voice, "voice_id") != "voice-1" {
		t.Fatalf("stored sync voice = %#v", voice)
	}
	providerData, err := voice.ProviderData.AsMiniMaxTenantVoiceProviderData()
	if err != nil {
		t.Fatalf("stored sync voice provider data = %#v", voice.ProviderData)
	}
	if providerData.Raw == nil || (*providerData.Raw)["gender"] != "female" {
		t.Fatalf("stored sync voice raw = %#v", providerData.Raw)
	}
}

func TestServerSyncMiniMaxTenantVoicesRetriesTransientEOF(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callCount.Add(1) == 1 {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Fatalf("hijack transient EOF response: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"base_resp":{"status_code":0,"status_msg":"ok"},
			"voices":[{"voice_id":"voice-1","voice_name":"retry narrator","voice_type":"system"}],
			"has_more":false
		}`))
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	srv.MiniMaxBaseURLs = []string{upstream.URL}
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("mmx-key"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	tenantBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	tenantBody.BaseUrl = stringPtr(upstream.URL)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &tenantBody}); err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}

	resp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncMiniMaxTenantVoices() error = %v", err)
	}
	syncResp, ok := resp.(adminhttp.SyncMiniMaxTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncMiniMaxTenantVoices() response = %#v", resp)
	}
	if syncResp.CreatedCount != 1 || callCount.Load() != 5 {
		t.Fatalf("sync result = %#v, upstream calls = %d", syncResp, callCount.Load())
	}
}

func TestServerSyncMiniMaxTenantVoicesCredentialRejected(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_resp":{"status_code":2049,"status_msg":"invalid api key"}}`))
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	srv.MiniMaxBaseURLs = []string{upstream.URL}
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("bad-key"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	tenantBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	tenantBody.BaseUrl = stringPtr(upstream.URL)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &tenantBody}); err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}

	resp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncMiniMaxTenantVoices() error = %v", err)
	}
	rejected, ok := resp.(adminhttp.SyncMiniMaxTenantVoices400JSONResponse)
	if !ok {
		t.Fatalf("SyncMiniMaxTenantVoices() response = %#v, want 400", resp)
	}
	if rejected.Error.Code != "INVALID_MINIMAX_TENANT" || !strings.Contains(rejected.Error.Message, "invalid api key") {
		t.Fatalf("SyncMiniMaxTenantVoices() error = %#v", rejected.Error)
	}
}

func TestServerSyncMiniMaxTenantVoicesFallsBackAfterRegionalAuthError(t *testing.T) {
	t.Parallel()

	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_resp":{"status_code":2049,"status_msg":"invalid api key"}}`))
	}))
	defer rejecting.Close()

	var successCount atomic.Int32
	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		successCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"base_resp":{"status_code":0,"status_msg":"ok"},
			"system_voice":[{"voice_id":"voice-cn-1","voice_name":"cn narrator"}],
			"has_more":false
		}`))
	}))
	defer success.Close()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBodyFromStrings(map[string]string{"api_key": "mmx-key", "voice_base_url": success.URL}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	tenantBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	tenantBody.BaseUrl = stringPtr(rejecting.URL)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &tenantBody}); err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}

	resp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncMiniMaxTenantVoices() error = %v", err)
	}
	synced, ok := resp.(adminhttp.SyncMiniMaxTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncMiniMaxTenantVoices() response = %#v", resp)
	}
	if synced.CreatedCount != 1 || successCount.Load() != 4 {
		t.Fatalf("sync result = %#v, success calls = %d", synced, successCount.Load())
	}
}

func TestServerSyncMiniMaxTenantVoicesReconcile(t *testing.T) {
	t.Parallel()

	var stage atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch stage.Load() {
		case 0:
			_, _ = w.Write([]byte(`{
				"base_resp":{"status_code":0,"status_msg":"ok"},
				"voices":[
					{"voice_id":"voice-1","voice_name":"first","voice_type":"system"},
					{"voice_id":"voice-2","voice_name":"second","voice_type":"system"}
				],
				"has_more":false
			}`))
		default:
			_, _ = w.Write([]byte(`{
				"base_resp":{"status_code":0,"status_msg":"ok"},
				"voices":[
					{"voice_id":"voice-1","voice_name":"first-updated","voice_type":"system"}
				],
				"has_more":false
			}`))
		}
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBodyFromStrings(map[string]string{"api_key": "mmx-key", "base_url": "https://models.example.invalid"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	tenantBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	tenantBody.BaseUrl = stringPtr(upstream.URL)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &tenantBody}); err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}
	manualVoice := apitypes.Voice{
		CreatedAt: srv.now(),
		Id:        "manual:tenant-a:keep",
		Provider: apitypes.VoiceProvider{
			Kind: miniMaxProviderKind,
			Name: string("tenant-a"),
		},
		Source:    apitypes.VoiceSourceManual,
		UpdatedAt: srv.now(),
	}
	voiceStore, err := srv.voiceStore()
	if err != nil {
		t.Fatalf("voiceStore() error = %v", err)
	}
	if err := writeVoice(ctx, voiceStore, manualVoice, nil); err != nil {
		t.Fatalf("writeVoice(manual) error = %v", err)
	}

	firstResp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("first SyncMiniMaxTenantVoices() error = %v", err)
	}
	first, ok := firstResp.(adminhttp.SyncMiniMaxTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("first SyncMiniMaxTenantVoices() response = %#v", firstResp)
	}
	if first.CreatedCount != 2 || first.UpdatedCount != 0 || first.DeletedCount != 0 {
		t.Fatalf("first SyncMiniMaxTenantVoices() result = %#v", first)
	}

	stage.Store(1)
	secondResp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("second SyncMiniMaxTenantVoices() error = %v", err)
	}
	second, ok := secondResp.(adminhttp.SyncMiniMaxTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("second SyncMiniMaxTenantVoices() response = %#v", secondResp)
	}
	if second.CreatedCount != 0 || second.UpdatedCount != 1 || second.DeletedCount != 1 {
		t.Fatalf("second SyncMiniMaxTenantVoices() result = %#v", second)
	}

	updatedVoice := requireStoredVoice(t, srv, ctx, "minimax-tenant:tenant-a:voice-1")
	if updatedVoice.Name == nil || *updatedVoice.Name != "first-updated" {
		t.Fatalf("updated sync voice = %#v", updatedVoice)
	}

	requireMissingVoice(t, srv, ctx, "minimax-tenant:tenant-a:voice-2")

	requireStoredVoice(t, srv, ctx, manualVoice.Id)
}

func TestServerSyncMiniMaxTenantVoicesFetchesAllVoiceTypes(t *testing.T) {
	t.Parallel()

	typeCounts := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		voiceType := r.URL.Query().Get("voice_type")
		typeCounts[voiceType]++
		w.Header().Set("Content-Type", "application/json")
		switch voiceType {
		case "all":
			_, _ = w.Write([]byte(`{
				"base_resp":{"status_code":0,"status_msg":"ok"},
				"voices":[
					{"voice_id":"voice-system-1","voice_name":"all-system"}
				],
				"has_more":false
			}`))
		case "system":
			_, _ = w.Write([]byte(`{
				"base_resp":{"status_code":0,"status_msg":"ok"},
				"system_voice":[
					{"voice_id":"voice-system-1","voice_name":"system narrator"}
				],
				"has_more":false
			}`))
		case "voice_cloning":
			_, _ = w.Write([]byte(`{
				"base_resp":{"status_code":0,"status_msg":"ok"},
				"voice_cloning":[
					{"voice_id":"voice-clone-1","voice_name":"clone narrator"}
				],
				"has_more":false
			}`))
		case "voice_generation":
			_, _ = w.Write([]byte(`{
				"base_resp":{"status_code":0,"status_msg":"ok"},
				"voice_generation":[
					{"voice_id":"voice-gen-1","voice_name":"generated narrator"}
				],
				"has_more":false
			}`))
		default:
			t.Fatalf("unexpected voice_type = %q", voiceType)
		}
	}))
	defer upstream.Close()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "cred-main",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBodyFromStrings(map[string]string{"api_key": "mmx-key", "base_url": "https://models.example.invalid"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	tenantBody := mustMiniMaxTenantUpsert(t, `{
		"name": "tenant-a",
		"app_id": "app-1",
		"group_id": "group-1",
		"credential_name": "cred-main"
	}`)
	tenantBody.BaseUrl = stringPtr(upstream.URL)
	if _, err := srv.CreateMiniMaxTenant(ctx, adminhttp.CreateMiniMaxTenantRequestObject{Body: &tenantBody}); err != nil {
		t.Fatalf("CreateMiniMaxTenant() error = %v", err)
	}

	resp, err := srv.SyncMiniMaxTenantVoices(ctx, adminhttp.SyncMiniMaxTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncMiniMaxTenantVoices() error = %v", err)
	}
	syncResp, ok := resp.(adminhttp.SyncMiniMaxTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncMiniMaxTenantVoices() response = %#v", resp)
	}
	if syncResp.CreatedCount != 3 || syncResp.UpdatedCount != 0 || syncResp.DeletedCount != 0 {
		t.Fatalf("SyncMiniMaxTenantVoices() result = %#v", syncResp)
	}
	if typeCounts["all"] != 1 || typeCounts["system"] != 1 || typeCounts["voice_cloning"] != 1 || typeCounts["voice_generation"] != 1 {
		t.Fatalf("voice type fetch counts = %#v", typeCounts)
	}

	for _, id := range []string{
		"minimax-tenant:tenant-a:voice-system-1",
		"minimax-tenant:tenant-a:voice-clone-1",
		"minimax-tenant:tenant-a:voice-gen-1",
	} {
		requireStoredVoice(t, srv, ctx, id)
	}
}

func TestServerVolcTenantsCRUDAndSyncVoices(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-main",
		Provider:  "volcengine",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	fakeClient := &fakeVolcSpeakerClient{
		speakers: []volcSpeaker{
			{
				VoiceType:  "zh_female_public",
				Name:       "Public Female",
				ResourceID: "seed-tts-2.0",
				Raw:        map[string]interface{}{"VoiceType": "zh_female_public", "ResourceID": "seed-tts-2.0"},
			},
		},
		pages: []*volcMegaTTSTrainStatusPage{{
			PageNumber: 1,
			PageSize:   100,
			TotalCount: 2,
			Statuses: []volcSpeakerStatus{
				{
					Alias:          "Doubao Female",
					Description:    "female voice",
					InstanceStatus: "active",
					ResourceID:     "seed-tts-2.0",
					SpeakerID:      "S_female_1",
					State:          "Success",
				},
				{
					Alias:          "Doubao Male",
					InstanceStatus: "active",
					ResourceID:     "seed-icl-2.0",
					SpeakerID:      "S_male_1",
					State:          "Success",
				},
			},
		}},
	}
	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return fakeClient, nil
	}

	resourceIDs := []string{"seed-tts-2.0", "seed-icl-2.0"}
	createBody := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "volc-main",
		Region:         stringPtr("cn-beijing"),
		ResourceIds:    &resourceIDs,
		Description:    stringPtr("primary tenant"),
	}
	createResp, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateVolcTenant() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateVolcTenant200JSONResponse)
	if !ok {
		t.Fatalf("CreateVolcTenant() response = %#v", createResp)
	}
	if created.Name != "tenant-a" || created.CredentialName != "volc-main" {
		t.Fatalf("CreateVolcTenant() tenant = %#v", created)
	}
	listResp, err := srv.ListVolcTenants(ctx, adminhttp.ListVolcTenantsRequestObject{})
	if err != nil {
		t.Fatalf("ListVolcTenants() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListVolcTenants200JSONResponse)
	if !ok || len(listed.Items) != 1 || listed.Items[0].Name != "tenant-a" {
		t.Fatalf("ListVolcTenants() response = %#v", listResp)
	}

	syncResp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncVolcTenantVoices() error = %v", err)
	}
	synced, ok := syncResp.(adminhttp.SyncVolcTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncVolcTenantVoices() response = %#v", syncResp)
	}
	if synced.CreatedCount != 3 || synced.UpdatedCount != 0 || synced.DeletedCount != 0 {
		t.Fatalf("SyncVolcTenantVoices() result = %#v", synced)
	}
	if len(fakeClient.requestedResourceIDs) != 1 || !slices.Equal(fakeClient.requestedResourceIDs[0], resourceIDs) {
		t.Fatalf("BatchListMegaTTSTrainStatus ResourceIDs = %#v, want %#v", fakeClient.requestedResourceIDs, resourceIDs)
	}
	for _, id := range []string{
		"volc-tenant:tenant-a:zh_female_public",
		"volc-tenant:tenant-a:S_female_1",
		"volc-tenant:tenant-a:S_male_1",
	} {
		voice := requireStoredVoice(t, srv, ctx, string(id))
		if voice.Provider.Kind != volcProviderKind || voice.Provider.Name != "tenant-a" {
			t.Fatalf("GetVoice(%s) provider = %#v", id, voice.Provider)
		}
		if id == "volc-tenant:tenant-a:zh_female_public" && voiceProviderDataString(apitypes.Voice(voice), "resource_id") != "seed-tts-2.0" {
			t.Fatalf("GetVoice(%s) resource_id = %q, want seed-tts-2.0", id, voiceProviderDataString(apitypes.Voice(voice), "resource_id"))
		}
		for _, removedKey := range []string{"source", "source_api", "speaker_id_prefix"} {
			if value := voiceProviderDataString(apitypes.Voice(voice), removedKey); value != "" {
				t.Fatalf("GetVoice(%s) provider_data[%s] = %q, want empty", id, removedKey, value)
			}
		}
	}

	fakeClient.speakers = []volcSpeaker{
		{
			VoiceType:  "zh_female_public",
			Name:       "Public Female Updated",
			ResourceID: "seed-tts-2.0",
			Raw:        map[string]interface{}{"VoiceType": "zh_female_public", "ResourceID": "seed-tts-2.0", "version": "2"},
		},
	}
	fakeClient.pages = []*volcMegaTTSTrainStatusPage{{
		PageNumber: 1,
		PageSize:   100,
		TotalCount: 1,
		Statuses: []volcSpeakerStatus{{
			Alias:          "Doubao Female Updated",
			Description:    "female voice updated",
			InstanceStatus: "active",
			ResourceID:     "seed-tts-2.0",
			SpeakerID:      "S_female_1",
			State:          "Success",
		}},
	}}
	resyncResp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncVolcTenantVoices(resync) error = %v", err)
	}
	resynced, ok := resyncResp.(adminhttp.SyncVolcTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncVolcTenantVoices(resync) response = %#v", resyncResp)
	}
	if resynced.CreatedCount != 0 || resynced.UpdatedCount != 2 || resynced.DeletedCount != 1 {
		t.Fatalf("SyncVolcTenantVoices(resync) result = %#v", resynced)
	}

	deleteResp, err := srv.DeleteVolcTenant(ctx, adminhttp.DeleteVolcTenantRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("DeleteVolcTenant() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteVolcTenant200JSONResponse); !ok {
		t.Fatalf("DeleteVolcTenant() response = %#v", deleteResp)
	}
	if _, err := getVoice(ctx, srv.VoiceStore, "volc-tenant:tenant-a:S_female_1"); err != kv.ErrNotFound {
		t.Fatalf("getVoice() after volc tenant delete err = %v, want kv.ErrNotFound", err)
	}
}

func TestServerVolcTenantPutGetAndValidation(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	invalidBody := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "missing-credential",
		Endpoint:       stringPtr("not-a-url"),
	}
	invalidResp, err := srv.PutVolcTenant(ctx, adminhttp.PutVolcTenantRequestObject{Name: "tenant-a", Body: &invalidBody})
	if err != nil {
		t.Fatalf("PutVolcTenant(invalid) error = %v", err)
	}
	if _, ok := invalidResp.(adminhttp.PutVolcTenant400JSONResponse); !ok {
		t.Fatalf("PutVolcTenant(invalid) response = %#v, want 400", invalidResp)
	}

	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-main",
		Provider:  "volc",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	resourceIDs := []string{" seed-tts-2.0 ", "", "seed-tts-2.0", "seed-icl-2.0"}
	body := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "volc-main",
		Endpoint:       stringPtr("https://speech.example.com/"),
		Region:         stringPtr(" cn-beijing "),
		ResourceIds:    &resourceIDs,
		Description:    stringPtr(" primary "),
	}
	putResp, err := srv.PutVolcTenant(ctx, adminhttp.PutVolcTenantRequestObject{Name: "tenant-a", Body: &body})
	if err != nil {
		t.Fatalf("PutVolcTenant() error = %v", err)
	}
	put, ok := putResp.(adminhttp.PutVolcTenant200JSONResponse)
	if !ok {
		t.Fatalf("PutVolcTenant() response = %#v", putResp)
	}
	if put.Endpoint == nil || *put.Endpoint != "https://speech.example.com/" || put.Region == nil || *put.Region != "cn-beijing" {
		t.Fatalf("PutVolcTenant() tenant = %#v", put)
	}
	if put.ResourceIds == nil || !slices.Equal(*put.ResourceIds, []string{"seed-tts-2.0", "seed-icl-2.0"}) {
		t.Fatalf("PutVolcTenant() resource_ids = %#v", put.ResourceIds)
	}

	getResp, err := srv.GetVolcTenant(ctx, adminhttp.GetVolcTenantRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("GetVolcTenant() error = %v", err)
	}
	got, ok := getResp.(adminhttp.GetVolcTenant200JSONResponse)
	if !ok || got.CredentialName != "volc-main" {
		t.Fatalf("GetVolcTenant() response = %#v", getResp)
	}
}

func TestServerVolcTenantErrorResponses(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	if resp, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{}); err != nil {
		t.Fatalf("CreateVolcTenant(nil body) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateVolcTenant400JSONResponse); !ok {
		t.Fatalf("CreateVolcTenant(nil body) response = %#v, want 400", resp)
	}
	if resp, err := srv.GetVolcTenant(ctx, adminhttp.GetVolcTenantRequestObject{Name: "missing"}); err != nil {
		t.Fatalf("GetVolcTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.GetVolcTenant404JSONResponse); !ok {
		t.Fatalf("GetVolcTenant(missing) response = %#v, want 404", resp)
	}
	if resp, err := srv.DeleteVolcTenant(ctx, adminhttp.DeleteVolcTenantRequestObject{Name: "missing"}); err != nil {
		t.Fatalf("DeleteVolcTenant(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteVolcTenant404JSONResponse); !ok {
		t.Fatalf("DeleteVolcTenant(missing) response = %#v, want 404", resp)
	}

	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-main",
		Provider:  "volc",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	createBody := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "volc-main",
	}
	if _, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateVolcTenant() error = %v", err)
	}
	if resp, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateVolcTenant(duplicate) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateVolcTenant409JSONResponse); !ok {
		t.Fatalf("CreateVolcTenant(duplicate) response = %#v, want 409", resp)
	}
	mismatchBody := createBody
	mismatchBody.Name = "other"
	if resp, err := srv.PutVolcTenant(ctx, adminhttp.PutVolcTenantRequestObject{Name: "tenant-a", Body: &mismatchBody}); err != nil {
		t.Fatalf("PutVolcTenant(mismatch) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutVolcTenant400JSONResponse); !ok {
		t.Fatalf("PutVolcTenant(mismatch) response = %#v, want 400", resp)
	}
	if resp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "missing"}); err != nil {
		t.Fatalf("SyncVolcTenantVoices(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.SyncVolcTenantVoices404JSONResponse); !ok {
		t.Fatalf("SyncVolcTenantVoices(missing) response = %#v, want 404", resp)
	}

	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return nil, errors.New("factory rejected")
	}
	if resp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"}); err != nil {
		t.Fatalf("SyncVolcTenantVoices(factory error) error = %v", err)
	} else if _, ok := resp.(adminhttp.SyncVolcTenantVoices400JSONResponse); !ok {
		t.Fatalf("SyncVolcTenantVoices(factory error) response = %#v, want 400", resp)
	}

	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return &fakeVolcSpeakerClient{speakersErr: errors.New("speakers unavailable"), timbresErr: errors.New("timbres unavailable")}, nil
	}
	if resp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"}); err != nil {
		t.Fatalf("SyncVolcTenantVoices(upstream error) error = %v", err)
	} else if _, ok := resp.(adminhttp.SyncVolcTenantVoices502JSONResponse); !ok {
		t.Fatalf("SyncVolcTenantVoices(upstream error) response = %#v, want 502", resp)
	}

	resourceIDs := []string{"seed-tts-2.0"}
	updateBody := createBody
	updateBody.ResourceIds = &resourceIDs
	if _, err := srv.PutVolcTenant(ctx, adminhttp.PutVolcTenantRequestObject{Name: "tenant-a", Body: &updateBody}); err != nil {
		t.Fatalf("PutVolcTenant(resource ids) error = %v", err)
	}
	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return &fakeVolcSpeakerClient{trainStatusErr: errors.New("train status unavailable")}, nil
	}
	if resp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"}); err != nil {
		t.Fatalf("SyncVolcTenantVoices(train status error) error = %v", err)
	} else if _, ok := resp.(adminhttp.SyncVolcTenantVoices502JSONResponse); !ok {
		t.Fatalf("SyncVolcTenantVoices(train status error) response = %#v, want 502", resp)
	}

	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return &fakeVolcSpeakerClient{pages: []*volcMegaTTSTrainStatusPage{{
			PageNumber: 1,
			PageSize:   100,
			TotalCount: 1,
			Statuses:   []volcSpeakerStatus{{ResourceID: "seed-tts-2.0"}},
		}}}, nil
	}
	if resp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"}); err != nil {
		t.Fatalf("SyncVolcTenantVoices(missing speaker id) error = %v", err)
	} else if _, ok := resp.(adminhttp.SyncVolcTenantVoices502JSONResponse); !ok {
		t.Fatalf("SyncVolcTenantVoices(missing speaker id) response = %#v, want 502", resp)
	}
}

func TestServerVolcSyncPublicOnlySkipsTrainStatusAPI(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-main",
		Provider:  "volcengine",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	fakeClient := &fakeVolcSpeakerClient{
		speakers: []volcSpeaker{
			{VoiceType: "public-a", Name: "Public A", ResourceID: "seed-tts-2.0", Raw: map[string]interface{}{"VoiceType": "public-a", "ResourceID": "seed-tts-2.0"}},
			{VoiceType: "public-b", Name: "Public B", ResourceID: "seed-icl-2.0", Raw: map[string]interface{}{"VoiceType": "public-b", "ResourceID": "seed-icl-2.0"}},
		},
	}
	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return fakeClient, nil
	}
	createBody := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "volc-main",
	}
	if _, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateVolcTenant() error = %v", err)
	}

	syncResp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncVolcTenantVoices() error = %v", err)
	}
	synced, ok := syncResp.(adminhttp.SyncVolcTenantVoices200JSONResponse)
	if !ok {
		t.Fatalf("SyncVolcTenantVoices() response = %#v", syncResp)
	}
	if synced.CreatedCount != 2 || synced.UpdatedCount != 0 || synced.DeletedCount != 0 {
		t.Fatalf("SyncVolcTenantVoices() result = %#v", synced)
	}
	if len(fakeClient.requestedResourceIDs) != 0 {
		t.Fatalf("BatchListMegaTTSTrainStatus requests = %#v, want none", fakeClient.requestedResourceIDs)
	}
	voice := requireStoredVoice(t, srv, ctx, "volc-tenant:tenant-a:public-a")
	if voiceProviderDataString(voice, "resource_id") != "seed-tts-2.0" {
		t.Fatalf("resource_id = %q, want seed-tts-2.0", voiceProviderDataString(voice, "resource_id"))
	}
}

func TestServerVolcSyncTimbreFallbackMapsICLResourceID(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-main",
		Provider:  "volcengine",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	fakeClient := &fakeVolcSpeakerClient{
		speakersErr: errors.New("ListSpeakers unsupported"),
		timbres: []volcPublicTimbre{
			{SpeakerID: "ICL_en_female_cc_cm_v1_tob", Name: "Charlie", Raw: map[string]interface{}{"SpeakerID": "ICL_en_female_cc_cm_v1_tob"}},
			{SpeakerID: "zh_female_vv_uranus_bigtts", Name: "VV", Raw: map[string]interface{}{"SpeakerID": "zh_female_vv_uranus_bigtts"}},
		},
	}
	srv.VolcSpeakerClientFactory = func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error) {
		return fakeClient, nil
	}
	resourceIDs := []string{"seed-tts-1.0", "seed-tts-2.0", "seed-icl-2.0"}
	createBody := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "volc-main",
		ResourceIds:    &resourceIDs,
	}
	if _, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateVolcTenant() error = %v", err)
	}
	syncResp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncVolcTenantVoices() error = %v", err)
	}
	if synced, ok := syncResp.(adminhttp.SyncVolcTenantVoices200JSONResponse); !ok || synced.CreatedCount != 2 {
		t.Fatalf("SyncVolcTenantVoices() response = %#v", syncResp)
	}
	icl := requireStoredVoice(t, srv, ctx, "volc-tenant:tenant-a:ICL_en_female_cc_cm_v1_tob")
	if got := voiceProviderDataString(icl, "resource_id"); got != "seed-tts-1.0" {
		t.Fatalf("ICL resource_id = %q, want seed-tts-1.0", got)
	}
	tts := requireStoredVoice(t, srv, ctx, "volc-tenant:tenant-a:zh_female_vv_uranus_bigtts")
	if got := voiceProviderDataString(tts, "resource_id"); got != "seed-tts-2.0" {
		t.Fatalf("TTS resource_id = %q, want seed-tts-2.0", got)
	}
}

func TestServerVolcTenantStoreNotConfigured(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	ctx := context.Background()
	createBody := adminhttp.VolcTenantUpsert{Name: "tenant-a", CredentialName: "cred"}
	for name, call := range map[string]func() (interface{}, error){
		"ListVolcTenants": func() (interface{}, error) {
			return srv.ListVolcTenants(ctx, adminhttp.ListVolcTenantsRequestObject{})
		},
		"CreateVolcTenant": func() (interface{}, error) {
			return srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody})
		},
		"DeleteVolcTenant": func() (interface{}, error) {
			return srv.DeleteVolcTenant(ctx, adminhttp.DeleteVolcTenantRequestObject{Name: "tenant-a"})
		},
		"GetVolcTenant": func() (interface{}, error) {
			return srv.GetVolcTenant(ctx, adminhttp.GetVolcTenantRequestObject{Name: "tenant-a"})
		},
		"PutVolcTenant": func() (interface{}, error) {
			return srv.PutVolcTenant(ctx, adminhttp.PutVolcTenantRequestObject{Name: "tenant-a", Body: &createBody})
		},
		"SyncVolcTenantVoices": func() (interface{}, error) {
			return srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"})
		},
	} {
		resp, err := call()
		if err != nil {
			t.Fatalf("%s() error = %v", name, err)
		}
		switch resp.(type) {
		case adminhttp.ListVolcTenants500JSONResponse,
			adminhttp.CreateVolcTenant500JSONResponse,
			adminhttp.DeleteVolcTenant500JSONResponse,
			adminhttp.GetVolcTenant500JSONResponse,
			adminhttp.PutVolcTenant500JSONResponse,
			adminhttp.SyncVolcTenantVoices500JSONResponse:
		default:
			t.Fatalf("%s() response = %#v, want 500 response", name, resp)
		}
	}
}

func TestServerVolcSyncRejectsInvalidCredential(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-main",
		Provider:  "volc",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	createBody := adminhttp.VolcTenantUpsert{
		Name:           "tenant-a",
		CredentialName: "volc-main",
	}
	if _, err := srv.CreateVolcTenant(ctx, adminhttp.CreateVolcTenantRequestObject{Body: &createBody}); err != nil {
		t.Fatalf("CreateVolcTenant() error = %v", err)
	}
	resp, err := srv.SyncVolcTenantVoices(ctx, adminhttp.SyncVolcTenantVoicesRequestObject{Name: "tenant-a"})
	if err != nil {
		t.Fatalf("SyncVolcTenantVoices() error = %v", err)
	}
	rejected, ok := resp.(adminhttp.SyncVolcTenantVoices400JSONResponse)
	if !ok {
		t.Fatalf("SyncVolcTenantVoices() response = %#v, want 400", resp)
	}
	if !strings.Contains(rejected.Error.Message, "missing openapi_access_key_id/openapi_access_key") {
		t.Fatalf("SyncVolcTenantVoices() error = %#v", rejected.Error)
	}
}

func TestVolcCredentialAndResourceHelpers(t *testing.T) {
	t.Parallel()

	appID, ak, sk, err := volcCredentialValues(apitypes.Credential{
		Name: "volc-main",
		Body: testVolcCredentialBodyFromStrings(map[string]string{
			"speech_app_id":         " app ",
			"openapi_access_key_id": " ak ",
			"openapi_access_key":    " sk ",
		}),
	})
	if err != nil {
		t.Fatalf("volcCredentialValues() error = %v", err)
	}
	if appID != "app" || ak != "ak" || sk != "sk" {
		t.Fatalf("volcCredentialValues() = %q, %q, %q", appID, ak, sk)
	}
	if _, _, _, err := volcCredentialValues(apitypes.Credential{Name: "missing", Body: testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak"})}); err == nil {
		t.Fatal("volcCredentialValues(missing secret) error = nil")
	}

	resourceIDs := volcResourceIDStrings([]string{" seed-tts-2.0 ", "", "seed-tts-2.0", "seed-icl-2.0"})
	if !slices.Equal(resourceIDs, []string{"seed-tts-2.0", "seed-icl-2.0"}) {
		t.Fatalf("volcResourceIDStrings() = %#v", resourceIDs)
	}
	resourceIDPtrs := volcResourceIDStringPtrs([]string{" seed-tts-2.0 ", "seed-tts-2.0", "seed-icl-2.0"})
	if len(resourceIDPtrs) != 2 || *resourceIDPtrs[0] != "seed-tts-2.0" || *resourceIDPtrs[1] != "seed-icl-2.0" {
		t.Fatalf("volcResourceIDStringPtrs() = %#v", resourceIDPtrs)
	}
	for _, tt := range []struct {
		speakerID string
		want      string
	}{
		{speakerID: "S_custom_voice", want: volcPublicICLResourceID},
		{speakerID: "zh_female_vv_uranus_bigtts", want: volcPublicTTSResourceID},
		{speakerID: "saturn_voice", want: volcPublicTTSResourceID},
		{speakerID: "zh_female_vv_mars_bigtts", want: volcPublicTTSV1ResourceID},
	} {
		if got := volcResourceIDForPublicTimbre(tt.speakerID); got != tt.want {
			t.Fatalf("volcResourceIDForPublicTimbre(%q) = %q, want %q", tt.speakerID, got, tt.want)
		}
	}
	speakerName := " Speaker Name "
	if got := firstVolcTimbreSpeakerName([]*speechsaasprod.TimbreInfoForListBigModelTTSTimbresOutput{
		nil,
		{SpeakerName: &speakerName},
	}); got != "Speaker Name" {
		t.Fatalf("firstVolcTimbreSpeakerName() = %q, want Speaker Name", got)
	}
	if got := firstVolcTimbreSpeakerName(nil); got != "" {
		t.Fatalf("firstVolcTimbreSpeakerName(nil) = %q, want empty", got)
	}
	for _, tt := range []struct {
		record volcSpeakerRecord
		want   string
	}{
		{record: volcSpeakerRecord{}, want: ""},
		{record: volcSpeakerRecord{status: &volcSpeakerStatus{SpeakerID: " S_status "}}, want: "S_status"},
		{record: volcSpeakerRecord{speaker: &volcSpeaker{VoiceType: " public_voice "}}, want: "public_voice"},
		{record: volcSpeakerRecord{timbre: &volcPublicTimbre{SpeakerID: " timbre_voice "}}, want: "timbre_voice"},
	} {
		if got := tt.record.providerVoiceID(); got != tt.want {
			t.Fatalf("providerVoiceID() = %q, want %q", got, tt.want)
		}
	}
	region := volcRegion(apitypes.VolcTenant{Region: stringPtr(" cn-shanghai ")})
	if region != "cn-shanghai" {
		t.Fatalf("volcRegion() = %q", region)
	}
	if got := stringValue(nil); got != "" {
		t.Fatalf("stringValue(nil) = %q", got)
	}
	if volcRegion(apitypes.VolcTenant{}) != defaultVolcRegion {
		t.Fatalf("volcRegion(empty) = %q", volcRegion(apitypes.VolcTenant{}))
	}
	raw := rawStructToMap(struct {
		SpeakerID string
	}{SpeakerID: "speaker-a"})
	if raw == nil || (*raw)["SpeakerID"] != "speaker-a" {
		t.Fatalf("rawStructToMap() = %#v", raw)
	}
	if rawStructToMap(func() {}) != nil {
		t.Fatal("rawStructToMap(func) should return nil")
	}
	if rawStructToMap(struct{}{}) != nil {
		t.Fatal("rawStructToMap(empty struct) should return nil")
	}
	if !equalStringPtr(nil, nil) {
		t.Fatal("equalStringPtr(nil, nil) = false")
	}
	if equalStringPtr(nil, stringPtr("value")) {
		t.Fatal("equalStringPtr(nil, value) = true")
	}
	if !equalStringPtr(stringPtr("value"), stringPtr("value")) {
		t.Fatal("equalStringPtr(equal values) = false")
	}
	if equalStringPtr(stringPtr("value"), stringPtr("other")) {
		t.Fatal("equalStringPtr(different values) = true")
	}
	if cloneMap(nil) != nil {
		t.Fatal("cloneMap(nil) should return nil")
	}
	originalMap := map[string]interface{}{"name": "voice-a"}
	clonedMap := cloneMap(&originalMap)
	originalMap["name"] = "voice-b"
	if clonedMap == nil || (*clonedMap)["name"] != "voice-a" {
		t.Fatalf("cloneMap() = %#v", clonedMap)
	}
	if cloneVoiceProviderData(nil) != nil {
		t.Fatal("cloneVoiceProviderData(nil) should return nil")
	}
	originalProvider := apitypes.VoiceProviderData{}
	voiceA := "voice-a"
	if err := originalProvider.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{VoiceId: &voiceA}); err != nil {
		t.Fatalf("FromMiniMaxTenantVoiceProviderData() error = %v", err)
	}
	clonedProvider := cloneVoiceProviderData(&originalProvider)
	voiceB := "voice-b"
	if err := originalProvider.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{VoiceId: &voiceB}); err != nil {
		t.Fatalf("FromMiniMaxTenantVoiceProviderData() error = %v", err)
	}
	clonedData, err := clonedProvider.AsMiniMaxTenantVoiceProviderData()
	if err != nil {
		t.Fatalf("cloneVoiceProviderData() = %#v", clonedProvider)
	}
	if clonedProvider == nil || clonedData.VoiceId == nil || *clonedData.VoiceId != "voice-a" {
		t.Fatalf("cloneVoiceProviderData() = %#v", clonedProvider)
	}
}

func TestTenantReferenceValidation(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	credentialStore, err := srv.credentialStore()
	if err != nil {
		t.Fatalf("credentialStore() error = %v", err)
	}
	miniMaxTenant := apitypes.MiniMaxTenant{Name: "mmx", CredentialName: "mmx-cred"}
	volcTenant := apitypes.VolcTenant{Name: "volc", CredentialName: "volc-cred"}

	if err := validateTenantReferences(ctx, credentialStore, miniMaxTenant); err == nil || !strings.Contains(err.Error(), `credential "mmx-cred" not found`) {
		t.Fatalf("validateTenantReferences(missing) error = %v", err)
	}
	if err := validateVolcTenantReferences(ctx, credentialStore, volcTenant); err == nil || !strings.Contains(err.Error(), `credential "volc-cred" not found`) {
		t.Fatalf("validateVolcTenantReferences(missing) error = %v", err)
	}

	seedCredential(t, srv, apitypes.Credential{
		Name:      "mmx-cred",
		Provider:  "minimax",
		Body:      testMiniMaxCredentialBody("mmx-key"),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	seedCredential(t, srv, apitypes.Credential{
		Name:      "volc-cred",
		Provider:  "volc",
		Body:      testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
		CreatedAt: srv.now(),
		UpdatedAt: srv.now(),
	})
	if err := validateTenantReferences(ctx, credentialStore, miniMaxTenant); err != nil {
		t.Fatalf("validateTenantReferences() error = %v", err)
	}
	if err := validateVolcTenantReferences(ctx, credentialStore, volcTenant); err != nil {
		t.Fatalf("validateVolcTenantReferences() error = %v", err)
	}
}

func TestVolcSpeakerClientForTenantValidation(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	tenant := apitypes.VolcTenant{
		CredentialName: "volc-main",
		Name:           "tenant-a",
		Region:         stringPtr("cn-beijing"),
	}
	if _, err := srv.volcSpeakerClientForTenant(ctx, apitypes.Credential{
		Name:     "wrong-provider",
		Provider: "minimax",
		Body:     testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
	}, tenant); err == nil || !strings.Contains(err.Error(), "provider must be volcengine") {
		t.Fatalf("volcSpeakerClientForTenant(wrong provider) error = %v", err)
	}
	if _, err := srv.volcSpeakerClientForTenant(ctx, apitypes.Credential{
		Name:     "missing-secret",
		Provider: "volc",
		Body:     testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak"}),
	}, tenant); err == nil || !strings.Contains(err.Error(), "missing openapi_access_key_id/openapi_access_key") {
		t.Fatalf("volcSpeakerClientForTenant(missing secret) error = %v", err)
	}
	client, err := srv.volcSpeakerClientForTenant(ctx, apitypes.Credential{
		Name:     "volc-main",
		Provider: "volcengine",
		Body:     testVolcCredentialBodyFromStrings(map[string]string{"speech_app_id": "app", "openapi_access_key_id": "ak", "openapi_access_key": "sk"}),
	}, tenant)
	if err != nil {
		t.Fatalf("volcSpeakerClientForTenant() error = %v", err)
	}
	if client == nil {
		t.Fatal("volcSpeakerClientForTenant() returned nil client")
	}
}

func TestVolcTrainStatusPageCaptureRawStatusesFallback(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"SpeakerID":"existing"}`)
	alreadyCaptured := volcMegaTTSTrainStatusPage{
		Statuses:    []volcSpeakerStatus{{SpeakerID: "existing"}},
		rawStatuses: []json.RawMessage{raw},
	}
	if err := alreadyCaptured.captureRawStatuses(); err != nil {
		t.Fatalf("captureRawStatuses(already captured) error = %v", err)
	}
	if string(alreadyCaptured.rawStatuses[0]) != string(raw) {
		t.Fatalf("raw status changed = %s", alreadyCaptured.rawStatuses[0])
	}

	page := volcMegaTTSTrainStatusPage{
		Statuses: []volcSpeakerStatus{{
			Alias:      "Voice",
			ResourceID: "seed-tts-2.0",
			SpeakerID:  "S_voice_1",
			State:      "Success",
		}},
	}
	if err := page.captureRawStatuses(); err != nil {
		t.Fatalf("captureRawStatuses() error = %v", err)
	}
	if got := page.Statuses[0].raw["SpeakerID"]; got != "S_voice_1" {
		t.Fatalf("raw SpeakerID = %#v", got)
	}
}

func TestVolcMegaTTSTrainStatusPagePreservesRawStatus(t *testing.T) {
	t.Parallel()

	var page volcMegaTTSTrainStatusPage
	if err := json.Unmarshal([]byte(`{
		"AppID": "9476442538",
		"TotalCount": 1,
		"Statuses": [{
			"Alias": "小茧",
			"DemoAudio": null,
			"ModelTypeDetails": [{"IclSpeakerId": "icl-1", "ResourceID": "seed-icl-2.0"}],
			"ResourceID": "seed-icl-2.0",
			"SpeakerID": "S_voice_1",
			"State": "Success"
		}]
	}`), &page); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(page.Statuses) != 1 {
		t.Fatalf("len(Statuses) = %d, want 1", len(page.Statuses))
	}
	status := page.Statuses[0]
	if status.SpeakerID != "S_voice_1" || status.ResourceID != "seed-icl-2.0" {
		t.Fatalf("status = %#v", status)
	}
	if status.raw["DemoAudio"] != nil {
		t.Fatalf("raw DemoAudio = %#v, want nil", status.raw["DemoAudio"])
	}
}

type fakeVolcSpeakerClient struct {
	speakers             []volcSpeaker
	speakersErr          error
	timbres              []volcPublicTimbre
	timbresErr           error
	pages                []*volcMegaTTSTrainStatusPage
	trainStatusErr       error
	requestedResourceIDs [][]string
}

func (f *fakeVolcSpeakerClient) ListSpeakersWithContext(_ context.Context, _ []string, pageNumber, pageSize int32) (*volcSpeakersPage, error) {
	if f.speakersErr != nil {
		return nil, f.speakersErr
	}
	if pageNumber > 1 {
		return &volcSpeakersPage{PageNumber: pageNumber, PageSize: pageSize, Total: int32(len(f.speakers))}, nil
	}
	return &volcSpeakersPage{
		PageNumber: pageNumber,
		PageSize:   pageSize,
		Speakers:   append([]volcSpeaker(nil), f.speakers...),
		Total:      int32(len(f.speakers)),
	}, nil
}

func (f *fakeVolcSpeakerClient) ListBigModelTTSTimbresWithContext(context.Context) ([]volcPublicTimbre, error) {
	if f.timbresErr != nil {
		return nil, f.timbresErr
	}
	return append([]volcPublicTimbre(nil), f.timbres...), nil
}

func (f *fakeVolcSpeakerClient) BatchListMegaTTSTrainStatusWithContext(_ context.Context, _ string, resourceIDs []string, pageNumber, _ int32) (*volcMegaTTSTrainStatusPage, error) {
	if f.trainStatusErr != nil {
		return nil, f.trainStatusErr
	}
	f.requestedResourceIDs = append(f.requestedResourceIDs, append([]string(nil), resourceIDs...))
	index := int(pageNumber - 1)
	if index < 0 || index >= len(f.pages) {
		return &volcMegaTTSTrainStatusPage{PageNumber: pageNumber, PageSize: 100}, nil
	}
	return f.pages[index], nil
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	fixed := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	return &Server{
		TenantStore:     kv.Prefixed(store, kv.Key{"minimax-tenants"}),
		VoiceStore:      kv.Prefixed(store, kv.Key{"voices"}),
		CredentialStore: kv.Prefixed(store, kv.Key{"credentials"}),
		Now: func() time.Time {
			return fixed
		},
	}
}

func requireStoredVoice(t *testing.T, srv *Server, ctx context.Context, id string) apitypes.Voice {
	t.Helper()

	store, err := srv.voiceStore()
	if err != nil {
		t.Fatalf("voiceStore() error = %v", err)
	}
	voice, err := getVoice(ctx, store, id)
	if err != nil {
		t.Fatalf("getVoice(%s) error = %v", id, err)
	}
	return voice
}

func requireMissingVoice(t *testing.T, srv *Server, ctx context.Context, id string) {
	t.Helper()

	store, err := srv.voiceStore()
	if err != nil {
		t.Fatalf("voiceStore() error = %v", err)
	}
	if _, err := getVoice(ctx, store, id); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("getVoice(%s) err = %v, want kv.ErrNotFound", id, err)
	}
}

func seedCredential(t *testing.T, srv *Server, credential apitypes.Credential) {
	t.Helper()

	data, err := json.Marshal(credential)
	if err != nil {
		t.Fatalf("json.Marshal(credential) error = %v", err)
	}
	store, err := srv.credentialStore()
	if err != nil {
		t.Fatalf("credentialStore() error = %v", err)
	}
	if err := store.Set(context.Background(), credentialKey(string(credential.Name)), data); err != nil {
		t.Fatalf("Store.Set(credential) error = %v", err)
	}
}

func mustMiniMaxTenantUpsert(t *testing.T, raw string) adminhttp.MiniMaxTenantUpsert {
	t.Helper()

	var upsert adminhttp.MiniMaxTenantUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return upsert
}

func mustVoiceUpsert(t *testing.T, raw string) adminhttp.VoiceUpsert {
	t.Helper()

	var upsert adminhttp.VoiceUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return upsert
}

func stringPtr(value string) *string {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
