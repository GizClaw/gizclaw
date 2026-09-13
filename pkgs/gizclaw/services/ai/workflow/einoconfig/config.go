// Package einoconfig maps and validates the public Eino Workflow contract.
package einoconfig

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"mime"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"

	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Validate rejects graph and Memory policies that the Eino Transformer cannot
// construct. Runtime model resolution and Store capability checks remain
// AgentHost responsibilities.
func Validate(public apitypes.EinoWorkflowSpec) error {
	if err := validateVoiceAdapter(public); err != nil {
		return fmt.Errorf("voice_adapter: %w", err)
	}
	graph, err := MapGraph(public.Graph)
	if err != nil {
		return fmt.Errorf("graph: %w", err)
	}
	config := genxeino.Config{
		Agent:      genxeino.AgentConfig{ID: "workflow-validation"},
		Graph:      graph,
		Components: validationComponents{},
		// Public Graph validation happens before a Workspace resolves its
		// RuntimeProfile Memory alias. Supply only the capability shape needed
		// to validate real Memory nodes; no Store operation is executed here.
		Memory: &genxeino.MemoryConfig{
			Store: validationMemoryStore{},
			Scope: memory.Scope{AppID: "workflow-validation"},
		},
	}
	if public.Limits != nil && public.Limits.MaxOutputBytes != nil {
		config.Limits.MaxOutputBytes = *public.Limits.MaxOutputBytes
	}
	if public.VoiceAdapter != nil && public.VoiceAdapter.StateVoices != nil {
		if err := validateVoiceWriters(graph, public.VoiceAdapter.StateVoices.Field); err != nil {
			return err
		}
	}
	return genxeino.ValidateConfig(config)
}

// validateVoiceWriters requires an upstream write on every possible control-flow
// path, including conditional routes. A parallel or later writer could race the
// first output snapshot even when another writer dominates the primary.
func validateVoiceWriters(graph genxeino.GraphDefinition, field string) error {
	var primary string
	for _, output := range graph.Outputs {
		if output.Primary {
			primary = output.Node
		}
	}
	next := make(map[string][]string)
	for _, edge := range graph.Edges {
		next[edge.From] = append(next[edge.From], edge.To)
	}
	for _, branch := range graph.Branches {
		next[branch.From] = append(next[branch.From], branch.Default)
		for _, route := range branch.Routes {
			next[branch.From] = append(next[branch.From], route.To)
		}
	}
	reachable := func(from, to string, stop map[string]bool) bool {
		seen := make(map[string]bool)
		pending := []string{from}
		for len(pending) > 0 {
			node := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if node == to {
				return true
			}
			if seen[node] || stop[node] {
				continue
			}
			seen[node] = true
			pending = append(pending, next[node]...)
		}
		return false
	}
	writers := make(map[string]bool)
	for _, node := range graph.Nodes {
		for _, target := range node.Outputs {
			if target != field || !reachable("start", node.ID, nil) {
				continue
			}
			if node.ID == primary || !reachable(node.ID, primary, nil) || reachable(primary, node.ID, nil) {
				return fmt.Errorf("voice_adapter.state_voices.field: writer %q must be strictly upstream of primary %q", node.ID, primary)
			}
			// An upstream edge alone is insufficient with any_predecessor:
			// another predecessor can start primary while this writer is running.
			if reachable("start", primary, map[string]bool{node.ID: true}) {
				return fmt.Errorf("voice_adapter.state_voices.field: every path to primary %q must pass writer %q before output", primary, node.ID)
			}
			writers[node.ID] = true
		}
	}
	if len(writers) == 0 {
		return fmt.Errorf("voice_adapter.state_voices.field: every path to primary %q must pass an upstream writer of %q", primary, field)
	}
	return nil
}

