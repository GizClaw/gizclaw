package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestRunRejectsMissingConfig(t *testing.T) {
	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "config path is required") {
		t.Fatalf("run(nil) error = %v", err)
	}
}

func TestRunRejectsMissingCase(t *testing.T) {
	err := run([]string{"-config", "config.json"})
	if err == nil || !strings.Contains(err.Error(), "-case is required") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestParseWorkspaceCaseRejectsUnsupported(t *testing.T) {
	if _, err := parseWorkspaceCase("roundtrip"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("parseWorkspaceCase() error = %v", err)
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
	if got.Workspace != "demo-workflow-realtime-roundtrip" {
		t.Fatalf("realtime workspace = %q", got.Workspace)
	}
	got, err = workspaceCasePushToTalkInterrupt.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(push) error = %v", err)
	}
	if got.workspaceMode() != "push_to_talk" {
		t.Fatalf("push workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-push-to-talk-interrupt" {
		t.Fatalf("push workspace = %q", got.Workspace)
	}
	got, err = workspaceCaseHistoryReplay.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(history-replay) error = %v", err)
	}
	if got.workspaceMode() != "push_to_talk" {
		t.Fatalf("history-replay workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-history-replay" {
		t.Fatalf("history-replay workspace = %q", got.Workspace)
	}
	got, err = workspaceCaseHumanReview.applyConfig(got)
	if err != nil {
		t.Fatalf("applyConfig(human-review) error = %v", err)
	}
	if got.workspaceMode() != "push_to_talk" {
		t.Fatalf("human-review workspace mode = %q", got.workspaceMode())
	}
	if got.Workspace != "demo-workflow-human-review" {
		t.Fatalf("human-review workspace = %q", got.Workspace)
	}
	if parsed, err := parseWorkspaceCase("human-review"); err != nil || parsed != workspaceCaseHumanReview {
		t.Fatalf("parseWorkspaceCase(human-review) = %q/%v", parsed, err)
	}
	if parsed, err := parseWorkspaceCase("history-replay"); err != nil || parsed != workspaceCaseHistoryReplay {
		t.Fatalf("parseWorkspaceCase(history-replay) = %q/%v", parsed, err)
	}
}

func TestWorkspaceCaseDispatchRejectsUnknown(t *testing.T) {
	_, err := (&personaDriver{}).runCase(context.Background(), workspaceCase("unknown"))
	if err == nil || !strings.Contains(err.Error(), "unsupported workspace case") {
		t.Fatalf("runCase(unknown) error = %v", err)
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

	realtime := workflowSpec(config{Workflow: workflowConfig{RealtimeModel: "rt", Session: realtimeSessionConfig{AuthMode: "v2"}, Output: realtimeOutputConfig{Speaker: "sp"}}})
	if realtime.Driver != rpcapi.WorkflowDriverDoubaoRealtime || realtime.DoubaoRealtime == nil || realtime.DoubaoRealtime.Realtime == nil {
		t.Fatalf("realtime spec = %+v", realtime)
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
    "realtime_model": "realtime"
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
		if cfg.Workspace != "doubao-realtime-workflow-push-to-talk-roundtrip" || cfg.Models.LLM != "chat" {
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
		if err := run([]string{"-config", configPath, "-context-config", contextConfigPath, "-case", "push-to-talk-roundtrip"}); err != nil {
			t.Fatalf("run() error = %v", err)
		}
	})
	if !dialed || !ensured || !selected || !transported || !ran {
		t.Fatalf("hooks dial/ensure/select/transport/run = %t/%t/%t/%t/%t", dialed, ensured, selected, transported, ran)
	}
	if !strings.Contains(output, "workspace=doubao-realtime-workflow-push-to-talk-roundtrip") || !strings.Contains(output, "round=1") {
		t.Fatalf("run output = %q", output)
	}
}

func TestRunConfigDirRunsRaidConfigs(t *testing.T) {
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
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	contextConfigPath := filepath.Join(dir, "context", "config.yaml")
	writeSetupContextConfig(t, contextConfigPath, serverKey, clientKey, "")
	writeWorkspaceConfig := func(name string) {
		t.Helper()
		path := filepath.Join(configDir, "flowcraft-"+name+".json")
		data := strings.ReplaceAll(`{
  "agent": "flowcraft",
  "models": {
    "llm": "chat",
    "tts": "tts",
    "asr": "asr"
  },
  "workflow": {
    "name": "workflow-WORKSPACE",
    "flowcraft": {
      "agent": {
        "id": "claw",
        "model": "generate_model"
      }
    }
  },
  "voice": "voice",
  "rounds": 1,
  "timeout": "1s",
  "persona": "persona"
}`, "WORKSPACE", "flowcraft-"+name)
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatalf("write config %s: %v", name, err)
		}
	}
	writeWorkspaceConfig("b")
	writeWorkspaceConfig("a")

	serveDone := make(chan error, 2)
	serveDone <- nil
	serveDone <- nil
	dialClientForRun = func(config) (*gizcli.Client, <-chan error, error) {
		return &gizcli.Client{}, serveDone, nil
	}
	var ensured []string
	ensureWorkspaceForRun = func(_ context.Context, _ *gizcli.Client, cfg config) (config, error) {
		ensured = append(ensured, cfg.Workspace)
		return cfg, nil
	}
	runWorkspaceCaseForRun = func(_ *personaDriver, _ context.Context, selected workspaceCase) (workspaceCaseResult, error) {
		if selected != workspaceCasePushToTalkRoundtrip {
			t.Fatalf("selected case = %q", selected)
		}
		return workspaceCaseResult{Rounds: []roundStats{{Index: 1, UserText: "你好", Transcript: "你好", AssistantText: "收到"}}}, nil
	}
	validateWorkspaceRuntimeForRun = func(context.Context, *personaDriver, runControlClient, config, []roundStats) (*workspaceRuntimeReport, error) {
		return nil, nil
	}

	output := captureStdout(t, func() {
		if err := run([]string{"-config-dir", configDir, "-context-config", contextConfigPath, "-case", "push-to-talk-roundtrip"}); err != nil {
			t.Fatalf("run() error = %v", err)
		}
	})
	want := []string{"workflow-flowcraft-a-push-to-talk-roundtrip", "workflow-flowcraft-b-push-to-talk-roundtrip"}
	if strings.Join(ensured, ",") != strings.Join(want, ",") {
		t.Fatalf("ensured workspaces = %v, want %v", ensured, want)
	}
	if !strings.Contains(output, "flowcraft-a.json") || !strings.Contains(output, "flowcraft-b.json") {
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
    "name": "e2e-flowcraft-journey"
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
	validateWorkspaceRuntimeForRun = func(context.Context, *personaDriver, runControlClient, config, []roundStats) (*workspaceRuntimeReport, error) {
		return nil, nil
	}
	if err := run([]string{"-config", configPath, "-context-config", contextConfigPath, "-case", "push-to-talk-roundtrip"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunRejectsConfigAndConfigDir(t *testing.T) {
	err := run([]string{"-config", "one.json", "-config-dir", "configs"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestFlowcraftConfigPaths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"flowcraft-b.json", "ignored.json", "flowcraft-a.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	paths, err := flowcraftConfigPaths(dir)
	if err != nil {
		t.Fatalf("flowcraftConfigPaths() error = %v", err)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "flowcraft-a.json") || !strings.HasSuffix(paths[1], "flowcraft-b.json") {
		t.Fatalf("flowcraftConfigPaths() = %v", paths)
	}
	if _, err := flowcraftConfigPaths(filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "no flowcraft configs") {
		t.Fatalf("empty flowcraftConfigPaths() error = %v", err)
	}
}

func TestDialClientRejectsInvalidPrivateKey(t *testing.T) {
	_, _, err := dialClient(config{ClientPrivateKey: "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid key text") {
		t.Fatalf("dialClient() error = %v", err)
	}
}

func TestEnsureWorkspacePutsWorkflowAndRecreatesWorkspace(t *testing.T) {
	control := &fakeRunControl{}
	searchEnabled := true
	musicEnabled := true
	searchCount := 3
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Models:    modelConfig{Realtime: "realtime"},
		Workflow: workflowConfig{
			Name:          "workflow-a",
			RealtimeModel: "realtime",
			Parameters: workspaceParameterConfig{
				Input: "realtime",
				Voice: workspaceVoiceConfig{
					RealtimeSpeakerID: "speaker-a",
				},
				Search: realtimeSearchConfig{
					Enabled:         &searchEnabled,
					Type:            "web_summary",
					ResultCount:     &searchCount,
					NoResultMessage: "none",
				},
				Music: realtimeMusicConfig{Enabled: &musicEnabled},
			},
			Session: realtimeSessionConfig{
				AuthMode:    "v2",
				BotName:     "豆包",
				Model:       "O",
				ResourceID:  "volc.speech.dialog",
				SystemRole:  "简短回答",
				VADWindowMS: 200,
			},
			Output: realtimeOutputConfig{Speaker: "speaker-a"},
		},
	}
	ensured, err := ensureWorkspace(context.Background(), control, cfg)
	if err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.createdWorkflow.Metadata.Name != "" || control.putWorkspace.Body.Name != "" {
		t.Fatalf("ensureWorkspace used wrong methods workflow.create=%+v workspace.put=%+v", control.createdWorkflow, control.putWorkspace)
	}
	if control.putWorkflow.Name != "workflow-a" || control.putWorkflow.Body.Metadata.Name != "workflow-a" {
		t.Fatalf("put workflow = %+v", control.putWorkflow)
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
	if control.putWorkflow.Body.Spec.Driver != rpcapi.WorkflowDriverDoubaoRealtime {
		t.Fatalf("workflow driver = %q", control.putWorkflow.Body.Spec.Driver)
	}
	if control.putWorkflow.Body.Spec.DoubaoRealtime == nil ||
		control.putWorkflow.Body.Spec.DoubaoRealtime.RealtimeModel == nil ||
		*control.putWorkflow.Body.Spec.DoubaoRealtime.RealtimeModel != "realtime" {
		t.Fatalf("workflow realtime_model = %#v", control.putWorkflow.Body.Spec.DoubaoRealtime)
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
		params.RealtimeModel == nil || *params.RealtimeModel != "realtime" ||
		params.Input == nil || *params.Input != rpcapi.WorkspaceInputModeRealtime {
		t.Fatalf("workspace parameters = %#v", params)
	}
	if params.Search == nil ||
		params.Search.Enabled == nil || !*params.Search.Enabled ||
		params.Search.Type == nil || *params.Search.Type != "web_summary" ||
		params.Search.ResultCount == nil || *params.Search.ResultCount != searchCount ||
		params.Search.NoResultMessage == nil || *params.Search.NoResultMessage != "none" {
		t.Fatalf("workspace search parameters = %#v", params.Search)
	}
	if params.Music == nil || params.Music.Enabled == nil || !*params.Music.Enabled {
		t.Fatalf("workspace music parameters = %#v", params.Music)
	}
	if params.Voice == nil {
		t.Fatalf("workspace voice parameters = %#v", params.Voice)
	}
	voice, err := params.Voice.AsDoubaoRealtimeInternalSpeakerParameters()
	if err != nil {
		t.Fatalf("workspace voice decode error = %v", err)
	}
	if voice.RealtimeSpeakerId != "speaker-a" {
		t.Fatalf("workspace voice = %#v", voice)
	}
}

func TestEnsureWorkspaceIgnoresMissingWorkspaceDelete(t *testing.T) {
	control := &fakeRunControl{
		deleteWorkspaceErr: rpcapi.Error{Code: rpcapi.RPCErrorCodeNotFound, Message: "workspace missing"},
	}
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
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
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
	}
	ensured, err := ensureWorkspace(context.Background(), control, cfg)
	if err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.createdWorkflow.Metadata.Name != "" || control.putWorkspace.Body.Name != "" {
		t.Fatalf("ensureWorkspace used wrong methods workflow.create=%+v workspace.put=%+v", control.createdWorkflow, control.putWorkspace)
	}
	if control.putWorkflow.Name != "workflow-a" || control.putWorkflow.Body.Metadata.Name != "workflow-a" {
		t.Fatalf("put workflow = %+v", control.putWorkflow)
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

func TestEnsureWorkspaceReturnsPutErrors(t *testing.T) {
	control := &fakeRunControl{putWorkflowErr: errors.New("denied")}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "upsert workflow") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestEnsureWorkspaceReturnsStopErrors(t *testing.T) {
	control := &fakeRunControl{stopErr: errors.New("busy")}
	_, err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
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
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
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
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
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
	}, []roundStats{{Transcript: "你好"}})
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
	}, []roundStats{{Transcript: "你好"}}); err != nil {
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
	}, []roundStats{{Transcript: "你好"}})
	if err != nil {
		t.Fatalf("validateWorkspaceRuntime() error = %v", err)
	}
	if report == nil || !report.MemoryAvailable || report.MemoryEnabled || report.RecallAvailable {
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
	createWorkflowErr  error
	putWorkflowErr     error
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
	createdWorkflow    rpcapi.WorkflowCreateRequest
	putWorkflow        rpcapi.WorkflowPutRequest
	createdWorkspace   rpcapi.WorkspaceCreateRequest
	putWorkspace       rpcapi.WorkspacePutRequest
	deletedWorkspace   string
	selectedWorkspace  string
	stopped            bool
	reloaded           bool
}

func (f *fakeRunControl) CreateWorkflow(_ context.Context, _ string, request rpcapi.WorkflowCreateRequest) (*rpcapi.WorkflowCreateResponse, error) {
	f.createdWorkflow = request
	if f.createWorkflowErr != nil {
		return nil, f.createWorkflowErr
	}
	return &request, nil
}

func (f *fakeRunControl) PutWorkflow(_ context.Context, _ string, request rpcapi.WorkflowPutRequest) (*rpcapi.WorkflowPutResponse, error) {
	f.putWorkflow = request
	if f.putWorkflowErr != nil {
		return nil, f.putWorkflowErr
	}
	return &request.Body, nil
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
