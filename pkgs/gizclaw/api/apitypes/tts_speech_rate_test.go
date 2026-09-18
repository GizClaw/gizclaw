package apitypes

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceParametersTTSSpeechRatePercent(t *testing.T) {
	t.Parallel()
	for _, agentType := range []string{
		"flowcraft", "eino", "doubao-realtime", "doubao-realtime-duplex", "dashscope-realtime", "ast-translate",
	} {
		t.Run(agentType, func(t *testing.T) {
			t.Parallel()
			var parameters WorkspaceParameters
			if err := json.Unmarshal([]byte(`{"agent_type":"`+agentType+`","tts_speech_rate_percent":70}`), &parameters); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			got, err := parameters.TTSSpeechRatePercent()
			if err != nil || got == nil || *got != 70 {
				t.Fatalf("TTSSpeechRatePercent() = %v, %v; want 70", got, err)
			}
			var absent WorkspaceParameters
			if err := json.Unmarshal([]byte(`{"agent_type":"`+agentType+`"}`), &absent); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got, err := absent.TTSSpeechRatePercent(); err != nil || got != nil {
				t.Fatalf("absent TTSSpeechRatePercent() = %v, %v; want nil", got, err)
			}
		})
	}
}

func TestWorkspaceParametersTTSSpeechRatePercentRejectsOutOfRange(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"49", "201"} {
		var parameters WorkspaceParameters
		if err := json.Unmarshal([]byte(`{"agent_type":"eino","tts_speech_rate_percent":`+value+`}`), &parameters); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if _, err := parameters.TTSSpeechRatePercent(); err == nil {
			t.Fatalf("TTSSpeechRatePercent(%s) error = nil, want range error", value)
		}
	}
	for _, value := range []int{TTSSpeechRatePercentMin, TTSSpeechRatePercentNormal, TTSSpeechRatePercentMax} {
		if err := ValidateTTSSpeechRatePercent(&value); err != nil {
			t.Fatalf("ValidateTTSSpeechRatePercent(%d) error = %v", value, err)
		}
	}
	if err := ValidateTTSSpeechRatePercent(nil); err != nil {
		t.Fatalf("ValidateTTSSpeechRatePercent(nil) error = %v", err)
	}
}

func TestWorkspaceParametersTTSSpeechRatePercentIgnoresNonVoiceVariants(t *testing.T) {
	t.Parallel()
	var parameters WorkspaceParameters
	if err := json.Unmarshal([]byte(`{"agent_type":"sfu"}`), &parameters); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, err := parameters.TTSSpeechRatePercent(); err != nil || got != nil {
		t.Fatalf("TTSSpeechRatePercent() = %v, %v; want nil", got, err)
	}
}
