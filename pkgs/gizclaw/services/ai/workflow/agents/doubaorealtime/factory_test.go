package doubaorealtime

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
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

func newTestDoubaoRealtimeHistory(t testing.TB) *workspace.HistoryStore {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	records, err := logstore.NewSQLStoreWithDB(db, "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	objects, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	return workspace.NewHistoryStore(records, objects, "demo")
}

func TestFactoryAgentInitiativeEnablesHiddenQuery(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	agent := apitypes.ConversationParametersInitiativeAgent
	policy := apitypes.ConversationParametersAgentInitiativePolicyOnReload
	params := testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
		Conversation: &apitypes.ConversationParameters{Initiative: &agent, AgentInitiativePolicy: &policy},
	})
	got, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-initiative", Name: "demo", Parameters: params},
		Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
			Model:           "workflow-dialog",
			InitiativeQuery: new("请先打个招呼"),
		}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	query := patternQuery(t, transformPattern(t, got))
	if query.Get("initiative") != "on_reload" {
		t.Fatalf("initiative = %q, want on_reload", query.Get("initiative"))
	}
	if query.Get("initiative_query") != "请先打个招呼" {
		t.Fatalf("initiative_query = %q, want workflow query", query.Get("initiative_query"))
	}
}

func TestFactoryPeerInitiativeSendsNoHiddenQuery(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	peer := apitypes.ConversationParametersInitiativePeer
	params := testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
		Conversation: &apitypes.ConversationParameters{Initiative: &peer},
	})
	got, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-peer", Name: "demo", Parameters: params},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "workflow-dialog"}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if query := patternQuery(t, transformPattern(t, got)); query.Has("initiative") {
		t.Fatalf("pattern enables initiative for peer conversation: %v", query)
	}
}

