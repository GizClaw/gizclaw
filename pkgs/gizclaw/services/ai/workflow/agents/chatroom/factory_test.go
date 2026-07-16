package chatroom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestFactoryCreatesChatRoomAgent(t *testing.T) {
	params := validWorkspaceParameters(t)
	agent, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &params},
		Workflow:  validWorkflow(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if agent == nil {
		t.Fatal("NewAgent() = nil")
	}
}

func TestFactoryRejectsInvalidSpec(t *testing.T) {
	for name, tc := range map[string]struct {
		spec    agenthost.Spec
		wantErr string
	}{
		"missing chatroom spec": {
			spec: agenthost.Spec{
				Workflow: apitypes.Workflow{
					Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverChatroom},
				},
			},
			wantErr: "spec.chatroom is required",
		},
		"wrong workspace parameters": {
			spec: agenthost.Spec{
				Workflow:  validWorkflow(),
				Workspace: apitypes.Workspace{Parameters: rawWorkspaceParameters(t, `{"agent_type":"flowcraft"}`)},
			},
			wantErr: "unsupported agent_type",
		},
		"bad workspace mode": {
			spec: agenthost.Spec{
				Workflow:  validWorkflow(),
				Workspace: apitypes.Workspace{Parameters: rawWorkspaceParameters(t, `{"agent_type":"chatroom","mode":"bad"}`)},
			},
			wantErr: "unsupported mode",
		},
		"bad workspace input": {
			spec: agenthost.Spec{
				Workflow:  validWorkflow(),
				Workspace: apitypes.Workspace{Parameters: rawWorkspaceParameters(t, `{"agent_type":"chatroom","input":"bad"}`)},
			},
			wantErr: "unsupported input",
		},
		"transcript enabled without transformer": {
			spec: agenthost.Spec{
				Workflow: validWorkflowWithTranscript("asr", true),
			},
			wantErr: "transformer is required",
		},
		"transcript enabled without asr model": {
			spec: agenthost.Spec{
				Workflow: validWorkflowWithTranscript("", true),
			},
			wantErr: "transcript.asr_model is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (Factory{}).NewAgent(context.Background(), tc.spec)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("NewAgent() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestAgentTransformForwardsTextInputAsTranscript(t *testing.T) {
	agent, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflow(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &recordingStream{
		chunks: []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: genx.Text("hello")},
			genx.NewTextEndOfStream(),
		},
		doneErr: genx.ErrDone,
	}
	output, err := agent.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	chunk, err := output.Next()
	if err != nil {
		t.Fatalf("output.Next() text error = %v", err)
	}
	if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel || chunk.Ctrl == nil || chunk.Ctrl.Label != transcriptLabel || chunk.Ctrl.StreamID != defaultInputStreamID || chunk.Part != genx.Text("hello") || chunk.IsEndOfStream() {
		t.Fatalf("text chunk = %#v", chunk)
	}
	chunk, err = output.Next()
	if err != nil {
		t.Fatalf("output.Next() eos error = %v", err)
	}
	if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel || chunk.Ctrl == nil || chunk.Ctrl.Label != transcriptLabel || chunk.Ctrl.StreamID != defaultInputStreamID || !chunk.IsEndOfStream() {
		t.Fatalf("text EOS = %#v", chunk)
	}
	if chunk, err := output.Next(); !errors.Is(err, genx.ErrDone) || chunk != nil {
		t.Fatalf("output.Next() = %#v, %v; want ErrDone", chunk, err)
	}
	if !input.waitClosed(100 * time.Millisecond) {
		t.Fatal("input stream was not closed")
	}
	if input.nexts != 3 {
		t.Fatalf("input Next calls = %d, want 3", input.nexts)
	}
}

func TestAgentTransformDrainsAudioInputWhenTranscriptDisabled(t *testing.T) {
	agent, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflow(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &recordingStream{
		chunks: []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
		},
		doneErr: genx.ErrDone,
	}
	output, err := agent.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if chunk, err := output.Next(); !errors.Is(err, genx.ErrDone) || chunk != nil {
		t.Fatalf("output.Next() = %#v, %v; want ErrDone without chunks", chunk, err)
	}
	if !input.waitClosed(100 * time.Millisecond) {
		t.Fatal("input stream was not closed")
	}
	if input.nexts != 3 {
		t.Fatalf("input Next calls = %d, want 3", input.nexts)
	}
}

func TestAgentTransformRejectsNilInput(t *testing.T) {
	agent, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflow(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if _, err := agent.Transform(context.Background(), "demo", nil); err == nil || !strings.Contains(err.Error(), "input stream is required") {
		t.Fatalf("Transform(nil) error = %v, want input stream error", err)
	}
}

func TestAgentTransformPropagatesInputError(t *testing.T) {
	agent, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflow(),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	want := errors.New("input failed")
	output, err := agent.Transform(context.Background(), "demo", &recordingStream{doneErr: want})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, want) {
		t.Fatalf("output.Next() error = %v, want %v", err, want)
	}
}

func TestWorkspaceTranscriptOverrideDisablesWorkflowTranscript(t *testing.T) {
	params := rawWorkspaceParameters(t, `{"agent_type":"chatroom","transcript":{"enabled":false}}`)
	agent, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workflow:  validWorkflowWithTranscript("asr", true),
		Workspace: apitypes.Workspace{Parameters: params},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if agent == nil {
		t.Fatal("NewAgent() = nil")
	}
}

func TestWorkspaceTranscriptOverrideModel(t *testing.T) {
	enabled := true
	model := "workspace-asr"
	var params apitypes.WorkspaceParameters
	if err := params.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Transcript: &apitypes.ChatRoomWorkspaceTranscriptParameters{Enabled: &enabled, AsrModel: &model},
	}); err != nil {
		t.Fatalf("FromChatRoomWorkspaceParameters() error = %v", err)
	}
	transformer := &scriptedASRTransformer{text: "hello"}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow:  validWorkflowWithTranscript("workflow-asr", true),
		Workspace: apitypes.Workspace{Parameters: &params},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	output, err := agent.Transform(context.Background(), "demo", &recordingStream{
		chunks: []*genx.MessageChunk{
			{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{EndOfStream: true}},
		},
		doneErr: genx.ErrDone,
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_ = drainOutput(t, output)
	if transformer.pattern != "model/workspace-asr" {
		t.Fatalf("ASR pattern = %q, want model/workspace-asr", transformer.pattern)
	}
}

func TestAgentTransformTranscriptForwardsTextOnlyInput(t *testing.T) {
	transformer := &scriptedASRTransformer{text: "unused"}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &recordingStream{
		chunks: []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: genx.Text("hello")},
			genx.NewTextEndOfStream(),
		},
		doneErr: genx.ErrDone,
	}
	output, err := agent.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	chunk, err := output.Next()
	if err != nil {
		t.Fatalf("output.Next() text error = %v", err)
	}
	if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel || chunk.Ctrl == nil || chunk.Ctrl.Label != transcriptLabel || chunk.Ctrl.StreamID != defaultInputStreamID || chunk.Part != genx.Text("hello") || chunk.IsEndOfStream() {
		t.Fatalf("text chunk = %#v", chunk)
	}
	chunk, err = output.Next()
	if err != nil {
		t.Fatalf("output.Next() eos error = %v", err)
	}
	if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel || chunk.Ctrl == nil || chunk.Ctrl.Label != transcriptLabel || chunk.Ctrl.StreamID != defaultInputStreamID || !chunk.IsEndOfStream() {
		t.Fatalf("text EOS = %#v", chunk)
	}
	if chunk, err := output.Next(); !isStreamDone(err) || chunk != nil {
		t.Fatalf("output.Next() = %#v, %v; want done", chunk, err)
	}
	if transformer.pattern != "" {
		t.Fatalf("ASR pattern = %q, want no ASR call", transformer.pattern)
	}
	if !input.waitClosed(100 * time.Millisecond) {
		t.Fatal("input stream was not closed")
	}
}

