package agenthost

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func resolveSafetyFence(ctx context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (string, error) {
	// A driver without a system prompt keeps a valid level as a no-op so one
	// device payload can carry the same level to every Workspace.
	if ws.Parameters == nil || workflow.Spec.Driver == apitypes.WorkflowDriverAstTranslate {
		return "", nil
	}
	level, err := ws.Parameters.SafetyFenceLevel()
	if err != nil {
		return "", fmt.Errorf("workspace %q: %w", ws.Name, err)
	}
	if level == nil || *level == apitypes.SafetyFenceLevelOff {
		return "", nil
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	if !ok {
		return "", fmt.Errorf("workspace %q safety_fence_level %q: bound RuntimeProfile is unavailable", ws.Name, *level)
	}
	var fence *apitypes.RuntimeProfileSafetyFence
	if profile.Spec.SafetyFences != nil {
		switch *level {
		case apitypes.SafetyFenceLevelGeneral:
			fence = profile.Spec.SafetyFences.General
		case apitypes.SafetyFenceLevelChild:
			fence = profile.Spec.SafetyFences.Child
		}
	}
	if fence == nil {
		return "", fmt.Errorf("workspace %q safety_fence_level %q: RuntimeProfile %q has no safety_fences.%s", ws.Name, *level, profile.Id, *level)
	}
	if err := apitypes.ValidateSafetyFencePrompt(fence.Prompt); err != nil {
		return "", fmt.Errorf("workspace %q safety_fence_level %q RuntimeProfile %q: %w", ws.Name, *level, profile.Id, err)
	}
	return fence.Prompt, nil
}