func TestFactoryOnceWhenEmptyInitiativeFollowsHistory(t *testing.T) {
	agent := apitypes.ConversationParametersInitiativeAgent
	policy := apitypes.ConversationParametersAgentInitiativePolicyOnceWhenEmpty
	params := testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{
		Conversation: &apitypes.ConversationParameters{Initiative: &agent, AgentInitiativePolicy: &policy},
	})
	history := newTestDoubaoRealtimeHistory(t)
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-once", Name: "demo", Parameters: params},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "workflow-dialog"}),
		Runtime:   workspace.Runtime{History: history},
	}
	factory := Factory{Transformer: recordingTransformer{}}

	got, err := factory.NewAgent(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if query := patternQuery(t, transformPattern(t, got)); query.Get("initiative") != "on_reload" {
		t.Fatalf("empty history should enable initiative, got %v", query)
	}

	if _, err := history.Append(context.Background(), workspace.AppendHistoryRequest{Type: "agent", Name: "agent", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	got, err = factory.NewAgent(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if query := patternQuery(t, transformPattern(t, got)); query.Has("initiative") {
		t.Fatalf("non-empty history should disable once_when_empty initiative, got %v", query)
	}

	spec.Runtime.History = nil
	if _, err := factory.NewAgent(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "history is required") {
		t.Fatalf("NewAgent() without history error = %v, want history required", err)
	}
}

func TestFactoryTTSRequestsTextOutputAndSynthesizesWithVoice(t *testing.T) {
	var validated []string
	mux := &ttsComposeMux{}
	factory := Factory{
		Transformer: mux,
		ValidateVoice: func(_ context.Context, owner, alias string) error {
			validated = append(validated, owner+"|"+alias)
			return nil
		},
	}
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-dialog-id", Name: "demo"},
		Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "doubao-dialog",
			Tts:   &apitypes.DoubaoRealtimeTTS{Voice: " narrator "},
		}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if len(validated) != 1 || validated[0] != "|narrator" {
		t.Fatalf("validated voices = %q, want the Peer runtime narrator", validated)
	}
	output, err := agent.Transform(context.Background(), emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var texts []string
	var audio int
	for {
		chunk, err := output.Next()
		if err != nil {
			if err == io.EOF || err == genx.ErrDone {
				break
			}
			t.Fatalf("Next() error = %v", err)
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			if part != "" {
				texts = append(texts, string(part))
			}
		case *genx.Blob:
			if chunk.Role == genx.RoleModel && len(part.Data) > 0 {
				audio++
			}
		}
	}
	if len(texts) != 1 || texts[0] != "hello" || audio != 1 {
		t.Fatalf("texts/audio = %q/%d, want the reply text and its synthesized audio", texts, audio)
	}
	patterns := mux.recorded()
	if len(patterns) != 2 || patterns[1] != "voice/narrator" {
		t.Fatalf("patterns = %q, want the realtime model then voice/narrator", patterns)
	}
	if got := patternQuery(t, patterns[0]).Get("output"); got != "text" {
		t.Fatalf("model pattern output = %q, want text; pattern=%s", got, patterns[0])
	}
}

func TestFactoryWithoutTTSKeepsProviderVoice(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-dialog-id", Name: "demo"},
		Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "doubao-dialog"}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if got := patternQuery(t, transformPattern(t, agent)).Get("output"); got != "" {
		t.Fatalf("output = %q, want provider default", got)
	}
}

func TestFactoryTTSValidation(t *testing.T) {
	owner := "owner-public-key"
	ownerMux := func(context.Context, string) (genx.TransformerMux, error) { return recordingTransformer{}, nil }
	for name, tc := range map[string]struct {
		factory   Factory
		workspace apitypes.Workspace
		voice     string
		want      string
	}{
		"empty voice": {
			factory:   Factory{Transformer: recordingTransformer{}, ValidateVoice: func(context.Context, string, string) error { return nil }},
			workspace: apitypes.Workspace{Id: "id", Name: "demo"},
			voice:     " ",
			want:      "tts.voice is required",
		},
		"missing validator": {
			factory:   Factory{Transformer: recordingTransformer{}},
			workspace: apitypes.Workspace{Id: "id", Name: "demo"},
			voice:     "narrator",
			want:      "requires a Voice validator",
		},
		"unknown voice": {
			factory: Factory{
				TransformerForOwner: ownerMux,
				ValidateVoice: func(_ context.Context, gotOwner, alias string) error {
					if gotOwner != owner || alias != "narrator" {
						t.Errorf("ValidateVoice(%q, %q), want owner runtime narrator", gotOwner, alias)
					}
					return errors.New("voice not found")
				},
			},
			workspace: apitypes.Workspace{Id: "id", Name: "demo", OwnerPublicKey: &owner},
			voice:     "narrator",
			want:      `resolve tts.voice "narrator": voice not found`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tc.factory.NewAgent(context.Background(), agenthost.Spec{
				Workspace: tc.workspace,
				Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{
					Model: "doubao-dialog",
					Tts:   &apitypes.DoubaoRealtimeTTS{Voice: tc.voice},
				}),
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewAgent() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// ttsComposeMux serves the realtime model with one text reply and the Voice
// with one audio packet per synthesized text route.
type ttsComposeMux struct {
	mu       sync.Mutex
	patterns []string
}

func (m *ttsComposeMux) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	m.mu.Lock()
	m.patterns = append(m.patterns, pattern)
	m.mu.Unlock()
	if strings.HasPrefix(pattern, "voice/") {
		return &fakeTTSStream{input: input}, nil
	}
	return &chunkSliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", EndOfStream: true}},
	}}, nil
}

func (m *ttsComposeMux) recorded() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.patterns...)
}

type chunkSliceStream struct {
	chunks []*genx.MessageChunk
}

func (s *chunkSliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *chunkSliceStream) Close() error { return nil }

func (s *chunkSliceStream) CloseWithError(error) error { return nil }

// fakeTTSStream consumes one text route and answers with one audio route.
type fakeTTSStream struct {
	input  genx.Stream
	output []*genx.MessageChunk
	read   bool
}

func (s *fakeTTSStream) Next() (*genx.MessageChunk, error) {
	if !s.read {
		s.read = true
		streamID := ""
		for {
			chunk, err := s.input.Next()
			if err != nil {
				break
			}
			if chunk.Ctrl != nil && streamID == "" {
				streamID = chunk.Ctrl.StreamID
			}
			if chunk.IsEndOfStream() {
				break
			}
		}
		s.output = []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
		}
	}
	if len(s.output) == 0 {
		return nil, io.EOF
	}
	chunk := s.output[0]
	s.output = s.output[1:]
	return chunk, nil
}

func (s *fakeTTSStream) Close() error { return s.input.Close() }

func (s *fakeTTSStream) CloseWithError(err error) error { return s.input.CloseWithError(err) }
