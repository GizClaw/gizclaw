package flowcraft

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	flowclaw "github.com/GizClaw/flowcraft/sdkx/claw"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/agenthost"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestAgentRunTurnBridgesASRClawAndTTS(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		claw: fakeClaw{events: []flowclaw.Event{
			{Type: flowclaw.EventToken, NodeID: "answer", Content: "好的"},
			{Type: flowclaw.EventToken, NodeID: "answer", Content: "呀"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	input := &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	if err := a.runTurn(context.Background(), input, output); err != nil {
		t.Fatalf("runTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	got := drainChunks(t, output.Stream())

	want := []struct {
		role  genx.Role
		name  string
		label string
		text  string
		blob  []byte
		eos   bool
	}{
		{role: genx.RoleUser, label: transcriptLabel, text: "你好"},
		{role: genx.RoleUser, label: transcriptLabel, text: "", eos: true},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, text: "好的"},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, text: "呀"},
		{role: genx.RoleModel, name: assistantLabel, label: assistantLabel, text: "", eos: true},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, blob: []byte{0xaa}},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, blob: nil, eos: true},
	}
	if len(got) != len(want) {
		t.Fatalf("chunks len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i, want := range want {
		chunk := got[i]
		if chunk.Role != want.role || chunk.Ctrl == nil || chunk.Ctrl.Label != want.label || chunk.Ctrl.StreamID != "audio" || chunk.Ctrl.EndOfStream != want.eos {
			t.Fatalf("chunk[%d] ctrl = %#v role=%s, want role=%s label=%s eos=%t", i, chunk.Ctrl, chunk.Role, want.role, want.label, want.eos)
		}
		if want.name != "" && chunk.Name != want.name {
			t.Fatalf("chunk[%d] name = %q, want %q", i, chunk.Name, want.name)
		}
		if want.text != "" || i == 1 || i == 4 {
			text, _ := chunk.Part.(genx.Text)
			if string(text) != want.text {
				t.Fatalf("chunk[%d] text = %q, want %q", i, text, want.text)
			}
		}
		if want.blob != nil || i == 6 {
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || blob.MIMEType != "audio/opus" || !reflect.DeepEqual(blob.Data, want.blob) {
				t.Fatalf("chunk[%d] blob = %#v", i, chunk.Part)
			}
		}
	}
}