func TestAgentTransformTranscriptClosesMultipleTextStreams(t *testing.T) {
	transformer := &scriptedASRTransformer{text: "unused"}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &recordingStream{
		chunks: []*genx.MessageChunk{
			{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("one"), Ctrl: &genx.StreamCtrl{StreamID: "text-1", Label: "transcript"}},
			{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "text-1", Label: "transcript", EndOfStream: true}},
			{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("two"), Ctrl: &genx.StreamCtrl{StreamID: "text-2", Label: "transcript"}},
		},
		doneErr: genx.ErrDone,
	}
	output, err := agent.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	chunks := drainOutput(t, output)
	if transformer.pattern != "" {
		t.Fatalf("ASR pattern = %q, want no ASR call", transformer.pattern)
	}
	if len(chunks) != 4 {
		t.Fatalf("chunks len = %d, want 4: %#v", len(chunks), chunks)
	}
	want := []struct {
		streamID string
		text     genx.Text
		eos      bool
	}{
		{streamID: "text-1", text: "one"},
		{streamID: "text-1", eos: true},
		{streamID: "text-2", text: "two"},
		{streamID: "text-2", eos: true},
	}
	for i, tc := range want {
		chunk := chunks[i]
		if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel || chunk.Ctrl == nil || chunk.Ctrl.Label != transcriptLabel || chunk.Ctrl.StreamID != tc.streamID || chunk.IsEndOfStream() != tc.eos || chunk.Part != tc.text {
			t.Fatalf("chunk[%d] = %#v, want stream=%q text=%q eos=%t", i, chunk, tc.streamID, tc.text, tc.eos)
		}
	}
}

