package model

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestVolcServiceTierModelCRUD(t *testing.T) {
	ctx := context.Background()
	srv := &Server{DB: newTestDB(t)}
	data := apitypes.VolcTenantModelProviderData{
		ApiMode:       apitypes.VolcTenantModelProviderDataApiModeChatCompletions,
		UpstreamModel: new("doubao-test"),
		ServiceTier:   new(apitypes.VolcTenantModelProviderDataServiceTierFast),
	}
	body := modelUpsertWithProviderData("fast-chat", apitypes.ModelProviderKindVolcTenant, modelProviderData(t, data))
	created, err := srv.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := created.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("CreateModel() = %#v", created)
	}
	got, err := srv.GetModel(ctx, adminhttp.GetModelRequestObject{Id: body.Id})
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := got.(adminhttp.GetModel200JSONResponse)
	if !ok {
		t.Fatalf("GetModel() = %#v", got)
	}
	decoded, err := stored.ProviderData.AsVolcTenantModelProviderData()
	if err != nil || decoded.ServiceTier == nil || *decoded.ServiceTier != *data.ServiceTier {
		t.Fatalf("stored provider_data = %#v, %v", decoded, err)
	}
	data.ServiceTier = nil
	body.ProviderData = modelProviderData(t, data)
	updated, err := srv.PutModel(ctx, adminhttp.PutModelRequestObject{Id: body.Id, Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.(adminhttp.PutModel200JSONResponse); !ok {
		t.Fatalf("PutModel() = %#v", updated)
	}
	got, err = srv.GetModel(ctx, adminhttp.GetModelRequestObject{Id: body.Id})
	if err != nil {
		t.Fatal(err)
	}
	stored, ok = got.(adminhttp.GetModel200JSONResponse)
	if !ok {
		t.Fatalf("GetModel() after update = %#v", got)
	}
	decoded, err = stored.ProviderData.AsVolcTenantModelProviderData()
	if err != nil || decoded.ServiceTier != nil {
		t.Fatalf("removed service_tier = %#v, %v", decoded.ServiceTier, err)
	}
}

func TestVolcServiceTierValidation(t *testing.T) {
	for _, tier := range []string{"auto", "default", "fast", "flex", "", "priority", "FAST"} {
		data := apitypes.VolcTenantModelProviderData{
			ApiMode: apitypes.VolcTenantModelProviderDataApiModeChatCompletions, UpstreamModel: new("doubao-test"),
			ServiceTier: new(apitypes.VolcTenantModelProviderDataServiceTier(tier)),
		}
		err := ValidateProviderData(apitypes.ModelKindLlm, apitypes.ModelProviderKindVolcTenant, modelProviderData(t, data))
		valid := tier == "auto" || tier == "default" || tier == "fast" || tier == "flex"
		if (err == nil) != valid {
			t.Errorf("service_tier %q: error = %v", tier, err)
		}
	}
	for _, kind := range []apitypes.ModelKind{apitypes.ModelKindAsr, apitypes.ModelKindTts, apitypes.ModelKindRealtime, apitypes.ModelKindRealtimeDuplex, apitypes.ModelKindTranslation, apitypes.ModelKindEmbedding} {
		mode, _ := volcAPIModeForModelKind(kind)
		data := apitypes.VolcTenantModelProviderData{ApiMode: mode, UpstreamModel: new("doubao-test"), ServiceTier: new(apitypes.VolcTenantModelProviderDataServiceTierFast)}
		if err := ValidateProviderData(kind, apitypes.ModelProviderKindVolcTenant, modelProviderData(t, data)); err == nil {
			t.Errorf("service_tier accepted for %s", kind)
		}
	}
}
