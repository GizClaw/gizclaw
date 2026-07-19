package flowcraft

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/flowcraft/sdk/graph/runner"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	sdkworkspace "github.com/GizClaw/flowcraft/sdk/workspace"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	ownedflowcraft "github.com/GizClaw/gizclaw-go/pkgs/agent/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const Type = "flowcraft"

const (
	defaultInputStreamID = "audio"
	selfStartStreamID    = "flowcraft-self-start"
	transcriptLabel      = "transcript"
	assistantLabel       = "assistant"
	interruptedError     = "interrupted"
)

type inputMode string

const (
	inputModePushToTalk inputMode = "push_to_talk"
	inputModeRealtime   inputMode = "realtime"
)

var flowcraftModelRoles = []struct {
	settingKey string
	modelsKey  string
	required   bool
}{
	{settingKey: "generate_model", modelsKey: "chat", required: true},
	{settingKey: "extract_model", modelsKey: "extractor"},
	{settingKey: "embedding_model", modelsKey: "embedder"},
}

type Factory struct {
	GenX    *peergenx.Service
	History logstore.MutableStore
	Memory  memory.Store
}

// InputProvider returns transient Flowcraft Board inputs immediately before a
// turn starts. Keys prefixed with tmp_ are intentionally not persisted.
type InputProvider func(context.Context) (map[string]any, error)

// ConfiguredAgentOptions supplies a Go-owned Flowcraft configuration to the
// shared adapter. It is used by specialized drivers that run the owned Agent without
// exposing arbitrary Flowcraft graph configuration in their public schemas.
type ConfiguredAgentOptions struct {
	Flowcraft             map[string]any
	GenerateModel         string
	ExtractModel          string
	EmbeddingModel        string
	ASRModel              string
	DefaultVoice          string
	NodeVoices            map[string]string
	Conversation          string
	AgentInitiativePolicy string
	InputMode             string
	LocalDir              string
	WorkspaceName         string
	InputProvider         InputProvider
	Toolkit               *agenthost.ToolkitContext
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if f.GenX == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	cfg, err := parseWorkflowConfig(spec)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.Runtime.LocalDir) == "" {
		return nil, fmt.Errorf("flowcraft: local workspace directory is required")
	}
	workspaceParams, err := flowcraftWorkspaceParameters(spec.Workspace.Parameters)
	if err != nil {
		return nil, err
	}
	starts, initiativePolicy := flowcraftConversationSettings(workspaceParams, cfg.Spec.Flowcraft)
	mode := string(inputModePushToTalk)
	if workspaceParams != nil && workspaceParams.Input != nil {
		if normalized := normalizeInputMode(string(*workspaceParams.Input)); normalized != "" {
			mode = string(normalized)
		}
	}
	options := ConfiguredAgentOptions{
		Flowcraft:             cfg.Spec.Flowcraft,
		ASRModel:              cfg.Spec.VoiceAdapter.ASRModel,
		DefaultVoice:          cfg.Spec.VoiceAdapter.DefaultVoice,
		NodeVoices:            cfg.Spec.VoiceAdapter.NodeVoices,
		Conversation:          starts,
		AgentInitiativePolicy: initiativePolicy,
		InputMode:             mode,
		LocalDir:              spec.Runtime.LocalDir,
		WorkspaceName:         spec.Workspace.Name,
		Toolkit:               spec.Toolkit,
	}
	if workspaceParams != nil {
		options.GenerateModel = stringValue(workspaceParams.GenerateModel)
		options.ExtractModel = stringValue(workspaceParams.ExtractModel)
		options.EmbeddingModel = stringValue(workspaceParams.EmbeddingModel)
	}
	return f.NewConfiguredAgent(ctx, options)
}

// NewConfiguredAgent creates an owned Flowcraft Agent from a Go-owned configuration.
func (f Factory) NewConfiguredAgent(ctx context.Context, options ConfiguredAgentOptions) (agenthost.Agent, error) {
	if f.GenX == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	options.LocalDir = strings.TrimSpace(options.LocalDir)
	if options.LocalDir == "" {
		return nil, fmt.Errorf("flowcraft: local workspace directory is required")
	}
	voice := normalizeVoiceAdapter(voiceAdapterConfig{
		ASRModel:     strings.TrimSpace(options.ASRModel),
		DefaultVoice: strings.TrimSpace(options.DefaultVoice),
		NodeVoices:   options.NodeVoices,
	})
	if err := voice.validate(); err != nil {
		return nil, err
	}
	ws, err := sdkworkspace.NewLocalWorkspace(options.LocalDir)
	if err != nil {
		return nil, err
	}
	toolkit := commonagent.EmptyToolkit()
	if options.Toolkit != nil {
		toolkit, err = options.Toolkit.BuildAgentToolkit(ctx)
		if err != nil {
			return nil, fmt.Errorf("flowcraft: build agent toolkit: %w", err)
		}
	}
	coreConfig, err := buildOwnedRuntimeConfig(ctx, f.GenX, options, ws, toolkit, f.History, f.Memory)
	if err != nil {
		return nil, err
	}
	core, err := ownedflowcraft.New(coreConfig)
	if err != nil {
		return nil, err
	}
	if err := validateVoiceAdapterResources(ctx, f.GenX, voice); err != nil {
		return nil, err
	}
	starts := strings.TrimSpace(options.Conversation)
	if starts == "" {
		starts = "peer"
	}
	initiativePolicy := strings.TrimSpace(options.AgentInitiativePolicy)
	if initiativePolicy == "" {
		initiativePolicy = flowcraftDefaultAgentInitiativePolicy(starts)
	}
	mode := normalizeInputMode(options.InputMode)
	if mode == "" {
		mode = inputModePushToTalk
	}
	return &agent{
		transformers:   f.GenX,
		runtime:        ownedRuntime{agent: core},
		history:        core,
		outputObserver: core,
		historyID:      strings.TrimSpace(options.WorkspaceName),
		memory:         f.Memory,
		asrModel:       voice.ASRModel,
		defaultVoice:   voice.DefaultVoice,
		nodeVoices:     voice.NodeVoices,
		starts:         starts,
		startPolicy:    initiativePolicy,
		inputMode:      mode,
		localDir:       options.LocalDir,
	}, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

type workflowConfig struct {
	Spec struct {
		Flowcraft    map[string]any     `json:"flowcraft"`
		VoiceAdapter voiceAdapterConfig `json:"voice_adapter"`
	} `json:"spec"`
}

type voiceAdapterConfig struct {
	ASRModel     string            `json:"asr_model"`
	DefaultVoice string            `json:"default_voice"`
	NodeVoices   map[string]string `json:"node_voices"`
}

func parseWorkflowConfig(spec agenthost.Spec) (workflowConfig, error) {
	data, err := json.Marshal(spec.Workflow)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("flowcraft: encode workflow: %w", err)
	}
	var cfg workflowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return workflowConfig{}, fmt.Errorf("flowcraft: decode workflow: %w", err)
	}
	if cfg.Spec.Flowcraft == nil {
		cfg.Spec.Flowcraft = map[string]any{}
	}
	if raw, ok := cfg.Spec.Flowcraft["voice_adapter"]; ok {
		adapterData, err := json.Marshal(raw)
		if err != nil {
			return workflowConfig{}, fmt.Errorf("flowcraft: encode voice_adapter: %w", err)
		}
		if err := json.Unmarshal(adapterData, &cfg.Spec.VoiceAdapter); err != nil {
			return workflowConfig{}, fmt.Errorf("flowcraft: decode voice_adapter: %w", err)
		}
		delete(cfg.Spec.Flowcraft, "voice_adapter")
	}
	cfg.Spec.VoiceAdapter.ASRModel = strings.TrimSpace(cfg.Spec.VoiceAdapter.ASRModel)
	cfg.Spec.VoiceAdapter.DefaultVoice = strings.TrimSpace(cfg.Spec.VoiceAdapter.DefaultVoice)
	for rawNodeID, voice := range cfg.Spec.VoiceAdapter.NodeVoices {
		nodeID := strings.TrimSpace(rawNodeID)
		voice = strings.TrimSpace(voice)
		delete(cfg.Spec.VoiceAdapter.NodeVoices, rawNodeID)
		if nodeID == "" || voice == "" {
			continue
		}
		cfg.Spec.VoiceAdapter.NodeVoices[nodeID] = voice
	}
	return cfg, nil
}

func (c workflowConfig) validate() error {
	return c.Spec.VoiceAdapter.validate()
}

func (c voiceAdapterConfig) validate() error {
	if strings.TrimSpace(c.ASRModel) == "" {
		return fmt.Errorf("flowcraft: voice_adapter.asr_model is required")
	}
	if strings.TrimSpace(c.DefaultVoice) == "" {
		return fmt.Errorf("flowcraft: voice_adapter.default_voice is required")
	}
	return nil
}

func normalizeVoiceAdapter(cfg voiceAdapterConfig) voiceAdapterConfig {
	cfg.ASRModel = strings.TrimSpace(cfg.ASRModel)
	cfg.DefaultVoice = strings.TrimSpace(cfg.DefaultVoice)
	nodeVoices := make(map[string]string, len(cfg.NodeVoices))
	for nodeID, voice := range cfg.NodeVoices {
		nodeID = strings.TrimSpace(nodeID)
		voice = strings.TrimSpace(voice)
		if nodeID != "" && voice != "" {
			nodeVoices[nodeID] = voice
		}
	}
	cfg.NodeVoices = nodeVoices
	return cfg
}

func flowcraftConversationStarts(cfg map[string]any) string {
	if conversation, ok := cfg["conversation"].(map[string]any); ok {
		if starts, ok := conversation["starts"].(string); ok && strings.TrimSpace(starts) != "" {
			return strings.TrimSpace(starts)
		}
	}
	return "peer"
}

func flowcraftWorkspaceParameters(parameters *apitypes.WorkspaceParameters) (*apitypes.FlowcraftWorkspaceParameters, error) {
	if parameters == nil {
		return nil, nil
	}
	agentType, err := parameters.Discriminator()
	if err != nil {
		return nil, fmt.Errorf("flowcraft: decode workspace parameters: %w", err)
	}
	if strings.TrimSpace(agentType) != Type {
		return nil, fmt.Errorf("flowcraft: decode workspace parameters: agent_type %q does not match %q", agentType, Type)
	}
	typed, err := parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		return nil, fmt.Errorf("flowcraft: decode workspace parameters: %w", err)
	}
	return &typed, nil
}