func TestAgentTransformReportsASRStartError(t *testing.T) {
	want := errors.New("asr unavailable")
	agent, err := (Factory{Transformer: errorTransformer{err: want}}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	output, err := agent.Transform(context.Background(), "demo", &recordingStream{
		chunks: []*genx.MessageChunk{
			{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}},
		},
		doneErr: genx.ErrDone,
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, want) {
		t.Fatalf("output.Next() error = %v, want %v", err, want)
	}
}

func TestAgentTransformReportsAudioInputError(t *testing.T) {
	want := errors.New("input failed")
	agent, err := (Factory{Transformer: &scriptedASRTransformer{text: "unused"}}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	output, err := agent.Transform(context.Background(), "demo", &recordingStream{
		chunks: []*genx.MessageChunk{
			{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}},
		},
		doneErr: want,
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, want) {
		t.Fatalf("output.Next() error = %v, want %v", err, want)
	}
}

func TestAgentTransformPreservesASRInputConsumerError(t *testing.T) {
	want := errors.New("asr input consumer failed")
	transformer := &consumerErrorASRTransformer{err: want, done: make(chan struct{})}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-a"},
	}); err != nil {
		t.Fatalf("input.Add() error = %v", err)
	}
	output, err := agent.Transform(context.Background(), "demo", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, want) {
		t.Fatalf("output.Next() error = %v, want %v", err, want)
	} else if errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output.Next() replaced consumer error with transport error: %v", err)
	}
	select {
	case <-transformer.done:
	case <-time.After(time.Second):
		t.Fatal("ASR transformer did not exit after consumer error")
	}
}

func TestAgentTransformCancellationStopsASRPipeline(t *testing.T) {
	transformer := &blockingASRTransformer{
		ready:    make(chan struct{}),
		inputErr: make(chan error, 1),
		done:     make(chan struct{}),
	}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-a"},
	}); err != nil {
		t.Fatalf("input.Add() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, err := agent.Transform(ctx, "demo", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	select {
	case <-transformer.ready:
	case <-time.After(time.Second):
		t.Fatal("ASR transformer did not start reading input")
	}
	cancel()
	if _, err := output.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("output.Next() error = %v, want context.Canceled", err)
	}
	select {
	case err := <-transformer.inputErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ASR input error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ASR input reader did not unblock after cancellation")
	}
	select {
	case <-transformer.done:
	case <-time.After(time.Second):
		t.Fatal("ASR transformer did not exit after cancellation")
	}
	if got := transformer.outputCloses.Load(); got != 1 {
		t.Fatalf("ASR output close calls = %d, want 1", got)
	}
	if err := input.Add(&genx.MessageChunk{Part: genx.Text("late")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("input.Add() after cancellation error = %v, want context.Canceled", err)
	}
}

