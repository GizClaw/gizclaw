package flowcraft

import (
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func testStringPtr(value string) *string { return &value }

func testOpenAICredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testVolcCredentialBody(arkAPIKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{ArkApiKey: testStringPtr(arkAPIKey)}); err != nil {
		panic(err)
	}
	return body
}

func testOpenAIModelProviderData(data apitypes.OpenAITenantModelProviderData) *apitypes.ModelProviderData {
	var out apitypes.ModelProviderData
	if err := out.FromOpenAITenantModelProviderData(data); err != nil {
		panic(err)
	}
	return &out
}

func testVolcModelProviderData(data apitypes.VolcTenantModelProviderData) *apitypes.ModelProviderData {
	var out apitypes.ModelProviderData
	if err := out.FromVolcTenantModelProviderData(data); err != nil {
		panic(err)
	}
	return &out
}

func testFlowcraftWorkspaceParameters(values map[string]any) *apitypes.WorkspaceParameters {
	typed := apitypes.FlowcraftWorkspaceParameters{
		AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
	}
	if value, _ := values["generate_model"].(string); value != "" {
		typed.GenerateModel = &value
	}
	if value, _ := values["extract_model"].(string); value != "" {
		typed.ExtractModel = &value
	}
	if value, _ := values["embedding_model"].(string); value != "" {
		typed.EmbeddingModel = &value
	}
	var out apitypes.WorkspaceParameters
	if err := out.FromFlowcraftWorkspaceParameters(typed); err != nil {
		panic(err)
	}
	return &out
}
