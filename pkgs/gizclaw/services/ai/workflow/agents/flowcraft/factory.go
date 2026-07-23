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
	"time"

	flowmemory "github.com/GizClaw/flowcraft/memory/recall"
	flowmemorystore "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	flowretrievalstore "github.com/GizClaw/flowcraft/memory/retrieval/workspace"
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
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/openai/openai-go/option"
)

const Type = "flowcraft"

// Factory maps the public GizClaw Workflow plus AgentHost-owned dependencies
// into the reusable GenX Flowcraft Transformer and Audio Dock.
type Factory struct {
	GenX          *peergenx.Service
	GenXForOwner  func(context.Context, string) (*peergenx.Service, error)
	History       logstore.MutableStore
	State         kv.Store
	MemoryObjects objectstore.ObjectStore
	MemoryLoader  memoryflowcraft.ModelLoader
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
	public := *spec.Workflow.Spec.Flowcraft
	owner := stringValue(spec.Workspace.OwnerPublicKey)
	initiativePolicy := ""
	if owner != "" && f.GenXForOwner != nil {
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
	}
	return f.newAgent(ctx, owner, workspaceName, public, nil, initiativePolicy)
}

func (f Factory) newAgent(ctx context.Context, owner, workspaceName string, public apitypes.FlowcraftWorkflowSpec, inputs InputProvider, initiativePolicy string) (agenthost.Agent, error) {
	if f.GenX == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	if err := public.Validate(); err != nil {
		return nil, fmt.Errorf("flowcraft: invalid workflow config: %w", err)
	}
	graph, publishNodes, err := mapGraph(public.Agent.Graph)
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
	agentID := strings.TrimSpace(public.Agent.Id)
	if agentID == "" {
		return nil, fmt.Errorf("flowcraft: agent.id is required")
	}
	scope := WorkspaceAgentScope(owner, workspaceName, agentID)
	config := genxflowcraft.Config{
		ID: agentID, Name: strings.TrimSpace(public.Agent.Name), Graph: graph,
		MaxIterations: intValue(public.Agent.MaxIterations), PublishNodes: publishNodes,
		Models: f.GenX.Generator(), History: f.History, HistoryScope: scope, ContextID: scope,
		BoardInputs: genxflowcraftBoardInputs(inputs),
	}
	config.Initiative = mapInitiative(public.Conversation, initiativePolicy)
	if public.Agent.Description != nil {
		config.Description = *public.Agent.Description
	}
	if f.State != nil {
		config.State = kv.Prefixed(f.State, kv.Key{"flowcraft", workspaceName, agentID})
	}

	var owned []io.Closer
	var agentMemory memory.Store
	if public.Memory != nil && public.Memory.Enabled {
		memoryBuild, err := f.BuildMemory(ctx, owner, workspaceName, agentID, *public.Memory)
		if err != nil {
			return nil, err
		}
		owned = append(owned, memoryBuild.Closer)
		config.Memory = memoryBuild.Store
		agentMemory = memoryBuild.Store
		config.MemoryScope = memory.Scope(scope)
		config.RecallProfiles = memoryBuild.RecallProfiles
		config.ObserveEnabled = memoryBuild.ObserveEnabled
		config.ObserveWaitForCompletion = memoryBuild.ObserveWaitForCompletion
		config.ObservationBuilder = memoryBuild.ObservationBuilder
	}

	core, err := genxflowcraft.New(config)
	if err != nil {
		return nil, errors.Join(err, closeAll(owned))
	}
	var transformer genx.Transformer = core
	if public.VoiceAdapter != nil {
		transformer, err = f.wrapAudio(core, *public.VoiceAdapter)
		if err != nil {
			return nil, errors.Join(err, closeAll(owned))
		}
	}
	return NewManagedAgent(transformer, owned, agentMemory, memory.Scope(scope)), nil
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
			return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: agent.graph.nodes[%d].type: %w", index, err)
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
		default:
			return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: unsupported agent.graph.nodes[%d].type %q", index, discriminator)
		}
		graph.Nodes = append(graph.Nodes, node)
	}
	if len(publish) == 0 {
		return flowgraph.GraphDefinition{}, nil, fmt.Errorf("flowcraft: agent.graph requires at least one publish node")
	}
	return graph, publish, nil
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