func TestAgentTransformTranscribesAudioInput(t *testing.T) {
	transformer := &scriptedASRTransformer{text: "hello"}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &recordingStream{
		chunks: []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a", Label: "input"}},
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a", Label: "input", EndOfStream: true}},
		},
		doneErr: genx.ErrDone,
	}
	output, err := agent.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainOutput(t, output)
	if transformer.pattern != "model/asr" {
		t.Fatalf("ASR pattern = %q, want model/asr", transformer.pattern)
	}
	if len(transformer.audio) != 1 || string(transformer.audio[0]) != string([]byte{1, 2, 3}) {
		t.Fatalf("ASR audio = %#v", transformer.audio)
	}
	var historyAudio, historyAudioEOS, transcriptText, transcriptEOS bool
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel || chunk.Ctrl.StreamID != "turn-a" {
			t.Fatalf("unexpected output chunk route = %#v", chunk)
		}
		switch chunk.Ctrl.Label {
		case genx.HistoryUserAudioLabel:
			if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/opus" && len(blob.Data) > 0 {
				historyAudio = true
			}
			if chunk.IsEndOfStream() {
				historyAudioEOS = true
			}
		case transcriptLabel:
			if text, ok := chunk.Part.(genx.Text); ok && text == "hello" && !chunk.IsEndOfStream() {
				transcriptText = true
			}
			if chunk.IsEndOfStream() {
				transcriptEOS = true
			}
		default:
			t.Fatalf("unexpected output label = %#v", chunk)
		}
	}
	if !historyAudio || !historyAudioEOS || !transcriptText || !transcriptEOS {
		t.Fatalf("output chunks missing flags audio=%t audioEOS=%t transcript=%t transcriptEOS=%t chunks=%#v", historyAudio, historyAudioEOS, transcriptText, transcriptEOS, chunks)
	}
	if !input.waitClosed(100 * time.Millisecond) {
		t.Fatal("input stream was not closed")
	}
}

func TestASRInputTransportConsumerCloseBeforeProducerDone(t *testing.T) {
	transport := newASRInputTransport(nil)
	consumer := transport.Stream()
	want := &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-a", EndOfStream: true},
	}
	if err := transport.Add(want); err != nil {
		t.Fatalf("transport.Add() error = %v", err)
	}
	got, err := consumer.Next()
	if err != nil {
		t.Fatalf("consumer.Next() error = %v", err)
	}
	if got == nil || !got.IsEndOfStream() || got.Ctrl.StreamID != "turn-a" {
		t.Fatalf("consumer.Next() = %#v, want audio EOS", got)
	}
	if err := consumer.Close(); err != nil {
		t.Fatalf("consumer.Close() error = %v", err)
	}
	if err := transport.Done(); err != nil {
		t.Fatalf("transport.Done() after consumer close error = %v", err)
	}
	if chunk, err := consumer.Next(); !isStreamDone(err) || chunk != nil {
		t.Fatalf("consumer.Next() after Done = %#v, %v; want done", chunk, err)
	}
}

