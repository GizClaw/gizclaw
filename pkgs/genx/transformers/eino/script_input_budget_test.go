package eino

import (
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestScriptInputBudgetUsesMessageTextView(t *testing.T) {
	const limit = 262144
	audio := strings.Repeat("A", limit*2)
	message := &schema.Message{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
		{Type: schema.ChatMessagePartTypeAudioURL, Audio: &schema.MessageInputAudio{
			MessagePartCommon: schema.MessagePartCommon{Base64Data: &audio, MIMEType: "audio/opus"},
		}},
		{Type: schema.ChatMessagePartTypeText, Text: "current text"},
	}}
	script, err := compileScript(t.Context(), ScriptNode{
		Language: ScriptStarlark,
		Source: `def run(input):
    messages = input["messages"]
    return {"answer": messages[0]["content"] + ":" + messages[1]["parts"][0]["text"]}
`,
		Limits: ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: limit, MaxOutputBytes: 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := script.run(t.Context(), map[string]any{
		"messages": []*schema.Message{schema.UserMessage("earlier context"), message},
	}, map[string]StateType{"answer": StateString})
	if err != nil {
		t.Fatal(err)
	}
	if result["answer"] != "earlier context:current text" {
		t.Fatalf("Script text view changed: %v", result)
	}
	if len(message.UserInputMultiContent) != 2 || message.UserInputMultiContent[0].Audio.Base64Data != &audio {
		t.Fatal("Script projection mutated the original audio message")
	}
}

func TestScriptInputBudgetStillRejectsVisiblePayloads(t *testing.T) {
	const limit = 262144
	script, err := compileScript(t.Context(), ScriptNode{
		Language: ScriptStarlark, Source: `def run(input):
    return {"answer": "should not run"}
`,
		Limits: ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: limit, MaxOutputBytes: 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"text":            strings.Repeat("x", limit),
		"binary":          make([]byte, limit),
		"message content": []*schema.Message{schema.UserMessage(strings.Repeat("x", limit))},
		"message text parts": []*schema.Message{{Role: schema.User, UserInputMultiContent: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeText, Text: strings.Repeat("x", limit)},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := script.run(t.Context(), map[string]any{"value": value}, map[string]StateType{"answer": StateString})
			if err == nil || !strings.Contains(err.Error(), "Script input: encoded value exceeds 262144 bytes") {
				t.Fatalf("visible payload escaped input budget: %v", err)
			}
		})
	}
}
