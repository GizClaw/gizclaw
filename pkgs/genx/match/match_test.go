package match

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestCompileRejectsUndefinedPlaceholder(t *testing.T) {
	_, err := Compile([]*Rule{{
		Name: "greet",
		Vars: map[string]Var{"name": {Label: "姓名", Type: "string"}},
		Patterns: []Pattern{{
			Input: "你好 [unknown]",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "not defined in vars") {
		t.Fatalf("expected placeholder error, got: %v", err)
	}
}

func TestParseLineTypedArgs(t *testing.T) {
	m := &Matcher{specs: map[string]map[string]Var{
		"r1": {
			"age": {Type: "int"},
			"ok":  {Type: "bool"},
		},
	}}

	r, ok := m.parseLine("r1: age=12, ok=true")
	if !ok {
		t.Fatal("parseLine should return ok")
	}
	if r.Rule != "r1" {
		t.Fatalf("unexpected rule: %s", r.Rule)
	}
	if v, ok := r.Args["age"].Value.(int64); !ok || v != 12 {
		t.Fatalf("unexpected age value: %#v", r.Args["age"])
	}
	if v, ok := r.Args["ok"].Value.(bool); !ok || !v {
		t.Fatalf("unexpected ok value: %#v", r.Args["ok"])
	}
}

type matchStream struct {
	chunks []*genx.MessageChunk
	idx    int
	err    error
	closed bool
	closes int
}

func (s *matchStream) Next() (*genx.MessageChunk, error) {
	if s.idx < len(s.chunks) {
		c := s.chunks[s.idx]
		s.idx++
		return c, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, genx.ErrDone
}

func (s *matchStream) Close() error {
	s.closed = true
	s.closes++
	return nil
}

func (s *matchStream) CloseWithError(error) error {
	s.closed = true
	s.closes++
	return nil
}

type matchGenerator struct {
	stream genx.Stream
	err    error
}

func (g matchGenerator) GenerateStream(context.Context, string, genx.ModelContext) (genx.Stream, error) {
	if g.err != nil {
		return nil, g.err
	}
	return g.stream, nil
}

func (g matchGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unused")
}

func TestCompileWithTplAndMatchEOF(t *testing.T) {
	rules := []*Rule{{
		Name: "topic_rule",
		Vars: map[string]Var{"topic": {Label: "主题", Type: "string"}},
		Patterns: []Pattern{{
			Input: "我喜欢[topic]",
		}},
	}}

	m, err := Compile(rules, WithTpl(`{{range .Rules}}{{.Name}} {{end}}`))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if !strings.Contains(m.SystemPrompt(), "topic_rule") {
		t.Fatalf("unexpected system prompt: %s", m.SystemPrompt())
	}

	stream := &matchStream{chunks: []*genx.MessageChunk{{Part: genx.Text("topic_rule: topic=恐龙\nraw text\n")}}, err: io.EOF}

	var mcb genx.ModelContextBuilder
	mcb.UserText("u", "我喜欢恐龙")

	results, err := Collect(m.Match(context.Background(), "gx/mock", mcb.Build(), WithGenerator(matchGenerator{stream: stream})))
	if err != nil {
		t.Fatalf("collect match failed: %v", err)
	}
	if !stream.closed {
		t.Fatal("expected stream to be closed")
	}
	if stream.closes != 1 {
		t.Fatalf("stream closes = %d, want 1", stream.closes)
	}
	if len(results) != 2 {
		t.Fatalf("unexpected match result length: %d %#v", len(results), results)
	}
	if results[0].Rule != "topic_rule" || results[0].Args["topic"].Value != "恐龙" {
		t.Fatalf("unexpected parsed result: %#v", results[0])
	}
	if results[1].RawText != "raw text" {
		t.Fatalf("unexpected raw text result: %#v", results[1])
	}
}

func TestCompileAndMatchErrorPaths(t *testing.T) {
	if _, err := Compile([]*Rule{{Name: "r"}}, WithTpl("{{")); err == nil {
		t.Fatal("expected template parse error")
	}

	rules := []*Rule{{Name: "r", Vars: map[string]Var{}, Patterns: []Pattern{{Input: "hello"}}}}
	m, err := Compile(rules)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var mcb genx.ModelContextBuilder
	_, err = Collect(m.Match(context.Background(), "gx/mock", mcb.Build(), WithGenerator(matchGenerator{err: errors.New("gen failed")})))
	if err == nil || !strings.Contains(err.Error(), "generate") {
		t.Fatalf("expected generate error, got: %v", err)
	}

	badStream := &matchStream{chunks: []*genx.MessageChunk{{Part: genx.Text("r\n")}}, err: errors.New("stream broken")}
	_, err = Collect(m.Match(context.Background(), "gx/mock", mcb.Build(), WithGenerator(matchGenerator{stream: badStream})))
	if err == nil || !strings.Contains(err.Error(), "stream broken") {
		t.Fatalf("expected stream error, got: %v", err)
	}
}

func TestParseHelpersAndPromptData(t *testing.T) {
	m := &Matcher{specs: map[string]map[string]Var{
		"rule": {
			"i": {Type: "int"},
			"f": {Type: "float"},
			"b": {Type: "bool"},
			"s": {Type: "string"},
		},
	}}

	r, ok := m.parseLine("rule: i=1, f=2.5, b=true, s=hello, unknown=1")
	if !ok || r.Rule != "rule" {
		t.Fatalf("unexpected parseLine result: %#v ok=%v", r, ok)
	}
	if _, ok := r.Args["i"].Value.(int64); !ok {
		t.Fatalf("expected int64 arg, got: %#v", r.Args["i"])
	}
	if _, ok := r.Args["f"].Value.(float64); !ok {
		t.Fatalf("expected float64 arg, got: %#v", r.Args["f"])
	}
	if _, ok := r.Args["b"].Value.(bool); !ok {
		t.Fatalf("expected bool arg, got: %#v", r.Args["b"])
	}

	raw, ok := m.parseLine("unknown: x=1")
	if !ok || raw.RawText == "" {
		t.Fatalf("expected raw text fallback, got: %#v", raw)
	}

	raw, ok = m.parseLine("   ")
	if !ok || raw.RawText == "" {
		t.Fatalf("expected raw text for blank, got: %#v", raw)
	}

	if _, err := buildPromptData([]*Rule{nil, {Name: "r"}}); err == nil ||
		!strings.Contains(err.Error(), "rule[0] is nil") {
		t.Fatalf("buildPromptData(nil rule) error = %v", err)
	}
}

func TestRuleJSONAndCompileValidation(t *testing.T) {
	ex := Example{Subject: "s", UserText: "u", FormattedTo: "f"}
	b, err := json.Marshal(ex)
	if err != nil || !strings.Contains(string(b), "f") {
		t.Fatalf("marshal example failed: %v %s", err, string(b))
	}

	if err := (&Example{}).UnmarshalJSON([]byte(`[]`)); err == nil {
		t.Fatal("expected empty array error")
	}
	if err := (*Example)(nil).UnmarshalJSON([]byte(`["x"]`)); err == nil {
		t.Fatal("expected nil receiver error")
	}

	p := Pattern{Input: "in", Output: "out"}
	b, err = json.Marshal(p)
	if err != nil || !strings.Contains(string(b), "out") {
		t.Fatalf("marshal pattern failed: %v %s", err, string(b))
	}

	var p2 Pattern
	if err := p2.UnmarshalJSON([]byte(`"hello"`)); err != nil {
		t.Fatalf("unmarshal string pattern failed: %v", err)
	}
	if p2.Input != "hello" || p2.Output != "" {
		t.Fatalf("unexpected string pattern: %#v", p2)
	}
	if err := p2.UnmarshalJSON([]byte(`["i","o"]`)); err != nil {
		t.Fatalf("unmarshal array pattern failed: %v", err)
	}
	if err := p2.UnmarshalJSON([]byte(`123`)); err == nil {
		t.Fatal("expected unsupported pattern error")
	}

	badType := &Rule{Name: "r", Vars: map[string]Var{"x": {Type: "bad"}}}
	if err := badType.compileTo(&promptData{}); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected invalid var type error, got: %v", err)
	}

	badLabel := &Rule{Name: "r", Vars: map[string]Var{"x": {Label: "[x]", Type: "string"}}}
	if err := badLabel.compileTo(&promptData{}); err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("expected label validation error, got: %v", err)
	}

	badPattern := &Rule{Name: "r", Vars: map[string]Var{"x": {Label: "X", Type: "string"}}, Patterns: []Pattern{{Input: "line\nnext"}}}
	if err := badPattern.compileTo(&promptData{}); err == nil || !strings.Contains(err.Error(), "contains newline") {
		t.Fatalf("expected newline validation error, got: %v", err)
	}

	if err := (&Rule{Name: "r"}).compileTo(nil); err == nil || !strings.Contains(err.Error(), "prompt data is nil") {
		t.Fatalf("expected nil prompt data error, got: %v", err)
	}

	in, out := expandPattern("rule", "hello [name]", map[string]Var{"name": {Label: "姓名"}})
	if in != "hello [姓名]" || !strings.Contains(out, "name=[姓名]") {
		t.Fatalf("unexpected expandPattern output: in=%q out=%q", in, out)
	}

	in, out = expandPattern("rule", "", map[string]Var{})
	if in != "" || out != "rule" {
		t.Fatalf("unexpected empty expandPattern output: in=%q out=%q", in, out)
	}
}

func TestCompileStrictValidation(t *testing.T) {
	valid := func() *Rule {
		return &Rule{
			Name:     "play_music",
			Vars:     map[string]Var{"title": {Label: "歌曲名", Type: "string"}},
			Patterns: []Pattern{{Input: "我想听[title]"}},
			Examples: []Example{{Subject: "play_music", UserText: "我想听卡农", FormattedTo: "play_music: title=卡农"}},
		}
	}
	tests := []struct {
		name  string
		rules []*Rule
		want  string
	}{
		{name: "empty", want: "rules are required"},
		{name: "nil", rules: []*Rule{nil}, want: "rule[0] is nil"},
		{name: "duplicate", rules: []*Rule{valid(), valid()}, want: "duplicate rule name"},
		{name: "blank name", rules: []*Rule{{Name: " "}}, want: "rule name"},
		{name: "padded name", rules: []*Rule{{Name: " route "}}, want: "rule name"},
		{name: "invalid variable", rules: []*Rule{{
			Name: "route", Vars: map[string]Var{"bad-name": {Label: "bad"}},
		}}, want: "var name"},
		{name: "padded label", rules: []*Rule{{
			Name: "route", Vars: map[string]Var{"name": {Label: " Name "}},
		}}, want: "label must be trimmed"},
		{name: "invalid type", rules: []*Rule{{
			Name: "route", Vars: map[string]Var{"name": {Label: "Name", Type: "number"}},
		}}, want: "invalid type"},
		{name: "malformed placeholder", rules: []*Rule{{
			Name: "route", Patterns: []Pattern{{Input: "hello [bad-name]"}},
		}}, want: "malformed placeholder"},
		{name: "nested placeholder", rules: []*Rule{{
			Name: "route", Vars: map[string]Var{"name": {Label: "Name"}},
			Patterns: []Pattern{{Input: "hello [[name]]"}},
		}}, want: "malformed placeholder"},
		{name: "unterminated placeholder", rules: []*Rule{{
			Name: "route", Patterns: []Pattern{{Input: "hello [name"}},
		}}, want: "malformed placeholder"},
		{name: "unknown placeholder", rules: []*Rule{{
			Name: "route", Patterns: []Pattern{{Input: "hello [name]"}},
		}}, want: "not defined in vars"},
		{name: "invalid example", rules: []*Rule{{
			Name: "route", Examples: []Example{{Subject: "route", FormattedTo: "route"}},
		}}, want: "output without user text"},
		{name: "invalid reference", rules: []*Rule{{
			Name: "route", References: map[string]string{" ": "value"},
		}}, want: "reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile(test.rules)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCompilePreservesEmptyLabelBehavior(t *testing.T) {
	matcher, err := Compile([]*Rule{{
		Name:     "route",
		Vars:     map[string]Var{"name": {}},
		Patterns: []Pattern{{Input: "hello [name]"}},
	}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if !strings.Contains(matcher.SystemPrompt(), "hello [name]") {
		t.Fatalf("SystemPrompt() = %q, want unexpanded empty-label placeholder", matcher.SystemPrompt())
	}
}

func TestParseChunkFramingAndProjection(t *testing.T) {
	rule := &Rule{
		Name: "measure",
		Vars: map[string]Var{
			"count": {Label: "数量", Type: "int"},
			"ok":    {Label: "是否", Type: "bool"},
		},
		Patterns: []Pattern{{Input: "[count] items"}},
	}
	matcher, err := Compile([]*Rule{rule})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	chunks := func(yield func(string, error) bool) {
		for _, chunk := range []string{
			"measure: count=12, ", "ok=not-bool\r\nunknown", " output\n\nmeasure",
		} {
			if !yield(chunk, nil) {
				return
			}
		}
	}
	results, err := Collect(matcher.Parse(chunks))
	if err != nil {
		t.Fatalf("Collect(Parse()) error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %#v", results)
	}
	want := []any{
		map[string]any{
			"rule": "measure",
			"args": map[string]any{
				"count": map[string]any{
					"value": int64(12), "var": map[string]any{"label": "数量", "type": "int"}, "has_value": true,
				},
				"ok": map[string]any{
					"value": "not-bool", "var": map[string]any{"label": "是否", "type": "bool"}, "has_value": true,
				},
			},
			"raw_text": "",
		},
		map[string]any{"rule": "", "args": map[string]any{}, "raw_text": "unknown output"},
		map[string]any{
			"rule": "measure",
			"args": map[string]any{
				"count": map[string]any{
					"value": nil, "var": map[string]any{"label": "数量", "type": "int"}, "has_value": false,
				},
				"ok": map[string]any{
					"value": nil, "var": map[string]any{"label": "是否", "type": "bool"}, "has_value": false,
				},
			},
			"raw_text": "",
		},
	}
	if got := Project(results); !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}

	rule.Vars["count"] = Var{Label: "mutated", Type: "float"}
	reparsed, err := Collect(matcher.Parse(func(yield func(string, error) bool) {
		yield("measure: count=7", nil)
	}))
	if err != nil {
		t.Fatalf("Collect(Parse()) after mutation error = %v", err)
	}
	if argument := reparsed[0].Args["count"]; argument.Var.Label != "数量" || argument.Value != int64(7) {
		t.Fatalf("compiled Matcher observed caller mutation: %#v", argument)
	}
}

func TestParsePreservesPrefixBeforeError(t *testing.T) {
	matcher, err := Compile([]*Rule{{Name: "route", Patterns: []Pattern{{Input: "hello"}}}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	streamErr := errors.New("stream failed")
	results, err := Collect(matcher.Parse(func(yield func(string, error) bool) {
		if yield("route\n", nil) {
			yield("", streamErr)
		}
	}))
	if !errors.Is(err, streamErr) {
		t.Fatalf("Collect(Parse()) error = %v, want %v", err, streamErr)
	}
	if len(results) != 1 || results[0].Rule != "route" {
		t.Fatalf("parsed prefix = %#v", results)
	}
}

func TestMatchStopsParsingAndClosesStreamWhenConsumerStops(t *testing.T) {
	matcher, err := Compile([]*Rule{{Name: "route", Patterns: []Pattern{{Input: "hello"}}}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	stream := &matchStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("route\n")},
		{Part: genx.Text("route\n")},
	}}
	for range matcher.Match(
		t.Context(),
		"model/router",
		(&genx.ModelContextBuilder{}).Build(),
		WithGenerator(matchGenerator{stream: stream}),
	) {
		break
	}
	if stream.idx != 1 {
		t.Fatalf("stream Next calls consumed %d chunks, want 1", stream.idx)
	}
	if stream.closes != 1 {
		t.Fatalf("stream closes = %d, want 1", stream.closes)
	}
}

func TestYAMLUnmarshalErrorBranches(t *testing.T) {
	var p Pattern
	if err := p.UnmarshalYAML([]byte("[a,b,c]")); err == nil || !strings.Contains(err.Error(), "invalid pattern array length") {
		t.Fatalf("expected yaml pattern length error, got: %v", err)
	}
	if err := (*Pattern)(nil).UnmarshalYAML([]byte("x")); err == nil {
		t.Fatal("expected nil pattern receiver error")
	}

	if err := (*Example)(nil).UnmarshalYAML([]byte("[\"x\"]")); err == nil {
		t.Fatal("expected nil example receiver error")
	}
	if err := (&Example{}).UnmarshalYAML([]byte("[]")); err == nil || !strings.Contains(err.Error(), "invalid example array length") {
		t.Fatalf("expected yaml example length error, got: %v", err)
	}

	if _, err := ParseRuleYAML([]byte("name: [")); err == nil {
		t.Fatal("expected parse rule yaml error")
	}
}
