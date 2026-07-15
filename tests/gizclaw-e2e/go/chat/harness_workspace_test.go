//go:build gizclaw_e2e

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/goccy/go-yaml"
)

func TestFetchChatServerInfoIncludesICEServers(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"protocol":"gizclaw-webrtc","public_key":%q,"signaling_path":"/webrtc/v1/offer","ice_servers":[{"urls":["turn:edge.example.com:3478"],"username":"edge","credential":"secret"}]}`, serverKey.Public.String())
	}))
	defer server.Close()

	info, err := fetchChatServerInfo(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("fetchChatServerInfo error = %v", err)
	}
	if !info.PublicKey.Equal(serverKey.Public) || info.SignalingURL != server.URL+"/webrtc/v1/offer" {
		t.Fatalf("server info = %+v", info)
	}
	if len(info.ICEServers) != 1 || len(info.ICEServers[0].URLs) != 1 ||
		info.ICEServers[0].URLs[0] != "turn:edge.example.com:3478" ||
		info.ICEServers[0].Username != "edge" || info.ICEServers[0].Credential != "secret" {
		t.Fatalf("ICE servers = %+v", info.ICEServers)
	}
}

func TestWorkspaceCaseAppliesInputMode(t *testing.T) {
	cfg := config{Workflow: workflowConfig{Name: "demo.workflow", Parameters: workspaceParameterConfig{Input: "push-to-talk"}}}
	got, err := workspaceCaseRealtimeRoundtrip.applyConfig(cfg)
	if err != nil {
		t.Fatalf("applyConfig(realtime) error = %v", err)
	}
	if got.workspaceMode() != "realtime" {
		t.Fatalf("realtime workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-rt" {
		t.Fatalf("realtime workspace = %q", got.Workspace)
	}
	got, err = workspaceCaseRealtimeAutoSplit.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(realtime-auto-split-history) error = %v", err)
	}
	if got.workspaceMode() != "realtime" {
		t.Fatalf("realtime auto split workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-rt-auto" {
		t.Fatalf("realtime auto split workspace = %q", got.Workspace)
	}
	got, err = workspaceCasePushToTalkInterrupt.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(push) error = %v", err)
	}
	if got.workspaceMode() != "push_to_talk" {
		t.Fatalf("push workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-ptt-int" {
		t.Fatalf("push workspace = %q", got.Workspace)
	}
	got, err = workspaceCaseHistoryReplay.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(history-replay) error = %v", err)
	}
	if got.workspaceMode() != "push_to_talk" {
		t.Fatalf("history-replay workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-hist" {
		t.Fatalf("history-replay workspace = %q", got.Workspace)
	}
	got, err = workspaceCaseHumanReview.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(human-review) error = %v", err)
	}
	if got.workspaceMode() != "push_to_talk" {
		t.Fatalf("human-review workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-review" {
		t.Fatalf("human-review workspace = %q", got.Workspace)
	}
}

func TestWorkspaceNameForCaseFitsCustomIDLimit(t *testing.T) {
	name := workspaceNameForCase("flowcraft-poetry-adventure-li-bai", workspaceCaseRealtimeAutoSplit)
	if len(name) > 48 {
		t.Fatalf("workspace name length = %d, want <= 48: %q", len(name), name)
	}
}

func TestRealtimeAutoSplitHistoryReplayPolicy(t *testing.T) {
	doubao := &personaDriver{cfg: config{Agent: "Doubao-Realtime"}}
	if doubao.realtimeAutoSplitRequiresReplay() {
		t.Fatal("doubao realtime auto split should not require user history replay audio")
	}
	ast := &personaDriver{cfg: config{Agent: "ast-translate"}}
	if !ast.realtimeAutoSplitRequiresReplay() {
		t.Fatal("ast translate auto split should require user history replay audio")
	}

	items := []rpcapi.PeerRunHistoryEntry{
		{Id: "old", Name: "transcript", Text: "旧消息", Type: rpcapi.PeerRunHistoryEntryTypeGear, ReplayAvailable: true},
		{Id: "text-only", Name: "transcript", Text: "第一段", Type: rpcapi.PeerRunHistoryEntryTypeGear, ReplayAvailable: false},
		{Id: "replayable", Name: "transcript", Text: "第二段", Type: rpcapi.PeerRunHistoryEntryTypeGear, ReplayAvailable: true},
		{Id: "agent", Name: "assistant", Text: "回复", Type: rpcapi.PeerRunHistoryEntryTypeAgent, ReplayAvailable: true},
	}
	before := map[string]struct{}{"old": {}}
	textOnlyAllowed := filterRealtimeAutoSplitGearHistory(items, before, false)
	if len(textOnlyAllowed) != 2 || textOnlyAllowed[0].Id != "text-only" || textOnlyAllowed[1].Id != "replayable" {
		t.Fatalf("text-only filter = %#v, want text-only and replayable", textOnlyAllowed)
	}
	replayRequired := filterRealtimeAutoSplitGearHistory(items, before, true)
	if len(replayRequired) != 1 || replayRequired[0].Id != "replayable" {
		t.Fatalf("replay-required filter = %#v, want replayable only", replayRequired)
	}
	if !isRealtimeAutoSplitIgnoredEventError("interrupted") {
		t.Fatal("realtime auto split should ignore assistant interrupted events")
	}
	if isRealtimeAutoSplitIgnoredEventError("other") {
		t.Fatal("realtime auto split ignored a non-interrupt error")
	}
}

func TestMatchRealtimeAutoSplitHistoryRequiresOrder(t *testing.T) {
	items := []rpcapi.PeerRunHistoryEntry{
		{Id: "2", Text: "klmnopqrst"},
		{Id: "1", Text: "abcdefghij"},
	}
	_, err := matchRealtimeAutoSplitHistory([]string{"abcdefghij", "klmnopqrst"}, items)
	if err == nil {
		t.Fatal("matchRealtimeAutoSplitHistory accepted out-of-order history")
	}
}

func TestMatchRealtimeAutoSplitHistoryAllowsExtraEntriesBetweenSegments(t *testing.T) {
	items := []rpcapi.PeerRunHistoryEntry{
		{Id: "1", Text: "第一段自动切分测试"},
		{Id: "extra", Text: "中间插入的其他历史"},
		{Id: "2", Text: "第二段自动切分测试"},
	}
	matched, err := matchRealtimeAutoSplitHistory([]string{"第一段自动切分测试", "第二段自动切分测试"}, items)
	if err != nil {
		t.Fatalf("matchRealtimeAutoSplitHistory() error = %v", err)
	}
	if len(matched) != 2 || matched[0].Id != "1" || matched[1].Id != "2" {
		t.Fatalf("matched = %#v, want ordered expected entries", matched)
	}
}

func TestWorkspaceCaseDispatchRejectsUnknown(t *testing.T) {
	_, err := (&personaDriver{}).runCase(context.Background(), workspaceCase("unknown"))
	if err == nil || !strings.Contains(err.Error(), "unsupported workspace case") {
		t.Fatalf("runCase(unknown) error = %v", err)
	}
}

func TestInterruptRoundsDefaultToOne(t *testing.T) {
	d := &personaDriver{cfg: config{Rounds: 3}}
	if got := d.interruptRoundCount(); got != 1 {
		t.Fatalf("interruptRoundCount() = %d, want 1", got)
	}
	d.cfg.Interrupt.Rounds = 2
	if got := d.interruptRoundCount(); got != 2 {
		t.Fatalf("interruptRoundCount(explicit) = %d, want 2", got)
	}
}

func TestInterruptWorkspaceConfigPathsIncludeExternalTTS(t *testing.T) {
	paths := interruptWorkspaceConfigPaths(t)
	var found bool
	for _, path := range paths {
		if filepath.Base(path) == "ast-translate-tts.json" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("interrupt configs exclude external TTS fixture")
	}
}

func TestRetryableLiveWorkspaceError(t *testing.T) {
	retryable := []error{
		errors.New("flowcraft: read ASR: buffer: read from closed buffer: websocket connect failed: Bad Gateway"),
		errors.New("flowcraft: read ASR: buffer: read from closed buffer: websocket read: unexpected EOF"),
		errors.New("ast websocket read: websocket: close 1006 (abnormal closure): unexpected EOF"),
		errors.New("round 2: transport: timeout; recent events: none"),
		errors.New("bytedance: response incomplete: length"),
		errors.New("buffer: read from closed buffer: genx: generate error: flowcraft: claw event error: recall ingest: extract: recall two-pass extractor: content llm: bytedance.generate: 15.007s"),
		errors.New("speech: POST \"http://gizclaw/v1/audio/speech\": 400 Bad Request"),
		errors.New("peer event error: buffer: read from closed buffer: doubaospeech: [Server processing timeout] node execution timeout (code=55001010)"),
		errors.New("peer event error: buffer: read from closed buffer: doubaospeech: [Server-side generic error] OperatorWrapper Process failed: big asr recv err. rpc timeout: CallWithTimeout: timeout in business code, timeout_config=3s"),
		errors.New("interrupt second transcript mismatch: similarity 0.21 below 0.45"),
	}
	for _, err := range retryable {
		if !isRetryableLiveWorkspaceError(err) {
			t.Fatalf("isRetryableLiveWorkspaceError(%q) = false", err)
		}
	}
	notRetryable := []error{
		nil,
		errors.New("read context config: no such file or directory"),
		errors.New("client private key: invalid key"),
		errors.New("interrupt missing second transcript"),
		errors.New("context deadline exceeded"),
	}
	for _, err := range notRetryable {
		if isRetryableLiveWorkspaceError(err) {
			t.Fatalf("isRetryableLiveWorkspaceError(%v) = true", err)
		}
	}
}

func TestHistoryReplayStreamHelpers(t *testing.T) {
	stream := "history-replay-1"
	other := "assistant-live"
	if !acceptHistoryReplayStream(apitypes.PeerStreamEvent{StreamId: &stream}, nil) {
		t.Fatal("history replay stream should be accepted without binding")
	}
	if acceptHistoryReplayStream(apitypes.PeerStreamEvent{StreamId: &other}, nil) {
		t.Fatal("non-history stream should not be accepted without binding")
	}
	var bound string
	if !acceptHistoryReplayStream(apitypes.PeerStreamEvent{StreamId: &stream}, &bound) || bound != stream {
		t.Fatalf("first bound stream = %q", bound)
	}
	if !acceptHistoryReplayStream(apitypes.PeerStreamEvent{StreamId: &stream}, &bound) {
		t.Fatal("same bound stream should be accepted")
	}
	if acceptHistoryReplayStream(apitypes.PeerStreamEvent{StreamId: &other}, &bound) {
		t.Fatal("different bound stream should be rejected")
	}
	if !acceptHistoryReplayStream(apitypes.PeerStreamEvent{}, &bound) {
		t.Fatal("missing stream id should be accepted for compatibility")
	}
	if got := totalFrameBytes([][]byte{{1, 2}, nil, {3, 4, 5}}); got != 5 {
		t.Fatalf("totalFrameBytes() = %d, want 5", got)
	}
}

func TestWorkflowSpecCoversTypedAgentSpecs(t *testing.T) {
	flowcraft := workflowSpec(config{
		Agent:  "flowcraft",
		Voice:  "voice-a",
		Models: modelConfig{ASR: "asr-a"},
		Workflow: workflowConfig{
			Flowcraft: map[string]interface{}{"agent": map[string]interface{}{"id": "demo"}},
		},
	})
	if flowcraft.Driver != rpcapi.WorkflowDriverFlowcraft || flowcraft.Flowcraft == nil {
		t.Fatalf("flowcraft spec = %+v", flowcraft)
	}
	if _, ok := (*flowcraft.Flowcraft)["voice_adapter"]; !ok {
		t.Fatalf("flowcraft voice adapter missing = %+v", *flowcraft.Flowcraft)
	}

	customSpeaker := true
	speechRate := 12
	ast := workflowSpec(config{
		Agent: "ast-translate",
		Workflow: workflowConfig{
			Translation: "translate-model",
			ASTTranslate: astTranslateConfig{
				Mode:            "s2s",
				Voice:           astTranslateVoiceConfig{SpeakerID: "speaker", IsCustomSpeaker: &customSpeaker, TTSResourceID: "tts", SpeechRate: &speechRate},
				SpeakerID:       "fallback-speaker",
				IsCustomSpeaker: &customSpeaker,
				TTSResourceID:   "fallback-tts",
				SpeechRate:      &speechRate,
			},
		},
	})
	if ast.Driver != rpcapi.WorkflowDriverAstTranslate || ast.AstTranslate == nil || ast.AstTranslate.Voice == nil {
		t.Fatalf("ast spec = %+v", ast)
	}
	if ast.AstTranslate.Mode == nil || *ast.AstTranslate.Mode != rpcapi.ASTTranslateModeS2s {
		t.Fatalf("ast mode = %#v", ast.AstTranslate.Mode)
	}

	realtime := workflowSpec(config{Workflow: workflowConfig{Model: "rt", Audio: defaultDoubaoRealtimeAudio()}})
	if realtime.Driver != rpcapi.WorkflowDriverDoubaoRealtime || realtime.DoubaoRealtime == nil || realtime.DoubaoRealtime.Model != "rt" || realtime.DoubaoRealtime.Audio == nil {
		t.Fatalf("realtime spec = %+v", realtime)
	}
}

func TestSetupWorkflowResourcesCoverWorkspaceConfigs(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "workspaces", "*.json"))
	if err != nil {
		t.Fatalf("glob configs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("workspace configs are missing")
	}
	resources := loadSetupWorkflowResources(t)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			var cfg config
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("decode config: %v", err)
			}
			wantName := cfg.Workflow.Name
			wantSpec := workflowSpec(cfg)
			resource, ok := resources[wantName]
			if !ok {
				t.Fatalf("setup workflow resource for %q is missing", wantName)
			}
			if resource.APIVersion != "gizclaw.admin/v1alpha1" || resource.Kind != "Workflow" {
				t.Fatalf("resource header = %s/%s", resource.APIVersion, resource.Kind)
			}
			if resource.Metadata.Name != wantName {
				t.Fatalf("resource workflow name = %q, want %q", resource.Metadata.Name, wantName)
			}
			gotSpec, err := json.Marshal(resource.Spec)
			if err != nil {
				t.Fatalf("marshal resource spec: %v", err)
			}
			wantSpecJSON, err := json.Marshal(wantSpec)
			if err != nil {
				t.Fatalf("marshal expected spec: %v", err)
			}
			if string(gotSpec) != string(wantSpecJSON) {
				t.Fatalf("setup workflow spec drifted\nresource=%s\nwant=%s", gotSpec, wantSpecJSON)
			}
		})
	}
}

type setupWorkflowResource struct {
	APIVersion string                    `json:"apiVersion"`
	Kind       string                    `json:"kind"`
	Metadata   apitypes.ResourceMetadata `json:"metadata"`
	Spec       rpcapi.WorkflowSpec       `json:"spec"`
}

func loadSetupWorkflowResources(t *testing.T) map[string]setupWorkflowResource {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "resources", "04-workflows", "*.yaml"))
	if err != nil {
		t.Fatalf("glob workflow resources: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("setup workflow resources are missing")
	}
	resources := make(map[string]setupWorkflowResource)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read setup workflow resource %s: %v", path, err)
		}
		jsonData, err := yaml.YAMLToJSON(data)
		if err != nil {
			t.Fatalf("convert setup workflow resource %s: %v", path, err)
		}
		var resource setupWorkflowResource
		if err := json.Unmarshal(jsonData, &resource); err != nil {
			t.Fatalf("decode setup workflow resource %s: %v", path, err)
		}
		hydrateSetupWorkflowResourceUnions(t, path, jsonData, &resource)
		if resource.Kind != "Workflow" {
			continue
		}
		resources[resource.Metadata.Name] = resource
	}
	return resources
}

func hydrateSetupWorkflowResourceUnions(t *testing.T, path string, data []byte, resource *setupWorkflowResource) {
	t.Helper()
	ast := resource.Spec.AstTranslate
	if ast == nil || ast.Voice == nil || ast.Voice.Value != nil {
		return
	}
	var raw struct {
		Spec struct {
			AstTranslate struct {
				Voice map[string]json.RawMessage `json:"voice"`
			} `json:"ast_translate"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode raw setup workflow resource %s: %v", path, err)
	}
	voiceData, err := json.Marshal(raw.Spec.AstTranslate.Voice)
	if err != nil {
		t.Fatalf("marshal raw ast translate voice %s: %v", path, err)
	}
	switch {
	case raw.Spec.AstTranslate.Voice["tts_voice"] != nil:
		var voice rpcapi.ASTTranslateExternalVoiceParameters
		if err := json.Unmarshal(voiceData, &voice); err != nil {
			t.Fatalf("decode external ast translate voice %s: %v", path, err)
		}
		if err := ast.Voice.FromASTTranslateExternalVoiceParameters(voice); err != nil {
			t.Fatalf("hydrate external ast translate voice %s: %v", path, err)
		}
	case raw.Spec.AstTranslate.Voice["speaker_id"] != nil:
		var voice rpcapi.ASTTranslateInternalSpeakerParameters
		if err := json.Unmarshal(voiceData, &voice); err != nil {
			t.Fatalf("decode internal ast translate voice %s: %v", path, err)
		}
		if err := ast.Voice.FromASTTranslateInternalSpeakerParameters(voice); err != nil {
			t.Fatalf("hydrate internal ast translate voice %s: %v", path, err)
		}
	}
}