func TestAgentRunTurnForksAssistantTextToTTSInput(t *testing.T) {
	transformer := &recordingVoiceTransformer{}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: transformer},
		claw: fakeClaw{events: []flowclaw.Event{
			{Type: flowclaw.EventToken, NodeID: "answer", Content: "好的"},
			{Type: flowclaw.EventToken, NodeID: "answer", Content: "呀"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	if err := a.runTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, output); err != nil {
		t.Fatalf("runTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())

	var assistantText []string
	firstAudio := -1
	for i, chunk := range chunks {
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/opus" && len(blob.Data) > 0 && firstAudio < 0 {
			firstAudio = i
		}
		if chunk.Ctrl == nil || chunk.Ctrl.Label != assistantLabel || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			assistantText = append(assistantText, string(text))
		}
	}
	if !reflect.DeepEqual(assistantText, []string{"好的", "呀"}) {
		t.Fatalf("assistant text chunks = %#v", assistantText)
	}
	if firstAudio >= 0 {
		textDoneIndex := -1
		for i, chunk := range chunks {
			if chunk.Ctrl != nil && chunk.Ctrl.Label == assistantLabel && chunk.IsEndOfStream() {
				if _, ok := chunk.Part.(genx.Text); ok {
					textDoneIndex = i
					break
				}
			}
		}
		if textDoneIndex < 0 || firstAudio < textDoneIndex {
			t.Fatalf("audio arrived before assistant text done: firstAudio=%d textDone=%d chunks=%#v", firstAudio, textDoneIndex, chunks)
		}
	}
	if got := transformer.Texts(); !reflect.DeepEqual(got, []string{"好的", "呀"}) {
		t.Fatalf("TTS input texts = %#v, want copied assistant tokens", got)
	}
}

func TestAgentTransformRunsTurnAndClosesOutput(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		claw: fakeClaw{events: []flowclaw.Event{
			{Type: flowclaw.EventToken, NodeID: "answer", Content: "收到"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	stream, err := a.Transform(context.Background(), "ignored", &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainChunks(t, stream)
	if len(chunks) == 0 {
		t.Fatal("Transform() produced no chunks")
	}
}

func TestFactoryNewAgentWritesClawConfig(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{"generate_model": "chat"}
	workflow := apitypes.WorkflowDocument{
		Spec: apitypes.FlowcraftWorkflowSpec{
			"flowcraft": map[string]any{
				"history": map[string]any{"enabled": false},
				"memory":  map[string]any{"enabled": false},
			},
			"voice_adapter": map[string]any{
				"asr_model":     "asr",
				"default_voice": "voice",
			},
		},
	}
	transformer, err := (Factory{GenX: service}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
		Workflow:  workflow,
		Runtime:   agenthost.WorkspaceRuntime{LocalDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	t.Cleanup(func() {
		if a, ok := transformer.(*agent); ok && a.claw != nil {
			_ = a.claw.CloseContext(context.Background())
		}
	})
	if transformer == nil {
		t.Fatal("NewAgent() agent = nil")
	}
}

func TestFactoryNewAgentFailsClosedOnDeniedVoice(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      denyResourceAuthorizer{kind: apitypes.ACLResourceKindVoice, id: "voice"},
		Models:          fakeModels{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{"generate_model": "chat"}
	workflow := apitypes.WorkflowDocument{
		Spec: apitypes.FlowcraftWorkflowSpec{
			"flowcraft": map[string]any{},
			"voice_adapter": map[string]any{
				"asr_model":     "asr",
				"default_voice": "voice",
			},
		},
	}
	_, err := (Factory{GenX: service}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
		Workflow:  workflow,
		Runtime:   agenthost.WorkspaceRuntime{LocalDir: t.TempDir()},
	})
	if err == nil || !errors.Is(err, peergenx.ErrDenied) || !strings.Contains(err.Error(), `resolve voice "voice"`) {
		t.Fatalf("NewAgent() error = %v, want denied voice", err)
	}
}

func TestParseWorkflowConfigTrimsNodeVoices(t *testing.T) {
	cfg, err := parseWorkflowConfig(agenthost.Spec{Workflow: apitypes.WorkflowDocument{
		Spec: apitypes.FlowcraftWorkflowSpec{
			"voice_adapter": map[string]any{
				"asr_model":     " asr ",
				"default_voice": " voice ",
				"node_voices": map[string]any{
					" answer ": " voice-a ",
					"":         "ignored",
					"empty":    " ",
				},
			},
		},
	}})
	if err != nil {
		t.Fatalf("parseWorkflowConfig() error = %v", err)
	}
	if cfg.Spec.VoiceAdapter.ASRModel != "asr" || cfg.Spec.VoiceAdapter.DefaultVoice != "voice" {
		t.Fatalf("voice adapter = %#v", cfg.Spec.VoiceAdapter)
	}
	if !reflect.DeepEqual(cfg.Spec.VoiceAdapter.NodeVoices, map[string]string{"answer": "voice-a"}) {
		t.Fatalf("node voices = %#v", cfg.Spec.VoiceAdapter.NodeVoices)
	}
}

func TestRealClawCloseContextHandlesNil(t *testing.T) {
	if err := (realClaw{}).CloseContext(context.Background()); err != nil {
		t.Fatalf("CloseContext(nil) error = %v", err)
	}
}

func TestFactoryValidation(t *testing.T) {
	if _, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "peergenx") {
		t.Fatalf("NewAgent(missing genx) error = %v", err)
	}
	if _, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(context.Background(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "asr_model") {
		t.Fatalf("NewAgent(missing workflow config) error = %v", err)
	}
}

func TestSynthesizeConvertsOggOpusToPeerOpusChunks(t *testing.T) {
	raw := buildOggPackets(t, opusHeadPacket(16000, 1), opusTagsPacket("test"), []byte{0x21}, []byte{0x22})
	a := &agent{
		transformers: fakeTransformerProvider{transformer: patternTransformer{
			pattern: "voice/voice",
			stream: &sliceStream{chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/ogg", Data: raw}},
			}},
		}},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	if err := a.synthesize(context.Background(), "audio", "answer", "voice", "hello", output); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 3 {
		t.Fatalf("chunks len = %d, want 3", len(chunks))
	}
	for i, want := range [][]byte{{0x21}, {0x22}, nil} {
		blob := chunks[i].Part.(*genx.Blob)
		if blob.MIMEType != "audio/opus" || !reflect.DeepEqual(blob.Data, want) {
			t.Fatalf("chunk[%d] blob = %#v, want %v", i, blob, want)
		}
	}
	if !chunks[2].IsEndOfStream() {
		t.Fatal("last chunk is not EOS")
	}
}

func TestSynthesizeRejectsUnsupportedAudioMIME(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: patternTransformer{
			pattern: "voice/voice",
			stream: &sliceStream{chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/mpeg", Data: []byte{1}}},
			}},
		}},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	err := a.synthesize(context.Background(), "audio", "answer", "voice", "hello", output)
	if err == nil || !strings.Contains(err.Error(), "unsupported TTS audio MIME") {
		t.Fatalf("synthesize() error = %v", err)
	}
}

func TestRunTurnReturnsClawEventError(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		claw: fakeClaw{events: []flowclaw.Event{
			{Type: flowclaw.EventError, Err: "boom", IsError: true},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	err := a.runTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, output)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runTurn() error = %v", err)
	}
}

func TestTranscribeInputTurnHandlesFinalTextDone(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: patternTransformer{
			pattern: "model/asr",
			stream: &sliceStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("你好"), Ctrl: &genx.StreamCtrl{EndOfStream: true}},
			}},
		}},
		asrModel: "asr",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	transcript, streamID, err := a.transcribeInputTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, output)
	if err != nil {
		t.Fatalf("transcribeInputTurn() error = %v", err)
	}
	if transcript != "你好" || streamID != "audio" {
		t.Fatalf("transcript=%q streamID=%q", transcript, streamID)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 2 {
		t.Fatalf("chunks len = %d, want 2", len(chunks))
	}
	if string(chunks[0].Part.(genx.Text)) != "你好" || chunks[0].IsEndOfStream() {
		t.Fatalf("text chunk = %#v", chunks[0])
	}
	if !chunks[1].IsEndOfStream() {
		t.Fatalf("eos chunk = %#v", chunks[1])
	}
}

func TestTranscribeInputTurnStartASRError(t *testing.T) {
	want := errors.New("start failed")
	a := &agent{
		transformers: fakeTransformerProvider{transformer: errorTransformer{err: want}},
		asrModel:     "asr",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	_, _, err := a.transcribeInputTurn(context.Background(), &sliceStream{}, output)
	if err == nil || !strings.Contains(err.Error(), "start ASR") || !errors.Is(err, want) {
		t.Fatalf("transcribeInputTurn() error = %v", err)
	}
}

func TestFeedASRInputReturnsInputError(t *testing.T) {
	want := errors.New("input failed")
	input := &sliceStream{err: want}
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	result := feedASRInput(context.Background(), input, asrInput, &lockedString{value: defaultInputStreamID})
	if !errors.Is(result.err, want) {
		t.Fatalf("feedASRInput() error = %v, want %v", result.err, want)
	}
	if _, err := asrInput.Stream().Next(); err == nil || !strings.Contains(err.Error(), "input failed") {
		t.Fatalf("asr input stream error = %v", err)
	}
}

func TestFeedASRInputStreamsRawOpusUnchanged(t *testing.T) {
	packet := []byte{0x11, 0x22, 0x33}
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	state := &lockedString{value: defaultInputStreamID}
	result := feedASRInput(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, asrInput, state)
	if result.err != nil {
		t.Fatalf("feedASRInput() error = %v", result.err)
	}
	if result.streamID != "audio" {
		t.Fatalf("streamID = %q, want audio", result.streamID)
	}

	stream := asrInput.Stream()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next data chunk error = %v", err)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || !reflect.DeepEqual(blob.Data, packet) || chunk.IsEndOfStream() {
		t.Fatalf("data chunk = %#v", chunk)
	}
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next eos chunk error = %v", err)
	}
	blob, ok = chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || len(blob.Data) != 0 || !chunk.IsEndOfStream() {
		t.Fatalf("eos chunk = %#v", chunk)
	}
	if _, err := stream.Next(); !agenthost.IsStreamDone(err) {
		t.Fatalf("final Next() error = %v, want done", err)
	}
}

