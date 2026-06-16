package doubaorealtime

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/agenthost"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

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
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
		Workflow: apitypes.WorkflowDocument{
			ApiVersion: apitypes.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
			Kind:       apitypes.FlowcraftWorkflowKindFlowcraftWorkflow,
			Metadata:   apitypes.WorkflowMetadata{Name: "demo-workflow"},
			Spec: apitypes.FlowcraftWorkflowSpec{
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
			},
		},
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
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
		Workflow: apitypes.WorkflowDocument{
			ApiVersion: apitypes.WorkflowAPIVersionGizclawFlowcraftv1alpha1,
			Kind:       apitypes.FlowcraftWorkflowKindFlowcraftWorkflow,
			Metadata:   apitypes.WorkflowMetadata{Name: "demo-workflow"},
			Spec: apitypes.FlowcraftWorkflowSpec{
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
			},
		},
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
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
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
