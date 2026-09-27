package agenthost

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func resolveSafetyFence(ctx context.Context, ws apitypes.Workspace, workflow apitypes.Workflow) (string, error) {
	// ASTTranslate has no system-prompt input. It retains the selection but does
	// not resolve a prompt or inject a variable.
	if workflow.Spec.Driver == apitypes.WorkflowDriverAstTranslate {
		return "", nil
	}
	var level *apitypes.SafetyFenceLevel
	if ws.Parameters != nil {
		var err error
		level, err = ws.Parameters.SafetyFenceLevel()
		if err != nil {
			return "", fmt.Errorf("workspace %q: %w", ws.Name, err)
		}
	}
	if level == nil {
		// Existing profiles without fences keep their pre-feature behavior.
		// Once a profile defines levels, its Workspace must select one.
		if profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile); !ok || profile.Spec.SafetyFences == nil {
			return "", nil
		}
		return "", fmt.Errorf("workspace %q has no safety_fence_level selection", ws.Name)
	}
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	if !ok {
		return "", fmt.Errorf("workspace %q safety_fence_level %q: bound RuntimeProfile is unavailable", ws.Name, *level)
	}
	var fence apitypes.RuntimeProfileSafetyFence
	var found bool
	if profile.Spec.SafetyFences != nil {
		fence, found = (*profile.Spec.SafetyFences)[*level]
	}
	if !found {
		return "", fmt.Errorf("workspace %q safety_fence_level %q: RuntimeProfile %q has no safety_fences.%s", ws.Name, *level, profile.Id, *level)
	}
	if err := apitypes.ValidateSafetyFencePrompt(fence.Prompt); err != nil {
		return "", fmt.Errorf("workspace %q safety_fence_level %q RuntimeProfile %q: %w", ws.Name, *level, profile.Id, err)
	}
	return fence.Prompt, nil
}