func TestASRInputTransportAbortUnblocksPendingDone(t *testing.T) {
	transport := newASRInputTransport(nil)
	consumer := transport.Stream()
	for range 64 {
		if err := transport.Add(&genx.MessageChunk{Part: genx.Text("audio")}); err != nil {
			t.Fatalf("transport.Add() error = %v", err)
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- transport.Done()
	}()
	deadline := time.Now().Add(time.Second)
	for {
		transport.mu.Lock()
		completing := transport.completing != nil
		transport.mu.Unlock()
		if completing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("transport.Done() did not enter completion")
		}
		runtime.Gosched()
	}
	want := errors.New("consumer stopped")
	if err := consumer.CloseWithError(want); err != nil {
		t.Fatalf("consumer.CloseWithError() error = %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("transport.Done() error = %v, want %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatal("transport.Done() remained blocked after consumer error")
	}
}

func TestAgentTransformTranscribesAudioInputAddsHistoryEOSOnInputDone(t *testing.T) {
	transformer := &scriptedASRTransformer{text: "hello"}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow: validWorkflowWithTranscript("asr", true),
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	input := &recordingStream{
		chunks: []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: " audio/ogg ; codecs=opus ", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-a", Label: "input"}},
		},
		doneErr: genx.ErrDone,
	}
	output, err := agent.Transform(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainOutput(t, output)
	var historyEOS *genx.MessageChunk
	for _, chunk := range chunks {
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel && chunk.IsEndOfStream() {
			historyEOS = chunk
			break
		}
	}
	if historyEOS == nil {
		t.Fatalf("history audio EOS missing: %#v", chunks)
	}
	blob, ok := historyEOS.Part.(*genx.Blob)
	if !ok || blob.MIMEType != " audio/ogg ; codecs=opus " || len(blob.Data) != 0 {
		t.Fatalf("history audio EOS part = %#v, want empty original MIME blob", historyEOS.Part)
	}
	if historyEOS.Role != genx.RoleUser || historyEOS.Name != transcriptLabel || historyEOS.Ctrl.StreamID != "turn-a" {
		t.Fatalf("history audio EOS route = %#v", historyEOS)
	}
	if !input.waitClosed(100 * time.Millisecond) {
		t.Fatal("input stream was not closed")
	}
}

func TestAgentTransformRealtimeTranscribesASRSegmentStreams(t *testing.T) {
	inputMode := apitypes.WorkspaceInputModeRealtime
	var params apitypes.WorkspaceParameters
	if err := params.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{Input: &inputMode}); err != nil {
		t.Fatalf("FromChatRoomWorkspaceParameters() error = %v", err)
	}
	transformer := &realtimeASRTransformer{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Name: transcriptLabel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: genx.HistoryUserAudioLabel}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
		{Role: genx.RoleUser, Name: transcriptLabel, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: transcriptLabel, BeginOfStream: true}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: transcriptLabel}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: transcriptLabel, EndOfStream: true}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1:asr:2", Label: genx.HistoryUserAudioLabel}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1:asr:2", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
		{Role: genx.RoleUser, Name: transcriptLabel, Ctrl: &genx.StreamCtrl{StreamID: "audio-1:asr:2", Label: transcriptLabel, BeginOfStream: true}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1:asr:2", Label: transcriptLabel}},
		{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1:asr:2", Label: transcriptLabel, EndOfStream: true}},
	}}
	agent, err := (Factory{Transformer: transformer}).NewAgent(context.Background(), agenthost.Spec{
		Workflow:  validWorkflowWithTranscript("asr", true),
		Workspace: apitypes.Workspace{Parameters: &params},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, err := agent.Transform(ctx, "demo", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	if err := input.Add(
		genx.NewBeginOfStream("audio-1"),
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{7}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "input"}},
	); err != nil {
		t.Fatalf("input.Add() error = %v", err)
	}

	chunks := make([]*genx.MessageChunk, 0, len(transformer.chunks))
	for len(chunks) < len(transformer.chunks) {
		chunks = append(chunks, nextOutputChunk(t, output))
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input.Done() error = %v", err)
	}
	if chunk, err := output.Next(); !isStreamDone(err) || chunk != nil {
		t.Fatalf("output.Next() after realtime chunks = %#v, %v; want done", chunk, err)
	}
	if transformer.pattern != "model/asr?emit_interim=true" {
		t.Fatalf("ASR pattern = %q, want realtime emit_interim pattern", transformer.pattern)
	}

	transcripts := map[string]string{}
	historyData := map[string]int{}
	historyEOS := map[string]int{}
	transcriptEOS := map[string]int{}
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			t.Fatalf("unexpected nil routed chunk = %#v", chunk)
		}
		if chunk.Role != genx.RoleUser || chunk.Name != transcriptLabel {
			t.Fatalf("chunk route = %#v, want user transcript", chunk)
		}
		switch chunk.Ctrl.Label {
		case genx.HistoryUserAudioLabel:
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				historyData[chunk.Ctrl.StreamID]++
			}
			if chunk.IsEndOfStream() {
				historyEOS[chunk.Ctrl.StreamID]++
			}
		case transcriptLabel:
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				transcripts[chunk.Ctrl.StreamID] += string(text)
			}
			if chunk.IsEndOfStream() {
				transcriptEOS[chunk.Ctrl.StreamID]++
			}
		default:
			t.Fatalf("unexpected label = %#v", chunk.Ctrl)
		}
	}
	if transcripts["audio-1"] != "first" || transcripts["audio-1:asr:2"] != "second" {
		t.Fatalf("transcripts by stream = %#v", transcripts)
	}
	if historyData["audio-1"] != 1 || historyData["audio-1:asr:2"] != 1 {
		t.Fatalf("history data by stream = %#v", historyData)
	}
	if historyEOS["audio-1"] != 1 || historyEOS["audio-1:asr:2"] != 1 || transcriptEOS["audio-1"] != 1 || transcriptEOS["audio-1:asr:2"] != 1 {
		t.Fatalf("eos counts history=%#v transcript=%#v", historyEOS, transcriptEOS)
	}
	if len(transformer.audio) != 1 || string(transformer.audio[0]) != string([]byte{7}) {
		t.Fatalf("ASR audio = %#v, want one forwarded input packet", transformer.audio)
	}
}

