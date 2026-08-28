package eino

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestGenXModelContextPreservesToolsCallsAndResults(t *testing.T) {
	t.Parallel()
	toolInfo := &schema.ToolInfo{
		Name: "current_peer", Desc: "Read from the current Peer.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"key": {Type: schema.String, Required: true},
		}),
	}
	context, err := genXModelContext([]*schema.Message{
		schema.UserMessage("read it"),
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID: "call-1", Type: "function",
				Function: schema.FunctionCall{Name: "current_peer", Arguments: `{"key":"x"}`},
			}},
		},
		{
			Role: schema.Tool, ToolCallID: "call-1", ToolName: "current_peer",
			Content: `{"value":"ok"}`,
		},
	}, model.WithTools([]*schema.ToolInfo{toolInfo}))
	if err != nil {
		t.Fatalf("genXModelContext() error = %v", err)
	}
	var tools []*genx.FuncTool
	for item := range context.Tools() {
		tool, ok := item.(*genx.FuncTool)
		if !ok {
			t.Fatalf("Tool type = %T", item)
		}
		tools = append(tools, tool)
	}
	if len(tools) != 1 || tools[0].Name != "current_peer" || tools[0].Argument == nil {
		t.Fatalf("Tools = %#v", tools)
	}
	var messages []*genx.Message
	for message := range context.Messages() {
		messages = append(messages, message)
	}
	if len(messages) != 3 {
		t.Fatalf("Messages = %#v", messages)
	}
	call, ok := messages[1].Payload.(*genx.ToolCall)
	if !ok || call.ID != "call-1" || call.FuncCall == nil ||
		call.FuncCall.Name != "current_peer" || call.FuncCall.Arguments != `{"key":"x"}` {
		t.Fatalf("ToolCall = %#v", messages[1].Payload)
	}
	result, ok := messages[2].Payload.(*genx.ToolResult)
	if !ok || result.ID != "call-1" || result.Result != `{"value":"ok"}` {
		t.Fatalf("ToolResult = %#v", messages[2].Payload)
	}
}

func TestEinoToolCallValidatesProviderOutput(t *testing.T) {
	t.Parallel()
	got, err := einoToolCall(&genx.ToolCall{
		ID: "call-1",
		FuncCall: &genx.FuncCall{
			Name: "current_peer", Arguments: `{"key":"x"}`,
		},
	}, 2)
	if err != nil {
		t.Fatalf("einoToolCall() error = %v", err)
	}
	if got.Index == nil || *got.Index != 2 || got.ID != "call-1" ||
		got.Function.Name != "current_peer" || got.Function.Arguments != `{"key":"x"}` {
		t.Fatalf("einoToolCall() = %#v", got)
	}
	if _, err := einoToolCall(&genx.ToolCall{
		ID: "call-2", FuncCall: &genx.FuncCall{Name: "bad", Arguments: `{`},
	}, 0); err == nil {
		t.Fatal("einoToolCall() accepted invalid JSON")
	}
}

func TestFactoryAllowsMemorylessWorkflowAndRequiresResolvedStoreForMemoryNodes(t *testing.T) {
	t.Parallel()
	spec := einoFactorySpec(t)
	factory := Factory{GenX: &peergenx.Service{}}
	if _, err := factory.NewAgent(t.Context(), spec); err != nil {
		t.Fatalf("NewAgent(memoryless) error = %v", err)
	}

	addEinoMemoryRecallNode(t, spec.Workflow.Spec.Eino)
	if _, err := factory.NewAgent(t.Context(), spec); err == nil ||
		!strings.Contains(err.Error(), "Graph Memory nodes require Memory") {
		t.Fatalf("NewAgent(memory without store) error = %v", err)
	}
}