func flowcraftConversationSettings(parameters *apitypes.FlowcraftWorkspaceParameters, cfg map[string]any) (string, string) {
	starts := flowcraftConversationStarts(cfg)
	policy := flowcraftDefaultAgentInitiativePolicy(starts)
	if parameters == nil || parameters.Conversation == nil {
		return starts, policy
	}
	if parameters.Conversation.Initiative != nil {
		switch strings.ToLower(strings.TrimSpace(string(*parameters.Conversation.Initiative))) {
		case "agent", "self":
			starts = "self"
			policy = flowcraftDefaultAgentInitiativePolicy(starts)
		case "peer", "user":
			starts = "peer"
			policy = flowcraftDefaultAgentInitiativePolicy(starts)
		}
	}
	if parameters.Conversation.AgentInitiativePolicy != nil {
		switch strings.ToLower(strings.TrimSpace(string(*parameters.Conversation.AgentInitiativePolicy))) {
		case "on_reload", "once_when_empty":
			policy = strings.ToLower(strings.TrimSpace(string(*parameters.Conversation.AgentInitiativePolicy)))
		}
	}
	return starts, policy
}

func flowcraftDefaultAgentInitiativePolicy(starts string) string {
	if strings.EqualFold(strings.TrimSpace(starts), "self") {
		return "on_reload"
	}
	return "once_when_empty"
}

type agent struct {
	transformers   transformerProvider
	runtime        runtimeClient
	history        historyReader
	outputObserver outputObserver
	historyID      string
	asrModel       string
	defaultVoice   string
	nodeVoices     map[string]string
	starts         string
	startPolicy    string
	inputMode      inputMode
	localDir       string
	inputProvider  InputProvider
	memory         memory.Store

	outputMu    sync.Mutex
	outputs     map[*genx.StreamBuilder]*flowcraftOutputState
	attachments map[string]*flowcraftOutputState

	selfStartMu sync.Mutex
	selfStarted bool
}

func (a *agent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	var runtimeHistoryAvailable bool
	if a != nil {
		_, runtimeHistoryAvailable = a.runtime.(historyReader)
	}
	historyAvailable := a != nil && (a.history != nil || runtimeHistoryAvailable)
	memoryAvailable := a != nil && a.memory != nil
	return apitypes.PeerRunWorkspaceState{
		RuntimeState:         apitypes.PeerRunStatusStateRunning,
		HistoryAvailable:     &historyAvailable,
		MemoryStatsAvailable: &memoryAvailable,
		RecallAvailable:      &memoryAvailable,
	}, nil
}

func (a *agent) ListHistory(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	messages, err := a.readHistory(ctx)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryListResponse{
			Available: false,
			Items:     []apitypes.PeerRunHistoryEntry{},
			HasNext:   false,
			Message:   &message,
		}, nil
	}
	offset, err := parseHistoryCursor(req.Cursor)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	limit := 50
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset > len(messages) {
		offset = len(messages)
	}
	remaining := len(messages) - offset
	if limit > remaining {
		limit = remaining
	}
	end := offset + limit
	items := make([]apitypes.PeerRunHistoryEntry, 0)
	now := time.Now().UTC()
	for i := offset; i < end; i++ {
		items = append(items, historyEntryFromMessage(a.historyID, i, now, messages[i]))
	}
	resp := apitypes.PeerRunHistoryListResponse{
		Available: true,
		Items:     items,
		HasNext:   end < len(messages),
	}
	if resp.HasNext {
		next := strconv.Itoa(end)
		resp.NextCursor = &next
	}
	if len(messages) == 0 {
		message := "flowcraft history is empty"
		resp.Message = &message
	}
	return resp, nil
}

func (a *agent) PlayHistory(ctx context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	messages, err := a.readHistory(ctx)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{
			Accepted:  false,
			HistoryId: req.HistoryId,
			State:     "unavailable",
			Message:   &message,
		}, nil
	}
	msg, ok := historyMessageByID(a.historyID, req.HistoryId, messages)
	if !ok {
		message := "history entry not found"
		return apitypes.PeerRunHistoryPlayResponse{
			Accepted:  false,
			HistoryId: req.HistoryId,
			State:     "not_found",
			Message:   &message,
		}, nil
	}
	text := strings.TrimSpace(msg.Content())
	if text == "" {
		message := "history entry has no text to replay"
		return apitypes.PeerRunHistoryPlayResponse{
			Accepted:  false,
			HistoryId: req.HistoryId,
			State:     "empty",
			Message:   &message,
		}, nil
	}
	output, streamID, epoch, ok := a.beginReplayOutput(ctx)
	if !ok {
		message := "flowcraft history replay requires an active peer output stream"
		return apitypes.PeerRunHistoryPlayResponse{
			Accepted:  false,
			HistoryId: req.HistoryId,
			State:     "unavailable",
			Message:   &message,
		}, nil
	}
	if msg.Role == flowmodel.RoleUser {
		if err := a.addOutput(output, epoch,
			textChunk(genx.RoleUser, transcriptLabel, streamID, transcriptLabel, text, false),
			textChunk(genx.RoleUser, transcriptLabel, streamID, transcriptLabel, "", true),
		); err != nil {
			message := err.Error()
			return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
		}
		return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: req.HistoryId, State: "played"}, nil
	}
	nodeID := "answer"
	if err := a.addOutput(output, epoch,
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, text, false),
		textChunk(genx.RoleModel, assistantLabel, streamID, assistantLabel, "", true),
	); err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
	}
	voice, ok := a.voiceForNode(nodeID)
	if !ok {
		return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: req.HistoryId, State: "played"}, nil
	}
	if err := a.synthesize(ctx, streamID, nodeID, voice, text, output, epoch); err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "audio_failed", Message: &message}, nil
	}
	return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: req.HistoryId, State: "played"}, nil
}

func (a *agent) MemoryStats(ctx context.Context, _ apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	if a == nil || a.memory == nil {
		message := "flowcraft memory store is not configured"
		return apitypes.PeerRunMemoryStatsResponse{
			Available: false,
			Enabled:   false,
			Message:   &message,
		}, nil
	}
	_ = ctx
	metadata := map[string]any{
		"capabilities": []string{"observe", "recall", "update", "delete"},
	}
	backend := "memory.Store"
	indexStatus := "available"
	resp := apitypes.PeerRunMemoryStatsResponse{
		Available:   true,
		Enabled:     true,
		Backend:     &backend,
		IndexStatus: &indexStatus,
		Metadata:    &metadata,
	}
	return resp, nil
}

func (a *agent) Recall(ctx context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	if a == nil || a.memory == nil {
		message := "flowcraft memory store is not configured"
		return apitypes.PeerRunRecallResponse{Available: false, Hits: []apitypes.PeerRunRecallHit{}, Message: &message}, nil
	}
	limit := 10
	if req.Limit != nil {
		limit = *req.Limit
	}
	filters := make([]memory.Filter, 0)
	if req.Filters != nil {
		for key, value := range *req.Filters {
			filters = append(filters, memory.Filter{Field: key, Operator: memory.FilterEqual, Value: value})
		}
	}
	slices.SortFunc(filters, func(left, right memory.Filter) int { return strings.Compare(left.Field, right.Field) })
	result, err := a.memory.Recall(ctx, memory.Query{Text: req.Query, Limit: limit, Filters: filters})
	if err != nil {
		return apitypes.PeerRunRecallResponse{}, fmt.Errorf("flowcraft: recall memory: %w", err)
	}
	hits := make([]apitypes.PeerRunRecallHit, 0, len(result.Matches))
	for index, match := range result.Matches {
		hits = append(hits, recallHitFromMemory(index, match))
	}
	return apitypes.PeerRunRecallResponse{Available: true, Hits: hits}, nil
}

type transformerProvider interface {
	Transformer() genx.Transformer
}

type runtimeClient interface {
	RoundTrip(runtimeRequest) (runtimeResponse, error)
	CloseContext(context.Context) error
}

type historyReader interface {
	History(context.Context, int) ([]flowmodel.Message, error)
}

type outputObserver interface {
	BeginOutput(streamID, user string)
	ObserveOutput(*genx.MessageChunk)
	InterruptOutput(streamID string)
}

type runtimeResponse interface {
	Next() (runtimeEvent, error)
}

type runtimeRequest struct {
	Context context.Context
	Text    string
	Inputs  map[string]any
}

type runtimeEvent struct {
	Type    string
	NodeID  string
	Content string
	Err     string
	IsError bool
}

const (
	runtimeEventToken = "token"
	runtimeEventError = "error"
)

type ownedRuntime struct {
	agent *ownedflowcraft.Agent
}

func (c ownedRuntime) RoundTrip(req runtimeRequest) (runtimeResponse, error) {
	if c.agent == nil {
		return nil, fmt.Errorf("flowcraft: nil owned runtime")
	}
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	input := &sliceStream{chunks: []*genx.MessageChunk{
		genx.NewBeginOfStream(genx.NewStreamID()),
		{Role: genx.RoleUser, Part: genx.Text(req.Text)},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{EndOfStream: true}},
	}}
	output, err := c.agent.Transform(ctx, "", input)
	if err != nil {
		return nil, err
	}
	return &ownedRuntimeResponse{stream: output}, nil
}

func (ownedRuntime) CloseContext(context.Context) error { return nil }

func (c ownedRuntime) History(ctx context.Context, limit int) ([]flowmodel.Message, error) {
	if c.agent == nil {
		return nil, fmt.Errorf("flowcraft: nil owned runtime")
	}
	return c.agent.History(ctx, limit)
}

type ownedRuntimeResponse struct{ stream genx.Stream }

func (r *ownedRuntimeResponse) Next() (runtimeEvent, error) {
	for {
		chunk, err := r.stream.Next()
		if commonagent.IsStreamEnd(err) {
			return runtimeEvent{}, io.EOF
		}
		if err != nil {
			return runtimeEvent{}, err
		}
		if chunk == nil || chunk.Role != genx.RoleModel {
			continue
		}
		if chunk.IsEndOfStream() {
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				return runtimeEvent{Type: runtimeEventError, Err: chunk.Ctrl.Error, IsError: true}, nil
			}
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok || text == "" {
			continue
		}
		return runtimeEvent{Type: runtimeEventToken, NodeID: chunk.Name, Content: string(text)}, nil
	}
}

func (a *agent) readHistory(ctx context.Context) ([]flowmodel.Message, error) {
	reader := a.history
	if reader == nil {
		reader, _ = a.runtime.(historyReader)
	}
	if reader == nil {
		return nil, fmt.Errorf("flowcraft history is not configured")
	}
	return reader.History(ctx, logstore.MaxLimit)
}

func (a *agent) resolveWorkspacePath(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return a.localDir
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}
	return filepath.Join(a.localDir, filepath.Clean(root))
}

