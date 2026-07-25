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

	t.Run("invalid edge", func(t *testing.T) {
		spec := cloneSpec(t, valid)
		spec.Graph.Edges[1].To = "missing"
		if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "unknown or invalid endpoint") {
			t.Fatalf("Validate(invalid edge) error = %v", err)
		}
	})
	t.Run("invalid memory", func(t *testing.T) {
		spec := cloneSpec(t, valid)
		spec.Memory = &apitypes.EinoMemory{Recall: &[]apitypes.EinoMemoryRecall{{
			QueryFrom: "input.text",
			Output:    "answer",
			TopK:      0,
		}}}
		if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "Memory.Recall[0] is invalid") {
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
