package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolkitrun"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type runStateContextKey struct{}

type compiledGraph struct {
	definition GraphDefinition
	runnable   compose.Runnable[map[string]any, map[string]any]
	fields     map[string]StateField
	primary    OutputDefinition
}

type compiledNode struct {
	node NodeDefinition
	run  func(context.Context, *runState) (map[string]any, map[string]bool, error)
}

func buildGraph(ctx context.Context, config *normalizedConfig, definition GraphDefinition, path string) (*compiledGraph, error) {
	fields := make(map[string]StateField, len(definition.State.Fields))
	for _, field := range definition.State.Fields {
		if field.Merge == "" {
			field.Merge = MergeReplace
		}
		fields[field.Name] = field
	}
	outputs := make(map[string][]OutputDefinition)
	var primary OutputDefinition
	for _, output := range definition.Outputs {
		outputs[output.Node] = append(outputs[output.Node], output)
		if output.Primary {
			primary = output
		}
	}
	graph := compose.NewGraph[map[string]any, map[string]any](
		compose.WithGenLocalState(func(ctx context.Context) *runState {
			state, _ := ctx.Value(runStateContextKey{}).(*runState)
			return state
		}),
	)
	for _, node := range definition.Nodes {
		native, err := addNativeComponentNode(ctx, config, node, outputs[node.ID])
		if err != nil {
			return nil, err
		}
		if native != nil {
			if err := graph.AddGraphNode(
				node.ID,
				native,
				compose.WithGraphCompileOptions(compose.WithGraphName(path+"."+node.ID)),
			); err != nil {
				return nil, fmt.Errorf("eino: add native component node %q: %w", node.ID, err)
			}
			continue
		}
		if node.Lambda != nil {
			resolved, err := config.Lambdas.ResolveLambda(ctx, node.Lambda.Lambda)
			if err != nil {
				return nil, fmt.Errorf("eino: resolve Lambda %q: %w", node.Lambda.Lambda, err)
			}
			if err := validateResolvedLambda(node, resolved, fields); err != nil {
				return nil, err
			}
			pre := compose.WithStatePreHandler(func(ctx context.Context, _ map[string]any, state *runState) (map[string]any, error) {
				return state.nodeInputs(node.Inputs)
			})
			post := compose.WithStatePostHandler(func(ctx context.Context, output map[string]any, state *runState) (map[string]any, error) {
				if err := state.writeNodeOutputs(node, output, outputs[node.ID], nil); err != nil {
					return nil, err
				}
				return map[string]any{node.ID: true}, nil
			})
			if err := graph.AddLambdaNode(node.ID, resolved.Lambda, pre, post); err != nil {
				return nil, fmt.Errorf("eino: add Lambda node %q: %w", node.ID, err)
			}
			continue
		}
		compiled, err := compileNode(ctx, config, node, fields, path)
		if err != nil {
			return nil, err
		}
		lambda := compose.InvokableLambda(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			var state *runState
			if err := compose.ProcessState(ctx, func(_ context.Context, current *runState) error {
				state = current
				return nil
			}); err != nil {
				return nil, fmt.Errorf("eino: access Graph state for node %q: %w", node.ID, err)
			}
			if state == nil {
				return nil, fmt.Errorf("eino: Graph state for node %q is nil", node.ID)
			}
			values, streamed, err := compiled.run(ctx, state)
			if err != nil {
				return nil, fmt.Errorf("eino: node %q: %w", node.ID, err)
			}
			if err := state.writeNodeOutputs(node, values, outputs[node.ID], streamed); err != nil {
				return nil, err
			}
			return map[string]any{node.ID: true}, nil
		})
		if err := graph.AddLambdaNode(node.ID, lambda); err != nil {
			return nil, fmt.Errorf("eino: add node %q: %w", node.ID, err)
		}
	}
	for _, edge := range definition.Edges {
		if err := graph.AddEdge(edge.From, edge.To); err != nil {
			return nil, fmt.Errorf("eino: add edge %s -> %s: %w", edge.From, edge.To, err)
		}
	}
	for _, branch := range definition.Branches {
		destinations := map[string]bool{branch.Default: true}
		for _, route := range branch.Routes {
			destinations[route.To] = true
		}
		if branch.Mode == BranchFirstMatch {
			einoBranch := compose.NewGraphBranch(func(ctx context.Context, _ map[string]any) (string, error) {
				state, err := graphState(ctx)
				if err != nil {
					return "", err
				}
				snapshot, err := state.snapshot()
				if err != nil {
					return "", err
				}
				for _, route := range branch.Routes {
					matched, err := evaluatePredicate(route.When, snapshot)
					if err != nil {
						return "", err
					}
					if matched {
						return route.To, nil
					}
				}
				return branch.Default, nil
			}, destinations)
			if err := graph.AddBranch(branch.From, einoBranch); err != nil {
				return nil, fmt.Errorf("eino: add branch from %q: %w", branch.From, err)
			}
		} else {
			einoBranch := compose.NewGraphMultiBranch(func(ctx context.Context, _ map[string]any) (map[string]bool, error) {
				state, err := graphState(ctx)
				if err != nil {
					return nil, err
				}
				snapshot, err := state.snapshot()
				if err != nil {
					return nil, err
				}
				selected := make(map[string]bool)
				for _, route := range branch.Routes {
					matched, err := evaluatePredicate(route.When, snapshot)
					if err != nil {
						return nil, err
					}
					if matched {
						selected[route.To] = true
					}
				}
				if len(selected) == 0 {
					selected[branch.Default] = true
				}
				return selected, nil
			}, destinations)
			if err := graph.AddBranch(branch.From, einoBranch); err != nil {
				return nil, fmt.Errorf("eino: add multi-branch from %q: %w", branch.From, err)
			}
		}
	}
	options := []compose.GraphCompileOption{compose.WithGraphName(definition.Name)}
	if definition.Compile.MaxRunSteps > 0 {
		options = append(options, compose.WithMaxRunSteps(definition.Compile.MaxRunSteps))
	}
	if definition.Compile.NodeTriggerMode == NodeTriggerAllPredecessor {
		options = append(options, compose.WithNodeTriggerMode(compose.AllPredecessor))
	} else {
		options = append(options, compose.WithNodeTriggerMode(compose.AnyPredecessor))
	}
	if len(definition.Compile.FanIn) > 0 {
		fanIn := make(map[string]compose.FanInMergeConfig, len(definition.Compile.FanIn))
		for node, value := range definition.Compile.FanIn {
			fanIn[node] = compose.FanInMergeConfig{StreamMergeWithSourceEOF: value.StreamMergeWithSourceEOF}
		}
		options = append(options, compose.WithFanInMergeConfig(fanIn))
	}
	runnable, err := graph.Compile(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("eino: compile %s: %w", path, err)
	}
	return &compiledGraph{
		definition: definition, runnable: runnable, fields: fields, primary: primary,
	}, nil
}

