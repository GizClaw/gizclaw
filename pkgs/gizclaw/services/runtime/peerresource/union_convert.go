package peerresource

import (
	"fmt"
	"reflect"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func convertUnionFieldWithParent(dst reflect.Value, src reflect.Value, parent reflect.Value) (bool, error) {
	if !src.IsValid() {
		return false, nil
	}
	if dst.Type() == reflect.TypeOf((*rpcapi.ModelProviderData)(nil)) && src.Type() == reflect.TypeOf((*apitypes.ModelProviderData)(nil)) {
		if src.IsNil() {
			return true, nil
		}
		converted, err := apiModelProviderDataToRPCForKind(src.Elem().Interface().(apitypes.ModelProviderData), providerKind(parent))
		if err != nil {
			return true, err
		}
		dst.Set(reflect.ValueOf(&converted))
		return true, nil
	}
	if dst.Type() == reflect.TypeOf((*apitypes.ModelProviderData)(nil)) && src.Type() == reflect.TypeOf((*rpcapi.ModelProviderData)(nil)) {
		if src.IsNil() {
			return true, nil
		}
		converted, err := rpcModelProviderDataToAPI(src.Elem().Interface().(rpcapi.ModelProviderData))
		if err != nil {
			return true, err
		}
		dst.Set(reflect.ValueOf(&converted))
		return true, nil
	}
	if dst.Type() == reflect.TypeOf((*rpcapi.VoiceProviderData)(nil)) && src.Type() == reflect.TypeOf((*apitypes.VoiceProviderData)(nil)) {
		if src.IsNil() {
			return true, nil
		}
		converted, err := apiVoiceProviderDataToRPCForKind(src.Elem().Interface().(apitypes.VoiceProviderData), providerKind(parent))
		if err != nil {
			return true, err
		}
		dst.Set(reflect.ValueOf(&converted))
		return true, nil
	}
	if dst.Type() == reflect.TypeOf((*apitypes.VoiceProviderData)(nil)) && src.Type() == reflect.TypeOf((*rpcapi.VoiceProviderData)(nil)) {
		if src.IsNil() {
			return true, nil
		}
		converted, err := rpcVoiceProviderDataToAPI(src.Elem().Interface().(rpcapi.VoiceProviderData))
		if err != nil {
			return true, err
		}
		dst.Set(reflect.ValueOf(&converted))
		return true, nil
	}
	return false, nil
}

func providerKind(parent reflect.Value) string {
	parent = indirectReflectValue(parent)
	if !parent.IsValid() || parent.Kind() != reflect.Struct {
		return ""
	}
	provider := parent.FieldByName("Provider")
	provider = indirectReflectValue(provider)
	if !provider.IsValid() || provider.Kind() != reflect.Struct {
		return ""
	}
	kind := provider.FieldByName("Kind")
	kind = indirectReflectValue(kind)
	if !kind.IsValid() {
		return ""
	}
	return fmt.Sprint(kind.Interface())
}

func rpcModelProviderDataToAPI(in rpcapi.ModelProviderData) (apitypes.ModelProviderData, error) {
	var out apitypes.ModelProviderData
	if typed, err := in.AsGeminiTenantModelProviderData(); err == nil {
		converted, err := convertType[apitypes.GeminiTenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromGeminiTenantModelProviderData(converted)
	}
	if typed, err := in.AsDashScopeTenantModelProviderData(); err == nil {
		converted, err := convertType[apitypes.DashScopeTenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromDashScopeTenantModelProviderData(converted)
	}
	if typed, err := in.AsOpenAITenantModelProviderData(); err == nil {
		converted, err := convertType[apitypes.OpenAITenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromOpenAITenantModelProviderData(converted)
	}
	if typed, err := in.AsVolcTenantModelProviderData(); err == nil {
		converted, err := convertType[apitypes.VolcTenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromVolcTenantModelProviderData(converted)
	}
	return out, fmt.Errorf("model provider data is empty or unsupported")
}

func apiModelProviderDataToRPC(in apitypes.ModelProviderData) (rpcapi.ModelProviderData, error) {
	return apiModelProviderDataToRPCForKind(in, "")
}

func apiModelProviderDataToRPCForKind(in apitypes.ModelProviderData, kind string) (rpcapi.ModelProviderData, error) {
	var out rpcapi.ModelProviderData
	switch kind {
	case "gemini-tenant":
		typed, err := in.AsGeminiTenantModelProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.GeminiTenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromGeminiTenantModelProviderData(converted)
	case "dashscope-tenant":
		typed, err := in.AsDashScopeTenantModelProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.DashScopeTenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromDashScopeTenantModelProviderData(converted)
	case "openai-tenant":
		typed, err := in.AsOpenAITenantModelProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.OpenAITenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromOpenAITenantModelProviderData(converted)
	case "volc-tenant":
		typed, err := in.AsVolcTenantModelProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.VolcTenantModelProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromVolcTenantModelProviderData(converted)
	}
	return out, fmt.Errorf("model provider data requires provider kind")
}

func rpcVoiceProviderDataToAPI(in rpcapi.VoiceProviderData) (apitypes.VoiceProviderData, error) {
	var out apitypes.VoiceProviderData
	if typed, err := in.AsGeminiTenantVoiceProviderData(); err == nil {
		converted, err := convertType[apitypes.GeminiTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromGeminiTenantVoiceProviderData(converted)
	}
	if typed, err := in.AsDashScopeTenantVoiceProviderData(); err == nil {
		converted, err := convertType[apitypes.DashScopeTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromDashScopeTenantVoiceProviderData(converted)
	}
	if typed, err := in.AsOpenAITenantVoiceProviderData(); err == nil {
		converted, err := convertType[apitypes.OpenAITenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromOpenAITenantVoiceProviderData(converted)
	}
	if typed, err := in.AsMiniMaxTenantVoiceProviderData(); err == nil {
		converted, err := convertType[apitypes.MiniMaxTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromMiniMaxTenantVoiceProviderData(converted)
	}
	if typed, err := in.AsVolcTenantVoiceProviderData(); err == nil {
		converted, err := convertType[apitypes.VolcTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromVolcTenantVoiceProviderData(converted)
	}
	return out, fmt.Errorf("voice provider data is empty or unsupported")
}

func apiVoiceProviderDataToRPC(in apitypes.VoiceProviderData) (rpcapi.VoiceProviderData, error) {
	return apiVoiceProviderDataToRPCForKind(in, "")
}

func apiVoiceProviderDataToRPCForKind(in apitypes.VoiceProviderData, kind string) (rpcapi.VoiceProviderData, error) {
	var out rpcapi.VoiceProviderData
	switch kind {
	case "gemini-tenant":
		typed, err := in.AsGeminiTenantVoiceProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.GeminiTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromGeminiTenantVoiceProviderData(converted)
	case "dashscope-tenant":
		typed, err := in.AsDashScopeTenantVoiceProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.DashScopeTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromDashScopeTenantVoiceProviderData(converted)
	case "openai-tenant":
		typed, err := in.AsOpenAITenantVoiceProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.OpenAITenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromOpenAITenantVoiceProviderData(converted)
	case "minimax-tenant":
		typed, err := in.AsMiniMaxTenantVoiceProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.MiniMaxTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromMiniMaxTenantVoiceProviderData(converted)
	case "volc-tenant":
		typed, err := in.AsVolcTenantVoiceProviderData()
		if err != nil {
			return out, err
		}
		converted, err := convertType[rpcapi.VolcTenantVoiceProviderData](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromVolcTenantVoiceProviderData(converted)
	}
	return out, fmt.Errorf("voice provider data requires provider kind")
}

func rpcWorkspaceParametersToAPI(in rpcapi.WorkspaceParameters) (apitypes.WorkspaceParameters, error) {
	var out apitypes.WorkspaceParameters
	if typed, err := in.AsFlowcraftWorkspaceParameters(); err == nil {
		converted, err := convertType[apitypes.FlowcraftWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromFlowcraftWorkspaceParameters(converted)
	}
	if typed, err := in.AsDoubaoRealtimeWorkspaceParameters(); err == nil {
		converted, err := convertType[apitypes.DoubaoRealtimeWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromDoubaoRealtimeWorkspaceParameters(converted)
	}
	if typed, err := in.AsASTTranslateWorkspaceParameters(); err == nil {
		converted, err := convertType[apitypes.ASTTranslateWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromASTTranslateWorkspaceParameters(converted)
	}
	if typed, err := in.AsChatRoomWorkspaceParameters(); err == nil {
		converted, err := convertType[apitypes.ChatRoomWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromChatRoomWorkspaceParameters(converted)
	}
	return out, fmt.Errorf("workspace parameters are empty or unsupported")
}

func apiWorkspaceParametersToRPC(in apitypes.WorkspaceParameters) (rpcapi.WorkspaceParameters, error) {
	var out rpcapi.WorkspaceParameters
	value, err := in.ValueByDiscriminator()
	if err != nil {
		return out, err
	}
	switch typed := value.(type) {
	case apitypes.FlowcraftWorkspaceParameters:
		converted, err := convertType[rpcapi.FlowcraftWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromFlowcraftWorkspaceParameters(converted)
	case apitypes.DoubaoRealtimeWorkspaceParameters:
		converted, err := convertType[rpcapi.DoubaoRealtimeWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromDoubaoRealtimeWorkspaceParameters(converted)
	case apitypes.ASTTranslateWorkspaceParameters:
		converted, err := convertType[rpcapi.ASTTranslateWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromASTTranslateWorkspaceParameters(converted)
	case apitypes.ChatRoomWorkspaceParameters:
		converted, err := convertType[rpcapi.ChatRoomWorkspaceParameters](typed)
		if err != nil {
			return out, err
		}
		return out, out.FromChatRoomWorkspaceParameters(converted)
	}
	return out, fmt.Errorf("workspace parameters are empty or unsupported")
}
