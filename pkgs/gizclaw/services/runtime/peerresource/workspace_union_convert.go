package peerresource

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

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
