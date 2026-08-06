package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	flowcraftagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const Type = "eino"

// Factory maps the strict product Workflow into the existing Eino Transformer.
type Factory struct {
	GenX         *peergenx.Service
	GenXForOwner func(context.Context, string) (*peergenx.Service, error)
	History      logstore.MutableStore
	MemoryStores *memorystore.Registry
	ServerRoot   string
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	public := spec.Workflow.Spec.Eino
	if public == nil {
		return nil, fmt.Errorf("eino: workflow spec.eino is required")
	}
	service, err := f.serviceForWorkspace(ctx, spec)
	if err != nil {
		return nil, err
	}
	graph, err := einoconfig.MapGraph(public.Graph)
	if err != nil {
		return nil, fmt.Errorf("eino: workflow graph: %w", err)
	}
	owner := ""
	if spec.Workspace.OwnerPublicKey != nil {
		owner = strings.TrimSpace(*spec.Workspace.OwnerPublicKey)
	}
	workspaceID := spec.Workspace.Id
	if workspaceID == "" {
		return nil, fmt.Errorf("eino: workspace id is required")
	}
	scope := flowcraftagent.WorkspaceAgentScope(owner, workspaceID, workspaceID)
	config := genxeino.Config{
		Agent: genxeino.AgentConfig{
			ID:        workspaceID,
			Name:      spec.Workflow.Id,
			ContextID: scope,
		},
		Graph:       graph,
		Components:  componentResolver{service: service},
		ToolInvoker: spec.ToolInvoker,
		History: &genxeino.HistoryConfig{
			Store: f.History, Scope: scope, Limit: 50,
		},
	}
	config.Initiative = mapInitiative(public.Conversation, spec.Workspace.Parameters)
	if public.Limits != nil && public.Limits.MaxOutputBytes != nil {
		config.Limits.MaxOutputBytes = *public.Limits.MaxOutputBytes
	}
	store := spec.Memory
	backend := strings.TrimSpace(spec.MemoryKind)
	memoryCloser := spec.MemoryCloser
	if spec.MemoryBinding != nil || spec.MemoryLayout != nil {
		if spec.MemoryBinding == nil || spec.MemoryLayout == nil {
			return nil, fmt.Errorf("eino: incomplete runtime memory binding")
		}
		request := memorystore.Request{
			WorkspaceID:     workspaceID,
			ProfileID:       spec.MemoryProfileID,
			ProfileRevision: spec.MemoryProfileRevision,
			BindingName:     spec.MemoryName,
			Layout:          *spec.MemoryLayout,
			Binding:         *spec.MemoryBinding,
			ModelLoader:     flowcraftagent.NewRuntimeMemoryLoader(service),
			ServerRoot:      f.ServerRoot,
		}
		var result memorystore.Result
		var err error
		if f.MemoryStores != nil {
			result, err = f.MemoryStores.Resolve(ctx, request)
		} else {
			result, err = memorystore.Build(ctx, request)
		}
		if err != nil {
			return nil, fmt.Errorf("eino: construct workspace memory: %w", err)
		}
		store = result.Store
		backend = result.Driver
		memoryCloser = result.Closer
	}
	if store != nil {
		bound, err := memory.BindApp(store, workspaceID)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("eino: bind workspace memory: %w", err), closeMemory(memoryCloser))
		}
		config.Memory = &genxeino.MemoryConfig{
			Store: bound,
			Scope: memory.Scope{AppID: workspaceID},
		}
	}
	transformer, err := genxeino.New(ctx, config)
	if err != nil {
		return nil, errors.Join(err, closeMemory(memoryCloser))
	}
	agent := agenthost.NewTransformerAgent(transformer)
	if config.Memory != nil {
		agent = agenthost.NewMemoryAgent(agent, config.Memory.Store, config.Memory.Scope, backend)
	}
	if memoryCloser != nil {
		agent = &managedAgent{Agent: agent, closer: orderedClosers{transformer, memoryCloser}}
	} else {
		agent = &managedAgent{Agent: agent, closer: transformer}
	}
	return agent, nil
}

func closeMemory(closer io.Closer) error {
	if closer == nil {
		return nil
	}
	return closer.Close()
}

func mapInitiative(conversation *apitypes.EinoConversation, parameters *apitypes.WorkspaceParameters) genxeino.InitiativePolicy {
	starts := apitypes.EinoConversationStartsPeer
	if conversation != nil && conversation.Starts != nil {
		starts = *conversation.Starts
	}
	policy := apitypes.FlowcraftConversationParametersAgentInitiativePolicyOnReload
	if parameters != nil {
		if value, err := parameters.AsEinoWorkspaceParameters(); err == nil && value.Conversation != nil {
			if value.Conversation.Initiative != nil {
				starts = apitypes.EinoConversationStarts(*value.Conversation.Initiative)
			}
			if value.Conversation.AgentInitiativePolicy != nil {
				policy = *value.Conversation.AgentInitiativePolicy
			}
		}
	}
	if starts != apitypes.EinoConversationStartsAgent {
		return genxeino.InitiativeDisabled
	}
	if policy == apitypes.FlowcraftConversationParametersAgentInitiativePolicyOnceWhenEmpty {
		return genxeino.InitiativeOnceWhenEmpty
	}
	return genxeino.InitiativeOnReload
}

