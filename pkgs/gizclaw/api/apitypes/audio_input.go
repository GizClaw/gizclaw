package apitypes

import "fmt"

// ValidateAudioInputPath rejects a present path outside the AudioInputPath
// enum. Absent is valid.
func ValidateAudioInputPath(value *AudioInputPath) error {
	if value != nil && !value.Valid() {
		return fmt.Errorf("unsupported audio_input %q", *value)
	}
	return nil
}

// AudioInput returns the validated audio input path stored on an Eino
// Workspace parameter variant. Other variants and absent values return nil.
func (t WorkspaceParameters) AudioInput() (*AudioInputPath, error) {
	discriminator, err := t.Discriminator()
	if err != nil {
		return nil, err
	}
	if WorkflowDriver(discriminator) != WorkflowDriverEino {
		return nil, nil
	}
	parameters, err := t.AsEinoWorkspaceParameters()
	if err != nil {
		return nil, err
	}
	if err := ValidateAudioInputPath(parameters.AudioInput); err != nil {
		return nil, err
	}
	return parameters.AudioInput, nil
}

// WorkspaceAudioInput is AudioInput for optional parameters; nil parameters
// select no path.
func WorkspaceAudioInput(parameters *WorkspaceParameters) (*AudioInputPath, error) {
	if parameters == nil {
		return nil, nil
	}
	return parameters.AudioInput()
}
