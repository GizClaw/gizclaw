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
      {"id":"prepare","type":"script","config":{"runtime":"js","source":"board.setVar('ready', true);"}},
      {"id":"route","type":"passthrough"},
      {"id":"recall","type":"memory_recall","config":{"query":{"text_from":"input"},"output":"memory_context","top_k":5}},
      {"id":"answer","type":"inference","publish":true,"config":{"model":{"id":{"provider":"gizclaw","name":"llm"}},"messages_channel":"answer","stream":true,"intent":{"text":{"max_output_tokens":2048}}}},
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
  "memory_hooks":{
    "context":{"query":{"current_message":true},"budget":{"max_items":5},"output":"memory_items","render":{"output":"memory_context"}},
    "turn":{"channel":"answer"}
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
	if roundTrip.Graph.Entry != "prepare" || len(roundTrip.Graph.Nodes) != 5 || roundTrip.MemoryHooks == nil {
		t.Fatalf("round trip = %#v", roundTrip)
	}
}

func TestFlowcraftWorkflowSpecAcceptsDottedRuntimeAliases(t *testing.T) {
	raw := strings.NewReplacer(
		`"name":"llm"`, `"name":"pet-care.model"`,
		`"asr_model":"asr"`, `"asr_model":"pet-care.asr"`,
		`"default_voice":"narrator"`, `"default_voice":"pet-care.pet","node_voices":{"answer":"pet-care.answer"}`,
	).Replace(flowcraftSpecJSON)

	var spec FlowcraftWorkflowSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if spec.VoiceAdapter == nil || spec.VoiceAdapter.AsrModel == nil || *spec.VoiceAdapter.AsrModel != "pet-care.asr" ||
		spec.VoiceAdapter.DefaultVoice == nil || *spec.VoiceAdapter.DefaultVoice != "pet-care.pet" ||
		spec.VoiceAdapter.NodeVoices == nil || (*spec.VoiceAdapter.NodeVoices)["answer"] != "pet-care.answer" {
		t.Fatalf("voice_adapter = %#v", spec.VoiceAdapter)
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"name":"pet-care.model"`) {
		t.Fatalf("encoded spec does not preserve dotted model alias: %s", encoded)
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
      type: inference
      publish: true
      config:
        model: {id: {provider: gizclaw, name: pet-care.model}}
        messages_channel: answer
        stream: true
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
		"legacy llm node":   {raw: strings.Replace(flowcraftSpecJSON, `"type":"inference"`, `"type":"llm"`, 1), want: "unsupported"},
		"scalar model":      {raw: strings.Replace(flowcraftSpecJSON, `"model":{"id":{"provider":"gizclaw","name":"llm"}}`, `"model":"llm"`, 1), want: "cannot unmarshal"},
		"tool names":        {raw: strings.Replace(flowcraftSpecJSON, `"messages_channel":"answer"`, `"messages_channel":"answer","tool_names":["echo"]`, 1), want: "unknown field"},
		"invalid token limit": {
			raw: strings.Replace(flowcraftSpecJSON, `"max_output_tokens":2048`, `"max_output_tokens":0`, 1), want: "must be positive",
		},
		"unknown text intent": {
			raw: strings.Replace(flowcraftSpecJSON, `"max_output_tokens":2048`, `"max_output_tokens":2048,"seed":1`, 1), want: "unknown field",
		},
		"unsupported memory dataset IDs": {
			raw: strings.Replace(flowcraftSpecJSON, `"budget":{"max_items":5}`, `"dataset_ids":["docs"],"budget":{"max_items":5}`, 1), want: "unknown field",
		},
		"unsupported recent-only memory query": {
			raw: strings.Replace(flowcraftSpecJSON, `"current_message":true`, `"recent_only":true`, 1), want: "unknown field",
		},
		"unknown node":      {raw: strings.Replace(flowcraftSpecJSON, `"type":"passthrough"`, `"type":"tool"`, 1), want: "unsupported"},
		"missing entry":     {raw: strings.Replace(flowcraftSpecJSON, `"entry": "prepare"`, `"entry": "missing"`, 1), want: "not a defined node"},
		"missing publisher": {raw: strings.Replace(flowcraftSpecJSON, `,"publish":true`, ``, 1), want: "publish=true"},
		"model resource ID": {raw: strings.Replace(flowcraftSpecJSON, `"name":"llm"`, `"name":"model/llm"`, 1), want: "RuntimeProfile alias"},
		"voice resource ID": {raw: strings.Replace(flowcraftSpecJSON, `"default_voice":"narrator"`, `"default_voice":"voice/narrator"`, 1), want: "RuntimeProfile alias"},
		"empty ASR alias":   {raw: strings.Replace(flowcraftSpecJSON, `"asr_model":"asr"`, `"asr_model":""`, 1), want: "1-63 bytes"},
		"blank voice alias": {raw: strings.Replace(flowcraftSpecJSON, `"default_voice":"narrator"`, `"default_voice":" "`, 1), want: "1-63 bytes"},
		"overlong model alias": {
			raw:  strings.Replace(flowcraftSpecJSON, `"name":"llm"`, `"name":"`+strings.Repeat("a", 64)+`"`, 1),
			want: "1-63 bytes",
		},
		"empty observation": {
			raw:  strings.Replace(flowcraftSpecJSON, `{"turns_from":"conversation"}`, `{}`, 1),
			want: "must select exactly one",
		},
		"empty observation facts": {
			raw:  strings.Replace(flowcraftSpecJSON, `{"turns_from":"conversation"}`, `{"facts":[]}`, 1),
			want: "must select exactly one",
		},
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
