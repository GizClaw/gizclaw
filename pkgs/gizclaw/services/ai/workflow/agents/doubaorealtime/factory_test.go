package doubaorealtime

import (
	"context"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

//go:fix inline
func stringPtr(value string) *string { return new(value) }

func testDoubaoRealtimeWorkflow(spec apitypes.DoubaoRealtimeWorkflowSpec) apitypes.Workflow {
	return apitypes.Workflow{
		Id: "demo-workflow",
		Spec: apitypes.WorkflowSpec{
			Driver:         apitypes.WorkflowDriverDoubaoRealtime,
			DoubaoRealtime: &spec,
		},
	}
}

func testDoubaoRealtimeWorkspaceParameters(t *testing.T, typed apitypes.DoubaoRealtimeWorkspaceParameters) *apitypes.WorkspaceParameters {
	t.Helper()
	if typed.AgentType == "" {
		typed.AgentType = apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime
	}
	var out apitypes.WorkspaceParameters
	if err := out.FromDoubaoRealtimeWorkspaceParameters(typed); err != nil {
		t.Fatalf("FromDoubaoRealtimeWorkspaceParameters() error = %v", err)
	}
	return &out
}

func TestFactoryUsesWorkflowDuplexConfig(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	strict := true
	speed := 1
	loudness := -1
	workflow := testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
		Model:        "doubao-dialog",
		Instructions: new("简短回答。"),
		Audio: &apitypes.DoubaoRealtimeAudio{
			Input: apitypes.DoubaoRealtimeAudioInput{Format: apitypes.DoubaoRealtimeAudioFormat{
				Type: apitypes.DoubaoRealtimeAudioFormatType("speech_opus"),
				Rate: 16000,
			}},
			Output: apitypes.DoubaoRealtimeAudioOutput{
				Format: apitypes.DoubaoRealtimeAudioFormat{Type: apitypes.DoubaoRealtimeAudioFormatType("ogg_opus"), Rate: 24000},
				Voice:  new("workflow-voice"),
				Speed:  &speed,
			},
		},
		Extension: &apitypes.DoubaoRealtimeExtension{Dialog: &apitypes.DoubaoRealtimeDialogExtension{
			Extra: &apitypes.DoubaoRealtimeDialogExtra{EnableMusic: &strict, AuditResponse: new("audit")},
		}},
	})
	params := testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
		Instructions: new("工作区覆盖指令。"),
		Audio: &apitypes.DoubaoRealtimeAudio{
			Input: apitypes.DoubaoRealtimeAudioInput{Format: apitypes.DoubaoRealtimeAudioFormat{
				Type: apitypes.DoubaoRealtimeAudioFormatType("speech_opus"),
				Rate: 16000,
			}},
			Output: apitypes.DoubaoRealtimeAudioOutput{
				Format:   apitypes.DoubaoRealtimeAudioFormat{Type: apitypes.DoubaoRealtimeAudioFormatType("ogg_opus"), Rate: 24000},
				Voice:    new("workspace-voice"),
				Loudness: &loudness,
			},
		},
	})
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-dialog-id", Name: "demo", Parameters: params},
		Workflow:  workflow,
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	got := transformPattern(t, agent)
	if !strings.HasPrefix(got, "model/doubao-dialog?") {
		t.Fatalf("pattern = %q, want model/doubao-dialog", got)
	}
	query := patternQuery(t, got)
	for key, want := range map[string]string{
		"instructions":       "工作区覆盖指令。",
		"input_format":       "speech_opus",
		"input_sample_rate":  "16000",
		"output_format":      "ogg_opus",
		"output_sample_rate": "24000",
		"output_voice":       "workspace-voice",
		"output_loudness":    "-1",
		"dialog_id":          "workspace-dialog-id",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("query[%s] = %q, want %q; pattern=%s", key, got, want, got)
		}
	}
	if query.Get("extension") == "" || !strings.Contains(query.Get("extension"), "enable_music") {
		t.Fatalf("extension query = %q, want extension JSON", query.Get("extension"))
	}
	if strings.Contains(got, "realtime_model") || strings.Contains(got, "vad_window_ms") || strings.Contains(got, "bot_name") {
		t.Fatalf("pattern contains old realtime params: %q", got)
	}
}

func TestFactoryUsesWorkspaceOwnerTransformer(t *testing.T) {
	owner := "owner-public-key"
	called := false
	agent, err := (Factory{
		Transformer: recordingTransformer{},
		TransformerForOwner: func(_ context.Context, gotOwner string) (genx.TransformerMux, error) {
			called = true
			if gotOwner != owner {
				t.Fatalf("owner = %q, want %q", gotOwner, owner)
			}
			return recordingTransformer{}, nil
		},
	}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-owner", Name: "pet-realtime", OwnerPublicKey: &owner},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "owner-model"}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if agent == nil || !called {
		t.Fatalf("NewAgent() = %#v, owner resolver called = %t", agent, called)
	}
}

func TestFactoryRejectsToolCallConfiguration(t *testing.T) {
	tools := []apitypes.DoubaoRealtimeFunctionTool{{
		Type: apitypes.DoubaoRealtimeFunctionToolTypeFunction,
		Name: "get_weather",
	}}
	for _, tt := range []struct {
		name string
		spec agenthost.Spec
	}{
		{
			name: "workflow",
			spec: agenthost.Spec{Workspace: apitypes.Workspace{Id: "workspace-workflow-tools"}, Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
				Model: "doubao-dialog",
				Tools: &tools,
			})},
		},
		{
			name: "workspace",
			spec: agenthost.Spec{
				Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "doubao-dialog"}),
				Workspace: apitypes.Workspace{Id: "workspace-parameters-tools", Parameters: testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
					Tools: &tools,
				})},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), tt.spec)
			if err == nil || !strings.Contains(err.Error(), "tools are unsupported") {
				t.Fatalf("NewAgent() error = %v, want tools unsupported", err)
			}
		})
	}
}