func addNativeComponentNode(
	ctx context.Context,
	config *normalizedConfig,
	node NodeDefinition,
	published []OutputDefinition,
) (compose.AnyGraph, error) {
	stateOption := compose.WithGenLocalState(func(ctx context.Context) *runState {
		state, _ := ctx.Value(runStateContextKey{}).(*runState)
		return state
	})
	switch {
	case node.Prompt != nil:
		template, err := buildPrompt(*node.Prompt)
		if err != nil {
			return nil, fmt.Errorf("eino: build Prompt node %q: %w", node.ID, err)
		}
		graph := compose.NewGraph[map[string]any, map[string]any](stateOption)
		prepare := compose.InvokableLambda(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			state, err := graphState(ctx)
			if err != nil {
				return nil, err
			}
			return state.nodeInputs(node.Inputs)
		})
		if err := graph.AddLambdaNode("prepare", prepare); err != nil {
			return nil, err
		}
		post := compose.WithStatePostHandler(func(
			_ context.Context,
			messages []*schema.Message,
			state *runState,
		) ([]*schema.Message, error) {
			if err := state.writeNodeOutputs(
				node,
				map[string]any{"messages": messages},
				published,
				nil,
			); err != nil {
				return nil, err
			}
			return messages, nil
		})
		if err := graph.AddChatTemplateNode("component", template, post); err != nil {
			return nil, err
		}
		finish := compose.InvokableLambda(func(_ context.Context, _ []*schema.Message) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		})
		if err := graph.AddLambdaNode("finish", finish); err != nil {
			return nil, err
		}
		if err := addNativeEdges(graph); err != nil {
			return nil, err
		}
		return graph, nil
	case node.ChatModel != nil:
		component, err := config.Components.ResolveChatModel(ctx, node.ChatModel.Model)
		if err != nil {
			return nil, fmt.Errorf("eino: resolve ChatModel %q: %w", node.ChatModel.Model, err)
		}
		if component == nil {
			return nil, fmt.Errorf("eino: ChatModel %q resolved nil", node.ChatModel.Model)
		}
		options := make([]model.Option, 0, 2)
		if node.ChatModel.Temperature != nil {
			options = append(options, model.WithTemperature(*node.ChatModel.Temperature))
		}
		if node.ChatModel.MaxTokens != nil {
			options = append(options, model.WithMaxTokens(*node.ChatModel.MaxTokens))
		}
		graph := compose.NewGraph[map[string]any, map[string]any](stateOption)
		prepare := compose.InvokableLambda(func(ctx context.Context, _ map[string]any) ([]*schema.Message, error) {
			state, err := graphState(ctx)
			if err != nil {
				return nil, err
			}
			inputs, err := state.nodeInputs(node.Inputs)
			if err != nil {
				return nil, err
			}
			messages, ok := inputs["messages"].([]*schema.Message)
			if !ok {
				return nil, fmt.Errorf("eino: ChatModel requires messages input")
			}
			return messages, nil
		})
		if err := graph.AddLambdaNode("prepare", prepare); err != nil {
			return nil, err
		}
		adapted := &streamingChatModel{
			component: component, options: options, node: node, published: published,
			toolkit: config.Toolkit,
		}
		post := compose.WithStatePostHandler(func(
			_ context.Context,
			message *schema.Message,
			state *runState,
		) (*schema.Message, error) {
			outputs := make(map[string]any, len(node.Outputs))
			streamed := make(map[string]bool)
			if _, requested := node.Outputs["text"]; requested {
				if message == nil || message.Content == "" {
					return nil, fmt.Errorf("eino: ChatModel returned no text")
				}
				outputs["text"] = message.Content
				streamed[node.Outputs["text"]] = true
			}
			if _, requested := node.Outputs["messages"]; requested {
				if message == nil {
					return nil, fmt.Errorf("eino: ChatModel returned no message")
				}
				outputs["messages"] = []*schema.Message{message}
			}
			if err := state.writeNodeOutputs(node, outputs, published, streamed); err != nil {
				return nil, err
			}
			return message, nil
		})
		if err := graph.AddChatModelNode("component", adapted, post); err != nil {
			return nil, err
		}
		finish := compose.InvokableLambda(func(_ context.Context, _ *schema.Message) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		})
		if err := graph.AddLambdaNode("finish", finish); err != nil {
			return nil, err
		}
		if err := addNativeEdges(graph); err != nil {
			return nil, err
		}
		return graph, nil
	case node.Retriever != nil:
		component, err := config.Components.ResolveRetriever(ctx, node.Retriever.Retriever)
		if err != nil {
			return nil, fmt.Errorf("eino: resolve Retriever %q: %w", node.Retriever.Retriever, err)
		}
		if component == nil {
			return nil, fmt.Errorf("eino: Retriever %q resolved nil", node.Retriever.Retriever)
		}
		graph := compose.NewGraph[map[string]any, map[string]any](stateOption)
		prepare := compose.InvokableLambda(func(ctx context.Context, _ map[string]any) (string, error) {
			state, err := graphState(ctx)
			if err != nil {
				return "", err
			}
			value, err := state.binding(node.Retriever.Query)
			if err != nil {
				return "", err
			}
			query, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("eino: Retriever query is not text")
			}
			return query, nil
		})
		if err := graph.AddLambdaNode("prepare", prepare); err != nil {
			return nil, err
		}
		post := compose.WithStatePostHandler(func(
			_ context.Context,
			documents []*schema.Document,
			state *runState,
		) ([]*schema.Document, error) {
			if err := state.writeNodeOutputs(
				node,
				map[string]any{"documents": documents},
				published,
				nil,
			); err != nil {
				return nil, err
			}
			return documents, nil
		})
		adapted := &configuredRetriever{component: component, topK: node.Retriever.TopK}
		if err := graph.AddRetrieverNode("component", adapted, post); err != nil {
			return nil, err
		}
		finish := compose.InvokableLambda(func(_ context.Context, _ []*schema.Document) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		})
		if err := graph.AddLambdaNode("finish", finish); err != nil {
			return nil, err
		}
		if err := addNativeEdges(graph); err != nil {
			return nil, err
		}
		return graph, nil
	default:
		return nil, nil
	}
}