func TestPrintWorkspaceRuntimeAndInterruptSummaries(t *testing.T) {
	output := captureStdout(t, func() {
		printWorkspaceRuntimeReport(workspaceRuntimeReport{Workspace: "ws", RuntimeState: "running", HistoryCount: 2})
		printInterruptSummary(interruptStats{Index: 1, FirstUser: "a", SecondUser: "b", SecondDownlinkPackets: 3})
	})
	if !strings.Contains(output, "workspace_runtime=") || !strings.Contains(output, "interrupt=") {
		t.Fatalf("summary output = %q", output)
	}
}

func TestRunWiresClientTransportAndPersonaDriver(t *testing.T) {
	restoreRunHooks(t)
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	contextConfigPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`{
  "agent": "doubao-realtime",
  "workflow": {
    "name": "doubao-realtime-workflow",
    "model": "realtime"
  },
  "models": {
    "llm": "chat",
    "tts": "tts",
    "asr": "asr",
    "realtime": "realtime"
  },
  "voice": "voice",
  "rounds": 1,
  "timeout": "1s",
  "persona": "persona"
}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	writeSetupContextConfig(t, contextConfigPath, serverKey, clientKey, "")

	var dialed, ensured, selected, transported, ran bool
	serveDone := make(chan error, 1)
	serveDone <- nil
	dialClientForRun = func(cfg config) (*gizcli.Client, <-chan error, error) {
		dialed = true
		if cfg.Workspace != "doubao-realtime-workflow-ptt" || cfg.Models.LLM != "chat" {
			t.Fatalf("dial cfg = %+v", cfg)
		}
		return &gizcli.Client{}, serveDone, nil
	}
	ensureWorkspaceForRun = func(ctx context.Context, client *gizcli.Client, cfg config) (config, error) {
		ensured = true
		if cfg.Workflow.Name != "doubao-realtime-workflow" || cfg.Models.Realtime != "realtime" {
			t.Fatalf("ensure cfg = %+v", cfg)
		}
		if cfg.workspaceMode() != "push_to_talk" {
			t.Fatalf("ensure workspace mode = %q", cfg.workspaceMode())
		}
		return cfg, nil
	}
	selectAndReloadAgentForRun = func(ctx context.Context, client *gizcli.Client, cfg config) error {
		selected = true
		if err := ctx.Err(); err != nil {
			t.Fatalf("ctx error before select: %v", err)
		}
		return nil
	}
	newChatTransportForRun = func(client *gizcli.Client) (*chatTransport, error) {
		transported = true
		return &chatTransport{}, nil
	}
	runWorkspaceCaseForRun = func(driver *personaDriver, ctx context.Context, selected workspaceCase) (workspaceCaseResult, error) {
		ran = true
		if selected != workspaceCasePushToTalkRoundtrip {
			t.Fatalf("selected case = %q", selected)
		}
		if driver.cfg.Voice != "voice" {
			t.Fatalf("driver = %+v", driver)
		}
		if driver.newTransport == nil {
			t.Fatalf("driver newTransport is nil")
		}
		if driver.reloadAgent == nil {
			t.Fatalf("driver reloadAgent is nil")
		}
		if err := driver.reloadAgent(ctx); err != nil {
			t.Fatalf("reloadAgent() error = %v", err)
		}
		if err := driver.resetTransport(); err != nil {
			t.Fatalf("resetTransport() error = %v", err)
		}
		if driver.transport == nil {
			t.Fatalf("driver transport is nil after reset")
		}
		return workspaceCaseResult{Rounds: []roundStats{{Index: 1, UserText: "你好", Transcript: "你好", AssistantText: "收到", DownlinkPackets: 1}}}, nil
	}

	output := captureStdout(t, func() {
		if err := runConfig(configPath, contextConfigPath, workspaceCasePushToTalkRoundtrip); err != nil {
			t.Fatalf("runConfig() error = %v", err)
		}
	})
	if !dialed || !ensured || !selected || !transported || !ran {
		t.Fatalf("hooks dial/ensure/select/transport/run = %t/%t/%t/%t/%t", dialed, ensured, selected, transported, ran)
	}
	if !strings.Contains(output, "workspace=doubao-realtime-workflow-ptt") || !strings.Contains(output, "round=1") {
		t.Fatalf("run output = %q", output)
	}
}

func TestRunSkipsEnsureWorkspaceWhenDisabled(t *testing.T) {
	restoreRunHooks(t)
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	contextConfigPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`{
  "agent": "flowcraft",
  "ensure_workspace": false,
  "models": {
    "llm": "chat",
    "tts": "tts",
    "asr": "asr"
  },
  "workflow": {
    "name": "flowcraft-journey-guide"
  },
  "voice": "voice",
  "rounds": 1,
  "timeout": "1s",
  "persona": "persona"
}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	writeSetupContextConfig(t, contextConfigPath, serverKey, clientKey, "")

	serveDone := make(chan error, 1)
	serveDone <- nil
	dialClientForRun = func(config) (*gizcli.Client, <-chan error, error) {
		return &gizcli.Client{}, serveDone, nil
	}
	ensureWorkspaceForRun = func(_ context.Context, _ *gizcli.Client, cfg config) (config, error) {
		t.Fatal("ensureWorkspaceForRun was called")
		return cfg, nil
	}
	runWorkspaceCaseForRun = func(*personaDriver, context.Context, workspaceCase) (workspaceCaseResult, error) {
		return workspaceCaseResult{Rounds: []roundStats{{Index: 1, UserText: "你好", Transcript: "你好", AssistantText: "收到"}}}, nil
	}
	validateWorkspaceRuntimeForRun = func(context.Context, *personaDriver, runControlClient, config, []roundStats, workspaceRuntimeValidationOptions) (*workspaceRuntimeReport, error) {
		return nil, nil
	}
	if err := runConfig(configPath, contextConfigPath, workspaceCasePushToTalkRoundtrip); err != nil {
		t.Fatalf("runConfig() error = %v", err)
	}
}