func TestLockedString(t *testing.T) {
	value := &lockedString{value: "a"}
	if got := value.Get(); got != "a" {
		t.Fatalf("initial value = %q", got)
	}
	value.Set("b")
	if got := value.Get(); got != "b" {
		t.Fatalf("updated value = %q", got)
	}
}

func TestSliceStreamCloseWithError(t *testing.T) {
	stream := &sliceStream{chunks: []*genx.MessageChunk{{}}}
	want := errors.New("closed")
	if err := stream.CloseWithError(want); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	if _, err := stream.Next(); !errors.Is(err, want) {
		t.Fatalf("Next() error = %v, want %v", err, want)
	}
}

func TestBuildClawConfigInjectsPeerResolvedOpenAIModel(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{"generate_model": "chat"}
	cfg := workflowConfig{}
	cfg.Spec.Flowcraft = map[string]any{
		"history": map[string]any{"enabled": false},
		"agent":   map[string]any{"system_prompt": "short"},
	}
	got, err := buildClawConfig(ctx, service, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
	}, cfg)
	if err != nil {
		t.Fatalf("buildClawConfig() error = %v", err)
	}

	wantPrefix := []string{
		"list:models",
		"auth:model:chat:model.read",
		"auth:model:chat:model.use",
		"get:tenant:openai:main",
		"auth:credential:openai-key:credential.read",
		"auth:credential:openai-key:credential.use",
		"get:credential:openai-key",
	}
	if len(events) < len(wantPrefix) || !reflect.DeepEqual(events[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("events prefix = %#v, want %#v; all events = %#v", events[:min(len(events), len(wantPrefix))], wantPrefix, events)
	}
	settings := got["settings"].(map[string]any)
	if settings["generate_model"] != "chat" {
		t.Fatalf("settings.generate_model = %v", settings["generate_model"])
	}
	models := got["models"].(map[string]any)
	if models["chat"] != "generate_model" {
		t.Fatalf("models.chat = %v", models["chat"])
	}
	llm := models["llm"].(map[string]any)
	model := llm["chat"].(map[string]any)
	if model["provider"] != "openai" || model["model"] != "gpt-test" || model["api_key"] != "test-key" || model["base_url"] != "https://llm.example/v1" {
		t.Fatalf("llm chat config = %#v", model)
	}
	modelConfig := model["config"].(map[string]any)
	thinking := modelConfig["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("llm chat thinking config = %#v", modelConfig)
	}
	agent := got["agent"].(map[string]any)
	if agent["system_prompt"] != "short" || agent["model"] != "generate_model" {
		t.Fatalf("agent config = %#v", agent)
	}
}

