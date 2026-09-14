package apitypes

import "testing"

func TestValidateSpeakerVoices(t *testing.T) {
	for _, tc := range []struct {
		name, alias string
		valid       bool
	}{
		{"旁白", "story.narrator", true}, {"", "story.narrator", false}, {"  ", "story.narrator", false}, {"【狐", "story.fox", false}, {"狐】", "story.fox", false}, {"狐", "Bad_Alias", false}, {"狐", "", false},
	} {
		voices := map[string]string{tc.name: tc.alias}
		if err := ValidateSpeakerVoices(&voices); (err == nil) != tc.valid {
			t.Errorf("%q=%q: %v", tc.name, tc.alias, err)
		}
	}
}
