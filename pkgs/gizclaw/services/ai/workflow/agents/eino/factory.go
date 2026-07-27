package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

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
	scope := flowcraftagent.WorkspaceAgentScope(owner, spec.Workspace.Name, spec.Workspace.Name)
	config := genxeino.Config{
		Agent: genxeino.AgentConfig{
			ID:        strings.TrimSpace(spec.Workspace.Name),
			Name:      strings.TrimSpace(spec.Workflow.Name),
			ContextID: scope,
		},
		Graph:      graph,
		Components: componentResolver{service: service},
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
			WorkspaceName:   strings.TrimSpace(spec.Workspace.Name),
			ProfileName:     spec.MemoryProfileName,
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
		bound, err := memory.BindApp(store, spec.Workspace.Name)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("eino: bind workspace memory: %w", err), closeMemory(memoryCloser))
		}
		config.Memory = &genxeino.MemoryConfig{
			Store: bound,
			Scope: memory.Scope{AppID: strings.TrimSpace(spec.Workspace.Name)},
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
			text, ok := chunk.Part.(genx.Text)
			if !ok || text == "" {
				continue
			}
			if writer.Send(schema.AssistantMessage(string(text), nil), nil) {
				return
			}
		}
	}()
	return reader, nil
}

func genXModelContext(input []*schema.Message, options ...model.Option) (genx.ModelContext, error) {
	common := model.GetCommonOptions(nil, options...)
	if len(common.Tools) != 0 || len(common.DeferredTools) != 0 || common.ToolChoice != nil ||
		common.ToolSearchTool != nil || common.AgenticToolChoice != nil {
		return nil, fmt.Errorf("eino: ToolCall is outside this Workflow driver")
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
	for _, message := range input {
		if message == nil {
			continue
		}
		if len(message.ToolCalls) != 0 || message.ToolCallID != "" {
			return nil, fmt.Errorf("eino: ToolCall is outside this Workflow driver")
		}
		switch message.Role {
		case schema.System:
			builder.PromptText(message.Name, message.Content)
		case schema.User:
			builder.UserText(message.Name, message.Content)
		case schema.Assistant:
			builder.ModelText(message.Name, message.Content)
		default:
			return nil, fmt.Errorf("eino: unsupported model message role %q", message.Role)
		}
	}
	return builder.Build(), nil
}