func TestBuildClawConfigMapsVolcTenantLLMToBytedance(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events, providerKind: apitypes.ModelProviderKindVolcTenant},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{"generate_model": "chat"}
	got, err := buildClawConfig(ctx, service, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
	}, workflowConfig{})
	if err != nil {
		t.Fatalf("buildClawConfig() error = %v", err)
	}
	model := got["models"].(map[string]any)["llm"].(map[string]any)["chat"].(map[string]any)
	if model["provider"] != "bytedance" || model["model"] != "doubao-lite" || model["api_key"] != "test-key" || model["region"] != "cn-beijing" {
		t.Fatalf("llm chat config = %#v", model)
	}
	modelConfig := model["config"].(map[string]any)
	thinking := modelConfig["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("llm chat thinking config = %#v", modelConfig)
	}
}

func TestBuildClawConfigInjectsOptionalModelRoles(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{
		"generate_model": "chat",
		"extract_model":  "extract",
	}
	got, err := buildClawConfig(ctx, service, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
	}, workflowConfig{})
	if err != nil {
		t.Fatalf("buildClawConfig() error = %v", err)
	}
	settings := got["settings"].(map[string]any)
	if settings["generate_model"] != "chat" || settings["extract_model"] != "extract" {
		t.Fatalf("settings = %#v", settings)
	}
	models := got["models"].(map[string]any)
	if models["chat"] != "generate_model" || models["extractor"] != "extract_model" {
		t.Fatalf("models aliases = %#v", models)
	}
	llm := models["llm"].(map[string]any)
	if _, ok := llm["chat"]; !ok {
		t.Fatalf("missing chat llm config: %#v", llm)
	}
	if _, ok := llm["extract"]; !ok {
		t.Fatalf("missing extract llm config: %#v", llm)
	}
}

