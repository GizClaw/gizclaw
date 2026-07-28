package flowcraft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flownode "github.com/GizClaw/flowcraft/sdk/graph/node"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxmatch "github.com/GizClaw/gizclaw-go/pkgs/genx/match"
)

type matchNodeConfig struct {
	Model  string            `json:"model"`
	Input  string            `json:"input"`
	Output string            `json:"output"`
	Rules  []*genxmatch.Rule `json:"rules"`
}

type matchNodeRuntime struct {
	config  matchNodeConfig
	matcher *genxmatch.Matcher
}

func compileMatchNode(id string, source map[string]any) (matchNodeRuntime, error) {
	var config matchNodeConfig
	raw, err := json.Marshal(source)
	if err != nil {
		return matchNodeRuntime{}, fmt.Errorf("flowcraft: match node %q: encode config: %w", id, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return matchNodeRuntime{}, fmt.Errorf("flowcraft: match node %q: decode config: %w", id, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return matchNodeRuntime{}, fmt.Errorf("flowcraft: match node %q: decode config: %w", id, err)
	}
	if config.Model == "" || strings.TrimSpace(config.Model) != config.Model ||
		strings.Contains(config.Model, "/") {
		return matchNodeRuntime{}, fmt.Errorf("flowcraft: match node %q requires a trimmed model alias without '/'", id)
	}
	if config.Input == "" || strings.TrimSpace(config.Input) != config.Input ||
		config.Output == "" || strings.TrimSpace(config.Output) != config.Output {
		return matchNodeRuntime{}, fmt.Errorf("flowcraft: match node %q requires trimmed input and output Board variables", id)
	}
	matcher, err := genxmatch.Compile(config.Rules)
	if err != nil {
		return matchNodeRuntime{}, fmt.Errorf("flowcraft: match node %q: %w", id, err)
	}
	return matchNodeRuntime{config: config, matcher: matcher}, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func registerMatchNodes(factory *flownode.Factory, config Config) {
	factory.RegisterBuilder("match", func(def flowgraph.NodeDefinition) (flowgraph.Node, error) {
		runtime, ok := config.matchNodes[def.ID]
		if !ok {
			return nil, fmt.Errorf("flowcraft: match node %q was not compiled", def.ID)
		}
		return &matchNode{
			id: def.ID, generator: config.Models,
			config: runtime.config, matcher: runtime.matcher,
		}, nil
	})
}

type matchNode struct {
	id        string
	generator genx.Generator
	config    matchNodeConfig
	matcher   *genxmatch.Matcher
}

func (node *matchNode) ID() string { return node.id }
func (*matchNode) Type() string    { return "match" }
func (node *matchNode) InputPorts() []flowgraph.Port {
	return []flowgraph.Port{{
		Name: node.config.Input, Type: flowgraph.PortTypeString, Required: true,
	}}
}
func (node *matchNode) OutputPorts() []flowgraph.Port {
	return []flowgraph.Port{{
		Name: node.config.Output, Type: flowgraph.PortTypeArray, Required: true,
	}}
}

func (node *matchNode) ExecuteBoard(ctx flowgraph.ExecutionContext, board *flowgraph.Board) error {
	value, ok := board.GetVar(node.config.Input)
	if !ok {
		return fmt.Errorf("flowcraft: match node %q input %q is missing", node.id, node.config.Input)
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf(
			"flowcraft: match node %q input %q must be string, got %T",
			node.id,
			node.config.Input,
			value,
		)
	}
	var modelContext genx.ModelContextBuilder
	modelContext.UserText(node.id+":input", text)
	results, err := genxmatch.Collect(node.matcher.Match(
		ctx.Context,
		"model/"+node.config.Model,
		modelContext.Build(),
		genxmatch.WithGenerator(node.generator),
	))
	if err != nil {
		return fmt.Errorf("flowcraft: match node %q: %w", node.id, err)
	}
	board.SetVar(node.config.Output, genxmatch.Project(results))
	return nil
}