func addNativeEdges(graph *compose.Graph[map[string]any, map[string]any]) error {
	for _, edge := range [][2]string{
		{compose.START, "prepare"},
		{"prepare", "component"},
		{"component", "finish"},
		{"finish", compose.END},
	} {
		if err := graph.AddEdge(edge[0], edge[1]); err != nil {
			return err
		}
	}
	return nil
}

type streamingChatModel struct {
	component model.BaseChatModel
	options   []model.Option
	node      NodeDefinition
	published []OutputDefinition
	toolkit   *genx.Toolkit
}

func (chatModel *streamingChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	state, err := graphState(ctx)
	if err != nil {
		return nil, err
	}
	callState := toolkitrun.FromContext(ctx)
	if callState == nil {
		callState = toolkitrun.New(chatModel.toolkit, 0)
	}
	messages := cloneMessages(input)
	var content strings.Builder
	textField := chatModel.node.Outputs["text"]
	for {
		tools, toolErr := einoToolInfos(chatModel.toolkit)
		if toolErr != nil {
			return nil, toolErr
		}
		callOptions := append(slices.Clone(chatModel.options), options...)
		if chatModel.toolkit != nil {
			callOptions = append(callOptions, model.WithTools(tools))
		}
		reader, streamErr := chatModel.component.Stream(ctx, messages, callOptions...)
		if streamErr != nil {
			return nil, streamErr
		}
		chunks, streamErr := chatModel.receiveRound(reader, state, textField, &content)
		reader.Close()
		if streamErr != nil {
			return nil, streamErr
		}
		message, concatErr := schema.ConcatMessages(chunks)
		if concatErr != nil {
			return nil, concatErr
		}
		if message == nil {
			return nil, fmt.Errorf("eino: ChatModel returned no message")
		}
		if len(message.ToolCalls) == 0 {
			message.Content = content.String()
			return message, nil
		}
		if callState == nil {
			return nil, fmt.Errorf("eino: ChatModel returned ToolCalls but Toolkit is not configured")
		}
		messages = append(messages, message)
		for _, call := range message.ToolCalls {
			result, invokeErr := callState.Invoke(ctx, genx.ToolCall{
				ID: call.ID,
				FuncCall: &genx.FuncCall{
					Name: call.Function.Name, Arguments: call.Function.Arguments,
				},
			})
			if invokeErr != nil {
				return nil, invokeErr
			}
			messages = append(messages, &schema.Message{
				Role: schema.Tool, Content: result.Result,
				ToolCallID: result.ID, ToolName: call.Function.Name,
			})
		}
	}
}

