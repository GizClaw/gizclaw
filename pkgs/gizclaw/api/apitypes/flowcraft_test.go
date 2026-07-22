package apitypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

const flowcraftSpecJSON = `{
  "agent": {
    "id": "assistant",
    "name": "Assistant",
    "graph": {
      "name": "Assistant",
      "entry": "prepare",
      "nodes": [
        {"id":"prepare","type":"script","config":{"source":"board.setVar('ready', true);"}},
        {"id":"route","type":"passthrough"},
        {"id":"answer","type":"llm","publish":true,"config":{"model":"llm","max_tokens":2048}}
      ],
      "edges": [
        {"from":"prepare","to":"route"},
        {"from":"route","to":"answer"},
        {"from":"answer","to":"__end__"}
      ]
    }
  },
  "conversation":{"starts":"peer"},
  "memory":{"enabled":true,"extract":{"model":"extractor"},"write":{"mode":"async_semantic","save_conversation":true,"board_facts":[{"board_var":"state","kind":"state","subject":"story","predicate":"progress","object":"origin","entities":["story","origin"]}]}},
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
	if roundTrip.Agent.Graph.Entry != "prepare" || len(roundTrip.Agent.Graph.Nodes) != 3 {
		t.Fatalf("round trip = %#v", roundTrip)
	}
	if roundTrip.Memory == nil || roundTrip.Memory.Write == nil || roundTrip.Memory.Write.BoardFacts == nil || len(*roundTrip.Memory.Write.BoardFacts) != 1 {
		t.Fatalf("round trip board facts = %#v", roundTrip.Memory)
	}
	fact := (*roundTrip.Memory.Write.BoardFacts)[0]
	if fact.Object == nil || *fact.Object != "origin" || fact.Entities == nil || len(*fact.Entities) != 2 {
		t.Fatalf("round trip board fact = %#v", fact)
	}
}

func TestFlowcraftWorkflowSpecYAMLDecode(t *testing.T) {
	var spec FlowcraftWorkflowSpec
	if err := yaml.Unmarshal([]byte(`
agent:
  id: assistant
  name: Assistant
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
	if spec.Agent.Graph.Entry != "answer" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestFlowcraftWorkflowSpecRejectsInvalidConfig(t *testing.T) {
	for name, test := range map[string]struct {
		raw  string
		want string
	}{
		"empty":             {raw: `{}`, want: "agent.id is required"},
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