func TestFactoryRejectsLiveAudioWithoutASR(t *testing.T) {
	t.Parallel()
	blank, voice := "  ", "speech.voice"
	for _, testCase := range []struct {
		name         string
		voiceAdapter *apitypes.VoiceAdapter
	}{
		{name: "omitted voice adapter"},
		{name: "blank ASR", voiceAdapter: &apitypes.VoiceAdapter{AsrModel: &blank}},
		{name: "TTS only", voiceAdapter: &apitypes.VoiceAdapter{DefaultVoice: &voice}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			spec := einoFactorySpec(t)
			spec.Workflow.Spec.Eino.VoiceAdapter = testCase.voiceAdapter
			var ttsCalls atomic.Int32
			service := &peergenx.Service{}
			if testCase.voiceAdapter != nil && stringPointerValue(testCase.voiceAdapter.DefaultVoice) != "" {
				service = peergenx.New(peergenx.Service{
					Voices:          einoTTSResources{},
					Credentials:     einoTTSResources{},
					ProviderTenants: einoTTSResources{},
					Builder:         einoTTSBuilder{calls: &ttsCalls},
				})
			}
			agent, err := (Factory{GenX: service}).NewAgent(t.Context(), spec)
			if err != nil {
				t.Fatalf("NewAgent() error = %v", err)
			}
			if closer, ok := agent.(io.Closer); ok {
				defer closer.Close()
			}

			input := einoAudioGuardInput()
			output, err := agent.Transform(t.Context(), input.Stream())
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			defer output.Close()
			if err := input.Add(
				&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "audio-route", BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("private audio")}, Ctrl: &genx.StreamCtrl{StreamID: "audio-route"}},
				&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "text-route", BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "text-route"}},
				&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "text-route", EndOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-route", EndOfStream: true}},
			); err != nil {
				t.Fatal(err)
			}
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}

			chunks := einoCollectChunks(t, output)
			unsupported := 0
			var text strings.Builder
			for _, chunk := range chunks {
				if chunk.Ctrl != nil && chunk.Ctrl.ErrorCode == einoAudioInputUnsupportedCode {
					unsupported++
					assertEinoAudioUnsupportedTerminal(t, chunk, "audio-route")
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					text.WriteString(string(part))
				}
			}
			if unsupported != 1 {
				t.Fatalf("unsupported terminals = %d, want 1; chunks = %#v", unsupported, chunks)
			}
			if got := text.String(); got != "hello" {
				t.Fatalf("accepted text output = %q, want %q; chunks = %#v", got, "hello", chunks)
			}
			wantTTSCalls := int32(0)
			if testCase.voiceAdapter != nil && stringPointerValue(testCase.voiceAdapter.DefaultVoice) != "" {
				wantTTSCalls = 1
			}
			if got := ttsCalls.Load(); got != wantTTSCalls {
				t.Fatalf("TTS calls = %d, want %d", got, wantTTSCalls)
			}
		})
	}
}

func TestResolveEinoInputModeAndASRPattern(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name        string
		parameters  *apitypes.WorkspaceParameters
		wantMode    apitypes.WorkspaceInputMode
		wantPattern string
		wantErr     bool
	}{
		{name: "omitted defaults to push-to-talk", wantMode: apitypes.WorkspaceInputModePushToTalk, wantPattern: "model/speech.asr"},
		{name: "push-to-talk", parameters: einoWorkspaceParameters(t, apitypes.WorkspaceInputModePushToTalk), wantMode: apitypes.WorkspaceInputModePushToTalk, wantPattern: "model/speech.asr"},
		{name: "realtime", parameters: einoWorkspaceParameters(t, apitypes.WorkspaceInputModeRealtime), wantMode: apitypes.WorkspaceInputModeRealtime, wantPattern: "model/speech.asr?emit_interim=true"},
		{name: "invalid", parameters: einoWorkspaceParameters(t, apitypes.WorkspaceInputMode("invalid")), wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mode, err := resolveEinoInputMode(testCase.parameters)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("resolveEinoInputMode() accepted invalid mode")
				}
				return
			}
			if err != nil || mode != testCase.wantMode {
				t.Fatalf("resolveEinoInputMode() = %q, %v, want %q", mode, err, testCase.wantMode)
			}
			if got := einoASRPattern("speech.asr", mode); got != testCase.wantPattern {
				t.Fatalf("einoASRPattern() = %q, want %q", got, testCase.wantPattern)
			}
		})
	}
}

