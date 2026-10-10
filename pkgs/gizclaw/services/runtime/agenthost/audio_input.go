package agenthost

import (
	"context"
	"fmt"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
)

// resolveASRModel reads only the owner Profile's binding for the current input
// mode. Omission means native audio. Stored Workspace input-source fields and
// legacy Workflow ASR configuration never participate in selection.
func resolveASRModel(ctx context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (string, error) {
	mode, err := apitypes.WorkspaceInput(workflow.Spec.Driver, ws.Parameters)
	if err != nil {
		return "", fmt.Errorf("workspace %q: %w", ws.Name, err)
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	if !ok {
		return "", nil
	}
	aliases := make([]string, 0, len(profile.Spec.Workflows))
	for alias := range profile.Spec.Workflows {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	selected, previous := "", ""
	for _, alias := range aliases {
		binding := profile.Spec.Workflows[alias]
		if binding.ResourceId != workflow.Id {
			continue
		}
		value := binding.PttAsrModel
		field := "ptt_asr_model"
		if mode == apitypes.WorkspaceInputModeRealtime {
			value = binding.RealtimeAsrModel
			field = "realtime_asr_model"
		}
		current := ""
		if value != nil {
			current = *value
			if err := runtimealias.Validate("ASR Model alias", current); err != nil {
				return "", fmt.Errorf("RuntimeProfile %q workflows.%s.%s: %w", profile.Id, alias, field, err)
			}
		}
		if previous != "" && selected != current {
			return "", fmt.Errorf("RuntimeProfile %q binds Workflow %q with conflicting %s in %q and %q", profile.Id, workflow.Id, field, previous, alias)
		}
		selected, previous = current, alias
	}
	return selected, nil
}