// MemoryBuild is the product-owned Store assembly shared by Flowcraft-backed
// workflow drivers. It never owns Graph execution or stream lifecycle.
type MemoryBuild struct {
	Store                    memory.Store
	Closer                   io.Closer
	RecallProfiles           []genxflowcraft.MemoryRecallProfile
	ObserveEnabled           bool
	ObserveWaitForCompletion bool
	ObservationBuilder       genxflowcraft.ObservationBuilder
}

// BuildMemory maps public Flowcraft Memory configuration to the provider-neutral
// Store used by a particular owner, Workspace, and Agent scope.
func (f Factory) BuildMemory(ctx context.Context, owner, workspaceName, agentID string, public apitypes.FlowcraftMemory) (MemoryBuild, error) {
	if f.MemoryObjects == nil {
		return MemoryBuild{}, fmt.Errorf("flowcraft: workspace %q memory requires a server object store", workspaceName)
	}
	workspace, err := newObjectWorkspace(f.MemoryObjects, memoryObjectPrefix(owner, workspaceName, agentID))
	if err != nil {
		return MemoryBuild{}, err
	}
	backend, err := flowmemorystore.New(workspace)
	if err != nil {
		return MemoryBuild{}, fmt.Errorf("flowcraft: workspace %q memory backend: %w", workspaceName, err)
	}
	retrievalIndex, err := flowretrievalstore.New(workspace)
	if err != nil {
		_ = backend.Close()
		return MemoryBuild{}, fmt.Errorf("flowcraft: workspace %q retrieval index: %w", workspaceName, err)
	}
	loader := f.MemoryLoader
	if loader == nil {
		loader = runtimeMemoryLoader{service: f.GenX}
	}
	runtimeConfig, mapped, err := mapMemoryConfig(public, loader, backend, retrievalIndex)
	if err != nil {
		_ = retrievalIndex.Close()
		_ = backend.Close()
		return MemoryBuild{}, fmt.Errorf("flowcraft: workspace %q memory: %w", workspaceName, err)
	}
	store, err := memoryflowcraft.New(ctx, runtimeConfig)
	if err != nil {
		_ = retrievalIndex.Close()
		_ = backend.Close()
		return MemoryBuild{}, fmt.Errorf("flowcraft: workspace %q memory: %w", workspaceName, err)
	}
	// closeAll reverses this construction-order slice: Store first, then its
	// retrieval and persistence dependencies.
	return MemoryBuild{
		Store: store, Closer: multiCloser{backend, retrievalIndex, store},
		RecallProfiles: mapped.recallProfiles, ObserveEnabled: mapped.observe,
		ObserveWaitForCompletion: mapped.observeWait, ObservationBuilder: mapped.observationBuilder,
	}, nil
}

// buildMemory retains the package-local test seam while callers outside this
// package use the explicit MemoryBuild result.
func (f Factory) buildMemory(ctx context.Context, owner, workspaceName, agentID string, public apitypes.FlowcraftMemory) (memory.Store, io.Closer, mappedMemoryConfig, error) {
	built, err := f.BuildMemory(ctx, owner, workspaceName, agentID, public)
	if err != nil {
		return nil, nil, mappedMemoryConfig{}, err
	}
	return built.Store, built.Closer, mappedMemoryConfig{
		recallProfiles: built.RecallProfiles, observe: built.ObserveEnabled,
		observeWait: built.ObserveWaitForCompletion, observationBuilder: built.ObservationBuilder,
	}, nil
}