func TestWrapAudioSupportsASROnlyTTSOnlyAndVoiceSelection(t *testing.T) {
	t.Parallel()
	mux := einoTestMux(func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) { return input, nil })
	core := einoTestTransformer(func(_ context.Context, input genx.Stream) (genx.Stream, error) { return input, nil })
	asr, fallback := "speech.asr", "speech.default"
	for _, voice := range []apitypes.VoiceAdapter{
		{AsrModel: &asr},
		{DefaultVoice: &fallback},
		{AsrModel: &asr, DefaultVoice: &fallback},
	} {
		if _, err := wrapAudio(mux, core, voice, nil, apitypes.WorkspaceInputModePushToTalk); err != nil {
			t.Fatalf("wrapAudio(%#v) error = %v", voice, err)
		}
	}

	resolver := einoVoiceResolver(
		fallback,
		map[string]string{
			"answer":      "speech.assistant",
			"narrate":     "speech.narrator",
			"silent-node": "",
		},
		map[string]string{
			"assistant": "answer",
			"narration": "narrate",
			"silent":    "silent-node",
		},
	)
	for _, testCase := range []struct {
		name string
		want string
	}{
		{name: "assistant", want: "voice/speech.assistant"},
		{name: "narration", want: "voice/speech.narrator"},
		{name: "other", want: "voice/speech.default"},
		{name: "silent", want: "voice/speech.default"},
	} {
		got, err := resolver(t.Context(), audiodock.VoiceRequest{Name: testCase.name})
		if err != nil || got != testCase.want {
			t.Fatalf("resolve Voice(%q) = %q, %v, want %q", testCase.name, got, err, testCase.want)
		}
	}
	withoutFallback := einoVoiceResolver(
		"",
		map[string]string{"answer": "speech.assistant"},
		map[string]string{"assistant": "answer"},
	)
	if got, err := withoutFallback(t.Context(), audiodock.VoiceRequest{Name: "other"}); err != nil || got != "" {
		t.Fatalf("resolve unmapped Voice = %q, %v, want disabled", got, err)
	}
}