func (chatModel *streamingChatModel) receiveRound(
	reader *schema.StreamReader[*schema.Message],
	state *runState,
	textField string,
	content *strings.Builder,
) ([]*schema.Message, error) {
	var chunks []*schema.Message
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			return chunks, nil
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			continue
		}
		chunks = append(chunks, chunk)
		if chunk.Content == "" {
			continue
		}
		content.WriteString(chunk.Content)
		for _, output := range chatModel.published {
			if output.Field == textField {
				if err := state.emit(output, chunk.Content); err != nil {
					return nil, err
				}
			}
		}
	}
}

func (chatModel *streamingChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if chatModel.toolkit == nil {
		return chatModel.component.Stream(
			ctx,
			input,
			append(slices.Clone(chatModel.options), options...)...,
		)
	}
	message, err := chatModel.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

type configuredRetriever struct {
	component retriever.Retriever
	topK      int
}

func (configured *configuredRetriever) Retrieve(
	ctx context.Context,
	query string,
	options ...retriever.Option,
) ([]*schema.Document, error) {
	options = append([]retriever.Option{retriever.WithTopK(configured.topK)}, options...)
	return configured.component.Retrieve(ctx, query, options...)
}

func graphState(ctx context.Context) (*runState, error) {
	var state *runState
	if err := compose.ProcessState(ctx, func(_ context.Context, current *runState) error {
		state = current
		return nil
	}); err != nil {
		return nil, err
	}
	if state == nil {
		return nil, fmt.Errorf("eino: Graph state is nil")
	}
	return state, nil
}

func (graph *compiledGraph) execute(ctx context.Context, state *runState) error {
	if state == nil {
		return fmt.Errorf("eino: run State is required")
	}
	ctx = context.WithValue(ctx, runStateContextKey{}, state)
	_, err := graph.runnable.Invoke(ctx, map[string]any{"start": true})
	if err != nil {
		return err
	}
	return state.required()
}

func validateResolvedLambda(node NodeDefinition, resolved ResolvedLambda, fields map[string]StateField) error {
	if resolved.Lambda == nil {
		return fmt.Errorf("eino: Lambda %q resolved nil", node.Lambda.Lambda)
	}
	if len(resolved.Inputs) == 0 || len(resolved.Outputs) == 0 {
		return fmt.Errorf("eino: Lambda %q requires input and output schemas", node.Lambda.Lambda)
	}
	if len(resolved.Inputs) != len(node.Inputs) || len(resolved.Outputs) != len(node.Outputs) {
		return fmt.Errorf("eino: Lambda %q port schema does not match node %q", node.Lambda.Lambda, node.ID)
	}
	fieldTypes := make(map[string]StateType, len(fields))
	for name, field := range fields {
		fieldTypes[name] = field.Type
	}
	for port, binding := range node.Inputs {
		expected, ok := resolved.Inputs[port]
		if !ok {
			return fmt.Errorf("eino: Lambda %q missing input %q", node.Lambda.Lambda, port)
		}
		if !validStateType(expected) {
			return fmt.Errorf("eino: Lambda %q input %q has unsupported type %q", node.Lambda.Lambda, port, expected)
		}
		actual, err := bindingStateType(binding, fieldTypes)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("eino: Lambda %q input %q requires %s, got %s", node.Lambda.Lambda, port, expected, actual)
		}
	}
	for port, field := range node.Outputs {
		expected, ok := resolved.Outputs[port]
		if !ok {
			return fmt.Errorf("eino: Lambda %q missing output %q", node.Lambda.Lambda, port)
		}
		if !validStateType(expected) {
			return fmt.Errorf("eino: Lambda %q output %q has unsupported type %q", node.Lambda.Lambda, port, expected)
		}
		if actual := fields[field].Type; actual != expected {
			return fmt.Errorf("eino: Lambda %q output %q requires %s, got %s", node.Lambda.Lambda, port, expected, actual)
		}
	}
	return nil
}

