package flowcraft

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	flowembedding "github.com/GizClaw/flowcraft/sdk/embedding"
	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	embeddingbytedance "github.com/GizClaw/flowcraft/sdkx/embedding/bytedance"
	embeddingopenai "github.com/GizClaw/flowcraft/sdkx/embedding/openai"
	embeddingqwen "github.com/GizClaw/flowcraft/sdkx/embedding/qwen"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	genxflowcraft "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
	"github.com/openai/openai-go/option"
)

const Type = "flowcraft"

// Factory maps the public GizClaw Workflow plus AgentHost-owned dependencies
// into the reusable GenX Flowcraft Transformer and Audio Dock.
type Factory struct {
	GenX             *peergenx.Service
	GenXForOwner     func(context.Context, string) (*peergenx.Service, error)
	History          logstore.MutableStore
	State            kv.Store
	Memory           memory.Store
	MemoryKind       string
	MemoryLaneRecall map[string]string
	MemoryStores     *memorystore.Registry
	ServerRoot       string
}

// InputProvider supplies product-owned transient Board values.
type InputProvider func(context.Context) (map[string]any, error)

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if spec.Workflow.Spec.Flowcraft == nil {
		return nil, fmt.Errorf("flowcraft: workflow spec.flowcraft is required")
	}
	workspaceName := strings.TrimSpace(spec.Workspace.Name)
	if workspaceName == "" {
		return nil, fmt.Errorf("flowcraft: workspace name is required")
	}
	workspaceID := strings.TrimSpace(spec.Workspace.Id)
	if workspaceID == "" {
		return nil, fmt.Errorf("flowcraft: workspace id is required")
	}
	public := *spec.Workflow.Spec.Flowcraft
	if spec.Memory != nil {
		f.Memory = spec.Memory
		f.MemoryKind = spec.MemoryKind
	}
	owner := stringValue(spec.Workspace.OwnerPublicKey)
	initiativePolicy := ""
	inputMode := apitypes.WorkspaceInputModePushToTalk
	if owner != "" {
		if f.GenXForOwner == nil {
			return nil, fmt.Errorf("flowcraft: workspace %q owner GenX resolver is required", workspaceName)
		}
		ownerGenX, err := f.GenXForOwner(ctx, owner)
		if err != nil {
			return nil, fmt.Errorf("flowcraft: workspace %q owner runtime: %w", workspaceName, err)
		}
		if ownerGenX == nil {
			return nil, fmt.Errorf("flowcraft: workspace %q owner runtime returned no GenX service", workspaceName)
		}
		f.GenX = ownerGenX
	}
	if spec.Workspace.Parameters != nil {
		parameters, err := spec.Workspace.Parameters.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode workspace parameters: %w", err)
		}
		if parameters.Conversation != nil {
			initiativePolicy = stringValue((*string)(parameters.Conversation.AgentInitiativePolicy))
			if parameters.Conversation.Initiative != nil {
				starts := apitypes.FlowcraftConversationStarts(*parameters.Conversation.Initiative)
				public.Conversation = &apitypes.FlowcraftConversation{Starts: &starts}
			}
		}
		inputMode, err = resolveFlowcraftInputMode(parameters.Input)
		if err != nil {
			return nil, err
		}
	}
	memoryCloser := spec.MemoryCloser
	if spec.MemoryBinding != nil || spec.MemoryLayout != nil {
		if spec.MemoryBinding == nil || spec.MemoryLayout == nil {
			return nil, fmt.Errorf("flowcraft: incomplete runtime memory binding")
		}
		request := memorystore.Request{
			WorkspaceID:     workspaceID,
			ProfileID:       spec.MemoryProfileID,
			ProfileRevision: spec.MemoryProfileRevision,
			BindingName:     spec.MemoryName,
			Layout:          *spec.MemoryLayout,
			Binding:         *spec.MemoryBinding,
			ModelLoader:     NewRuntimeMemoryLoader(f.GenX),
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
			return nil, fmt.Errorf("flowcraft: construct workspace memory: %w", err)
		}
		f.Memory = result.Store
		f.MemoryKind = result.Driver
		memoryCloser = joinClosers(memoryCloser, result.Closer)
	}
	if f.Memory != nil && f.MemoryKind == string(apitypes.RuntimeProfileMemoryDriverFlowcraft) && spec.MemoryLayout != nil {
		f.MemoryLaneRecall = flowcraftLaneRecall(spec.MemoryLayout.Spec.Flowcraft.Lanes)
	}
	return f.newAgent(ctx, owner, workspaceID, spec.Workflow.Id, public, spec.ToolInvoker, spec.BoardInputs, initiativePolicy, inputMode, memoryCloser)
}

