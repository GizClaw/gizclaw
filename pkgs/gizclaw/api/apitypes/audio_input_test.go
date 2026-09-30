package apitypes

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceParametersAudioInput(t *testing.T) {
	t.Parallel()
	for _, path := range []AudioInputPath{AudioInputPathAsr, AudioInputPathModel} {
		var parameters WorkspaceParameters
		if err := json.Unmarshal([]byte(`{"agent_type":"eino","audio_input":"`+string(path)+`"}`), &parameters); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		got, err := WorkspaceAudioInput(&parameters)
		if err != nil || got == nil || *got != path {
			t.Fatalf("WorkspaceAudioInput() = %v, %v; want %q", got, err, path)
		}
	}
	for _, body := range []string{`{"agent_type":"eino"}`, `{"agent_type":"flowcraft"}`, `{"agent_type":"sfu"}`} {
		var parameters WorkspaceParameters
		if err := json.Unmarshal([]byte(body), &parameters); err != nil {
			t.Fatalf("json.Unmarshal(%s) error = %v", body, err)
		}
		if got, err := parameters.AudioInput(); err != nil || got != nil {
			t.Fatalf("AudioInput(%s) = %v, %v; want nil", body, got, err)
		}
	}
	if got, err := WorkspaceAudioInput(nil); err != nil || got != nil {
		t.Fatalf("WorkspaceAudioInput(nil) = %v, %v; want nil", got, err)
	}
}

func TestWorkspaceParametersAudioInputRejectsUnknownPath(t *testing.T) {
	t.Parallel()
	var parameters WorkspaceParameters
	if err := json.Unmarshal([]byte(`{"agent_type":"eino","audio_input":"direct"}`), &parameters); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, err := parameters.AudioInput(); err == nil {
		t.Fatal("AudioInput() accepted an unknown path")
	}
	if err := ValidateAudioInputPath(nil); err != nil {
		t.Fatalf("ValidateAudioInputPath(nil) error = %v", err)
	}
}
