package voice

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerVoiceCRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	srv := &Server{
		Store: kv.NewMemory(nil),
		Now: func() time.Time {
			return now
		},
	}
	body := adminhttp.VoiceUpsert{
		Id:     "manual:voice-1",
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKind("openai-tenant"),
			Name: "main",
		},
	}
	createdResp, err := srv.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateVoice() error = %v", err)
	}
	created, ok := createdResp.(adminhttp.CreateVoice200JSONResponse)
	if !ok {
		t.Fatalf("CreateVoice() response = %#v", createdResp)
	}
	if created.CreatedAt != now || created.UpdatedAt != now {
		t.Fatalf("created timestamps = %s/%s, want %s", created.CreatedAt, created.UpdatedAt, now)
	}

	source := adminhttp.VoiceSource(apitypes.VoiceSourceManual)
	providerKind := adminhttp.VoiceProviderKind("openai-tenant")
	providerName := "main"
	listResp, err := srv.ListVoices(ctx, adminhttp.ListVoicesRequestObject{
		Params: adminhttp.ListVoicesParams{
			ProviderKind: &providerKind,
			ProviderName: &providerName,
			Source:       &source,
		},
	})
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].Id != "manual:voice-1" {
		t.Fatalf("ListVoices() items = %#v", listed.Items)
	}

	description := "updated"
	body.Description = &description
	putResp, err := srv.PutVoice(ctx, adminhttp.PutVoiceRequestObject{Id: "manual:voice-1", Body: &body})
	if err != nil {
		t.Fatalf("PutVoice() error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutVoice200JSONResponse)
	if !ok {
		t.Fatalf("PutVoice() response = %#v", putResp)
	}
	if updated.Description == nil || *updated.Description != description {
		t.Fatalf("PutVoice() description = %#v", updated.Description)
	}
	if updated.CreatedAt != now || updated.UpdatedAt != now {
		t.Fatalf("updated timestamps = %s/%s, want %s", updated.CreatedAt, updated.UpdatedAt, now)
	}

	getResp, err := srv.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: "manual:voice-1"})
	if err != nil {
		t.Fatalf("GetVoice() error = %v", err)
	}
	if _, ok := getResp.(adminhttp.GetVoice200JSONResponse); !ok {
		t.Fatalf("GetVoice() response = %#v", getResp)
	}

	deleteResp, err := srv.DeleteVoice(ctx, adminhttp.DeleteVoiceRequestObject{Id: "manual:voice-1"})
	if err != nil {
		t.Fatalf("DeleteVoice() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteVoice200JSONResponse); !ok {
		t.Fatalf("DeleteVoice() response = %#v", deleteResp)
	}
	missingResp, err := srv.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: "manual:voice-1"})
	if err != nil {
		t.Fatalf("GetVoice(missing) error = %v", err)
	}
	if _, ok := missingResp.(adminhttp.GetVoice404JSONResponse); !ok {
		t.Fatalf("GetVoice(missing) response = %#v", missingResp)
	}
}

func TestProviderDataStringAndLegacyDecode(t *testing.T) {
	kind := apitypes.VoiceProviderKindMinimaxTenant
	voice := apitypes.Voice{
		Provider:     apitypes.VoiceProvider{Kind: kind, Name: "tenant"},
		ProviderData: ProviderData(kind, map[string]any{"voice_id": " voice-1 "}),
	}
	if got := ProviderDataString(voice, "voice_id"); got != "voice-1" {
		t.Fatalf("ProviderDataString() = %q, want voice-1", got)
	}

	var decoded apitypes.Voice
	if err := Decode([]byte(`{
		"id": "provider:tenant:voice-1",
		"provider": {"kind": "minimax-tenant", "name": "tenant"},
		"source": "sync",
		"provider_voice_id": "voice-1",
		"provider_voice_type": "system"
	}`), &decoded); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if ProviderDataString(decoded, "voice_id") != "voice-1" || ProviderDataString(decoded, "voice_type") != "system" {
		t.Fatalf("decoded provider data = %#v", decoded.ProviderData)
	}
}