func TestDialClientRejectsInvalidPrivateKey(t *testing.T) {
	_, _, err := dialClient(config{ClientPrivateKey: "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid key text") {
		t.Fatalf("dialClient() error = %v", err)
	}
}

func TestEnsureWorkspaceRequiresSetupWorkflowAndRecreatesWorkspace(t *testing.T) {
	control := &fakeRunControl{}
	audio := defaultDoubaoRealtimeAudio()
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Models:    modelConfig{Realtime: "realtime"},
		Workflow: workflowConfig{
			Name:  "workflow-a",
			Model: "realtime",
			Audio: audio,
			Parameters: workspaceParameterConfig{
				Input: "realtime",
				Model: "realtime",
				Audio: audio,
			},
		},
	}
	ensured, err := ensureWorkspace(context.Background(), control, cfg)
	if err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.getWorkflow.Name != "workflow-a" {
		t.Fatalf("get workflow = %+v", control.getWorkflow)
	}
	if !control.stopped {
		t.Fatal("server run was not stopped before workspace recreate")
	}
	if control.deletedWorkspace != "workspace-a" {
		t.Fatalf("deleted workspace = %q", control.deletedWorkspace)
	}
	if ensured.Workflow.Name != "workflow-a" {
		t.Fatalf("ensured workflow name = %q", ensured.Workflow.Name)
	}
	if control.createdWorkspace.Name != "workspace-a" || control.createdWorkspace.WorkflowName != "workflow-a" {
		t.Fatalf("created workspace = %+v", control.createdWorkspace)
	}
	if ensured.Workspace != "workspace-a" {
		t.Fatalf("ensured workspace name = %q", ensured.Workspace)
	}
	if control.createdWorkspace.Parameters == nil {
		t.Fatalf("workspace parameters = %#v", control.createdWorkspace.Parameters)
	}
	params, err := control.createdWorkspace.Parameters.AsDoubaoRealtimeWorkspaceParameters()
	if err != nil {
		t.Fatalf("workspace parameters decode error = %v", err)
	}
	if params.AgentType != rpcapi.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime ||
		params.Model == nil || *params.Model != "realtime" ||
		params.Input == nil || *params.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("workspace parameters = %#v", params)
	}
	if params.Audio == nil || params.Audio.Output.Voice == nil || *params.Audio.Output.Voice != "zh_female_vv_jupiter_bigtts" {
		t.Fatalf("workspace audio parameters = %#v", params.Audio)
	}
}

