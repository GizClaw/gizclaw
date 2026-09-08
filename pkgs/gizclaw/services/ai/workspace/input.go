package workspace

import "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

// workspaceParametersWithInput projects stored Workspace parameters with the
// input mode replaced. A Workspace that inherits its parameters keeps
// inheriting every other field: only the agent_type discriminator required by
// the Workflow driver and the input override are written. Drivers without
// input support preserve the original parameters without an update.
func workspaceParametersWithInput(
	parameters *apitypes.WorkspaceParameters,
	driver apitypes.WorkflowDriver,
	input apitypes.WorkspaceInputMode,
) (*apitypes.WorkspaceParameters, error) {
	variant := string(driver)
	if parameters != nil {
		discriminator, err := parameters.Discriminator()
		if err != nil {
			return nil, invalidWorkspaceReference("workspace: unreadable parameters: %v", err)
		}
		if discriminator != variant {
			return nil, invalidWorkspaceReference("workspace: parameters agent_type is %q, want %q", discriminator, variant)
		}
	}
	updated := &apitypes.WorkspaceParameters{}
	switch variant {
	case string(apitypes.WorkflowDriverAstTranslate):
		value := apitypes.ASTTranslateWorkspaceParameters{AgentType: apitypes.ASTTranslateWorkspaceParametersAgentTypeAstTranslate}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsASTTranslateWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromASTTranslateWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverDoubaoRealtime):
		value := apitypes.DoubaoRealtimeWorkspaceParameters{AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsDoubaoRealtimeWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromDoubaoRealtimeWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverEino):
		value := apitypes.EinoWorkspaceParameters{AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsEinoWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromEinoWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverFlowcraft):
		value := apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsFlowcraftWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromFlowcraftWorkspaceParameters(value)
	default:
		return parameters, nil
	}
}

func decodeWorkspaceParametersVariant[T any](
	parameters *apitypes.WorkspaceParameters,
	out *T,
	as func(apitypes.WorkspaceParameters) (T, error),
) error {
	if parameters == nil {
		return nil
	}
	value, err := as(*parameters)
	if err != nil {
		return invalidWorkspaceReference("workspace: unreadable parameters: %v", err)
	}
	*out = value
	return nil
}
