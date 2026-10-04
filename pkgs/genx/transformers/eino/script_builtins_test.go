package eino

import (
	"strings"
	"testing"
	"time"
)

func TestScriptJSONAndRegexRespectInputOwnership(t *testing.T) {
	source := `def run(input):
    values = json.decode(json.encode(input["values"]))
    values["score"] += 100
    matched = regex_find(input["text"], ("badge=([a-z]+)", "i"))
    rendered = regex_replace(input["text"], ("badge", "gi"), "award")
    return {"answer": json.encode({"score": values["score"], "badge": matched[1], "rendered": rendered})}
`
	script, err := compileScript(t.Context(), ScriptNode{Language: ScriptStarlark, Source: source, Entrypoint: "run",
		Limits: ScriptLimits{MaxExecutionSteps: 10000, Timeout: time.Second, MaxInputBytes: 65536, MaxOutputBytes: 65536}})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"score": int64(0)}
	result, err := script.run(t.Context(), map[string]any{"values": values, "text": "BADGE=moon badge=river"}, map[string]StateType{"answer": StateString})
	if err != nil {
		t.Fatal(err)
	}
	answer := result["answer"].(string)
	if values["score"] != int64(0) || !strings.Contains(answer, `"score":100`) ||
		!strings.Contains(answer, `"badge":"moon"`) || !strings.Contains(answer, "award=moon award=river") {
		t.Fatalf("answer=%s, caller values=%v", answer, values)
	}
}

func TestScriptRegexRejectsUnsupportedPatternsAndFlags(t *testing.T) {
	for _, pattern := range []string{`("(?=secret)", "")`, `("badge", "y")`} {
		script, err := compileScript(t.Context(), ScriptNode{Language: ScriptStarlark,
			Source: "def run(input):\n    return {\"answer\": regex_find(\"text\", " + pattern + ")}\n",
			Limits: ScriptLimits{MaxExecutionSteps: 10000, Timeout: time.Second, MaxInputBytes: 65536, MaxOutputBytes: 65536}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := script.run(t.Context(), map[string]any{}, map[string]StateType{"answer": StateString}); err == nil {
			t.Fatalf("unsupported regex accepted: %s", pattern)
		}
	}
}