func compileNode(
	ctx context.Context,
	config *normalizedConfig,
	node NodeDefinition,
	fields map[string]StateField,
	path string,
) (*compiledNode, error) {
	result := &compiledNode{node: node}
	switch {
	case node.Transform != nil:
		result.run = func(_ context.Context, state *runState) (map[string]any, map[string]bool, error) {
			inputs, err := state.nodeInputs(node.Inputs)
			if err != nil {
				return nil, nil, err
			}
			outputs, err := runTransform(*node.Transform, inputs)
			return outputs, nil, err
		}
	case node.Script != nil:
		script, err := compileScript(ctx, *node.Script)
		if err != nil {
			return nil, err
		}
		outputTypes := make(map[string]StateType, len(node.Outputs))
		for port, field := range node.Outputs {
			outputTypes[port] = fields[field].Type
		}
		result.run = func(ctx context.Context, state *runState) (map[string]any, map[string]bool, error) {
			inputs, err := state.nodeInputs(node.Inputs)
			if err != nil {
				return nil, nil, err
			}
			outputs, err := script.run(ctx, inputs, outputTypes)
			return outputs, nil, err
		}
	case node.Passthrough != nil:
		result.run = func(_ context.Context, state *runState) (map[string]any, map[string]bool, error) {
			inputs, err := state.nodeInputs(node.Inputs)
			if err != nil {
				return nil, nil, err
			}
			for port, value := range inputs {
				return map[string]any{port: value}, nil, nil
			}
			return nil, nil, fmt.Errorf("Passthrough has no input")
		}
	case node.Subgraph != nil:
		child, err := buildGraph(ctx, config, node.Subgraph.Graph, path+"."+node.ID)
		if err != nil {
			return nil, err
		}
		result.run = childRunner(node, child)
	case node.Race != nil:
		runner, err := compileRace(ctx, config, node, path)
		if err != nil {
			return nil, err
		}
		result.run = runner
	case node.Batch != nil:
		runner, err := compileBatch(ctx, config, node, path)
		if err != nil {
			return nil, err
		}
		result.run = runner
	default:
		return nil, fmt.Errorf("eino: unsupported node %q", node.ID)
	}
	return result, nil
}

