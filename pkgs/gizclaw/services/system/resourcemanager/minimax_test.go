package resourcemanager

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestApplyMiniMaxTenantCreatesResource(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "MiniMaxTenant",
		"metadata": {"name": "main"},
		"spec": {
			"app_id": "app",
			"group_id": "group",
			"credential_name": "minimax-main",
			"description": "primary tenant"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("action = %q, want created", result.Action)
	}
	if minimax.putTenantCount != 1 {
		t.Fatalf("putTenantCount = %d, want 1", minimax.putTenantCount)
	}
	if minimax.tenants["main"].CredentialName != "minimax-main" {
		t.Fatalf("credential name = %q, want minimax-main", minimax.tenants["main"].CredentialName)
	}
}

func TestApplyVolcTenantCreatesResource(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "VolcTenant",
		"metadata": {"name": "volc-main"},
		"spec": {
			"credential_name": "volc-cred",
			"region": "cn-beijing",
			"resource_ids": ["seed-tts-2.0"],
			"description": "primary tenant"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("action = %q, want created", result.Action)
	}
	if minimax.putVolcCount != 1 {
		t.Fatalf("putVolcCount = %d, want 1", minimax.putVolcCount)
	}
	if minimax.volcTenants["volc-main"].CredentialName != "volc-cred" {
		t.Fatalf("credential name = %q, want volc-cred", minimax.volcTenants["volc-main"].CredentialName)
	}
}

func TestGetVolcTenantReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.volcTenants["volc-main"] = apitypes.VolcTenant{
		CreatedAt:      time.Now().UTC(),
		CredentialName: "volc-cred",
		Name:           "volc-main",
		Region:         stringPtr("cn-shanghai"),
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindVolcTenant, "volc-main")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	tenant, err := resource.AsVolcTenantResource()
	if err != nil {
		t.Fatalf("AsVolcTenantResource returned error: %v", err)
	}
	if tenant.Metadata.Name != "volc-main" {
		t.Fatalf("metadata.name = %q, want volc-main", tenant.Metadata.Name)
	}
	if tenant.Spec.CredentialName != "volc-cred" {
		t.Fatalf("credential_name = %q, want volc-cred", tenant.Spec.CredentialName)
	}
}