func TestEnsureWorkspaceIgnoresMissingWorkspaceDelete(t *testing.T) {
	control := &fakeRunControl{
		deleteWorkspaceErr: rpcapi.Error{Code: rpcapi.RPCErrorCodeNotFound, Message: "workspace missing"},
	}
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	}
	if _, err := ensureWorkspace(context.Background(), control, cfg); err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.deletedWorkspace != "workspace-a" || control.createdWorkspace.Name != "workspace-a" {
		t.Fatalf("deleted/created workspace = %q/%+v", control.deletedWorkspace, control.createdWorkspace)
	}
}

func TestEnsureWorkspaceAlwaysRecreatesWorkspace(t *testing.T) {
	control := &fakeRunControl{}
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	}
	ensured, err := ensureWorkspace(context.Background(), control, cfg)
	if err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.getWorkflow.Name != "workflow-a" {
		t.Fatalf("get workflow = %+v", control.getWorkflow)
	}
	if control.deletedWorkspace != "workspace-a" {
		t.Fatalf("deleted workspace = %q", control.deletedWorkspace)
	}
	if control.createdWorkspace.Name != "workspace-a" || control.createdWorkspace.WorkflowName != "workflow-a" {
		t.Fatalf("created workspace = %+v", control.createdWorkspace)
	}
	if ensured.Workflow.Name != "workflow-a" || ensured.Workspace != "workspace-a" {
		t.Fatalf("ensured config = %+v", ensured)
	}
}

