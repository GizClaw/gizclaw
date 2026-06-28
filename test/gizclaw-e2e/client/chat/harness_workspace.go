//go:build gizclaw_e2e

package chat

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizwebrtc"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

var (
	dialClientForRun      = dialClient
	ensureWorkspaceForRun = func(ctx context.Context, client *gizcli.Client, cfg config) (config, error) {
		return ensureWorkspace(ctx, client, cfg)
	}
	selectAndReloadAgentForRun = func(ctx context.Context, client *gizcli.Client, cfg config) error {
		return selectAndReloadAgent(ctx, client, cfg)
	}
	newChatTransportForRun         = newChatTransport
	runWorkspaceCaseForRun         = (*personaDriver).runCase
	validateWorkspaceRuntimeForRun = validateWorkspaceRuntime
)

type workspaceCase string

const (
	workspaceCasePushToTalkRoundtrip workspaceCase = "push-to-talk-roundtrip"
	workspaceCasePushToTalkInterrupt workspaceCase = "push-to-talk-interrupt"
	workspaceCaseRealtimeRoundtrip   workspaceCase = "realtime-roundtrip"
	workspaceCaseRealtimeInterrupt   workspaceCase = "realtime-interrupt"
	workspaceCaseRealtimeAutoSplit   workspaceCase = "realtime-auto-split-history"
	workspaceCaseHistoryReplay       workspaceCase = "history-replay"
	workspaceCaseHumanReview         workspaceCase = "human-review"
)

type workspaceCaseResult struct {
	Rounds     []roundStats
	Interrupts []interruptStats
}

