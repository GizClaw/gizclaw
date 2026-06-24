package doubaorealtime

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/agenthost"
)

func stringPtr(value string) *string { return &value }

func testDoubaoRealtimeWorkspaceParameters(values map[string]any) *apitypes.WorkspaceParameters {
	typed := apitypes.DoubaoRealtimeWorkspaceParameters{
		AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime,
	}
	if value, _ := values["realtime_model"].(string); value != "" {
		typed.RealtimeModel = &value
	}
	if value, ok := values["temperature"].(float64); ok {
		v := float32(value)
		typed.Temperature = &v
	}
	mergeTestRealtimeWorkspaceValue(&typed, values["realtime_config"])
	mergeTestRealtimeWorkspaceValue(&typed, values["realtime"])
	var out apitypes.WorkspaceParameters
	if err := out.FromDoubaoRealtimeWorkspaceParameters(typed); err != nil {
		panic(err)
	}
	return &out
}

func mergeTestRealtimeWorkspaceValue(typed *apitypes.DoubaoRealtimeWorkspaceParameters, value any) {
	values, ok := value.(map[string]any)
	if !ok {
		return
	}
	if session, ok := values["session"].(map[string]any); ok {
		if typed.Session == nil {
			typed.Session = &apitypes.DoubaoRealtimeSessionParameters{}
		}
		if value, _ := session["model"].(string); value != "" {
			typed.Session.UpstreamModel = &value
		}
		if value, _ := session["upstream_model"].(string); value != "" {
			typed.Session.UpstreamModel = &value
		}
		if value, _ := session["system_role"].(string); value != "" {
			typed.Session.SystemRole = &value
		}
		if value, ok := session["vad_window_ms"].(int); ok {
			typed.Session.VadWindowMs = &value
		}
	}
	if output, ok := values["output"].(map[string]any); ok {
		voice, _ := output["speaker"].(string)
		if voice == "" {
			voice, _ = output["voice"].(string)
		}
		if voice != "" {
			var data apitypes.DoubaoRealtimeVoiceParameters
			if err := data.FromDoubaoRealtimeInternalSpeakerParameters(apitypes.DoubaoRealtimeInternalSpeakerParameters{RealtimeSpeakerId: voice}); err != nil {
				panic(err)
			}
			typed.Voice = &data
		}
	}
}

func testDoubaoRealtimeWorkflow(values map[string]any) apitypes.WorkflowDocument {
	spec := apitypes.DoubaoRealtimeWorkflowSpec{}
	if value, _ := values["model"].(string); value != "" {
		spec.Model = &value
	}
	if value, _ := values["realtime_model"].(string); value != "" {
		spec.RealtimeModel = &value
	}
	if value, ok := values["realtime"].(map[string]any); ok {
		spec.Realtime = &value
	}
	if value, ok := values["realtime_config"].(map[string]any); ok {
		spec.RealtimeConfig = &value
	}
	return apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{Name: "demo-workflow"},
		Spec: apitypes.WorkflowSpec{
			Driver:         apitypes.WorkflowDriverDoubaoRealtime,
			DoubaoRealtime: &spec,
		},
	}
}

