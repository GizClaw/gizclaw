package einoconfig

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"strings"
	"testing"
)

func TestValidateUnknownSpeakerPolicy(t *testing.T) {
	fallback, blank := "story.default", " "
	preserve, normalize, bad := apitypes.VoiceAdapterUnknownSpeakerPolicyPreserve, apitypes.VoiceAdapterUnknownSpeakerPolicyDefaultVoice, apitypes.VoiceAdapterUnknownSpeakerPolicy("bad")
	for _, tc := range []struct {
		name      string
		policy    *apitypes.VoiceAdapterUnknownSpeakerPolicy
		voice     *string
		wantError string
	}{
		{"legacy", nil, nil, ""}, {"preserve without voice", &preserve, nil, ""}, {"normalize", &normalize, &fallback, ""}, {"missing default", &normalize, nil, "requires default_voice"}, {"blank default", &normalize, &blank, "requires default_voice"}, {"invalid", &bad, &fallback, "must be preserve or default_voice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateVoiceAdapter(apitypes.EinoWorkflowSpec{VoiceAdapter: &apitypes.VoiceAdapter{UnknownSpeakerPolicy: tc.policy, DefaultVoice: tc.voice}})
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("err=%v want %q", err, tc.wantError)
			}
		})
	}
	field := "continue"
	spec := apitypes.EinoWorkflowSpec{VoiceAdapter: &apitypes.VoiceAdapter{UnknownSpeakerPolicy: &normalize, DefaultVoice: &fallback}, Conversation: &apitypes.EinoConversation{ContinueFrom: &field}}
	if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "unknown_speaker_policy") {
		t.Fatalf("continuation error=%v", err)
	}
}