func TestChunkHelpers(t *testing.T) {
	if isAudioChunk(nil) {
		t.Fatal("isAudioChunk(nil) = true")
	}
	if isAudioChunk(&genx.MessageChunk{Part: genx.Text("hello")}) {
		t.Fatal("isAudioChunk(text) = true")
	}
	if !isAudioChunk(&genx.MessageChunk{Part: &genx.Blob{MIMEType: " Audio/OGG ; codecs=opus "}}) {
		t.Fatal("isAudioChunk(audio/ogg) = false")
	}
	chunk := textChunk("", "hello", false)
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != defaultInputStreamID {
		t.Fatalf("textChunk default stream = %#v", chunk)
	}
	if got := baseMIME(" Audio/OGG ; codecs=opus "); got != "audio/ogg" {
		t.Fatalf("baseMIME = %q, want audio/ogg", got)
	}
}

func TestNormalizeASRTranscriptChunk(t *testing.T) {
	if got := normalizeASRTranscriptChunk(nil, "fallback"); got != nil {
		t.Fatalf("normalizeASRTranscriptChunk(nil) = %#v, want nil", got)
	}
	if got := normalizeASRTranscriptChunk(&genx.MessageChunk{Part: genx.Text("")}, "fallback"); got != nil {
		t.Fatalf("normalizeASRTranscriptChunk(empty text) = %#v, want nil", got)
	}

	text := normalizeASRTranscriptChunk(&genx.MessageChunk{Part: genx.Text("hello")}, " fallback ")
	if text == nil || text.Role != genx.RoleUser || text.Name != transcriptLabel || text.Ctrl == nil || text.Ctrl.StreamID != "fallback" || text.Ctrl.Label != transcriptLabel || text.Part != genx.Text("hello") {
		t.Fatalf("normalized text = %#v", text)
	}

	eos := normalizeASRTranscriptChunk(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true}}, "")
	if eos == nil || eos.Ctrl == nil || eos.Ctrl.StreamID != defaultInputStreamID || eos.Ctrl.Label != transcriptLabel || eos.Part != genx.Text("") || !eos.IsEndOfStream() {
		t.Fatalf("normalized eos = %#v", eos)
	}

	historyAudio := normalizeASRTranscriptChunk(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{Label: genx.HistoryUserAudioLabel},
	}, "")
	if historyAudio == nil || historyAudio.Role != genx.RoleUser || historyAudio.Name != transcriptLabel || historyAudio.Ctrl == nil || historyAudio.Ctrl.StreamID != defaultInputStreamID || historyAudio.Ctrl.Label != genx.HistoryUserAudioLabel {
		t.Fatalf("normalized history audio = %#v", historyAudio)
	}

	bos := normalizeASRTranscriptChunk(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "asr-1", BeginOfStream: true}}, "")
	if bos == nil || bos.Ctrl == nil || bos.Ctrl.StreamID != "asr-1" || !bos.IsBeginOfStream() {
		t.Fatalf("normalized bos = %#v", bos)
	}
}