func parseHistoryCursor(cursor *string) (int, error) {
	if cursor == nil || strings.TrimSpace(*cursor) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(strings.TrimSpace(*cursor))
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("flowcraft: invalid history cursor %q", *cursor)
	}
	return offset, nil
}

func historyEntryFromMessage(contextID string, index int, createdAt time.Time, msg flowmodel.Message) apitypes.PeerRunHistoryEntry {
	text := strings.TrimSpace(msg.Content())
	entryType := apitypes.PeerRunHistoryEntryTypeAgent
	name := "agent"
	entry := apitypes.PeerRunHistoryEntry{
		CreatedAt:       createdAt,
		Id:              historyEntryID(contextID, index),
		Name:            name,
		ReplayAvailable: text != "",
		Text:            text,
		Type:            entryType,
	}
	if msg.Role == flowmodel.RoleUser {
		gearID := "flowcraft"
		entry.Type = apitypes.PeerRunHistoryEntryTypeGear
		entry.GearId = &gearID
		entry.Name = "gear"
	}
	return entry
}

func historyMessageByID(contextID, historyID string, messages []flowmodel.Message) (flowmodel.Message, bool) {
	for i, msg := range messages {
		if historyEntryID(contextID, i) == historyID {
			return msg, true
		}
	}
	return flowmodel.Message{}, false
}

func historyEntryID(contextID string, index int) string {
	return fmt.Sprintf("%s:%06d", contextIDOrDefault(contextID), index)
}

func contextIDOrDefault(contextID string) string {
	contextID = strings.TrimSpace(contextID)
	if contextID == "" {
		return "default"
	}
	return contextID
}

func recallHitFromMemory(index int, match memory.Match) apitypes.PeerRunRecallHit {
	id := strings.TrimSpace(match.Fact.ID)
	if id == "" {
		id = fmt.Sprintf("hit-%06d", index)
	}
	metadata := make(map[string]any, len(match.Fact.Attributes)+1)
	maps.Copy(metadata, match.Fact.Attributes)
	if match.Fact.Revision != "" {
		metadata["revision"] = match.Fact.Revision
	}
	var sourceID *string
	if len(match.Fact.Sources) > 0 {
		value := strings.TrimSpace(match.Fact.Sources[0].ObservationID)
		if value != "" {
			sourceID = &value
		}
	}
	sourceType := "memory"
	var createdAt *time.Time
	if !match.Fact.CreatedAt.IsZero() {
		value := match.Fact.CreatedAt
		createdAt = &value
	}
	return apitypes.PeerRunRecallHit{
		Id: id, Score: match.Score, Snippet: match.Fact.Text, SourceId: sourceID,
		SourceType: &sourceType, CreatedAt: createdAt, Metadata: &metadata,
	}
}

type directoryStats struct {
	StorageBytes   int64
	FileCount      int64
	JSONLLineCount int64
	LastUpdatedAt  time.Time
}

func inspectDirectoryStats(root string) (directoryStats, error) {
	var stats directoryStats
	if strings.TrimSpace(root) == "" {
		return stats, nil
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stats.FileCount++
		stats.StorageBytes += info.Size()
		if info.ModTime().After(stats.LastUpdatedAt) {
			stats.LastUpdatedAt = info.ModTime().UTC()
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		lines, err := countFileLines(path)
		if err != nil {
			return err
		}
		stats.JSONLLineCount += lines
		return nil
	})
	if err != nil {
		return directoryStats{}, err
	}
	return stats, nil
}

func countFileLines(path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var lines int64
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			lines++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return lines, nil
}

func (a *agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if a == nil {
		return nil, fmt.Errorf("flowcraft: agent is nil")
	}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	observations := newFlowcraftOutputObservations(a.outputObserver)
	a.registerOutput(ctx, output, observations)
	go a.run(ctx, input, output, observations)
	return &flowcraftOutputStream{Stream: output.Stream(), observations: observations}, nil
}

func (a *agent) run(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, observations *flowcraftOutputObservations) {
	defer func() {
		a.clearActiveOutput(output)
		if a.runtime != nil {
			_ = a.runtime.CloseContext(context.Background())
		}
	}()

	current := a.startSelfTurnIfNeeded(ctx, output, observations)
	if a.inputMode == inputModeRealtime {
		a.runRealtime(ctx, input, output, current, observations)
		return
	}

	readerCtx, cancelReader := context.WithCancel(ctx)
	defer cancelReader()
	turns := make(chan flowcraftInputTurn, 4)
	readerDone := make(chan error, 1)
	go func() {
		readerDone <- a.readInputTurns(readerCtx, input, turns)
	}()

	inputDone := false
	var inputErr error
	var completed *flowcraftActiveTurn
	startTurn := func(turn flowcraftInputTurn) {
		if completed != nil {
			a.finishCompletedOutput(output, completed, observations)
			completed = nil
		}
		current = a.startFlowcraftTurn(ctx, output, turn, observations)
	}
	for {
		if current == nil && inputDone {
			select {
			case turn, ok := <-turns:
				if ok {
					startTurn(turn)
					continue
				}
			default:
			}
			if inputErr != nil && !isFlowcraftInputDone(inputErr) && !errors.Is(inputErr, context.Canceled) {
				_ = output.Unexpected(genx.Usage{}, inputErr)
			} else {
				_ = output.Done(genx.Usage{})
			}
			return
		}

		if current == nil {
			select {
			case turn, ok := <-turns:
				if !ok {
					inputDone = true
					continue
				}
				startTurn(turn)
			case err := <-readerDone:
				inputDone = true
				inputErr = err
				readerDone = nil
			case <-ctx.Done():
				_ = output.Unexpected(genx.Usage{}, ctx.Err())
				return
			}
			continue
		}

		select {
		case turn, ok := <-turns:
			if !ok {
				inputDone = true
				continue
			}
			current.cancel()
			_ = a.interruptOutput(output, current.streamID, current.epoch)
			completed = nil
			current = a.startFlowcraftTurn(ctx, output, turn, observations)
		case err := <-current.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				if isFlowcraftInputDone(err) {
					current = nil
					continue
				}
				_ = output.Unexpected(genx.Usage{}, err)
				return
			}
			completed = current
			current = nil
		case err := <-readerDone:
			inputDone = true
			inputErr = err
			readerDone = nil
		case <-ctx.Done():
			current.cancel()
			_ = output.Unexpected(genx.Usage{}, ctx.Err())
			return
		}
	}
}

type flowcraftInputTurn struct {
	streamID string
	stream   genx.Stream
}

type flowcraftActiveTurn struct {
	streamID string
	epoch    uint64
	cancel   context.CancelFunc
	done     <-chan error
}

type flowcraftOutputStream struct {
	genx.Stream
	observations        *flowcraftOutputObservations
	observationDeferred atomic.Bool
}

var _ agenthost.OutputObservationStream = (*flowcraftOutputStream)(nil)

func (s *flowcraftOutputStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if chunk != nil && !s.observationDeferred.Load() {
		s.ObserveOutput(chunk)
	}
	return chunk, err
}

// DeferOutputObservation asks the final consumer to acknowledge output after
// any downstream buffering or audio drain completes.
func (s *flowcraftOutputStream) DeferOutputObservation() {
	s.observationDeferred.Store(true)
}

// ObserveOutput records a chunk acknowledged by the final consumer.
func (s *flowcraftOutputStream) ObserveOutput(chunk *genx.MessageChunk) {
	if s != nil && s.observations != nil {
		s.observations.observe(chunk)
	}
}

type flowcraftOutputObservations struct {
	mu       sync.Mutex
	streams  map[string]flowcraftOutputObservation
	observer outputObserver
}

type flowcraftOutputObservation struct {
	produced bool
	audio    bool
	drained  bool
}

func newFlowcraftOutputObservations(observers ...outputObserver) *flowcraftOutputObservations {
	result := &flowcraftOutputObservations{streams: make(map[string]flowcraftOutputObservation)}
	if len(observers) > 0 {
		result.observer = observers[0]
	}
	return result
}

func (o *flowcraftOutputObservations) begin(streamID string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.streams[streamID] = flowcraftOutputObservation{}
	o.mu.Unlock()
}

func (o *flowcraftOutputObservations) produce(chunks ...*genx.MessageChunk) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, chunk := range chunks {
		if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil ||
			chunk.Ctrl.Label != assistantLabel || chunk.IsEndOfStream() {
			continue
		}
		state := o.streams[chunk.Ctrl.StreamID]
		state.produced = true
		o.streams[chunk.Ctrl.StreamID] = state
	}
}

func (o *flowcraftOutputObservations) observe(chunk *genx.MessageChunk) {
	if o == nil || chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil ||
		chunk.Ctrl.Label != assistantLabel {
		return
	}
	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	o.mu.Lock()
	state := o.streams[streamID]
	if chunk.IsEndOfStream() && strings.TrimSpace(chunk.Ctrl.Error) != "" {
		delete(o.streams, streamID)
		o.mu.Unlock()
		o.observeHistory(chunk)
		return
	}
	if !chunk.IsEndOfStream() {
		state.drained = false
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			state.audio = true
		}
		o.streams[streamID] = state
		o.mu.Unlock()
		o.observeHistory(chunk)
		return
	}
	switch chunk.Part.(type) {
	case genx.Text:
		if !state.audio {
			state.drained = true
		}
	case *genx.Blob:
		state.drained = true
	}
	o.streams[streamID] = state
	o.mu.Unlock()
	o.observeHistory(chunk)
}

func (o *flowcraftOutputObservations) observeHistory(chunk *genx.MessageChunk) {
	if o != nil && o.observer != nil {
		o.observer.ObserveOutput(chunk)
	}
}