func (f Factory) newAgent(ctx context.Context, owner, workspaceID, workflowName string, public apitypes.FlowcraftWorkflowSpec, toolInvoker genx.ToolInvoker, inputs InputProvider, initiativePolicy string, inputMode apitypes.WorkspaceInputMode, memoryCloser io.Closer) (agenthost.Agent, error) {
	if f.GenX == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	if err := public.Validate(); err != nil {
		return nil, fmt.Errorf("flowcraft: invalid workflow config: %w", err)
	}
	graph, publishNodes, err := mapGraph(public.Graph)
	if err != nil {
		return nil, err
	}
	for _, node := range graph.Nodes {
		if node.Type != "llm" {
			continue
		}
		alias, _ := node.Config["model"].(string)
		if _, err := f.GenX.ResolveGenerator(ctx, modelPattern(alias)); err != nil {
			return nil, fmt.Errorf("flowcraft: resolve model alias %q for node %q: %w", alias, node.ID, err)
		}
	}
	agentID := workspaceID
	scope := WorkspaceAgentScope(owner, workspaceID, agentID)
	memoryScope := memory.Scope{AppID: workspaceID}
	config := genxflowcraft.Config{
		ID: agentID, Name: strings.TrimSpace(workflowName), Graph: graph,
		MaxIterations: intValue(public.MaxIterations), PublishNodes: publishNodes,
		Models: f.GenX.Generator(), History: f.History, HistoryScope: scope, ContextID: scope,
		BoardInputs: genxflowcraftBoardInputs(inputs), ToolInvoker: toolInvoker,
	}
	config.Initiative = mapInitiative(public.Conversation, initiativePolicy)
	if f.State != nil {
		config.State = flowcraftStateStore(f.State, scope)
	}

	var owned []io.Closer
	if memoryCloser != nil {
		owned = append(owned, memoryCloser)
	}
	var agentMemory memory.Store
	if f.Memory != nil {
		config.Memory = f.Memory
		agentMemory = f.Memory
		config.MemoryScope = memoryScope
		config.MemoryLaneRecall = maps.Clone(f.MemoryLaneRecall)
	}

	core, err := genxflowcraft.New(config)
	if err != nil {
		return nil, errors.Join(err, closeAll(owned))
	}
	owned = append(owned, core)
	var transformer genx.Transformer = core
	if public.VoiceAdapter != nil {
		transformer, err = f.wrapAudio(core, *public.VoiceAdapter, inputMode)
		if err != nil {
			return nil, errors.Join(err, closeAll(owned))
		}
	}
	backend := "flowcraft"
	if f.Memory != nil {
		backend = strings.TrimSpace(f.MemoryKind)
	}
	return NewManagedAgentWithBackend(transformer, owned, agentMemory, memoryScope, backend), nil
}

func flowcraftLaneRecall(lanes []apitypes.FlowcraftMemoryLanePolicy) map[string]string {
	result := make(map[string]string)
	for _, lane := range lanes {
		name := strings.TrimSpace(lane.Name)
		recall := stringValue(lane.Recall)
		if name != "" && recall != "" {
			result[name] = recall
		}
	}
	return result
}

func mapInitiative(conversation *apitypes.FlowcraftConversation, policy string) genxflowcraft.InitiativePolicy {
	if conversation == nil || conversation.Starts == nil || *conversation.Starts != apitypes.FlowcraftConversationStartsAgent {
		return genxflowcraft.InitiativeDisabled
	}
	if policy == string(apitypes.FlowcraftConversationParametersAgentInitiativePolicyOnceWhenEmpty) {
		return genxflowcraft.InitiativeOnceWhenEmpty
	}
	return genxflowcraft.InitiativeOnReload
}