func TestEinoVoiceAdapterHasASR(t *testing.T) {
	t.Parallel()
	asr, blank, voice := "speech.asr", "  ", "speech.default"
	for _, testCase := range []struct {
		name  string
		voice *apitypes.VoiceAdapter
		want  bool
	}{
		{name: "omitted"},
		{name: "empty", voice: &apitypes.VoiceAdapter{}},
		{name: "blank", voice: &apitypes.VoiceAdapter{AsrModel: &blank}},
		{name: "tts only", voice: &apitypes.VoiceAdapter{DefaultVoice: &voice}},
		{name: "asr", voice: &apitypes.VoiceAdapter{AsrModel: &asr}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := einoVoiceAdapterHasASR(testCase.voice); got != testCase.want {
				t.Fatalf("einoVoiceAdapterHasASR() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestEinoAudioInputGuardRejectsLiveAudioBeforeEOS(t *testing.T) {
	t.Parallel()
	for _, mimeType := range []string{
		"audio/ogg",
		"audio/opus",
		"audio/pcm",
		"audio/L16; rate=16000; channels=1",
	} {
		t.Run(mimeType, func(t *testing.T) {
			t.Parallel()
			spy := &einoAudioGuardSpy{}
			input := einoAudioGuardInput()
			output, err := (einoAudioInputGuard{next: spy}).Transform(t.Context(), input.Stream())
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			defer output.Close()

			if err := input.Add(&genx.MessageChunk{
				Role: genx.RoleUser,
				Ctrl: &genx.StreamCtrl{StreamID: "audio-route", BeginOfStream: true},
			}, &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: &genx.Blob{MIMEType: mimeType, Data: []byte("private audio")},
				Ctrl: &genx.StreamCtrl{StreamID: "audio-route"},
			}); err != nil {
				t.Fatal(err)
			}
			terminal := einoNextChunk(t, output)
			assertEinoAudioUnsupportedTerminal(t, terminal, "audio-route")
			if downstream := spy.snapshot(); len(downstream) != 0 {
				t.Fatalf("buffered rejected route reached downstream: %#v", downstream)
			}

			if err := input.Add(&genx.MessageChunk{
				Role: genx.RoleUser,
				Part: &genx.Blob{MIMEType: mimeType, Data: []byte("more private audio")},
				Ctrl: &genx.StreamCtrl{StreamID: "audio-route"},
			}, &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: &genx.Blob{MIMEType: mimeType},
				Ctrl: &genx.StreamCtrl{StreamID: "audio-route", EndOfStream: true},
			}); err != nil {
				t.Fatal(err)
			}
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			if rest := einoCollectChunks(t, output); len(rest) != 0 {
				t.Fatalf("unexpected output after rejection = %#v", rest)
			}
			if downstream := spy.snapshot(); len(downstream) != 0 {
				t.Fatalf("rejected route reached downstream: %#v", downstream)
			}
		})
	}
}

func TestEinoAudioInputGuardKeepsRoutesIndependent(t *testing.T) {
	t.Parallel()
	spy := &einoAudioGuardSpy{}
	input := einoAudioGuardInput()
	output, err := (einoAudioInputGuard{next: spy}).Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()

	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "text", BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "text"}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(" world"), Ctrl: &genx.StreamCtrl{StreamID: "text", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("reused"), Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true, EndOfStream: true}},
	); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}

	chunks := einoCollectChunks(t, output)
	terminals := 0
	var text strings.Builder
	var reused strings.Builder
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.ErrorCode == einoAudioInputUnsupportedCode {
			terminals++
			assertEinoAudioUnsupportedTerminal(t, chunk, "audio")
		}
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == "text" {
			if part, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(part))
			}
		}
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == "audio" && chunk.Ctrl.ErrorCode == "" {
			if part, ok := chunk.Part.(genx.Text); ok {
				reused.WriteString(string(part))
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("unsupported terminals = %d, want 1; chunks = %#v", terminals, chunks)
	}
	if got := text.String(); got != "hello world" {
		t.Fatalf("accepted text = %q, want %q", got, "hello world")
	}
	if got := reused.String(); got != "reused" {
		t.Fatalf("reused route text = %q, want %q", got, "reused")
	}
}

func TestEinoAudioInputGuardPreservesNonRejectingInput(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		chunk *genx.MessageChunk
	}{
		{name: "empty audio", chunk: &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "empty"}}},
		{name: "audio eos", chunk: &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "empty", EndOfStream: true}}},
		{name: "history audio", chunk: &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "history", Label: genx.HistoryUserAudioLabel}}},
		{name: "malformed mime", chunk: &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/[", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "malformed"}}},
		{name: "non audio blob", chunk: &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "image/png", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "image"}}},
		{name: "control only", chunk: &genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "control", BeginOfStream: true}}},
		{name: "model audio", chunk: &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "model"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			input := einoAudioGuardInput()
			output, err := (einoAudioInputGuard{next: &einoAudioGuardSpy{}}).Transform(t.Context(), input.Stream())
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			defer output.Close()
			if err := input.Add(testCase.chunk); err != nil {
				t.Fatal(err)
			}
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			chunks := einoCollectChunks(t, output)
			if len(chunks) != 1 || chunks[0].Ctrl == nil || chunks[0].Ctrl.ErrorCode != "" {
				t.Fatalf("preserved chunks = %#v, want original non-error chunk", chunks)
			}
		})
	}
}

