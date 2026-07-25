package apitypes

import (
	"fmt"
	"math"
	"strings"
)

// Validate verifies the constraints shared by HTTP, RPC, and internal
// DashScope Realtime Workflow writes.
func (s DashScopeRealtimeWorkflowSpec) Validate() error {
	if strings.TrimSpace(s.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if err := validateOptionalNonBlank("voice", s.Voice); err != nil {
		return err
	}
	if err := validateOptionalNonBlank("asr_model", s.AsrModel); err != nil {
		return err
	}
	if s.InputAudioFormat != nil && !s.InputAudioFormat.Valid() {
		return fmt.Errorf("input_audio_format %q is unsupported", *s.InputAudioFormat)
	}
	if s.OutputAudioFormat != nil && !s.OutputAudioFormat.Valid() {
		return fmt.Errorf("output_audio_format %q is unsupported", *s.OutputAudioFormat)
	}
	if s.Vad != nil && !s.Vad.Valid() {
		return fmt.Errorf("vad %q is unsupported", *s.Vad)
	}
	if s.Modalities != nil {
		values := make([]string, len(*s.Modalities))
		for index, value := range *s.Modalities {
			if !value.Valid() {
				return fmt.Errorf("modalities contains unsupported value %q", value)
			}
			values[index] = string(value)
		}
		if err := validateModalities(values); err != nil {
			return err
		}
	}
	return validateDashScopeRealtimeNumbers(s.Temperature, s.MaxOutputTokens)
}

// Validate verifies the constraints shared by HTTP, RPC, and internal
// DashScope Realtime Workspace writes.
func (p DashScopeRealtimeWorkspaceParameters) Validate() error {
	if !p.AgentType.Valid() {
		return fmt.Errorf("agent_type %q is unsupported", p.AgentType)
	}
	if err := validateOptionalNonBlank("model", p.Model); err != nil {
		return err
	}
	if err := validateOptionalNonBlank("voice", p.Voice); err != nil {
		return err
	}
	if err := validateOptionalNonBlank("asr_model", p.AsrModel); err != nil {
		return err
	}
	if p.InputAudioFormat != nil && !p.InputAudioFormat.Valid() {
		return fmt.Errorf("input_audio_format %q is unsupported", *p.InputAudioFormat)
	}
	if p.OutputAudioFormat != nil && !p.OutputAudioFormat.Valid() {
		return fmt.Errorf("output_audio_format %q is unsupported", *p.OutputAudioFormat)
	}
	if p.Vad != nil && !p.Vad.Valid() {
		return fmt.Errorf("vad %q is unsupported", *p.Vad)
	}
	if p.Modalities != nil {
		values := make([]string, len(*p.Modalities))
		for index, value := range *p.Modalities {
			if !value.Valid() {
				return fmt.Errorf("modalities contains unsupported value %q", value)
			}
			values[index] = string(value)
		}
		if err := validateModalities(values); err != nil {
			return err
		}
	}
	return validateDashScopeRealtimeNumbers(p.Temperature, p.MaxOutputTokens)
}

func validateDashScopeRealtimeNumbers(temperature *float32, maxOutputTokens *int) error {
	if temperature != nil {
		value := float64(*temperature)
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 2 {
			return fmt.Errorf("temperature must be between 0 and 2")
		}
	}
	if maxOutputTokens != nil && *maxOutputTokens < 1 {
		return fmt.Errorf("max_output_tokens must be at least 1")
	}
	return nil
}

func validateModalities(values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("modalities must contain at least one value")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return fmt.Errorf("modalities contains duplicate value %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

// Validate verifies the constraints shared by HTTP, RPC, and internal Doubao
// Realtime Duplex Workflow writes.
func (s DoubaoRealtimeDuplexWorkflowSpec) Validate() error {
	if strings.TrimSpace(s.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if err := validateOptionalNonBlank("voice", s.Voice); err != nil {
		return err
	}
	if s.Format != nil && !s.Format.Valid() {
		return fmt.Errorf("format %q is unsupported", *s.Format)
	}
	if s.InputFormat != nil && !s.InputFormat.Valid() {
		return fmt.Errorf("input_format %q is unsupported", *s.InputFormat)
	}
	if s.SampleRate != nil && !s.SampleRate.Valid() {
		return fmt.Errorf("sample_rate %d is unsupported", *s.SampleRate)
	}
	return validateDoubaoRealtimeDuplexNumbers(
		s.InputSampleRate, s.InputChannels, s.OutputSpeed, s.OutputLoudness,
	)
}

// Validate verifies the constraints shared by HTTP, RPC, and internal Doubao
// Realtime Duplex Workspace writes.
func (p DoubaoRealtimeDuplexWorkspaceParameters) Validate() error {
	if !p.AgentType.Valid() {
		return fmt.Errorf("agent_type %q is unsupported", p.AgentType)
	}
	if err := validateOptionalNonBlank("model", p.Model); err != nil {
		return err
	}
	if err := validateOptionalNonBlank("voice", p.Voice); err != nil {
		return err
	}
	if p.Format != nil && !p.Format.Valid() {
		return fmt.Errorf("format %q is unsupported", *p.Format)
	}
	if p.InputFormat != nil && !p.InputFormat.Valid() {
		return fmt.Errorf("input_format %q is unsupported", *p.InputFormat)
	}
	if p.SampleRate != nil && !p.SampleRate.Valid() {
		return fmt.Errorf("sample_rate %d is unsupported", *p.SampleRate)
	}
	return validateDoubaoRealtimeDuplexNumbers(
		p.InputSampleRate, p.InputChannels, p.OutputSpeed, p.OutputLoudness,
	)
}

func validateDoubaoRealtimeDuplexNumbers(inputSampleRate, inputChannels, outputSpeed, outputLoudness *int) error {
	if inputSampleRate != nil && (*inputSampleRate < 8000 || *inputSampleRate > 48000) {
		return fmt.Errorf("input_sample_rate must be between 8000 and 48000")
	}
	if inputChannels != nil && (*inputChannels < 1 || *inputChannels > 2) {
		return fmt.Errorf("input_channels must be between 1 and 2")
	}
	if outputSpeed != nil && (*outputSpeed < -50 || *outputSpeed > 100) {
		return fmt.Errorf("output_speed must be between -50 and 100")
	}
	if outputLoudness != nil && (*outputLoudness < -50 || *outputLoudness > 100) {
		return fmt.Errorf("output_loudness must be between -50 and 100")
	}
	return nil
}

func validateOptionalNonBlank(name string, value *string) error {
	if value != nil && strings.TrimSpace(*value) == "" {
		return fmt.Errorf("%s must not be blank", name)
	}
	return nil
}