func runConfig(configPath, contextConfigPath string, selectedCase workspaceCase) error {
	cfg, err := loadConfig(configPath, contextConfigPath)
	if err != nil {
		return err
	}
	cfg, err = selectedCase.applyConfig(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	client, serveDone, err := dialClientForRun(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
		<-serveDone
	}()

	if cfg.shouldEnsureWorkspace() {
		ensured, err := ensureWorkspaceForRun(ctx, client, cfg)
		if err != nil {
			return err
		}
		cfg = ensured
	}

	openaiHTTPClient := client.HTTPClient(gizcli.ServiceOpenAI)
	openaiHTTPClient.Timeout = cfg.timeout
	openaiClient := openai.NewClient(
		option.WithAPIKey("gizclaw-peer"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(openaiHTTPClient),
	)
	driver := &personaDriver{
		cfg:           cfg,
		client:        openaiClient,
		runtimeClient: client,
		newTransport: func() (*chatTransport, error) {
			return newChatTransportForRun(client)
		},
		reloadAgent: func(ctx context.Context) error {
			return selectAndReloadAgentForRun(ctx, client, cfg)
		},
	}
	defer driver.close()
	result, err := runWorkspaceCaseForRun(driver, ctx, selectedCase)
	if len(result.Rounds) > 0 {
		printRunSummary(cfg, result.Rounds)
	}
	for _, interrupt := range result.Interrupts {
		printInterruptSummary(interrupt)
	}
	if err != nil {
		return err
	}
	if selectedCase == workspaceCaseRealtimeAutoSplit {
		return nil
	}
	report, err := validateWorkspaceRuntimeForRun(ctx, driver, client, cfg, result.Rounds)
	if report != nil {
		printWorkspaceRuntimeReport(*report)
	}
	return err
}

func (c workspaceCase) applyConfig(cfg config) (config, error) {
	if strings.TrimSpace(cfg.Workflow.Name) == "" {
		return config{}, fmt.Errorf("workflow.name is required")
	}
	cfg.Workspace = workspaceNameForCase(cfg.Workflow.Name, c)
	switch c {
	case workspaceCasePushToTalkRoundtrip, workspaceCasePushToTalkInterrupt, workspaceCaseHistoryReplay, workspaceCaseHumanReview:
		cfg.Workflow.Parameters.Input = string(rpcapi.WorkspaceInputModePushToTalk)
		if c == workspaceCaseHistoryReplay && cfg.Rounds < 1 {
			cfg.Rounds = 1
		}
		if c == workspaceCaseHumanReview && cfg.Rounds < 3 {
			cfg.Rounds = 3
		}
	case workspaceCaseRealtimeRoundtrip, workspaceCaseRealtimeInterrupt, workspaceCaseRealtimeAutoSplit:
		cfg.Workflow.Parameters.Input = string(rpcapi.WorkspaceInputModeRealtime)
	default:
		return config{}, fmt.Errorf("unsupported workspace case %q", c)
	}
	return cfg, nil
}

func workspaceNameForCase(workflowName string, selected workspaceCase) string {
	name := strings.Trim(strings.ToLower(strings.TrimSpace(workflowName))+"-"+string(selected), "-")
	replacer := strings.NewReplacer("_", "-", ".", "-", " ", "-")
	return compactWorkspaceName(replacer.Replace(name))
}

func compactWorkspaceName(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func (d *personaDriver) runCase(ctx context.Context, selected workspaceCase) (workspaceCaseResult, error) {
	switch selected {
	case workspaceCasePushToTalkRoundtrip:
		rounds, err := d.runPushToTalkRoundtrip(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	case workspaceCaseRealtimeRoundtrip:
		rounds, err := d.runRealtimeRoundtrip(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	case workspaceCasePushToTalkInterrupt:
		interrupts, err := d.runPushToTalkInterrupt(ctx)
		return workspaceCaseResult{Interrupts: interrupts}, err
	case workspaceCaseRealtimeInterrupt:
		interrupts, err := d.runRealtimeInterrupt(ctx)
		return workspaceCaseResult{Interrupts: interrupts}, err
	case workspaceCaseRealtimeAutoSplit:
		return workspaceCaseResult{}, d.runRealtimeAutoSplitHistory(ctx)
	case workspaceCaseHistoryReplay:
		rounds, err := d.runPushToTalkRoundtrip(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	case workspaceCaseHumanReview:
		rounds, err := d.runHumanReview(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	default:
		return workspaceCaseResult{}, fmt.Errorf("unsupported workspace case %q", selected)
	}
}

func dialClient(cfg config) (*gizcli.Client, <-chan error, error) {
	keyPair, err := parsePrivateKey(cfg.ClientPrivateKey)
	if err != nil {
		return nil, nil, err
	}
	serverPK, err := parsePublicKey(cfg.Server.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	client := &gizcli.Client{
		KeyPair: keyPair,
		DialTransport: func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			if cfg.Server.Transport == "webrtc" {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				return gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
					SignalingURL:   cfg.Server.SignalingURL,
					CipherMode:     gizwebrtc.CipherMode(cfg.Server.CipherMode),
					SecurityPolicy: securityPolicy,
				})
			}
			l, err := (&giznoise.ListenConfig{
				Addr:           ":0",
				CipherMode:     giznoise.CipherMode(cfg.Server.CipherMode),
				SecurityPolicy: securityPolicy,
			}).Listen(key)
			if err != nil {
				return nil, nil, err
			}
			udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
			if err != nil {
				_ = l.Close()
				return nil, nil, err
			}
			conn, err := l.Dial(serverPK, udpAddr)
			if err != nil {
				_ = l.Close()
				return nil, nil, err
			}
			return l, conn, nil
		},
	}
	if err := client.Dial(serverPK, cfg.Server.Addr); err != nil {
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() {
		done <- client.Serve()
	}()
	return client, done, nil
}

type runControlClient interface {
	GetWorkflow(context.Context, string, rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error)
	StopServerRun(context.Context, string) (*rpcapi.ServerStopRunResponse, error)
	DeleteWorkspace(context.Context, string, rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error)
	CreateWorkspace(context.Context, string, rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error)
	SetServerRunWorkspace(context.Context, string, rpcapi.ServerSetRunWorkspaceRequest) (*rpcapi.ServerSetRunWorkspaceResponse, error)
	ReloadServerRunWorkspace(context.Context, string) (*rpcapi.ServerReloadRunWorkspaceResponse, error)
	GetServerRunWorkspace(context.Context, string) (*rpcapi.ServerGetRunWorkspaceResponse, error)
	ListServerRunWorkspaceHistory(context.Context, string, rpcapi.ServerListRunWorkspaceHistoryRequest) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error)
	PlayServerRunWorkspaceHistory(context.Context, string, rpcapi.ServerPlayRunWorkspaceHistoryRequest) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error)
	GetServerRunWorkspaceMemoryStats(context.Context, string, rpcapi.ServerGetRunWorkspaceMemoryStatsRequest) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error)
	ServerRunWorkspaceRecall(context.Context, string, rpcapi.ServerRunWorkspaceRecallRequest) (*rpcapi.ServerRunWorkspaceRecallResponse, error)
}

func ensureWorkspace(ctx context.Context, client runControlClient, cfg config) (config, error) {
	workflowDisplayName := cfg.Workflow.Name
	workspaceDisplayName := cfg.Workspace

	workflow, err := client.GetWorkflow(ctx, "workspacetest.workflow.get", rpcapi.WorkflowGetRequest{
		Name: cfg.Workflow.Name,
	})
	if err != nil {
		if isRPCNotFound(err) {
			return config{}, fmt.Errorf("get workflow %q (%s): not found; run test/gizclaw-e2e/setup/reset_data.sh init before chat tests", cfg.Workflow.Name, workflowDisplayName)
		}
		return config{}, fmt.Errorf("get workflow %q (%s): %w", cfg.Workflow.Name, workflowDisplayName, err)
	}
	if workflow == nil || strings.TrimSpace(workflow.Metadata.Name) == "" {
		return config{}, fmt.Errorf("get workflow %q (%s): empty workflow id", cfg.Workflow.Name, workflowDisplayName)
	}
	if workflow.Metadata.Name != cfg.Workflow.Name {
		return config{}, fmt.Errorf("get workflow %q (%s): returned workflow id %q", cfg.Workflow.Name, workflowDisplayName, workflow.Metadata.Name)
	}

	workspace, err := workspaceDocument(cfg)
	if err != nil {
		return config{}, fmt.Errorf("build workspace %q (%s): %w", cfg.Workspace, workspaceDisplayName, err)
	}
	fmt.Printf("workspace_progress event=workspace_recreate_start workspace=%s workflow=%s\n", cfg.Workspace, cfg.Workflow.Name)
	if _, err := client.StopServerRun(ctx, "workspacetest.run.stop"); err != nil {
		return config{}, fmt.Errorf("stop active workspace before recreate %q (%s): %w", cfg.Workspace, workspaceDisplayName, err)
	}
	if _, err := client.DeleteWorkspace(ctx, "workspacetest.workspace.delete", rpcapi.WorkspaceDeleteRequest{
		Name: cfg.Workspace,
	}); err != nil {
		if !isRPCNotFound(err) {
			return config{}, fmt.Errorf("delete workspace %q (%s): %w", cfg.Workspace, workspaceDisplayName, err)
		}
		fmt.Printf("workspace_progress event=workspace_delete_missing workspace=%s\n", cfg.Workspace)
	} else {
		fmt.Printf("workspace_progress event=workspace_delete_done workspace=%s\n", cfg.Workspace)
	}
	createdWorkspace, err := client.CreateWorkspace(ctx, "workspacetest.workspace.create", workspace)
	if err != nil {
		return config{}, fmt.Errorf("create workspace %q (%s): %w", cfg.Workspace, workspaceDisplayName, err)
	}
	if createdWorkspace == nil || strings.TrimSpace(createdWorkspace.Name) == "" {
		return config{}, fmt.Errorf("create workspace %q (%s): empty workspace id", cfg.Workspace, workspaceDisplayName)
	}
	if createdWorkspace.Name != cfg.Workspace {
		return config{}, fmt.Errorf("create workspace %q (%s): returned workspace id %q", cfg.Workspace, workspaceDisplayName, createdWorkspace.Name)
	}
	fmt.Printf("workspace_progress event=workspace_create_done workspace=%s workflow=%s\n", cfg.Workspace, cfg.Workflow.Name)
	return cfg, nil
}

func isRPCNotFound(err error) bool {
	var rpcErr rpcapi.Error
	return errors.As(err, &rpcErr) && rpcErr.Code == rpcapi.RPCErrorCodeNotFound
}

func workflowDocument(cfg config) rpcapi.WorkflowCreateRequest {
	description := cfg.Workflow.Description
	if description == "" {
		description = "workflow"
	}
	spec := workflowSpec(cfg)
	return rpcapi.WorkflowCreateRequest{
		Metadata: rpcapi.WorkflowMetadata{
			Name:        cfg.Workflow.Name,
			Description: &description,
		},
		Spec: spec,
	}
}

func workflowSpec(cfg config) rpcapi.WorkflowSpec {
	if cfg.isFlowcraftAgent() {
		flowcraft := cloneWorkflowMap(cfg.Workflow.Flowcraft)
		flowcraft["voice_adapter"] = map[string]interface{}{
			"asr_model":     cfg.Workflow.VoiceAdapter.ASRModel,
			"default_voice": cfg.Workflow.VoiceAdapter.DefaultVoice,
			"node_voices":   cfg.Workflow.VoiceAdapter.NodeVoices,
		}
		return rpcapi.WorkflowSpec{
			Driver:    rpcapi.WorkflowDriver("flowcraft"),
			Flowcraft: (*rpcapi.FlowcraftWorkflowSpec)(&flowcraft),
		}
	}
	if cfg.isASTTranslateAgent() {
		spec := rpcapi.ASTTranslateWorkflowSpec{
			TranslationModel: cfg.Workflow.Translation,
		}
		if cfg.Workflow.ASTTranslate.Mode != "" {
			mode := rpcapi.ASTTranslateMode(cfg.Workflow.ASTTranslate.Mode)
			spec.Mode = &mode
		}
		if voice := astTranslateVoiceParams(cfg.Workflow.ASTTranslate.Voice); voice != nil {
			spec.Voice = voice
		}
		if cfg.Workflow.ASTTranslate.SpeakerID != "" {
			spec.SpeakerId = &cfg.Workflow.ASTTranslate.SpeakerID
		}
		spec.IsCustomSpeaker = cfg.Workflow.ASTTranslate.IsCustomSpeaker
		if cfg.Workflow.ASTTranslate.TTSResourceID != "" {
			spec.TtsResourceId = &cfg.Workflow.ASTTranslate.TTSResourceID
		}
		spec.SpeechRate = cfg.Workflow.ASTTranslate.SpeechRate
		spec.EnableSourceLanguageDetect = cfg.Workflow.ASTTranslate.EnableSourceLanguageDetect
		spec.Denoise = cfg.Workflow.ASTTranslate.Denoise
		if cfg.Workflow.ASTTranslate.ResourceID != "" {
			spec.ResourceId = &cfg.Workflow.ASTTranslate.ResourceID
		}
		return rpcapi.WorkflowSpec{
			Driver:       rpcapi.WorkflowDriverAstTranslate,
			AstTranslate: &spec,
		}
	}
	return rpcapi.WorkflowSpec{
		Driver: rpcapi.WorkflowDriver("doubao-realtime"),
		DoubaoRealtime: &rpcapi.DoubaoRealtimeWorkflowSpec{
			Model:        cfg.Workflow.Model,
			Instructions: optionalString(cfg.Workflow.Instructions),
			Audio:        cfg.Workflow.Audio,
			Tools:        cfg.Workflow.Tools,
			Extension:    cfg.Workflow.Extension,
		},
	}
}

func cloneWorkflowMap(in map[string]interface{}) rpcapi.FlowcraftWorkflowSpec {
	out := make(rpcapi.FlowcraftWorkflowSpec, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func workspaceDocument(cfg config) (rpcapi.WorkspaceCreateRequest, error) {
	var parameters rpcapi.WorkspaceParameters
	switch {
	case cfg.isFlowcraftAgent():
		typed := rpcapi.FlowcraftWorkspaceParameters{
			AgentType:      rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
			Input:          optionalWorkspaceInputMode(cfg.Workflow.Parameters.Input),
			GenerateModel:  optionalString(cfg.Workflow.Parameters.GenerateModel),
			ExtractModel:   optionalString(cfg.Workflow.Parameters.ExtractModel),
			EmbeddingModel: optionalString(cfg.Workflow.Parameters.EmbeddingModel),
		}
		if err := parameters.FromFlowcraftWorkspaceParameters(typed); err != nil {
			return rpcapi.WorkspaceCreateRequest{}, fmt.Errorf("encode flowcraft workspace parameters: %w", err)
		}
	case cfg.isASTTranslateAgent():
		typed := rpcapi.ASTTranslateWorkspaceParameters{
			AgentType:                  rpcapi.ASTTranslateWorkspaceParametersAgentTypeAstTranslate,
			Input:                      optionalWorkspaceInputMode(cfg.Workflow.Parameters.Input),
			TranslationModel:           optionalString(cfg.Workflow.Parameters.TranslationModel),
			LangPair:                   optionalString(cfg.Workflow.Parameters.LangPair),
			Mode:                       optionalASTTranslateMode(cfg.Workflow.Parameters.Mode),
			Voice:                      astTranslateWorkspaceVoiceParams(cfg.Workflow.Parameters.Voice),
			SpeakerId:                  optionalString(cfg.Workflow.Parameters.SpeakerID),
			IsCustomSpeaker:            cfg.Workflow.Parameters.IsCustomSpeaker,
			TtsResourceId:              optionalString(cfg.Workflow.Parameters.TTSResourceID),
			SpeechRate:                 cfg.Workflow.Parameters.SpeechRate,
			EnableSourceLanguageDetect: cfg.Workflow.Parameters.EnableSourceLanguageDetect,
			Denoise:                    cfg.Workflow.Parameters.Denoise,
		}
		if err := parameters.FromASTTranslateWorkspaceParameters(typed); err != nil {
			return rpcapi.WorkspaceCreateRequest{}, fmt.Errorf("encode ast translate workspace parameters: %w", err)
		}
	default:
		typed := rpcapi.DoubaoRealtimeWorkspaceParameters{
			AgentType:    rpcapi.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime,
			Input:        optionalWorkspaceInputMode(cfg.Workflow.Parameters.Input),
			Model:        optionalString(cfg.Workflow.Parameters.Model),
			Instructions: optionalString(cfg.Workflow.Parameters.Instructions),
			Audio:        cfg.Workflow.Parameters.Audio,
			Tools:        cfg.Workflow.Parameters.Tools,
			Extension:    cfg.Workflow.Parameters.Extension,
		}
		if err := parameters.FromDoubaoRealtimeWorkspaceParameters(typed); err != nil {
			return rpcapi.WorkspaceCreateRequest{}, fmt.Errorf("encode doubao realtime workspace parameters: %w", err)
		}
	}
	return rpcapi.WorkspaceCreateRequest{
		Name:         cfg.Workspace,
		WorkflowName: cfg.Workflow.Name,
		Parameters:   &parameters,
	}, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalWorkspaceInputMode(value string) *rpcapi.WorkspaceInputMode {
	if value == "" {
		return nil
	}
	mode := rpcapi.WorkspaceInputMode(value)
	return &mode
}

func optionalASTTranslateMode(value string) *rpcapi.ASTTranslateMode {
	if value == "" {
		return nil
	}
	mode := rpcapi.ASTTranslateMode(value)
	return &mode
}

func astTranslateWorkspaceVoiceParams(value workspaceVoiceConfig) *rpcapi.ASTTranslateVoiceParameters {
	return astTranslateVoiceParams(astTranslateVoiceConfig{
		SpeakerID:       value.SpeakerID,
		IsCustomSpeaker: value.IsCustomSpeaker,
		TTSResourceID:   value.TTSResourceID,
		SpeechRate:      value.SpeechRate,
		TTSVoice:        value.TTSVoice,
	})
}

func astTranslateVoiceParams(value astTranslateVoiceConfig) *rpcapi.ASTTranslateVoiceParameters {
	var voice rpcapi.ASTTranslateVoiceParameters
	switch {
	case value.SpeakerID != "":
		typed := rpcapi.ASTTranslateInternalSpeakerParameters{
			SpeakerId:       value.SpeakerID,
			IsCustomSpeaker: value.IsCustomSpeaker,
			TtsResourceId:   optionalString(value.TTSResourceID),
			SpeechRate:      value.SpeechRate,
		}
		if err := voice.FromASTTranslateInternalSpeakerParameters(typed); err != nil {
			return nil
		}
		return &voice
	case value.TTSVoice != "":
		if err := voice.FromASTTranslateExternalVoiceParameters(rpcapi.ASTTranslateExternalVoiceParameters{
			TtsVoice: value.TTSVoice,
		}); err != nil {
			return nil
		}
		return &voice
	default:
		return nil
	}
}

func selectAndReloadAgent(ctx context.Context, client runControlClient, cfg config) error {
	selection := rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: cfg.Workspace}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := client.SetServerRunWorkspace(ctx, "workspacetest.run.workspace.set", selection); err != nil {
			return fmt.Errorf("select workspace %q: %w", cfg.Workspace, err)
		}
		if _, err := client.ReloadServerRunWorkspace(ctx, "workspacetest.run.workspace.reload"); err != nil {
			if !isAgentAlreadyRunning(err) || time.Now().After(deadline) {
				return fmt.Errorf("reload workspace %q: %w", cfg.Workspace, err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		status, err := client.GetServerRunWorkspace(ctx, "workspacetest.run.workspace.get")
		if err != nil {
			return fmt.Errorf("get run workspace: %w", err)
		}
		if status.RuntimeState == rpcapi.PeerRunStatusStateRunning {
			if status.WorkspaceName != cfg.Workspace {
				return fmt.Errorf("running workspace = %q, want %q", status.WorkspaceName, cfg.Workspace)
			}
			return nil
		}
		if status.RuntimeState == rpcapi.PeerRunStatusStateError {
			message := ""
			if status.Message != nil {
				message = *status.Message
			}
			return fmt.Errorf("workspace %q failed to start: %s", cfg.Workspace, message)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("workspace %q did not reach running state; last=%s", cfg.Workspace, status.RuntimeState)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

type workspaceRuntimeReport struct {
	Workspace        string `json:"workspace"`
	RuntimeState     string `json:"runtime_state"`
	HistoryCount     int    `json:"history_count"`
	ReplayHistoryID  string `json:"replay_history_id"`
	ReplayState      string `json:"replay_state"`
	ReplayText       string `json:"replay_text,omitempty"`
	ReplayAudioASR   string `json:"replay_audio_asr,omitempty"`
	ReplayPackets    int    `json:"replay_packets,omitempty"`
	MemoryAvailable  bool   `json:"memory_available"`
	MemoryEnabled    bool   `json:"memory_enabled"`
	MemoryItemCount  int64  `json:"memory_item_count"`
	MemoryBytes      int64  `json:"memory_bytes"`
	RecallAvailable  bool   `json:"recall_available"`
	RecallHitCount   int    `json:"recall_hit_count"`
	RecallQueryChars int    `json:"recall_query_chars"`
}

func validateWorkspaceRuntime(ctx context.Context, driver *personaDriver, client runControlClient, cfg config, stats []roundStats) (*workspaceRuntimeReport, error) {
	if !cfg.isFlowcraftAgent() {
		return nil, nil
	}
	state, err := client.GetServerRunWorkspace(ctx, "workspacetest.runtime.workspace.get")
	if err != nil {
		return nil, fmt.Errorf("runtime rpc get workspace: %w", err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning || state.WorkspaceName != cfg.Workspace {
		if err := selectAndReloadAgent(ctx, client, cfg); err != nil {
			return nil, fmt.Errorf("runtime rpc reload workspace: %w", err)
		}
		state, err = client.GetServerRunWorkspace(ctx, "workspacetest.runtime.workspace.get")
		if err != nil {
			return nil, fmt.Errorf("runtime rpc get workspace: %w", err)
		}
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning {
		return nil, fmt.Errorf("runtime rpc workspace state = %s, want running", state.RuntimeState)
	}
	if state.WorkspaceName != cfg.Workspace {
		return nil, fmt.Errorf("runtime rpc workspace = %q, want %q", state.WorkspaceName, cfg.Workspace)
	}
	report := &workspaceRuntimeReport{
		Workspace:    state.WorkspaceName,
		RuntimeState: string(state.RuntimeState),
	}

	limit := 20
	var history *rpcapi.ServerListRunWorkspaceHistoryResponse
	historyDeadline := time.NewTimer(60 * time.Second)
	defer historyDeadline.Stop()
	for {
		history, err = client.ListServerRunWorkspaceHistory(ctx, "workspacetest.runtime.history", rpcapi.ServerListRunWorkspaceHistoryRequest{Limit: &limit})
		if err != nil {
			return nil, fmt.Errorf("runtime rpc history: %w", err)
		}
		if !history.Available {
			message := ""
			if history.Message != nil {
				message = *history.Message
			}
			return nil, fmt.Errorf("runtime rpc history unavailable: %s", message)
		}
		if len(history.Items) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("runtime rpc history returned no items: %w", ctx.Err())
		case <-historyDeadline.C:
			return nil, fmt.Errorf("runtime rpc history returned no items")
		case <-time.After(500 * time.Millisecond):
		}
	}

	memory, err := client.GetServerRunWorkspaceMemoryStats(ctx, "workspacetest.runtime.memory.stats", rpcapi.ServerGetRunWorkspaceMemoryStatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("runtime rpc memory stats: %w", err)
	}

	query := runtimeRecallQuery(stats)
	recallAvailable := false
	recallHitCount := 0
	if memory.Available && memory.Enabled {
		recall, err := client.ServerRunWorkspaceRecall(ctx, "workspacetest.runtime.recall", rpcapi.ServerRunWorkspaceRecallRequest{Query: query})
		if err != nil {
			return nil, fmt.Errorf("runtime rpc recall: %w", err)
		}
		if !recall.Available {
			message := ""
			if recall.Message != nil {
				message = *recall.Message
			}
			return nil, fmt.Errorf("runtime rpc recall unavailable: %s", message)
		}
		recallAvailable = recall.Available
		recallHitCount = len(recall.Hits)
	}

	historyItem, ok := replayHistoryItem(history.Items)
	if !ok {
		return nil, fmt.Errorf("runtime rpc history has no replayable agent item")
	}
	if driver != nil {
		driver.drainTransport()
	}
	play, err := client.PlayServerRunWorkspaceHistory(ctx, "workspacetest.runtime.history.play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{
		HistoryId: historyItem.Id,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime rpc history play: %w", err)
	}
	if !play.Accepted {
		message := ""
		if play.Message != nil {
			message = *play.Message
		}
		return nil, fmt.Errorf("runtime rpc history play rejected state=%s: %s", play.State, message)
	}

	report.HistoryCount = len(history.Items)
	report.ReplayHistoryID = historyItem.Id
	report.ReplayState = play.State
	if driver != nil {
		replay, err := driver.verifyHistoryReplay(ctx, historyItem)
		if err != nil {
			return nil, fmt.Errorf("runtime rpc history replay output: %w", err)
		}
		report.ReplayText = replay.Text
		report.ReplayAudioASR = replay.AudioASR
		report.ReplayPackets = replay.DownlinkPackets
	}
	report.MemoryAvailable = memory.Available
	report.MemoryEnabled = memory.Enabled
	report.MemoryItemCount = memory.ItemCount
	report.MemoryBytes = memory.StorageBytes
	report.RecallAvailable = recallAvailable
	report.RecallHitCount = recallHitCount
	report.RecallQueryChars = runeCount(query)
	return report, nil
}

func runtimeRecallQuery(stats []roundStats) string {
	for _, stat := range stats {
		if text := strings.TrimSpace(stat.Transcript); text != "" {
			return text
		}
		if text := strings.TrimSpace(stat.UserText); text != "" {
			return text
		}
	}
	return "你好"
}

func replayHistoryItem(items []rpcapi.PeerRunHistoryEntry) (rpcapi.PeerRunHistoryEntry, bool) {
	for _, item := range items {
		if item.Type == rpcapi.PeerRunHistoryEntryTypeAgent && item.ReplayAvailable && strings.TrimSpace(item.Text) != "" {
			return item, true
		}
	}
	for _, item := range items {
		if item.Type == rpcapi.PeerRunHistoryEntryTypeAgent && item.ReplayAvailable {
			return item, true
		}
	}
	return rpcapi.PeerRunHistoryEntry{}, false
}

func boolPtr(value bool) *bool {
	return &value
}

func printRunSummary(cfg config, stats []roundStats) {
	fmt.Printf("server=%s workflow=%s workspace=%s agent=%s rounds=%d output_dir=%s\n", cfg.Server.Addr, cfg.Workflow.Name, cfg.Workspace, cfg.Agent, cfg.Rounds, cfg.OutputDir)
	for _, stat := range stats {
		fmt.Printf("round=%d user_chars=%d transcript_chars=%d assistant_chars=%d input_packets=%d input_bytes=%d downlink_packets=%d downlink_bytes=%d events=%d workspace_uplink_send=%s after_eos_transcript_start=%s after_eos_transcript_done=%s transcript_first_before_eos=%t after_eos_text_first_chunk=%s assistant_text_done=%s text_first_after_transcript_done=%s after_eos_audio_first_chunk=%s audio_first_before_text_done=%t after_eos_complete=%s workspace_total=%s\n",
			stat.Index,
			runeCount(stat.UserText),
			runeCount(stat.Transcript),
			runeCount(stat.AssistantText),
			stat.InputOpusPackets,
			stat.InputOpusBytes,
			stat.DownlinkPackets,
			stat.DownlinkBytes,
			stat.EventCount,
			stat.UplinkSend.Round(time.Millisecond),
			stat.FirstTranscriptChunk.Round(time.Millisecond),
			stat.TranscriptDone.Round(time.Millisecond),
			stat.FirstTranscriptBeforeEOS,
			stat.FirstAssistantTextChunk.Round(time.Millisecond),
			stat.AssistantTextDone.Round(time.Millisecond),
			textAfterTranscriptDone(stat).Round(time.Millisecond),
			stat.FirstAudioChunk.Round(time.Millisecond),
			stat.FirstAudioBeforeTextDone,
			stat.ResponseTotal.Round(time.Millisecond),
			stat.WorkspaceTotal.Round(time.Millisecond),
		)
		fmt.Printf("round_detail=%s\n", encodeJSONLine(map[string]string{
			"user":                  stat.UserText,
			"transcript":            stat.Transcript,
			"assistant_first_delta": stat.FirstAssistantText,
			"assistant":             stat.AssistantText,
			"assistant_audio_asr":   stat.AssistantAudioASR,
		}))
	}
	fmt.Printf("timing_summary=%s\n", encodeJSONLine(roundTimingSummary(stats)))
}

func printWorkspaceRuntimeReport(report workspaceRuntimeReport) {
	fmt.Printf("workspace_runtime=%s\n", encodeJSONLine(report))
}

func printInterruptSummary(stat interruptStats) {
	fmt.Printf("interrupt=%s\n", encodeJSONLine(map[string]interface{}{
		"round":                      stat.Index,
		"first_user":                 stat.FirstUser,
		"second_user":                stat.SecondUser,
		"downlink_before_interrupt":  stat.DownlinkBeforeInterrupt,
		"interrupted_after_ms":       float64(stat.InterruptedAfter.Microseconds()) / 1000,
		"interrupted_stream_id":      stat.InterruptedStreamID,
		"second_transcript":          stat.SecondTranscript,
		"second_assistant":           stat.SecondAssistantText,
		"second_assistant_audio_asr": stat.SecondAssistantAudioASR,
		"second_downlink_packets":    stat.SecondDownlinkPackets,
		"second_transcript_done_ms":  float64(stat.SecondTranscriptDone.Microseconds()) / 1000,
		"second_text_first_ms":       float64(stat.SecondFirstText.Microseconds()) / 1000,
		"second_text_done_ms":        float64(stat.SecondAssistantTextDone.Microseconds()) / 1000,
		"second_audio_first_ms":      float64(stat.SecondFirstAudio.Microseconds()) / 1000,
		"second_audio_done_ms":       float64(stat.SecondAudioDone.Microseconds()) / 1000,
		"second_response_total_ms":   float64(stat.SecondResponseTotal.Microseconds()) / 1000,
	}))
}

type timingSummary struct {
	Count int     `json:"count"`
	MinMS float64 `json:"min_ms"`
	AvgMS float64 `json:"avg_ms"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	MaxMS float64 `json:"max_ms"`
}

func roundTimingSummary(stats []roundStats) map[string]timingSummary {
	return map[string]timingSummary{
		"workspace_uplink_send":            summarizeDurations(stats, func(s roundStats) time.Duration { return s.UplinkSend }),
		"after_eos_transcript_first":       summarizeDurations(stats, func(s roundStats) time.Duration { return s.FirstTranscriptChunk }),
		"after_eos_transcript_start":       summarizeDurations(stats, func(s roundStats) time.Duration { return s.FirstTranscriptChunk }),
		"after_eos_transcript_done":        summarizeDurations(stats, func(s roundStats) time.Duration { return s.TranscriptDone }),
		"after_eos_text_first":             summarizeDurations(stats, func(s roundStats) time.Duration { return s.FirstAssistantTextChunk }),
		"assistant_text_done":              summarizeDurations(stats, func(s roundStats) time.Duration { return s.AssistantTextDone }),
		"text_first_after_transcript_done": summarizeDurations(stats, textAfterTranscriptDone),
		"after_eos_audio_first":            summarizeDurations(stats, func(s roundStats) time.Duration { return s.FirstAudioChunk }),
		"after_eos_complete":               summarizeDurations(stats, func(s roundStats) time.Duration { return s.ResponseTotal }),
		"workspace_total_including_send":   summarizeDurations(stats, func(s roundStats) time.Duration { return s.WorkspaceTotal }),
	}
}

func textAfterTranscriptDone(stat roundStats) time.Duration {
	if stat.TranscriptDone <= 0 || stat.FirstAssistantTextChunk <= 0 {
		return 0
	}
	delta := stat.FirstAssistantTextChunk - stat.TranscriptDone
	if delta <= 0 {
		return 0
	}
	return delta
}

func summarizeDurations(stats []roundStats, pick func(roundStats) time.Duration) timingSummary {
	values := make([]float64, 0, len(stats))
	for _, stat := range stats {
		value := pick(stat)
		if value <= 0 {
			continue
		}
		values = append(values, durationMilliseconds(value))
	}
	if len(values) == 0 {
		return timingSummary{}
	}
	sort.Float64s(values)
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return timingSummary{
		Count: len(values),
		MinMS: values[0],
		AvgMS: sum / float64(len(values)),
		P50MS: percentile(values, 0.50),
		P95MS: percentile(values, 0.95),
		MaxMS: values[len(values)-1],
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	index := int(p*float64(len(sorted)-1) + 0.5)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func durationMilliseconds(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}