func TestApplyVolcTenantUnchangedSkipsPut(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.volcTenants["volc-main"] = apitypes.VolcTenant{
		CreatedAt:      time.Now().UTC(),
		CredentialName: "volc-cred",
		Name:           "volc-main",
		Region:         stringPtr("cn-shanghai"),
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "VolcTenant",
		"metadata": {"name": "volc-main"},
		"spec": {
			"credential_name": "volc-cred",
			"region": "cn-shanghai"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if minimax.putVolcCount != 0 {
		t.Fatalf("putVolcCount = %d, want 0", minimax.putVolcCount)
	}
}

func TestApplyVolcTenantUpdatesResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.volcTenants["volc-main"] = apitypes.VolcTenant{
		CreatedAt:      time.Now().UTC(),
		CredentialName: "volc-cred",
		Name:           "volc-main",
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "VolcTenant",
		"metadata": {"name": "volc-main"},
		"spec": {
			"credential_name": "volc-cred",
			"region": "cn-shanghai"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if minimax.putVolcCount != 1 {
		t.Fatalf("putVolcCount = %d, want 1", minimax.putVolcCount)
	}
}

func TestPutVolcTenantWritesAndReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "VolcTenant",
		"metadata": {"name": "volc-main"},
		"spec": {
			"credential_name": "volc-cred"
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if minimax.putVolcCount != 1 {
		t.Fatalf("putVolcCount = %d, want 1", minimax.putVolcCount)
	}
	tenant, err := resource.AsVolcTenantResource()
	if err != nil {
		t.Fatalf("AsVolcTenantResource returned error: %v", err)
	}
	if tenant.Metadata.Name != "volc-main" {
		t.Fatalf("metadata.name = %q, want volc-main", tenant.Metadata.Name)
	}
}

func TestDeleteVolcTenantDeletesAndReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.volcTenants["volc-main"] = apitypes.VolcTenant{
		CreatedAt:      time.Now().UTC(),
		CredentialName: "volc-cred",
		Name:           "volc-main",
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Delete(context.Background(), apitypes.ResourceKindVolcTenant, "volc-main")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	tenant, err := resource.AsVolcTenantResource()
	if err != nil {
		t.Fatalf("AsVolcTenantResource returned error: %v", err)
	}
	if tenant.Metadata.Name != "volc-main" {
		t.Fatalf("metadata.name = %q, want volc-main", tenant.Metadata.Name)
	}
	if _, ok := minimax.volcTenants["volc-main"]; ok {
		t.Fatal("volc tenant still exists after Delete")
	}
}

func TestDeleteMiniMaxTenantDeletesAndReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.tenants["main"] = apitypes.MiniMaxTenant{
		AppId:          "new-app",
		CreatedAt:      time.Now().UTC(),
		CredentialName: "minimax-main",
		GroupId:        "group",
		Name:           "main",
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Delete(context.Background(), apitypes.ResourceKindMiniMaxTenant, "main")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	tenant, err := resource.AsMiniMaxTenantResource()
	if err != nil {
		t.Fatalf("AsMiniMaxTenantResource returned error: %v", err)
	}
	if tenant.Metadata.Name != "main" {
		t.Fatalf("metadata.name = %q, want main", tenant.Metadata.Name)
	}
	if _, ok := minimax.tenants["main"]; ok {
		t.Fatal("minimax tenant still exists after Delete")
	}
}

func TestGetMiniMaxTenantReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.tenants["main"] = apitypes.MiniMaxTenant{
		AppId:          "old-app",
		CreatedAt:      time.Now().UTC(),
		CredentialName: "minimax-main",
		GroupId:        "group",
		Name:           "main",
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Get(context.Background(), apitypes.ResourceKindMiniMaxTenant, "main")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	tenant, err := resource.AsMiniMaxTenantResource()
	if err != nil {
		t.Fatalf("AsMiniMaxTenantResource returned error: %v", err)
	}
	if tenant.Metadata.Name != "main" {
		t.Fatalf("metadata.name = %q, want main", tenant.Metadata.Name)
	}
	if tenant.Spec.CredentialName != "minimax-main" {
		t.Fatalf("credential_name = %q, want minimax-main", tenant.Spec.CredentialName)
	}
}

func TestApplyMiniMaxTenantUnchangedSkipsPut(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.tenants["main"] = apitypes.MiniMaxTenant{
		AppId:          "new-app",
		CreatedAt:      time.Now().UTC(),
		CredentialName: "minimax-main",
		GroupId:        "group",
		Name:           "main",
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "MiniMaxTenant",
		"metadata": {"name": "main"},
		"spec": {
			"app_id": "new-app",
			"group_id": "group",
			"credential_name": "minimax-main"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if minimax.putTenantCount != 0 {
		t.Fatalf("putTenantCount = %d, want 0", minimax.putTenantCount)
	}
}

func TestApplyMiniMaxTenantUpdatesResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.tenants["main"] = apitypes.MiniMaxTenant{
		CreatedAt:      time.Now().UTC(),
		CredentialName: "minimax-main",
		GroupId:        "group",
		Name:           "main",
		UpdatedAt:      time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "MiniMaxTenant",
		"metadata": {"name": "main"},
		"spec": {
			"app_id": "app",
			"group_id": "group",
			"credential_name": "minimax-main"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if minimax.putTenantCount != 1 {
		t.Fatalf("putTenantCount = %d, want 1", minimax.putTenantCount)
	}
}

func TestApplyVoiceUpdatesResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.voices["voice-1"] = apitypes.Voice{
		CreatedAt: time.Now().UTC(),
		Id:        "voice-1",
		Name:      ptr("Old"),
		Provider:  apitypes.VoiceProvider{Kind: "minimax", Name: "main"},
		Source:    apitypes.VoiceSourceManual,
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Voice",
		"metadata": {"name": "voice-1"},
		"spec": {
			"name": "New",
			"provider": {"kind": "minimax", "name": "main"},
			"source": "manual"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("action = %q, want updated", result.Action)
	}
	if minimax.putVoiceCount != 1 {
		t.Fatalf("putVoiceCount = %d, want 1", minimax.putVoiceCount)
	}
}

func TestApplyVoiceCreatesResource(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Voice",
		"metadata": {"name": "voice-1"},
		"spec": {
			"name": "Narrator",
			"provider": {"kind": "minimax", "name": "main"},
			"source": "manual"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("action = %q, want created", result.Action)
	}
	if minimax.putVoiceCount != 1 {
		t.Fatalf("putVoiceCount = %d, want 1", minimax.putVoiceCount)
	}
}

func TestApplyVoiceUnchangedSkipsPut(t *testing.T) {
	name := "Narrator"
	minimax := newFakeMiniMax()
	minimax.voices["voice-1"] = apitypes.Voice{
		CreatedAt: time.Now().UTC(),
		Id:        "voice-1",
		Name:      &name,
		Provider:  apitypes.VoiceProvider{Kind: "minimax", Name: "main"},
		Source:    apitypes.VoiceSourceManual,
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	result, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Voice",
		"metadata": {"name": "voice-1"},
		"spec": {
			"name": "Narrator",
			"provider": {"kind": "minimax", "name": "main"},
			"source": "manual"
		}
	}`))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("action = %q, want unchanged", result.Action)
	}
	if minimax.putVoiceCount != 0 {
		t.Fatalf("putVoiceCount = %d, want 0", minimax.putVoiceCount)
	}
}

func TestPutVoiceWritesAndReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Voice",
		"metadata": {"name": "voice-1"},
		"spec": {
			"name": "Narrator",
			"provider": {"kind": "minimax", "name": "main"},
			"source": "manual"
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if minimax.putVoiceCount != 1 {
		t.Fatalf("putVoiceCount = %d, want 1", minimax.putVoiceCount)
	}
	voice, err := resource.AsVoiceResource()
	if err != nil {
		t.Fatalf("AsVoiceResource returned error: %v", err)
	}
	if voice.Metadata.Name != "voice-1" {
		t.Fatalf("metadata.name = %q, want voice-1", voice.Metadata.Name)
	}
	if voice.Spec.Provider.Name != "main" {
		t.Fatalf("provider.name = %q, want main", voice.Spec.Provider.Name)
	}
}

func TestDeleteVoiceDeletesAndReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	minimax.voices["voice-1"] = apitypes.Voice{
		CreatedAt: time.Now().UTC(),
		Id:        "voice-1",
		Provider:  apitypes.VoiceProvider{Kind: "local", Name: "manual"},
		Source:    apitypes.VoiceSourceManual,
		UpdatedAt: time.Now().UTC(),
	}
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Delete(context.Background(), apitypes.ResourceKindVoice, "voice-1")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	voice, err := resource.AsVoiceResource()
	if err != nil {
		t.Fatalf("AsVoiceResource returned error: %v", err)
	}
	if voice.Metadata.Name != "voice-1" {
		t.Fatalf("metadata.name = %q, want voice-1", voice.Metadata.Name)
	}
	if _, ok := minimax.voices["voice-1"]; ok {
		t.Fatal("voice still exists after Delete")
	}
}

func TestPutMiniMaxTenantWritesAndReturnsResource(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	resource, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "MiniMaxTenant",
		"metadata": {"name": "main"},
		"spec": {
			"group_id": "group",
			"credential_name": "minimax-main"
		}
	}`))
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	tenant, err := resource.AsMiniMaxTenantResource()
	if err != nil {
		t.Fatalf("AsMiniMaxTenantResource returned error: %v", err)
	}
	if tenant.Metadata.Name != "main" {
		t.Fatalf("metadata.name = %q, want main", tenant.Metadata.Name)
	}
}

func TestMiniMaxServiceErrorResponses(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	minimax.getTenantStatus = 500
	_, _, err := manager.getMiniMaxTenant(context.Background(), "main")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	minimax.getTenantStatus = 0
	minimax.putTenantStatus = 400
	err = manager.putMiniMaxTenant(context.Background(), "main", adminhttp.MiniMaxTenantUpsert{})
	assertResourceError(t, err, 400, "INVALID_MINIMAX_TENANT")

	minimax.putTenantStatus = 500
	err = manager.putMiniMaxTenant(context.Background(), "main", adminhttp.MiniMaxTenantUpsert{})
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	minimax.getVoiceStatus = 500
	_, _, err = manager.getVoice(context.Background(), "voice")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	minimax.getVoiceStatus = 0
	for _, tc := range []struct {
		status int
		code   string
	}{
		{status: 400, code: "INVALID_VOICE"},
		{status: 409, code: "VOICE_CONFLICT"},
		{status: 500, code: "INTERNAL_ERROR"},
	} {
		minimax.putVoiceStatus = tc.status
		err = manager.putVoice(context.Background(), "voice", adminhttp.VoiceUpsert{})
		assertResourceError(t, err, tc.status, tc.code)
	}
}

func TestVolcAndVoiceServiceDeleteErrors(t *testing.T) {
	minimax := newFakeMiniMax()
	manager := New(Services{ProviderTenants: minimax, Voices: minimax})

	_, exists, err := manager.deleteVolcTenant(context.Background(), "missing")
	if err != nil {
		t.Fatalf("deleteVolcTenant(missing) error = %v", err)
	}
	if exists {
		t.Fatal("deleteVolcTenant(missing) exists = true")
	}

	_, exists, err = manager.deleteVoice(context.Background(), "missing")
	if err != nil {
		t.Fatalf("deleteVoice(missing) error = %v", err)
	}
	if exists {
		t.Fatal("deleteVoice(missing) exists = true")
	}

	minimax.getTenantStatus = 500
	_, _, err = manager.getVolcTenant(context.Background(), "volc-main")
	assertResourceError(t, err, 500, "INTERNAL_ERROR")

	minimax.getTenantStatus = 0
	for _, tc := range []struct {
		status int
		code   string
	}{
		{status: 400, code: "INVALID_VOLC_TENANT"},
		{status: 500, code: "INTERNAL_ERROR"},
	} {
		minimax.putTenantStatus = tc.status
		err = manager.putVolcTenant(context.Background(), "volc-main", adminhttp.VolcTenantUpsert{})
		assertResourceError(t, err, tc.status, tc.code)
	}
}

func TestVolcAndVoiceMissingServiceErrors(t *testing.T) {
	manager := New(Services{})

	if _, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "VolcTenant",
		"metadata": {"name": "volc-main"},
		"spec": {"app_id": "app", "credential_name": "cred"}
	}`)); err == nil {
		t.Fatal("Apply(VolcTenant) error = nil")
	}
	if _, err := manager.Get(context.Background(), apitypes.ResourceKindVolcTenant, "volc-main"); err == nil {
		t.Fatal("Get(VolcTenant) error = nil")
	}
	if _, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Voice",
		"metadata": {"name": "voice-1"},
		"spec": {"source": "manual", "provider": {"kind": "local", "name": "manual"}}
	}`)); err == nil {
		t.Fatal("Put(Voice) error = nil")
	}
	if _, err := manager.Delete(context.Background(), apitypes.ResourceKindVoice, "voice-1"); err == nil {
		t.Fatal("Delete(Voice) error = nil")
	}
}

type fakeMiniMax struct {
	tenants         map[string]apitypes.MiniMaxTenant
	volcTenants     map[string]apitypes.VolcTenant
	voices          map[string]apitypes.Voice
	putTenantCount  int
	putVolcCount    int
	putVoiceCount   int
	getTenantStatus int
	putTenantStatus int
	getVoiceStatus  int
	putVoiceStatus  int
}

func newFakeMiniMax() *fakeMiniMax {
	return &fakeMiniMax{
		tenants:     map[string]apitypes.MiniMaxTenant{},
		volcTenants: map[string]apitypes.VolcTenant{},
		voices:      map[string]apitypes.Voice{},
	}
}

func (f *fakeMiniMax) ListMiniMaxTenants(context.Context, adminhttp.ListMiniMaxTenantsRequestObject) (adminhttp.ListMiniMaxTenantsResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) CreateDashScopeTenant(context.Context, adminhttp.CreateDashScopeTenantRequestObject) (adminhttp.CreateDashScopeTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) ListDashScopeTenants(context.Context, adminhttp.ListDashScopeTenantsRequestObject) (adminhttp.ListDashScopeTenantsResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) DeleteDashScopeTenant(context.Context, adminhttp.DeleteDashScopeTenantRequestObject) (adminhttp.DeleteDashScopeTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) PutDashScopeTenant(context.Context, adminhttp.PutDashScopeTenantRequestObject) (adminhttp.PutDashScopeTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) CreateGeminiTenant(context.Context, adminhttp.CreateGeminiTenantRequestObject) (adminhttp.CreateGeminiTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) ListGeminiTenants(context.Context, adminhttp.ListGeminiTenantsRequestObject) (adminhttp.ListGeminiTenantsResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) DeleteGeminiTenant(context.Context, adminhttp.DeleteGeminiTenantRequestObject) (adminhttp.DeleteGeminiTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) GetGeminiTenant(context.Context, adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) PutGeminiTenant(context.Context, adminhttp.PutGeminiTenantRequestObject) (adminhttp.PutGeminiTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) CreateOpenAITenant(context.Context, adminhttp.CreateOpenAITenantRequestObject) (adminhttp.CreateOpenAITenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) ListOpenAITenants(context.Context, adminhttp.ListOpenAITenantsRequestObject) (adminhttp.ListOpenAITenantsResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) DeleteOpenAITenant(context.Context, adminhttp.DeleteOpenAITenantRequestObject) (adminhttp.DeleteOpenAITenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) GetOpenAITenant(context.Context, adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) PutOpenAITenant(context.Context, adminhttp.PutOpenAITenantRequestObject) (adminhttp.PutOpenAITenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) CreateMiniMaxTenant(context.Context, adminhttp.CreateMiniMaxTenantRequestObject) (adminhttp.CreateMiniMaxTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) DeleteMiniMaxTenant(_ context.Context, request adminhttp.DeleteMiniMaxTenantRequestObject) (adminhttp.DeleteMiniMaxTenantResponseObject, error) {
	tenant, ok := f.tenants[string(request.Name)]
	if !ok {
		return adminhttp.DeleteMiniMaxTenant404JSONResponse(apitypes.NewErrorResponse("MINIMAX_TENANT_NOT_FOUND", "not found")), nil
	}
	delete(f.tenants, string(request.Name))
	return adminhttp.DeleteMiniMaxTenant200JSONResponse(tenant), nil
}

func (f *fakeMiniMax) GetMiniMaxTenant(_ context.Context, request adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	if f.getTenantStatus == 500 {
		return adminhttp.GetMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	tenant, ok := f.tenants[string(request.Name)]
	if !ok {
		return adminhttp.GetMiniMaxTenant404JSONResponse(apitypes.NewErrorResponse("MINIMAX_TENANT_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetMiniMaxTenant200JSONResponse(tenant), nil
}

func (f *fakeMiniMax) PutMiniMaxTenant(_ context.Context, request adminhttp.PutMiniMaxTenantRequestObject) (adminhttp.PutMiniMaxTenantResponseObject, error) {
	switch f.putTenantStatus {
	case 400:
		return adminhttp.PutMiniMaxTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_MINIMAX_TENANT", "invalid")), nil
	case 500:
		return adminhttp.PutMiniMaxTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	f.putTenantCount++
	body := *request.Body
	now := time.Now().UTC()
	tenant := apitypes.MiniMaxTenant{
		BaseUrl:        body.BaseUrl,
		CreatedAt:      now,
		CredentialName: body.CredentialName,
		Description:    body.Description,
		GroupId:        body.GroupId,
		Name:           body.Name,
		UpdatedAt:      now,
	}
	f.tenants[string(request.Name)] = tenant
	return adminhttp.PutMiniMaxTenant200JSONResponse(tenant), nil
}

func (f *fakeMiniMax) SyncMiniMaxTenantVoices(context.Context, adminhttp.SyncMiniMaxTenantVoicesRequestObject) (adminhttp.SyncMiniMaxTenantVoicesResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) ListVolcTenants(context.Context, adminhttp.ListVolcTenantsRequestObject) (adminhttp.ListVolcTenantsResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) CreateVolcTenant(context.Context, adminhttp.CreateVolcTenantRequestObject) (adminhttp.CreateVolcTenantResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) DeleteVolcTenant(_ context.Context, request adminhttp.DeleteVolcTenantRequestObject) (adminhttp.DeleteVolcTenantResponseObject, error) {
	tenant, ok := f.volcTenants[string(request.Name)]
	if !ok {
		return adminhttp.DeleteVolcTenant404JSONResponse(apitypes.NewErrorResponse("VOLC_TENANT_NOT_FOUND", "not found")), nil
	}
	delete(f.volcTenants, string(request.Name))
	return adminhttp.DeleteVolcTenant200JSONResponse(tenant), nil
}

func (f *fakeMiniMax) GetVolcTenant(_ context.Context, request adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	if f.getTenantStatus == 500 {
		return adminhttp.GetVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	tenant, ok := f.volcTenants[string(request.Name)]
	if !ok {
		return adminhttp.GetVolcTenant404JSONResponse(apitypes.NewErrorResponse("VOLC_TENANT_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetVolcTenant200JSONResponse(tenant), nil
}

func (f *fakeMiniMax) PutVolcTenant(_ context.Context, request adminhttp.PutVolcTenantRequestObject) (adminhttp.PutVolcTenantResponseObject, error) {
	switch f.putTenantStatus {
	case 400:
		return adminhttp.PutVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", "invalid")), nil
	case 500:
		return adminhttp.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	f.putVolcCount++
	body := *request.Body
	now := time.Now().UTC()
	tenant := apitypes.VolcTenant{
		CreatedAt:      now,
		CredentialName: body.CredentialName,
		Description:    body.Description,
		Endpoint:       body.Endpoint,
		Name:           body.Name,
		Region:         body.Region,
		ResourceIds:    body.ResourceIds,
		UpdatedAt:      now,
	}
	f.volcTenants[string(request.Name)] = tenant
	return adminhttp.PutVolcTenant200JSONResponse(tenant), nil
}

func (f *fakeMiniMax) SyncVolcTenantVoices(context.Context, adminhttp.SyncVolcTenantVoicesRequestObject) (adminhttp.SyncVolcTenantVoicesResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) CreateVoice(context.Context, adminhttp.CreateVoiceRequestObject) (adminhttp.CreateVoiceResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) ListVoices(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	return nil, nil
}

func (f *fakeMiniMax) DeleteVoice(_ context.Context, request adminhttp.DeleteVoiceRequestObject) (adminhttp.DeleteVoiceResponseObject, error) {
	voice, ok := f.voices[string(request.Id)]
	if !ok {
		return adminhttp.DeleteVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", "not found")), nil
	}
	delete(f.voices, string(request.Id))
	return adminhttp.DeleteVoice200JSONResponse(voice), nil
}

func (f *fakeMiniMax) GetVoice(_ context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	if f.getVoiceStatus == 500 {
		return adminhttp.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	voice, ok := f.voices[string(request.Id)]
	if !ok {
		return adminhttp.GetVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetVoice200JSONResponse(voice), nil
}

func (f *fakeMiniMax) PutVoice(_ context.Context, request adminhttp.PutVoiceRequestObject) (adminhttp.PutVoiceResponseObject, error) {
	switch f.putVoiceStatus {
	case 400:
		return adminhttp.PutVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", "invalid")), nil
	case 409:
		return adminhttp.PutVoice409JSONResponse(apitypes.NewErrorResponse("VOICE_CONFLICT", "conflict")), nil
	case 500:
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "failed")), nil
	}
	f.putVoiceCount++
	body := *request.Body
	now := time.Now().UTC()
	voice := apitypes.Voice{
		CreatedAt:    now,
		Description:  body.Description,
		Id:           body.Id,
		Name:         body.Name,
		Provider:     body.Provider,
		ProviderData: body.ProviderData,
		Source:       body.Source,
		UpdatedAt:    now,
	}
	f.voices[string(request.Id)] = voice
	return adminhttp.PutVoice200JSONResponse(voice), nil
}