func validateVoiceAdapter(public apitypes.EinoWorkflowSpec) error {
	adapter := public.VoiceAdapter
	if adapter == nil {
		return nil
	}
	asr := strings.TrimSpace(stringValue(adapter.AsrModel))
	defaultVoice := strings.TrimSpace(stringValue(adapter.DefaultVoice))
	var nodeVoices map[string]string
	if adapter.NodeVoices != nil {
		nodeVoices = *adapter.NodeVoices
	}
	if asr != "" {
		if err := runtimealias.Validate("ASR model alias", asr); err != nil {
			return fmt.Errorf("asr_model: %w", err)
		}
	}
	if defaultVoice != "" {
		if err := runtimealias.Validate("default Voice alias", defaultVoice); err != nil {
			return fmt.Errorf("default_voice: %w", err)
		}
	}
	if selector := adapter.StateVoices; selector != nil {
		found := false
		for _, field := range public.Graph.State.Fields {
			if field.Name == selector.Field && field.Type == apitypes.EinoStateFieldTypeString {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("state_voices.field: must reference a declared string state field")
		}
		if len(selector.Voices) == 0 {
			return fmt.Errorf("state_voices.voices: mapping must not be empty")
		}
		for value, alias := range selector.Voices {
			if err := runtimealias.Validate("state Voice alias", alias); err != nil {
				return fmt.Errorf("state_voices.voices.%s: %w", value, err)
			}
		}
		primaryText := false
		for _, output := range public.Graph.Outputs {
			mediaType, _, err := mime.ParseMediaType(output.MimeType)
			if output.Primary != nil && *output.Primary && err == nil && strings.EqualFold(mediaType, "text/plain") {
				primaryText = true
			}
		}
		if !primaryText {
			return fmt.Errorf("state_voices: primary output must be text/plain")
		}
	}
	textOutputNodes := make(map[string]struct{}, len(public.Graph.Outputs))
	for _, output := range public.Graph.Outputs {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(output.MimeType))
		if err == nil && strings.EqualFold(mediaType, "text/plain") {
			textOutputNodes[strings.TrimSpace(output.Node)] = struct{}{}
		}
	}
	for nodeID, alias := range nodeVoices {
		if _, ok := textOutputNodes[nodeID]; !ok {
			return fmt.Errorf("node_voices.%s: node has no text/plain graph output", nodeID)
		}
		if err := runtimealias.Validate("node Voice alias", strings.TrimSpace(alias)); err != nil {
			return fmt.Errorf("node_voices.%s: %w", nodeID, err)
		}
	}
	return nil
}

type validationComponents struct{}

func (validationComponents) ResolveChatModel(context.Context, string) (model.BaseChatModel, error) {
	return nil, memory.ErrUnavailable
}

func (validationComponents) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return nil, memory.ErrUnavailable
}

type validationMemoryStore struct{}

func (validationMemoryStore) SupportsDirectFactObservation() bool { return true }

func (validationMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, memory.ErrUnavailable
}

func (validationMemoryStore) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, memory.ErrUnavailable
}

func (validationMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, memory.ErrUnavailable
}

func (validationMemoryStore) Delete(context.Context, memory.DeleteRequest) error {
	return memory.ErrUnavailable
}

func (validationMemoryStore) Wait(context.Context, memory.OperationRequest) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, memory.ErrUnavailable
}

