package einoconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	valid := decodeSpec(t, `{
		"graph": {
			"name": "validation-test",
			"compile": {"node_trigger_mode": "any_predecessor"},
			"state": {"fields": [{"name": "answer", "type": "string", "merge": "replace"}]},
			"nodes": [{
				"id": "answer",
				"type": "passthrough",
				"inputs": {"value": {"from": "input.text"}},
				"outputs": {"value": "answer"}
			}],
			"edges": [{"from": "start", "to": "answer"}, {"from": "answer", "to": "end"}],
			"branches": [],
			"outputs": [{
				"node": "answer",
				"field": "answer",
				"name": "assistant",
				"mime_type": "text/plain",
				"primary": true
			}]
		}
	}`)
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}

	t.Run("voice adapter", func(t *testing.T) {
		spec := cloneSpec(t, valid)
		asr, voice := "speech.asr", "speech.voice"
		nodes := map[string]string{"answer": voice}
		spec.VoiceAdapter = &apitypes.VoiceAdapter{
			AsrModel:     &asr,
			DefaultVoice: &voice,
			NodeVoices:   &nodes,
		}
		if err := Validate(spec); err != nil {
			t.Fatalf("Validate(voice adapter) error = %v", err)
		}

		tests := []struct {
			name    string
			mutate  func(*apitypes.EinoWorkflowSpec)
			wantErr string
		}{
			{name: "empty", mutate: func(spec *apitypes.EinoWorkflowSpec) {
				spec.VoiceAdapter = &apitypes.VoiceAdapter{}
			}, wantErr: "must configure"},
			{name: "invalid alias", mutate: func(spec *apitypes.EinoWorkflowSpec) {
				invalid := "INVALID ALIAS"
				spec.VoiceAdapter = &apitypes.VoiceAdapter{AsrModel: &invalid}
			}, wantErr: "asr_model"},
			{name: "unknown node", mutate: func(spec *apitypes.EinoWorkflowSpec) {
				mapped := map[string]string{"missing": voice}
				spec.VoiceAdapter = &apitypes.VoiceAdapter{NodeVoices: &mapped}
			}, wantErr: "no text/plain graph output"},
			{name: "non-text output", mutate: func(spec *apitypes.EinoWorkflowSpec) {
				spec.Graph.Outputs[0].MimeType = "application/octet-stream"
				mapped := map[string]string{"answer": voice}
				spec.VoiceAdapter = &apitypes.VoiceAdapter{NodeVoices: &mapped}
			}, wantErr: "no text/plain graph output"},
		}
		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				candidate := cloneSpec(t, valid)
				testCase.mutate(&candidate)
				if err := Validate(candidate); err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("Validate() error = %v, want %q", err, testCase.wantErr)
				}
			})
		}
	})

	t.Run("valid memory graph", func(t *testing.T) {
		spec := decodeSpec(t, `{
			"graph": {
				"name": "memory",
				"compile": {"node_trigger_mode": "any_predecessor"},
				"state": {"fields": [
					{"name": "recalled", "type": "string", "merge": "replace"},
					{"name": "answer", "type": "string", "merge": "replace"}
				]},
				"nodes": [
					{
						"id": "recall",
						"type": "memory_recall",
						"query_from": "input.text",
						"output": "recalled",
						"top_k": 5
					},
					{
						"id": "answer",
						"type": "passthrough",
						"inputs": {"value": {"from": "recalled"}},
						"outputs": {"value": "answer"}
					},
					{
						"id": "observe",
						"type": "memory_observe",
						"facts": [{"text_from": "answer"}],
						"wait_for_completion": true
					}
				],
				"edges": [
					{"from": "start", "to": "recall"},
					{"from": "recall", "to": "answer"},
					{"from": "answer", "to": "observe"},
					{"from": "observe", "to": "end"}
				],
				"branches": [],
				"outputs": [{
					"node": "answer",
					"field": "answer",
					"name": "assistant",
					"mime_type": "text/plain",
					"primary": true
				}]
			}
		}`)
		if err := Validate(spec); err != nil {
			t.Fatalf("Validate(memory graph) error = %v", err)
		}
	})

	t.Run("invalid edge", func(t *testing.T) {
		spec := cloneSpec(t, valid)
		spec.Graph.Edges[1].To = "missing"
		if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "unknown or invalid endpoint") {
			t.Fatalf("Validate(invalid edge) error = %v", err)
		}
	})
	t.Run("invalid memory", func(t *testing.T) {
		spec := decodeSpec(t, `{
			"graph": {
				"name": "invalid-memory",
				"compile": {"node_trigger_mode": "any_predecessor"},
				"state": {"fields": [{"name": "answer", "type": "string", "merge": "replace"}]},
				"nodes": [{
					"id": "recall",
					"type": "memory_recall",
					"query_from": "input.text",
					"output": "answer",
					"top_k": 0
				}],
				"edges": [{"from": "start", "to": "recall"}, {"from": "recall", "to": "end"}],
				"branches": [],
				"outputs": []
			}
		}`)
		if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "MemoryRecall") {
			t.Fatalf("Validate(invalid memory) error = %v", err)
		}
	})
	t.Run("negative script execution limit", func(t *testing.T) {
		spec := decodeSpec(t, `{
			"graph": {
				"name": "negative-script-limit",
				"compile": {"node_trigger_mode": "any_predecessor"},
				"state": {"fields": [{"name": "answer", "type": "string", "merge": "replace"}]},
				"nodes": [{
					"id": "answer",
					"type": "script",
					"inputs": {},
					"outputs": {"value": "answer"},
					"language": "starlark",
					"entrypoint": "main",
					"source": "def main(inputs):\n    return {\"value\": \"ok\"}",
					"limits": {
						"max_execution_steps": -1,
						"timeout": "1s",
						"max_input_bytes": 1024,
						"max_output_bytes": 1024
					}
				}],
				"edges": [{"from": "start", "to": "answer"}, {"from": "answer", "to": "end"}],
				"branches": [],
				"outputs": [{
					"node": "answer",
					"field": "answer",
					"name": "assistant",
					"mime_type": "text/plain",
					"primary": true
				}]
			}
		}`)
		if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "max_execution_steps must be positive") {
			t.Fatalf("Validate(negative script execution limit) error = %v", err)
		}
	})
}