type managedAgent struct {
	agenthost.Agent
	closer    io.Closer
	closeOnce sync.Once
	closeErr  error
}

type orderedClosers []io.Closer

func (closers orderedClosers) Close() error {
	var result error
	for _, closer := range closers {
		if closer != nil {
			result = errors.Join(result, closer.Close())
		}
	}
	return result
}

func (a *managedAgent) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		if a.closer != nil {
			a.closeErr = a.closer.Close()
		}
	})
	return a.closeErr
}

func (f Factory) serviceForWorkspace(ctx context.Context, spec agenthost.Spec) (*peergenx.Service, error) {
	if spec.Workspace.OwnerPublicKey == nil || strings.TrimSpace(*spec.Workspace.OwnerPublicKey) == "" {
		if f.GenX == nil {
			return nil, fmt.Errorf("eino: GenX service is required")
		}
		return f.GenX, nil
	}
	if f.GenXForOwner == nil {
		return nil, fmt.Errorf("eino: workspace %q owner GenX resolver is required", spec.Workspace.Name)
	}
	service, err := f.GenXForOwner(ctx, strings.TrimSpace(*spec.Workspace.OwnerPublicKey))
	if err != nil {
		return nil, fmt.Errorf("eino: workspace %q owner runtime: %w", spec.Workspace.Name, err)
	}
	if service == nil {
		return nil, fmt.Errorf("eino: workspace %q owner runtime returned no service", spec.Workspace.Name)
	}
	return service, nil
}

type componentResolver struct {
	service *peergenx.Service
}

func (r componentResolver) ResolveChatModel(ctx context.Context, alias string) (model.BaseChatModel, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" || strings.Contains(alias, "/") {
		return nil, fmt.Errorf("eino: invalid model alias %q", alias)
	}
	if r.service == nil {
		return nil, fmt.Errorf("eino: GenX service is required")
	}
	if _, err := r.service.ResolveGenerator(ctx, "model/"+alias); err != nil {
		return nil, err
	}
	return genXChatModel{generator: r.service.Generator(), pattern: "model/" + alias}, nil
}

func (componentResolver) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return nil, fmt.Errorf("eino: retriever nodes are not exposed")
}

type genXChatModel struct {
	generator genx.Generator
	pattern   string
}

func (m genXChatModel) Generate(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.Message, error) {
	reader, err := m.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	return schema.ConcatMessages(chunks)
}

func (m genXChatModel) Stream(ctx context.Context, input []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m.generator == nil {
		return nil, fmt.Errorf("eino: GenX generator is required")
	}
	modelContext, err := genXModelContext(input, options...)
	if err != nil {
		return nil, err
	}
	stream, err := m.generator.GenerateStream(ctx, m.pattern, modelContext)
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](8)
	go func() {
		defer writer.Close()
		defer stream.Close()
		toolIndex := 0
		for {
			chunk, nextErr := stream.Next()
			if nextErr != nil {
				if !errors.Is(nextErr, io.EOF) {
					writer.Send(nil, nextErr)
				}
				return
			}
			if chunk == nil {
				continue
			}
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				writer.Send(nil, fmt.Errorf("eino: model stream: %s", chunk.Ctrl.Error))
				return
			}
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				if writer.Send(schema.AssistantMessage(string(text), nil), nil) {
					return
				}
			}
			if chunk.ToolCall != nil {
				call, err := einoToolCall(chunk.ToolCall, toolIndex)
				if err != nil {
					writer.Send(nil, err)
					return
				}
				toolIndex++
				if writer.Send(&schema.Message{
					Role: schema.Assistant, ToolCalls: []schema.ToolCall{call},
				}, nil) {
					return
				}
			}
		}
	}()
	return reader, nil
}