// MapGraph maps the public Eino Graph to the Transformer contract.
func MapGraph(public apitypes.EinoGraph) (genxeino.GraphDefinition, error) {
	graph := genxeino.GraphDefinition{
		Name: public.Name,
		Compile: genxeino.GraphCompileConfig{
			NodeTriggerMode: genxeino.NodeTriggerMode(public.Compile.NodeTriggerMode),
		},
		State: genxeino.StateDefinition{},
	}
	if public.Compile.MaxRunSteps != nil {
		graph.Compile.MaxRunSteps = *public.Compile.MaxRunSteps
	}
	if public.Compile.FanIn != nil {
		graph.Compile.FanIn = make(map[string]genxeino.FanInConfig, len(*public.Compile.FanIn))
		for name, fanIn := range *public.Compile.FanIn {
			graph.Compile.FanIn[name] = genxeino.FanInConfig{
				StreamMergeWithSourceEOF: fanIn.StreamMergeWithSourceEof != nil && *fanIn.StreamMergeWithSourceEof,
			}
		}
	}
	for _, field := range public.State.Fields {
		graph.State.Fields = append(graph.State.Fields, genxeino.StateField{
			Name:     field.Name,
			Type:     genxeino.StateType(field.Type),
			Required: field.Required != nil && *field.Required,
			Merge:    genxeino.MergePolicy(field.Merge),
		})
	}
	for index, publicNode := range public.Nodes {
		node, err := mapNode(publicNode)
		if err != nil {
			return genxeino.GraphDefinition{}, fmt.Errorf("nodes[%d]: %w", index, err)
		}
		graph.Nodes = append(graph.Nodes, node)
	}
	for _, edge := range public.Edges {
		graph.Edges = append(graph.Edges, genxeino.EdgeDefinition{From: edge.From, To: edge.To})
	}
	for _, publicBranch := range public.Branches {
		branch := genxeino.BranchDefinition{
			From: publicBranch.From, Mode: genxeino.BranchMode(publicBranch.Mode), Default: publicBranch.Default,
		}
		for _, publicRoute := range publicBranch.Routes {
			branch.Routes = append(branch.Routes, genxeino.BranchRoute{
				When: mapPredicate(publicRoute.When), To: publicRoute.To,
			})
		}
		graph.Branches = append(graph.Branches, branch)
	}
	for _, output := range public.Outputs {
		graph.Outputs = append(graph.Outputs, genxeino.OutputDefinition{
			Node: output.Node, Field: output.Field, Name: output.Name, MIMEType: output.MimeType,
			Primary: output.Primary != nil && *output.Primary,
		})
	}
	return graph, nil
}