func TestBuildClawConfigRejectsUnsupportedProvider(t *testing.T) {
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      recordingAuthorizer{events: &events},
		Models:          fakeModels{events: &events, providerKind: apitypes.ModelProviderKindGeminiTenant},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{"generate_model": "chat"}
	_, err := buildClawConfig(context.Background(), service, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
	}, workflowConfig{})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("buildClawConfig() error = %v", err)
	}
}

func drainChunks(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) {
				return chunks
			}
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

type fakeTransformerProvider struct {
	transformer genx.Transformer
}

func (p fakeTransformerProvider) Transformer() genx.Transformer {
	return p.transformer
}

type fakeVoiceTransformer struct{}

func (fakeVoiceTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	switch pattern {
	case "model/asr":
		return &sliceStream{chunks: []*genx.MessageChunk{{Part: genx.Text("你好")}}}, nil
	case "voice/voice-answer":
		return &sliceStream{chunks: []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xaa}}},
			{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}},
		}}, nil
	default:
		return nil, errors.New("unexpected pattern: " + pattern)
	}
}

type recordingVoiceTransformer struct {
	mu    sync.Mutex
	texts []string
}

func (t *recordingVoiceTransformer) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	switch pattern {
	case "model/asr":
		return &sliceStream{chunks: []*genx.MessageChunk{{Part: genx.Text("你好")}}}, nil
	case "voice/voice-answer":
		return &recordingVoiceStream{
			input: input,
			owner: t,
			chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xaa}}},
				{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}},
			},
		}, nil
	default:
		return nil, errors.New("unexpected pattern: " + pattern)
	}
}

func (t *recordingVoiceTransformer) Texts() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.texts...)
}

func (t *recordingVoiceTransformer) appendText(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.texts = append(t.texts, text)
}

type recordingVoiceStream struct {
	input  genx.Stream
	owner  *recordingVoiceTransformer
	chunks []*genx.MessageChunk
	once   sync.Once
	err    error
}

func (s *recordingVoiceStream) Next() (*genx.MessageChunk, error) {
	s.once.Do(func() {
		for {
			chunk, err := s.input.Next()
			if err != nil {
				if agenthost.IsStreamDone(err) {
					return
				}
				s.err = err
				return
			}
			if chunk == nil || chunk.IsEndOfStream() {
				continue
			}
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				s.owner.appendText(string(text))
			}
		}
	})
	if s.err != nil {
		return nil, s.err
	}
	if len(s.chunks) == 0 {
		return nil, genx.Done(genx.Usage{})
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *recordingVoiceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *recordingVoiceStream) CloseWithError(err error) error {
	s.err = err
	s.chunks = nil
	return nil
}

