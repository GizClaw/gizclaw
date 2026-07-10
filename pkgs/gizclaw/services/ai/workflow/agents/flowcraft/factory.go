package flowcraft

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	sdkworkspace "github.com/GizClaw/flowcraft/sdk/workspace"
	flowclaw "github.com/GizClaw/flowcraft/sdkx/claw"
	"gopkg.in/yaml.v3"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "flowcraft"

const (
	defaultInputStreamID = "audio"
	selfStartStreamID    = "flowcraft-self-start"
	transcriptLabel      = "transcript"
	assistantLabel       = "assistant"
	interruptedError     = "interrupted"
)

const opusFrameDuration = 20 * time.Millisecond

type inputMode string

const (
	inputModePushToTalk inputMode = "push_to_talk"
	inputModeRealtime   inputMode = "realtime"
)

var clawModelRoles = []struct {
	settingKey string
	modelsKey  string
	required   bool
}{
	{settingKey: "generate_model", modelsKey: "chat", required: true},
	{settingKey: "extract_model", modelsKey: "extractor"},
	{settingKey: "embedding_model", modelsKey: "embedder"},
}

type Factory struct {
	GenX *peergenx.Service
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
	clawConfig, err := buildClawConfig(ctx, f.GenX, spec, cfg)
	if err != nil {
		return nil, err
	}
	if err := validateVoiceAdapterResources(ctx, f.GenX, cfg); err != nil {
		return nil, err
	}
	if err := writeClawConfig(spec.Runtime.LocalDir, clawConfig); err != nil {
		return nil, err
	}
	ws, err := sdkworkspace.NewLocalWorkspace(spec.Runtime.LocalDir)
	if err != nil {
		return nil, err
	}
	claw, err := flowclaw.New(ws)
	if err != nil {
		return nil, err
	}
	starts, initiativePolicy := flowcraftConversationSettings(workspaceParams, cfg.Spec.Flowcraft)
	inputMode := inputModePushToTalk
	if workspaceParams != nil && workspaceParams.Input != nil {
		if mode := normalizeInputMode(string(*workspaceParams.Input)); mode != "" {
			inputMode = mode
		}
	}
	return &agent{
		transformers: f.GenX,
		claw:         realClaw{Claw: claw},
		asrModel:     cfg.Spec.VoiceAdapter.ASRModel,
		defaultVoice: cfg.Spec.VoiceAdapter.DefaultVoice,
		nodeVoices:   cfg.Spec.VoiceAdapter.NodeVoices,
		starts:       starts,
		startPolicy:  initiativePolicy,
		inputMode:    inputMode,
		localDir:     spec.Runtime.LocalDir,
	}, nil
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
	if strings.TrimSpace(c.Spec.VoiceAdapter.ASRModel) == "" {
		return fmt.Errorf("flowcraft: voice_adapter.asr_model is required")
	}
	if strings.TrimSpace(c.Spec.VoiceAdapter.DefaultVoice) == "" {
		return fmt.Errorf("flowcraft: voice_adapter.default_voice is required")
	}
	return nil
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
	transformers transformerProvider
	claw         clawClient
	asrModel     string
	defaultVoice string
	nodeVoices   map[string]string
	starts       string
	startPolicy  string
	inputMode    inputMode
	localDir     string

	outputMu       sync.Mutex
	activeOutput   *genx.StreamBuilder
	activeStreamID string
	outputEpoch    uint64

	selfStartMu sync.Mutex
	selfStarted bool
}

func (a *agent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	return apitypes.PeerRunWorkspaceState{RuntimeState: "running"}, nil
}

func (a *agent) ListHistory(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	var debugResp debugHistoryResponse
	if err := a.callDebug(ctx, http.MethodGet, "/debug/history", nil, &debugResp); err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryListResponse{
			Available: false,
			Items:     []apitypes.PeerRunHistoryEntry{},
			HasNext:   false,
			Message:   &message,
		}, nil
	}
	if !debugResp.Enabled {
		message := "flowcraft history is disabled"
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
	if offset > len(debugResp.Messages) {
		offset = len(debugResp.Messages)
	}
	remaining := len(debugResp.Messages) - offset
	if limit > remaining {
		limit = remaining
	}
	end := offset + limit
	items := make([]apitypes.PeerRunHistoryEntry, 0)
	now := time.Now().UTC()
	for i := offset; i < end; i++ {
		items = append(items, historyEntryFromMessage(debugResp.ContextID, i, now, debugResp.Messages[i]))
	}
	resp := apitypes.PeerRunHistoryListResponse{
		Available: true,
		Items:     items,
		HasNext:   end < len(debugResp.Messages),
	}
	if resp.HasNext {
		next := strconv.Itoa(end)
		resp.NextCursor = &next
	}
	if len(debugResp.Messages) == 0 {
		message := "flowcraft history is empty"
		resp.Message = &message
	}
	return resp, nil
}

