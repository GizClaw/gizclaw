package agenthost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func audioInputWorkspace(t *testing.T, mode apitypes.WorkspaceInputMode, oldSource string) apitypes.Workspace {
	t.Helper()
	var parameters apitypes.WorkspaceParameters
	raw := `{"agent_type":"eino","input":"` + string(mode) + `","audio_input":"` + oldSource + `","ptt_asr_model":"attacker"}`
	if err := json.Unmarshal([]byte(raw), &parameters); err != nil {
		t.Fatal(err)
	}
	return systemWorkspace("audio-workspace", "assistant", &parameters)
}

func TestASRSelectionUsesProfileModeAndIgnoresWorkspaceAndWorkflow(t *testing.T) {
	profile := apitypes.RuntimeProfile{Id: "owner", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
		"assistant": {ResourceId: "assistant", PttAsrModel: new("ptt-asr"), RealtimeAsrModel: new("stream-asr")},
	}}}
	workflow := apitypes.Workflow{Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &apitypes.EinoWorkflowSpec{VoiceAdapter: &apitypes.VoiceAdapter{AsrModel: new("ignored-workflow-asr")}}}}
	for _, tc := range []struct {
		mode apitypes.WorkspaceInputMode
		want string
	}{{apitypes.WorkspaceInputModePushToTalk, "ptt-asr"}, {apitypes.WorkspaceInputModeRealtime, "stream-asr"}} {
		got, err := resolveASRModel(withRuntimeProfile(t.Context(), profile), audioInputWorkspace(t, tc.mode, "model"), workflow)
		if err != nil || got != tc.want {
			t.Fatalf("mode=%s ASR=%q error=%v", tc.mode, got, err)
		}
	}
	binding := profile.Spec.Workflows["assistant"]
	binding.PttAsrModel = nil
	profile.Spec.Workflows["assistant"] = binding
	got, err := resolveASRModel(withRuntimeProfile(t.Context(), profile), audioInputWorkspace(t, apitypes.WorkspaceInputModePushToTalk, "asr"), workflow)
	if err != nil || got != "" {
		t.Fatalf("omitted Profile Model must pass native audio: %q, %v", got, err)
	}
	if got, err := resolveASRModel(t.Context(), audioInputWorkspace(t, apitypes.WorkspaceInputModePushToTalk, "asr"), workflow); err != nil || got != "" {
		t.Fatalf("legacy Workflow ASR was used: %q %v", got, err)
	}
}

func TestASRSelectionRejectsConflictingBindings(t *testing.T) {
	profile := apitypes.RuntimeProfile{Id: "owner", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
		"assistant": {ResourceId: "assistant", PttAsrModel: new("asr")}, "other": {ResourceId: "assistant"},
	}}}
	_, err := resolveASRModel(withRuntimeProfile(t.Context(), profile), audioInputWorkspace(t, apitypes.WorkspaceInputModePushToTalk, ""), apitypes.Workflow{Id: "assistant"})
	if err == nil || !strings.Contains(err.Error(), "conflicting ptt_asr_model") {
		t.Fatalf("conflict error=%v", err)
	}
}

func TestASRSelectionUsesIntrinsicRealtimeModeWithoutParameters(t *testing.T) {
	profile := apitypes.RuntimeProfile{Id: "owner", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
		"assistant": {ResourceId: "assistant", PttAsrModel: new("ptt-asr"), RealtimeAsrModel: new("stream-asr")},
	}}}
	for _, driver := range []apitypes.WorkflowDriver{apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex} {
		got, err := resolveASRModel(withRuntimeProfile(t.Context(), profile), apitypes.Workspace{Name: "default"}, apitypes.Workflow{Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: driver}})
		if err != nil || got != "stream-asr" {
			t.Fatalf("driver=%s ASR=%q error=%v", driver, got, err)
		}
	}
}

func TestResolverCarriesOwnerProfileASRModel(t *testing.T) {
	profile := apitypes.RuntimeProfile{Id: "owner", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"assistant": {ResourceId: "assistant", PttAsrModel: new("asr")}}}}
	ws := audioInputWorkspace(t, apitypes.WorkspaceInputModePushToTalk, "model")
	ws.OwnerPublicKey = new("owner")
	resolver := ServiceResolver{Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{"assistant": {Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino}}}}, RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil }}
	spec, err := resolver.resolveWorkspace(withRuntimeProfile(t.Context(), apitypes.RuntimeProfile{Id: "wrong"}), ws)
	if err != nil || spec.ASRModel != "asr" {
		t.Fatalf("Spec.ASRModel=%q error=%v", spec.ASRModel, err)
	}
}

func TestMergeWorkspaceStateCarriesAudioInput(t *testing.T) {
	state := apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateRunning}
	mergeWorkspaceState(&state, apitypes.PeerRunWorkspaceState{AudioInput: new(apitypes.AudioInputPathModel)})
	if state.AudioInput == nil || *state.AudioInput != apitypes.AudioInputPathModel {
		t.Fatal("effective input path lost")
	}
}
