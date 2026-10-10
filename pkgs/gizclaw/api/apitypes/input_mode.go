package apitypes

import "fmt"

// InputMode returns the Workspace input mode. Duplex and DashScope are always
// realtime; other drivers use push-to-talk when the mode is omitted.
func (t WorkspaceParameters) InputMode() (WorkspaceInputMode, error) {
	discriminator, err := t.Discriminator()
	if err != nil {
		return "", err
	}
	var value *WorkspaceInputMode
	switch WorkflowDriver(discriminator) {
	case WorkflowDriverEino:
		parameters, err := t.AsEinoWorkspaceParameters()
		if err != nil {
			return "", err
		}
		value = parameters.Input
	case WorkflowDriverDoubaoRealtime:
		parameters, err := t.AsDoubaoRealtimeWorkspaceParameters()
		if err != nil {
			return "", err
		}
		value = parameters.Input
	case WorkflowDriverDoubaoRealtimeDuplex, WorkflowDriverDashscopeRealtime:
		return WorkspaceInputModeRealtime, nil
	case WorkflowDriverAstTranslate:
		parameters, err := t.AsASTTranslateWorkspaceParameters()
		if err != nil {
			return "", err
		}
		value = parameters.Input
	default:
		return WorkspaceInputModePushToTalk, nil
	}
	if value == nil {
		return WorkspaceInputModePushToTalk, nil
	}
	if !value.Valid() {
		return "", fmt.Errorf("unsupported input mode %q", *value)
	}
	return *value, nil
}

// WorkspaceInput resolves the input mode, including a driver's default when
// the Workspace has no parameters.
func WorkspaceInput(driver WorkflowDriver, parameters *WorkspaceParameters) (WorkspaceInputMode, error) {
	if parameters == nil {
		if driver == WorkflowDriverDoubaoRealtimeDuplex || driver == WorkflowDriverDashscopeRealtime {
			return WorkspaceInputModeRealtime, nil
		}
		return WorkspaceInputModePushToTalk, nil
	}
	return parameters.InputMode()
}