func mapGraph(source apitypes.FlowcraftGraph) (flowgraph.GraphDefinition, []string, error) {
	graph := flowgraph.GraphDefinition{Name: strings.TrimSpace(source.Name), Entry: strings.TrimSpace(source.Entry)}
	graph.Edges = make([]flowgraph.EdgeDefinition, 0, len(valueOrZero(source.Edges)))
	for _, edge := range valueOrZero(source.Edges) {
		graph.Edges = append(graph.Edges, flowgraph.EdgeDefinition{From: edge.From, To: edge.To, Condition: stringValue(edge.Condition)})
	}
	publish := make([]string, 0)
	for index, raw := range source.Nodes {
		discriminator, err := raw.Discriminator()
		if err != nil {
			return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: graph.nodes[%d].type: %w", index, err)
		}
		var node flowgraph.NodeDefinition
		switch discriminator {
		case "llm":
			typed, err := raw.AsFlowcraftLLMNode()
			if err != nil {
				return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: decode LLM node %d: %w", index, err)
			}
			node = flowgraph.NodeDefinition{ID: typed.Id, Type: "llm", SkipCondition: stringValue(typed.SkipCondition), Config: llmNodeConfig(typed.Config)}
			if boolValue(typed.Publish) {
				publish = append(publish, typed.Id)
			}
		case "script":
			typed, err := raw.AsFlowcraftScriptNode()
			if err != nil {
				return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: decode script node %d: %w", index, err)
			}
			node = flowgraph.NodeDefinition{ID: typed.Id, Type: "script", SkipCondition: stringValue(typed.SkipCondition), Config: map[string]any{"source": typed.Config.Source}}
			if boolValue(typed.Publish) {
				publish = append(publish, typed.Id)
			}
		case "passthrough":
			typed, err := raw.AsFlowcraftPassthroughNode()
			if err != nil {
				return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: decode passthrough node %d: %w", index, err)
			}
			node = flowgraph.NodeDefinition{ID: typed.Id, Type: "passthrough", SkipCondition: stringValue(typed.SkipCondition)}
			if boolValue(typed.Publish) {
				publish = append(publish, typed.Id)
			}
		case "memory_recall":
			typed, err := raw.AsFlowcraftMemoryRecallNode()
			if err != nil {
				return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: decode memory recall node %d: %w", index, err)
			}
			node = flowgraph.NodeDefinition{ID: typed.Id, Type: "memory_recall", SkipCondition: stringValue(typed.SkipCondition), Config: memoryRecallNodeConfig(typed.Config)}
			if boolValue(typed.Publish) {
				publish = append(publish, typed.Id)
			}
		case "memory_observe":
			typed, err := raw.AsFlowcraftMemoryObserveNode()
			if err != nil {
				return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: decode memory observe node %d: %w", index, err)
			}
			node = flowgraph.NodeDefinition{ID: typed.Id, Type: "memory_observe", SkipCondition: stringValue(typed.SkipCondition), Config: memoryObserveNodeConfig(typed.Config)}
			if boolValue(typed.Publish) {
				publish = append(publish, typed.Id)
			}
		default:
			return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: unsupported graph.nodes[%d].type %q", index, discriminator)
		}
		graph.Nodes = append(graph.Nodes, node)
	}
	if len(publish) == 0 {
		return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: graph requires at least one publish node")
	}
	return graph, publish, nil
}