func TestEnsureWorkspaceReturnsGetWorkflowErrors(t *testing.T) {
	control := &fakeRunControl{getWorkflowErr: errors.New("denied")}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "get workflow") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestEnsureWorkspaceReturnsSetupHintWhenWorkflowMissing(t *testing.T) {
	control := &fakeRunControl{getWorkflowErr: rpcapi.Error{Code: rpcapi.RPCErrorCodeNotFound, Message: "missing"}}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "docker-compose-up.sh") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestEnsureWorkspaceReturnsStopErrors(t *testing.T) {
	control := &fakeRunControl{stopErr: errors.New("busy")}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "stop active workspace") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestEnsureWorkspaceReturnsDeleteErrors(t *testing.T) {
	control := &fakeRunControl{deleteWorkspaceErr: errors.New("denied")}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "delete workspace") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestEnsureWorkspaceReturnsCreateErrors(t *testing.T) {
	control := &fakeRunControl{createWorkspaceErr: errors.New("denied")}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", Model: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "create workspace") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestSelectAndReloadAgentReachesRunningWorkspace(t *testing.T) {
	workspace := "doubao-realtime"
	control := &fakeRunControl{
		workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{{
			RuntimeState:  rpcapi.PeerRunStatusStateRunning,
			WorkspaceName: workspace,
		}},
	}
	if err := selectAndReloadAgent(context.Background(), control, config{Workspace: workspace}); err != nil {
		t.Fatalf("selectAndReloadAgent() error = %v", err)
	}
	if control.selectedWorkspace != workspace {
		t.Fatalf("selected workspace = %q", control.selectedWorkspace)
	}
	if !control.reloaded {
		t.Fatal("reload was not called")
	}
}

