package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flownode "github.com/GizClaw/flowcraft/sdk/graph/node"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxmatch "github.com/GizClaw/gizclaw-go/pkgs/genx/match"
)

func TestCompileMatchNodeValidatesAndOwnsConfig(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"model":  "route",
			"input":  "input",
			"output": "matches",
			"rules": []any{map[string]any{
				"name": "play_music",
				"vars": map[string]any{
					"title": map[string]any{"label": "歌曲名", "type": "string"},
				},
				"patterns": []any{"我想听[title]"},
			}},
		}
	}
	runtime, err := compileMatchNode("match", valid())
	if err != nil {
		t.Fatalf("compileMatchNode() error = %v", err)
	}
	if runtime.config.Model != "route" || runtime.config.Input != "input" || runtime.config.Output != "matches" {
		t.Fatalf("normalized config = %#v", runtime.config)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "unknown field", mutate: func(config map[string]any) { config["extra"] = true }, want: "unknown field"},
		{name: "empty model", mutate: func(config map[string]any) { config["model"] = " " }, want: "model alias"},
		{name: "padded model", mutate: func(config map[string]any) { config["model"] = " route " }, want: "trimmed model alias"},
		{name: "qualified model", mutate: func(config map[string]any) { config["model"] = "model/route" }, want: "without '/'"},
		{name: "empty input", mutate: func(config map[string]any) { config["input"] = "" }, want: "Board variables"},
		{name: "padded input", mutate: func(config map[string]any) { config["input"] = " input " }, want: "trimmed input"},
		{name: "empty output", mutate: func(config map[string]any) { config["output"] = "" }, want: "Board variables"},
		{name: "padded output", mutate: func(config map[string]any) { config["output"] = " output " }, want: "trimmed input"},
		{name: "empty rules", mutate: func(config map[string]any) { config["rules"] = []any{} }, want: "rules are required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid()
			test.mutate(config)
			if _, err := compileMatchNode("match", config); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileMatchNode() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestMatchNodeRegistrationAndJSONEOFTerminals(t *testing.T) {
	decoder := json.NewDecoder(strings.NewReader(`{"value":1}`))
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		t.Fatalf("ensureJSONEOF(valid) error = %v", err)
	}
	decoder = json.NewDecoder(strings.NewReader(`{} {}`))
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("Decode(first value) error = %v", err)
	}
	if err := ensureJSONEOF(decoder); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("ensureJSONEOF(trailing) error = %v", err)
	}
	decoder = json.NewDecoder(strings.NewReader(`{} {`))
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("Decode(first malformed value) error = %v", err)
	}
	if err := ensureJSONEOF(decoder); err == nil {
		t.Fatal("ensureJSONEOF() accepted malformed trailing JSON")
	}

	if _, err := compileMatchNode("match", map[string]any{"model": make(chan int)}); err == nil || !strings.Contains(err.Error(), "encode config") {
		t.Fatalf("compileMatchNode(unencodable) error = %v", err)
	}
	runtime, err := compileMatchNode("route", map[string]any{
		"model": "router", "input": "input", "output": "matches",
		"rules": []*genxmatch.Rule{{Name: "route", Patterns: []genxmatch.Pattern{{Input: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("compileMatchNode() error = %v", err)
	}
	factory := flownode.NewFactory()
	registerMatchNodes(factory, Config{matchNodes: map[string]matchNodeRuntime{"route": runtime}})
	if _, err := factory.Build(flowgraph.NodeDefinition{ID: "missing", Type: "match"}); err == nil || !strings.Contains(err.Error(), "was not compiled") {
		t.Fatalf("Build(missing) error = %v", err)
	}
	built, err := factory.Build(flowgraph.NodeDefinition{ID: "route", Type: "match"})
	if err != nil {
		t.Fatalf("Build(route) error = %v", err)
	}
	if built.ID() != "route" || built.Type() != "match" {
		t.Fatalf("built node identity = (%q, %q)", built.ID(), built.Type())
	}
}

func TestMatchNodeCannotBePublished(t *testing.T) {
	_, err := normalizeConfig(Config{
		ID: "match",
		Graph: flowgraph.GraphDefinition{
			Name: "match", Entry: "route",
			Nodes: []flowgraph.NodeDefinition{
				{
					ID: "route", Type: "match",
					Config: map[string]any{
						"model": "router", "input": "input", "output": "matches",
						"rules": []*genxmatch.Rule{{
							Name:     "route",
							Patterns: []genxmatch.Pattern{{Input: "hello"}},
						}},
					},
				},
				{
					ID: "finish", Type: "script",
					Config: map[string]any{"source": `board.setVar("answer", "done");`},
				},
			},
			Edges: []flowgraph.EdgeDefinition{{From: "route", To: "finish"}},
		},
		PublishNodes: []string{"route"},
		Models:       &flowcraftMatchGenerator{},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be a PublishNodes target") {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
}

func TestMatchNodeExecutesAtomicallyWithTypedPorts(t *testing.T) {
	runtime, err := compileMatchNode("route", map[string]any{
		"model": "router", "input": "request", "output": "matches",
		"rules": []any{map[string]any{
			"name": "play_music",
			"vars": map[string]any{
				"title": map[string]any{"label": "歌曲名", "type": "string"},
			},
			"patterns": []any{"我想听[title]"},
		}},
	})
	if err != nil {
		t.Fatalf("compileMatchNode() error = %v", err)
	}
	generator := &flowcraftMatchGenerator{chunks: []*genx.MessageChunk{
		{Part: genx.Text("play_music: title=")},
		{Part: genx.Text("卡农\n")},
	}}
	node := &matchNode{
		id: "route", generator: generator, config: runtime.config, matcher: runtime.matcher,
	}
	if got := node.InputPorts(); !reflect.DeepEqual(got, []flowgraph.Port{{
		Name: "request", Type: flowgraph.PortTypeString, Required: true,
	}}) {
		t.Fatalf("InputPorts() = %#v", got)
	}
	if got := node.OutputPorts(); !reflect.DeepEqual(got, []flowgraph.Port{{
		Name: "matches", Type: flowgraph.PortTypeArray, Required: true,
	}}) {
		t.Fatalf("OutputPorts() = %#v", got)
	}
	board := flowgraph.NewBoard()
	board.SetVar("request", "我想听卡农")
	if err := node.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, board); err != nil {
		t.Fatalf("ExecuteBoard() error = %v", err)
	}
	got, ok := board.GetVar("matches")
	if !ok {
		t.Fatal("matches output is missing")
	}
	want := []any{map[string]any{
		"rule": "play_music",
		"args": map[string]any{
			"title": map[string]any{
				"value":     "卡农",
				"var":       map[string]any{"label": "歌曲名", "type": "string"},
				"has_value": true,
			},
		},
		"raw_text": "",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matches = %#v, want %#v", got, want)
	}
	generator.mu.Lock()
	defer generator.mu.Unlock()
	if generator.pattern != "model/router" {
		t.Fatalf("model pattern = %q", generator.pattern)
	}
	if generator.closes != 1 {
		t.Fatalf("stream closes = %d, want 1", generator.closes)
	}
	if len(generator.prompts) != 1 || len(generator.messages) != 1 || generator.messages[0].Role != genx.RoleUser {
		t.Fatalf("model context prompts=%#v messages=%#v", generator.prompts, generator.messages)
	}
	if tools := collectTools(generator.modelContext); len(tools) != 0 {
		t.Fatalf("Match advertised tools: %#v", tools)
	}
}

func TestMatchNodeRejectsNonStringAndDoesNotPublishPartialOutput(t *testing.T) {
	matcher, err := genxmatch.Compile([]*genxmatch.Rule{{
		Name: "route", Patterns: []genxmatch.Pattern{{Input: "hello"}},
	}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	node := &matchNode{
		id: "route", generator: &flowcraftMatchGenerator{},
		config:  matchNodeConfig{Model: "router", Input: "input", Output: "matches"},
		matcher: matcher,
	}
	board := flowgraph.NewBoard()
	board.SetVar("input", 42)
	if err := node.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, board); err == nil ||
		!strings.Contains(err.Error(), "must be string") {
		t.Fatalf("ExecuteBoard(non-string) error = %v", err)
	}

	streamErr := errors.New("provider disconnected")
	generator := &flowcraftMatchGenerator{
		chunks: []*genx.MessageChunk{{Part: genx.Text("route\n")}},
		err:    streamErr,
	}
	node.generator = generator
	board.SetVar("input", "")
	if err := node.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, board); !errors.Is(err, streamErr) {
		t.Fatalf("ExecuteBoard(stream error) = %v, want %v", err, streamErr)
	}
	if _, ok := board.GetVar("matches"); ok {
		t.Fatal("Match published a parsed prefix after stream failure")
	}
	if generator.closes != 1 {
		t.Fatalf("stream closes = %d, want 1", generator.closes)
	}
}

func TestMatchNodeIsSafeForConcurrentBoards(t *testing.T) {
	matcher, err := genxmatch.Compile([]*genxmatch.Rule{{
		Name:     "route",
		Vars:     map[string]genxmatch.Var{"value": {Label: "值", Type: "string"}},
		Patterns: []genxmatch.Pattern{{Input: "[value]"}},
	}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	generator := &flowcraftMatchGenerator{responseFromInput: true}
	node := &matchNode{
		id: "route", generator: generator,
		config:  matchNodeConfig{Model: "router", Input: "input", Output: "matches"},
		matcher: matcher,
	}
	const count = 16
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			text := fmt.Sprintf("value-%d", index)
			board := flowgraph.NewBoard()
			board.SetVar("input", text)
			if err := node.ExecuteBoard(flowgraph.ExecutionContext{Context: t.Context()}, board); err != nil {
				failures <- err
				return
			}
			projected, _ := board.GetVar("matches")
			items := projected.([]any)
			args := items[0].(map[string]any)["args"].(map[string]any)
			value := args["value"].(map[string]any)["value"]
			if value != text {
				failures <- fmt.Errorf("match value = %#v, want %q", value, text)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

type flowcraftMatchGenerator struct {
	mu sync.Mutex

	chunks            []*genx.MessageChunk
	err               error
	responseFromInput bool

	pattern      string
	modelContext genx.ModelContext
	prompts      []*genx.Prompt
	messages     []*genx.Message
	closes       int
}

func (generator *flowcraftMatchGenerator) GenerateStream(
	_ context.Context,
	pattern string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	generator.pattern = pattern
	generator.modelContext = modelContext
	generator.prompts = collectPrompts(modelContext)
	generator.messages = collectMessages(modelContext)
	chunks := generator.chunks
	if generator.responseFromInput {
		input := ""
		if len(generator.messages) > 0 {
			if contents, ok := generator.messages[0].Payload.(genx.Contents); ok && len(contents) > 0 {
				input = string(contents[0].(genx.Text))
			}
		}
		chunks = []*genx.MessageChunk{{Part: genx.Text("route: value=" + input + "\n")}}
	}
	return &flowcraftMatchStream{
		chunks: chunks, terminalErr: generator.err,
		onClose: func() {
			generator.mu.Lock()
			generator.closes++
			generator.mu.Unlock()
		},
	}, nil
}

func (*flowcraftMatchGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("Invoke must not be used")
}

type flowcraftMatchStream struct {
	chunks      []*genx.MessageChunk
	index       int
	terminalErr error
	onClose     func()
}

func (stream *flowcraftMatchStream) Next() (*genx.MessageChunk, error) {
	if stream.index < len(stream.chunks) {
		chunk := stream.chunks[stream.index]
		stream.index++
		return chunk, nil
	}
	if stream.terminalErr != nil {
		return nil, stream.terminalErr
	}
	return nil, io.EOF
}

func (stream *flowcraftMatchStream) Close() error {
	if stream.onClose != nil {
		stream.onClose()
		stream.onClose = nil
	}
	return nil
}

func (stream *flowcraftMatchStream) CloseWithError(error) error {
	return stream.Close()
}

func collectPrompts(modelContext genx.ModelContext) []*genx.Prompt {
	var result []*genx.Prompt
	for prompt := range modelContext.Prompts() {
		result = append(result, prompt)
	}
	return result
}

func collectMessages(modelContext genx.ModelContext) []*genx.Message {
	var result []*genx.Message
	for message := range modelContext.Messages() {
		result = append(result, message)
	}
	return result
}

func collectTools(modelContext genx.ModelContext) []genx.Tool {
	var result []genx.Tool
	for tool := range modelContext.Tools() {
		result = append(result, tool)
	}
	return result
}