type patternTransformer struct {
	pattern string
	stream  genx.Stream
}

func (t patternTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	if pattern != t.pattern {
		return nil, errors.New("unexpected pattern: " + pattern)
	}
	return t.stream, nil
}

type errorTransformer struct {
	err error
}

func (t errorTransformer) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return nil, t.err
}

type fakeClaw struct {
	events []flowclaw.Event
	text   string
}

func (c fakeClaw) RoundTrip(req flowclaw.Request) (clawResponse, error) {
	c.text = req.Text
	return &fakeClawResponse{events: append([]flowclaw.Event(nil), c.events...)}, nil
}

func (fakeClaw) CloseContext(context.Context) error { return nil }

type fakeClawResponse struct {
	events []flowclaw.Event
}

func (r *fakeClawResponse) Next() (flowclaw.Event, error) {
	if len(r.events) == 0 {
		return flowclaw.Event{}, io.EOF
	}
	ev := r.events[0]
	r.events = r.events[1:]
	return ev, nil
}

func TestBuildClawConfigRequiresGenerateModel(t *testing.T) {
	_, err := buildClawConfig(context.Background(), peergenx.New(peergenx.Service{}), agenthost.Spec{}, workflowConfig{})
	if err == nil || !strings.Contains(err.Error(), "generate_model") {
		t.Fatalf("buildClawConfig() error = %v, want generate_model", err)
	}
}

func TestBuildClawConfigFailsClosedOnDeniedModel(t *testing.T) {
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Authorizer:      denyAuthorizer{},
		Models:          fakeModels{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	params := map[string]any{"generate_model": "chat"}
	_, err := buildClawConfig(context.Background(), service, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: testFlowcraftWorkspaceParameters(params)},
	}, workflowConfig{})
	if err == nil || !strings.Contains(err.Error(), "not accessible as a generator") {
		t.Fatalf("buildClawConfig() error = %v, want inaccessible model", err)
	}
	if !reflect.DeepEqual(events, []string{"list:models"}) {
		t.Fatalf("model events after ACL denial = %#v, want list only", events)
	}
}

func TestMergeTranscriptHandlesFullAndDeltaResults(t *testing.T) {
	text := mergeTranscript("", "你好")
	text = mergeTranscript(text, "你好世界")
	text = mergeTranscript(text, "！")
	if text != "你好世界！" {
		t.Fatalf("merged transcript = %q", text)
	}
	if got := mergeTranscript("你好世界", "你好"); got != "你好世界" {
		t.Fatalf("prefix merge = %q", got)
	}
}

func TestOpusFramesFromOggSkipsHeaders(t *testing.T) {
	frameA := []byte{0x11, 0x22}
	frameB := []byte{0x33, 0x44}
	raw := buildOggPackets(t, opusHeadPacket(16000, 1), opusTagsPacket("test"), frameA, frameB)
	frames, err := opusFramesFromOgg(raw)
	if err != nil {
		t.Fatalf("opusFramesFromOgg() error = %v", err)
	}
	if !reflect.DeepEqual(frames, [][]byte{frameA, frameB}) {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestWorkflowConfigValidation(t *testing.T) {
	var cfg workflowConfig
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "asr_model") {
		t.Fatalf("validate missing ASR = %v", err)
	}
	cfg.Spec.VoiceAdapter.ASRModel = "asr"
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "default_voice") {
		t.Fatalf("validate missing voice = %v", err)
	}
	cfg.Spec.VoiceAdapter.DefaultVoice = "voice"
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
}

