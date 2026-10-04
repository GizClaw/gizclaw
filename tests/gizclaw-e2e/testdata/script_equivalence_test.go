package testdata_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func normalizeScriptContract(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for name, field := range value {
			result[name] = normalizeScriptContract(field)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, field := range value {
			result[index] = normalizeScriptContract(field)
		}
		return result
	case string:
		// Board views may contain serialized JSON. Object key ordering is not
		// part of their business contract; all contents and array order are.
		if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
			var decoded any
			if json.Unmarshal([]byte(value), &decoded) == nil {
				return normalizeScriptContract(decoded)
			}
		}
		// A rendered prompt can embed a JSON state view after natural-language
		// context. Preserve its prefix/suffix exactly and compare that view's
		// structure rather than implementation-dependent object-key order.
		if start := strings.IndexByte(value, '{'); start >= 0 {
			decoder := json.NewDecoder(strings.NewReader(value[start:]))
			var decoded map[string]any
			if decoder.Decode(&decoded) == nil {
				return []any{value[:start], normalizeScriptContract(decoded), normalizeScriptContract(value[start+int(decoder.InputOffset()):])}
			}
		}
		return value
	default:
		return value
	}
}

// Reference outputs were captured from the pre-retirement business scripts at
// 6fce3ae416ffeba9e72c479eaee9c2785e2b298d, with a fixed role-seeding clock.
// The 70 reachable scripts cover empty, opening and explicit player input; six
// unreachable nodes were excluded after traversing the original Graph edges.
func TestBusinessScriptReferenceContracts(t *testing.T) {
	raw, err := os.ReadFile("business_script_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		// Cases share six complete input snapshots. Keeping them once avoids
		// repeating large prompts without dropping any reference contract.
		Inputs []json.RawMessage `json:"inputs"`
		Cases  []struct {
			Workflow string         `json:"workflow"`
			Node     string         `json:"node"`
			Name     string         `json:"name"`
			Input    int            `json:"input"`
			Expected map[string]any `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Cases) != 210 {
		t.Fatalf("reference case count = %d", len(reference.Cases))
	}
	for _, test := range reference.Cases {
		t.Run(test.Workflow+"/"+test.Node+"/"+test.Name, func(t *testing.T) {
			if test.Input < 0 || test.Input >= len(reference.Inputs) {
				t.Fatalf("unknown reference input %d", test.Input)
			}
			var input map[string]any
			if err := json.Unmarshal(reference.Inputs[test.Input], &input); err != nil {
				t.Fatal(err)
			}
			path := strings.TrimPrefix(test.Workflow, "tests/gizclaw-e2e/testdata/")
			graph := fixtureGraph(t, path)
			for index := range graph.Nodes {
				if graph.Nodes[index].ID == test.Node && graph.Nodes[index].Script != nil {
					// Freeze only the role-seeding clock; execute the committed
					// business program through the production sandbox unchanged.
					graph.Nodes[index].Script.Source = strings.ReplaceAll(graph.Nodes[index].Script.Source, "now_millis()", "1728000000000")
				}
			}
			fields := runFixtureScript(t, graph, test.Node, input)
			node := fixtureNode(t, graph, test.Node)
			actual := map[string]any{"values": fields[node.Outputs["values"]], "channels": fields[node.Outputs["channels"]], "text": fields[node.Outputs["text"]]}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(normalizeScriptContract(actual), normalizeScriptContract(test.Expected)) {
				want, _ := json.Marshal(test.Expected)
				t.Fatalf("business contract mismatch\nactual: %s\nreference: %s", encoded, want)
			}
		})
	}
}