func (o *flowcraftOutputObservations) take(streamID string) flowcraftOutputObservation {
	if o == nil {
		return flowcraftOutputObservation{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	state := o.streams[streamID]
	delete(o.streams, streamID)
	return state
}

type flowcraftTranscriptTurn struct {
	streamID     string
	transcript   string
	historyAudio []*genx.MessageChunk
}

func (a *agent) startFlowcraftTurn(ctx context.Context, output *genx.StreamBuilder, turn flowcraftInputTurn, observations *flowcraftOutputObservations) *flowcraftActiveTurn {
	streamID := strings.TrimSpace(turn.streamID)
	if streamID == "" {
		streamID = genx.NewStreamID()
	}
	epoch := a.setActiveOutput(output, streamID, observations)
	observations.begin(streamID)
	turnCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- a.runTurn(turnCtx, turn.stream, output, epoch, streamID)
	}()
	return &flowcraftActiveTurn{
		streamID: streamID,
		epoch:    epoch,
		cancel:   cancel,
		done:     done,
	}
}

func (a *agent) startFlowcraftTranscriptTurn(ctx context.Context, output *genx.StreamBuilder, streamID, transcript string, emitTranscript bool, observations *flowcraftOutputObservations, historyAudio ...*genx.MessageChunk) *flowcraftActiveTurn {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = genx.NewStreamID()
	}
	epoch := a.setActiveOutput(output, streamID, observations)
	observations.begin(streamID)
	if len(historyAudio) > 0 {
		if err := a.addOutput(output, epoch, historyAudio...); err != nil {
			done := make(chan error, 1)
			done <- err
			return &flowcraftActiveTurn{streamID: streamID, epoch: epoch, cancel: func() {}, done: done}
		}
	}
	if emitTranscript {
		if err := a.addOutput(output, epoch,
			textChunk(genx.RoleUser, transcriptLabel, streamID, transcriptLabel, strings.TrimSpace(transcript), false),
			textChunk(genx.RoleUser, transcriptLabel, streamID, transcriptLabel, "", true),
		); err != nil {
			done := make(chan error, 1)
			done <- err
			return &flowcraftActiveTurn{streamID: streamID, epoch: epoch, cancel: func() {}, done: done}
		}
	}
	turnCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- a.runTranscriptTurn(turnCtx, transcript, streamID, output, epoch, false)
	}()
	return &flowcraftActiveTurn{
		streamID: streamID,
		epoch:    epoch,
		cancel:   cancel,
		done:     done,
	}
}

func (a *agent) startSelfTurnIfNeeded(ctx context.Context, output *genx.StreamBuilder, observations *flowcraftOutputObservations) *flowcraftActiveTurn {
	if !a.shouldSelfStart(ctx) {
		return nil
	}
	streamID := selfStartStreamID
	epoch := a.setActiveOutput(output, streamID, observations)
	observations.begin(streamID)
	turnCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- a.runFlowcraftTextTurn(turnCtx, "", streamID, output, epoch)
	}()
	return &flowcraftActiveTurn{
		streamID: streamID,
		epoch:    epoch,
		cancel:   cancel,
		done:     done,
	}
}

func (a *agent) shouldSelfStart(ctx context.Context) bool {
	if !strings.EqualFold(strings.TrimSpace(a.starts), "self") {
		return false
	}
	a.selfStartMu.Lock()
	if a.selfStarted {
		a.selfStartMu.Unlock()
		return false
	}
	a.selfStarted = true
	a.selfStartMu.Unlock()

	if strings.EqualFold(strings.TrimSpace(a.startPolicy), "on_reload") {
		return true
	}
	empty, err := a.historyEmpty(ctx)
	if err != nil {
		return true
	}
	return empty
}

func (a *agent) historyEmpty(ctx context.Context) (bool, error) {
	messages, err := a.readHistory(ctx)
	if err != nil {
		return false, err
	}
	return len(messages) == 0, nil
}

func (a *agent) runRealtime(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, current *flowcraftActiveTurn, observations *flowcraftOutputObservations) {
	transformer := a.transformers.Transformer()
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	asr, err := transformer.Transform(ctx, "model/"+a.asrModel+"?emit_interim=true", asrInput.Stream())
	if err != nil {
		_ = output.Unexpected(genx.Usage{}, fmt.Errorf("flowcraft: start ASR: %w", err))
		return
	}
	defer func() { _ = asr.Close() }()

	streamIDState := &lockedString{value: defaultInputStreamID}
	historyAudio := &realtimeHistoryAudioBuffer{}
	inputStarted := make(chan string, 4)
	feedDone := make(chan feedASRResult, 1)
	go func() {
		feedDone <- feedRealtimeASRInput(ctx, input, asrInput, streamIDState, inputStarted)
	}()

	asrResults := make(chan streamResult, 1)
	go readStreamResults(asr, asrResults)

	var asrDone bool
	var asrErr error
	var feedResult feedASRResult
	feedClosed := false
	realtimeTurnIndex := 0
	var pending []flowcraftTranscriptTurn
	var completed *flowcraftActiveTurn
	startPending := func() {
		if current != nil || len(pending) == 0 {
			return
		}
		turn := pending[0]
		pending = pending[1:]
		if completed != nil {
			a.finishCompletedOutput(output, completed, observations)
			completed = nil
		}
		current = a.startFlowcraftTranscriptTurn(ctx, output, turn.streamID, turn.transcript, true, observations, turn.historyAudio...)
	}
	queueTranscript := func(text string, asrStreamID string) {
		realtimeTurnIndex++
		streamID := realtimeTurnStreamID(streamIDState.Get(), realtimeTurnIndex)
		audio := historyAudio.drain(asrStreamID, streamID)
		turn := flowcraftTranscriptTurn{streamID: streamID, transcript: text, historyAudio: audio}
		if current != nil {
			current.cancel()
			_ = a.interruptOutput(output, current.streamID, current.epoch)
			current = nil
			pending = nil
		}
		pending = append(pending, turn)
		startPending()
	}
	interruptCurrent := func() {
		pending = nil
		if current == nil {
			if completed != nil {
				a.finishCompletedOutput(output, completed, observations)
				completed = nil
			}
			return
		}
		current.cancel()
		_ = a.interruptOutput(output, current.streamID, current.epoch)
		current = nil
		completed = nil
	}
	interruptForInput := func(streamID string) {
		if current == nil || !realtimeInputInterruptsCurrent(current.streamID, streamID) {
			return
		}
		interruptCurrent()
	}
	var asrTranscript string
	var asrTranscriptStreamID string
	asrTranscriptOpen := false
	failCurrent := func(err error) bool {
		if err == nil || isFlowcraftInputDone(err) || errors.Is(err, context.Canceled) {
			return false
		}
		if current != nil {
			current.cancel()
			current = nil
		}
		pending = nil
		_ = output.Unexpected(genx.Usage{}, err)
		return true
	}
	handleASRChunk := func(chunk *genx.MessageChunk) {
		if chunk == nil {
			return
		}
		asrStreamID := realtimeASRStreamID(chunk, streamIDState.Get())
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Label) == genx.HistoryUserAudioLabel {
			historyAudio.append(chunk, asrStreamID)
			return
		}
		if chunk.IsBeginOfStream() {
			interruptCurrent()
			asrTranscript = ""
			asrTranscriptStreamID = asrStreamID
			asrTranscriptOpen = true
			return
		}
		if chunk.IsEndOfStream() {
			if !asrTranscriptOpen {
				return
			}
			transcript := strings.TrimSpace(asrTranscript)
			asrTranscript = ""
			asrTranscriptOpen = false
			if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
				return
			}
			if transcript != "" {
				queueTranscript(transcript, asrTranscriptStreamID)
			}
			asrTranscriptStreamID = ""
			return
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok || strings.TrimSpace(string(text)) == "" {
			return
		}
		if asrTranscriptOpen {
			if asrTranscriptStreamID == "" {
				asrTranscriptStreamID = asrStreamID
			}
			asrTranscript = mergeTranscript(asrTranscript, string(text))
			return
		}
		queueTranscript(string(text), asrStreamID)
	}

	for {
		startPending()
		if current == nil && len(pending) == 0 && asrDone {
			if !feedClosed {
				feedResult = <-feedDone
				feedClosed = true
			}
			if feedResult.err != nil && !isFlowcraftInputDone(feedResult.err) && !errors.Is(feedResult.err, context.Canceled) {
				_ = output.Unexpected(genx.Usage{}, fmt.Errorf("flowcraft: feed ASR: %w", feedResult.err))
				return
			}
			if asrErr != nil && !isFlowcraftInputDone(asrErr) && !errors.Is(asrErr, context.Canceled) {
				_ = output.Unexpected(genx.Usage{}, fmt.Errorf("flowcraft: read ASR: %w", asrErr))
				return
			}
			_ = output.Done(genx.Usage{})
			return
		}

		if current == nil {
			select {
			case <-inputStarted:
				interruptCurrent()
				continue
			case result := <-asrResults:
				if result.err != nil {
					if failCurrent(fmt.Errorf("flowcraft: read ASR: %w", result.err)) {
						return
					}
					asrDone = true
					asrErr = result.err
					continue
				}
				if result.chunk == nil {
					continue
				}
				handleASRChunk(result.chunk)
			case feedResult = <-feedDone:
				feedClosed = true
				feedDone = nil
				if feedResult.err != nil {
					if failCurrent(fmt.Errorf("flowcraft: feed ASR: %w", feedResult.err)) {
						return
					}
				}
			case <-ctx.Done():
				_ = output.Unexpected(genx.Usage{}, ctx.Err())
				return
			}
			continue
		}

		select {
		case err := <-current.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				_ = output.Unexpected(genx.Usage{}, err)
				return
			}
			completed = current
			current = nil
			continue
		default:
		}

		select {
		case streamID := <-inputStarted:
			interruptForInput(streamID)
		case result := <-asrResults:
			if result.err != nil {
				if failCurrent(fmt.Errorf("flowcraft: read ASR: %w", result.err)) {
					return
				}
				asrDone = true
				asrErr = result.err
				continue
			}
			if result.chunk == nil {
				continue
			}
			handleASRChunk(result.chunk)
		case err := <-current.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				_ = output.Unexpected(genx.Usage{}, err)
				return
			}
			completed = current
			current = nil
		case feedResult = <-feedDone:
			feedClosed = true
			feedDone = nil
			if feedResult.err != nil {
				if failCurrent(fmt.Errorf("flowcraft: feed ASR: %w", feedResult.err)) {
					return
				}
			}
		case <-ctx.Done():
			current.cancel()
			_ = output.Unexpected(genx.Usage{}, ctx.Err())
			return
		}
	}
}

func (a *agent) readInputTurns(ctx context.Context, input genx.Stream, turns chan<- flowcraftInputTurn) error {
	defer close(turns)
	if input == nil {
		return fmt.Errorf("flowcraft: input stream is required")
	}

	type openTurn struct {
		streamID string
		input    *genx.StreamBuilder
	}
	var current *openTurn
	closeCurrent := func() {
		if current == nil {
			return
		}
		_ = current.input.Done(genx.Usage{})
		current = nil
	}
	startTurn := func(streamID string) error {
		closeCurrent()
		streamID = strings.TrimSpace(streamID)
		if streamID == "" {
			streamID = genx.NewStreamID()
		}
		builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
		turn := flowcraftInputTurn{streamID: streamID, stream: builder.Stream()}
		select {
		case <-ctx.Done():
			_ = builder.Unexpected(genx.Usage{}, ctx.Err())
			return ctx.Err()
		case turns <- turn:
		}
		current = &openTurn{streamID: streamID, input: builder}
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			closeCurrent()
			return err
		}
		chunk, err := input.Next()
		if err != nil {
			closeCurrent()
			if isFlowcraftInputDone(err) {
				return nil
			}
			return err
		}
		if chunk == nil {
			continue
		}

		if chunk.IsBeginOfStream() {
			streamID := chunkStreamID(chunk)
			if err := startTurn(streamID); err != nil {
				return err
			}
		}
		if current == nil {
			if isAudioChunk(chunk) {
				continue
			}
			if err := startTurn(chunkStreamID(chunk)); err != nil {
				return err
			}
		}
		turnChunk := cloneTurnChunk(chunk, current.streamID)
		if err := current.input.Add(turnChunk); err != nil {
			closeCurrent()
			return err
		}
		if chunk.IsEndOfStream() {
			closeCurrent()
		}
	}
}