func TestVoiceHelperIndexesAndSemanticEquality(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	kind := apitypes.VoiceProviderKindMinimaxTenant

	if got := StableID(kind, "main", "voice-1"); got != "minimax-tenant:main:voice-1" {
		t.Fatalf("StableID() = %q", got)
	}
	if got := ProviderData(kind, map[string]any{"voice_id": " ", "nil": nil}); got != nil {
		t.Fatalf("ProviderData() = %#v, want nil", got)
	}
	raw := map[string]any{"nested": "value"}
	if got := RawMapValue(&raw); got == nil {
		t.Fatalf("RawMapValue() = nil, want map")
	}
	if got := RawMapValue(nil); got != nil {
		t.Fatalf("RawMapValue(nil) = %#v, want nil", got)
	}

	name := "Voice One"
	description := "Primary"
	providerData := ProviderData(kind, map[string]any{
		"voice_id":   customStringer(" voice-1 "),
		"voice_type": " system ",
	})
	voice := apitypes.Voice{
		Id:           "manual:voice-1",
		Name:         &name,
		Description:  &description,
		Provider:     apitypes.VoiceProvider{Kind: kind, Name: "main"},
		ProviderData: providerData,
		Source:       apitypes.VoiceSourceManual,
	}
	if got := ProviderDataString(voice, "voice_id"); got != "voice-1" {
		t.Fatalf("ProviderDataString(Stringer) = %q, want voice-1", got)
	}
	if got := ProviderDataString(voice, "voice_type"); got != "system" {
		t.Fatalf("ProviderDataString(string) = %q, want system", got)
	}
	if !SemanticEqual(voice, voice) {
		t.Fatalf("SemanticEqual(same voice) = false")
	}
	changed := voice
	changed.Description = stringPtr("Secondary")
	if SemanticEqual(voice, changed) {
		t.Fatalf("SemanticEqual(different description) = true")
	}
	changed = voice
	changed.ProviderData = ProviderData(kind, map[string]any{"voice_id": "voice-2"})
	if SemanticEqual(voice, changed) {
		t.Fatalf("SemanticEqual(different provider data) = true")
	}

	if err := Write(ctx, store, voice, nil); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	listed, err := ListProvider(ctx, store, kind, "main")
	if err != nil {
		t.Fatalf("ListProvider(main) error = %v", err)
	}
	if len(listed) != 1 || listed[0].Id != voice.Id {
		t.Fatalf("ListProvider(main) = %#v", listed)
	}

	updated := voice
	updated.Provider = apitypes.VoiceProvider{Kind: kind, Name: "secondary"}
	updated.ProviderData = ProviderData(kind, map[string]any{"voice_id": "voice-2"})
	if err := Write(ctx, store, updated, &voice); err != nil {
		t.Fatalf("Write(updated) error = %v", err)
	}
	oldProviderList, err := ListProvider(ctx, store, kind, "main")
	if err != nil {
		t.Fatalf("ListProvider(old) error = %v", err)
	}
	if len(oldProviderList) != 0 {
		t.Fatalf("ListProvider(old) = %#v, want empty after stale index cleanup", oldProviderList)
	}
	newProviderList, err := ListProvider(ctx, store, kind, "secondary")
	if err != nil {
		t.Fatalf("ListProvider(new) error = %v", err)
	}
	if len(newProviderList) != 1 || newProviderList[0].Id != updated.Id {
		t.Fatalf("ListProvider(new) = %#v", newProviderList)
	}

	if err := Delete(ctx, store, updated); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	newProviderList, err = ListProvider(ctx, store, kind, "secondary")
	if err != nil {
		t.Fatalf("ListProvider(after delete) error = %v", err)
	}
	if len(newProviderList) != 0 {
		t.Fatalf("ListProvider(after delete) = %#v, want empty", newProviderList)
	}
}