func TestEinoAudioInputGuardConcurrentTransforms(t *testing.T) {
	t.Parallel()
	guard := einoAudioInputGuard{next: &einoAudioGuardSpy{}}
	var wait sync.WaitGroup
	for index := range 16 {
		wait.Go(func() {
			input := einoAudioGuardInput()
			output, err := guard.Transform(t.Context(), input.Stream())
			if err != nil {
				t.Errorf("Transform(%d) error = %v", index, err)
				return
			}
			defer output.Close()
			streamID := string(rune('a' + index))
			if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}}); err != nil {
				t.Errorf("Add(%d) error = %v", index, err)
				return
			}
			if err := input.Done(genx.Usage{}); err != nil {
				t.Errorf("Done(%d) error = %v", index, err)
				return
			}
			chunks := einoCollectChunks(t, output)
			if len(chunks) != 1 {
				t.Errorf("Transform(%d) chunks = %#v", index, chunks)
				return
			}
			assertEinoAudioUnsupportedTerminal(t, chunks[0], streamID)
		})
	}
	wait.Wait()
}

func TestEinoAudioInputGuardCancellationClosesInput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	input := &einoAudioGuardBlockingStream{closed: make(chan struct{})}
	output, err := (einoAudioInputGuard{next: einoTestTransformer(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		return input, nil
	})}).Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	cancel()
	select {
	case <-input.closed:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close the guarded input")
	}
	if _, err := output.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next() error = %v, want context.Canceled", err)
	}
}

func TestEinoAudioInputGuardPropagatesDownstreamFailure(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("downstream failed")
	input := &einoAudioGuardBlockingStream{closed: make(chan struct{})}
	output, err := (einoAudioInputGuard{next: einoTestTransformer(func(context.Context, genx.Stream) (genx.Stream, error) {
		return einoAudioGuardErrorStream{err: wantErr}, nil
	})}).Transform(t.Context(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	if _, err := output.Next(); !errors.Is(err, wantErr) {
		t.Fatalf("Next() error = %v, want %v", err, wantErr)
	}
	select {
	case <-input.closed:
	case <-time.After(time.Second):
		t.Fatal("downstream failure did not close the guarded input")
	}
}

func einoAudioGuardInput() *genx.StreamBuilder {
	return genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
}

func einoNextChunk(t testing.TB, stream genx.Stream) *genx.MessageChunk {
	t.Helper()
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		chunk, err := stream.Next()
		resultCh <- result{chunk: chunk, err: err}
	}()
	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("Next() error = %v", got.err)
		}
		return got.chunk
	case <-time.After(time.Second):
		t.Fatal("Next() did not return promptly")
		return nil
	}
}

func einoCollectChunks(t testing.TB, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			return chunks
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

func assertEinoAudioUnsupportedTerminal(t testing.TB, chunk *genx.MessageChunk, streamID string) {
	t.Helper()
	if chunk == nil || chunk.Role != genx.RoleModel || chunk.Name != "assistant" || chunk.Ctrl == nil {
		t.Fatalf("terminal = %#v", chunk)
	}
	text, ok := chunk.Part.(genx.Text)
	if !ok || text != "" || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != "assistant" ||
		!chunk.Ctrl.EndOfStream || chunk.Ctrl.ErrorCode != einoAudioInputUnsupportedCode ||
		chunk.Ctrl.Error != einoAudioInputUnsupportedMessage || chunk.Ctrl.ErrorRetryable ||
		chunk.Ctrl.FailureClass != genx.FailureClassTransform {
		t.Fatalf("terminal = %#v", chunk)
	}
}

type einoAudioGuardSpy struct {
	mu     sync.Mutex
	chunks []*genx.MessageChunk
}

type einoAudioGuardBlockingStream struct {
	closed    chan struct{}
	closeOnce sync.Once
}

func (s *einoAudioGuardBlockingStream) Next() (*genx.MessageChunk, error) {
	<-s.closed
	return nil, io.ErrClosedPipe
}

func (s *einoAudioGuardBlockingStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *einoAudioGuardBlockingStream) CloseWithError(error) error {
	return s.Close()
}

type einoAudioGuardErrorStream struct {
	err error
}

func (s einoAudioGuardErrorStream) Next() (*genx.MessageChunk, error) { return nil, s.err }
func (einoAudioGuardErrorStream) Close() error                        { return nil }
func (einoAudioGuardErrorStream) CloseWithError(error) error          { return nil }

func (s *einoAudioGuardSpy) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	go func() {
		defer input.Close()
		for {
			chunk, err := input.Next()
			if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
				_ = output.Done(genx.Usage{})
				return
			}
			if err != nil {
				_ = output.Abort(err)
				return
			}
			s.mu.Lock()
			s.chunks = append(s.chunks, chunk.Clone())
			s.mu.Unlock()
			if err := output.Add(chunk.Clone()); err != nil {
				return
			}
		}
	}()
	return output.Stream(), nil
}

