package eino

import (
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestScriptRetainsTextMessageParts(t *testing.T) {
	script, err := compileScript(t.Context(), ScriptNode{Language: ScriptStarlark,
		Source: `def run(input):
    text = ""
    for message in input["messages"]:
        if message.get("role") == "user":
            text = message.get("content", "")
            if not text:
                text = "".join([part.get("text", "") for part in message.get("parts", []) if part.get("type") == "text"])
    return {"answer": text}
`, Limits: ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 4096, MaxOutputBytes: 4096}})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []*schema.Message{
		{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeText, Text: "first "},
			{Type: schema.ChatMessagePartTypeText, Text: "second"},
		}},
		{Role: schema.User, MultiContent: []schema.ChatMessagePart{
			{Type: schema.ChatMessagePartTypeText, Text: "first "},
			{Type: schema.ChatMessagePartTypeText, Text: "second"},
		}},
	} {
		result, err := script.run(t.Context(), map[string]any{"messages": []*schema.Message{message}}, map[string]StateType{"answer": StateString})
		if err != nil || result["answer"] != "first second" {
			t.Fatalf("message-parts fallback = %v, %v", result, err)
		}
	}
}
