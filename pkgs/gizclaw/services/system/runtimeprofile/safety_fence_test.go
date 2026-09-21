package runtimeprofile

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestSafetyFenceValidationAndRevision(t *testing.T) {
	input := appConfigUpsert(nil)
	previous := ""
	for _, prompt := range []string{"general", "child", " \n", strings.Repeat("围", 4096)} {
		input.Spec.SafetyFences = &apitypes.RuntimeProfileSafetyFences{Child: &apitypes.RuntimeProfileSafetyFence{Prompt: prompt}}
		item, err := normalizeProfile(input, "")
		if err != nil {
			t.Fatal(err)
		}
		if item.Revision == previous {
			t.Fatal("fence did not affect revision")
		}
		previous = item.Revision
		input.Spec.SafetyFences.Child.Prompt = "mutated"
		if item.Spec.SafetyFences.Child.Prompt != prompt {
			t.Fatal("normalization retained mutable caller pointer")
		}
	}
	for _, prompt := range []string{"", strings.Repeat("围", 4097)} {
		input.Spec.SafetyFences = &apitypes.RuntimeProfileSafetyFences{General: &apitypes.RuntimeProfileSafetyFence{Prompt: prompt}}
		if _, err := normalizeProfile(input, ""); err == nil {
			t.Fatal("accepted invalid prompt")
		}
	}
}
