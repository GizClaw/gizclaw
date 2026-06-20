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
  "workspace": "doubao-realtime",
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
		if cfg.Workspace != "doubao-realtime" || cfg.Models.LLM != "chat" {
			t.Fatalf("dial cfg = %+v", cfg)
		}
		return &gizcli.Client{}, serveDone, nil
	}
	ensureWorkspaceForRun = func(ctx context.Context, client *gizcli.Client, cfg config) error {
		ensured = true
		if cfg.Workflow.Name != "doubao-realtime-workflow" || cfg.Models.Realtime != "realtime" {
			t.Fatalf("ensure cfg = %+v", cfg)
		}
		return nil
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
	runPersonaDriverForRun = func(driver *personaDriver, ctx context.Context) ([]roundStats, error) {
		ran = true
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
		return []roundStats{{Index: 1, UserText: "你好", Transcript: "你好", AssistantText: "收到", DownlinkPackets: 1}}, nil
	}

	output := captureStdout(t, func() {
		if err := run([]string{"-config", configPath, "-context-config", contextConfigPath}); err != nil {
			t.Fatalf("run() error = %v", err)
		}
	})
	if !dialed || !ensured || !selected || !transported || !ran {
		t.Fatalf("hooks dial/ensure/select/transport/run = %t/%t/%t/%t/%t", dialed, ensured, selected, transported, ran)
	}
	if !strings.Contains(output, "workspace=doubao-realtime") || !strings.Contains(output, "round=1") {
		t.Fatalf("run output = %q", output)
	}
}

func TestDialClientRejectsInvalidPrivateKey(t *testing.T) {
	_, _, err := dialClient(config{ClientPrivateKey: "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid key text") {
		t.Fatalf("dialClient() error = %v", err)
	}
}

func TestEnsureWorkspaceCreatesWorkflowAndWorkspace(t *testing.T) {
	control := &fakeRunControl{}
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Models:    modelConfig{Realtime: "realtime"},
		Workflow: workflowConfig{
			Name:          "workflow-a",
			RealtimeModel: "realtime",
			Parameters:    workspaceParameterConfig{E2E: boolPtr(true)},
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
	if err := ensureWorkspace(context.Background(), control, cfg); err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.createdWorkflow.Metadata.Name != "workflow-a" {
		t.Fatalf("created workflow = %+v", control.createdWorkflow)
	}
	if got := control.createdWorkflow.Spec["realtime_model"]; got != "realtime" {
		t.Fatalf("workflow realtime_model = %#v", got)
	}
	if control.createdWorkspace.Name != "workspace-a" || control.createdWorkspace.WorkflowName != "workflow-a" {
		t.Fatalf("created workspace = %+v", control.createdWorkspace)
	}
	if control.createdWorkspace.Parameters == nil {
		t.Fatalf("workspace parameters = %#v", control.createdWorkspace.Parameters)
	}
	workspaceParams, err := control.createdWorkspace.Parameters.AsDoubaoRealtimeWorkspaceParameters()
	if err != nil {
		t.Fatalf("decode workspace parameters: %v", err)
	}
	if workspaceParams.E2e == nil || !*workspaceParams.E2e {
		t.Fatalf("workspace parameters = %#v", workspaceParams)
	}
}

func TestEnsureWorkspaceUpdatesOnConflict(t *testing.T) {
	control := &fakeRunControl{
		createWorkflowErr:  rpcapi.Error{Code: rpcapi.RPCErrorCodeConflict, Message: "workflow exists"},
		createWorkspaceErr: rpcapi.Error{Code: rpcapi.RPCErrorCodeConflict, Message: "workspace exists"},
	}
	cfg := config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
	}
	if err := ensureWorkspace(context.Background(), control, cfg); err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
	if control.putWorkflow.Name != "workflow-a" {
		t.Fatalf("put workflow = %+v", control.putWorkflow)
	}
	if control.putWorkspace.Name != "workspace-a" {
		t.Fatalf("put workspace = %+v", control.putWorkspace)
	}
}

func TestEnsureWorkspaceReturnsCreateErrors(t *testing.T) {
	control := &fakeRunControl{createWorkflowErr: errors.New("denied")}
	err := ensureWorkspace(context.Background(), control, config{
		Workspace: "workspace-a",
		Agent:     "doubao-realtime",
		Workflow:  workflowConfig{Name: "workflow-a", RealtimeModel: "realtime"},
	})
	if err == nil || !strings.Contains(err.Error(), "create workflow") {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}
}

func TestSelectAndReloadAgentReachesRunningWorkspace(t *testing.T) {
	workspace := "doubao-realtime"
	control := &fakeRunControl{
		statuses: []*rpcapi.ServerGetRunStatusResponse{{
			State:         rpcapi.PeerRunStatusStateRunning,
			WorkspaceName: &workspace,
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
			want:    "get run status",
		},
		{
			name: "wrong workspace",
			control: &fakeRunControl{statuses: []*rpcapi.ServerGetRunStatusResponse{{
				State:         rpcapi.PeerRunStatusStateRunning,
				WorkspaceName: &other,
			}}},
			want: "running workspace",
		},
		{
			name: "run error",
			control: &fakeRunControl{statuses: []*rpcapi.ServerGetRunStatusResponse{{
				State:   rpcapi.PeerRunStatusStateError,
				Message: stringPtr("boom"),
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
		!strings.Contains(output, `"assistant":"收到"`) {
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
	origRun := runPersonaDriverForRun
	t.Cleanup(func() {
		dialClientForRun = origDial
		ensureWorkspaceForRun = origEnsure
		selectAndReloadAgentForRun = origSelect
		newChatTransportForRun = origTransport
		runPersonaDriverForRun = origRun
	})
}

type fakeRunControl struct {
	createWorkflowErr  error
	putWorkflowErr     error
	createWorkspaceErr error
	putWorkspaceErr    error
	setErr             error
	reloadErr          error
	statusErr          error
	statuses           []*rpcapi.ServerGetRunStatusResponse
	createdWorkflow    rpcapi.WorkflowCreateRequest
	putWorkflow        rpcapi.WorkflowPutRequest
	createdWorkspace   rpcapi.WorkspaceCreateRequest
	putWorkspace       rpcapi.WorkspacePutRequest
	selectedWorkspace  string
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

func (f *fakeRunControl) PutWorkspace(_ context.Context, _ string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	f.putWorkspace = request
	if f.putWorkspaceErr != nil {
		return nil, f.putWorkspaceErr
	}
	return &request.Body, nil
}

func (f *fakeRunControl) SetServerRunAgent(_ context.Context, _ string, request rpcapi.ServerSetRunAgentRequest) (*rpcapi.ServerSetRunAgentResponse, error) {
	f.selectedWorkspace = request.WorkspaceName
	return &rpcapi.ServerSetRunAgentResponse{}, f.setErr
}

func (f *fakeRunControl) ReloadServerRun(context.Context, string) (*rpcapi.ServerReloadRunResponse, error) {
	f.reloaded = true
	return &rpcapi.ServerReloadRunResponse{}, f.reloadErr
}

func (f *fakeRunControl) GetServerRunStatus(context.Context, string, ...rpcapi.ServerGetRunStatusRequest) (*rpcapi.ServerGetRunStatusResponse, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	if len(f.statuses) == 0 {
		return &rpcapi.ServerGetRunStatusResponse{State: rpcapi.PeerRunStatusStateRunning}, nil
	}
	status := f.statuses[0]
	f.statuses = f.statuses[1:]
	return status, nil
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(v bool) *bool {
	return &v
}