func (s *einoAudioGuardSpy) snapshot() []*genx.MessageChunk {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*genx.MessageChunk, 0, len(s.chunks))
	for _, chunk := range s.chunks {
		result = append(result, chunk.Clone())
	}
	return result
}

func einoWorkspaceParameters(t testing.TB, mode apitypes.WorkspaceInputMode) *apitypes.WorkspaceParameters {
	t.Helper()
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromEinoWorkspaceParameters(apitypes.EinoWorkspaceParameters{
		AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino,
		Input:     &mode,
	}); err != nil {
		t.Fatal(err)
	}
	return &parameters
}

func TestFactoryBindsOnlyWorkspaceAppAndReportsConfiguredBackend(t *testing.T) {
	t.Parallel()
	store := &einoMemoryStore{}
	spec := einoFactorySpec(t)
	owner := "owner-public-key"
	spec.Workspace.OwnerPublicKey = &owner
	spec.Memory = store
	spec.MemoryKind = "volc_mem0"
	service := &peergenx.Service{}
	agent, err := (Factory{
		GenX: service,
		GenXForOwner: func(context.Context, string) (*peergenx.Service, error) {
			return service, nil
		},
	}).NewAgent(t.Context(), spec)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	response, err := agent.Recall(t.Context(), apitypes.PeerRunRecallRequest{Query: "remember"})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(response.Hits) != 1 || response.Hits[0].Snippet != "remembered" {
		t.Fatalf("Recall() = %#v", response)
	}
	store.mu.Lock()
	scope := store.query.Scope
	store.mu.Unlock()
	if scope.AppID != spec.Workspace.Id || scope.AgentID != "" {
		t.Fatalf("Recall scope = %#v, want only Workspace AppID", scope)
	}
	if scope.UserID != "" || scope.RunID != "" {
		t.Fatalf("Recall scope rewrote inner dimensions: %#v", scope)
	}
	stats, err := agent.MemoryStats(t.Context(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if stats.Backend == nil || *stats.Backend != "volc_mem0" {
		t.Fatalf("MemoryStats backend = %v", stats.Backend)
	}
	if stats.Metadata == nil {
		t.Fatal("MemoryStats metadata is nil")
	}
	metadataScope, ok := (*stats.Metadata)["scope"].(map[string]any)
	if !ok || metadataScope["app_id"] != spec.Workspace.Id ||
		metadataScope["agent_id"] != "" {
		t.Fatalf("MemoryStats scope = %#v", (*stats.Metadata)["scope"])
	}
}

func addEinoMemoryRecallNode(t testing.TB, spec *apitypes.EinoWorkflowSpec) {
	t.Helper()
	var node apitypes.EinoNode
	if err := node.FromEinoMemoryRecallNode(apitypes.EinoMemoryRecallNode{
		Id:        "recall",
		Type:      apitypes.EinoMemoryRecallNodeTypeMemoryRecall,
		QueryFrom: "input.text",
		Output:    "answer",
		TopK:      5,
	}); err != nil {
		t.Fatal(err)
	}
	spec.Graph.Nodes = append(spec.Graph.Nodes, node)
	spec.Graph.Edges = []apitypes.EinoEdge{
		{From: "start", To: "recall"},
		{From: "recall", To: "answer"},
		{From: "answer", To: "end"},
	}
}

func einoFactorySpec(t testing.TB) agenthost.Spec {
	t.Helper()
	var public apitypes.EinoWorkflowSpec
	if err := json.Unmarshal([]byte(`{
		"graph": {
			"name": "factory-test",
			"compile": {"node_trigger_mode": "any_predecessor"},
			"state": {"fields": [{"name": "answer", "type": "string", "merge": "replace"}]},
			"nodes": [{
				"id": "answer",
				"type": "passthrough",
				"inputs": {"value": {"from": "input.text"}},
				"outputs": {"value": "answer"}
			}],
			"edges": [{"from": "start", "to": "answer"}, {"from": "answer", "to": "end"}],
			"branches": [],
			"outputs": [{
				"node": "answer",
				"field": "answer",
				"name": "assistant",
				"mime_type": "text/plain",
				"primary": true
			}]
		}
	}`), &public); err != nil {
		t.Fatalf("decode Eino factory fixture: %v", err)
	}
	return agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-id-a", Name: "workspace-a"},
		Workflow: apitypes.Workflow{
			Id: "workflow-a",
			Spec: apitypes.WorkflowSpec{
				Driver: apitypes.WorkflowDriverEino,
				Eino:   &public,
			},
		},
	}
}

type einoMemoryStore struct {
	mu    sync.Mutex
	query memory.Query
}

type einoTestTransformer func(context.Context, genx.Stream) (genx.Stream, error)

func (f einoTestTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return f(ctx, input)
}

type einoTTSBuilder struct {
	calls *atomic.Int32
}

func (einoTTSBuilder) BuildGenerator(context.Context, peergenx.GeneratorConfig) (genx.Generator, error) {
	return nil, errors.New("unexpected generator build")
}

func (b einoTTSBuilder) BuildTransformer(context.Context, peergenx.TransformerConfig) (genx.Transformer, error) {
	return einoTestTransformer(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
		b.calls.Add(1)
		return input, nil
	}), nil
}