func (a *agent) PlayHistory(ctx context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	var debugResp debugHistoryResponse
	if err := a.callDebug(ctx, http.MethodGet, "/debug/history", nil, &debugResp); err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{
			Accepted:  false,
			HistoryId: req.HistoryId,
			State:     "unavailable",
			Message:   &message,
		}, nil
	}
	msg, ok := historyMessageByID(debugResp.ContextID, req.HistoryId, debugResp.Messages)
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
	output, streamID, epoch, ok := a.beginReplayOutput()
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
	var debugResp debugMemoryResponse
	if err := a.callDebug(ctx, http.MethodGet, "/debug/memory", nil, &debugResp); err != nil {
		message := err.Error()
		return apitypes.PeerRunMemoryStatsResponse{
			Available: false,
			Enabled:   false,
			Message:   &message,
		}, nil
	}
	memoryRootPath := a.resolveWorkspacePath(debugResp.Root)
	memoryStats, err := inspectDirectoryStats(memoryRootPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return apitypes.PeerRunMemoryStatsResponse{}, fmt.Errorf("flowcraft: inspect memory root: %w", err)
	}
	workspaceStats, err := inspectDirectoryStats(a.localDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return apitypes.PeerRunMemoryStatsResponse{}, fmt.Errorf("flowcraft: inspect workspace root: %w", err)
	}
	metadata := map[string]interface{}{
		"root":                 debugResp.Root,
		"root_path":            memoryRootPath,
		"workspace_path":       a.localDir,
		"scope":                debugResp.Scope,
		"write":                debugResp.Write,
		"recall":               debugResp.Recall,
		"retrieval":            debugResp.Retrieval,
		"layout":               debugResp.Layout,
		"file_count":           workspaceStats.FileCount,
		"memory_file_count":    memoryStats.FileCount,
		"memory_storage_bytes": memoryStats.StorageBytes,
	}
	embeddingEnabled := false
	embeddingStatus := "disabled"
	indexStatus := "disabled"
	if debugResp.Enabled {
		indexStatus = "ready"
	}
	resp := apitypes.PeerRunMemoryStatsResponse{
		Available:        true,
		Enabled:          debugResp.Enabled,
		ItemCount:        memoryStats.JSONLLineCount,
		StorageBytes:     workspaceStats.StorageBytes,
		EmbeddingEnabled: &embeddingEnabled,
		EmbeddingStatus:  &embeddingStatus,
		IndexStatus:      &indexStatus,
		Metadata:         &metadata,
	}
	if backend := debugResp.RetrievalBackend(); backend != "" {
		resp.Backend = &backend
	}
	if !workspaceStats.LastUpdatedAt.IsZero() {
		updatedAt := workspaceStats.LastUpdatedAt
		resp.LastUpdatedAt = &updatedAt
	}
	return resp, nil
}

func (a *agent) Recall(ctx context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	payload := map[string]interface{}{"text": req.Query}
	if req.Limit != nil {
		payload["top_k"] = *req.Limit
	}
	if req.Filters != nil {
		for key, value := range *req.Filters {
			payload[key] = value
		}
	}
	var debugResp debugRecallResponse
	if err := a.callDebug(ctx, http.MethodPost, "/debug/recall", payload, &debugResp); err != nil {
		message := err.Error()
		return apitypes.PeerRunRecallResponse{
			Available: false,
			Hits:      []apitypes.PeerRunRecallHit{},
			Message:   &message,
		}, nil
	}
	if !debugResp.Enabled {
		message := "flowcraft memory recall is disabled"
		return apitypes.PeerRunRecallResponse{
			Available: false,
			Hits:      []apitypes.PeerRunRecallHit{},
			Message:   &message,
		}, nil
	}
	hits := make([]apitypes.PeerRunRecallHit, 0, len(debugResp.Hits))
	for i, hit := range debugResp.Hits {
		hits = append(hits, recallHitFromDebug(i, hit))
	}
	return apitypes.PeerRunRecallResponse{
		Available: true,
		Hits:      hits,
	}, nil
}