func TestVoiceForNodeFallsBackToDefault(t *testing.T) {
	a := &agent{defaultVoice: "default", nodeVoices: map[string]string{"answer": "answer-voice"}}
	if got := a.voiceForNode("answer"); got != "answer-voice" {
		t.Fatalf("answer voice = %q", got)
	}
	if got := a.voiceForNode("other"); got != "default" {
		t.Fatalf("fallback voice = %q", got)
	}
}

func TestConfigHelperBranches(t *testing.T) {
	if got := openAIThinkingConfigValue("enable_thinking", "off"); got != false {
		t.Fatalf("enable_thinking off = %#v, want false", got)
	}
	if got := openAIThinkingConfigValue("thinking.type", "disabled"); got != "disabled" {
		t.Fatalf("thinking.type disabled = %#v", got)
	}
	for _, value := range []string{"disabled", "disable", "off", "false", "0", "none", "no"} {
		if !isDisabledThinkingLevel(value) {
			t.Fatalf("isDisabledThinkingLevel(%q) = false, want true", value)
		}
	}
	if isDisabledThinkingLevel("auto") {
		t.Fatal("isDisabledThinkingLevel(auto) = true, want false")
	}
	if got := mapString(map[string]any{"empty": " ", "name": "  value  "}, "empty", "name"); got != "value" {
		t.Fatalf("mapString() = %q, want value", got)
	}
	if got := firstString(nil, ptrString(" "), ptrString("x")); got != "x" {
		t.Fatalf("firstString() = %q, want x", got)
	}
}

func ptrString(value string) *string {
	return &value
}

func opusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	return packet
}

func opusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	return packet
}

func buildOggPackets(t *testing.T, packets ...[]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	sw, err := ogg.NewStreamWriter(&out, 77)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	for i, packet := range packets {
		if _, err := sw.WritePacket(packet, uint64(i), i == len(packets)-1); err != nil {
			t.Fatalf("WritePacket %d: %v", i, err)
		}
	}
	return out.Bytes()
}

type testPeer struct{}

func (testPeer) PublicKey() giznet.PublicKey {
	var key giznet.PublicKey
	key[0] = 1
	return key
}

type recordingAuthorizer struct {
	events *[]string
}

func (a recordingAuthorizer) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	*a.events = append(*a.events, "auth:"+string(request.Resource.Kind)+":"+request.Resource.Id+":"+string(request.Permission))
	return nil
}

type denyAuthorizer struct{}

func (denyAuthorizer) Authorize(context.Context, acl.AuthorizeRequest) error {
	return acl.ErrDenied
}

type denyResourceAuthorizer struct {
	kind apitypes.ACLResourceKind
	id   string
}

func (a denyResourceAuthorizer) Authorize(_ context.Context, request acl.AuthorizeRequest) error {
	if request.Resource.Kind == a.kind && request.Resource.Id == a.id {
		return acl.ErrDenied
	}
	return nil
}

type fakeModels struct {
	events       *[]string
	providerKind apitypes.ModelProviderKind
}

func appendEvent(events *[]string, event string) {
	if events != nil {
		*events = append(*events, event)
	}
}

func (f fakeModels) GetModel(_ context.Context, request adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error) {
	appendEvent(f.events, "get:model:"+request.Id)
	return adminservice.GetModel200JSONResponse(f.model(request.Id)), nil
}

func (f fakeModels) ListModels(_ context.Context, request adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
	appendEvent(f.events, "list:models")
	if request.Params.Cursor != nil && *request.Params.Cursor != "" {
		return adminservice.ListModels200JSONResponse(adminservice.ModelList{}), nil
	}
	return adminservice.ListModels200JSONResponse(adminservice.ModelList{
		Items: []apitypes.Model{
			f.model("chat"),
			f.model("extract"),
			f.model("embed"),
		},
	}), nil
}

