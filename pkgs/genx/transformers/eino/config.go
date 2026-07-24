package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"mime"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const defaultMaxOutputBytes = 4 << 20

// Config declares one reusable Eino-backed Transformer.
type Config struct {
	Agent      AgentConfig
	Graph      GraphDefinition
	Components ComponentResolver
	Lambdas    LambdaResolver
	State      *StatePersistenceConfig
	History    *HistoryConfig
	Memory     *MemoryConfig
	Limits     Limits
}

// AgentConfig declares stable Transformer and conversation identity.
type AgentConfig struct {
	ID          string
	Name        string
	Description string
	ContextID   string
}

// Limits bounds package-owned output buffering.
type Limits struct {
	MaxOutputBytes int
}

// StatePersistenceConfig enables optimistic persistent Graph state.
type StatePersistenceConfig struct {
	Store  StateStore
	Scope  string
	Fields []string
}

// StateSnapshot is a versioned persistent state value.
type StateSnapshot struct {
	Version string
	Fields  map[string]any
}

// StateStore owns optimistic persistent Graph state.
type StateStore interface {
	Load(context.Context, string) (StateSnapshot, error)
	CompareAndSwap(context.Context, string, string, map[string]any) (StateSnapshot, error)
}

// HistoryConfig enables ordered short-term conversation History.
type HistoryConfig struct {
	Store logstore.MutableStore
	Scope string
	Limit int
}

// MemoryConfig enables provider-neutral Recall and Observe.
type MemoryConfig struct {
	Store   memory.Store
	Scope   memory.Scope
	Recall  []RecallDefinition
	Observe ObservePolicy
}

// RecallDefinition binds recalled Memory to a Graph state field.
type RecallDefinition struct {
	QueryFrom string
	Output    string
	TopK      int
}

// ObserveDefinition maps a text state field and optional attributes to one
// provider-neutral FactCandidate.
type ObserveDefinition struct {
	TextFrom   string
	Attributes map[string]string
}

// ObservePolicy declares post-delivery Memory behavior.
type ObservePolicy struct {
	Enabled           bool
	WaitForCompletion bool
	Facts             []ObserveDefinition
}

type normalizedConfig struct {
	Config
	fields    map[string]StateField
	nodes     map[string]NodeDefinition
	outputs   map[string][]OutputDefinition
	primary   OutputDefinition
	graphCopy GraphDefinition
}

func normalizeConfig(source Config) (*normalizedConfig, error) {
	config := source
	config.State = cloneStateConfig(source.State)
	config.History = cloneHistoryConfig(source.History)
	config.Memory = cloneMemoryConfig(source.Memory)
	config.Agent.ID = strings.TrimSpace(config.Agent.ID)
	config.Agent.Name = strings.TrimSpace(config.Agent.Name)
	config.Agent.ContextID = strings.TrimSpace(config.Agent.ContextID)
	if config.Agent.ID == "" {
		return nil, fmt.Errorf("eino: Agent.ID is required")
	}
	if config.Limits.MaxOutputBytes == 0 {
		config.Limits.MaxOutputBytes = defaultMaxOutputBytes
	}
	if config.Limits.MaxOutputBytes < 0 {
		return nil, fmt.Errorf("eino: Limits.MaxOutputBytes cannot be negative")
	}
	graph, err := cloneGraph(source.Graph)
	if err != nil {
		return nil, fmt.Errorf("eino: clone Graph: %w", err)
	}
	config.Graph = graph
	result := &normalizedConfig{
		Config: config, graphCopy: graph,
		fields: make(map[string]StateField), nodes: make(map[string]NodeDefinition),
		outputs: make(map[string][]OutputDefinition),
	}
	if err := result.validateGraph(graph, "Graph", 0); err != nil {
		return nil, err
	}
	if err := result.validateOptionalConfig(); err != nil {
		return nil, err
	}
	return result, nil
}

func cloneStateConfig(source *StatePersistenceConfig) *StatePersistenceConfig {
	if source == nil {
		return nil
	}
	result := *source
	result.Fields = slices.Clone(source.Fields)
	return &result
}

func cloneHistoryConfig(source *HistoryConfig) *HistoryConfig {
	if source == nil {
		return nil
	}
	result := *source
	return &result
}

func cloneMemoryConfig(source *MemoryConfig) *MemoryConfig {
	if source == nil {
		return nil
	}
	result := *source
	result.Recall = slices.Clone(source.Recall)
	result.Observe.Facts = make([]ObserveDefinition, len(source.Observe.Facts))
	for index, fact := range source.Observe.Facts {
		result.Observe.Facts[index] = fact
		result.Observe.Facts[index].Attributes = make(map[string]string, len(fact.Attributes))
		maps.Copy(result.Observe.Facts[index].Attributes, fact.Attributes)
	}
	return &result
}

func cloneGraph(source GraphDefinition) (GraphDefinition, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return GraphDefinition{}, err
	}
	var graph GraphDefinition
	if err := json.Unmarshal(data, &graph); err != nil {
		return GraphDefinition{}, err
	}
	if err := restorePredicateValues(source, &graph); err != nil {
		return GraphDefinition{}, err
	}
	return graph, nil
}