func cloneTurnChunk(chunk *genx.MessageChunk, streamID string) *genx.MessageChunk {
	cloned := chunk.Clone()
	if cloned.Ctrl == nil {
		cloned.Ctrl = &genx.StreamCtrl{}
	}
	cloned.Ctrl.StreamID = streamID
	return cloned
}

func chunkStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}

func isAudioChunk(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok {
		return false
	}
	return isAudioMIME(blob.MIMEType)
}

func isFlowcraftInputDone(err error) bool {
	return errors.Is(err, io.EOF) || agenthost.IsStreamDone(err)
}

func (a *agent) runTurn(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, epoch uint64, defaultStreamID string) error {
	transcript, streamID, err := a.transcribeInputTurn(ctx, input, output, epoch, defaultStreamID)
	if err != nil {
		return err
	}
	return a.runTranscriptTurn(ctx, transcript, streamID, output, epoch, false)
}

func (a *agent) runTranscriptTurn(ctx context.Context, transcript, streamID string, output *genx.StreamBuilder, epoch uint64, emitTranscript bool) error {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return fmt.Errorf("flowcraft: ASR produced empty transcript")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = defaultInputStreamID
	}
	if emitTranscript {
		if err := a.addOutput(output, epoch,
			textChunk(genx.RoleUser, transcriptLabel, streamID, transcriptLabel, transcript, false),
			textChunk(genx.RoleUser, transcriptLabel, streamID, transcriptLabel, "", true),
		); err != nil {
			return err
		}
	}
	return a.runFlowcraftTextTurn(ctx, transcript, streamID, output, epoch)
}

func (a *agent) runFlowcraftTextTurn(ctx context.Context, text, streamID string, output *genx.StreamBuilder, epoch uint64) error {
	text = strings.TrimSpace(text)
	a.setActiveStreamID(output, streamID)
	if a.outputObserver != nil {
		a.outputObserver.BeginOutput(streamID, text)
	}

	var inputs map[string]any
	if a.inputProvider != nil {
		var err error
		inputs, err = a.inputProvider(ctx)
		if err != nil {
			return fmt.Errorf("flowcraft: provide turn inputs: %w", err)
		}
	}
	resp, err := a.runtime.RoundTrip(runtimeRequest{Context: ctx, Text: text, Inputs: inputs})
	if err != nil {
		return fmt.Errorf("flowcraft: owned runtime round trip: %w", err)
	}
	var currentNodeID string
	var tts *ttsSession
	emittedAudio := false
	sawToken := false
	closeTTS := func() error {
		if tts == nil {
			return nil
		}
		session := tts
		tts = nil
		if err := session.CloseInput(); err != nil {
			return err
		}
		if err := session.Wait(); err != nil {
			return err
		}
		emittedAudio = true
		return nil
	}
	addTTSText := func(nodeID, text string) error {
		if tts == nil {
			voice, ok := a.voiceForNode(nodeID)
			if !ok {
				return nil
			}
			session, err := a.startTTS(ctx, streamID, nodeID, voice, output, epoch)
			if err != nil {
				return err
			}
			tts = session
		}
		return tts.AddText(text)
	}
	defer func() { _ = closeTTS() }()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ev, err := resp.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("flowcraft: read owned runtime event: %w", err)
		}
		if ev.Type == runtimeEventError || ev.IsError {
			message := ev.Err
			if message == "" {
				message = ev.Content
			}
			if sawToken && isPartialResponseLimitError(message) {
				break
			}
			return fmt.Errorf("flowcraft: owned runtime event error: %s", message)
		}
		if ev.Type != "" && ev.Type != runtimeEventToken || ev.Content == "" {
			continue
		}
		sawToken = true
		nodeID := strings.TrimSpace(ev.NodeID)
		if nodeID == "" {
			nodeID = assistantLabel
		}
		if currentNodeID != "" && nodeID != currentNodeID {
			if err := closeTTS(); err != nil {
				return err
			}
		}
		currentNodeID = nodeID
		if err := a.addOutput(output, epoch, textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, ev.Content, false)); err != nil {
			return err
		}
		if err := addTTSText(nodeID, ev.Content); err != nil {
			return err
		}
	}
	if err := closeTTS(); err != nil {
		return err
	}
	if err := a.addOutput(output, epoch, textChunk(genx.RoleModel, assistantLabel, streamID, assistantLabel, "", true)); err != nil {
		return err
	}
	if emittedAudio {
		return a.addOutput(output, epoch, audioChunk(assistantLabel, streamID, nil, true))
	}
	return nil
}

func isPartialResponseLimitError(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "response incomplete") && strings.Contains(message, "length")
}

type streamResult struct {
	chunk *genx.MessageChunk
	err   error
}

func readStreamResults(stream genx.Stream, results chan<- streamResult) {
	for {
		chunk, err := stream.Next()
		results <- streamResult{chunk: chunk, err: err}
		if err != nil {
			return
		}
	}
}

const defaultFlowcraftOutputKey = "__default__"

type flowcraftOutputState struct {
	mu             sync.Mutex
	attachmentID   string
	output         *genx.StreamBuilder
	activeStreamID string
	observations   *flowcraftOutputObservations
	epoch          uint64
}

func flowcraftOutputKey(ctx context.Context) string {
	if id := strings.TrimSpace(agenthost.AttachmentID(ctx)); id != "" {
		return id
	}
	return defaultFlowcraftOutputKey
}

func (a *agent) registerOutput(ctx context.Context, output *genx.StreamBuilder, observations *flowcraftOutputObservations) *flowcraftOutputState {
	state := &flowcraftOutputState{
		attachmentID:   flowcraftOutputKey(ctx),
		output:         output,
		activeStreamID: defaultInputStreamID,
		observations:   observations,
	}
	a.outputMu.Lock()
	if a.outputs == nil {
		a.outputs = make(map[*genx.StreamBuilder]*flowcraftOutputState)
	}
	if a.attachments == nil {
		a.attachments = make(map[string]*flowcraftOutputState)
	}
	a.outputs[output] = state
	a.attachments[state.attachmentID] = state
	a.outputMu.Unlock()
	return state
}

func (a *agent) outputState(output *genx.StreamBuilder) *flowcraftOutputState {
	if a == nil || output == nil {
		return nil
	}
	a.outputMu.Lock()
	state := a.outputs[output]
	a.outputMu.Unlock()
	return state
}

func (a *agent) ensureOutputState(output *genx.StreamBuilder) *flowcraftOutputState {
	if state := a.outputState(output); state != nil {
		return state
	}
	return a.registerOutput(context.Background(), output, nil)
}

func (a *agent) setActiveOutput(output *genx.StreamBuilder, streamID string, observations ...*flowcraftOutputObservations) uint64 {
	state := a.ensureOutputState(output)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.epoch++
	state.activeStreamID = streamID
	state.observations = nil
	if len(observations) > 0 {
		state.observations = observations[0]
	}
	return state.epoch
}

func (a *agent) setActiveStreamID(output *genx.StreamBuilder, streamID string) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	state := a.outputState(output)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.activeStreamID = streamID
	state.mu.Unlock()
}

func (a *agent) clearActiveOutput(output *genx.StreamBuilder) {
	a.outputMu.Lock()
	state := a.outputs[output]
	delete(a.outputs, output)
	if state != nil && a.attachments[state.attachmentID] == state {
		delete(a.attachments, state.attachmentID)
	}
	a.outputMu.Unlock()
}

func (a *agent) beginReplayOutput(ctx context.Context) (*genx.StreamBuilder, string, uint64, bool) {
	a.outputMu.Lock()
	state := a.attachments[flowcraftOutputKey(ctx)]
	a.outputMu.Unlock()
	if state == nil || state.output == nil {
		return nil, "", 0, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.epoch++
	streamID := strings.TrimSpace(state.activeStreamID)
	if streamID == "" {
		streamID = defaultInputStreamID
	}
	return state.output, streamID, state.epoch, true
}

func (a *agent) currentOutputEpoch(output *genx.StreamBuilder) uint64 {
	state := a.outputState(output)
	if state == nil {
		state = a.registerOutput(context.Background(), output, nil)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.epoch
}

func (a *agent) addOutput(output *genx.StreamBuilder, epoch uint64, chunks ...*genx.MessageChunk) error {
	state := a.outputState(output)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.epoch != epoch {
		return nil
	}
	state.observations.produce(chunks...)
	return output.Add(chunks...)
}

func (a *agent) watchInputInterrupt(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, streamID string, epoch uint64, cancel func()) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		chunk, err := input.Next()
		if err != nil {
			return
		}
		if chunk == nil || !chunk.IsBeginOfStream() {
			continue
		}
		if a.interruptOutput(output, streamID, epoch) && cancel != nil {
			cancel()
		}
		return
	}
}

func (a *agent) interruptOutput(output *genx.StreamBuilder, streamID string, epoch uint64) bool {
	if output == nil {
		return false
	}
	if strings.TrimSpace(streamID) == "" {
		streamID = defaultInputStreamID
	}
	state := a.outputState(output)
	if state == nil {
		return false
	}
	state.mu.Lock()
	if state.epoch != epoch {
		state.mu.Unlock()
		return false
	}
	state.epoch++
	state.mu.Unlock()
	if a.outputObserver != nil {
		a.outputObserver.InterruptOutput(streamID)
	}
	a.discardAssistantOutput(output, streamID)
	return addInterruptedOutputEOS(output, streamID)
}

func (a *agent) interruptQueuedOutput(output *genx.StreamBuilder, streamID string, epoch uint64) bool {
	if output == nil {
		return false
	}
	if strings.TrimSpace(streamID) == "" {
		streamID = defaultInputStreamID
	}
	state := a.outputState(output)
	if state == nil {
		return false
	}
	state.mu.Lock()
	if state.epoch != epoch {
		state.mu.Unlock()
		return false
	}
	a.discardAssistantOutput(output, streamID)
	state.epoch++
	state.mu.Unlock()
	if a.outputObserver != nil {
		a.outputObserver.InterruptOutput(streamID)
	}
	return addInterruptedOutputEOS(output, streamID)
}