func buildPrompt(config PromptNode) (*prompt.DefaultChatTemplate, error) {
	var format schema.FormatType
	switch config.Format {
	case PromptFString:
		format = schema.FString
	case PromptGoTemplate:
		format = schema.GoTemplate
	case PromptJinja2:
		format = schema.Jinja2
	default:
		return nil, fmt.Errorf("unsupported format %q", config.Format)
	}
	templates := make([]schema.MessagesTemplate, 0, len(config.Messages))
	for _, message := range config.Messages {
		if message.Placeholder != "" {
			if message.Role != "" || message.Template != "" {
				return nil, fmt.Errorf("placeholder cannot also define Role or Template")
			}
			templates = append(templates, schema.MessagesPlaceholder(message.Placeholder, message.Optional))
			continue
		}
		template, err := messageForRole(message.Role, message.Template)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return prompt.FromMessages(format, templates...), nil
}

func childRunner(node NodeDefinition, child *compiledGraph) func(context.Context, *runState) (map[string]any, map[string]bool, error) {
	return func(ctx context.Context, parent *runState) (map[string]any, map[string]bool, error) {
		inputs, err := parent.nodeInputs(node.Inputs)
		if err != nil {
			return nil, nil, err
		}
		childState, err := newRunState(child.fields, graphInputFromNodeInputs(inputs), inputs, &captureEmitter{})
		if err != nil {
			return nil, nil, err
		}
		capture := &captureEmitter{values: make(map[string]any)}
		childState.emitter = capture
		if err := child.execute(ctx, childState); err != nil {
			return nil, nil, err
		}
		return maps.Clone(capture.values), nil, nil
	}
}

func graphInputFromNodeInputs(inputs map[string]any) graphInput {
	result := graphInput{}
	if text, ok := inputs["text"].(string); ok {
		result.Text = text
	}
	if messages, ok := inputs["messages"].([]*schema.Message); ok {
		result.Messages = messages
	}
	if parts, ok := inputs["parts"].([]any); ok {
		result.Parts = parts
	}
	return result
}

type captureEmitter struct {
	values map[string]any
}

func (emitter *captureEmitter) Emit(output OutputDefinition, value any) error {
	if emitter.values == nil {
		emitter.values = make(map[string]any)
	}
	current, exists := emitter.values[output.Name]
	if text, ok := value.(string); ok && exists {
		emitter.values[output.Name] = current.(string) + text
	} else {
		emitter.values[output.Name] = value
	}
	return nil
}