// memoryObjectPrefix keeps the physical object key independent from public
// workspace and Agent name lengths. Flowcraft adds its native scope to several
// retrieval filenames, while filesystem-backed ObjectStores also derive
// metadata filenames from the complete key. A short deterministic prefix keeps
// those composed names below filesystem component limits without changing the
// logical memory.Scope stored in facts.
func memoryObjectPrefix(owner, workspaceName, agentID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(owner) + "\x00" + strings.TrimSpace(workspaceName) + "\x00" + strings.TrimSpace(agentID)))
	return fmt.Sprintf("fc/%x", digest[:16])
}

type mappedMemoryConfig struct {
	recallProfiles     []genxflowcraft.MemoryRecallProfile
	observe            bool
	observeWait        bool
	observationBuilder genxflowcraft.ObservationBuilder
}

func mapMemoryConfig(public apitypes.FlowcraftMemory, loader memoryflowcraft.ModelLoader, backend *flowmemorystore.Backend, retrievalIndex *flowretrievalstore.Index) (memoryflowcraft.Config, mappedMemoryConfig, error) {
	config := memoryflowcraft.Config{
		Loader: loader, RetrievalIndex: retrievalIndex, TemporalStore: backend.TemporalStore(), EvidenceStore: backend.EvidenceStore(), SideEffectOutbox: backend.SideEffectOutbox(),
	}
	if public.Extract != nil && boolDefault(public.Extract.Enabled, true) {
		config.Extraction.Model = stringValue(public.Extract.Model)
		config.Extraction.Mode = flowmemory.LLMExtractionMode(stringValue((*string)(public.Extract.Mode)))
		config.Extraction.SystemPrompt = memoryExtractionPrompt(*public.Extract, public.Layout)
		config.Extraction.SchemaName = stringValue(public.Extract.SchemaName)
		if public.Extract.Temperature != nil {
			value := float64(*public.Extract.Temperature)
			config.Extraction.Temperature = &value
		}
		if value := stringValue(public.Extract.StageTimeout); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return memoryflowcraft.Config{}, mappedMemoryConfig{}, fmt.Errorf("extract.stage_timeout: %w", err)
			}
			config.Extraction.StageTimeout = duration
		}
	}
	if public.Embedding != nil && boolValue(public.Embedding.Enabled) {
		config.Embedding.Model = stringValue(public.Embedding.Model)
		if config.Embedding.Model == "" {
			return memoryflowcraft.Config{}, mappedMemoryConfig{}, fmt.Errorf("embedding.model is required when embedding is enabled")
		}
	}
	if public.Rerank != nil && boolValue(public.Rerank.Enabled) {
		config.Rerank.Model = stringValue(public.Rerank.Model)
		if config.Rerank.Model == "" {
			return memoryflowcraft.Config{}, mappedMemoryConfig{}, fmt.Errorf("rerank.model is required when rerank is enabled")
		}
	}
	if public.Recall != nil {
		config.GraphEnabled = boolValue(public.Recall.GraphEnabled)
	}
	writeMode := "sync"
	if public.Write != nil && public.Write.Mode != nil {
		writeMode = string(*public.Write.Mode)
	}
	if public.Write != nil {
		if public.Write.Tier != nil {
			config.Tier = string(*public.Write.Tier)
		}
	}
	if writeMode == "async_semantic" && config.Extraction.Model != "" {
		config.AsyncQueue = backend.AsyncSemanticQueue()
	}
	observe := memoryObserveEnabled(public)
	mapped := mappedMemoryConfig{
		recallProfiles: mapRecallProfiles(public),
		observe:        observe,
		observeWait:    observe && writeMode == "sync",
	}
	mapped.observationBuilder = observationBuilder(public.Write)
	return config, mapped, nil
}