func (a *agent) finishCompletedOutput(output *genx.StreamBuilder, completed *flowcraftActiveTurn, observations *flowcraftOutputObservations) {
	if completed == nil {
		return
	}
	state := observations.take(completed.streamID)
	if !state.produced || state.drained {
		return
	}
	_ = a.interruptQueuedOutput(output, completed.streamID, completed.epoch)
}

func (a *agent) discardAssistantOutput(output *genx.StreamBuilder, streamID string) int {
	return output.Discard(func(chunk *genx.MessageChunk) bool {
		return chunk != nil && chunk.Role == genx.RoleModel && chunk.Ctrl != nil &&
			chunk.Ctrl.StreamID == streamID && chunk.Ctrl.Label == assistantLabel
	})
}

func addInterruptedOutputEOS(output *genx.StreamBuilder, streamID string) bool {
	textEOS := textChunk(genx.RoleModel, assistantLabel, streamID, assistantLabel, "", true)
	audioEOS := audioChunk(assistantLabel, streamID, nil, true)
	textEOS.Ctrl.Error = interruptedError
	audioEOS.Ctrl.Error = interruptedError
	return output.Add(textEOS, audioEOS) == nil
}

func (a *agent) transcribeInputTurn(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, epoch uint64, defaultStreamID string) (string, string, error) {
	prefetched, err := readInputTurn(ctx, input, defaultStreamID)
	turnStreamID := strings.TrimSpace(prefetched.streamID)
	if turnStreamID == "" {
		turnStreamID = defaultInputStreamID
	}
	if err != nil {
		return "", turnStreamID, err
	}
	if prefetched.hasText && !prefetched.hasAudio {
		if err := a.addOutput(output, epoch,
			textChunk(genx.RoleUser, transcriptLabel, turnStreamID, transcriptLabel, prefetched.transcript, false),
			textChunk(genx.RoleUser, transcriptLabel, turnStreamID, transcriptLabel, "", true),
		); err != nil {
			return "", turnStreamID, err
		}
		return prefetched.transcript, turnStreamID, nil
	}
	input = &sliceStream{chunks: prefetched.chunks}
	transformer := a.transformers.Transformer()
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	asr, err := transformer.Transform(ctx, "model/"+a.asrModel, asrInput.Stream())
	if err != nil {
		return "", turnStreamID, fmt.Errorf("flowcraft: start ASR: %w", err)
	}
	defer func() { _ = asr.Close() }()

	feedDone := make(chan feedASRResult, 1)
	go func() {
		emitHistoryAudio := func(chunk *genx.MessageChunk) error {
			return a.addOutput(output, epoch, userAudioHistoryChunk(chunk, turnStreamID))
		}
		result := feedASRInput(ctx, input, asrInput, turnStreamID, emitHistoryAudio)
		feedDone <- result
	}()

	var transcript string
	transcriptEOS := false
	for {
		chunk, err := asr.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) {
				break
			}
			result := <-feedDone
			if result.err != nil {
				return "", turnStreamID, result.err
			}
			return "", turnStreamID, fmt.Errorf("flowcraft: read ASR: %w", err)
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Label) == genx.HistoryUserAudioLabel {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if chunk.IsEndOfStream() {
			transcriptEOS = true
			if text != "" {
				part := string(text)
				transcript = mergeTranscript(transcript, part)
				if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, turnStreamID, transcriptLabel, part, false)); err != nil {
					return "", turnStreamID, err
				}
			}
			if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, turnStreamID, transcriptLabel, "", true)); err != nil {
				return "", turnStreamID, err
			}
			continue
		}
		if !ok || text == "" {
			continue
		}
		part := string(text)
		transcript = mergeTranscript(transcript, part)
		if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, turnStreamID, transcriptLabel, part, false)); err != nil {
			return "", turnStreamID, err
		}
	}
	result := <-feedDone
	if result.err != nil {
		return "", turnStreamID, result.err
	}
	if !transcriptEOS {
		if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, turnStreamID, transcriptLabel, "", true)); err != nil {
			return "", turnStreamID, err
		}
	}
	return transcript, turnStreamID, nil
}

type prefetchedInputTurn struct {
	chunks     []*genx.MessageChunk
	transcript string
	streamID   string
	hasText    bool
	hasAudio   bool
}

func readInputTurn(ctx context.Context, input genx.Stream, defaultStreamID string) (prefetchedInputTurn, error) {
	turn := prefetchedInputTurn{streamID: strings.TrimSpace(defaultStreamID)}
	if turn.streamID == "" {
		turn.streamID = defaultInputStreamID
	}
	if input == nil {
		return turn, fmt.Errorf("flowcraft: input stream is required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return turn, err
		}
		chunk, err := input.Next()
		if err != nil {
			if isFlowcraftInputDone(err) {
				return turn, nil
			}
			return turn, err
		}
		if chunk == nil {
			continue
		}
		cloned := chunk.Clone()
		turn.chunks = append(turn.chunks, cloned)
		if cloned.Ctrl != nil && strings.TrimSpace(cloned.Ctrl.StreamID) != "" {
			turn.streamID = strings.TrimSpace(cloned.Ctrl.StreamID)
		}
		if text, ok := cloned.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" {
			turn.hasText = true
			turn.transcript = mergeTranscript(turn.transcript, string(text))
		}
		if blob, ok := cloned.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) && len(blob.Data) > 0 {
			turn.hasAudio = true
		}
	}
}

func (a *agent) synthesize(ctx context.Context, streamID, nodeID, voice, text string, output *genx.StreamBuilder, epoch uint64) error {
	transformer := a.transformers.Transformer()
	input := []*genx.MessageChunk{
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, text, false),
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, "", true),
	}
	tts, err := transformer.Transform(ctx, "voice/"+voice, &sliceStream{chunks: input})
	if err != nil {
		return fmt.Errorf("flowcraft: start TTS voice %q: %w", voice, err)
	}
	defer func() { _ = tts.Close() }()
	return a.drainTTSOutput(ctx, streamID, nodeID, voice, tts, output, epoch, true)
}

func (a *agent) synthesizeTextSegment(ctx context.Context, streamID, nodeID, voice, text string, output *genx.StreamBuilder, epoch uint64, emitEOS bool) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	transformer := a.transformers.Transformer()
	input := []*genx.MessageChunk{
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, text, false),
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, "", true),
	}
	tts, err := transformer.Transform(ctx, "voice/"+voice, &sliceStream{chunks: input})
	if err != nil {
		return fmt.Errorf("flowcraft: start TTS voice %q: %w", voice, err)
	}
	defer func() { _ = tts.Close() }()
	return a.drainTTSOutput(ctx, streamID, nodeID, voice, tts, output, epoch, emitEOS)
}

type ttsSession struct {
	input    *genx.StreamBuilder
	done     chan error
	streamID string
	nodeID   string
}

func (a *agent) startTTS(ctx context.Context, streamID, nodeID, voice string, output *genx.StreamBuilder, epoch uint64) (*ttsSession, error) {
	transformer := a.transformers.Transformer()
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	tts, err := transformer.Transform(ctx, "voice/"+voice, input.Stream())
	if err != nil {
		return nil, fmt.Errorf("flowcraft: start TTS voice %q: %w", voice, err)
	}
	session := &ttsSession{
		input:    input,
		done:     make(chan error, 1),
		streamID: streamID,
		nodeID:   nodeID,
	}
	go func() {
		defer func() { _ = tts.Close() }()
		session.done <- a.drainTTSOutput(ctx, streamID, nodeID, voice, tts, output, epoch, true)
	}()
	return session, nil
}

func (s *ttsSession) AddText(text string) error {
	if s == nil {
		return fmt.Errorf("flowcraft: TTS session is nil")
	}
	return s.input.Add(textChunk(genx.RoleModel, s.nodeID, s.streamID, assistantLabel, text, false))
}

func (s *ttsSession) CloseInput() error {
	if s == nil {
		return nil
	}
	if err := s.input.Add(textChunk(genx.RoleModel, s.nodeID, s.streamID, assistantLabel, "", true)); err != nil {
		return err
	}
	return s.input.Done(genx.Usage{})
}

func (s *ttsSession) Wait() error {
	if s == nil {
		return nil
	}
	return <-s.done
}

func (a *agent) drainTTSOutput(ctx context.Context, streamID, nodeID, voice string, tts genx.Stream, output *genx.StreamBuilder, epoch uint64, emitEOS bool) error {
	oggDecoder := newOggOpusFrameDecoder()
	audioStarted := false
	emitFrame := func(frame []byte) error {
		chunks := make([]*genx.MessageChunk, 0, 2)
		if !audioStarted {
			bos := audioChunk(nodeID, streamID, nil, false)
			bos.Ctrl.BeginOfStream = true
			chunks = append(chunks, bos)
			audioStarted = true
		}
		chunks = append(chunks, audioChunk(nodeID, streamID, frame, false))
		return a.addOutput(output, epoch, chunks...)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := tts.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) {
				break
			}
			return fmt.Errorf("flowcraft: read TTS voice %q: %w", voice, err)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			continue
		}
		switch baseMIME(blob.MIMEType) {
		case "audio/opus":
			if err := emitFrame(blob.Data); err != nil {
				return err
			}
		case "audio/ogg", "application/ogg":
			frames, err := oggDecoder.Write(blob.Data)
			if err != nil {
				return fmt.Errorf("flowcraft: decode TTS ogg opus: %w", err)
			}
			for _, frame := range frames {
				if err := emitFrame(frame); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("flowcraft: unsupported TTS audio MIME %q; want audio/ogg or audio/opus", blob.MIMEType)
		}
	}
	if err := oggDecoder.Close(); err != nil {
		return fmt.Errorf("flowcraft: decode TTS ogg opus: %w", err)
	}
	if !emitEOS {
		return nil
	}
	return a.addOutput(output, epoch, audioChunk(nodeID, streamID, nil, true))
}

func (a *agent) voiceForNode(nodeID string) (string, bool) {
	if a.nodeVoices != nil {
		if voice := strings.TrimSpace(a.nodeVoices[nodeID]); voice != "" {
			return voice, true
		}
		if len(a.nodeVoices) > 0 {
			return "", false
		}
	}
	voice := strings.TrimSpace(a.defaultVoice)
	return voice, voice != ""
}

type feedASRResult struct {
	streamID string
	err      error
}

type historyAudioEmitter func(*genx.MessageChunk) error