func (f fakeModels) model(id string) apitypes.Model {
	providerKind := f.providerKind
	if providerKind == "" {
		providerKind = apitypes.ModelProviderKindOpenaiTenant
	}
	kind := apitypes.ModelKindLlm
	if id == "asr" || id == "e2e-asr" {
		kind = apitypes.ModelKindAsr
		providerKind = apitypes.ModelProviderKindVolcTenant
	}
	openAIUpstream := "gpt-test"
	volcUpstream := "doubao-lite"
	thinkingParam := "thinking.type"
	thinkingLevel := "disabled"
	var providerData *apitypes.ModelProviderData
	switch providerKind {
	case apitypes.ModelProviderKindVolcTenant:
		providerData = testVolcModelProviderData(apitypes.VolcTenantModelProviderData{
			UpstreamModel:        &volcUpstream,
			ThinkingParam:        &thinkingParam,
			DefaultThinkingLevel: &thinkingLevel,
		})
	default:
		providerData = testOpenAIModelProviderData(apitypes.OpenAITenantModelProviderData{
			UpstreamModel:        &openAIUpstream,
			ThinkingParam:        &thinkingParam,
			DefaultThinkingLevel: &thinkingLevel,
		})
	}
	return apitypes.Model{
		Id:   id,
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: providerKind,
			Name: "main",
		},
		ProviderData: providerData,
	}
}

type fakeVoices struct {
	events *[]string
}

func (f fakeVoices) GetVoice(_ context.Context, request adminservice.GetVoiceRequestObject) (adminservice.GetVoiceResponseObject, error) {
	*f.events = append(*f.events, "get:voice:"+request.Id)
	return adminservice.GetVoice200JSONResponse(apitypes.Voice{
		Id: request.Id,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindVolcTenant,
			Name: "main",
		},
	}), nil
}

type fakeCredentials struct {
	events *[]string
}

func (f fakeCredentials) GetCredential(_ context.Context, request adminservice.GetCredentialRequestObject) (adminservice.GetCredentialResponseObject, error) {
	*f.events = append(*f.events, "get:credential:"+request.Name)
	body := testOpenAICredentialBody("test-key")
	if strings.Contains(request.Name, "volc") {
		body = testVolcCredentialBody("test-key")
	}
	return adminservice.GetCredential200JSONResponse(apitypes.Credential{
		Name: request.Name,
		Body: body,
	}), nil
}

type fakeTenants struct {
	events *[]string
}

func (f fakeTenants) GetOpenAITenant(_ context.Context, request adminservice.GetOpenAITenantRequestObject) (adminservice.GetOpenAITenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:openai:"+request.Name)
	baseURL := "https://llm.example/v1"
	return adminservice.GetOpenAITenant200JSONResponse(apitypes.OpenAITenant{
		Name:           request.Name,
		CredentialName: "openai-key",
		BaseUrl:        &baseURL,
	}), nil
}

func (f fakeTenants) GetGeminiTenant(_ context.Context, request adminservice.GetGeminiTenantRequestObject) (adminservice.GetGeminiTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:gemini:"+request.Name)
	return adminservice.GetGeminiTenant200JSONResponse(apitypes.GeminiTenant{
		Name:           request.Name,
		CredentialName: "gemini-key",
	}), nil
}

func (f fakeTenants) GetDashScopeTenant(context.Context, adminservice.GetDashScopeTenantRequestObject) (adminservice.GetDashScopeTenantResponseObject, error) {
	panic("unexpected dashscope tenant lookup")
}

func (f fakeTenants) GetMiniMaxTenant(context.Context, adminservice.GetMiniMaxTenantRequestObject) (adminservice.GetMiniMaxTenantResponseObject, error) {
	panic("unexpected minimax tenant lookup")
}

func (f fakeTenants) GetVolcTenant(_ context.Context, request adminservice.GetVolcTenantRequestObject) (adminservice.GetVolcTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:volc:"+request.Name)
	region := "cn-beijing"
	return adminservice.GetVolcTenant200JSONResponse(apitypes.VolcTenant{
		Name:           request.Name,
		CredentialName: "volc-key",
		Region:         &region,
	}), nil
}
