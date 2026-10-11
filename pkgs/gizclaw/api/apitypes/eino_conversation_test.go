package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEinoConversationIgnoresStarts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, input string
		want        *EinoConversationInitiative
	}{
		{"old agent", `{"starts":"agent"}`, nil},
		{"old peer", `{"starts":"peer"}`, nil},
		{"old object", `{"starts":{"obsolete":true}}`, nil},
		{"old list", `{"starts":["agent"]}`, nil},
		{"old unrepresentable number", `{"starts":1e999}`, nil},
		{"old null", `{"starts":null}`, nil},
		{"initiative agent", `{"initiative":"agent","starts":"peer"}`, new(EinoConversationInitiativeAgent)},
		{"initiative peer", `{"initiative":"peer","starts":"agent"}`, new(EinoConversationInitiativePeer)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var conversation EinoConversation
			if err := json.Unmarshal([]byte(test.input), &conversation); err != nil {
				t.Fatal(err)
			}
			if conversation.Starts != nil {
				t.Fatal("retained an ignored starts value")
			}
			initiative := conversation.Initiative
			if (initiative == nil) != (test.want == nil) || initiative != nil && *initiative != *test.want {
				t.Fatalf("initiative = %v, want %v", initiative, test.want)
			}
			encoded, err := json.Marshal(conversation)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), `"starts"`) {
				t.Fatalf("encoded ignored field: %s", encoded)
			}
		})
	}
}

func TestEinoConversationMarshalDropsStarts(t *testing.T) {
	t.Parallel()
	conversation := EinoConversation{
		Starts: "agent", Initiative: new(EinoConversationInitiativePeer), ContinueFrom: new("keep-speaking"),
	}
	encoded, err := json.Marshal(conversation)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"continue_from":"keep-speaking","initiative":"peer"}` {
		t.Fatalf("encoded conversation = %s", encoded)
	}
}

func TestEinoConversationRejectsOtherUnknownFields(t *testing.T) {
	t.Parallel()
	var conversation EinoConversation
	if err := json.Unmarshal([]byte(`{"starts":"agent","start":"agent"}`), &conversation); err == nil || !strings.Contains(err.Error(), `unknown field "start"`) {
		t.Fatalf("unknown field error = %v", err)
	}
}