func feedASRInput(ctx context.Context, input genx.Stream, asrInput *genx.StreamBuilder, streamID string, emitHistoryAudio historyAudioEmitter) feedASRResult {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = defaultInputStreamID
	}
	audioSeen := false
	lastAudioMIME := "audio/pcm"
	fail := func(err error) feedASRResult {
		_ = asrInput.Unexpected(genx.Usage{}, err)
		return feedASRResult{streamID: streamID, err: err}
	}
	if input == nil {
		return fail(fmt.Errorf("flowcraft: input stream is required"))
	}

	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		chunk, err := input.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) || errors.Is(err, io.EOF) {
				if err := asrInput.Done(genx.Usage{}); err != nil {
					return feedASRResult{streamID: streamID, err: err}
				}
				if !audioSeen {
					return feedASRResult{streamID: streamID, err: io.EOF}
				}
				if emitHistoryAudio != nil {
					if err := emitHistoryAudio(userAudioHistoryEOSChunk(streamID, lastAudioMIME)); err != nil {
						return feedASRResult{streamID: streamID, err: err}
					}
				}
				return feedASRResult{streamID: streamID}
			}
			return fail(err)
		}
		if chunk == nil {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) && len(blob.Data) > 0 {
			audioSeen = true
			lastAudioMIME = blob.MIMEType
			if err := asrInput.Add(chunk.Clone()); err != nil {
				return feedASRResult{streamID: streamID, err: err}
			}
			if emitHistoryAudio != nil {
				if err := emitHistoryAudio(chunk); err != nil {
					return feedASRResult{streamID: streamID, err: err}
				}
			}
		}
		if chunk.IsEndOfStream() {
			eos := chunk.Clone()
			if _, ok := eos.Part.(*genx.Blob); !ok {
				eos.Part = &genx.Blob{MIMEType: lastAudioMIME}
			}
			if eos.Ctrl == nil {
				eos.Ctrl = &genx.StreamCtrl{}
			}
			eos.Ctrl.StreamID = streamID
			eos.Ctrl.EndOfStream = true
			if err := asrInput.Add(eos); err != nil {
				return feedASRResult{streamID: streamID, err: err}
			}
			if emitHistoryAudio != nil {
				if err := emitHistoryAudio(eos); err != nil {
					return feedASRResult{streamID: streamID, err: err}
				}
			}
			if err := asrInput.Done(genx.Usage{}); err != nil {
				return feedASRResult{streamID: streamID, err: err}
			}
			return feedASRResult{streamID: streamID}
		}
	}
}

func feedRealtimeASRInput(ctx context.Context, input genx.Stream, asrInput *genx.StreamBuilder, streamIDState *lockedString, inputStarted chan string) feedASRResult {
	streamID := streamIDState.Get()
	if streamID == "" {
		streamID = defaultInputStreamID
		streamIDState.Set(streamID)
	}
	notifiedStreamID := ""
	notifyStarted := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || inputStarted == nil {
			return
		}
		if id == notifiedStreamID {
			return
		}
		notifiedStreamID = id
		select {
		case inputStarted <- id:
		default:
			select {
			case <-inputStarted:
			default:
			}
			select {
			case inputStarted <- id:
			default:
			}
		}
	}
	fail := func(err error) feedASRResult {
		_ = asrInput.Unexpected(genx.Usage{}, err)
		return feedASRResult{streamID: streamID, err: err}
	}
	if input == nil {
		return fail(fmt.Errorf("flowcraft: input stream is required"))
	}
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		chunk, err := input.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) || errors.Is(err, io.EOF) {
				if err := asrInput.Done(genx.Usage{}); err != nil {
					return feedASRResult{streamID: streamID, err: err}
				}
				return feedASRResult{streamID: streamID}
			}
			return fail(err)
		}
		if chunk == nil {
			continue
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) != "" {
			streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
			streamIDState.Set(streamID)
		}
		if chunk.IsBeginOfStream() {
			notifyStarted(streamID)
			continue
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || !isAudioMIME(blob.MIMEType) {
			continue
		}
		notifyStarted(streamID)
		next := chunk.Clone()
		if next.Ctrl == nil {
			next.Ctrl = &genx.StreamCtrl{}
		}
		if strings.TrimSpace(next.Ctrl.StreamID) == "" {
			next.Ctrl.StreamID = streamID
		}
		if err := asrInput.Add(next); err != nil {
			return feedASRResult{streamID: streamID, err: err}
		}
	}
}

type realtimeHistoryAudioBuffer struct {
	mu       sync.Mutex
	byStream map[string][]*genx.MessageChunk
}

func (b *realtimeHistoryAudioBuffer) append(chunk *genx.MessageChunk, streamID string) {
	if b == nil || chunk == nil {
		return
	}
	if _, ok := chunk.Part.(*genx.Blob); !ok {
		return
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = defaultInputStreamID
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byStream == nil {
		b.byStream = make(map[string][]*genx.MessageChunk)
	}
	b.byStream[streamID] = append(b.byStream[streamID], userAudioHistoryChunk(chunk, streamID))
}

func (b *realtimeHistoryAudioBuffer) drain(sourceStreamID string, targetStreamID string) []*genx.MessageChunk {
	if b == nil {
		return nil
	}
	sourceStreamID = strings.TrimSpace(sourceStreamID)
	if sourceStreamID == "" {
		sourceStreamID = defaultInputStreamID
	}
	b.mu.Lock()
	chunks := b.byStream[sourceStreamID]
	delete(b.byStream, sourceStreamID)
	b.mu.Unlock()
	if len(chunks) == 0 {
		return nil
	}
	out := make([]*genx.MessageChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		next := userAudioHistoryChunk(chunk, targetStreamID)
		next.Ctrl.EndOfStream = chunk.IsEndOfStream()
		out = append(out, next)
	}
	return out
}

func userAudioHistoryChunk(chunk *genx.MessageChunk, streamID string) *genx.MessageChunk {
	if strings.TrimSpace(streamID) == "" {
		streamID = defaultInputStreamID
	}
	next := chunk.Clone()
	next.Role = genx.RoleUser
	next.Name = transcriptLabel
	if next.Ctrl == nil {
		next.Ctrl = &genx.StreamCtrl{}
	}
	next.Ctrl.StreamID = streamID
	next.Ctrl.Label = genx.HistoryUserAudioLabel
	return next
}

func userAudioHistoryEOSChunk(streamID, mimeType string) *genx.MessageChunk {
	if strings.TrimSpace(streamID) == "" {
		streamID = defaultInputStreamID
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "audio/pcm"
	}
	return &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: transcriptLabel,
		Part: &genx.Blob{MIMEType: mimeType},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel, EndOfStream: true},
	}
}

func realtimeASRStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		if streamID := strings.TrimSpace(chunk.Ctrl.StreamID); streamID != "" {
			return streamID
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return defaultInputStreamID
	}
	return fallback
}

func realtimeTurnStreamID(prefix string, index int) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = defaultInputStreamID
	}
	if index <= 0 {
		index = 1
	}
	return fmt.Sprintf("%s:rt:%d", prefix, index)
}

func realtimeInputInterruptsCurrent(currentStreamID, inputStreamID string) bool {
	currentStreamID = strings.TrimSpace(currentStreamID)
	inputStreamID = strings.TrimSpace(inputStreamID)
	if currentStreamID == "" || inputStreamID == "" {
		return true
	}
	return currentStreamID != inputStreamID && !strings.HasPrefix(currentStreamID, inputStreamID+":")
}

type lockedString struct {
	mu    sync.RWMutex
	value string
}

func (s *lockedString) Set(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = value
}

func (s *lockedString) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

func textChunk(role genx.Role, name, streamID, label, text string, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: role,
		Name: name,
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: eos},
	}
}

func audioChunk(name, streamID string, data []byte, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: name,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: data},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: assistantLabel, EndOfStream: eos},
	}
}

func mergeTranscript(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" || next == "" {
		if current != "" {
			return current
		}
		return next
	}
	if strings.HasPrefix(next, current) {
		return next
	}
	if strings.HasPrefix(current, next) {
		return current
	}
	currentNorm := normalizeTranscriptText(current)
	nextNorm := normalizeTranscriptText(next)
	if currentNorm != "" && nextNorm != "" {
		if strings.HasPrefix(nextNorm, currentNorm) {
			return next
		}
		if strings.HasPrefix(currentNorm, nextNorm) {
			return current
		}
	}
	return current + next
}