type transformerProvider interface {
	Transformer() genx.Transformer
}

type clawClient interface {
	RoundTrip(flowclaw.Request) (clawResponse, error)
	CloseContext(context.Context) error
}

type debugHTTPClaw interface {
	ServeDebugHTTP(http.ResponseWriter, *http.Request)
}

type clawResponse interface {
	Next() (flowclaw.Event, error)
}

type realClaw struct {
	Claw *flowclaw.Claw
}

func (c realClaw) RoundTrip(req flowclaw.Request) (clawResponse, error) {
	return c.Claw.RoundTrip(req)
}

func (c realClaw) CloseContext(ctx context.Context) error {
	if c.Claw == nil {
		return nil
	}
	return c.Claw.CloseContext(ctx)
}

func (c realClaw) ServeDebugHTTP(w http.ResponseWriter, r *http.Request) {
	if c.Claw == nil {
		http.Error(w, "flowcraft: nil claw runtime", http.StatusServiceUnavailable)
		return
	}
	c.Claw.ServeDebugHTTP(w, r)
}

func (a *agent) callDebug(ctx context.Context, method, path string, body any, out any) error {
	debugger, ok := a.claw.(debugHTTPClaw)
	if !ok || debugger == nil {
		return fmt.Errorf("flowcraft debug API is not available")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("flowcraft: encode debug request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	debugger.ServeDebugHTTP(rec, req)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&problem)
		if problem.Error == "" {
			problem.Error = res.Status
		}
		return fmt.Errorf("flowcraft debug %s %s: %s", method, path, problem.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("flowcraft: decode debug response: %w", err)
	}
	return nil
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

type debugHistoryResponse struct {
	Enabled   bool                `json:"enabled"`
	ContextID string              `json:"context_id"`
	Count     int                 `json:"count"`
	Messages  []flowmodel.Message `json:"messages,omitempty"`
}

type debugMemoryResponse struct {
	Enabled   bool                   `json:"enabled"`
	Root      string                 `json:"root"`
	Scope     map[string]interface{} `json:"scope"`
	Write     map[string]interface{} `json:"write"`
	Recall    map[string]interface{} `json:"recall"`
	Retrieval map[string]interface{} `json:"retrieval"`
	Layout    map[string]interface{} `json:"layout"`
}

func (r debugMemoryResponse) RetrievalBackend() string {
	if raw, ok := r.Retrieval["backend"].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

type debugRecallResponse struct {
	Enabled bool             `json:"enabled"`
	Count   int              `json:"count"`
	Hits    []debugRecallHit `json:"hits"`
}

type debugRecallHit struct {
	ID        string   `json:"id,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Content   string   `json:"content"`
	Subject   string   `json:"subject,omitempty"`
	Predicate string   `json:"predicate,omitempty"`
	Object    string   `json:"object,omitempty"`
	Entities  []string `json:"entities,omitempty"`
	Score     float64  `json:"score,omitempty"`
	Sources   []string `json:"sources,omitempty"`
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

func recallHitFromDebug(index int, hit debugRecallHit) apitypes.PeerRunRecallHit {
	id := strings.TrimSpace(hit.ID)
	if id == "" {
		id = fmt.Sprintf("hit-%06d", index)
	}
	snippet := strings.TrimSpace(hit.Content)
	if snippet == "" {
		snippet = strings.TrimSpace(strings.Join([]string{hit.Subject, hit.Predicate, hit.Object}, " "))
	}
	sourceType := strings.TrimSpace(hit.Kind)
	var sourceTypePtr *string
	if sourceType != "" {
		sourceTypePtr = &sourceType
	}
	var sourceIDPtr *string
	if len(hit.Sources) > 0 && strings.TrimSpace(hit.Sources[0]) != "" {
		sourceID := strings.TrimSpace(hit.Sources[0])
		sourceIDPtr = &sourceID
	}
	metadata := map[string]interface{}{
		"subject":   hit.Subject,
		"predicate": hit.Predicate,
		"object":    hit.Object,
		"entities":  hit.Entities,
		"sources":   hit.Sources,
	}
	return apitypes.PeerRunRecallHit{
		Id:         id,
		Score:      hit.Score,
		Snippet:    snippet,
		SourceType: sourceTypePtr,
		SourceId:   sourceIDPtr,
		Metadata:   &metadata,
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
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	a.setActiveOutput(output, defaultInputStreamID)
	go a.run(ctx, input, output)
	return output.Stream(), nil
}

func (a *agent) run(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) {
	defer func() {
		a.clearActiveOutput(output)
		if a.claw != nil {
			_ = a.claw.CloseContext(context.Background())
		}
	}()

	current := a.startSelfTurnIfNeeded(ctx, output)
	if a.inputMode == inputModeRealtime {
		a.runRealtime(ctx, input, output, current)
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
	for {
		if current == nil && inputDone {
			select {
			case turn, ok := <-turns:
				if ok {
					current = a.startFlowcraftTurn(ctx, output, turn)
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
				current = a.startFlowcraftTurn(ctx, output, turn)
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
			current = a.startFlowcraftTurn(ctx, output, turn)
		case err := <-current.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				if isFlowcraftInputDone(err) {
					current = nil
					continue
				}
				_ = output.Unexpected(genx.Usage{}, err)
				return
			}
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

type flowcraftTranscriptTurn struct {
	streamID     string
	transcript   string
	historyAudio []*genx.MessageChunk
}

func (a *agent) startFlowcraftTurn(ctx context.Context, output *genx.StreamBuilder, turn flowcraftInputTurn) *flowcraftActiveTurn {
	streamID := strings.TrimSpace(turn.streamID)
	if streamID == "" {
		streamID = genx.NewStreamID()
	}
	epoch := a.setActiveOutput(output, streamID)
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

func (a *agent) startFlowcraftTranscriptTurn(ctx context.Context, output *genx.StreamBuilder, streamID, transcript string, emitTranscript bool, historyAudio ...*genx.MessageChunk) *flowcraftActiveTurn {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = genx.NewStreamID()
	}
	epoch := a.setActiveOutput(output, streamID)
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

func (a *agent) startSelfTurnIfNeeded(ctx context.Context, output *genx.StreamBuilder) *flowcraftActiveTurn {
	if !a.shouldSelfStart(ctx) {
		return nil
	}
	streamID := selfStartStreamID
	epoch := a.setActiveOutput(output, streamID)
	turnCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- a.runClawTextTurn(turnCtx, "", streamID, output, epoch)
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
	var resp debugHistoryResponse
	if err := a.callDebug(ctx, http.MethodGet, "/debug/history", nil, &resp); err != nil {
		return false, err
	}
	return resp.Count == 0 && len(resp.Messages) == 0, nil
}

func (a *agent) runRealtime(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, current *flowcraftActiveTurn) {
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
	startPending := func() {
		if current != nil || len(pending) == 0 {
			return
		}
		turn := pending[0]
		pending = pending[1:]
		current = a.startFlowcraftTranscriptTurn(ctx, output, turn.streamID, turn.transcript, true, turn.historyAudio...)
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
			return
		}
		current.cancel()
		_ = a.interruptOutput(output, current.streamID, current.epoch)
		current = nil
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
	return a.runClawTextTurn(ctx, transcript, streamID, output, epoch)
}

func (a *agent) runClawTextTurn(ctx context.Context, text, streamID string, output *genx.StreamBuilder, epoch uint64) error {
	text = strings.TrimSpace(text)
	a.setActiveStreamID(streamID)

	resp, err := a.claw.RoundTrip(flowclaw.Request{Context: ctx, Text: text})
	if err != nil {
		return fmt.Errorf("flowcraft: claw round trip: %w", err)
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
			return fmt.Errorf("flowcraft: read claw event: %w", err)
		}
		if ev.Type == flowclaw.EventError || ev.IsError {
			if ev.Err == "" {
				ev.Err = ev.Content
			}
			if sawToken && isClawPartialResponseLimitError(ev.Err) {
				break
			}
			return fmt.Errorf("flowcraft: claw event error: %s", ev.Err)
		}
		if ev.Type != flowclaw.EventToken || ev.Content == "" {
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

func isClawPartialResponseLimitError(message string) bool {
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

func (a *agent) setActiveOutput(output *genx.StreamBuilder, streamID string) uint64 {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	a.outputEpoch++
	a.activeOutput = output
	a.activeStreamID = streamID
	return a.outputEpoch
}

func (a *agent) setActiveStreamID(streamID string) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	a.activeStreamID = streamID
}

func (a *agent) clearActiveOutput(output *genx.StreamBuilder) {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	if a.activeOutput == output {
		a.activeOutput = nil
		a.activeStreamID = ""
	}
}

func (a *agent) beginReplayOutput() (*genx.StreamBuilder, string, uint64, bool) {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	if a.activeOutput == nil {
		return nil, "", 0, false
	}
	a.outputEpoch++
	streamID := strings.TrimSpace(a.activeStreamID)
	if streamID == "" {
		streamID = defaultInputStreamID
	}
	return a.activeOutput, streamID, a.outputEpoch, true
}

func (a *agent) currentOutputEpoch() uint64 {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	return a.outputEpoch
}

func (a *agent) isCurrentOutputEpoch(epoch uint64) bool {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	return a.outputEpoch == epoch
}

func (a *agent) addOutput(output *genx.StreamBuilder, epoch uint64, chunks ...*genx.MessageChunk) error {
	if !a.isCurrentOutputEpoch(epoch) {
		return nil
	}
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
	a.outputMu.Lock()
	if a.outputEpoch != epoch {
		a.outputMu.Unlock()
		return false
	}
	a.outputEpoch++
	a.outputMu.Unlock()

	textEOS := textChunk(genx.RoleModel, assistantLabel, streamID, assistantLabel, "", true)
	audioEOS := audioChunk(assistantLabel, streamID, nil, true)
	textEOS.Ctrl.Error = interruptedError
	audioEOS.Ctrl.Error = interruptedError
	return output.Add(textEOS, audioEOS) == nil
}

func (a *agent) waitOpusFrame(ctx context.Context, epoch uint64) error {
	if !a.isCurrentOutputEpoch(epoch) {
		return context.Canceled
	}
	timer := time.NewTimer(opusFrameDuration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	if !a.isCurrentOutputEpoch(epoch) {
		return context.Canceled
	}
	return nil
}

func (a *agent) transcribeInputTurn(ctx context.Context, input genx.Stream, output *genx.StreamBuilder, epoch uint64, defaultStreamID string) (string, string, error) {
	prefetched, err := readInputTurn(ctx, input, defaultStreamID)
	if err != nil {
		return "", prefetched.streamID, err
	}
	if prefetched.hasText && !prefetched.hasAudio {
		if err := a.addOutput(output, epoch,
			textChunk(genx.RoleUser, transcriptLabel, prefetched.streamID, transcriptLabel, prefetched.transcript, false),
			textChunk(genx.RoleUser, transcriptLabel, prefetched.streamID, transcriptLabel, "", true),
		); err != nil {
			return "", prefetched.streamID, err
		}
		return prefetched.transcript, prefetched.streamID, nil
	}
	input = &sliceStream{chunks: prefetched.chunks}
	transformer := a.transformers.Transformer()
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	asr, err := transformer.Transform(ctx, "model/"+a.asrModel, asrInput.Stream())
	if err != nil {
		return "", "", fmt.Errorf("flowcraft: start ASR: %w", err)
	}
	defer func() { _ = asr.Close() }()

	defaultStreamID = strings.TrimSpace(defaultStreamID)
	if defaultStreamID == "" {
		defaultStreamID = defaultInputStreamID
	}
	streamIDState := &lockedString{value: defaultStreamID}
	feedDone := make(chan feedASRResult, 1)
	go func() {
		emitHistoryAudio := func(chunk *genx.MessageChunk) error {
			return a.addOutput(output, epoch, userAudioHistoryChunk(chunk, streamIDState.Get()))
		}
		result := feedASRInput(ctx, input, asrInput, streamIDState, defaultStreamID, emitHistoryAudio)
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
				return "", result.streamID, result.err
			}
			return "", result.streamID, fmt.Errorf("flowcraft: read ASR: %w", err)
		}
		text, ok := chunk.Part.(genx.Text)
		if chunk.IsEndOfStream() {
			transcriptEOS = true
			if text != "" {
				part := string(text)
				transcript = mergeTranscript(transcript, part)
				if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, streamIDState.Get(), transcriptLabel, part, false)); err != nil {
					return "", streamIDState.Get(), err
				}
			}
			if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, streamIDState.Get(), transcriptLabel, "", true)); err != nil {
				return "", streamIDState.Get(), err
			}
			continue
		}
		if !ok || text == "" {
			continue
		}
		part := string(text)
		transcript = mergeTranscript(transcript, part)
		if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, streamIDState.Get(), transcriptLabel, part, false)); err != nil {
			return "", streamIDState.Get(), err
		}
	}
	result := <-feedDone
	if result.err != nil {
		return "", result.streamID, result.err
	}
	if result.streamID == "" {
		result.streamID = defaultInputStreamID
	}
	if !transcriptEOS {
		if err := a.addOutput(output, epoch, textChunk(genx.RoleUser, transcriptLabel, result.streamID, transcriptLabel, "", true)); err != nil {
			return "", result.streamID, err
		}
	}
	return transcript, result.streamID, nil
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
			if err := a.addOutput(output, epoch, audioChunk(nodeID, streamID, blob.Data, false)); err != nil {
				return err
			}
			if err := a.waitOpusFrame(ctx, epoch); err != nil {
				return err
			}
		case "audio/ogg", "application/ogg":
			frames, err := oggDecoder.Write(blob.Data)
			if err != nil {
				return fmt.Errorf("flowcraft: decode TTS ogg opus: %w", err)
			}
			for _, frame := range frames {
				if err := a.addOutput(output, epoch, audioChunk(nodeID, streamID, frame, false)); err != nil {
					return err
				}
				if err := a.waitOpusFrame(ctx, epoch); err != nil {
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

func feedASRInput(ctx context.Context, input genx.Stream, asrInput *genx.StreamBuilder, streamIDState *lockedString, defaultStreamID string, emitHistoryAudio historyAudioEmitter) feedASRResult {
	streamID := strings.TrimSpace(defaultStreamID)
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
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			streamID = chunk.Ctrl.StreamID
			streamIDState.Set(streamID)
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
			if eos.Ctrl.StreamID == "" {
				eos.Ctrl.StreamID = streamID
			}
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

func writeClawConfig(root string, cfg map[string]any) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("flowcraft: create workspace dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("flowcraft: encode claw config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), data, 0o600); err != nil {
		return fmt.Errorf("flowcraft: write claw config: %w", err)
	}
	return nil
}

func buildClawConfig(ctx context.Context, genxService *peergenx.Service, spec agenthost.Spec, cfg workflowConfig) (map[string]any, error) {
	out := deepCopyMap(cfg.Spec.Flowcraft)
	if out == nil {
		out = map[string]any{}
	}
	workspaceParams, err := flowcraftWorkspaceParameters(spec.Workspace.Parameters)
	if err != nil {
		return nil, err
	}
	settings := ensureMap(out, "settings")
	models := ensureMap(out, "models")
	llm := ensureMap(models, "llm")
	var accessibleModels map[string]peergenx.GeneratorConfig
	for _, role := range clawModelRoles {
		modelID, ok, err := modelIDForRole(workspaceParams, out, role.settingKey, role.required)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if accessibleModels == nil {
			accessibleModels, err = accessibleGeneratorModels(ctx, genxService)
			if err != nil {
				return nil, err
			}
		}
		generatorCfg, ok := accessibleModels[modelID]
		if !ok {
			return nil, fmt.Errorf("flowcraft: model %q is not accessible as a generator", modelID)
		}
		modelCfg, err := clawModelConfig(generatorCfg)
		if err != nil {
			return nil, err
		}
		settings[role.settingKey] = modelID
		models[role.modelsKey] = role.settingKey
		llm[modelID] = modelCfg
	}
	ensureDefaultAgent(out)
	return out, nil
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

func validateVoiceAdapterResources(ctx context.Context, genxService *peergenx.Service, cfg workflowConfig) error {
	if genxService == nil {
		return fmt.Errorf("flowcraft: peergenx service is required")
	}
	if _, err := genxService.ResolveTransformer(ctx, "model/"+cfg.Spec.VoiceAdapter.ASRModel); err != nil {
		return fmt.Errorf("flowcraft: resolve ASR model %q: %w", cfg.Spec.VoiceAdapter.ASRModel, err)
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
	addVoice(cfg.Spec.VoiceAdapter.DefaultVoice)
	for _, voice := range cfg.Spec.VoiceAdapter.NodeVoices {
		addVoice(voice)
	}
	for _, voice := range voices {
		if _, err := genxService.ResolveTransformer(ctx, "voice/"+voice); err != nil {
			return fmt.Errorf("flowcraft: resolve voice %q: %w", voice, err)
		}
	}
	return nil
}

func modelIDForRole(parameters *apitypes.FlowcraftWorkspaceParameters, cfg map[string]any, key string, required bool) (string, bool, error) {
	if parameters != nil {
		var value *string
		switch key {
		case "generate_model":
			value = parameters.GenerateModel
		case "extract_model":
			value = parameters.ExtractModel
		case "embedding_model":
			value = parameters.EmbeddingModel
		}
		if value != nil {
			if text := strings.TrimSpace(*value); text != "" {
				return text, true, nil
			}
		}
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

func clawModelConfig(cfg peergenx.GeneratorConfig) (map[string]any, error) {
	switch cfg.Tenant.Kind {
	case string(apitypes.ModelProviderKindOpenaiTenant):
		return resolveOpenAIClawModelConfig(cfg)
	case string(apitypes.ModelProviderKindVolcTenant):
		return resolveVolcClawModelConfig(cfg)
	default:
		return nil, fmt.Errorf("flowcraft: model %q provider %q is not supported by claw config generation", cfg.Model.Id, cfg.Tenant.Kind)
	}
}

func resolveOpenAIClawModelConfig(cfg peergenx.GeneratorConfig) (map[string]any, error) {
	if cfg.Tenant.OpenAI == nil {
		return nil, fmt.Errorf("flowcraft: openai tenant is required")
	}
	var providerData apitypes.OpenAITenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsOpenAITenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode openai provider_data: %w", err)
		}
	}
	upstream := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	body, err := cfg.Credential.Body.AsOpenAICredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey, body.Token)
	if apiKey == "" {
		return nil, fmt.Errorf("flowcraft: credential %q missing api_key", cfg.Credential.Name)
	}
	out := map[string]any{
		"provider": "openai",
		"model":    upstream,
		"api_key":  apiKey,
	}
	if extra := openAIThinkingConfig(providerData); len(extra) > 0 {
		out["config"] = extra
	}
	if baseURL := firstString(cfg.Tenant.OpenAI.BaseUrl, body.BaseUrl); baseURL != "" {
		out["base_url"] = baseURL
	}
	return out, nil
}

func resolveVolcClawModelConfig(cfg peergenx.GeneratorConfig) (map[string]any, error) {
	if cfg.Tenant.Volc == nil {
		return nil, fmt.Errorf("flowcraft: volc tenant is required")
	}
	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode volc provider_data: %w", err)
		}
	}
	model := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	if model == "" {
		return nil, fmt.Errorf("flowcraft: model %q missing upstream model", cfg.Model.Id)
	}
	body, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("flowcraft: credential %q missing api_key", cfg.Credential.Name)
	}
	out := map[string]any{
		"provider": "bytedance",
		"model":    model,
		"api_key":  apiKey,
	}
	if extra := volcThinkingConfig(providerData); len(extra) > 0 {
		out["config"] = extra
	}
	if baseURL := firstString(cfg.Tenant.Volc.Endpoint); baseURL != "" {
		out["base_url"] = baseURL
	}
	if region := firstString(cfg.Tenant.Volc.Region); region != "" {
		out["region"] = region
	}
	return out, nil
}

func openAIThinkingConfig(data apitypes.OpenAITenantModelProviderData) map[string]any {
	param := firstString(data.ThinkingParam, data.ThinkingLevelParam)
	level := firstString(data.DefaultThinkingLevel)
	if param == "" || level == "" {
		return nil
	}
	out := map[string]any{}
	setNestedConfigValue(out, param, openAIThinkingConfigValue(param, level))
	return out
}

func volcThinkingConfig(data apitypes.VolcTenantModelProviderData) map[string]any {
	param := firstString(data.ThinkingParam, data.ThinkingLevelParam)
	level := firstString(data.DefaultThinkingLevel)
	if param == "" || level == "" {
		return nil
	}
	out := map[string]any{}
	setNestedConfigValue(out, param, openAIThinkingConfigValue(param, level))
	return out
}

func openAIThinkingConfigValue(param, level string) any {
	if strings.EqualFold(strings.TrimSpace(param), "enable_thinking") {
		return !isDisabledThinkingLevel(level)
	}
	return level
}

func isDisabledThinkingLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "disabled", "disable", "off", "false", "0", "none", "no":
		return true
	default:
		return false
	}
}

func setNestedConfigValue(out map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}
	current := out
	for _, raw := range parts[:len(parts)-1] {
		part := strings.TrimSpace(raw)
		if part == "" {
			return
		}
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last != "" {
		current[last] = value
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
