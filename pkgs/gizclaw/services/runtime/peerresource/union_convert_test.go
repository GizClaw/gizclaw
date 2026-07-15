package peerresource

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestConvertTypePreservesNonCredentialUnions(t *testing.T) {
	supportTextOnly := true
	var rpcModel rpcapi.ModelProviderData
	if err := rpcModel.FromOpenAITenantModelProviderData(rpcapi.OpenAITenantModelProviderData{
		SupportTextOnly: &supportTextOnly,
	}); err != nil {
		t.Fatalf("FromOpenAITenantModelProviderData() error = %v", err)
	}
	apiModel, err := convertType[apitypes.ModelProviderData](rpcModel)
	if err != nil {
		t.Fatalf("convert model provider data error = %v", err)
	}
	apiModelValue, err := apiModel.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatalf("AsOpenAITenantModelProviderData() error = %v", err)
	}
	if apiModelValue.SupportTextOnly == nil || !*apiModelValue.SupportTextOnly {
		t.Fatalf("api model provider data = %+v", apiModelValue)
	}

	voiceID := "voice-1"
	var apiVoice apitypes.VoiceProviderData
	if err := apiVoice.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{
		VoiceId: &voiceID,
	}); err != nil {
		t.Fatalf("FromMiniMaxTenantVoiceProviderData() error = %v", err)
	}
	rpcVoiceRecord, err := convertType[rpcapi.Voice](apitypes.Voice{
		Id: "voice-1",
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindMinimaxTenant,
			Name: "minimax",
		},
		ProviderData: &apiVoice,
	})
	if err != nil {
		t.Fatalf("convert voice provider data error = %v", err)
	}
	if rpcVoiceRecord.ProviderData == nil {
		t.Fatal("rpc voice provider data = nil")
	}
	rpcVoiceValue, err := rpcVoiceRecord.ProviderData.AsMiniMaxTenantVoiceProviderData()
	if err != nil {
		t.Fatalf("AsMiniMaxTenantVoiceProviderData() error = %v", err)
	}
	if rpcVoiceValue.VoiceId == nil || *rpcVoiceValue.VoiceId != voiceID {
		t.Fatalf("rpc voice provider data = %+v", rpcVoiceValue)
	}

	input := rpcapi.WorkspaceInputModePushToTalk
	var rpcWorkspace rpcapi.WorkspaceParameters
	if err := rpcWorkspace.FromChatRoomWorkspaceParameters(rpcapi.ChatRoomWorkspaceParameters{
		Input: &input,
	}); err != nil {
		t.Fatalf("FromChatRoomWorkspaceParameters() error = %v", err)
	}
	apiWorkspace, err := convertType[apitypes.WorkspaceParameters](rpcWorkspace)
	if err != nil {
		t.Fatalf("convert workspace parameters error = %v", err)
	}
	apiWorkspaceValue, err := apiWorkspace.AsChatRoomWorkspaceParameters()
	if err != nil {
		t.Fatalf("AsChatRoomWorkspaceParameters() error = %v", err)
	}
	if apiWorkspaceValue.AgentType != apitypes.ChatRoomWorkspaceParametersAgentTypeChatroom {
		t.Fatalf("api workspace parameters = %+v", apiWorkspaceValue)
	}

	voicePrompt := "warm"
	var apiPetWorkspace apitypes.WorkspaceParameters
	if err := apiPetWorkspace.FromPetWorkspaceParameters(apitypes.PetWorkspaceParameters{
		AgentType: apitypes.PetWorkspaceParametersAgentTypePet,
		Voice:     apitypes.PetVoiceParameters{VoiceId: "voice-pet", Prompt: &voicePrompt},
	}); err != nil {
		t.Fatalf("FromPetWorkspaceParameters() error = %v", err)
	}
	rpcPetWorkspace, err := convertType[rpcapi.WorkspaceParameters](apiPetWorkspace)
	if err != nil {
		t.Fatalf("convert pet workspace parameters to RPC error = %v", err)
	}
	rpcPetValue, err := rpcPetWorkspace.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("AsPetWorkspaceParameters(RPC) error = %v", err)
	}
	if rpcPetValue.AgentType != rpcapi.PetWorkspaceParametersAgentTypePet || rpcPetValue.Voice.VoiceId != "voice-pet" {
		t.Fatalf("rpc pet workspace parameters = %+v", rpcPetValue)
	}
	roundTrippedPetWorkspace, err := convertType[apitypes.WorkspaceParameters](rpcPetWorkspace)
	if err != nil {
		t.Fatalf("convert pet workspace parameters to API error = %v", err)
	}
	apiPetValue, err := roundTrippedPetWorkspace.AsPetWorkspaceParameters()
	if err != nil {
		t.Fatalf("AsPetWorkspaceParameters(API) error = %v", err)
	}
	if apiPetValue.Voice.VoiceId != "voice-pet" || apiPetValue.Voice.Prompt == nil || *apiPetValue.Voice.Prompt != voicePrompt {
		t.Fatalf("round-tripped pet workspace parameters = %+v", apiPetValue)
	}
}
