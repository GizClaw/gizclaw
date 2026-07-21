package peerresource

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestModelRPCProjectionUsesExactlyTheSelectedProviderData(t *testing.T) {
	upstream := "model-upstream"
	dashScopeMode := apitypes.DashScopeTenantModelProviderDataApiModeChatCompletions
	volcMode := apitypes.VolcTenantModelProviderDataApiModeChatCompletions
	tests := []struct {
		kind apitypes.ModelProviderKind
		data apitypes.ModelProviderData
	}{
		{kind: apitypes.ModelProviderKindOpenaiTenant, data: testModelProviderData(t, apitypes.OpenAITenantModelProviderData{UpstreamModel: upstream})},
		{kind: apitypes.ModelProviderKindGeminiTenant, data: testModelProviderData(t, apitypes.GeminiTenantModelProviderData{UpstreamModel: upstream})},
		{kind: apitypes.ModelProviderKindDashscopeTenant, data: testModelProviderData(t, apitypes.DashScopeTenantModelProviderData{ApiMode: &dashScopeMode, UpstreamModel: &upstream})},
		{kind: apitypes.ModelProviderKindVolcTenant, data: testModelProviderData(t, apitypes.VolcTenantModelProviderData{ApiMode: &volcMode, UpstreamModel: &upstream})},
		{kind: apitypes.ModelProviderKindMinimaxTenant, data: testModelProviderData(t, apitypes.MiniMaxTenantModelProviderData{ApiMode: apitypes.MiniMaxTenantModelProviderDataApiModeChatCompletions, UpstreamModel: upstream})},
		{kind: apitypes.ModelProviderKindDeepseekTenant, data: testModelProviderData(t, apitypes.DeepSeekTenantModelProviderData{ApiMode: apitypes.DeepSeekTenantModelProviderDataApiModeChatCompletions, UpstreamModel: upstream})},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			got, err := modelRPCProjection("chat", apitypes.RuntimeProfileBinding{}, apitypes.Model{
				Kind:         apitypes.ModelKindLlm,
				Provider:     apitypes.ModelProvider{Kind: tt.kind, Name: "main"},
				ProviderData: tt.data,
			})
			if err != nil {
				t.Fatalf("modelRPCProjection() error = %v", err)
			}
			if got.ProviderKind != rpcapi.ModelProviderKind(tt.kind) || modelRPCProviderDataCount(got) != 1 {
				t.Fatalf("modelRPCProjection() = %#v", got)
			}
			if tt.kind == apitypes.ModelProviderKindDashscopeTenant &&
				(got.DashScopeTenant == nil || got.DashScopeTenant.ApiMode == nil || string(*got.DashScopeTenant.ApiMode) != string(dashScopeMode)) {
				t.Fatalf("modelRPCProjection() lost DashScope api_mode: %#v", got.DashScopeTenant)
			}
		})
	}

	_, err := modelRPCProjection("chat", apitypes.RuntimeProfileBinding{}, apitypes.Model{
		Kind:         apitypes.ModelKindLlm,
		Provider:     apitypes.ModelProvider{Kind: apitypes.ModelProviderKindOpenaiTenant, Name: "main"},
		ProviderData: tests[len(tests)-1].data,
	})
	if err == nil {
		t.Fatal("modelRPCProjection() accepted provider kind/data mismatch")
	}
}

func testModelProviderData(t *testing.T, value any) apitypes.ModelProviderData {
	t.Helper()
	falseValue := false
	var data apitypes.ModelProviderData
	var err error
	switch value := value.(type) {
	case apitypes.OpenAITenantModelProviderData:
		value.SupportJsonOutput, value.SupportToolCalls, value.SupportTextOnly = &falseValue, &falseValue, &falseValue
		value.UseSystemRole, value.SupportTemperature, value.SupportThinking = &falseValue, &falseValue, &falseValue
		err = data.FromOpenAITenantModelProviderData(value)
	case apitypes.GeminiTenantModelProviderData:
		value.SupportJsonOutput, value.SupportToolCalls, value.SupportTextOnly = &falseValue, &falseValue, &falseValue
		value.UseSystemRole, value.SupportTemperature, value.SupportThinking = &falseValue, &falseValue, &falseValue
		err = data.FromGeminiTenantModelProviderData(value)
	case apitypes.DashScopeTenantModelProviderData:
		value.SupportJsonOutput, value.SupportToolCalls, value.SupportTextOnly = &falseValue, &falseValue, &falseValue
		value.UseSystemRole, value.SupportTemperature, value.SupportThinking = &falseValue, &falseValue, &falseValue
		err = data.FromDashScopeTenantModelProviderData(value)
	case apitypes.VolcTenantModelProviderData:
		value.SupportJsonOutput, value.SupportToolCalls, value.SupportTextOnly = &falseValue, &falseValue, &falseValue
		value.UseSystemRole, value.SupportTemperature, value.SupportThinking = &falseValue, &falseValue, &falseValue
		err = data.FromVolcTenantModelProviderData(value)
	case apitypes.MiniMaxTenantModelProviderData:
		value.SupportJsonOutput, value.SupportToolCalls, value.SupportTextOnly = &falseValue, &falseValue, &falseValue
		value.UseSystemRole, value.SupportTemperature, value.SupportThinking = &falseValue, &falseValue, &falseValue
		err = data.FromMiniMaxTenantModelProviderData(value)
	case apitypes.DeepSeekTenantModelProviderData:
		value.SupportJsonOutput, value.SupportToolCalls, value.SupportTextOnly = &falseValue, &falseValue, &falseValue
		value.UseSystemRole, value.SupportTemperature, value.SupportThinking = &falseValue, &falseValue, &falseValue
		err = data.FromDeepSeekTenantModelProviderData(value)
	default:
		t.Fatalf("unsupported provider data type %T", value)
	}
	if err != nil {
		t.Fatalf("encode provider data: %v", err)
	}
	return data
}

func modelRPCProviderDataCount(model rpcapi.Model) int {
	count := 0
	for _, present := range []bool{
		model.OpenAITenant != nil,
		model.GeminiTenant != nil,
		model.DashScopeTenant != nil,
		model.VolcTenant != nil,
		model.MiniMaxTenant != nil,
		model.DeepSeekTenant != nil,
	} {
		if present {
			count++
		}
	}
	return count
}
