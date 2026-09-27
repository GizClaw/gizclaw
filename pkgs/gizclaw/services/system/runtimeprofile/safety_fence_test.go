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
		fences := apitypes.RuntimeProfileSafetyFences{"child": {Prompt: prompt}}
		input.Spec.SafetyFences = &fences
		item, err := normalizeProfile(input, "")
		if err != nil {
			t.Fatal(err)
		}
		if item.Revision == previous {
			t.Fatal("fence did not affect revision")
		}
		previous = item.Revision
		(*input.Spec.SafetyFences)["child"] = apitypes.RuntimeProfileSafetyFence{Prompt: "mutated"}
		if (*item.Spec.SafetyFences)["child"].Prompt != prompt {
			t.Fatal("normalization retained mutable caller pointer")
		}
	}
	for _, prompt := range []string{"", strings.Repeat("围", 4097)} {
		fences := apitypes.RuntimeProfileSafetyFences{"general": {Prompt: prompt}}
		input.Spec.SafetyFences = &fences
		if _, err := normalizeProfile(input, ""); err == nil {
			t.Fatal("accepted invalid prompt")
		}
	}
}

func TestSafetyFenceAcceptsFourProfileDefinedLevels(t *testing.T) {
	input := appConfigUpsert(nil)
	fences := apitypes.RuntimeProfileSafetyFences{
		"alpha":   {Prompt: "Complete alpha prompt"},
		"bravo":   {Prompt: "Complete bravo prompt"},
		"charlie": {Prompt: "Complete charlie prompt"},
		"delta":   {Prompt: "Complete delta prompt"},
	}
	input.Spec.SafetyFences = &fences
	item, err := normalizeProfile(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(*item.Spec.SafetyFences) != 4 {
		t.Fatalf("fence count = %d", len(*item.Spec.SafetyFences))
	}
	for _, id := range []string{"", "Bad", "two words"} {
		bad := apitypes.RuntimeProfileSafetyFences{id: {Prompt: "complete"}}
		input.Spec.SafetyFences = &bad
		if _, err := normalizeProfile(input, ""); err == nil {
			t.Fatalf("accepted invalid identifier %q", id)
		}
	}
}