func TestFactoryWorkspaceCanOverrideModelAndMode(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	input := apitypes.WorkspaceInputModeRealtime
	params := testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
		Model: new("workspace-dialog"),
		Input: &input,
	})
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-override", Name: "demo", Parameters: params},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "workflow-dialog"}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	got := transformPattern(t, agent)
	if !strings.HasPrefix(got, "model/workspace-dialog?") || !strings.Contains(got, "mode=realtime") {
		t.Fatalf("pattern = %q, want workspace model and realtime mode", got)
	}
}

func TestFactoryValidation(t *testing.T) {
	if _, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "transformer") {
		t.Fatalf("NewAgent(missing transformer) error = %v", err)
	}
	if _, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "workflow") {
		t.Fatalf("NewAgent(missing workflow) error = %v", err)
	}
	if _, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-missing-model"},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{}),
	}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("NewAgent(missing model) error = %v", err)
	}
}

func TestFactoryRequiresCanonicalWorkspaceID(t *testing.T) {
	_, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "display-name"},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "doubao-dialog"}),
	})
	if err == nil || !strings.Contains(err.Error(), "canonical workspace id") {
		t.Fatalf("NewAgent(missing workspace id) error = %v", err)
	}
}

func TestFactoryUsesExactCanonicalWorkspaceID(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	workflow := testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "doubao-dialog"})
	for _, workspaceID := range []string{"workspace-a", "workspace-b"} {
		agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
			Workspace: apitypes.Workspace{Id: workspaceID, Name: "same-display-name"},
			Workflow:  workflow,
		})
		if err != nil {
			t.Fatalf("NewAgent(%q) error = %v", workspaceID, err)
		}
		if got := patternQuery(t, transformPattern(t, agent)).Get("dialog_id"); got != workspaceID {
			t.Fatalf("dialog_id = %q, want %q", got, workspaceID)
		}
	}
}

func TestResolveRealtimeConfigOutputLimitPrecedence(t *testing.T) {
	workflowLimit := 20
	for _, test := range []struct {
		name      string
		override  *int
		wantLimit int
	}{
		{name: "inherit", wantLimit: 20},
		{name: "positive override", override: new(10), wantLimit: 10},
		{name: "explicit zero disables", override: new(0), wantLimit: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var parameters *apitypes.WorkspaceParameters
			if test.override != nil {
				parameters = testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
					MaxOutputRunes: test.override,
				})
			}
			config, err := resolveRealtimeConfig(agenthost.Spec{
				Workspace: apitypes.Workspace{Id: "workspace-limit", Parameters: parameters},
				Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
					Model: "doubao-dialog", MaxOutputRunes: &workflowLimit,
				}),
			})
			if err != nil {
				t.Fatalf("resolveRealtimeConfig() error = %v", err)
			}
			if config.MaxOutputRunes != test.wantLimit {
				t.Fatalf("MaxOutputRunes = %d, want %d", config.MaxOutputRunes, test.wantLimit)
			}
		})
	}
}

func TestResolveRealtimeConfigRejectsNegativeOutputLimit(t *testing.T) {
	negative := -1
	_, err := resolveRealtimeConfig(agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-limit"},
		Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "doubao-dialog", MaxOutputRunes: &negative,
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "max_output_runes") {
		t.Fatalf("resolveRealtimeConfig() error = %v", err)
	}
}

func TestWorkflowParamStringCoversPrimitiveAndJSONValues(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  string
		ok    bool
	}{
		{name: "bool", value: true, want: "true", ok: true},
		{name: "int32", value: int32(12), want: "12", ok: true},
		{name: "int64", value: int64(13), want: "13", ok: true},
		{name: "float64 int", value: float64(14), want: "14", ok: true},
		{name: "float64 decimal", value: float64(1.5), want: "1.5", ok: true},
		{name: "float32 decimal", value: float32(2.5), want: "2.5", ok: true},
		{name: "json", value: []map[string]string{{"name": "tool"}}, want: `[{"name":"tool"}]`, ok: true},
		{name: "empty string", value: " ", ok: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := workflowParamString(tt.value)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("workflowParamString(%#v) = %q, %v; want %q, %v", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func transformPattern(t *testing.T, agent agenthost.Agent) string {
	t.Helper()
	stream, err := agent.Transform(context.Background(), emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	return string(chunk.Part.(genx.Text))
}

func patternQuery(t *testing.T, pattern string) url.Values {
	t.Helper()
	_, rawQuery, ok := strings.Cut(pattern, "?")
	if !ok {
		t.Fatalf("pattern %q has no query", pattern)
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("ParseQuery(%q) error = %v", rawQuery, err)
	}
	return query
}

type recordingTransformer struct{}

func (recordingTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	return &singleChunkStream{chunk: &genx.MessageChunk{Part: genx.Text(pattern)}}, nil
}

type singleChunkStream struct {
	chunk *genx.MessageChunk
}

func (s *singleChunkStream) Next() (*genx.MessageChunk, error) {
	if s.chunk == nil {
		return nil, io.EOF
	}
	chunk := s.chunk
	s.chunk = nil
	return chunk, nil
}

func (s *singleChunkStream) Close() error { return nil }

func (s *singleChunkStream) CloseWithError(error) error { return nil }

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }

func (emptyStream) Close() error { return nil }

func (emptyStream) CloseWithError(error) error { return nil }
