package apitypes

import (
	"encoding/json"
	"testing"
)

func TestWorkspaceInputDefaultsAndModes(t *testing.T) {
	for _, tc := range []struct {
		driver WorkflowDriver
		want   WorkspaceInputMode
	}{
		{WorkflowDriverEino, WorkspaceInputModePushToTalk},
		{WorkflowDriverDoubaoRealtime, WorkspaceInputModePushToTalk},
		{WorkflowDriverDoubaoRealtimeDuplex, WorkspaceInputModeRealtime},
		{WorkflowDriverDashscopeRealtime, WorkspaceInputModeRealtime},
	} {
		for _, raw := range []string{"", `{"agent_type":"` + string(tc.driver) + `"}`} {
			var parameters *WorkspaceParameters
			if raw != "" {
				if err := json.Unmarshal([]byte(raw), &parameters); err != nil {
					t.Fatal(err)
				}
			}
			got, err := WorkspaceInput(tc.driver, parameters)
			if err != nil || got != tc.want {
				t.Fatalf("driver=%s parameters=%s mode=%s error=%v", tc.driver, raw, got, err)
			}
		}
	}
	for _, mode := range []WorkspaceInputMode{WorkspaceInputModePushToTalk, WorkspaceInputModeRealtime, "invalid"} {
		var parameters WorkspaceParameters
		if err := json.Unmarshal([]byte(`{"agent_type":"eino","input":"`+string(mode)+`"}`), &parameters); err != nil {
			t.Fatal(err)
		}
		got, err := WorkspaceInput(WorkflowDriverEino, &parameters)
		if mode.Valid() && (err != nil || got != mode) || !mode.Valid() && err == nil {
			t.Fatalf("mode=%s got=%s error=%v", mode, got, err)
		}
	}
}
