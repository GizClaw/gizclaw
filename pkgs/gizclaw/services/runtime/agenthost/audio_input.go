package agenthost

import (
	"context"
	"fmt"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// resolveAudioInput returns the audio input path selected for one Eino
// Workspace: its own audio_input parameter, else the audio_input of the owner
// RuntimeProfile's binding of the Workflow. Nil leaves the Workflow default.
func resolveAudioInput(ctx context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (*apitypes.AudioInputPath, error) {
	if workflow.Spec.Driver != apitypes.WorkflowDriverEino {
		return nil, nil
	}
	selected, err := apitypes.WorkspaceAudioInput(ws.Parameters)
	if err != nil {
		return nil, fmt.Errorf("workspace %q: %w", ws.Name, err)
	}
	if selected != nil {
		return selected, nil
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	if !ok {
		return nil, nil
	}
	selected, err = profileWorkflowAudioInput(profile, workflow.Id)
	if err != nil {
		return nil, fmt.Errorf("workspace %q: %w", ws.Name, err)
	}
	return selected, nil
}

// profileWorkflowAudioInput returns the audio_input the profile selects for a
// Workflow resource. A Workspace stores the Workflow ID rather than the alias
// it was created through, so every binding of that Workflow must agree.
func profileWorkflowAudioInput(profile apitypes.RuntimeProfile, workflowID string) (*apitypes.AudioInputPath, error) {
	aliases := make([]string, 0, len(profile.Spec.Workflows))
	for alias := range profile.Spec.Workflows {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	var selected *apitypes.AudioInputPath
	for _, alias := range aliases {
		binding := profile.Spec.Workflows[alias]
		if binding.ResourceId != workflowID || binding.AudioInput == nil {
			continue
		}
		if err := apitypes.ValidateAudioInputPath(binding.AudioInput); err != nil {
			return nil, fmt.Errorf("RuntimeProfile %q workflows.%s: %w", profile.Id, alias, err)
		}
		if selected != nil && *selected != *binding.AudioInput {
			return nil, fmt.Errorf("RuntimeProfile %q binds Workflow %q with conflicting audio_input %q and %q", profile.Id, workflowID, *selected, *binding.AudioInput)
		}
		selected = new(*binding.AudioInput)
	}
	return selected, nil
}