func normalizeTranscriptText(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func opusFramesFromOgg(raw []byte) ([][]byte, error) {
	var frames [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(raw)) {
		if err != nil {
			return nil, err
		}
		if codecconv.IsOpusHeadPacket(packet.Data) || codecconv.IsOpusTagsPacket(packet.Data) || len(packet.Data) == 0 {
			continue
		}
		frames = append(frames, append([]byte(nil), packet.Data...))
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no opus audio packets found")
	}
	return frames, nil
}

type oggOpusFrameDecoder struct {
	pending               []byte
	packet                []byte
	expectingContinuation bool
	currentPacketBOS      bool
}

func newOggOpusFrameDecoder() *oggOpusFrameDecoder {
	return &oggOpusFrameDecoder{}
}

func (d *oggOpusFrameDecoder) Write(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	d.pending = append(d.pending, data...)
	var frames [][]byte
	for {
		page, ok, err := d.nextPage()
		if err != nil {
			return nil, err
		}
		if !ok {
			return frames, nil
		}
		pageFrames, err := d.consumePage(page)
		if err != nil {
			return nil, err
		}
		frames = append(frames, pageFrames...)
	}
}

func (d *oggOpusFrameDecoder) Close() error {
	if len(d.pending) != 0 {
		return fmt.Errorf("truncated ogg page: %d pending bytes", len(d.pending))
	}
	if d.expectingContinuation || len(d.packet) != 0 {
		return fmt.Errorf("stream ended with unterminated ogg packet")
	}
	return nil
}

func (d *oggOpusFrameDecoder) nextPage() (*ogg.Page, bool, error) {
	const oggPageHeaderSize = 27
	if len(d.pending) == 0 {
		return nil, false, nil
	}
	if len(d.pending) < oggPageHeaderSize {
		if len(d.pending) < len(ogg.CapturePattern) && !strings.HasPrefix(ogg.CapturePattern, string(d.pending)) {
			return nil, false, fmt.Errorf("invalid ogg capture pattern prefix %q", d.pending)
		}
		if len(d.pending) >= len(ogg.CapturePattern) && string(d.pending[:len(ogg.CapturePattern)]) != ogg.CapturePattern {
			return nil, false, fmt.Errorf("invalid ogg capture pattern prefix %q", d.pending)
		}
		return nil, false, nil
	}
	if string(d.pending[:4]) != ogg.CapturePattern {
		return nil, false, fmt.Errorf("invalid ogg capture pattern %q", d.pending[:4])
	}
	segmentCount := int(d.pending[26])
	headerLen := oggPageHeaderSize + segmentCount
	if len(d.pending) < headerLen {
		return nil, false, nil
	}
	payloadLen := 0
	for _, segment := range d.pending[oggPageHeaderSize:headerLen] {
		payloadLen += int(segment)
	}
	pageLen := headerLen + payloadLen
	if len(d.pending) < pageLen {
		return nil, false, nil
	}
	page, err := ogg.ParsePage(d.pending[:pageLen])
	if err != nil {
		return nil, false, err
	}
	d.pending = d.pending[pageLen:]
	return page, true, nil
}

func (d *oggOpusFrameDecoder) consumePage(page *ogg.Page) ([][]byte, error) {
	if page == nil {
		return nil, fmt.Errorf("ogg page is nil")
	}
	if page.HasContinuation() {
		if !d.expectingContinuation {
			return nil, fmt.Errorf("unexpected ogg continuation page")
		}
	} else if d.expectingContinuation {
		return nil, fmt.Errorf("missing ogg continuation page")
	}

	var frames [][]byte
	payloadOffset := 0
	for segmentIndex, segment := range page.Segments {
		if !d.expectingContinuation && len(d.packet) == 0 {
			d.currentPacketBOS = page.HasBOS() && segmentIndex == 0
		}
		chunkLen := int(segment)
		if payloadOffset+chunkLen > len(page.Payload) {
			return nil, fmt.Errorf("ogg segment overflows payload")
		}
		if chunkLen > 0 {
			d.packet = append(d.packet, page.Payload[payloadOffset:payloadOffset+chunkLen]...)
		}
		payloadOffset += chunkLen
		if segment == 255 {
			d.expectingContinuation = true
			continue
		}
		packet := append([]byte(nil), d.packet...)
		d.packet = d.packet[:0]
		d.expectingContinuation = false
		d.currentPacketBOS = false
		if len(packet) == 0 || codecconv.IsOpusHeadPacket(packet) || codecconv.IsOpusTagsPacket(packet) {
			continue
		}
		frames = append(frames, packet)
	}
	if payloadOffset != len(page.Payload) {
		return nil, fmt.Errorf("ogg page has trailing payload")
	}
	return frames, nil
}

func baseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func isAudioMIME(mimeType string) bool {
	return strings.HasPrefix(baseMIME(mimeType), "audio/")
}

type sliceStream struct {
	chunks []*genx.MessageChunk
	err    error
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.chunks) == 0 {
		return nil, genx.Done(genx.Usage{})
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *sliceStream) CloseWithError(err error) error {
	s.err = err
	s.chunks = nil
	return nil
}

func buildOwnedRuntimeConfig(
	ctx context.Context,
	genxService *peergenx.Service,
	options ConfiguredAgentOptions,
	workspace sdkworkspace.Workspace,
	toolkit commonagent.Toolkit,
	history logstore.MutableStore,
	memoryStore memory.Store,
) (ownedflowcraft.Config, error) {
	raw := deepCopyMap(options.Flowcraft)
	if raw == nil {
		raw = make(map[string]any)
	}
	ensureDefaultAgent(raw)
	accessible, err := accessibleGeneratorModels(ctx, genxService)
	if err != nil {
		return ownedflowcraft.Config{}, err
	}
	models := make(map[string]ownedflowcraft.GenXModel)
	for _, role := range flowcraftModelRoles {
		modelID, ok, err := configuredModelIDForRole(options, raw, role.settingKey, role.required)
		if err != nil {
			return ownedflowcraft.Config{}, err
		}
		if !ok {
			continue
		}
		if _, ok := accessible[modelID]; !ok {
			return ownedflowcraft.Config{}, fmt.Errorf("flowcraft: model %q is not accessible as a generator", modelID)
		}
		model := ownedflowcraft.GenXModel{Generator: genxService.Generator(), Pattern: "model/" + modelID}
		models[role.settingKey] = model
		models[role.modelsKey] = model
		models[modelID] = model
	}
	resolver, err := ownedflowcraft.NewGenXResolver(models)
	if err != nil {
		return ownedflowcraft.Config{}, err
	}

	agentValues := ensureMap(raw, "agent")
	agentID := mapString(agentValues, "id")
	if agentID == "" {
		agentID = "claw"
	}
	modelRef := mapString(agentValues, "model")
	if modelRef == "" {
		modelRef = "generate_model"
	}
	systemPrompt := mapString(agentValues, "system_prompt")
	graphDefinition := flowgraph.GraphDefinition{}
	if graphValue, ok := agentValues["graph"]; ok && graphValue != nil {
		data, err := json.Marshal(graphValue)
		if err != nil {
			return ownedflowcraft.Config{}, fmt.Errorf("flowcraft: encode agent graph: %w", err)
		}
		if err := json.Unmarshal(data, &graphDefinition); err != nil {
			return ownedflowcraft.Config{}, fmt.Errorf("flowcraft: decode agent graph: %w", err)
		}
	} else {
		graphDefinition = flowgraph.GraphDefinition{
			Name: agentID, Entry: "answer",
			Nodes: []flowgraph.NodeDefinition{{
				ID: "answer", Type: "llm", Config: map[string]any{"model": modelRef, "system_prompt": systemPrompt},
			}},
			Edges: []flowgraph.EdgeDefinition{{From: "answer", To: flowgraph.END}},
		}
	}
	maxIterations := intValue(agentValues["max_iterations"])
	if maxIterations == 0 {
		maxIterations = 8
	}
	parallel := runner.ParallelConfig{}
	if value, ok := agentValues["parallel"]; ok && value != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return ownedflowcraft.Config{}, fmt.Errorf("flowcraft: encode parallel config: %w", err)
		}
		if err := json.Unmarshal(data, &parallel); err != nil {
			return ownedflowcraft.Config{}, fmt.Errorf("flowcraft: decode parallel config: %w", err)
		}
	}
	publishNodes := ownedPublisherNodes(agentValues)
	workspaceName := strings.TrimSpace(options.WorkspaceName)
	if workspaceName == "" {
		workspaceName = agentID
	}
	return ownedflowcraft.Config{
		ID: agentID, Conversation: workspaceName, HistoryWorkspace: workspaceName,
		Graph: graphDefinition, Resolver: resolver,
		Workspace: workspace, Toolkit: toolkit, History: history, Memory: memoryStore, PublishNodes: publishNodes,
		MaxIterations: maxIterations, Parallel: parallel, MaxToolCalls: 32, ToolTimeout: 30 * time.Second,
		InputProvider: options.InputProvider, ExternalOutputObservation: true,
	}, nil
}

func ownedPublisherNodes(agentValues map[string]any) map[string]bool {
	publisher, ok := agentValues["publisher"].(map[string]any)
	if !ok {
		return nil
	}
	nodes, ok := publisher["nodes"].(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]bool)
	for nodeID, raw := range nodes {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		publish, ok := config["publish"].(bool)
		if ok && publish {
			result[nodeID] = true
		}
	}
	return result
}

func intValue(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	default:
		return 0
	}
}

func accessibleGeneratorModels(ctx context.Context, genxService *peergenx.Service) (map[string]peergenx.GeneratorConfig, error) {
	if genxService == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	configs, err := genxService.ListAccessibleGeneratorConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: list accessible generator models: %w", err)
	}
	out := make(map[string]peergenx.GeneratorConfig, len(configs))
	for _, cfg := range configs {
		out[string(cfg.Model.Id)] = cfg
	}
	return out, nil
}

func validateVoiceAdapterResources(ctx context.Context, genxService *peergenx.Service, cfg voiceAdapterConfig) error {
	if genxService == nil {
		return fmt.Errorf("flowcraft: peergenx service is required")
	}
	if _, err := genxService.ResolveTransformer(ctx, "model/"+cfg.ASRModel); err != nil {
		return fmt.Errorf("flowcraft: resolve ASR model %q: %w", cfg.ASRModel, err)
	}
	voices := make([]string, 0)
	seen := map[string]bool{}
	addVoice := func(voice string) {
		voice = strings.TrimSpace(voice)
		if voice == "" || seen[voice] {
			return
		}
		seen[voice] = true
		voices = append(voices, voice)
	}
	addVoice(cfg.DefaultVoice)
	for _, voice := range cfg.NodeVoices {
		addVoice(voice)
	}
	for _, voice := range voices {
		if _, err := genxService.ResolveTransformer(ctx, "voice/"+voice); err != nil {
			return fmt.Errorf("flowcraft: resolve voice %q: %w", voice, err)
		}
	}
	return nil
}

func configuredModelIDForRole(options ConfiguredAgentOptions, cfg map[string]any, key string, required bool) (string, bool, error) {
	var value string
	switch key {
	case "generate_model":
		value = options.GenerateModel
	case "extract_model":
		value = options.ExtractModel
	case "embedding_model":
		value = options.EmbeddingModel
	}
	if value = strings.TrimSpace(value); value != "" {
		return value, true, nil
	}
	if settings, ok := cfg["settings"].(map[string]any); ok {
		if text, ok := settings[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), true, nil
		}
	}
	if required {
		return "", false, fmt.Errorf("flowcraft: %s is required", key)
	}
	return "", false, nil
}

func normalizeInputMode(value any) inputMode {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "push", "push_to_talk", "push-to-talk", "ptt":
		return inputModePushToTalk
	case "realtime", "real_time", "real-time":
		return inputModeRealtime
	default:
		return ""
	}
}

func ensureDefaultAgent(cfg map[string]any) {
	agent := ensureMap(cfg, "agent")
	if _, ok := agent["id"]; !ok {
		agent["id"] = "claw"
	}
	if _, ok := agent["name"]; !ok {
		agent["name"] = "Claw"
	}
	if _, ok := agent["model"]; !ok {
		agent["model"] = "generate_model"
	}
	if _, ok := agent["system_prompt"]; !ok {
		agent["system_prompt"] = "你是一个简短、自然的中文语音聊天助手。"
	}
	if _, ok := agent["max_iterations"]; !ok {
		agent["max_iterations"] = 8
	}
}

func ensureMap(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	if existing, ok := values[key].(map[string]any); ok {
		return existing
	}
	next := map[string]any{}
	values[key] = next
	return next
}

func deepCopyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case *string:
			if typed != nil && strings.TrimSpace(*typed) != "" {
				return strings.TrimSpace(*typed)
			}
		}
	}
	return ""
}