func TestReadTranscriptReportsASRReadError(t *testing.T) {
	want := errors.New("asr read failed")
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	err := readTranscript(context.Background(), &recordingStream{doneErr: want}, output, &lockedString{value: "turn-a"})
	if err == nil || !strings.Contains(err.Error(), "read ASR") || !errors.Is(err, want) {
		t.Fatalf("readTranscript() error = %v, want wrapped ASR error", err)
	}
}

func TestReadTranscriptHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	err := readTranscript(ctx, &recordingStream{}, output, &lockedString{value: "turn-a"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readTranscript(canceled) error = %v, want context.Canceled", err)
	}
}

func validWorkflow() apitypes.Workflow {
	return apitypes.Workflow{
		Name: "chatroom",
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverChatroom,
			Chatroom: &apitypes.ChatRoomWorkflowSpec{
				History: apitypes.ChatRoomWorkflowHistorySpec{},
			},
		},
	}
}

func validWorkflowWithTranscript(asrModel string, enabled bool) apitypes.Workflow {
	workflow := validWorkflow()
	if asrModel == "" {
		workflow.Spec.Chatroom.Transcript = &apitypes.ChatRoomWorkflowTranscriptSpec{Enabled: &enabled}
	} else {
		workflow.Spec.Chatroom.Transcript = &apitypes.ChatRoomWorkflowTranscriptSpec{Enabled: &enabled, AsrModel: &asrModel}
	}
	return workflow
}

func validWorkspaceParameters(t *testing.T) apitypes.WorkspaceParameters {
	t.Helper()
	mode := apitypes.ChatRoomModeDirect
	input := apitypes.WorkspaceInputModePushToTalk
	var params apitypes.WorkspaceParameters
	if err := params.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Mode:  &mode,
		Input: &input,
	}); err != nil {
		t.Fatalf("FromChatRoomWorkspaceParameters() error = %v", err)
	}
	return params
}

func rawWorkspaceParameters(t *testing.T, raw string) *apitypes.WorkspaceParameters {
	t.Helper()
	var params apitypes.WorkspaceParameters
	if err := params.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	return &params
}

func drainOutput(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	defer stream.Close()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if isStreamDone(err) {
			return chunks
		}
		if err != nil {
			t.Fatalf("output.Next() error = %v", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}

func nextOutputChunk(t *testing.T, stream genx.Stream) *genx.MessageChunk {
	t.Helper()
	ch := make(chan struct {
		chunk *genx.MessageChunk
		err   error
	}, 1)
	go func() {
		chunk, err := stream.Next()
		ch <- struct {
			chunk *genx.MessageChunk
			err   error
		}{chunk: chunk, err: err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("output.Next() error = %v", got.err)
		}
		if got.chunk == nil {
			t.Fatal("output.Next() returned nil chunk")
		}
		return got.chunk
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for output chunk")
		return nil
	}
}

type recordingStream struct {
	mu      sync.Mutex
	chunks  []*genx.MessageChunk
	idx     int
	doneErr error
	nexts   int
	closed  bool
}

func (s *recordingStream) Next() (*genx.MessageChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nexts++
	if s.idx < len(s.chunks) {
		chunk := s.chunks[s.idx]
		s.idx++
		return chunk, nil
	}
	if s.doneErr != nil {
		return nil, s.doneErr
	}
	return nil, genx.ErrDone
}

func (s *recordingStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *recordingStream) CloseWithError(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if !errors.Is(err, genx.ErrDone) {
		s.doneErr = err
	}
	return nil
}

func (s *recordingStream) waitClosed(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

type scriptedASRTransformer struct {
	mu      sync.Mutex
	pattern string
	text    string
	audio   [][]byte
}

func (t *scriptedASRTransformer) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	t.mu.Lock()
	t.pattern = pattern
	t.mu.Unlock()
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	go func() {
		streamID := defaultInputStreamID
		audioSeen := false
		lastAudioMIME := "audio/opus"
		var history []*genx.MessageChunk
		for {
			chunk, err := input.Next()
			if err != nil {
				if errors.Is(err, io.EOF) || isStreamDone(err) {
					break
				}
				_ = output.Abort(fmt.Errorf("fake ASR input: %w", err))
				return
			}
			if chunk == nil {
				continue
			}
			if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) != "" {
				streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				audioSeen = true
				if strings.TrimSpace(blob.MIMEType) != "" {
					lastAudioMIME = blob.MIMEType
				}
				t.mu.Lock()
				t.audio = append(t.audio, append([]byte(nil), blob.Data...))
				t.mu.Unlock()
				history = append(history, &genx.MessageChunk{
					Role: genx.RoleUser,
					Name: transcriptLabel,
					Part: &genx.Blob{MIMEType: blob.MIMEType, Data: append([]byte(nil), blob.Data...)},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel},
				})
			}
			if chunk.IsEndOfStream() {
				break
			}
		}
		if err := input.Close(); err != nil {
			_ = output.Abort(fmt.Errorf("fake ASR close input: %w", err))
			return
		}
		if audioSeen {
			history = append(history, &genx.MessageChunk{
				Role: genx.RoleUser,
				Name: transcriptLabel,
				Part: &genx.Blob{MIMEType: lastAudioMIME},
				Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel, EndOfStream: true},
			})
		}
		for _, chunk := range history {
			if err := output.Add(chunk); err != nil {
				return
			}
		}
		_ = output.Add(
			&genx.MessageChunk{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text(t.text), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: transcriptLabel}},
			&genx.MessageChunk{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: transcriptLabel, EndOfStream: true}},
		)
		_ = output.Done(genx.Usage{})
	}()
	return output.Stream(), nil
}