func TestSelectAndReloadAgentErrors(t *testing.T) {
	workspace := "doubao-realtime"
	other := "other"
	tests := []struct {
		name    string
		control *fakeRunControl
		want    string
	}{
		{
			name:    "set fails",
			control: &fakeRunControl{setErr: errors.New("set failed")},
			want:    "select workspace",
		},
		{
			name:    "reload fails",
			control: &fakeRunControl{reloadErr: errors.New("reload failed")},
			want:    "reload workspace",
		},
		{
			name:    "status fails",
			control: &fakeRunControl{statusErr: errors.New("status failed")},
			want:    "get run workspace",
		},
		{
			name: "wrong workspace",
			control: &fakeRunControl{workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{{
				RuntimeState:  rpcapi.PeerRunStatusStateRunning,
				WorkspaceName: other,
			}}},
			want: "running workspace",
		},
		{
			name: "run error",
			control: &fakeRunControl{workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{{
				RuntimeState: rpcapi.PeerRunStatusStateError,
				Message:      stringPtr("boom"),
			}}},
			want: "failed to start",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := selectAndReloadAgent(context.Background(), tt.control, config{Workspace: workspace})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("selectAndReloadAgent() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateWorkspaceRuntimeForFlowcraft(t *testing.T) {
	workspace := "flowcraft-voice"
	control := &fakeRunControl{
		workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{
			{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: workspace},
		},
	}
	report, err := validateWorkspaceRuntime(context.Background(), nil, control, config{
		Workspace: workspace,
		Agent:     "flowcraft",
	}, []roundStats{{Transcript: "你好"}}, workspaceRuntimeValidationOptions{})
	if err != nil {
		t.Fatalf("validateWorkspaceRuntime() error = %v", err)
	}
	if report == nil || report.Workspace != workspace || report.HistoryCount != 1 || report.ReplayState != "played" || !report.MemoryEnabled || !report.RecallAvailable {
		t.Fatalf("runtime report = %+v", report)
	}
	if control.reloaded {
		t.Fatalf("runtime validation reloaded an already active workspace")
	}
}

func TestValidateWorkspaceRuntimeReloadsDifferentWorkspace(t *testing.T) {
	workspace := "flowcraft-voice"
	control := &fakeRunControl{
		workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{
			{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: "other"},
			{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: workspace},
			{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: workspace},
		},
	}
	if _, err := validateWorkspaceRuntime(context.Background(), nil, control, config{
		Workspace: workspace,
		Agent:     "flowcraft",
	}, []roundStats{{Transcript: "你好"}}, workspaceRuntimeValidationOptions{}); err != nil {
		t.Fatalf("validateWorkspaceRuntime() error = %v", err)
	}
	if control.selectedWorkspace != workspace || !control.reloaded {
		t.Fatalf("selected/reloaded = %q/%t", control.selectedWorkspace, control.reloaded)
	}
}

func TestValidateWorkspaceRuntimeAllowsDisabledMemory(t *testing.T) {
	workspace := "flowcraft-func-chat"
	control := &fakeRunControl{
		workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{
			{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: workspace},
		},
		memory: &rpcapi.ServerGetRunWorkspaceMemoryStatsResponse{Available: true, Enabled: false},
	}
	report, err := validateWorkspaceRuntime(context.Background(), nil, control, config{
		Workspace: workspace,
		Agent:     "flowcraft",
	}, []roundStats{{Transcript: "你好"}}, workspaceRuntimeValidationOptions{})
	if err != nil {
		t.Fatalf("validateWorkspaceRuntime() error = %v", err)
	}
	if report == nil || !report.MemoryAvailable || report.MemoryEnabled || report.RecallAvailable {
		t.Fatalf("runtime report = %+v", report)
	}
}

func TestValidateWorkspaceRuntimeAllowsMissingReplayWhenConfigured(t *testing.T) {
	workspace := "flowcraft-voice"
	control := &fakeRunControl{
		workspaceStates: []*rpcapi.ServerGetRunWorkspaceResponse{
			{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: workspace},
		},
		history: &rpcapi.ServerListRunWorkspaceHistoryResponse{
			Available: true,
			Items: []rpcapi.PeerRunHistoryEntry{{
				Id:              "gear:000000",
				CreatedAt:       time.Now(),
				Name:            "transcript",
				ReplayAvailable: false,
				Text:            "用户输入",
				Type:            rpcapi.PeerRunHistoryEntryTypeGear,
			}},
		},
	}
	report, err := validateWorkspaceRuntime(context.Background(), nil, control, config{
		Workspace: workspace,
		Agent:     "flowcraft",
	}, []roundStats{{Transcript: "你好"}}, workspaceRuntimeValidationOptions{AllowMissingReplay: true})
	if err != nil {
		t.Fatalf("validateWorkspaceRuntime() error = %v", err)
	}
	if report == nil || report.HistoryCount != 1 || report.ReplayState != "" || report.ReplayHistoryID != "" {
		t.Fatalf("runtime report = %+v", report)
	}
}

func TestPrintRunSummary(t *testing.T) {
	output := captureStdout(t, func() {
		printRunSummary(config{
			Server:    serverConfig{Addr: "127.0.0.1:9820"},
			Workspace: "demo",
			Agent:     "doubao-realtime",
			Rounds:    1,
			OutputDir: "/tmp/out",
		}, []roundStats{{
			Index:                   1,
			UserText:                "你好",
			InputASR:                "你好",
			Transcript:              "你好",
			AssistantText:           "收到",
			AssistantAudioASR:       "收到",
			FirstAssistantText:      "收",
			InputOpusPackets:        2,
			InputOpusBytes:          10,
			DownlinkPackets:         3,
			DownlinkBytes:           20,
			EventCount:              4,
			UplinkSend:              8 * time.Millisecond,
			ResponseTotal:           40 * time.Millisecond,
			FirstTranscriptChunk:    10 * time.Millisecond,
			TranscriptDone:          15 * time.Millisecond,
			FirstAssistantTextChunk: 20 * time.Millisecond,
			FirstAudioChunk:         30 * time.Millisecond,
			WorkspaceTotal:          time.Second,
		}})
	})
	if !strings.Contains(output, "workspace=demo") ||
		!strings.Contains(output, "round=1") ||
		!strings.Contains(output, "workspace_uplink_send=8ms") ||
		!strings.Contains(output, "after_eos_transcript_start=10ms") ||
		!strings.Contains(output, "after_eos_transcript_done=15ms") ||
		!strings.Contains(output, "after_eos_text_first_chunk=20ms") ||
		!strings.Contains(output, "text_first_after_transcript_done=5ms") ||
		!strings.Contains(output, "after_eos_audio_first_chunk=30ms") ||
		!strings.Contains(output, "after_eos_complete=40ms") ||
		!strings.Contains(output, "workspace_total=1s") ||
		!strings.Contains(output, "timing_summary=") ||
		!strings.Contains(output, `"user":"你好"`) ||
		!strings.Contains(output, `"transcript":"你好"`) ||
		!strings.Contains(output, `"assistant_first_delta":"收"`) ||
		!strings.Contains(output, `"assistant":"收到"`) ||
		!strings.Contains(output, `"assistant_audio_asr":"收到"`) {
		t.Fatalf("summary output = %q", output)
	}
	if strings.Contains(output, "input_transcribe") ||
		strings.Contains(output, "input_asr") ||
		strings.Contains(output, "generate=") ||
		strings.Contains(output, "synthesize=") ||
		strings.Contains(output, "first_text_chunk") {
		t.Fatalf("summary output includes local timing fields: %q", output)
	}
}

func TestRoundTimingSummary(t *testing.T) {
	summary := roundTimingSummary([]roundStats{
		{
			UplinkSend:              5 * time.Millisecond,
			FirstTranscriptChunk:    10 * time.Millisecond,
			TranscriptDone:          15 * time.Millisecond,
			FirstAssistantTextChunk: 20 * time.Millisecond,
			FirstAudioChunk:         30 * time.Millisecond,
			ResponseTotal:           40 * time.Millisecond,
			WorkspaceTotal:          50 * time.Millisecond,
		},
		{
			UplinkSend:              15 * time.Millisecond,
			FirstTranscriptChunk:    30 * time.Millisecond,
			TranscriptDone:          35 * time.Millisecond,
			FirstAssistantTextChunk: 40 * time.Millisecond,
			FirstAudioChunk:         50 * time.Millisecond,
			ResponseTotal:           60 * time.Millisecond,
			WorkspaceTotal:          70 * time.Millisecond,
		},
	})
	if got := summary["after_eos_transcript_first"]; got.Count != 2 || got.MinMS != 10 || got.AvgMS != 20 || got.MaxMS != 30 {
		t.Fatalf("after_eos_transcript_first summary = %+v", got)
	}
	if got := summary["after_eos_transcript_start"]; got.Count != 2 || got.MinMS != 10 || got.AvgMS != 20 || got.MaxMS != 30 {
		t.Fatalf("after_eos_transcript_start summary = %+v", got)
	}
	if got := summary["after_eos_transcript_done"]; got.Count != 2 || got.MinMS != 15 || got.AvgMS != 25 || got.MaxMS != 35 {
		t.Fatalf("after_eos_transcript_done summary = %+v", got)
	}
	if got := summary["after_eos_text_first"]; got.Count != 2 || got.MinMS != 20 || got.MaxMS != 40 {
		t.Fatalf("after_eos_text_first summary = %+v", got)
	}
	if got := summary["text_first_after_transcript_done"]; got.Count != 2 || got.MinMS != 5 || got.MaxMS != 5 {
		t.Fatalf("text_first_after_transcript_done summary = %+v", got)
	}
	if got := summary["after_eos_audio_first"]; got.Count != 2 || got.MinMS != 30 || got.MaxMS != 50 {
		t.Fatalf("after_eos_audio_first summary = %+v", got)
	}
	if got := summary["workspace_total_including_send"]; got.Count != 2 || got.MinMS != 50 || got.MaxMS != 70 {
		t.Fatalf("workspace_total_including_send summary = %+v", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = old
	}()
	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(data)
}

func restoreRunHooks(t *testing.T) {
	t.Helper()
	origDial := dialClientForRun
	origEnsure := ensureWorkspaceForRun
	origSelect := selectAndReloadAgentForRun
	origTransport := newChatTransportForRun
	origRun := runWorkspaceCaseForRun
	origValidate := validateWorkspaceRuntimeForRun
	t.Cleanup(func() {
		dialClientForRun = origDial
		ensureWorkspaceForRun = origEnsure
		selectAndReloadAgentForRun = origSelect
		newChatTransportForRun = origTransport
		runWorkspaceCaseForRun = origRun
		validateWorkspaceRuntimeForRun = origValidate
	})
}

type fakeRunControl struct {
	getWorkflowErr     error
	createWorkspaceErr error
	putWorkspaceErr    error
	deleteWorkspaceErr error
	stopErr            error
	setErr             error
	reloadErr          error
	statusErr          error
	workspaceStates    []*rpcapi.ServerGetRunWorkspaceResponse
	history            *rpcapi.ServerListRunWorkspaceHistoryResponse
	play               *rpcapi.ServerPlayRunWorkspaceHistoryResponse
	memory             *rpcapi.ServerGetRunWorkspaceMemoryStatsResponse
	recall             *rpcapi.ServerRunWorkspaceRecallResponse
	getWorkflow        rpcapi.WorkflowGetRequest
	workflow           *rpcapi.WorkflowGetResponse
	createdWorkspace   rpcapi.WorkspaceCreateRequest
	putWorkspace       rpcapi.WorkspacePutRequest
	deletedWorkspace   string
	selectedWorkspace  string
	stopped            bool
	reloaded           bool
}

func (f *fakeRunControl) GetWorkflow(_ context.Context, _ string, request rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error) {
	f.getWorkflow = request
	if f.getWorkflowErr != nil {
		return nil, f.getWorkflowErr
	}
	if f.workflow != nil {
		return f.workflow, nil
	}
	return &rpcapi.WorkflowGetResponse{
		Name: request.Name,
	}, nil
}

func (f *fakeRunControl) CreateWorkspace(_ context.Context, _ string, request rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error) {
	f.createdWorkspace = request
	if f.createWorkspaceErr != nil {
		return nil, f.createWorkspaceErr
	}
	return &request, nil
}

func (f *fakeRunControl) DeleteWorkspace(_ context.Context, _ string, request rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error) {
	f.deletedWorkspace = request.Name
	if f.deleteWorkspaceErr != nil {
		return nil, f.deleteWorkspaceErr
	}
	return &rpcapi.WorkspaceDeleteResponse{Name: request.Name}, nil
}

func (f *fakeRunControl) StopServerRun(context.Context, string) (*rpcapi.ServerStopRunResponse, error) {
	f.stopped = true
	if f.stopErr != nil {
		return nil, f.stopErr
	}
	return &rpcapi.ServerStopRunResponse{State: rpcapi.PeerRunStatusStateStopped}, nil
}

func (f *fakeRunControl) PutWorkspace(_ context.Context, _ string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	f.putWorkspace = request
	if f.putWorkspaceErr != nil {
		return nil, f.putWorkspaceErr
	}
	return &request.Body, nil
}

func (f *fakeRunControl) SetServerRunWorkspace(_ context.Context, _ string, request rpcapi.ServerSetRunWorkspaceRequest) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
	f.selectedWorkspace = request.WorkspaceName
	return &rpcapi.ServerSetRunWorkspaceResponse{}, f.setErr
}

func (f *fakeRunControl) ReloadServerRunWorkspace(context.Context, string) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
	f.reloaded = true
	return &rpcapi.ServerReloadRunWorkspaceResponse{}, f.reloadErr
}

func (f *fakeRunControl) GetServerRunWorkspace(context.Context, string) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	if len(f.workspaceStates) == 0 {
		return &rpcapi.ServerGetRunWorkspaceResponse{RuntimeState: rpcapi.PeerRunStatusStateRunning, WorkspaceName: f.selectedWorkspace}, nil
	}
	status := f.workspaceStates[0]
	f.workspaceStates = f.workspaceStates[1:]
	return status, nil
}

func (f *fakeRunControl) ListServerRunWorkspaceHistory(context.Context, string, rpcapi.ServerListRunWorkspaceHistoryRequest) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error) {
	if f.history != nil {
		return f.history, nil
	}
	return &rpcapi.ServerListRunWorkspaceHistoryResponse{
		Available: true,
		Items: []rpcapi.PeerRunHistoryEntry{{
			Id:              "ctx:000000",
			CreatedAt:       time.Now(),
			Name:            "agent",
			ReplayAvailable: true,
			Text:            "历史回复",
			Type:            rpcapi.PeerRunHistoryEntryTypeAgent,
		}},
		HasNext: false,
	}, nil
}