func TestFactoryUsesWorkflowModel(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	workspaceParams := map[string]any{
		"realtime": map[string]any{
			"session": map[string]any{
				"system_role": "workspace 覆盖。",
			},
			"output": map[string]any{
				"speaker": "workspace-speaker",
			},
		},
	}
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: testDoubaoRealtimeWorkspaceParameters(workspaceParams)},
		Workflow: testDoubaoRealtimeWorkflow(map[string]any{
			"model": "doubao-dialog",
			"realtime": map[string]any{
				"session": map[string]any{
					"model":         "O",
					"system_role":   "简短回答。",
					"vad_window_ms": 180,
				},
				"input": map[string]any{
					"format":    "speech_opus",
					"transcode": true,
				},
				"output": map[string]any{
					"speaker": "speaker-id",
					"format":  "ogg_opus",
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	stream, err := agent.Transform(context.Background(), "ignored", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	got := string(chunk.Part.(genx.Text))
	if !strings.HasPrefix(got, "model/doubao-dialog?") ||
		!strings.Contains(got, "speaker=workspace-speaker") ||
		!strings.Contains(got, "upstream_model=O") ||
		!strings.Contains(got, "vad_window_ms=180") ||
		!strings.Contains(got, "system_role=workspace+%E8%A6%86%E7%9B%96%E3%80%82") ||
		strings.Contains(got, "format=") ||
		strings.Contains(got, "input_format=") ||
		strings.Contains(got, "input_transcode=") {
		t.Fatalf("pattern = %q, want workflow realtime query params", got)
	}
}

func TestFactoryMergesRealtimeConfigAndWorkspaceParams(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	workspaceParams := map[string]any{
		"agent_type":     Type,
		"model":          "ignored-workspace-model",
		"realtime_model": "ignored-realtime-model",
		"temperature":    0.5,
		"realtime_config": map[string]any{
			"input": map[string]any{
				"channel": int64(1),
			},
			"output": map[string]any{
				"voice": "workspace-voice",
			},
		},
	}
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: testDoubaoRealtimeWorkspaceParameters(workspaceParams)},
		Workflow: testDoubaoRealtimeWorkflow(map[string]any{
			"realtime_model": "model/realtime?resource_id=base-resource",
			"realtime_config": map[string]any{
				"session": map[string]any{
					"bot_name": "豆包",
					"model":    "O",
				},
				"input": map[string]any{
					"sample_rate": 16000,
				},
			},
			"realtime": map[string]any{
				"output": map[string]any{
					"speaker": "workflow-speaker",
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	stream, err := agent.Transform(context.Background(), "ignored", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	got := string(chunk.Part.(genx.Text))
	if !strings.HasPrefix(got, "model/realtime?") ||
		!strings.Contains(got, "resource_id=base-resource") ||
		!strings.Contains(got, "bot_name=%E8%B1%86%E5%8C%85") ||
		!strings.Contains(got, "upstream_model=O") ||
		!strings.Contains(got, "speaker=workspace-voice") ||
		!strings.Contains(got, "temperature=0.5") ||
		strings.Contains(got, "input_sample_rate=") ||
		strings.Contains(got, "input_channels=") ||
		strings.Contains(got, "ignored-workspace-model") ||
		strings.Contains(got, "ignored-realtime-model") {
		t.Fatalf("pattern = %q, want merged realtime params with workspace overrides", got)
	}
}

func TestFactoryUsesWorkspaceModelAndRealtimeParams(t *testing.T) {
	factory := Factory{Transformer: recordingTransformer{}}
	workspaceParams := map[string]any{
		"agent_type":     Type,
		"realtime_model": "doubao-dialog",
		"temperature":    0.4,
		"realtime": map[string]any{
			"session": map[string]any{
				"model":       "O",
				"system_role": "简短回答。",
			},
			"input": map[string]any{
				"format": "speech_opus",
			},
			"output": map[string]any{
				"voice": "workspace-voice",
			},
		},
	}
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: testDoubaoRealtimeWorkspaceParameters(workspaceParams)},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	stream, err := agent.Transform(context.Background(), "ignored", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	got := string(chunk.Part.(genx.Text))
	if !strings.HasPrefix(got, "model/doubao-dialog?") ||
		!strings.Contains(got, "upstream_model=O") ||
		!strings.Contains(got, "system_role=%E7%AE%80%E7%9F%AD%E5%9B%9E%E7%AD%94%E3%80%82") ||
		!strings.Contains(got, "speaker=workspace-voice") ||
		!strings.Contains(got, "temperature=0.4") ||
		strings.Contains(got, "format=") {
		t.Fatalf("pattern = %q, want workspace realtime params", got)
	}
}

func TestMergeDoubaoRealtimeTypedParamsCoversOptionalSections(t *testing.T) {
	input := apitypes.WorkspaceInputModeRealtime
	temperature := float32(0.7)
	vad := 320
	searchEnabled := true
	resultCount := 3
	musicEnabled := true
	var voice apitypes.DoubaoRealtimeVoiceParameters
	if err := voice.FromDoubaoRealtimeInternalSpeakerParameters(apitypes.DoubaoRealtimeInternalSpeakerParameters{RealtimeSpeakerId: "speaker-a"}); err != nil {
		t.Fatalf("FromDoubaoRealtimeInternalSpeakerParameters() error = %v", err)
	}
	params := mergeDoubaoRealtimeTypedParams(nil, apitypes.DoubaoRealtimeWorkspaceParameters{
		Input:       &input,
		Temperature: &temperature,
		Session: &apitypes.DoubaoRealtimeSessionParameters{
			BotName:           stringPtr("bot"),
			SystemRole:        stringPtr("role"),
			UpstreamModel:     stringPtr("model"),
			VadWindowMs:       &vad,
			SpeakingStyle:     stringPtr("calm"),
			CharacterManifest: stringPtr("manifest"),
			ResourceId:        stringPtr("resource"),
		},
		Search: &apitypes.DoubaoRealtimeSearchParameters{
			Enabled:         &searchEnabled,
			Type:            stringPtr("web"),
			BotId:           stringPtr("bot-id"),
			ResultCount:     &resultCount,
			NoResultMessage: stringPtr("none"),
		},
		Music: &apitypes.DoubaoRealtimeMusicParameters{Enabled: &musicEnabled},
		Voice: &voice,
	})
	for key, want := range map[string]any{
		"mode":                     "realtime",
		"temperature":              temperature,
		"bot_name":                 "bot",
		"system_role":              "role",
		"upstream_model":           "model",
		"vad_window_ms":            vad,
		"speaking_style":           "calm",
		"character_manifest":       "manifest",
		"resource_id":              "resource",
		"search_enabled":           true,
		"search_type":              "web",
		"search_bot_id":            "bot-id",
		"search_result_count":      resultCount,
		"search_no_result_message": "none",
		"music_enabled":            true,
		"speaker":                  "speaker-a",
	} {
		if got := params[key]; got != want {
			t.Fatalf("params[%q] = %#v, want %#v; params=%#v", key, got, want, params)
		}
	}
}

func TestFactoryValidation(t *testing.T) {
	if _, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "transformer") {
		t.Fatalf("NewAgent(missing transformer) error = %v", err)
	}
	if _, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("NewAgent(missing model) error = %v", err)
	}
}

type recordingTransformer struct{}

func (recordingTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	return &singleChunkStream{chunk: &genx.MessageChunk{Part: genx.Text(pattern)}}, nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }

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

func (*singleChunkStream) Close() error {
	return nil
}

func (*singleChunkStream) CloseWithError(error) error {
	return nil
}
