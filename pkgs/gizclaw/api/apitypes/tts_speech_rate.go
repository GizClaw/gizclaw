package apitypes

import "fmt"

// Workspace tts_speech_rate_percent bounds. The value is a percentage of the
// provider's normal speaking rate: 50 is half speed and 200 is double speed.
const (
	TTSSpeechRatePercentMin    = 50
	TTSSpeechRatePercentNormal = 100
	TTSSpeechRatePercentMax    = 200
)

// ValidateTTSSpeechRatePercent rejects a present rate outside
// TTSSpeechRatePercentMin..TTSSpeechRatePercentMax. Absent is valid.
func ValidateTTSSpeechRatePercent(value *int) error {
	if value == nil {
		return nil
	}
	if *value < TTSSpeechRatePercentMin || *value > TTSSpeechRatePercentMax {
		return fmt.Errorf("tts_speech_rate_percent must be between %d and %d", TTSSpeechRatePercentMin, TTSSpeechRatePercentMax)
	}
	return nil
}

// TTSSpeechRatePercent returns the validated speech rate stored on a
// voice-producing Workspace parameter variant. Variants without synthesized
// speech and absent values return nil.
func (t WorkspaceParameters) TTSSpeechRatePercent() (*int, error) {
	discriminator, err := t.Discriminator()
	if err != nil {
		return nil, err
	}
	var value *int
	switch WorkflowDriver(discriminator) {
	case WorkflowDriverFlowcraft:
		parameters, err := t.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.TtsSpeechRatePercent
	case WorkflowDriverEino:
		parameters, err := t.AsEinoWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.TtsSpeechRatePercent
	case WorkflowDriverDoubaoRealtime:
		parameters, err := t.AsDoubaoRealtimeWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.TtsSpeechRatePercent
	case WorkflowDriverDoubaoRealtimeDuplex:
		parameters, err := t.AsDoubaoRealtimeDuplexWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.TtsSpeechRatePercent
	case WorkflowDriverDashscopeRealtime:
		parameters, err := t.AsDashScopeRealtimeWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.TtsSpeechRatePercent
	case WorkflowDriverAstTranslate:
		parameters, err := t.AsASTTranslateWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		value = parameters.TtsSpeechRatePercent
	default:
		return nil, nil
	}
	if err := ValidateTTSSpeechRatePercent(value); err != nil {
		return nil, err
	}
	return value, nil
}

// WorkspaceTTSSpeechRatePercent is TTSSpeechRatePercent for optional
// parameters; nil parameters keep the Workflow default.
func WorkspaceTTSSpeechRatePercent(parameters *WorkspaceParameters) (*int, error) {
	if parameters == nil {
		return nil, nil
	}
	return parameters.TTSSpeechRatePercent()
}
