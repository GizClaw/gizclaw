package peergenx

import "testing"

func TestSafetyFenceFillsOnlyThePlaceholder(t *testing.T) {
	for name, test := range map[string]struct {
		instructions, fence, want string
	}{
		"placed by Workflow": {"You are Momo.\n${safety_fence}\nAnswer briefly.", "Stay child safe.", "You are Momo.\nStay child safe.\nAnswer briefly."},
		"level off":          {"${safety_fence}\n\nYou are Momo.", "", "You are Momo."},
		"not referenced":     {"You are Momo.", "Stay child safe.", "You are Momo."},
		"no instructions":    {"", "Stay child safe.", ""},
	} {
		data := map[string]any{"instructions": test.instructions}
		if test.fence != "" {
			data[SafetyFenceParam] = test.fence
		}
		if got := fencedInstructions(data); got != test.want {
			t.Fatalf("%s: fenced = %q; want %q", name, got, test.want)
		}
	}
}
