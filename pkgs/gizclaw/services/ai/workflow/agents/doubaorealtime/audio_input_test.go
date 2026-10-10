package doubaorealtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestFactoryProfileASRSendsTextToRealtimeModel(t *testing.T) {
	for _, mode := range []apitypes.WorkspaceInputMode{apitypes.WorkspaceInputModePushToTalk, apitypes.WorkspaceInputModeRealtime} {
		for _, audio := range []bool{true, false} {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			var asrAudio atomic.Int32
			mux := &asrModelMux{}
			factory := Factory{
				TransformerForOwner: func(context.Context, string) (genx.TransformerMux, error) { return mux, nil },
				BuildASR: func(_ context.Context, owner, alias string, gotMode apitypes.WorkspaceInputMode) (genx.Transformer, error) {
					if owner != "owner" || alias != "input.speech" || gotMode != mode {
						t.Fatalf("ASR selection: owner=%s alias=%s mode=%s", owner, alias, gotMode)
					}
					return asrStage{audio: &asrAudio}, nil
				},
			}
			agent, err := factory.NewAgent(ctx, agenthost.Spec{
				ASRModel:  "input.speech",
				Workspace: apitypes.Workspace{Id: "workspace", OwnerPublicKey: new("owner"), Parameters: testDoubaoRealtimeWorkspaceParameters(t, apitypes.DoubaoRealtimeWorkspaceParameters{Input: &mode})},
				Workflow:  testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "realtime"}),
			})
			if err != nil {
				t.Fatal(err)
			}
			var part genx.Part = genx.Text("typed input")
			want, wantASR := "typed input", int32(0)
			if audio {
				part = &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}
				want, wantASR = "ASR input", 1
			}
			output, err := agent.Transform(ctx, &singleChunkStream{chunk: &genx.MessageChunk{Role: genx.RoleUser, Part: part, Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true, EndOfStream: true}}})
			if err != nil {
				t.Fatal(err)
			}
			var reply strings.Builder
			for {
				chunk, err := output.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if text, ok := chunk.Part.(genx.Text); ok && chunk.Role == genx.RoleModel {
					reply.WriteString(string(text))
				}
			}
			_ = output.Close()
			if mux.audio != 0 || mux.text != want || reply.String() != want || asrAudio.Load() != wantASR {
				t.Fatalf("mode=%s audio=%v Model text=%q audio=%d reply=%q ASR calls=%d", mode, audio, mux.text, mux.audio, reply.String(), asrAudio.Load())
			}
		}
	}
}

func TestFactoryProfileASRFailureNeverUsesNativeAudio(t *testing.T) {
	spec := agenthost.Spec{ASRModel: "asr", Workflow: testDoubaoRealtimeWorkflow(apitypes.DoubaoRealtimeWorkflowSpec{Model: "realtime"})}
	for _, builder := range []func(context.Context, string, string, apitypes.WorkspaceInputMode) (genx.Transformer, error){
		nil,
		func(context.Context, string, string, apitypes.WorkspaceInputMode) (genx.Transformer, error) {
			return nil, errors.New("unavailable ASR")
		},
		func(context.Context, string, string, apitypes.WorkspaceInputMode) (genx.Transformer, error) {
			return nil, nil
		},
	} {
		if _, err := (Factory{Transformer: recordingTransformer{}, BuildASR: builder}).NewAgent(t.Context(), spec); err == nil {
			t.Fatal("invalid explicit ASR fell back to native audio")
		}
	}
}

type asrStage struct{ audio *atomic.Int32 }

func (s asrStage) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	return &asrStageStream{Stream: input, audio: s.audio}, nil
}

type asrStageStream struct {
	genx.Stream
	audio *atomic.Int32
}

func (s *asrStageStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if chunk == nil || err != nil {
		return chunk, err
	}
	if _, ok := chunk.Part.(*genx.Blob); ok {
		s.audio.Add(1)
		chunk = chunk.Clone()
		chunk.Part, chunk.Name, chunk.Ctrl.Label = genx.Text("ASR input"), "transcript", "transcript"
	}
	return chunk, nil
}

type asrModelMux struct {
	text  string
	audio int
}

func (m *asrModelMux) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return &asrModelStream{Stream: input, model: m}, nil
}

type asrModelStream struct {
	genx.Stream
	model *asrModelMux
}

func (s *asrModelStream) Next() (*genx.MessageChunk, error) {
	for {
		chunk, err := s.Stream.Next()
		if err != nil {
			return nil, err
		}
		if _, ok := chunk.Part.(*genx.Blob); ok {
			s.model.audio++
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			s.model.text += string(text)
		}
		if chunk.IsEndOfStream() {
			return &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(s.model.text), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true, EndOfStream: true}}, nil
		}
	}
}