type einoTTSResources struct{}

func (einoTTSResources) GetVoice(_ context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	return adminhttp.GetVoice200JSONResponse(apitypes.Voice{
		Id: request.Id,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindVolcTenant,
			Id:   "volc-main",
		},
	}), nil
}

func (einoTTSResources) GetCredential(_ context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	return adminhttp.GetCredential200JSONResponse(apitypes.Credential{Id: request.Id}), nil
}

func (einoTTSResources) GetVolcTenant(_ context.Context, request adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	return adminhttp.GetVolcTenant200JSONResponse(apitypes.VolcTenant{Id: request.Id, CredentialId: "voice-credential"}), nil
}

func (einoTTSResources) GetDeepSeekTenant(context.Context, adminhttp.GetDeepSeekTenantRequestObject) (adminhttp.GetDeepSeekTenantResponseObject, error) {
	return nil, errors.New("unexpected DeepSeek tenant lookup")
}

func (einoTTSResources) GetOpenAITenant(context.Context, adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	return nil, errors.New("unexpected OpenAI tenant lookup")
}

func (einoTTSResources) GetGeminiTenant(context.Context, adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	return nil, errors.New("unexpected Gemini tenant lookup")
}

func (einoTTSResources) GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	return nil, errors.New("unexpected DashScope tenant lookup")
}

func (einoTTSResources) GetMiniMaxTenant(context.Context, adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	return nil, errors.New("unexpected MiniMax tenant lookup")
}

type einoTestMux func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f einoTestMux) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return f(ctx, pattern, input)
}

func (*einoMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func (s *einoMemoryStore) Recall(_ context.Context, query memory.Query) (memory.RecallResult, error) {
	s.mu.Lock()
	s.query = query
	s.mu.Unlock()
	return memory.RecallResult{Matches: []memory.Match{{
		Fact:  memory.Fact{ID: "fact-a", Text: "remembered"},
		Score: 1,
	}}}, nil
}

func (*einoMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, errors.New("unexpected Update")
}

func (*einoMemoryStore) Delete(context.Context, memory.DeleteRequest) error {
	return errors.New("unexpected Delete")
}
