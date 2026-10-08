package agenthost

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func audioInputWorkspace(t *testing.T, workflowID string, path *apitypes.AudioInputPath) apitypes.Workspace {
	t.Helper()
	ws := systemWorkspace("audio-workspace", workflowID, &apitypes.WorkspaceParameters{})
	if err := ws.Parameters.FromEinoWorkspaceParameters(apitypes.EinoWorkspaceParameters{
		AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino, AudioInput: path,
	}); err != nil {
		t.Fatal(err)
	}
	return ws
}

func audioInputProfile(bindings map[string]apitypes.RuntimeProfileWorkflowBinding) apitypes.RuntimeProfile {
	return apitypes.RuntimeProfile{Id: "owner-profile", Spec: apitypes.RuntimeProfileSpec{Workflows: bindings}}
}

func audioInputBinding(resourceID string, path *apitypes.AudioInputPath) apitypes.RuntimeProfileWorkflowBinding {
	return apitypes.RuntimeProfileWorkflowBinding{ResourceId: resourceID, AudioInput: path}
}

func TestResolveAudioInputPrefersWorkspaceOverProfile(t *testing.T) {
	asr, model := apitypes.AudioInputPathAsr, apitypes.AudioInputPathModel
	eino := apitypes.Workflow{Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino}}
	for _, test := range []struct {
		name      string
		workspace *apitypes.AudioInputPath
		profile   *apitypes.RuntimeProfile
		workflow  apitypes.Workflow
		want      *apitypes.AudioInputPath
	}{
		{name: "neither selects", workflow: eino},
		{name: "workspace only", workspace: &model, workflow: eino, want: &model},
		{
			name: "profile only", workflow: eino, want: &model,
			profile: new(audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{"assistant": audioInputBinding("assistant", &model)})),
		},
		{
			name: "workspace overrides profile", workspace: &asr, workflow: eino, want: &asr,
			profile: new(audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{"assistant": audioInputBinding("assistant", &model)})),
		},
		{
			name: "binding of another Workflow", workflow: eino,
			profile: new(audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{"other": audioInputBinding("other", &model)})),
		},
		{
			name: "aliases of one Workflow", workflow: eino, want: &model,
			profile: new(audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{
				"assistant": audioInputBinding("assistant", nil), "assistant-v2": audioInputBinding("assistant", &model),
			})),
		},
		{
			name:     "other driver ignores the profile",
			workflow: apitypes.Workflow{Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverDoubaoRealtime}},
			profile:  new(audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{"assistant": audioInputBinding("assistant", &model)})),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			if test.profile != nil {
				ctx = withRuntimeProfile(ctx, *test.profile)
			}
			got, err := resolveAudioInput(ctx, audioInputWorkspace(t, "assistant", test.workspace), test.workflow)
			if err != nil {
				t.Fatalf("resolveAudioInput() error = %v", err)
			}
			if (got == nil) != (test.want == nil) || got != nil && *got != *test.want {
				t.Fatalf("resolveAudioInput() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestResolveAudioInputRejectsUnusableSelections(t *testing.T) {
	asr, model, unknown := apitypes.AudioInputPathAsr, apitypes.AudioInputPathModel, apitypes.AudioInputPath("direct")
	eino := apitypes.Workflow{Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino}}

	if _, err := resolveAudioInput(t.Context(), audioInputWorkspace(t, "assistant", &unknown), eino); err == nil ||
		!strings.Contains(err.Error(), `workspace "audio-workspace": unsupported audio_input "direct"`) {
		t.Fatalf("unknown Workspace audio_input error = %v", err)
	}
	conflicting := audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{
		"assistant": audioInputBinding("assistant", &model), "assistant-v2": audioInputBinding("assistant", &asr),
	})
	if _, err := resolveAudioInput(withRuntimeProfile(t.Context(), conflicting), audioInputWorkspace(t, "assistant", nil), eino); err == nil ||
		!strings.Contains(err.Error(), "conflicting audio_input") {
		t.Fatalf("conflicting profile bindings error = %v", err)
	}
	invalid := audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{"assistant": audioInputBinding("assistant", &unknown)})
	if _, err := resolveAudioInput(withRuntimeProfile(t.Context(), invalid), audioInputWorkspace(t, "assistant", nil), eino); err == nil ||
		!strings.Contains(err.Error(), `workflows.assistant: unsupported audio_input "direct"`) {
		t.Fatalf("unknown profile audio_input error = %v", err)
	}
}

func TestResolverCarriesOwnerProfileAudioInput(t *testing.T) {
	model := apitypes.AudioInputPathModel
	profile := audioInputProfile(map[string]apitypes.RuntimeProfileWorkflowBinding{"assistant": audioInputBinding("assistant", &model)})
	ws := audioInputWorkspace(t, "assistant", nil)
	ws.OwnerPublicKey = new("owner")
	resolver := ServiceResolver{
		Workflows: fakeWorkflowService{items: map[string]apitypes.Workflow{
			"assistant": {Id: "assistant", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino}},
		}},
		RuntimeProfileForOwner: func(context.Context, string) (apitypes.RuntimeProfile, error) { return profile, nil },
	}
	spec, err := resolver.resolveWorkspace(withRuntimeProfile(t.Context(), apitypes.RuntimeProfile{Id: "wrong-profile"}), ws)
	if err != nil {
		t.Fatalf("resolveWorkspace() error = %v", err)
	}
	if spec.AudioInput == nil || *spec.AudioInput != model {
		t.Fatalf("Spec.AudioInput = %v, want %q", spec.AudioInput, model)
	}
}

func TestMergeWorkspaceStateCarriesAudioInput(t *testing.T) {
	state := apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateRunning}
	mergeWorkspaceState(&state, apitypes.PeerRunWorkspaceState{AudioInput: new(apitypes.AudioInputPathModel)})
	if state.AudioInput == nil || *state.AudioInput != apitypes.AudioInputPathModel {
		t.Fatalf("merged audio_input = %v", state.AudioInput)
	}
	mergeWorkspaceState(&state, apitypes.PeerRunWorkspaceState{})
	if state.AudioInput == nil {
		t.Fatal("merge without audio_input cleared the reported path")
	}
}