func restorePredicateValues(source GraphDefinition, target *GraphDefinition) error {
	if len(source.Branches) != len(target.Branches) || len(source.Nodes) != len(target.Nodes) {
		return fmt.Errorf("Graph shape changed while copying")
	}
	for index := range source.Branches {
		if err := restorePredicateValue(source.Branches[index].Routes, target.Branches[index].Routes); err != nil {
			return fmt.Errorf("Branch[%d]: %w", index, err)
		}
	}
	for index := range source.Nodes {
		sourceNode := source.Nodes[index]
		targetNode := &target.Nodes[index]
		switch {
		case sourceNode.Subgraph != nil:
			if err := restorePredicateValues(sourceNode.Subgraph.Graph, &targetNode.Subgraph.Graph); err != nil {
				return err
			}
		case sourceNode.Batch != nil:
			if err := restorePredicateValues(sourceNode.Batch.Graph, &targetNode.Batch.Graph); err != nil {
				return err
			}
		case sourceNode.Race != nil:
			if sourceNode.Race.Winner.When != nil {
				if err := copyPredicateValue(sourceNode.Race.Winner.When, targetNode.Race.Winner.When); err != nil {
					return err
				}
			}
			for branchIndex := range sourceNode.Race.Branches {
				if err := restorePredicateValues(
					sourceNode.Race.Branches[branchIndex].Graph,
					&targetNode.Race.Branches[branchIndex].Graph,
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func restorePredicateValue(source, target []BranchRoute) error {
	if len(source) != len(target) {
		return fmt.Errorf("route shape changed while copying")
	}
	for index := range source {
		if err := copyPredicateValue(&source[index].When, &target[index].When); err != nil {
			return fmt.Errorf("Route[%d]: %w", index, err)
		}
	}
	return nil
}

func copyPredicateValue(source, target *Predicate) error {
	value, err := cloneValue(source.Value)
	if err != nil && source.Value != nil {
		return err
	}
	target.Value = value
	if len(source.All) != len(target.All) || len(source.Any) != len(target.Any) ||
		(source.Not == nil) != (target.Not == nil) {
		return fmt.Errorf("predicate shape changed while copying")
	}
	for index := range source.All {
		if err := copyPredicateValue(&source.All[index], &target.All[index]); err != nil {
			return err
		}
	}
	for index := range source.Any {
		if err := copyPredicateValue(&source.Any[index], &target.Any[index]); err != nil {
			return err
		}
	}
	if source.Not != nil {
		return copyPredicateValue(source.Not, target.Not)
	}
	return nil
}

func (config *normalizedConfig) validateGraph(graph GraphDefinition, path string, depth int) error {
	if depth > 16 {
		return fmt.Errorf("eino: %s exceeds maximum nesting depth 16", path)
	}
	if strings.TrimSpace(graph.Name) != graph.Name {
		return fmt.Errorf("eino: %s Name cannot contain surrounding whitespace", path)
	}
	switch graph.Compile.NodeTriggerMode {
	case "", NodeTriggerAnyPredecessor:
	case NodeTriggerAllPredecessor:
	default:
		return fmt.Errorf("eino: %s has unsupported NodeTriggerMode %q", path, graph.Compile.NodeTriggerMode)
	}
	if graph.Compile.MaxRunSteps < 0 {
		return fmt.Errorf("eino: %s MaxRunSteps cannot be negative", path)
	}
	fields := make(map[string]StateType, len(graph.State.Fields))
	for index, field := range graph.State.Fields {
		if field.Name == "" || strings.TrimSpace(field.Name) != field.Name || !validStateType(field.Type) {
			return fmt.Errorf("eino: %s State.Fields[%d] is invalid", path, index)
		}
		if field.Merge == "" {
			field.Merge = MergeReplace
		}
		if !validMerge(field.Type, field.Merge) {
			return fmt.Errorf("eino: %s State field %q has incompatible merge %q", path, field.Name, field.Merge)
		}
		if _, duplicate := fields[field.Name]; duplicate {
			return fmt.Errorf("eino: %s has duplicate State field %q", path, field.Name)
		}
		fields[field.Name] = field.Type
		if depth == 0 {
			config.fields[field.Name] = field
		}
	}
	nodes := make(map[string]NodeDefinition, len(graph.Nodes))
	needsComponents := false
	needsLambdas := false
	for index, node := range graph.Nodes {
		if node.ID == "" || strings.TrimSpace(node.ID) != node.ID ||
			node.ID == "start" || node.ID == "end" || node.kindCount() != 1 {
			return fmt.Errorf("eino: %s Nodes[%d] requires a non-reserved ID and exactly one node body", path, index)
		}
		if _, duplicate := nodes[node.ID]; duplicate {
			return fmt.Errorf("eino: %s has duplicate node %q", path, node.ID)
		}
		for port, binding := range node.Inputs {
			if strings.TrimSpace(port) == "" {
				return fmt.Errorf("eino: %s node %q has blank input port", path, node.ID)
			}
			if err := validateBinding(binding, fields); err != nil {
				return fmt.Errorf("eino: %s node %q input %q: %w", path, node.ID, port, err)
			}
		}
		nodeOutputFields := make(map[string]struct{}, len(node.Outputs))
		for port, field := range node.Outputs {
			if strings.TrimSpace(port) == "" {
				return fmt.Errorf("eino: %s node %q has blank output port", path, node.ID)
			}
			if _, ok := fields[field]; !ok {
				return fmt.Errorf("eino: %s node %q output %q targets undeclared field %q", path, node.ID, port, field)
			}
			if _, duplicate := nodeOutputFields[field]; duplicate {
				return fmt.Errorf("eino: %s node %q writes State field %q more than once", path, node.ID, field)
			}
			nodeOutputFields[field] = struct{}{}
		}
		if err := config.validateNode(node, fields, path, depth); err != nil {
			return err
		}
		if err := validateNodePorts(node, fields, path); err != nil {
			return err
		}
		needsComponents = needsComponents || node.ChatModel != nil || node.Retriever != nil
		needsLambdas = needsLambdas || node.Lambda != nil
		nodes[node.ID] = node
		if depth == 0 {
			config.nodes[node.ID] = node
		}
	}
	if needsComponents && config.Components == nil {
		return fmt.Errorf("eino: Components is required by %s", path)
	}
	if needsLambdas && config.Lambdas == nil {
		return fmt.Errorf("eino: Lambdas is required by %s", path)
	}
	if len(nodes) == 0 {
		return fmt.Errorf("eino: %s requires Nodes", path)
	}
	adjacency := make(map[string][]string, len(nodes)+1)
	reverse := make(map[string][]string, len(nodes)+1)
	for index, edge := range graph.Edges {
		if !validEndpoint(edge.From, nodes, true) || !validEndpoint(edge.To, nodes, false) {
			return fmt.Errorf("eino: %s Edges[%d] has unknown or invalid endpoint", path, index)
		}
		if edge.From == edge.To {
			if graph.Compile.NodeTriggerMode == NodeTriggerAllPredecessor {
				return fmt.Errorf("eino: %s Edges[%d] creates an all_predecessor self-cycle", path, index)
			}
		}
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		reverse[edge.To] = append(reverse[edge.To], edge.From)
	}
	for index, branch := range graph.Branches {
		if _, ok := nodes[branch.From]; !ok {
			return fmt.Errorf("eino: %s Branches[%d] has unknown source %q", path, index, branch.From)
		}
		if branch.Mode != BranchFirstMatch && branch.Mode != BranchAllMatch {
			return fmt.Errorf("eino: %s Branches[%d] has unsupported mode %q", path, index, branch.Mode)
		}
		if len(branch.Routes) == 0 || !validEndpoint(branch.Default, nodes, false) {
			return fmt.Errorf("eino: %s Branches[%d] requires routes and valid Default", path, index)
		}
		destinations := map[string]struct{}{branch.Default: {}}
		for routeIndex, route := range branch.Routes {
			if !validEndpoint(route.To, nodes, false) {
				return fmt.Errorf("eino: %s Branches[%d].Routes[%d] has unknown target", path, index, routeIndex)
			}
			if err := validatePredicate(route.When, fields); err != nil {
				return fmt.Errorf("eino: %s Branches[%d].Routes[%d]: %w", path, index, routeIndex, err)
			}
			destinations[route.To] = struct{}{}
		}
		for target := range destinations {
			adjacency[branch.From] = append(adjacency[branch.From], target)
			reverse[target] = append(reverse[target], branch.From)
		}
	}
	if len(adjacency["start"]) == 0 || len(reverse["end"]) == 0 {
		return fmt.Errorf("eino: %s requires paths from start and to end", path)
	}
	reachable := graphReachable(adjacency, "start")
	toEnd := graphReachable(reverse, "end")
	for node := range nodes {
		if !reachable[node] {
			return fmt.Errorf("eino: %s node %q is unreachable", path, node)
		}
		if !toEnd[node] {
			return fmt.Errorf("eino: %s node %q cannot reach end", path, node)
		}
	}
	for node := range graph.Compile.FanIn {
		if _, ok := nodes[node]; !ok {
			return fmt.Errorf("eino: %s FanIn references unknown node %q", path, node)
		}
		if len(reverse[node]) < 2 {
			return fmt.Errorf("eino: %s FanIn node %q requires at least two predecessors", path, node)
		}
	}
	if err := validateStateWriters(nodes, graph.Branches, adjacency, reverse, path); err != nil {
		return err
	}
	cyclic := graphHasCycle(adjacency)
	if graph.Compile.NodeTriggerMode == NodeTriggerAllPredecessor && cyclic {
		return fmt.Errorf("eino: %s all_predecessor Graph cannot contain a cycle", path)
	}
	if cyclic && graph.Compile.MaxRunSteps <= 0 {
		return fmt.Errorf("eino: %s cyclic Graph requires positive MaxRunSteps", path)
	}
	primary := 0
	seenOutputs := make(map[string]struct{}, len(graph.Outputs))
	seenSources := make(map[string]struct{}, len(graph.Outputs))
	for index, output := range graph.Outputs {
		stateType, ok := fields[output.Field]
		if !ok {
			return fmt.Errorf("eino: %s Outputs[%d] references undeclared field %q", path, index, output.Field)
		}
		node, ok := nodes[output.Node]
		if !ok || !nodeProduces(node, output.Field) {
			return fmt.Errorf("eino: %s Outputs[%d] references invalid node field", path, index)
		}
		if output.Name == "" || strings.TrimSpace(output.Name) != output.Name {
			return fmt.Errorf("eino: %s Outputs[%d] requires Name", path, index)
		}
		if _, duplicate := seenOutputs[output.Name]; duplicate {
			return fmt.Errorf("eino: %s has duplicate output name %q", path, output.Name)
		}
		source := output.Node + "\x00" + output.Field
		if _, duplicate := seenSources[source]; duplicate {
			return fmt.Errorf("eino: %s publishes %s.%s more than once", path, output.Node, output.Field)
		}
		seenOutputs[output.Name] = struct{}{}
		seenSources[source] = struct{}{}
		if err := validateOutputMIME(stateType, output.MIMEType); err != nil {
			return fmt.Errorf("eino: %s Outputs[%d]: %w", path, index, err)
		}
		if output.Primary {
			primary++
		}
		if depth == 0 {
			config.outputs[output.Node] = append(config.outputs[output.Node], output)
			if output.Primary {
				config.primary = output
			}
		}
	}
	if len(graph.Outputs) == 0 || primary != 1 {
		return fmt.Errorf("eino: %s requires exactly one primary Output", path)
	}
	for _, output := range graph.Outputs {
		if output.Primary && canReachAvoiding(adjacency, "start", "end", output.Node) {
			return fmt.Errorf(
				"eino: %s has a start-to-end path that bypasses primary Output node %q",
				path,
				output.Node,
			)
		}
	}
	return nil
}

func (config *normalizedConfig) validateNode(node NodeDefinition, fields map[string]StateType, path string, depth int) error {
	nodePath := fmt.Sprintf("%s node %q", path, node.ID)
	switch {
	case node.Prompt != nil:
		if node.Prompt.Format != PromptFString && node.Prompt.Format != PromptGoTemplate && node.Prompt.Format != PromptJinja2 {
			return fmt.Errorf("eino: %s has unsupported Prompt format %q", nodePath, node.Prompt.Format)
		}
		if len(node.Prompt.Messages) == 0 {
			return fmt.Errorf("eino: %s requires Prompt messages", nodePath)
		}
	case node.ChatModel != nil:
		if strings.TrimSpace(node.ChatModel.Model) == "" {
			return fmt.Errorf("eino: %s requires model", nodePath)
		}
		if node.ChatModel.Temperature != nil && (math.IsNaN(float64(*node.ChatModel.Temperature)) || math.IsInf(float64(*node.ChatModel.Temperature), 0)) {
			return fmt.Errorf("eino: %s Temperature must be finite", nodePath)
		}
		if node.ChatModel.MaxTokens != nil && *node.ChatModel.MaxTokens <= 0 {
			return fmt.Errorf("eino: %s MaxTokens must be positive", nodePath)
		}
	case node.Transform != nil:
		if node.Transform.Operation == "" {
			return fmt.Errorf("eino: %s requires Transform operation", nodePath)
		}
	case node.Script != nil:
		if node.Script.Language != ScriptStarlark || strings.TrimSpace(node.Script.Source) == "" ||
			node.Script.Limits.MaxExecutionSteps == 0 || node.Script.Limits.Timeout <= 0 ||
			node.Script.Limits.MaxInputBytes <= 0 || node.Script.Limits.MaxOutputBytes <= 0 {
			return fmt.Errorf("eino: %s has invalid Script configuration", nodePath)
		}
	case node.Lambda != nil:
		if strings.TrimSpace(node.Lambda.Lambda) == "" {
			return fmt.Errorf("eino: %s requires Lambda name", nodePath)
		}
	case node.Race != nil:
		if node.Race.MaxConcurrency <= 0 || len(node.Race.Branches) == 0 {
			return fmt.Errorf("eino: %s has invalid Race configuration", nodePath)
		}
		switch node.Race.Winner.Mode {
		case RaceFirstOutput, RaceFirstSuccess:
			if node.Race.Winner.When != nil {
				return fmt.Errorf("eino: %s Race winner %q cannot define When", nodePath, node.Race.Winner.Mode)
			}
		case RacePredicate:
			if node.Race.Winner.When == nil {
				return fmt.Errorf("eino: %s predicate Race winner requires When", nodePath)
			}
		default:
			return fmt.Errorf("eino: %s has unsupported Race winner mode %q", nodePath, node.Race.Winner.Mode)
		}
		branchIDs := make(map[string]struct{}, len(node.Race.Branches))
		for index, branch := range node.Race.Branches {
			if strings.TrimSpace(branch.ID) == "" || strings.TrimSpace(branch.ID) != branch.ID {
				return fmt.Errorf("eino: %s Race branch %d requires ID", nodePath, index)
			}
			if _, duplicate := branchIDs[branch.ID]; duplicate {
				return fmt.Errorf("eino: %s has duplicate Race branch %q", nodePath, branch.ID)
			}
			branchIDs[branch.ID] = struct{}{}
			if err := config.validateGraph(branch.Graph, fmt.Sprintf("%s Race[%s]", nodePath, branch.ID), depth+1); err != nil {
				return err
			}
			if node.Race.Winner.When != nil {
				branchFields := stateTypes(branch.Graph.State.Fields)
				if err := validatePredicate(*node.Race.Winner.When, branchFields); err != nil {
					return fmt.Errorf("eino: %s Race[%s] winner predicate: %w", nodePath, branch.ID, err)
				}
			}
		}
	case node.Batch != nil:
		if node.Batch.MaxConcurrency <= 0 {
			return fmt.Errorf("eino: %s requires positive Batch MaxConcurrency", nodePath)
		}
		if err := validateBinding(node.Batch.Items, fields); err != nil {
			return fmt.Errorf("eino: %s Batch Items: %w", nodePath, err)
		}
		if err := config.validateGraph(node.Batch.Graph, nodePath+" Batch", depth+1); err != nil {
			return err
		}
	case node.Passthrough != nil:
		if len(node.Inputs) != 1 || len(node.Outputs) != 1 {
			return fmt.Errorf("eino: %s Passthrough requires one input and output", nodePath)
		}
	case node.Retriever != nil:
		if strings.TrimSpace(node.Retriever.Retriever) == "" || node.Retriever.TopK <= 0 {
			return fmt.Errorf("eino: %s has invalid Retriever configuration", nodePath)
		}
		if err := validateBinding(node.Retriever.Query, fields); err != nil {
			return fmt.Errorf("eino: %s Retriever Query: %w", nodePath, err)
		}
	case node.Subgraph != nil:
		if err := config.validateGraph(node.Subgraph.Graph, nodePath+" Subgraph", depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateNodePorts(node NodeDefinition, fields map[string]StateType, path string) error {
	nodePath := fmt.Sprintf("%s node %q", path, node.ID)
	inputType := func(port string) (StateType, error) {
		binding, ok := node.Inputs[port]
		if !ok {
			return "", fmt.Errorf("eino: %s requires input port %q", nodePath, port)
		}
		return bindingStateType(binding, fields)
	}
	outputType := func(port string) (StateType, error) {
		field, ok := node.Outputs[port]
		if !ok {
			return "", fmt.Errorf("eino: %s requires output port %q", nodePath, port)
		}
		return fields[field], nil
	}
	requireExact := func(actual map[string]Binding, expected ...string) error {
		if !sameKeys(actual, expected) {
			return fmt.Errorf("eino: %s input ports must be exactly %v", nodePath, expected)
		}
		return nil
	}
	requireExactOutputs := func(expected ...string) error {
		if !sameStringKeys(node.Outputs, expected) {
			return fmt.Errorf("eino: %s output ports must be exactly %v", nodePath, expected)
		}
		return nil
	}
	requireType := func(actual, expected StateType, port string) error {
		if actual != expected {
			return fmt.Errorf("eino: %s port %q requires %s, got %s", nodePath, port, expected, actual)
		}
		return nil
	}

	switch {
	case node.Prompt != nil:
		if err := requireExactOutputs("messages"); err != nil {
			return err
		}
		if actual, _ := outputType("messages"); actual != StateMessages {
			return requireType(actual, StateMessages, "messages")
		}
		for _, message := range node.Prompt.Messages {
			if message.Placeholder == "" {
				if message.Role == "" || message.Template == "" {
					return fmt.Errorf("eino: %s Prompt messages require Role and Template", nodePath)
				}
				continue
			}
			actual, err := inputType(message.Placeholder)
			if err != nil {
				return err
			}
			if err := requireType(actual, StateMessages, message.Placeholder); err != nil {
				return err
			}
		}
	case node.ChatModel != nil:
		if err := requireExact(node.Inputs, "messages"); err != nil {
			return err
		}
		actual, err := inputType("messages")
		if err != nil {
			return err
		}
		if err := requireType(actual, StateMessages, "messages"); err != nil {
			return err
		}
		if len(node.Outputs) == 0 || len(node.Outputs) > 2 {
			return fmt.Errorf("eino: %s requires text and/or messages output", nodePath)
		}
		for port := range node.Outputs {
			actual, _ := outputType(port)
			switch port {
			case "text":
				if err := requireType(actual, StateString, port); err != nil {
					return err
				}
			case "messages":
				if err := requireType(actual, StateMessages, port); err != nil {
					return err
				}
			default:
				return fmt.Errorf("eino: %s has unsupported ChatModel output %q", nodePath, port)
			}
		}
	case node.Transform != nil:
		switch node.Transform.Operation {
		case TransformSelect:
			if err := requireExact(node.Inputs, "value"); err != nil {
				return err
			}
			if err := requireExactOutputs("value"); err != nil {
				return err
			}
			in, err := inputType("value")
			if err != nil {
				return err
			}
			out, _ := outputType("value")
			if in != out {
				return fmt.Errorf("eino: %s select input and output types differ", nodePath)
			}
		case TransformConcatText:
			if len(node.Transform.Order) == 0 || !sameKeys(node.Inputs, node.Transform.Order) {
				return fmt.Errorf("eino: %s concat_text inputs must match non-empty Order", nodePath)
			}
			seen := make(map[string]struct{}, len(node.Transform.Order))
			for _, port := range node.Transform.Order {
				if _, duplicate := seen[port]; duplicate {
					return fmt.Errorf("eino: %s concat_text has duplicate Order port %q", nodePath, port)
				}
				seen[port] = struct{}{}
				actual, err := inputType(port)
				if err != nil {
					return err
				}
				if err := requireType(actual, StateString, port); err != nil {
					return err
				}
			}
			if err := requireExactOutputs("text"); err != nil {
				return err
			}
			if actual, _ := outputType("text"); actual != StateString {
				return requireType(actual, StateString, "text")
			}
		case TransformDecodeJSON:
			if err := requireExact(node.Inputs, "text"); err != nil {
				return err
			}
			if err := requireExactOutputs("object"); err != nil {
				return err
			}
			if node.Transform.MaxInputBytes <= 0 || node.Transform.MaxOutputBytes <= 0 {
				return fmt.Errorf("eino: %s decode_json requires positive input and output byte limits", nodePath)
			}
			if actual, _ := inputType("text"); actual != StateString {
				return requireType(actual, StateString, "text")
			}
			if actual, _ := outputType("object"); actual != StateObject {
				return requireType(actual, StateObject, "object")
			}
		case TransformBuildMessages:
			if len(node.Transform.Messages) == 0 {
				return fmt.Errorf("eino: %s build_messages requires Messages", nodePath)
			}
			expected := make([]string, 0, len(node.Transform.Messages))
			seen := make(map[string]struct{})
			for _, message := range node.Transform.Messages {
				if (message.Input == "") == (message.Text == "") {
					return fmt.Errorf("eino: %s build_messages items require exactly one Input or Text", nodePath)
				}
				if message.Role != PromptSystem && message.Role != PromptUser && message.Role != PromptAssistant {
					return fmt.Errorf("eino: %s build_messages has unsupported role %q", nodePath, message.Role)
				}
				if message.Input != "" {
					if _, duplicate := seen[message.Input]; !duplicate {
						seen[message.Input] = struct{}{}
						expected = append(expected, message.Input)
					}
					actual, err := inputType(message.Input)
					if err != nil {
						return err
					}
					if err := requireType(actual, StateString, message.Input); err != nil {
						return err
					}
				}
			}
			if !sameKeys(node.Inputs, expected) {
				return fmt.Errorf("eino: %s build_messages inputs must match referenced items", nodePath)
			}
			if err := requireExactOutputs("messages"); err != nil {
				return err
			}
			if actual, _ := outputType("messages"); actual != StateMessages {
				return requireType(actual, StateMessages, "messages")
			}
		default:
			return fmt.Errorf("eino: %s has unsupported Transform operation %q", nodePath, node.Transform.Operation)
		}
	case node.Script != nil:
		if len(node.Outputs) == 0 {
			return fmt.Errorf("eino: %s Script requires declared output ports", nodePath)
		}
	case node.Lambda != nil:
		if len(node.Inputs) == 0 || len(node.Outputs) == 0 {
			return fmt.Errorf("eino: %s Lambda requires declared input and output ports", nodePath)
		}
	case node.Race != nil:
		for _, graph := range raceGraphs(node.Race.Branches) {
			if err := validateCompositeInputs(nodePath, node.Inputs, fields, graph); err != nil {
				return err
			}
		}
		if err := validateCompositeOutputs(nodePath, node.Outputs, fields, raceGraphs(node.Race.Branches)); err != nil {
			return err
		}
	case node.Batch != nil:
		if len(node.Inputs) != 0 {
			return fmt.Errorf("eino: %s Batch uses Items and cannot define node Inputs", nodePath)
		}
		itemType, err := bindingStateType(node.Batch.Items, fields)
		if err != nil {
			return err
		}
		if itemType != StateList {
			return fmt.Errorf("eino: %s Batch Items must be a list", nodePath)
		}
		if err := requireExactOutputs("items"); err != nil {
			return err
		}
		if actual, _ := outputType("items"); actual != StateList {
			return requireType(actual, StateList, "items")
		}
	case node.Passthrough != nil:
		if err := requireExact(node.Inputs, "value"); err != nil {
			return err
		}
		if err := requireExactOutputs("value"); err != nil {
			return err
		}
		in, _ := inputType("value")
		out, _ := outputType("value")
		if in != out {
			return fmt.Errorf("eino: %s Passthrough input and output types differ", nodePath)
		}
	case node.Retriever != nil:
		if len(node.Inputs) != 0 {
			return fmt.Errorf("eino: %s Retriever uses Query and cannot define node Inputs", nodePath)
		}
		queryType, err := bindingStateType(node.Retriever.Query, fields)
		if err != nil {
			return err
		}
		if queryType != StateString {
			return fmt.Errorf("eino: %s Retriever Query must be text", nodePath)
		}
		if err := requireExactOutputs("documents"); err != nil {
			return err
		}
		if actual, _ := outputType("documents"); actual != StateDocuments {
			return requireType(actual, StateDocuments, "documents")
		}
	case node.Subgraph != nil:
		if err := validateCompositeInputs(nodePath, node.Inputs, fields, node.Subgraph.Graph); err != nil {
			return err
		}
		if err := validateCompositeOutputs(nodePath, node.Outputs, fields, []GraphDefinition{node.Subgraph.Graph}); err != nil {
			return err
		}
	}
	return nil
}

func sameKeys[V any](actual map[string]V, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, ok := actual[key]; !ok {
			return false
		}
	}
	return true
}

func sameStringKeys(actual map[string]string, expected []string) bool {
	return sameKeys(actual, expected)
}

func bindingStateType(binding Binding, fields map[string]StateType) (StateType, error) {
	source := strings.TrimSpace(binding.From)
	switch source {
	case "input.text", "memory.recalled":
		return StateString, nil
	case "input.messages", "history.messages":
		return StateMessages, nil
	case "input.parts":
		return StateList, nil
	default:
		stateType, ok := fields[source]
		if !ok {
			return "", fmt.Errorf("eino: binding source %q is not declared", source)
		}
		return stateType, nil
	}
}

func stateTypes(fields []StateField) map[string]StateType {
	result := make(map[string]StateType, len(fields))
	for _, field := range fields {
		result[field.Name] = field.Type
	}
	return result
}

func raceGraphs(branches []RaceBranch) []GraphDefinition {
	result := make([]GraphDefinition, len(branches))
	for index, branch := range branches {
		result[index] = branch.Graph
	}
	return result
}

func validateCompositeInputs(
	nodePath string,
	inputs map[string]Binding,
	parentFields map[string]StateType,
	graph GraphDefinition,
) error {
	childFields := stateTypes(graph.State.Fields)
	allowed := make(map[string]StateType, len(childFields)+3)
	maps.Copy(allowed, childFields)
	allowed["text"] = StateString
	allowed["messages"] = StateMessages
	allowed["parts"] = StateList
	for port, binding := range inputs {
		expected, ok := allowed[port]
		if !ok {
			return fmt.Errorf("eino: %s composite input %q has no matching child input or State field", nodePath, port)
		}
		actual, err := bindingStateType(binding, parentFields)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("eino: %s composite input %q requires %s, got %s", nodePath, port, expected, actual)
		}
	}
	for _, required := range []struct {
		source string
		port   string
	}{
		{source: "input.text", port: "text"},
		{source: "input.messages", port: "messages"},
		{source: "input.parts", port: "parts"},
	} {
		if graphUsesBinding(graph, required.source) {
			if _, ok := inputs[required.port]; !ok {
				return fmt.Errorf("eino: %s composite requires input %q for child binding %q", nodePath, required.port, required.source)
			}
		}
	}
	return nil
}

func validateCompositeOutputs(
	nodePath string,
	outputs map[string]string,
	parentFields map[string]StateType,
	graphs []GraphDefinition,
) error {
	var schema map[string]StateType
	for index, graph := range graphs {
		childFields := stateTypes(graph.State.Fields)
		current := make(map[string]StateType, len(graph.Outputs))
		for _, output := range graph.Outputs {
			current[output.Name] = childFields[output.Field]
		}
		if index == 0 {
			schema = current
			continue
		}
		if len(current) != len(schema) {
			return fmt.Errorf("eino: %s composite child output schemas differ", nodePath)
		}
		for name, stateType := range schema {
			if current[name] != stateType {
				return fmt.Errorf("eino: %s composite child output %q schemas differ", nodePath, name)
			}
		}
	}
	if len(schema) == 0 || len(outputs) != len(schema) {
		return fmt.Errorf("eino: %s outputs must match child Graph outputs", nodePath)
	}
	for port, stateType := range schema {
		field, ok := outputs[port]
		if !ok {
			return fmt.Errorf("eino: %s is missing composite output %q", nodePath, port)
		}
		if parentFields[field] != stateType {
			return fmt.Errorf("eino: %s composite output %q requires %s, got %s", nodePath, port, stateType, parentFields[field])
		}
	}
	return nil
}

func validateStateWriters(
	nodes map[string]NodeDefinition,
	branches []BranchDefinition,
	adjacency map[string][]string,
	reverse map[string][]string,
	path string,
) error {
	writers := make(map[string][]string)
	for nodeID, node := range nodes {
		for _, field := range node.Outputs {
			writers[field] = append(writers[field], nodeID)
		}
	}
	for field, nodeIDs := range writers {
		if len(nodeIDs) < 2 {
			continue
		}
		for left := range len(nodeIDs) {
			for right := left + 1; right < len(nodeIDs); right++ {
				a, b := nodeIDs[left], nodeIDs[right]
				if canReach(adjacency, a, b) || canReach(adjacency, b, a) ||
					exclusiveFirstMatchDestinations(branches, reverse, a, b) {
					continue
				}
				return fmt.Errorf(
					"eino: %s nodes %q and %q may concurrently write State field %q",
					path, a, b, field,
				)
			}
		}
	}
	return nil
}

func canReach(adjacency map[string][]string, from, to string) bool {
	if from == to {
		return true
	}
	return graphReachable(adjacency, from)[to]
}

func canReachAvoiding(adjacency map[string][]string, from, to, avoided string) bool {
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, target := range adjacency[node] {
			if target == avoided || seen[target] {
				continue
			}
			if target == to {
				return true
			}
			seen[target] = true
			queue = append(queue, target)
		}
	}
	return false
}

func exclusiveFirstMatchDestinations(
	branches []BranchDefinition,
	reverse map[string][]string,
	left, right string,
) bool {
	for _, branch := range branches {
		if branch.Mode != BranchFirstMatch {
			continue
		}
		destinations := map[string]struct{}{branch.Default: {}}
		for _, route := range branch.Routes {
			destinations[route.To] = struct{}{}
		}
		_, hasLeft := destinations[left]
		_, hasRight := destinations[right]
		if hasLeft && hasRight &&
			onlyPredecessor(reverse[left], branch.From) &&
			onlyPredecessor(reverse[right], branch.From) {
			return true
		}
	}
	return false
}

func onlyPredecessor(predecessors []string, source string) bool {
	if len(predecessors) == 0 {
		return false
	}
	for _, predecessor := range predecessors {
		if predecessor != source {
			return false
		}
	}
	return true
}

func (config *normalizedConfig) validateOptionalConfig() error {
	if config.State != nil {
		config.State.Scope = strings.TrimSpace(config.State.Scope)
		if config.State.Store == nil || config.State.Scope == "" || len(config.State.Fields) == 0 {
			return fmt.Errorf("eino: State requires Store, Scope, and Fields")
		}
		seen := make(map[string]struct{}, len(config.State.Fields))
		for _, field := range config.State.Fields {
			if _, ok := config.fields[field]; !ok {
				return fmt.Errorf("eino: State field %q is not declared", field)
			}
			if _, duplicate := seen[field]; duplicate {
				return fmt.Errorf("eino: State contains duplicate field %q", field)
			}
			seen[field] = struct{}{}
		}
	}
	if config.History != nil {
		config.History.Scope = strings.TrimSpace(config.History.Scope)
		if config.History.Limit <= 0 || config.History.Limit > logstore.MaxLimit {
			return fmt.Errorf("eino: History.Limit must be between 1 and %d", logstore.MaxLimit)
		}
	}
	if config.Memory != nil {
		config.Memory.Scope = memory.Scope(strings.TrimSpace(string(config.Memory.Scope)))
		if config.Memory.Store == nil || config.Memory.Scope == "" {
			return fmt.Errorf("eino: Memory requires Store and Scope")
		}
		for index, recall := range config.Memory.Recall {
			if recall.TopK <= 0 || strings.TrimSpace(recall.QueryFrom) == "" {
				return fmt.Errorf("eino: Memory.Recall[%d] is invalid", index)
			}
			if field, ok := config.fields[recall.Output]; !ok || field.Type != StateString {
				return fmt.Errorf("eino: Memory.Recall[%d] Output must be a string State field", index)
			}
		}
		if config.Memory.Observe.WaitForCompletion {
			if !config.Memory.Observe.Enabled {
				return fmt.Errorf("eino: Memory Observe wait requires Enabled")
			}
			if _, ok := config.Memory.Store.(memory.OperationWaiter); !ok {
				return fmt.Errorf("eino: Memory Observe wait requires memory.OperationWaiter")
			}
		}
		for index, fact := range config.Memory.Observe.Facts {
			if field, ok := config.fields[fact.TextFrom]; !ok || field.Type != StateString {
				return fmt.Errorf("eino: Memory Observe Fact[%d] TextFrom must be a string State field", index)
			}
			for attribute, source := range fact.Attributes {
				if strings.TrimSpace(attribute) == "" {
					return fmt.Errorf("eino: Memory Observe Fact[%d] has blank attribute", index)
				}
				if _, ok := config.fields[source]; !ok {
					return fmt.Errorf("eino: Memory Observe Fact[%d] references unknown field %q", index, source)
				}
			}
		}
	}
	return nil
}

func validateBinding(binding Binding, fields map[string]StateType) error {
	source := strings.TrimSpace(binding.From)
	switch source {
	case "input.text", "input.messages", "input.parts", "history.messages", "memory.recalled":
		return nil
	}
	if _, ok := fields[source]; !ok {
		return fmt.Errorf("binding source %q is not declared", source)
	}
	return nil
}

func validEndpoint(endpoint string, nodes map[string]NodeDefinition, from bool) bool {
	if endpoint == "start" {
		return from
	}
	if endpoint == "end" {
		return !from
	}
	_, ok := nodes[endpoint]
	return ok
}

func graphReachable(adjacency map[string][]string, start string) map[string]bool {
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, target := range adjacency[node] {
			if !seen[target] {
				seen[target] = true
				queue = append(queue, target)
			}
		}
	}
	return seen
}

func graphHasCycle(adjacency map[string][]string) bool {
	color := make(map[string]uint8)
	var visit func(string) bool
	visit = func(node string) bool {
		if color[node] == 1 {
			return true
		}
		if color[node] == 2 || node == "end" {
			return false
		}
		color[node] = 1
		if slices.ContainsFunc(adjacency[node], visit) {
			return true
		}
		color[node] = 2
		return false
	}
	return visit("start")
}

func nodeProduces(node NodeDefinition, field string) bool {
	for _, output := range node.Outputs {
		if output == field {
			return true
		}
	}
	return false
}

func validateOutputMIME(stateType StateType, value string) error {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid MIME type %q", value)
	}
	if stateType == StateString && !strings.HasPrefix(mediaType, "text/") {
		return fmt.Errorf("string output requires text MIME type")
	}
	if stateType == StateBlob && strings.HasPrefix(mediaType, "text/") {
		return fmt.Errorf("blob output requires non-text MIME type")
	}
	if stateType != StateString && stateType != StateBlob {
		return fmt.Errorf("only string and blob fields can be published")
	}
	return nil
}

func valueMatchesStateType(value any, stateType StateType) bool {
	switch stateType {
	case StateString:
		_, ok := value.(string)
		return ok
	case StateBoolean:
		_, ok := value.(bool)
		return ok
	case StateInteger:
		switch number := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		case float64:
			return math.Trunc(number) == number
		default:
			return false
		}
	case StateNumber:
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			return true
		default:
			return false
		}
	case StateObject:
		_, ok := value.(map[string]any)
		return ok
	case StateList:
		_, ok := value.([]any)
		return ok
	default:
		return value != nil
	}
}