func memoryRecallNodeConfig(source apitypes.FlowcraftMemoryRecallNodeConfig) map[string]any {
	query := map[string]any{"text_from": source.Query.TextFrom}
	if source.Query.Kinds != nil {
		kinds := make([]string, 0, len(*source.Query.Kinds))
		for _, kind := range *source.Query.Kinds {
			kinds = append(kinds, string(kind))
		}
		query["kinds"] = kinds
	}
	if source.Query.Lanes != nil {
		query["lanes"] = slices.Clone(*source.Query.Lanes)
	}
	if source.Query.Filters != nil {
		query["filters"] = slices.Clone(*source.Query.Filters)
	}
	result := map[string]any{
		"query":  query,
		"output": source.Output,
		"top_k":  source.TopK,
	}
	if source.Render != nil {
		render := map[string]any{}
		setString(render, "header", source.Render.Header)
		setString(render, "item_prefix", source.Render.ItemPrefix)
		setValue(render, "max_items", source.Render.MaxItems)
		result["render"] = render
	}
	return result
}

func memoryObserveNodeConfig(source apitypes.FlowcraftMemoryObserveNodeConfig) map[string]any {
	observations := make([]map[string]any, 0, len(source.Observations))
	for _, observation := range source.Observations {
		item := map[string]any{}
		setString(item, "turns_from", observation.TurnsFrom)
		setString(item, "text_from", observation.TextFrom)
		if observation.Facts != nil {
			item["facts"] = slices.Clone(*observation.Facts)
		}
		observations = append(observations, item)
	}
	result := map[string]any{"observations": observations}
	setValue(result, "wait_for_completion", source.WaitForCompletion)
	return result
}

func llmNodeConfig(source apitypes.FlowcraftLLMNodeConfig) map[string]any {
	result := map[string]any{"model": strings.TrimSpace(source.Model)}
	setString(result, "system_prompt", source.SystemPrompt)
	setString(result, "output_key", source.OutputKey)
	setString(result, "messages_channel", source.MessagesChannel)
	setValue(result, "temperature", source.Temperature)
	setValue(result, "max_tokens", source.MaxTokens)
	setValue(result, "json_mode", source.JsonMode)
	setValue(result, "thinking", source.Thinking)
	setValue(result, "track_steps", source.TrackSteps)
	return result
}

func (f Factory) wrapAudio(core genx.Transformer, voice apitypes.FlowcraftVoiceAdapter, inputMode apitypes.WorkspaceInputMode) (genx.Transformer, error) {
	config := audiodock.Config{Agent: core}
	if alias := stringValue(voice.AsrModel); alias != "" {
		config.ASR = patternTransformer{mux: f.GenX.Transformer(), pattern: flowcraftASRPattern(alias, inputMode)}
	}
	defaultVoice := stringValue(voice.DefaultVoice)
	nodeVoices := maps.Clone(valueOrZero(voice.NodeVoices))
	if defaultVoice != "" || len(nodeVoices) != 0 {
		config.TTS = f.GenX.Transformer()
		config.ResolveVoice = func(_ context.Context, request audiodock.VoiceRequest) (string, error) {
			alias := strings.TrimSpace(nodeVoices[request.Name])
			if alias == "" {
				alias = defaultVoice
			}
			if alias == "" {
				return "", nil
			}
			return voicePattern(alias), nil
		}
	}
	return audiodock.New(config)
}

func resolveFlowcraftInputMode(input *apitypes.WorkspaceInputMode) (apitypes.WorkspaceInputMode, error) {
	if input == nil {
		return apitypes.WorkspaceInputModePushToTalk, nil
	}
	if !input.Valid() {
		return "", fmt.Errorf("flowcraft: unsupported workspace input %q", *input)
	}
	return *input, nil
}

func flowcraftASRPattern(alias string, inputMode apitypes.WorkspaceInputMode) string {
	pattern := modelPattern(alias)
	if inputMode != apitypes.WorkspaceInputModeRealtime {
		return pattern
	}
	separator := "?"
	if strings.Contains(pattern, "?") {
		separator = "&"
	}
	return pattern + separator + "emit_interim=true"
}

type patternTransformer struct {
	mux     genx.TransformerMux
	pattern string
}

func (t patternTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.mux.Transform(ctx, t.pattern, input)
}

type runtimeMemoryLoader struct{ service *peergenx.Service }