func mapRecallProfiles(public apitypes.FlowcraftMemory) []genxflowcraft.MemoryRecallProfile {
	if public.Recall == nil || !boolDefault(public.Recall.Enabled, true) || public.Recall.Profiles == nil {
		return nil
	}
	laneKinds := make(map[string]string)
	laneRecall := make(map[string]string)
	if public.Layout != nil && public.Layout.Lanes != nil {
		for _, lane := range *public.Layout.Lanes {
			laneKinds[lane.Name] = lane.Kind
			laneRecall[lane.Name] = stringValue(lane.Recall)
		}
	}
	names := make([]string, 0, len(*public.Recall.Profiles))
	for name := range *public.Recall.Profiles {
		names = append(names, name)
	}
	slices.Sort(names)
	result := make([]genxflowcraft.MemoryRecallProfile, 0, len(names))
	for _, name := range names {
		profile := (*public.Recall.Profiles)[name]
		filters := make([]memory.Filter, 0)
		if profile.Query != nil {
			for _, kind := range valueOrZero(profile.Query.Kinds) {
				filters = append(filters, memory.Filter{Field: "kind", Operator: memory.FilterEqual, Value: kind})
			}
			for _, lane := range valueOrZero(profile.Query.Lanes) {
				if kind := laneKinds[lane]; kind != "" {
					filters = append(filters, memory.Filter{Field: "kind", Operator: memory.FilterEqual, Value: kind})
				}
			}
			for _, filter := range valueOrZero(profile.Query.Filters) {
				operator := memory.FilterEqual
				if filter.Operator != nil {
					operator = memory.FilterOperator(*filter.Operator)
				}
				filters = append(filters, memory.Filter{Field: filter.Field, Operator: operator, Value: filter.Value})
			}
		}
		if boolValue(public.Recall.IncludeRetired) {
			filters = append(filters, memory.Filter{Field: "include_retired", Operator: memory.FilterEqual, Value: true})
		}
		result = append(result, genxflowcraft.MemoryRecallProfile{
			BoardVariable: profile.Output, QueryText: recallQueryText(profile.Query), Limit: profile.TopK, Filters: filters, Renderer: recallRenderer(profile.Render, recallGuidance(profile.Query, laneRecall)),
		})
	}
	return result
}

func recallQueryText(query *apitypes.FlowcraftMemoryRecallQuery) string {
	if query == nil {
		return ""
	}
	return stringValue(query.Text)
}

func recallGuidance(query *apitypes.FlowcraftMemoryRecallQuery, lanes map[string]string) []string {
	if query == nil {
		return nil
	}
	var guidance []string
	for _, lane := range valueOrZero(query.Lanes) {
		if text := strings.TrimSpace(lanes[lane]); text != "" {
			guidance = append(guidance, text)
		}
	}
	return guidance
}

func recallRenderer(config *apitypes.FlowcraftMemoryRecallRender, guidance []string) genxflowcraft.RecallRenderer {
	if config == nil && len(guidance) == 0 {
		return nil
	}
	var header, prefix string
	var maxItems int
	if config != nil {
		header = stringValue(config.Header)
		prefix = stringValue(config.ItemPrefix)
		maxItems = intValue(config.MaxItems)
	}
	if prefix == "" {
		prefix = "- "
	}
	return func(_ context.Context, matches []memory.Match) (string, error) {
		if maxItems > 0 && len(matches) > maxItems {
			matches = matches[:maxItems]
		}
		var lines []string
		for _, match := range matches {
			if text := strings.TrimSpace(match.Fact.Text); text != "" {
				lines = append(lines, prefix+text)
			}
		}
		if len(lines) == 0 {
			return "", nil
		}
		if header != "" {
			lines = append([]string{header}, lines...)
		}
		if len(guidance) != 0 {
			lines = append([]string{"Recall policy:", strings.Join(guidance, "\n")}, lines...)
		}
		return strings.Join(lines, "\n"), nil
	}
}

func memoryObserveEnabled(public apitypes.FlowcraftMemory) bool {
	if public.Write == nil {
		return false
	}
	return boolValue(public.Write.SaveConversation) || len(valueOrZero(public.Write.BoardFacts)) > 0
}

