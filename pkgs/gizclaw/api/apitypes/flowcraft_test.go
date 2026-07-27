package apitypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

const flowcraftSpecJSON = `{
  "graph": {
    "name": "Assistant",
    "entry": "prepare",
    "nodes": [
      {"id":"prepare","type":"script","config":{"source":"board.setVar('ready', true);"}},
      {"id":"route","type":"passthrough"},
      {"id":"recall","type":"memory_recall","config":{"query":{"text_from":"input"},"output":"memory_context","top_k":5}},
      {"id":"answer","type":"llm","publish":true,"config":{"model":"llm","max_tokens":2048}},
      {"id":"observe","type":"memory_observe","config":{"observations":[{"turns_from":"conversation"}],"wait_for_completion":false}}
    ],
    "edges": [
      {"from":"prepare","to":"route"},
      {"from":"route","to":"recall"},
      {"from":"recall","to":"answer"},
      {"from":"answer","to":"observe"},
      {"from":"observe","to":"__end__"}
    ]
  },
  "conversation":{"starts":"peer"},
  "voice_adapter":{"asr_model":"asr","default_voice":"narrator"}
}`

func TestFlowcraftWorkflowSpecJSONRoundTrip(t *testing.T) {
	var spec FlowcraftWorkflowSpec
	if err := json.Unmarshal([]byte(flowcraftSpecJSON), &spec); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip FlowcraftWorkflowSpec
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Graph.Entry != "prepare" || len(roundTrip.Graph.Nodes) != 5 {
		t.Fatalf("round trip = %#v", roundTrip)
	}
}

func TestFlowcraftWorkflowSpecYAMLDecode(t *testing.T) {
	var spec FlowcraftWorkflowSpec
	if err := yaml.Unmarshal([]byte(`
graph:
  name: Assistant
  entry: answer
  nodes:
    - id: answer
      type: llm
      publish: true
      config: {model: llm}
`), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Graph.Entry != "answer" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestFlowcraftWorkflowSpecRejectsInvalidConfig(t *testing.T) {
	for name, test := range map[string]struct {
		raw  string
		want string
	}{
		"empty":             {raw: `{}`, want: "graph: name is required"},
		"unknown top level": {raw: strings.TrimSuffix(flowcraftSpecJSON, "}") + `,"history":{}}`, want: "unknown field"},
		"tool names":        {raw: strings.Replace(flowcraftSpecJSON, `"max_tokens":2048`, `"max_tokens":2048,"tool_names":["echo"]`, 1), want: "unknown field"},
		"unknown node":      {raw: strings.Replace(flowcraftSpecJSON, `"type":"passthrough"`, `"type":"tool"`, 1), want: "unsupported"},
		"missing entry":     {raw: strings.Replace(flowcraftSpecJSON, `"entry": "prepare"`, `"entry": "missing"`, 1), want: "not a defined node"},
		"missing publisher": {raw: strings.Replace(flowcraftSpecJSON, `,"publish":true`, ``, 1), want: "publish=true"},
		"model resource ID": {raw: strings.Replace(flowcraftSpecJSON, `"model":"llm"`, `"model":"model/llm"`, 1), want: "RuntimeProfile alias"},
		"voice resource ID": {raw: strings.Replace(flowcraftSpecJSON, `"default_voice":"narrator"`, `"default_voice":"voice/narrator"`, 1), want: "RuntimeProfile alias"},
	} {
		t.Run(name, func(t *testing.T) {
			var spec FlowcraftWorkflowSpec
			err := json.Unmarshal([]byte(test.raw), &spec)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("json.Unmarshal() error = %v, want %q", err, test.want)
			}
		})
	}
}