func TestMapGraphPromptMessageForms(t *testing.T) {
	t.Parallel()
	spec := decodeSpec(t, `{
		"graph": {
			"name": "prompt-message-forms",
			"compile": {"node_trigger_mode": "any_predecessor"},
			"state": {"fields": [{"name": "messages", "type": "messages", "merge": "replace"}]},
			"nodes": [{
				"id": "prompt",
				"type": "prompt",
				"inputs": {"history": {"from": "input.messages"}},
				"outputs": {"messages": "messages"},
				"format": "f_string",
				"messages": [
					{"role": "system", "template": "Be helpful."},
					{"placeholder": "history", "optional": true}
				]
			}],
			"edges": [{"from": "start", "to": "prompt"}, {"from": "prompt", "to": "end"}],
			"branches": [],
			"outputs": [{"node": "prompt", "field": "messages", "name": "assistant", "mime_type": "application/json", "primary": true}]
		}
	}`)

	graph, err := MapGraph(spec.Graph)
	if err != nil {
		t.Fatalf("MapGraph() error = %v", err)
	}
	messages := graph.Nodes[0].Prompt.Messages
	if len(messages) != 2 {
		t.Fatalf("prompt messages = %#v, want two", messages)
	}
	if messages[0].Role != "system" || messages[0].Template != "Be helpful." || messages[0].Placeholder != "" {
		t.Fatalf("role/template message = %#v", messages[0])
	}
	if messages[1].Role != "" || messages[1].Template != "" || messages[1].Placeholder != "history" || !messages[1].Optional {
		t.Fatalf("placeholder message = %#v", messages[1])
	}
}

func decodeSpec(t testing.TB, data string) apitypes.EinoWorkflowSpec {
	t.Helper()
	var spec apitypes.EinoWorkflowSpec
	if err := json.Unmarshal([]byte(data), &spec); err != nil {
		t.Fatalf("decode Eino Workflow: %v", err)
	}
	return spec
}

func cloneSpec(t testing.TB, source apitypes.EinoWorkflowSpec) apitypes.EinoWorkflowSpec {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatalf("encode Eino Workflow: %v", err)
	}
	return decodeSpec(t, string(data))
}