func observationBuilder(write *apitypes.FlowcraftMemoryWrite) genxflowcraft.ObservationBuilder {
	return func(ctx context.Context, input genxflowcraft.ObservationInput) (memory.Observation, error) {
		observation := memory.Observation{ID: input.StreamID}
		if write != nil && boolValue(write.SaveConversation) {
			var err error
			observation, err = genxflowcraft.DefaultObservationBuilder(ctx, input)
			if err != nil {
				return memory.Observation{}, err
			}
		}
		if write == nil || write.BoardFacts == nil {
			return observation, nil
		}
		for _, fact := range *write.BoardFacts {
			value, ok := input.BoardVariables[fact.BoardVar]
			if !ok {
				continue
			}
			text := boardFactText(value)
			if text == "" {
				continue
			}
			if required := strings.TrimSpace(stringValue(fact.RequiredPrefix)); required != "" {
				index := strings.Index(text, required)
				if index < 0 {
					continue
				}
				text = strings.TrimSpace(text[index:])
			}
			attributes := make(map[string]any)
			if kind := strings.TrimSpace(stringValue(fact.Kind)); kind != "" {
				attributes["kind"] = kind
			}
			if subject := strings.TrimSpace(stringValue(fact.Subject)); subject != "" {
				attributes["subject"] = subject
			}
			if predicate := strings.TrimSpace(stringValue(fact.Predicate)); predicate != "" {
				attributes["predicate"] = predicate
			}
			if object := strings.TrimSpace(stringValue(fact.Object)); object != "" {
				attributes["object"] = object
			}
			if fact.Entities != nil {
				entities := nonEmptyStrings(*fact.Entities)
				if len(entities) > 0 {
					attributes["entities"] = entities
				}
			}
			observation.Facts = append(observation.Facts, memory.FactCandidate{Text: text, Attributes: attributes})
		}
		return observation, nil
	}
}

