package apitypes

import (
	"fmt"
	"unicode/utf8"
)

// ValidateSafetyFenceLevel rejects unknown levels; omission preserves the stored level.
func ValidateSafetyFenceLevel(value *SafetyFenceLevel) error {
	if value != nil && !value.Valid() {
		return fmt.Errorf("unsupported safety_fence_level %q", *value)
	}
	return nil
}

// SafetyFenceLevel returns the validated level stored on an AI Workspace.
// Absent values and SFU Workspaces return nil.
func (t WorkspaceParameters) SafetyFenceLevel() (*SafetyFenceLevel, error) {
	discriminator, err := t.Discriminator()
	if err != nil {
		return nil, err
	}
	var value *SafetyFenceLevel
	switch WorkflowDriver(discriminator) {
	case WorkflowDriverFlowcraft:
		parameters, err := t.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.SafetyFenceLevel
	case WorkflowDriverEino:
		parameters, err := t.AsEinoWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.SafetyFenceLevel
	case WorkflowDriverDoubaoRealtime:
		parameters, err := t.AsDoubaoRealtimeWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.SafetyFenceLevel
	case WorkflowDriverDoubaoRealtimeDuplex:
		parameters, err := t.AsDoubaoRealtimeDuplexWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.SafetyFenceLevel
	case WorkflowDriverDashscopeRealtime:
		parameters, err := t.AsDashScopeRealtimeWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.SafetyFenceLevel
	case WorkflowDriverAstTranslate:
		parameters, err := t.AsASTTranslateWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.SafetyFenceLevel
	default:
		return nil, nil
	}
	if err := ValidateSafetyFenceLevel(value); err != nil {
		return nil, err
	}
	return value, nil
}

// ValidateSafetyFencePrompt checks the schema character bounds.
func ValidateSafetyFencePrompt(prompt string) error {
	if prompt == "" || utf8.RuneCountInString(prompt) > 4096 {
		return fmt.Errorf("prompt must contain 1..4096 characters")
	}
	return nil
}