type consumerErrorASRTransformer struct {
	err  error
	done chan struct{}
}

func (t *consumerErrorASRTransformer) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	go func() {
		defer close(t.done)
		if _, err := input.Next(); err != nil {
			_ = output.Abort(err)
			return
		}
		_ = input.CloseWithError(t.err)
	}()
	return output.Stream(), nil
}

type blockingASRTransformer struct {
	ready        chan struct{}
	inputErr     chan error
	done         chan struct{}
	outputCloses atomic.Int32
}

func (t *blockingASRTransformer) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	go func() {
		defer close(t.done)
		defer input.Close()
		if _, err := input.Next(); err != nil {
			t.inputErr <- err
			_ = output.Abort(err)
			return
		}
		close(t.ready)
		_, err := input.Next()
		t.inputErr <- err
		if err != nil {
			_ = output.Abort(err)
			return
		}
		_ = output.Done(genx.Usage{})
	}()
	return &closeCountingStream{Stream: output.Stream(), closes: &t.outputCloses}, nil
}

type closeCountingStream struct {
	genx.Stream
	closes *atomic.Int32
}

func (s *closeCountingStream) Close() error {
	s.closes.Add(1)
	return s.Stream.Close()
}

func (s *closeCountingStream) CloseWithError(err error) error {
	s.closes.Add(1)
	return s.Stream.CloseWithError(err)
}

type realtimeASRTransformer struct {
	mu      sync.Mutex
	pattern string
	chunks  []*genx.MessageChunk
	audio   [][]byte
}

func (t *realtimeASRTransformer) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	t.mu.Lock()
	t.pattern = pattern
	t.mu.Unlock()
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), len(t.chunks)+2)
	go func() {
		defer input.Close()
		emitted := false
		for {
			chunk, err := input.Next()
			if err != nil {
				if errors.Is(err, io.EOF) || isStreamDone(err) {
					break
				}
				_ = output.Abort(fmt.Errorf("fake realtime ASR input: %w", err))
				return
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || len(blob.Data) == 0 {
				continue
			}
			t.mu.Lock()
			t.audio = append(t.audio, append([]byte(nil), blob.Data...))
			t.mu.Unlock()
			if emitted {
				continue
			}
			emitted = true
			for _, out := range t.chunks {
				if err := output.Add(out.Clone()); err != nil {
					return
				}
			}
		}
		_ = output.Done(genx.Usage{})
	}()
	return output.Stream(), nil
}

type errorTransformer struct {
	err error
}

func (t errorTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, t.err
}
