package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/google/jsonschema-go/jsonschema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
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

const (
	Type = "eino"

	einoAudioInputUnsupportedCode    = "EINO_AUDIO_INPUT_UNSUPPORTED"
	einoAudioInputUnsupportedMessage = "Eino audio input requires voice_adapter.asr_model"
)

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
	if err := einoconfig.Validate(*public); err != nil {
		return nil, fmt.Errorf("eino: invalid workflow config: %w", err)
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
	inputMode, err := resolveEinoInputMode(spec.Workspace.Parameters)
	if err != nil {
		return nil, err
	}
	if public.VoiceAdapter != nil {
		if err := preflightVoiceAdapter(ctx, service, *public.VoiceAdapter, inputMode); err != nil {
			return nil, err
		}
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
	var composed genx.Transformer = transformer
	if public.VoiceAdapter != nil {
		composed, err = wrapAudio(service.Transformer(), transformer, *public.VoiceAdapter, public.Graph.Outputs, inputMode)
		if err != nil {
			return nil, errors.Join(err, transformer.Close(), closeMemory(memoryCloser))
		}
	}
	if !einoVoiceAdapterHasASR(public.VoiceAdapter) {
		composed = einoAudioInputGuard{next: composed}
	}
	agent := agenthost.NewTransformerAgent(composed)
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

func resolveEinoInputMode(parameters *apitypes.WorkspaceParameters) (apitypes.WorkspaceInputMode, error) {
	if parameters == nil {
		return apitypes.WorkspaceInputModePushToTalk, nil
	}
	value, err := parameters.AsEinoWorkspaceParameters()
	if err != nil {
		return "", fmt.Errorf("eino: decode workspace parameters: %w", err)
	}
	if value.Input == nil {
		return apitypes.WorkspaceInputModePushToTalk, nil
	}
	if !value.Input.Valid() {
		return "", fmt.Errorf("eino: unsupported workspace input %q", *value.Input)
	}
	return *value.Input, nil
}

func preflightVoiceAdapter(ctx context.Context, service *peergenx.Service, voice apitypes.VoiceAdapter, inputMode apitypes.WorkspaceInputMode) error {
	if alias := stringPointerValue(voice.AsrModel); alias != "" {
		resolved, err := service.ResolveTransformer(ctx, einoASRPattern(alias, inputMode))
		if err != nil {
			return fmt.Errorf("eino: resolve voice_adapter.asr_model %q: %w", alias, err)
		}
		if resolved.Model == nil || resolved.Model.Kind != apitypes.ModelKindAsr {
			return fmt.Errorf("eino: voice_adapter.asr_model %q is not an ASR model", alias)
		}
	}
	aliases := make(map[string]struct{})
	if alias := stringPointerValue(voice.DefaultVoice); alias != "" {
		aliases[alias] = struct{}{}
	}
	if voice.NodeVoices != nil {
		for _, alias := range *voice.NodeVoices {
			aliases[strings.TrimSpace(alias)] = struct{}{}
		}
	}
	for alias := range aliases {
		resolved, err := service.ResolveTransformer(ctx, einoVoicePattern(alias))
		if err != nil {
			return fmt.Errorf("eino: resolve voice_adapter Voice %q: %w", alias, err)
		}
		if resolved.Voice == nil {
			return fmt.Errorf("eino: voice_adapter Voice %q is not a Voice resource", alias)
		}
	}
	return nil
}

func wrapAudio(
	mux genx.TransformerMux,
	core genx.Transformer,
	voice apitypes.VoiceAdapter,
	outputs []apitypes.EinoOutput,
	inputMode apitypes.WorkspaceInputMode,
) (genx.Transformer, error) {
	config := audiodock.Config{Agent: core}
	if alias := stringPointerValue(voice.AsrModel); alias != "" {
		config.ASR = einoPatternTransformer{mux: mux, pattern: einoASRPattern(alias, inputMode)}
	}
	defaultVoice := stringPointerValue(voice.DefaultVoice)
	nodeVoices := map[string]string(nil)
	if voice.NodeVoices != nil {
		nodeVoices = maps.Clone(*voice.NodeVoices)
	}
	if defaultVoice != "" || len(nodeVoices) != 0 {
		config.TTS = mux
		config.ResolveVoice = einoVoiceResolver(defaultVoice, nodeVoices, einoOutputNodes(outputs))
	}
	return audiodock.New(config)
}

func einoOutputNodes(outputs []apitypes.EinoOutput) map[string]string {
	result := make(map[string]string, len(outputs))
	for _, output := range outputs {
		result[strings.TrimSpace(output.Name)] = strings.TrimSpace(output.Node)
	}
	return result
}

func einoVoiceResolver(defaultVoice string, nodeVoices, outputNodes map[string]string) audiodock.VoiceResolver {
	return func(_ context.Context, request audiodock.VoiceRequest) (string, error) {
		alias := strings.TrimSpace(nodeVoices[outputNodes[strings.TrimSpace(request.Name)]])
		if alias == "" {
			alias = strings.TrimSpace(defaultVoice)
		}
		if alias == "" {
			return "", nil
		}
		return einoVoicePattern(alias), nil
	}
}

func einoASRPattern(alias string, inputMode apitypes.WorkspaceInputMode) string {
	pattern := "model/" + strings.TrimSpace(alias)
	if inputMode == apitypes.WorkspaceInputModeRealtime {
		return pattern + "?emit_interim=true"
	}
	return pattern
}

func einoVoicePattern(alias string) string { return "voice/" + strings.TrimSpace(alias) }

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func einoVoiceAdapterHasASR(voice *apitypes.VoiceAdapter) bool {
	return voice != nil && stringPointerValue(voice.AsrModel) != ""
}

// einoAudioInputGuard keeps the product-level live-audio capability at the
// Eino factory boundary. The provider-neutral Eino Transformer deliberately
// does not own ASR configuration or infer a default ASR model.
type einoAudioInputGuard struct {
	next genx.Transformer
}

func (g einoAudioInputGuard) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if g.next == nil {
		return nil, fmt.Errorf("eino: audio input guard requires a downstream Transformer")
	}
	if input == nil {
		return nil, fmt.Errorf("eino: audio input guard requires an input Stream")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	guardedInput := &einoAudioInputStream{
		source:   input,
		output:   output,
		pending:  make(map[string][]*genx.MessageChunk),
		rejected: make(map[string]struct{}),
	}
	downstream, err := g.next.Transform(ctx, guardedInput)
	if err != nil {
		_ = guardedInput.CloseWithError(err)
		_ = output.Abort(err)
		return nil, err
	}
	if downstream == nil {
		err := fmt.Errorf("eino: audio input guard received a nil downstream Stream")
		_ = guardedInput.CloseWithError(err)
		_ = output.Abort(err)
		return nil, err
	}

	guardedOutput := &einoAudioOutputStream{
		stream:     output.Stream(),
		builder:    output,
		downstream: downstream,
		input:      guardedInput,
	}
	guardedOutput.stopContext = context.AfterFunc(ctx, func() {
		_ = guardedOutput.closeStreams(context.Cause(ctx))
	})
	go guardedOutput.forward()
	return guardedOutput, nil
}

type einoAudioInputStream struct {
	source genx.Stream
	output *genx.StreamBuilder

	pending      map[string][]*genx.MessageChunk
	pendingOrder []string
	ready        []*genx.MessageChunk
	rejected     map[string]struct{}
	sourceErr    error
}

func (s *einoAudioInputStream) Next() (*genx.MessageChunk, error) {
	for {
		if len(s.ready) != 0 {
			chunk := s.ready[0]
			s.ready[0] = nil
			s.ready = s.ready[1:]
			return chunk, nil
		}
		if s.sourceErr != nil {
			return nil, s.sourceErr
		}
		chunk, err := s.source.Next()
		if err != nil {
			s.flushPending()
			s.sourceErr = err
			continue
		}
		streamID := einoChunkStreamID(chunk)
		if _, rejected := s.rejected[streamID]; streamID != "" && rejected {
			if chunk.IsEndOfStream() {
				delete(s.rejected, streamID)
			}
			continue
		}
		_, pending := s.pending[streamID]
		if streamID != "" && einoControlOnly(chunk) && (chunk.IsBeginOfStream() || pending) {
			s.buffer(streamID, chunk)
			if chunk.IsEndOfStream() {
				s.release(streamID)
			}
			continue
		}
		blob, audio := einoOrdinaryUserAudio(chunk)
		if audio && len(blob.Data) == 0 {
			s.buffer(streamID, chunk)
			if chunk.IsEndOfStream() {
				s.release(streamID)
			}
			continue
		}
		if audio {
			s.discard(streamID)
			s.rejected[streamID] = struct{}{}
			if err := s.output.Add(einoAudioInputUnsupportedTerminal(streamID)); err != nil {
				_ = s.source.CloseWithError(err)
				return nil, err
			}
			if chunk.IsEndOfStream() {
				delete(s.rejected, streamID)
			}
			continue
		}
		s.release(streamID)
		s.ready = append(s.ready, chunk)
	}
}

func (s *einoAudioInputStream) buffer(streamID string, chunk *genx.MessageChunk) {
	if len(s.pending[streamID]) == 0 {
		s.pendingOrder = append(s.pendingOrder, streamID)
	}
	s.pending[streamID] = append(s.pending[streamID], chunk)
}

func (s *einoAudioInputStream) release(streamID string) {
	pending := s.pending[streamID]
	s.discard(streamID)
	if len(pending) != 0 {
		s.ready = append(s.ready, pending...)
	}
}

func (s *einoAudioInputStream) discard(streamID string) {
	delete(s.pending, streamID)
	for index := 0; index < len(s.pendingOrder); {
		if s.pendingOrder[index] != streamID {
			index++
			continue
		}
		s.pendingOrder = slices.Delete(s.pendingOrder, index, index+1)
	}
}

func (s *einoAudioInputStream) flushPending() {
	for len(s.pendingOrder) != 0 {
		s.release(s.pendingOrder[0])
	}
}

func (s *einoAudioInputStream) Close() error {
	return s.source.Close()
}

func (s *einoAudioInputStream) CloseWithError(err error) error {
	return s.source.CloseWithError(err)
}

func einoOrdinaryUserAudio(chunk *genx.MessageChunk) (*genx.Blob, bool) {
	if chunk == nil || chunk.Role != genx.RoleUser || einoChunkStreamID(chunk) == "" ||
		chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.Label) == genx.HistoryUserAudioLabel {
		return nil, false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil {
		return nil, false
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return nil, false
	}
	mediaType, _, _ := strings.Cut(mimeType, ";")
	return blob, strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "audio/")
}

func einoControlOnly(chunk *genx.MessageChunk) bool {
	return chunk != nil && chunk.Part == nil && chunk.ToolCall == nil
}

func einoChunkStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}

