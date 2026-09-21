package peergenx

import "strings"

// SafetyFenceParam is the transformer pattern parameter carrying the Workspace
// safety fence prompt selected from the RuntimeProfile. Builders substitute it
// for SafetyFencePlaceholder in the instructions they pass to the provider.
const SafetyFenceParam = "safety_fence"

// SafetyFencePlaceholder marks where realtime instructions take the safety
// fence. Realtime Workflows have no Graph, so their instructions template is
// the only place a Workflow decides whether and where the fence applies.
const SafetyFencePlaceholder = "${safety_fence}"

// fencedInstructions replaces every SafetyFencePlaceholder in the pattern
// instructions with the safety fence, or with nothing when no fence is
// selected. Instructions without the placeholder are passed through: GizClaw
// never places the fence on its own.
func fencedInstructions(data map[string]any) string {
	instructions := mapString(data, "instructions")
	if !strings.Contains(instructions, SafetyFencePlaceholder) {
		return instructions
	}
	return strings.TrimSpace(strings.ReplaceAll(instructions, SafetyFencePlaceholder, mapString(data, SafetyFenceParam)))
}