func mapNode(public apitypes.EinoNode) (genxeino.NodeDefinition, error) {
	discriminator, err := public.Discriminator()
	if err != nil {
		return genxeino.NodeDefinition{}, err
	}
	switch discriminator {
	case "prompt":
		value, err := public.AsEinoPromptNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Prompt = &genxeino.PromptNode{Format: genxeino.PromptFormat(value.Format)}
		for _, message := range value.Messages {
			node.Prompt.Messages = append(node.Prompt.Messages, genxeino.PromptMessage{
				Role: genxeino.PromptRole(stringEnumValue(message.Role)), Template: stringValue(message.Template),
				Placeholder: stringValue(message.Placeholder), Optional: boolValue(message.Optional),
			})
		}
		return node, nil
	case "chat_model":
		value, err := public.AsEinoChatModelNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.ChatModel = &genxeino.ChatModelNode{
			Model: value.Model, Temperature: value.Temperature, MaxTokens: value.MaxTokens,
		}
		return node, nil
	case "transform":
		value, err := public.AsEinoTransformNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Transform = &genxeino.TransformNode{
			Operation: genxeino.TransformOperation(value.Operation),
			Order:     valueOrZero(value.Order), Separator: stringValue(value.Separator),
			MaxInputBytes: intValue(value.MaxInputBytes), MaxOutputBytes: intValue(value.MaxOutputBytes),
		}
		for _, message := range valueOrZero(value.Messages) {
			node.Transform.Messages = append(node.Transform.Messages, genxeino.TransformMessage{
				Role: genxeino.PromptRole(message.Role), Input: stringValue(message.Input), Text: stringValue(message.Text),
			})
		}
		return node, nil
	case "script":
		value, err := public.AsEinoScriptNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		if value.Limits.MaxExecutionSteps <= 0 {
			return genxeino.NodeDefinition{}, errors.New("script max_execution_steps must be positive")
		}
		timeout, err := time.ParseDuration(value.Limits.Timeout)
		if err != nil {
			return genxeino.NodeDefinition{}, fmt.Errorf("script timeout: %w", err)
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Script = &genxeino.ScriptNode{
			Language: genxeino.ScriptLanguage(value.Language), Entrypoint: value.Entrypoint, Source: value.Source,
			Limits: genxeino.ScriptLimits{
				MaxExecutionSteps: uint64(value.Limits.MaxExecutionSteps), Timeout: timeout,
				MaxInputBytes: value.Limits.MaxInputBytes, MaxOutputBytes: value.Limits.MaxOutputBytes,
			},
		}
		return node, nil
	case "race":
		value, err := public.AsEinoRaceNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Race = &genxeino.RaceNode{
			Winner:         genxeino.RaceWinnerDefinition{Mode: genxeino.RaceWinnerMode(value.Winner.Mode)},
			MaxConcurrency: intValue(value.MaxConcurrency),
		}
		if value.Winner.When != nil {
			predicate := mapPredicate(*value.Winner.When)
			node.Race.Winner.When = &predicate
		}
		for _, branch := range value.Branches {
			graph, err := MapGraph(branch.Graph)
			if err != nil {
				return genxeino.NodeDefinition{}, err
			}
			node.Race.Branches = append(node.Race.Branches, genxeino.RaceBranch{ID: branch.Id, Graph: graph})
		}
		return node, nil
	case "batch":
		value, err := public.AsEinoBatchNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		graph, err := MapGraph(value.Graph)
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Batch = &genxeino.BatchNode{
			Items: genxeino.Binding{From: value.Items.From}, Graph: graph,
			MaxConcurrency: intValue(value.MaxConcurrency),
		}
		return node, nil
	case "passthrough":
		value, err := public.AsEinoPassthroughNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Passthrough = &genxeino.PassthroughNode{}
		return node, nil
	case "memory_recall":
		value, err := public.AsEinoMemoryRecallNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.MemoryRecall = &genxeino.MemoryRecallNode{
			QueryFrom: value.QueryFrom,
			Output:    value.Output,
			TopK:      value.TopK,
		}
		return node, nil
	case "memory_observe":
		value, err := public.AsEinoMemoryObserveNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.MemoryObserve = &genxeino.MemoryObserveNode{
			WaitForCompletion: boolValue(value.WaitForCompletion),
		}
		for _, fact := range value.Facts {
			attributes := map[string]string(nil)
			if fact.Attributes != nil {
				attributes = maps.Clone(*fact.Attributes)
			}
			node.MemoryObserve.Facts = append(node.MemoryObserve.Facts, genxeino.ObserveDefinition{
				TextFrom: fact.TextFrom, Attributes: attributes,
			})
		}
		return node, nil
	case "subgraph":
		value, err := public.AsEinoSubgraphNode()
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		graph, err := MapGraph(value.Graph)
		if err != nil {
			return genxeino.NodeDefinition{}, err
		}
		node := nodeBase(value.Id, value.Inputs, value.Outputs)
		node.Subgraph = &genxeino.SubgraphNode{Graph: graph}
		return node, nil
	default:
		return genxeino.NodeDefinition{}, fmt.Errorf("unsupported node type %q", discriminator)
	}
}

func nodeBase(id string, inputs *map[string]apitypes.EinoBinding, outputs *map[string]string) genxeino.NodeDefinition {
	node := genxeino.NodeDefinition{ID: id}
	if inputs != nil {
		node.Inputs = make(map[string]genxeino.Binding, len(*inputs))
		for name, binding := range *inputs {
			node.Inputs[name] = genxeino.Binding{From: binding.From}
		}
	}
	if outputs != nil {
		node.Outputs = make(map[string]string, len(*outputs))
		maps.Copy(node.Outputs, *outputs)
	}
	return node
}

func mapPredicate(public apitypes.EinoPredicate) genxeino.Predicate {
	predicate := genxeino.Predicate{
		Field: stringValue(public.Field), Value: public.Value,
	}
	if public.Op != nil {
		predicate.Op = genxeino.PredicateOperator(*public.Op)
	}
	if public.All != nil {
		predicate.All = make([]genxeino.Predicate, 0, len(*public.All))
		for _, child := range *public.All {
			predicate.All = append(predicate.All, mapPredicate(child))
		}
	}
	if public.Any != nil {
		predicate.Any = make([]genxeino.Predicate, 0, len(*public.Any))
		for _, child := range *public.Any {
			predicate.Any = append(predicate.Any, mapPredicate(child))
		}
	}
	if public.Not != nil {
		child := mapPredicate(*public.Not)
		predicate.Not = &child
	}
	return predicate
}

func valueOrZero[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringEnumValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