func einoAudioInputUnsupportedTerminal(streamID string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: "assistant",
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:       streamID,
			Label:          "assistant",
			Error:          einoAudioInputUnsupportedMessage,
			ErrorCode:      einoAudioInputUnsupportedCode,
			ErrorRetryable: false,
			FailureClass:   genx.FailureClassTransform,
			EndOfStream:    true,
		},
	}
}

type einoAudioOutputStream struct {
	stream     genx.Stream
	builder    *genx.StreamBuilder
	downstream genx.Stream
	input      genx.Stream

	closeOnce   sync.Once
	stopContext func() bool
}

func (s *einoAudioOutputStream) Next() (*genx.MessageChunk, error) {
	return s.stream.Next()
}

func (s *einoAudioOutputStream) Close() error {
	return s.close(nil)
}

func (s *einoAudioOutputStream) CloseWithError(err error) error {
	return s.close(err)
}

func (s *einoAudioOutputStream) close(err error) error {
	if s.stopContext != nil {
		s.stopContext()
	}
	return s.closeStreams(err)
}

func (s *einoAudioOutputStream) closeStreams(err error) error {
	var result error
	s.closeOnce.Do(func() {
		if err != nil {
			result = errors.Join(s.stream.CloseWithError(err), s.downstream.CloseWithError(err), s.input.CloseWithError(err))
			return
		}
		result = errors.Join(s.stream.Close(), s.downstream.Close(), s.input.Close())
	})
	return result
}

func (s *einoAudioOutputStream) forward() {
	defer s.downstream.Close()
	defer func() {
		if s.stopContext != nil {
			s.stopContext()
		}
	}()
	for {
		chunk, err := s.downstream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
				_ = s.input.Close()
				_ = s.builder.Done(genx.Usage{})
				return
			}
			_ = s.input.CloseWithError(err)
			_ = s.builder.Abort(err)
			return
		}
		if err := s.builder.Add(chunk); err != nil {
			_ = s.downstream.CloseWithError(err)
			_ = s.input.CloseWithError(err)
			return
		}
	}
}

type einoPatternTransformer struct {
	mux     genx.TransformerMux
	pattern string
}

func (t einoPatternTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.mux.Transform(ctx, t.pattern, input)
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
