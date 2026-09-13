package einoconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestStateVoiceWriterPaths(t *testing.T) {
	for _, test := range []struct {
		name, edges, branches string
		wantError             bool
	}{
		{"upstream", `[["start","select"],["select","answer"],["answer","end"]]`, `[]`, false},
		{"parallel", `[["start","select"],["start","answer"],["select","end"],["answer","end"]]`, `[]`, true},
		{"downstream", `[["start","answer"],["answer","select"],["select","end"]]`, `[]`, true},
		{"edge bypass", `[["start","select"],["select","answer"],["start","answer"],["answer","end"]]`, `[]`, true},
		{"branch bypass", `[["start","gate"],["select","answer"],["answer","end"]]`, `[{"from":"gate","mode":"first_match","routes":[{"when":{"field":"input.text","op":"eq","value":"fox"},"to":"select"}],"default":"answer"}]`, true},
		{"branch after writer", `[["start","select"],["select","gate"],["answer","end"]]`, `[{"from":"gate","mode":"first_match","routes":[{"when":{"field":"speaker","op":"eq","value":"fox"},"to":"answer"}],"default":"answer"}]`, false},
		{"unreachable writer", `[["start","answer"],["answer","end"]]`, `[]`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := decodeSpec(t, `{"voice_adapter":{"state_voices":{"field":"speaker","voices":{"fox":"story.fox"}}},"graph":{
    "name":"writers","compile":{"node_trigger_mode":"any_predecessor"},
    "state":{"fields":[{"name":"speaker","type":"string","merge":"replace"},{"name":"answer","type":"string","merge":"replace"},{"name":"gate","type":"string","merge":"replace"}]},
    "nodes":[{"id":"select","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"speaker"}},
    {"id":"gate","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"gate"}},
    {"id":"answer","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"answer"}}],
    "edges":[],"branches":[],"outputs":[{"node":"answer","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}`)
			if test.branches == `[]` {
				spec.Graph.Nodes = append(spec.Graph.Nodes[:1], spec.Graph.Nodes[2:]...)
			}
			var edges [][2]string
			if err := json.Unmarshal([]byte(test.edges), &edges); err != nil {
				t.Fatal(err)
			}
			for _, edge := range edges {
				spec.Graph.Edges = append(spec.Graph.Edges, apitypes.EinoEdge{From: edge[0], To: edge[1]})
			}
			if err := json.Unmarshal([]byte(test.branches), &spec.Graph.Branches); err != nil {
				t.Fatal(err)
			}
			// Validate the public contract, not only the reachability helper.
			err := Validate(spec)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "state_voices.field") {
					t.Fatalf("expected selector error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStateVoiceRejectsAdditionalParallelWriter(t *testing.T) {
	spec := decodeSpec(t, `{"voice_adapter":{"state_voices":{"field":"speaker","voices":{"fox":"story.fox"}}},"graph":{
 "name":"writers","compile":{"node_trigger_mode":"any_predecessor"},
 "state":{"fields":[{"name":"speaker","type":"string","merge":"replace"},{"name":"answer","type":"string","merge":"replace"},{"name":"gate","type":"string","merge":"replace"}]},
 "nodes":[{"id":"select","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"speaker"}},
 {"id":"late","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"speaker"}},
 {"id":"answer","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"answer"}}],
 "edges":[{"from":"start","to":"select"},{"from":"select","to":"answer"},{"from":"select","to":"late"},{"from":"late","to":"end"},{"from":"answer","to":"end"}],"branches":[],
 "outputs":[{"node":"answer","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}`)
	if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "strictly upstream") {
		t.Fatalf("error = %v", err)
	}
}

func TestStateVoiceRejectsParallelWritersBeforeAnyPredecessor(t *testing.T) {
	spec := decodeSpec(t, `{"voice_adapter":{"state_voices":{"field":"speaker","voices":{"fox":"story.fox"}}},"graph":{
 "name":"writers","compile":{"node_trigger_mode":"any_predecessor"},
 "state":{"fields":[{"name":"speaker","type":"string","merge":"replace"},{"name":"answer","type":"string","merge":"replace"}]},
 "nodes":[{"id":"first","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"speaker"}},
 {"id":"second","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"speaker"}},
 {"id":"answer","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"answer"}}],
 "edges":[{"from":"start","to":"first"},{"from":"start","to":"second"},{"from":"first","to":"answer"},{"from":"second","to":"answer"},{"from":"answer","to":"end"}],"branches":[],
 "outputs":[{"node":"answer","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}`)
	if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "every path") {
		t.Fatalf("parallel writers accepted: %v", err)
	}
	// Sequential writers both complete before primary and are supported.
	spec.Graph.Edges = []apitypes.EinoEdge{{From: "start", To: "first"}, {From: "first", To: "second"}, {From: "second", To: "answer"}, {From: "answer", To: "end"}}
	if err := Validate(spec); err != nil {
		t.Fatal(err)
	}
}