func TestServerVoiceValidationAndPagination(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	srv := &Server{Store: store, Now: func() time.Time { return now }}

	if resp, err := (*Server)(nil).ListVoices(ctx, adminhttp.ListVoicesRequestObject{}); err != nil {
		t.Fatalf("ListVoices(nil server) error = %v", err)
	} else if _, ok := resp.(adminhttp.ListVoices500JSONResponse); !ok {
		t.Fatalf("ListVoices(nil server) response = %#v", resp)
	}
	if resp, err := srv.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{}); err != nil {
		t.Fatalf("CreateVoice(nil body) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateVoice400JSONResponse); !ok {
		t.Fatalf("CreateVoice(nil body) response = %#v", resp)
	}
	if resp, err := srv.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &adminhttp.VoiceUpsert{
		Id:     "manual:bad",
		Source: apitypes.VoiceSource("bad"),
		Provider: apitypes.VoiceProvider{
			Kind: "openai-tenant",
			Name: "main",
		},
	}}); err != nil {
		t.Fatalf("CreateVoice(invalid source) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateVoice400JSONResponse); !ok {
		t.Fatalf("CreateVoice(invalid source) response = %#v", resp)
	}

	first := voiceUpsert("manual:voice-a", "main")
	second := voiceUpsert("manual:voice-b", "main")
	for _, body := range []adminhttp.VoiceUpsert{first, second} {
		if resp, err := srv.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateVoice(%s) error = %v", body.Id, err)
		} else if _, ok := resp.(adminhttp.CreateVoice200JSONResponse); !ok {
			t.Fatalf("CreateVoice(%s) response = %#v", body.Id, resp)
		}
	}
	if resp, err := srv.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &first}); err != nil {
		t.Fatalf("CreateVoice(duplicate) error = %v", err)
	} else if _, ok := resp.(adminhttp.CreateVoice409JSONResponse); !ok {
		t.Fatalf("CreateVoice(duplicate) response = %#v", resp)
	}

	limit := int32(1)
	pageResp, err := srv.ListVoices(ctx, adminhttp.ListVoicesRequestObject{
		Params: adminhttp.ListVoicesParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListVoices(page 1) error = %v", err)
	}
	page, ok := pageResp.(adminhttp.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices(page 1) response = %#v", pageResp)
	}
	if len(page.Items) != 1 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("ListVoices(page 1) = %#v", page)
	}
	pageResp, err = srv.ListVoices(ctx, adminhttp.ListVoicesRequestObject{
		Params: adminhttp.ListVoicesParams{Cursor: page.NextCursor, Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListVoices(page 2) error = %v", err)
	}
	page, ok = pageResp.(adminhttp.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices(page 2) response = %#v", pageResp)
	}
	if len(page.Items) != 1 || page.HasNext {
		t.Fatalf("ListVoices(page 2) = %#v", page)
	}

	if resp, err := srv.PutVoice(ctx, adminhttp.PutVoiceRequestObject{Id: "manual:voice-a"}); err != nil {
		t.Fatalf("PutVoice(nil body) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutVoice400JSONResponse); !ok {
		t.Fatalf("PutVoice(nil body) response = %#v", resp)
	}
	mismatch := voiceUpsert("manual:other", "main")
	if resp, err := srv.PutVoice(ctx, adminhttp.PutVoiceRequestObject{Id: "manual:voice-a", Body: &mismatch}); err != nil {
		t.Fatalf("PutVoice(id mismatch) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutVoice400JSONResponse); !ok {
		t.Fatalf("PutVoice(id mismatch) response = %#v", resp)
	}

	syncVoice := apitypes.Voice{
		Id:       "sync:voice",
		Provider: apitypes.VoiceProvider{Kind: "openai-tenant", Name: "main"},
		Source:   apitypes.VoiceSourceSync,
	}
	if err := Write(ctx, store, syncVoice, nil); err != nil {
		t.Fatalf("Write(sync voice) error = %v", err)
	}
	syncUpdate := voiceUpsert("sync:voice", "main")
	if resp, err := srv.PutVoice(ctx, adminhttp.PutVoiceRequestObject{Id: "sync:voice", Body: &syncUpdate}); err != nil {
		t.Fatalf("PutVoice(sync) error = %v", err)
	} else if _, ok := resp.(adminhttp.PutVoice409JSONResponse); !ok {
		t.Fatalf("PutVoice(sync) response = %#v", resp)
	}

	if resp, err := srv.DeleteVoice(ctx, adminhttp.DeleteVoiceRequestObject{Id: "missing"}); err != nil {
		t.Fatalf("DeleteVoice(missing) error = %v", err)
	} else if _, ok := resp.(adminhttp.DeleteVoice404JSONResponse); !ok {
		t.Fatalf("DeleteVoice(missing) response = %#v", resp)
	}
}

func TestVoiceBoundaryBranches(t *testing.T) {
	ctx := context.Background()
	store := kv.NewMemory(nil)
	kind := apitypes.VoiceProviderKind("openai-tenant")

	if got := ProviderDataString(apitypes.Voice{}, "voice_id"); got != "" {
		t.Fatalf("ProviderDataString(no data) = %q, want empty", got)
	}
	if got := ProviderDataString(apitypes.Voice{
		Provider:     apitypes.VoiceProvider{Kind: kind, Name: "main"},
		ProviderData: ProviderData(apitypes.VoiceProviderKindMinimaxTenant, map[string]any{"voice_type": "system"}),
	}, "voice_id"); got != "" {
		t.Fatalf("ProviderDataString(missing key) = %q, want empty", got)
	}

	manual := voiceUpsert("manual:with-provider-data", "main")
	manual.ProviderData = ProviderData(kind, map[string]any{"voice_id": "voice-1"})
	normalized, err := normalizeVoiceUpsert(manual, manual.Id)
	if err != nil {
		t.Fatalf("normalizeVoiceUpsert(provider data) error = %v", err)
	}
	if normalized.ProviderData == nil || ProviderDataString(normalized, "voice_id") != "voice-1" {
		t.Fatalf("normalized provider data = %#v", normalized.ProviderData)
	}
	if _, err := normalizeVoiceUpsert(adminhttp.VoiceUpsert{Id: "manual:missing-source"}, ""); err == nil {
		t.Fatalf("normalizeVoiceUpsert(missing source) error = nil, want error")
	}
	if _, err := normalizeVoiceUpsert(adminhttp.VoiceUpsert{
		Id:     "manual:missing-kind",
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Name: "main",
		},
	}, ""); err == nil {
		t.Fatalf("normalizeVoiceUpsert(missing provider kind) error = nil, want error")
	}

	syncVoice := apitypes.Voice{
		Id:       "sync:voice",
		Provider: apitypes.VoiceProvider{Kind: kind, Name: "main"},
		Source:   apitypes.VoiceSourceSync,
	}
	if !SemanticEqual(syncVoice, syncVoice) {
		t.Fatalf("SemanticEqual(sync voice) = false")
	}
	withDescription := syncVoice
	withDescription.Description = stringPtr("description")
	if SemanticEqual(syncVoice, withDescription) {
		t.Fatalf("SemanticEqual(nil/non-nil description) = true")
	}

	if err := Write(ctx, store, syncVoice, nil); err != nil {
		t.Fatalf("Write(sync voice) error = %v", err)
	}
	source := adminhttp.VoiceSource(apitypes.VoiceSourceManual)
	pageResp, err := (&Server{Store: store}).ListVoices(ctx, adminhttp.ListVoicesRequestObject{
		Params: adminhttp.ListVoicesParams{Source: &source},
	})
	if err != nil {
		t.Fatalf("ListVoices(source mismatch) error = %v", err)
	}
	page, ok := pageResp.(adminhttp.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices(source mismatch) response = %#v", pageResp)
	}
	if len(page.Items) != 0 {
		t.Fatalf("ListVoices(source mismatch) items = %#v, want empty", page.Items)
	}

	if _, err := Get(ctx, store, "missing"); err == nil {
		t.Fatalf("Get(missing) error = nil, want error")
	}
	if got := unescapeStoreSegment("%zz"); got != "%zz" {
		t.Fatalf("unescapeStoreSegment(invalid) = %q, want original", got)
	}
}

type customStringer string

func (s customStringer) String() string {
	return string(s)
}

func voiceUpsert(id, providerName string) adminhttp.VoiceUpsert {
	return adminhttp.VoiceUpsert{
		Id:     id,
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Kind: "openai-tenant",
			Name: providerName,
		},
	}
}

func stringPtr(value string) *string {
	return &value
}
