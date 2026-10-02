package mem0

import (
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestSelfHostedExtractionPreservesSpeakerAndSourceTime(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2023, 5, 8, 9, 0, 0, 0, time.FixedZone("source", 8*3600))
	input := memory.Observation{ObservedAt: observedAt, Turns: []memory.Turn{
		{Role: memory.RoleUser, Speaker: "Alice", Text: "I moved yesterday."},
		{Role: memory.RoleAssistant, Speaker: "Bob", Text: "I moved last year.", ObservedAt: observedAt.Add(time.Hour)},
	}}
	messages := mem0Messages(input, SelfHosted)
	if len(messages) != 2 || !strings.Contains(messages[0].Content, "2023-05-08T01:00:00Z") ||
		!strings.Contains(messages[0].Content, "Alice: I moved yesterday.") ||
		!strings.Contains(messages[1].Content, "2023-05-08T02:00:00Z") ||
		!strings.Contains(messages[1].Content, "Bob: I moved last year.") || messages[1].Role != memory.RoleAssistant {
		t.Fatalf("self-hosted extraction context = %+v", messages)
	}
	for _, flavor := range []Flavor{Platform, VolcPlatform} {
		platform := mem0Messages(input, flavor)
		if platform[0].Content != input.Turns[0].Text || platform[0].Name != "Alice" {
			t.Fatalf("%s message changed: %+v", flavor, platform[0])
		}
	}
}