func boardFactText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func memoryExtractionPrompt(extract apitypes.FlowcraftMemoryExtract, layout *apitypes.FlowcraftMemoryLayout) string {
	prompt := stringValue(extract.SystemPrompt)
	if layout == nil || layout.Lanes == nil || len(*layout.Lanes) == 0 {
		return prompt
	}
	var lines []string
	for _, lane := range *layout.Lanes {
		line := fmt.Sprintf("- %s (kind=%s): %s", lane.Name, lane.Kind, stringValue(lane.Description))
		if instruction := stringValue(lane.Extract); instruction != "" {
			line += " " + instruction
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	return strings.TrimSpace(prompt + "\n\nMemory lanes:\n" + strings.Join(lines, "\n"))
}

func (f Factory) wrapAudio(core genx.Transformer, voice apitypes.FlowcraftVoiceAdapter) (genx.Transformer, error) {
	config := audiodock.Config{Agent: core}
	if alias := stringValue(voice.AsrModel); alias != "" {
		config.ASR = patternTransformer{mux: f.GenX.Transformer(), pattern: modelPattern(alias)}
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

type patternTransformer struct {
	mux     genx.TransformerMux
	pattern string
}

func (t patternTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.mux.Transform(ctx, t.pattern, input)
}

type runtimeMemoryLoader struct{ service *peergenx.Service }

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
			return nil, fmt.Errorf("flowcraft: embedding credential %q has no api_key", config.Credential.Name)
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
			return nil, fmt.Errorf("flowcraft: embedding credential %q has no api_key", config.Credential.Name)
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
			return nil, fmt.Errorf("flowcraft: embedding credential %q has no ark_api_key", config.Credential.Name)
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
	owned       []io.Closer
	memory      memory.Store
	memoryScope memory.Scope
	closeOnce   sync.Once
	closeErr    error
}

// NewManagedAgent exposes the product runtime surface for a direct reusable
// Flowcraft Transformer while retaining ownership of its Store resources.
func NewManagedAgent(transformer genx.Transformer, owned []io.Closer, agentMemory memory.Store, scope memory.Scope) agenthost.Agent {
	return &managedAgent{
		Agent: agenthost.NewTransformerAgent(transformer), owned: owned,
		memory: agentMemory, memoryScope: scope,
	}
}

func (a *managedAgent) Status(ctx context.Context) (apitypes.PeerRunWorkspaceState, error) {
	status, err := a.Agent.Status(ctx)
	if err != nil {
		return status, err
	}
	available := a.memory != nil
	status.MemoryStatsAvailable = &available
	status.RecallAvailable = &available
	return status, nil
}

func (a *managedAgent) MemoryStats(ctx context.Context, _ apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	if a == nil || a.memory == nil {
		message := "workspace memory is not enabled"
		return apitypes.PeerRunMemoryStatsResponse{Available: true, Enabled: false, Message: &message}, nil
	}
	backend := "flowcraft"
	metadata := map[string]any{"scope": string(a.memoryScope)}
	response := apitypes.PeerRunMemoryStatsResponse{
		Available: true, Enabled: true, Backend: &backend, Metadata: &metadata,
	}
	if provider, ok := a.memory.(memory.StatisticsProvider); ok {
		stats, err := provider.Stats(ctx, a.memoryScope)
		if err != nil {
			return apitypes.PeerRunMemoryStatsResponse{}, err
		}
		response.ItemCount = stats.ItemCount
		if !stats.LastUpdatedAt.IsZero() {
			response.LastUpdatedAt = &stats.LastUpdatedAt
		}
	}
	return response, nil
}

func (a *managedAgent) Recall(ctx context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	if a == nil || a.memory == nil {
		message := "workspace memory is not enabled"
		return apitypes.PeerRunRecallResponse{Available: true, Hits: []apitypes.PeerRunRecallHit{}, Message: &message}, nil
	}
	limit := 10
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}
	filters := make([]memory.Filter, 0)
	if req.Filters != nil {
		keys := make([]string, 0, len(*req.Filters))
		for key := range *req.Filters {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			filters = append(filters, memory.Filter{Field: key, Operator: memory.FilterEqual, Value: (*req.Filters)[key]})
		}
	}
	result, err := a.memory.Recall(ctx, memory.Query{Scope: a.memoryScope, Text: req.Query, Limit: limit, Filters: filters})
	if err != nil {
		return apitypes.PeerRunRecallResponse{}, err
	}
	hits := make([]apitypes.PeerRunRecallHit, 0, len(result.Matches))
	for _, match := range result.Matches {
		metadata := maps.Clone(match.Fact.Attributes)
		hit := apitypes.PeerRunRecallHit{
			Id: match.Fact.ID, Score: match.Score, Snippet: match.Fact.Text,
			CreatedAt: &match.Fact.CreatedAt, Metadata: &metadata,
		}
		if len(match.Fact.Sources) > 0 && strings.TrimSpace(match.Fact.Sources[0].ObservationID) != "" {
			hit.SourceId = &match.Fact.Sources[0].ObservationID
			sourceType := "observation"
			hit.SourceType = &sourceType
		}
		hits = append(hits, hit)
	}
	return apitypes.PeerRunRecallResponse{Available: true, Hits: hits}, nil
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

// WorkspaceAgentScope is the stable owner/Workspace/Agent namespace shared by
// History, State, and Flowcraft Memory for product-owned fixed Graphs.
func WorkspaceAgentScope(owner, workspaceName, agentID string) string {
	parts := make([]string, 0, 6)
	if owner = strings.TrimSpace(owner); owner != "" {
		parts = append(parts, "o", scopeToken(owner))
	}
	return strings.Join(append(parts, "w", scopeToken(workspaceName), "a", scopeToken(agentID)), "/")
}

func workspaceAgentScope(owner, workspaceName, agentID string) string {
	return WorkspaceAgentScope(owner, workspaceName, agentID)
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