// NewRuntimeMemoryLoader resolves Flowcraft extraction and embedding aliases
// through the immutable RuntimeProfile-backed GenX service for one generation.
func NewRuntimeMemoryLoader(service *peergenx.Service) memoryflowcraft.ModelLoader {
	return runtimeMemoryLoader{service: service}
}

func (l runtimeMemoryLoader) LoadLLM(_ context.Context, alias string) (flowllm.LLM, error) {
	if l.service == nil {
		return nil, fmt.Errorf("flowcraft: RuntimeProfile model loader is not configured")
	}
	return genxflowcraft.ResolveLLM(l.service.Generator(), alias)
}

func (l runtimeMemoryLoader) LoadEmbedder(ctx context.Context, alias string) (flowembedding.Embedder, error) {
	if l.service == nil {
		return nil, fmt.Errorf("flowcraft: RuntimeProfile embedding loader is not configured")
	}
	config, err := l.service.ResolveEmbedding(ctx, modelPattern(alias))
	if err != nil {
		return nil, err
	}
	return buildRuntimeEmbedder(config)
}

func buildRuntimeEmbedder(config peergenx.EmbeddingConfig) (flowembedding.Embedder, error) {
	modelName := string(config.Model.Id)
	switch config.Tenant.Kind {
	case string(apitypes.ModelProviderKindOpenaiTenant):
		if config.Tenant.OpenAI == nil {
			return nil, fmt.Errorf("flowcraft: OpenAI embedding tenant is required")
		}
		body, err := config.Credential.Body.AsOpenAICredentialBody()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode OpenAI embedding credential: %w", err)
		}
		data, err := config.Model.ProviderData.AsOpenAITenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode OpenAI embedding model: %w", err)
		}
		if upstream := strings.TrimSpace(data.UpstreamModel); upstream != "" {
			modelName = upstream
		}
		apiKey := firstNonEmpty(body.ApiKey, body.Token)
		if apiKey == "" {
			return nil, fmt.Errorf("flowcraft: embedding credential %q has no api_key", config.Credential.Id)
		}
		var options []option.RequestOption
		if baseURL := firstNonEmpty(config.Tenant.OpenAI.BaseUrl, body.BaseUrl); baseURL != "" {
			options = append(options, option.WithBaseURL(baseURL))
		}
		return embeddingopenai.New(apiKey, modelName, options...), nil

	case string(apitypes.ModelProviderKindDashscopeTenant):
		if config.Tenant.DashScope == nil {
			return nil, fmt.Errorf("flowcraft: DashScope embedding tenant is required")
		}
		body, err := config.Credential.Body.AsDashScopeCredentialBody()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode DashScope embedding credential: %w", err)
		}
		data, err := config.Model.ProviderData.AsDashScopeTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode DashScope embedding model: %w", err)
		}
		modelName = firstNonEmpty(data.UpstreamModel, &modelName)
		apiKey := firstNonEmpty(body.ApiKey, body.Token)
		if apiKey == "" {
			return nil, fmt.Errorf("flowcraft: embedding credential %q has no api_key", config.Credential.Id)
		}
		return embeddingqwen.New(
			apiKey,
			modelName,
			firstNonEmpty(config.Tenant.DashScope.BaseUrl, body.BaseUrl),
		)

	case string(apitypes.ModelProviderKindVolcTenant):
		if config.Tenant.Volc == nil {
			return nil, fmt.Errorf("flowcraft: Volc embedding tenant is required")
		}
		body, err := config.Credential.Body.AsVolcCredentialBody()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode Volc embedding credential: %w", err)
		}
		data, err := config.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode Volc embedding model: %w", err)
		}
		modelName = firstNonEmpty(data.UpstreamModel, &modelName)
		apiKey := firstNonEmpty(body.ArkApiKey)
		if apiKey == "" {
			return nil, fmt.Errorf("flowcraft: embedding credential %q has no ark_api_key", config.Credential.Id)
		}
		return embeddingbytedance.New(
			apiKey,
			modelName,
			firstNonEmpty(config.Tenant.Volc.Endpoint),
			firstNonEmpty(config.Tenant.Volc.Region),
		)
	default:
		return nil, fmt.Errorf("flowcraft: embedding provider %q is unsupported", config.Tenant.Kind)
	}
}