func genXModelContext(input []*schema.Message, options ...model.Option) (genx.ModelContext, error) {
	common := model.GetCommonOptions(nil, options...)
	if len(common.DeferredTools) != 0 || common.ToolChoice != nil ||
		common.ToolSearchTool != nil || common.AgenticToolChoice != nil {
		return nil, fmt.Errorf("eino: unsupported model Tool option")
	}
	builder := &genx.ModelContextBuilder{Params: &genx.ModelParams{}}
	if common.MaxTokens != nil {
		builder.Params.MaxTokens = *common.MaxTokens
	}
	if common.Temperature != nil {
		builder.Params.Temperature = *common.Temperature
	}
	if common.TopP != nil {
		builder.Params.TopP = *common.TopP
	}
	for index, info := range common.Tools {
		tool, err := genXFuncTool(info)
		if err != nil {
			return nil, fmt.Errorf("eino: model Tool %d: %w", index, err)
		}
		builder.AddTool(tool)
	}
	for _, message := range input {
		if message == nil {
			continue
		}
		switch message.Role {
		case schema.System:
			if len(message.ToolCalls) != 0 || message.ToolCallID != "" {
				return nil, fmt.Errorf("eino: system message contains Tool state")
			}
			builder.PromptText(message.Name, message.Content)
		case schema.User:
			if len(message.ToolCalls) != 0 || message.ToolCallID != "" {
				return nil, fmt.Errorf("eino: user message contains Tool state")
			}
			builder.UserText(message.Name, message.Content)
		case schema.Assistant:
			if message.ToolCallID != "" {
				return nil, fmt.Errorf("eino: assistant message contains Tool result ID")
			}
			if message.Content != "" {
				builder.ModelText(message.Name, message.Content)
			}
			for _, call := range message.ToolCalls {
				if err := appendGenXToolCall(builder, message.Name, call); err != nil {
					return nil, err
				}
			}
		case schema.Tool:
			if len(message.ToolCalls) != 0 {
				return nil, fmt.Errorf("eino: Tool result message contains Tool calls")
			}
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, fmt.Errorf("eino: Tool result message has no call ID")
			}
			builder.Messages = append(builder.Messages, &genx.Message{
				Role: genx.RoleTool, Name: message.ToolName,
				Payload: &genx.ToolResult{ID: message.ToolCallID, Result: message.Content},
			})
		default:
			return nil, fmt.Errorf("eino: unsupported model message role %q", message.Role)
		}
	}
	return builder.Build(), nil
}

func genXFuncTool(info *schema.ToolInfo) (*genx.FuncTool, error) {
	if info == nil {
		return nil, fmt.Errorf("definition is nil")
	}
	name := strings.TrimSpace(info.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	var argument jsonschema.Schema
	if info.ParamsOneOf == nil {
		argument.Type = "object"
	} else {
		source, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil {
			return nil, fmt.Errorf("Tool %q schema: %w", name, err)
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			return nil, fmt.Errorf("encode Tool %q schema: %w", name, err)
		}
		if err := json.Unmarshal(encoded, &argument); err != nil {
			return nil, fmt.Errorf("convert Tool %q schema: %w", name, err)
		}
	}
	if _, err := argument.Resolve(nil); err != nil {
		return nil, fmt.Errorf("resolve Tool %q schema: %w", name, err)
	}
	return &genx.FuncTool{
		Name: name, Description: strings.TrimSpace(info.Desc), Argument: &argument,
	}, nil
}

func appendGenXToolCall(builder *genx.ModelContextBuilder, name string, call schema.ToolCall) error {
	if builder == nil {
		return fmt.Errorf("eino: model context builder is nil")
	}
	id := strings.TrimSpace(call.ID)
	toolName := strings.TrimSpace(call.Function.Name)
	if id == "" || toolName == "" {
		return fmt.Errorf("eino: Tool call ID and name are required")
	}
	arguments := strings.TrimSpace(call.Function.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	if !json.Valid([]byte(arguments)) {
		return fmt.Errorf("eino: Tool call %q arguments are invalid JSON", toolName)
	}
	builder.Messages = append(builder.Messages, &genx.Message{
		Role: genx.RoleModel, Name: name,
		Payload: &genx.ToolCall{ID: id, FuncCall: &genx.FuncCall{
			Name: toolName, Arguments: arguments,
		}},
	})
	return nil
}

func einoToolCall(call *genx.ToolCall, index int) (schema.ToolCall, error) {
	if call == nil || call.FuncCall == nil {
		return schema.ToolCall{}, fmt.Errorf("eino: model returned an incomplete Tool call")
	}
	id := strings.TrimSpace(call.ID)
	name := strings.TrimSpace(call.FuncCall.Name)
	if id == "" || name == "" {
		return schema.ToolCall{}, fmt.Errorf("eino: model returned a Tool call without ID or name")
	}
	arguments := strings.TrimSpace(call.FuncCall.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	if !json.Valid([]byte(arguments)) {
		return schema.ToolCall{}, fmt.Errorf("eino: model returned invalid JSON arguments for Tool %q", name)
	}
	return schema.ToolCall{
		Index: &index, ID: id, Type: "function",
		Function: schema.FunctionCall{Name: name, Arguments: arguments},
	}, nil
}
