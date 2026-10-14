package admincmd

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestAdminValidateUnknownSpeakerPolicy(t *testing.T) {
	for _, kind := range []string{"eino", "eino-sequential"} {
		original, err := os.ReadFile("../../../../tests/gizclaw-e2e/testdata/speaker-segments/" + kind + ".json")
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name, policy   string
			missingDefault bool
			wantError      string
		}{
			{name: "default fallback", policy: "default_voice"},
			{name: "explicit preserve", policy: "preserve"},
			{name: "preserve without default", policy: "preserve", missingDefault: true},
			{name: "missing default", policy: "default_voice", missingDefault: true, wantError: "requires default_voice"},
			{name: "unknown policy", policy: "unsupported", wantError: "unknown_speaker_policy"},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				var spec map[string]any
				if err := json.Unmarshal(original, &spec); err != nil {
					t.Fatal(err)
				}
				adapter := spec["eino"].(map[string]any)["voice_adapter"].(map[string]any)
				adapter["unknown_speaker_policy"] = tc.policy
				if tc.missingDefault {
					delete(adapter, "default_voice")
				}
				resource := map[string]any{"apiVersion": "gizclaw.admin/v1alpha1", "kind": "Workflow", "metadata": map[string]any{"id": "speaker-test"}, "spec": spec}
				data, err := json.Marshal(resource)
				if err != nil {
					t.Fatal(err)
				}
				cmd := NewCmd()
				cmd.SetIn(strings.NewReader(string(data)))
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				cmd.SetArgs([]string{"validate", "-f", "-"})
				err = cmd.Execute()
				if tc.wantError == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error=%v want %q", err, tc.wantError)
				}
			})
		}
	}
}