type managedAgent struct {
	agenthost.Agent
	owned     []io.Closer
	closeOnce sync.Once
	closeErr  error
}

// NewManagedAgent exposes the product runtime surface for a direct reusable
// Flowcraft Transformer while retaining ownership of its Store resources.
func NewManagedAgent(transformer genx.Transformer, owned []io.Closer, agentMemory memory.Store, scope memory.Scope) agenthost.Agent {
	return NewManagedAgentWithBackend(transformer, owned, agentMemory, scope, "flowcraft")
}

// NewManagedAgentWithBackend retains command-owned provider identity without
// inferring it from a concrete Store implementation.
func NewManagedAgentWithBackend(transformer genx.Transformer, owned []io.Closer, agentMemory memory.Store, scope memory.Scope, backend string) agenthost.Agent {
	return &managedAgent{
		Agent: agenthost.NewMemoryAgent(agenthost.NewTransformerAgent(transformer), agentMemory, scope, backend),
		owned: owned,
	}
}

func (a *managedAgent) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() { a.closeErr = closeAll(a.owned) })
	return a.closeErr
}

type multiCloser []io.Closer

func (closers multiCloser) Close() error { return closeAll(closers) }

func closeAll(closers []io.Closer) error {
	var err error
	for index := len(closers) - 1; index >= 0; index-- {
		if closers[index] != nil {
			err = errors.Join(err, closers[index].Close())
		}
	}
	return err
}

func genxflowcraftBoardInputs(provider InputProvider) func(context.Context) (map[string]any, error) {
	if provider == nil {
		return nil
	}
	return func(ctx context.Context) (map[string]any, error) { return provider(ctx) }
}

// WorkspaceAgentScope is the stable owner/canonical-Workspace-ID/Agent
// namespace shared by History, State, and Flowcraft Memory.
func WorkspaceAgentScope(owner, workspaceID, agentID string) string {
	parts := make([]string, 0, 6)
	if owner = strings.TrimSpace(owner); owner != "" {
		parts = append(parts, "o", scopeToken(owner))
	}
	return strings.Join(append(parts, "w", scopeToken(workspaceID), "a", scopeToken(agentID)), "/")
}

func workspaceAgentScope(owner, workspaceID, agentID string) string {
	return WorkspaceAgentScope(owner, workspaceID, agentID)
}

func flowcraftStateStore(base kv.Store, scope string) kv.Store {
	return kv.Prefixed(base, append(kv.Key{"flowcraft"}, strings.Split(scope, "/")...))
}

// scopeToken keeps the product-owned owner/Workspace/Agent namespace short.
// Flowcraft embeds the logical scope in retrieval namespaces and WAL object
// keys, so preserving raw public keys and resource names can exceed filesystem
// component limits after ObjectStore metadata encoding.
func scopeToken(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return fmt.Sprintf("%x", digest[:8])
}

func modelPattern(alias string) string {
	alias = strings.Trim(strings.TrimSpace(alias), "/")
	if strings.Contains(alias, "/") {
		return alias
	}
	return "model/" + alias
}

func voicePattern(alias string) string {
	alias = strings.Trim(strings.TrimSpace(alias), "/")
	if strings.Contains(alias, "/") {
		return alias
	}
	return "voice/" + alias
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func boolValue(value *bool) bool { return value != nil && *value }

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func firstNonEmpty(values ...*string) string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return strings.TrimSpace(*value)
		}
	}
	return ""
}

func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func joinClosers(closers ...io.Closer) io.Closer {
	var filtered []io.Closer
	for _, closer := range closers {
		if closer != nil {
			filtered = append(filtered, closer)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return closerList(filtered)
}

type closerList []io.Closer

func (closers closerList) Close() error { return closeAll(closers) }

func setString(target map[string]any, key string, value *string) {
	if value != nil {
		target[key] = *value
	}
}

func setValue[T any](target map[string]any, key string, value *T) {
	if value != nil {
		target[key] = *value
	}
}
