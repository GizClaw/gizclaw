package einoconfig

import (
	"testing"
)

func TestPublicStructuredOutputMapsAndValidates(t *testing.T) {
	public := decodeSpec(t, `{"audio_input":"model","graph":{
		"name":"structured","compile":{"node_trigger_mode":"any_predecessor"},
		"state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},
		"nodes":[{"id":"control","type":"chat_model","model":"native",
			"inputs":{"messages":{"from":"input.messages"}},"outputs":{"text":"answer"},
			"structured_output":{"name":"turn_intent","schema":{"type":"object","additionalProperties":false,
			"required":["mode"],"properties":{"mode":{"type":"string","enum":["plot","research"]}}}}}],
		"edges":[{"from":"start","to":"control"},{"from":"control","to":"end"}],"branches":[],
		"outputs":[{"node":"control","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]
	}}`)
	if err := Validate(public); err != nil {
		t.Fatal(err)
	}
	graph, err := MapGraph(public.Graph)
	if err != nil {
		t.Fatal(err)
	}
	output := graph.Nodes[0].ChatModel.StructuredOutput
	if output == nil || output.Name != "turn_intent" || output.Schema.Properties["mode"].Type != "string" || len(output.Schema.Properties["mode"].Enum) != 2 {
		t.Fatalf("structured schema lost in mapping: %#v", output)
	}
	value, err := public.Graph.Nodes[0].AsEinoChatModelNode()
	if err != nil {
		t.Fatal(err)
	}
	value.StructuredOutput.Schema["type"] = "string"
	if err := public.Graph.Nodes[0].FromEinoChatModelNode(value); err != nil {
		t.Fatal(err)
	}
	if err := Validate(public); err == nil {
		t.Fatal("non-object structured schema was accepted")
	}
	if output.Schema.Type != "object" {
		t.Fatal("mapping retained caller-owned mutable schema data")
	}
}