func (f *fakeRunControl) PlayServerRunWorkspaceHistory(context.Context, string, rpcapi.ServerPlayRunWorkspaceHistoryRequest) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error) {
	if f.play != nil {
		return f.play, nil
	}
	return &rpcapi.ServerPlayRunWorkspaceHistoryResponse{Accepted: true, HistoryId: "ctx:000000", State: "played"}, nil
}

func (f *fakeRunControl) GetServerRunWorkspaceMemoryStats(context.Context, string, rpcapi.ServerGetRunWorkspaceMemoryStatsRequest) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error) {
	if f.memory != nil {
		return f.memory, nil
	}
	return &rpcapi.ServerGetRunWorkspaceMemoryStatsResponse{Available: true, Enabled: true, ItemCount: 1, StorageBytes: 10}, nil
}

func (f *fakeRunControl) ServerRunWorkspaceRecall(context.Context, string, rpcapi.ServerRunWorkspaceRecallRequest) (*rpcapi.ServerRunWorkspaceRecallResponse, error) {
	if f.recall != nil {
		return f.recall, nil
	}
	return &rpcapi.ServerRunWorkspaceRecallResponse{Available: true, Hits: []rpcapi.PeerRunRecallHit{}}, nil
}

func stringPtr(s string) *string {
	return &s
}
