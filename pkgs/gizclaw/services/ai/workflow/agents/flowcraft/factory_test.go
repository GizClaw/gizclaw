package flowcraft

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestAgentStatusReportsRunning(t *testing.T) {
	state, err := (&agent{}).Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if state.RuntimeState != "running" {
		t.Fatalf("RuntimeState = %q, want running", state.RuntimeState)
	}
	if state.HistoryAvailable == nil || *state.HistoryAvailable || state.MemoryStatsAvailable == nil || *state.MemoryStatsAvailable || state.RecallAvailable == nil || *state.RecallAvailable {
		t.Fatalf("unconfigured availability = %+v", state)
	}
}

func TestAgentStatusAdvertisesConfiguredFeatures(t *testing.T) {
	a := &agent{runtime: fakeDebugClaw{}, memory: &fakeMemoryStore{}}
	state, err := a.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if state.HistoryAvailable == nil || !*state.HistoryAvailable || state.MemoryStatsAvailable == nil || !*state.MemoryStatsAvailable || state.RecallAvailable == nil || !*state.RecallAvailable {
		t.Fatalf("configured availability = %+v", state)
	}
}

func TestAgentKeepsOutputEpochPerAttachment(t *testing.T) {
	a := &agent{}
	outputA := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	outputB := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	observationsA := newFlowcraftOutputObservations()
	observationsB := newFlowcraftOutputObservations()
	ctxA := agenthost.WithAttachmentID(context.Background(), "gear-a")
	ctxB := agenthost.WithAttachmentID(context.Background(), "gear-b")
	a.registerOutput(ctxA, outputA, observationsA)
	a.registerOutput(ctxB, outputB, observationsB)
	epochA := a.setActiveOutput(outputA, "stream-a", observationsA)
	epochB := a.setActiveOutput(outputB, "stream-b", observationsB)

	if err := a.addOutput(outputA, epochA, textChunk(genx.RoleModel, "answer", "stream-a", assistantLabel, "A", false)); err != nil {
		t.Fatalf("add output A: %v", err)
	}
	if err := a.addOutput(outputB, epochB, textChunk(genx.RoleModel, "answer", "stream-b", assistantLabel, "B", false)); err != nil {
		t.Fatalf("add output B: %v", err)
	}
	if output, streamID, _, ok := a.beginReplayOutput(ctxA); !ok || output != outputA || streamID != "stream-a" {
		t.Fatalf("attachment A replay target = (%p, %q, %t), want (%p, stream-a, true)", output, streamID, ok, outputA)
	}
	if output, streamID, _, ok := a.beginReplayOutput(ctxB); !ok || output != outputB || streamID != "stream-b" {
		t.Fatalf("attachment B replay target = (%p, %q, %t), want (%p, stream-b, true)", output, streamID, ok, outputB)
	}
	if err := outputA.Done(genx.Usage{}); err != nil {
		t.Fatalf("done output A: %v", err)
	}
	if err := outputB.Done(genx.Usage{}); err != nil {
		t.Fatalf("done output B: %v", err)
	}
	chunksA := drainChunks(t, outputA.Stream())
	chunksB := drainChunks(t, outputB.Stream())
	if text, _ := chunksA[0].Part.(genx.Text); text != "A" {
		t.Fatalf("output A chunks = %#v", chunksA)
	}
	if text, _ := chunksB[0].Part.(genx.Text); text != "B" {
		t.Fatalf("output B chunks = %#v", chunksB)
	}
}

func TestAgentProvidesTransientInputsForEveryClawTurn(t *testing.T) {
	claw := &inputRecordingClaw{}
	call := 0
	a := &agent{
		runtime: claw,
		inputProvider: func(context.Context) (map[string]any, error) {
			call++
			return map[string]any{"tmp_pet_attribute_prompt": fmt.Sprintf("turn-%d", call)}, nil
		},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	for _, text := range []string{"one", "two"} {
		if err := a.runFlowcraftTextTurn(context.Background(), text, "audio", output, a.currentOutputEpoch(output)); err != nil {
			t.Fatalf("runFlowcraftTextTurn(%q) error = %v", text, err)
		}
	}
	requests := claw.Requests()
	if len(requests) != 2 {
		t.Fatalf("RoundTrip requests = %d, want 2", len(requests))
	}
	if requests[0].Inputs["tmp_pet_attribute_prompt"] != "turn-1" || requests[1].Inputs["tmp_pet_attribute_prompt"] != "turn-2" {
		t.Fatalf("RoundTrip inputs = %#v", requests)
	}
}

func TestAgentInputProviderFailureStopsBeforeClawRoundTrip(t *testing.T) {
	claw := &inputRecordingClaw{}
	a := &agent{
		runtime: claw,
		inputProvider: func(context.Context) (map[string]any, error) {
			return nil, errors.New("pet lookup failed")
		},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	err := a.runFlowcraftTextTurn(context.Background(), "hello", "audio", output, a.currentOutputEpoch(output))
	if err == nil || !strings.Contains(err.Error(), "pet lookup failed") {
		t.Fatalf("runFlowcraftTextTurn() error = %v", err)
	}
	if requests := claw.Requests(); len(requests) != 0 {
		t.Fatalf("RoundTrip requests = %#v, want none", requests)
	}
}

func TestAgentRunTurnBridgesASRClawAndTTS(t *testing.T) {
	transformer := &recordingVoiceTransformer{}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: transformer},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "answer", Content: "好的"},
			{Type: runtimeEventToken, NodeID: "answer", Content: "呀"},
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
	if err := a.runTurn(context.Background(), input, output, a.currentOutputEpoch(output), "audio"); err != nil {
		t.Fatalf("runTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	allChunks := drainChunks(t, output.Stream())
	if countHistoryAudioChunks(allChunks, "audio") == 0 {
		t.Fatalf("missing history audio chunks: %#v", allChunks)
	}
	got := flowcraftNonHistoryChunks(allChunks)

	want := []struct {
		role  genx.Role
		name  string
		label string
		text  string
		blob  []byte
		audio bool
		bos   bool
		eos   bool
	}{
		{role: genx.RoleUser, label: transcriptLabel, text: "你好"},
		{role: genx.RoleUser, label: transcriptLabel, text: "", eos: true},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, text: "好的"},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, text: "呀"},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, audio: true, bos: true},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, blob: []byte{0xaa}, audio: true},
		{role: genx.RoleModel, name: "answer", label: assistantLabel, audio: true, eos: true},
		{role: genx.RoleModel, name: assistantLabel, label: assistantLabel, text: "", eos: true},
		{role: genx.RoleModel, name: assistantLabel, label: assistantLabel, audio: true, eos: true},
	}
	if len(got) != len(want) {
		t.Fatalf("chunks len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i, want := range want {
		chunk := got[i]
		if chunk.Role != want.role || chunk.Ctrl == nil || chunk.Ctrl.Label != want.label || chunk.Ctrl.StreamID != "audio" || chunk.Ctrl.BeginOfStream != want.bos || chunk.Ctrl.EndOfStream != want.eos {
			t.Fatalf("chunk[%d] ctrl = %#v role=%s, want role=%s label=%s bos=%t eos=%t", i, chunk.Ctrl, chunk.Role, want.role, want.label, want.bos, want.eos)
		}
		if want.name != "" && chunk.Name != want.name {
			t.Fatalf("chunk[%d] name = %q, want %q", i, chunk.Name, want.name)
		}
		if !want.audio {
			text, _ := chunk.Part.(genx.Text)
			if string(text) != want.text {
				t.Fatalf("chunk[%d] text = %q, want %q", i, text, want.text)
			}
		}
		if want.audio {
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
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "answer", Content: "好的"},
			{Type: runtimeEventToken, NodeID: "answer", Content: "呀"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	if err := a.runTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, output, a.currentOutputEpoch(output), "audio"); err != nil {
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
	if firstAudio < 0 {
		t.Fatalf("missing assistant audio chunks: %#v", chunks)
	}
	if got := transformer.Texts(); !reflect.DeepEqual(got, []string{"好的", "呀"}) {
		t.Fatalf("TTS input texts = %#v, want copied assistant tokens", got)
	}
}

func TestAgentRunTurnFlushesTTSWhenNodeChanges(t *testing.T) {
	transformer := &recordingVoiceTransformer{}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: transformer},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "checkpoint", Content: "第一段"},
			{Type: runtimeEventToken, NodeID: "audit", Content: "第二段"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices: map[string]string{
			"checkpoint": "voice-answer",
			"audit":      "voice-answer",
		},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	if err := a.runFlowcraftTextTurn(context.Background(), "开始", "audio", output, a.currentOutputEpoch(output)); err != nil {
		t.Fatalf("runFlowcraftTextTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())

	firstNodeAudio := -1
	secondNodeText := -1
	for i, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Label == assistantLabel && chunk.Name == "audit" {
			if text, ok := chunk.Part.(genx.Text); ok && text == "第二段" {
				secondNodeText = i
			}
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/opus" && len(blob.Data) > 0 && chunk.Name == "checkpoint" {
			firstNodeAudio = i
		}
	}
	if firstNodeAudio < 0 || secondNodeText < 0 || firstNodeAudio > secondNodeText {
		t.Fatalf("first node audio index=%d second node text index=%d chunks=%#v", firstNodeAudio, secondNodeText, chunks)
	}
	if got := transformer.Texts(); !reflect.DeepEqual(got, []string{"第一段", "第二段"}) {
		t.Fatalf("TTS input texts = %#v, want both node texts flushed", got)
	}
}

func TestAgentRunTurnSkipsTTSForUnconfiguredNodeVoice(t *testing.T) {
	transformer := &recordingVoiceTransformer{}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: transformer},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "answer", Content: "好的。"},
			{Type: runtimeEventToken, NodeID: "tool_call", Content: `<function name="weather_forecast"><parameter name="city">上海</parameter></function>`},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	if err := a.runTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, output, a.currentOutputEpoch(output), "audio"); err != nil {
		t.Fatalf("runTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	_ = drainChunks(t, output.Stream())
	if got := transformer.Texts(); !reflect.DeepEqual(got, []string{"好的。"}) {
		t.Fatalf("TTS input texts = %#v, want only answer node text", got)
	}
}

func TestAgentTransformRunsTurnAndClosesOutput(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "answer", Content: "收到"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	stream, err := a.Transform(context.Background(), "ignored", &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{0xff}}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		genx.NewBeginOfStream("audio-1"),
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainChunks(t, stream)
	if len(chunks) == 0 {
		t.Fatal("Transform() produced no chunks")
	}
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" && chunk.Ctrl.StreamID != "audio-1" {
			t.Fatalf("unexpected stream id %q in chunk %#v", chunk.Ctrl.StreamID, chunk)
		}
	}
}

func TestAgentTransformUsesTextInputAsTranscript(t *testing.T) {
	claw := &recordingClaw{events: []runtimeEvent{
		{Type: runtimeEventToken, NodeID: "answer", Content: "我记得你来看我了"},
	}}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime:      claw,
		asrModel:     "asr",
	}
	stream, err := a.Transform(context.Background(), "ignored", &sliceStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("你好，我今天来看看你。"), Ctrl: &genx.StreamCtrl{StreamID: "text-1", Label: transcriptLabel}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "text-1", Label: transcriptLabel, EndOfStream: true}},
	}})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drainChunks(t, stream)
	if got := claw.Texts(); !reflect.DeepEqual(got, []string{"你好，我今天来看看你。"}) {
		t.Fatalf("claw texts = %#v", got)
	}
	var sawTranscript, sawAnswer bool
	for _, chunk := range chunks {
		text, _ := chunk.Part.(genx.Text)
		if chunk.Role == genx.RoleUser && chunk.Ctrl != nil && chunk.Ctrl.Label == transcriptLabel && text == "你好，我今天来看看你。" {
			sawTranscript = true
		}
		if chunk.Role == genx.RoleModel && chunk.Ctrl != nil && chunk.Ctrl.Label == assistantLabel && text == "我记得你来看我了" {
			sawAnswer = true
		}
	}
	if !sawTranscript || !sawAnswer {
		t.Fatalf("text transcript/answer not found transcript=%t answer=%t chunks=%#v", sawTranscript, sawAnswer, chunks)
	}
}

func TestAgentTransformSelfStartsEmptyConversation(t *testing.T) {
	ctx := t.Context()
	claw := &recordingClaw{
		events: []runtimeEvent{{Type: runtimeEventToken, NodeID: "opening", Content: "欢迎开始"}},
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/debug/history" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"enabled":true,"count":0,"messages":[]}`))
		},
	}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime:      claw,
		starts:       "self",
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	stream, err := a.Transform(ctx, "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk := nextChunkWithTimeout(t, stream)
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != selfStartStreamID || chunk.Name != "opening" {
		t.Fatalf("self-start chunk = %#v", chunk)
	}
	if text, _ := chunk.Part.(genx.Text); text != "欢迎开始" {
		t.Fatalf("self-start text = %q", text)
	}
	if got := claw.Texts(); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("claw texts = %#v, want empty self-start turn", got)
	}
}

func TestAgentTransformDoesNotSelfStartWithExistingHistory(t *testing.T) {
	claw := &recordingClaw{
		events: []runtimeEvent{{Type: runtimeEventToken, NodeID: "opening", Content: "不该出现"}},
		handler: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/debug/history" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"enabled":true,"count":1,"messages":[{"role":"assistant","parts":[{"type":"text","text":"已开场"}]}]}`))
		},
	}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime:      claw,
		starts:       "self",
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	stream, err := a.Transform(context.Background(), "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input done: %v", err)
	}
	if _, err := stream.Next(); !agenthost.IsStreamDone(err) && !errors.Is(err, io.EOF) {
		t.Fatalf("stream.Next() error = %v, want done", err)
	}
	if got := claw.Texts(); len(got) != 0 {
		t.Fatalf("claw texts = %#v, want no self-start", got)
	}
}

func TestAgentTransformRunsMultipleTurns(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "answer", Content: "收到"},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	ctx := t.Context()
	stream, err := a.Transform(ctx, "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Add(
		genx.NewBeginOfStream("audio-1"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true}},
	); err != nil {
		t.Fatalf("add first turn: %v", err)
	}
	var chunks []*genx.MessageChunk
	firstTextDone := false
	firstAudioDone := false
	for !firstTextDone || !firstAudioDone {
		chunk := nextChunkWithTimeout(t, stream)
		chunks = append(chunks, chunk)
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "audio-1" || chunk.Ctrl.Label != assistantLabel || !chunk.Ctrl.EndOfStream {
			continue
		}
		switch chunk.Part.(type) {
		case genx.Text:
			firstTextDone = true
		case *genx.Blob:
			firstAudioDone = true
		}
	}
	if err := input.Add(
		genx.NewBeginOfStream("audio-2"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-2"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-2", EndOfStream: true}},
	); err != nil {
		t.Fatalf("add second turn: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input Done() error = %v", err)
	}

	chunks = append(chunks, drainChunks(t, stream)...)
	seenTranscript := map[string]bool{}
	seenAssistant := map[string]bool{}
	seenAudio := map[string]bool{}
	seenHistoryAudio := map[string]bool{}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.Error == interruptedError {
			t.Fatalf("completed turn was interrupted after its output drained: %#v", chunk)
		}
		streamID := chunk.Ctrl.StreamID
		switch {
		case chunk.Ctrl.Label == genx.HistoryUserAudioLabel:
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				seenHistoryAudio[streamID] = true
			}
		case chunk.Ctrl.Label == transcriptLabel:
			seenTranscript[streamID] = true
		case chunk.Ctrl.Label == assistantLabel:
			if text, ok := chunk.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" {
				seenAssistant[streamID] = true
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				seenAudio[streamID] = true
			}
		}
	}
	for _, streamID := range []string{"audio-1", "audio-2"} {
		if !seenHistoryAudio[streamID] || !seenTranscript[streamID] || !seenAssistant[streamID] || !seenAudio[streamID] {
			t.Fatalf("stream %s seen history_audio=%t transcript=%t assistant=%t audio=%t chunks=%#v", streamID, seenHistoryAudio[streamID], seenTranscript[streamID], seenAssistant[streamID], seenAudio[streamID], chunks)
		}
	}
}

func TestAgentTransformInterruptsCurrentTurnOnNextBOS(t *testing.T) {
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime:      &interruptClaw{},
		asrModel:     "asr",
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	stream, err := a.Transform(context.Background(), "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Add(
		genx.NewBeginOfStream("audio-1"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true}},
	); err != nil {
		t.Fatalf("add first turn: %v", err)
	}

	var chunks []*genx.MessageChunk
	for {
		chunk := nextChunkWithTimeout(t, stream)
		chunks = append(chunks, chunk)
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == "audio-1" {
			if text, ok := chunk.Part.(genx.Text); ok && string(text) == "旧回复" {
				break
			}
		}
	}

	if err := input.Add(
		genx.NewBeginOfStream("audio-2"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-2"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-2", EndOfStream: true}},
	); err != nil {
		t.Fatalf("add second turn: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input Done() error = %v", err)
	}
	chunks = append(chunks, drainChunks(t, stream)...)

	var interruptedTextEOS, interruptedAudioEOS, secondText bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID == "audio-1" && chunk.Ctrl.EndOfStream && chunk.Ctrl.Error == interruptedError {
			switch chunk.Part.(type) {
			case genx.Text:
				interruptedTextEOS = true
			case *genx.Blob:
				interruptedAudioEOS = true
			}
		}
		if chunk.Ctrl.StreamID == "audio-2" {
			if text, ok := chunk.Part.(genx.Text); ok && string(text) == "新回复" {
				secondText = true
			}
		}
	}
	if !interruptedTextEOS || !interruptedAudioEOS || !secondText {
		t.Fatalf("interrupt/result flags textEOS=%t audioEOS=%t secondText=%t chunks=%#v", interruptedTextEOS, interruptedAudioEOS, secondText, chunks)
	}
}

func TestAgentRealtimeModeRunsDefiniteASRChunksAsTurns(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: &realtimeASRTransformer{texts: []string{"第一段", "第二段"}}},
		runtime: fakeEchoClaw{
			nodeID: "answer",
			prefix: "回复:",
		},
		asrModel:  "asr",
		inputMode: inputModeRealtime,
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	ctx := t.Context()
	stream, err := a.Transform(ctx, "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Add(
		genx.NewBeginOfStream("audio-1"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true}},
	); err != nil {
		t.Fatalf("add realtime audio: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input Done() error = %v", err)
	}
	chunks := drainChunks(t, stream)

	transcripts := map[string]string{}
	assistant := map[string]string{}
	historyAudio := map[string]int{}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.IsEndOfStream() {
			continue
		}
		switch chunk.Ctrl.Label {
		case genx.HistoryUserAudioLabel:
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				historyAudio[chunk.Ctrl.StreamID]++
			}
		case transcriptLabel:
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				transcripts[chunk.Ctrl.StreamID] += string(text)
			}
		case assistantLabel:
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				assistant[chunk.Ctrl.StreamID] += string(text)
			}
		}
	}
	if transcripts["audio-1:rt:1"] != "第一段" || transcripts["audio-1:rt:2"] != "第二段" {
		t.Fatalf("transcripts by stream = %#v, chunks=%#v", transcripts, chunks)
	}
	if historyAudio["audio-1:rt:1"] == 0 || historyAudio["audio-1:rt:2"] == 0 {
		t.Fatalf("history audio by stream = %#v, chunks=%#v", historyAudio, chunks)
	}
	if assistant["audio-1:rt:2"] != "回复:第二段" {
		t.Fatalf("assistant by stream = %#v, chunks=%#v", assistant, chunks)
	}
}

func TestAgentRealtimeModeInterruptsOnTranscriptBOS(t *testing.T) {
	claw := &interruptClaw{}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: &realtimeASRTransformer{chunks: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", BeginOfStream: true}},
			{Part: genx.Text("第一段"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}},
			{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", EndOfStream: true}},
			{Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", BeginOfStream: true}},
			{Part: genx.Text("第二"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}},
			{Part: genx.Text("第二段"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}},
			{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", EndOfStream: true}},
		}}},
		runtime:   claw,
		asrModel:  "asr",
		inputMode: inputModeRealtime,
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := a.Transform(ctx, "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Add(
		genx.NewBeginOfStream("audio-1"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{4}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{5}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{6}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{7}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true}},
	); err != nil {
		t.Fatalf("add realtime audio: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input Done() error = %v", err)
	}

	transcripts := map[string]string{}
	var interruptedTextEOS, interruptedAudioEOS, partialTranscript bool
	var chunks []*genx.MessageChunk
	for len(chunks) < 64 && (!interruptedTextEOS || !interruptedAudioEOS || transcripts["audio-1:rt:1"] != "第一段" || transcripts["audio-1:rt:2"] != "第二段") {
		chunk := nextChunkWithTimeout(t, stream)
		chunks = append(chunks, chunk)
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.IsEndOfStream() && chunk.Ctrl.Error == interruptedError {
			switch chunk.Part.(type) {
			case genx.Text:
				interruptedTextEOS = true
			case *genx.Blob:
				interruptedAudioEOS = true
			}
			continue
		}
		if chunk.IsEndOfStream() {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok {
			continue
		}
		if chunk.Ctrl.Label == transcriptLabel {
			transcripts[chunk.Ctrl.StreamID] += string(text)
			if string(text) == "第二" {
				partialTranscript = true
			}
		}
	}
	waitForClawCalls(t, claw, 2)
	cancel()
	_ = stream.Close()
	if !interruptedTextEOS || !interruptedAudioEOS {
		t.Fatalf("partial ASR did not interrupt current turn: textEOS=%t audioEOS=%t chunks=%#v", interruptedTextEOS, interruptedAudioEOS, chunks)
	}
	if partialTranscript {
		t.Fatalf("partial ASR chunk was emitted as transcript: transcripts=%#v chunks=%#v", transcripts, chunks)
	}
	if transcripts["audio-1:rt:1"] != "第一段" || transcripts["audio-1:rt:2"] != "第二段" {
		t.Fatalf("transcripts by stream = %#v, chunks=%#v", transcripts, chunks)
	}
}

func TestAgentRealtimeModePropagatesASRErrorWhileTurnActive(t *testing.T) {
	wantErr := errors.New("asr exploded")
	a := &agent{
		transformers: fakeTransformerProvider{transformer: &realtimeASRTransformer{
			chunks: []*genx.MessageChunk{
				{Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", BeginOfStream: true}},
				{Part: genx.Text("第一段"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}},
				{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", EndOfStream: true}},
			},
			err: wantErr,
		}},
		runtime:   &interruptClaw{},
		asrModel:  "asr",
		inputMode: inputModeRealtime,
	}
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	stream, err := a.Transform(context.Background(), "ignored", input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Add(
		genx.NewBeginOfStream("audio-1"),
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{2}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{4}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1"}},
	); err != nil {
		t.Fatalf("add realtime audio: %v", err)
	}

	for {
		_, err := stream.Next()
		if err == nil {
			continue
		}
		if !strings.Contains(err.Error(), "flowcraft: read ASR") || !errors.Is(err, wantErr) {
			t.Fatalf("stream error = %v, want wrapped ASR error", err)
		}
		break
	}
}

func TestAgentListHistoryUsesOwnedRuntimeHistory(t *testing.T) {
	a := &agent{historyID: "ctx", runtime: fakeDebugClaw{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/debug/history" {
			t.Fatalf("debug request = %s %s, want GET /debug/history", r.Method, r.URL.Path)
		}
		writeTestJSON(t, w, map[string]any{
			"enabled":    true,
			"context_id": "ctx",
			"count":      2,
			"messages": []map[string]any{
				{"role": "user", "parts": []map[string]any{{"type": "text", "text": "你好"}}},
				{"role": "assistant", "parts": []map[string]any{{"type": "text", "text": "好的"}}},
			},
		})
	}}}
	limit := 1
	resp, err := a.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if !resp.Available || !resp.HasNext || resp.NextCursor == nil || *resp.NextCursor != "1" {
		t.Fatalf("history response = %+v", resp)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.Id != "ctx:000000" ||
		item.Type != apitypes.PeerRunHistoryEntryTypeGear ||
		item.GearId == nil || *item.GearId != "flowcraft" ||
		item.Name != "gear" ||
		item.Text != "你好" {
		t.Fatalf("history item = %+v", item)
	}
}

func TestHistoryCursorAndIDHelpers(t *testing.T) {
	if got, err := parseHistoryCursor(nil); err != nil || got != 0 {
		t.Fatalf("parseHistoryCursor(nil) = %d, %v; want 0 nil", got, err)
	}
	blank := " \t "
	if got, err := parseHistoryCursor(&blank); err != nil || got != 0 {
		t.Fatalf("parseHistoryCursor(blank) = %d, %v; want 0 nil", got, err)
	}
	cursor := " 12 "
	if got, err := parseHistoryCursor(&cursor); err != nil || got != 12 {
		t.Fatalf("parseHistoryCursor(%q) = %d, %v; want 12 nil", cursor, got, err)
	}
	bad := "-1"
	if _, err := parseHistoryCursor(&bad); err == nil {
		t.Fatal("parseHistoryCursor(-1) succeeded")
	}
	if got := contextIDOrDefault(" "); got != "default" {
		t.Fatalf("contextIDOrDefault(blank) = %q, want default", got)
	}
	if got := historyEntryID("", 3); got != "default:000003" {
		t.Fatalf("historyEntryID(empty, 3) = %q, want default:000003", got)
	}
}

func TestAgentMemoryStatsReportsInjectedStore(t *testing.T) {
	a := &agent{memory: &fakeMemoryStore{}}
	resp, err := a.MemoryStats(context.Background(), nil)
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if !resp.Available || !resp.Enabled {
		t.Fatalf("memory stats = %+v", resp)
	}
	if resp.Backend == nil || *resp.Backend != "memory.Store" {
		t.Fatalf("memory backend = %v, want memory.Store", resp.Backend)
	}
	if resp.Metadata == nil {
		t.Fatal("memory metadata is nil")
	}
}

func TestAgentMemoryStatsReportsUnavailableMessage(t *testing.T) {
	resp, err := (&agent{}).MemoryStats(context.Background(), nil)
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if resp.Available || resp.Message == nil || !strings.Contains(*resp.Message, "not configured") {
		t.Fatalf("MemoryStats() = %+v, want unavailable message", resp)
	}
	if resp.Metadata != nil {
		t.Fatalf("MemoryStats() metadata = %+v, want nil", resp.Metadata)
	}
}

func TestAgentRecallUsesInjectedMemoryStore(t *testing.T) {
	store := &fakeMemoryStore{recall: memory.RecallResult{Matches: []memory.Match{{
		Fact:  memory.Fact{ID: "fact-1", Text: "喜欢咖啡", Attributes: map[string]any{"kind": "note"}, Sources: []memory.SourceRef{{ObservationID: "turn-1"}}},
		Score: 0.8,
	}}}}
	a := &agent{memory: store}
	limit := 3
	filters := map[string]any{"kind": "note"}
	resp, err := a.Recall(context.Background(), apitypes.PeerRunRecallRequest{Query: "咖啡", Limit: &limit, Filters: &filters})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !resp.Available || len(resp.Hits) != 1 {
		t.Fatalf("recall response = %+v", resp)
	}
	hit := resp.Hits[0]
	if hit.Id != "fact-1" || hit.Snippet != "喜欢咖啡" || hit.Score != 0.8 || hit.SourceType == nil || *hit.SourceType != "memory" || hit.SourceId == nil || *hit.SourceId != "turn-1" {
		t.Fatalf("recall hit = %+v", hit)
	}
	if store.query.Text != "咖啡" || store.query.Limit != 3 || len(store.query.Filters) != 1 || store.query.Filters[0].Field != "kind" {
		t.Fatalf("memory query = %+v", store.query)
	}
}

func TestAgentPlayHistoryEmitsAssistantTextAndAudio(t *testing.T) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		defaultVoice: "voice-answer",
		historyID:    "ctx",
		runtime: fakeDebugClaw{handler: func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/debug/history" {
				t.Fatalf("debug request = %s %s, want GET /debug/history", r.Method, r.URL.Path)
			}
			writeTestJSON(t, w, map[string]any{
				"enabled":    true,
				"context_id": "ctx",
				"count":      2,
				"messages": []map[string]any{
					{"role": "user", "parts": []map[string]any{{"type": "text", "text": "你好"}}},
					{"role": "assistant", "parts": []map[string]any{{"type": "text", "text": "好的"}}},
				},
			})
		}},
	}
	a.setActiveOutput(output, "audio")
	resp, err := a.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: "ctx:000001"})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if !resp.Accepted || resp.State != "played" {
		t.Fatalf("PlayHistory() response = %+v", resp)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	var textChunks []string
	var audioChunks int
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			textChunks = append(textChunks, string(text))
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/opus" && len(blob.Data) > 0 {
			audioChunks++
		}
	}
	if !reflect.DeepEqual(textChunks, []string{"好的"}) || audioChunks != 1 {
		t.Fatalf("replay chunks text=%#v audio=%d all=%#v", textChunks, audioChunks, chunks)
	}
}

func TestAgentPlayHistoryInterruptsStaleOutputEpoch(t *testing.T) {
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		defaultVoice: "voice-answer",
		historyID:    "ctx",
		runtime: fakeDebugClaw{handler: func(w http.ResponseWriter, r *http.Request) {
			writeTestJSON(t, w, map[string]any{
				"enabled":    true,
				"context_id": "ctx",
				"count":      1,
				"messages": []map[string]any{
					{"role": "assistant", "parts": []map[string]any{{"type": "text", "text": "历史回复"}}},
				},
			})
		}},
	}
	staleEpoch := a.setActiveOutput(output, "audio")
	if err := a.addOutput(output, staleEpoch, textChunk(genx.RoleModel, "answer", "audio", assistantLabel, "旧回复1", false)); err != nil {
		t.Fatalf("add stale prelude: %v", err)
	}
	resp, err := a.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: "ctx:000000"})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("PlayHistory() response = %+v", resp)
	}
	if err := a.addOutput(output, staleEpoch, textChunk(genx.RoleModel, "answer", "audio", assistantLabel, "旧回复2", false)); err != nil {
		t.Fatalf("add stale after replay: %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	var texts []string
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			texts = append(texts, string(text))
		}
	}
	if !reflect.DeepEqual(texts, []string{"旧回复1", "历史回复"}) {
		t.Fatalf("texts after replay interrupt = %#v, want stale post-replay suppressed", texts)
	}
}

func TestFactoryNewAgentWritesClawConfig(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Models:          fakeModels{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	generateModel := "chat"
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{GenerateModel: &generateModel}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters() error = %v", err)
	}
	workflow := testFlowcraftWorkflow(apitypes.FlowcraftWorkflowSpec{
		"history": map[string]any{"enabled": false},
		"memory":  map[string]any{"enabled": false},
		"voice_adapter": map[string]any{
			"asr_model":     "asr",
			"default_voice": "voice",
		},
	})
	transformer, err := (Factory{GenX: service}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: &workspaceParams},
		Workflow:  workflow,
		Runtime:   workspace.Runtime{LocalDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	t.Cleanup(func() {
		if a, ok := transformer.(*agent); ok && a.runtime != nil {
			_ = a.runtime.CloseContext(context.Background())
		}
	})
	if transformer == nil {
		t.Fatal("NewAgent() agent = nil")
	}
	gotAgent, ok := transformer.(*agent)
	if !ok {
		t.Fatalf("NewAgent() type = %T, want *agent", transformer)
	}
	if gotAgent.inputMode != inputModePushToTalk {
		t.Fatalf("agent inputMode = %q, want %q", gotAgent.inputMode, inputModePushToTalk)
	}
}

func TestFactoryNewAgentInjectsWorkspaceScopedHistoryStore(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Models:          fakeModels{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	generateModel := "chat"
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{GenerateModel: &generateModel}); err != nil {
		t.Fatal(err)
	}
	workflow := testFlowcraftWorkflow(apitypes.FlowcraftWorkflowSpec{
		"history": map[string]any{"enabled": true},
		"memory":  map[string]any{"enabled": false},
		"voice_adapter": map[string]any{
			"asr_model":     "asr",
			"default_voice": "voice",
		},
	})
	backend := &memoryHistoryLogStore{}
	localDir := t.TempDir()
	transformer, err := (Factory{GenX: service, History: backend}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace", Parameters: &workspaceParams},
		Workflow:  workflow,
		Runtime:   workspace.Runtime{LocalDir: localDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, ok := transformer.(*agent)
	if !ok {
		t.Fatalf("NewAgent() = %T", transformer)
	}
	t.Cleanup(func() { _ = agent.runtime.CloseContext(context.Background()) })
	messages, err := agent.readHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 || agent.historyID != "workspace" {
		t.Fatalf("history = %#v, id = %q", messages, agent.historyID)
	}
	backend.mu.Lock()
	queries := backend.queryCalls
	var query logstore.Query
	if queries > 0 {
		query = backend.queries[len(backend.queries)-1]
	}
	backend.mu.Unlock()
	if queries == 0 {
		t.Fatal("owned history did not query the injected LogStore")
	}
	if !slices.ContainsFunc(query.Matchers, func(matcher logstore.AttributeMatcher) bool {
		return matcher.Name == "workspace_name" && matcher.Value == "workspace"
	}) {
		t.Fatalf("history query matchers = %#v, want workspace_name=workspace", query.Matchers)
	}
	if _, err := os.Stat(filepath.Join(localDir, "history", "conversation", "messages.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace history file exists or stat failed: %v", err)
	}
}

func TestFactoryNewAgentReadsWorkspaceInputMode(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Models:          fakeModels{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	generateModel := "chat"
	input := apitypes.WorkspaceInputModeRealtime
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{GenerateModel: &generateModel, Input: &input}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters() error = %v", err)
	}
	workflow := testFlowcraftWorkflow(apitypes.FlowcraftWorkflowSpec{
		"voice_adapter": map[string]any{
			"asr_model":     "asr",
			"default_voice": "voice",
		},
	})
	transformer, err := (Factory{GenX: service}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: &workspaceParams},
		Workflow:  workflow,
		Runtime:   workspace.Runtime{LocalDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	t.Cleanup(func() {
		if a, ok := transformer.(*agent); ok && a.runtime != nil {
			_ = a.runtime.CloseContext(context.Background())
		}
	})
	gotAgent, ok := transformer.(*agent)
	if !ok {
		t.Fatalf("NewAgent() type = %T, want *agent", transformer)
	}
	if gotAgent.inputMode != inputModeRealtime {
		t.Fatalf("agent inputMode = %q, want %q", gotAgent.inputMode, inputModeRealtime)
	}
}

func TestFactoryNewAgentRejectsNonFlowcraftWorkspaceParameters(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	service := peergenx.New(peergenx.Service{
		Peer:            testPeer{},
		Models:          fakeModels{events: &events},
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
	})
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{TranslationModel: new("translate")}); err != nil {
		t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
	}
	workflow := testFlowcraftWorkflow(apitypes.FlowcraftWorkflowSpec{
		"voice_adapter": map[string]any{
			"asr_model":     "asr",
			"default_voice": "voice",
		},
	})
	_, err := (Factory{GenX: service}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "ws", Parameters: &workspaceParams},
		Workflow:  workflow,
		Runtime:   workspace.Runtime{LocalDir: t.TempDir()},
	})
	if err == nil || !strings.Contains(err.Error(), "decode workspace parameters") {
		t.Fatalf("NewAgent() error = %v, want workspace parameter decode failure", err)
	}
}

func TestFlowcraftConversationSettingsReadsWorkspaceInitiative(t *testing.T) {
	initiative := apitypes.FlowcraftConversationParametersInitiativeAgent
	policyValue := apitypes.FlowcraftConversationParametersAgentInitiativePolicyOnReload
	workspaceParams := apitypes.FlowcraftWorkspaceParameters{
		Conversation: &apitypes.FlowcraftConversationParameters{
			Initiative:            &initiative,
			AgentInitiativePolicy: &policyValue,
		},
	}
	starts, policy := flowcraftConversationSettings(&workspaceParams, map[string]any{
		"conversation": map[string]any{"starts": "peer"},
	})
	if starts != "self" || policy != "on_reload" {
		t.Fatalf("settings = %q/%q, want self/on_reload", starts, policy)
	}

	initiative = apitypes.FlowcraftConversationParametersInitiativePeer
	workspaceParams = apitypes.FlowcraftWorkspaceParameters{
		Conversation: &apitypes.FlowcraftConversationParameters{Initiative: &initiative},
	}
	starts, policy = flowcraftConversationSettings(&workspaceParams, map[string]any{
		"conversation": map[string]any{"starts": "self"},
	})
	if starts != "peer" || policy != "once_when_empty" {
		t.Fatalf("settings = %q/%q, want peer/once_when_empty", starts, policy)
	}

	starts, policy = flowcraftConversationSettings(nil, map[string]any{
		"conversation": map[string]any{"starts": "self"},
	})
	if starts != "self" || policy != "on_reload" {
		t.Fatalf("settings = %q/%q, want self/on_reload", starts, policy)
	}

	initiative = apitypes.FlowcraftConversationParametersInitiativeAgent
	workspaceParams = apitypes.FlowcraftWorkspaceParameters{
		Conversation: &apitypes.FlowcraftConversationParameters{Initiative: &initiative},
	}
	starts, policy = flowcraftConversationSettings(&workspaceParams, map[string]any{
		"conversation": map[string]any{"starts": "peer"},
	})
	if starts != "self" || policy != "on_reload" {
		t.Fatalf("settings = %q/%q, want self/on_reload", starts, policy)
	}

	policyValue = apitypes.FlowcraftConversationParametersAgentInitiativePolicyOnceWhenEmpty
	workspaceParams = apitypes.FlowcraftWorkspaceParameters{
		Conversation: &apitypes.FlowcraftConversationParameters{
			Initiative:            &initiative,
			AgentInitiativePolicy: &policyValue,
		},
	}
	starts, policy = flowcraftConversationSettings(&workspaceParams, map[string]any{
		"conversation": map[string]any{"starts": "peer"},
	})
	if starts != "self" || policy != "once_when_empty" {
		t.Fatalf("settings = %q/%q, want self/once_when_empty", starts, policy)
	}
}

func TestParseWorkflowConfigTrimsNodeVoices(t *testing.T) {
	cfg, err := parseWorkflowConfig(agenthost.Spec{Workflow: testFlowcraftWorkflow(apitypes.FlowcraftWorkflowSpec{
		"voice_adapter": map[string]any{
			"asr_model":     " asr ",
			"default_voice": " voice ",
			"node_voices": map[string]any{
				" answer ": " voice-a ",
				"":         "ignored",
				"empty":    " ",
			},
		},
	})})
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

func TestOwnedClawCloseContextHandlesNil(t *testing.T) {
	if err := (ownedRuntime{}).CloseContext(context.Background()); err != nil {
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
	if err := a.synthesize(context.Background(), "audio", "answer", "voice", "hello", output, a.currentOutputEpoch(output)); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 4 {
		t.Fatalf("chunks len = %d, want 4", len(chunks))
	}
	if !chunks[0].IsBeginOfStream() || chunks[0].Ctrl.StreamID != "audio" || chunks[0].Ctrl.Label != assistantLabel {
		t.Fatalf("audio BOS = %#v", chunks[0])
	}
	for i, want := range [][]byte{{0x21}, {0x22}, nil} {
		blob := chunks[i+1].Part.(*genx.Blob)
		if blob.MIMEType != "audio/opus" || !reflect.DeepEqual(blob.Data, want) {
			t.Fatalf("chunk[%d] blob = %#v, want %v", i+1, blob, want)
		}
	}
	if !chunks[3].IsEndOfStream() {
		t.Fatal("last chunk is not EOS")
	}
}

func TestDrainTTSOutputStreamsOggFramesBeforeTTSEOS(t *testing.T) {
	raw := buildOggPackets(t, opusHeadPacket(16000, 1), opusTagsPacket("test"), []byte{0x21})
	tts := newBlockingChunkStream()
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	a := &agent{}
	epoch := a.setActiveOutput(output, "audio")

	done := make(chan error, 1)
	go func() {
		done <- a.drainTTSOutput(context.Background(), "audio", "answer", "voice", tts, output, epoch, true)
	}()

	tts.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg", Data: raw}})
	chunk := nextChunkWithTimeout(t, output.Stream())
	if chunk.Ctrl == nil || !chunk.Ctrl.BeginOfStream || chunk.Ctrl.StreamID != "audio" || chunk.Ctrl.Label != assistantLabel {
		t.Fatalf("streamed audio BOS = %#v", chunk)
	}
	chunk = nextChunkWithTimeout(t, output.Stream())
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{0x21}) {
		t.Fatalf("first streamed chunk = %#v", chunk)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "audio" || chunk.Ctrl.Label != assistantLabel || chunk.Ctrl.EndOfStream {
		t.Fatalf("first streamed ctrl = %#v", chunk.Ctrl)
	}
	select {
	case err := <-done:
		t.Fatalf("drainTTSOutput returned before TTS EOS: %v", err)
	default:
	}

	tts.Close()
	if err := <-done; err != nil {
		t.Fatalf("drainTTSOutput() error = %v", err)
	}
	chunk = nextChunkWithTimeout(t, output.Stream())
	if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream {
		t.Fatalf("final audio eos = %#v", chunk)
	}
}

func TestSynthesizeTextSegmentCanOmitAudioEOS(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: patternTransformer{
			pattern: "voice/voice",
			stream: &sliceStream{chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x44}}},
			}},
		}},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	if err := a.synthesizeTextSegment(context.Background(), "audio", "answer", "voice", " hello ", output, a.currentOutputEpoch(output), false); err != nil {
		t.Fatalf("synthesizeTextSegment() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 2 {
		t.Fatalf("chunks len = %d, want BOS and one audio frame", len(chunks))
	}
	if chunks[0].Ctrl == nil || !chunks[0].Ctrl.BeginOfStream || chunks[0].Ctrl.StreamID != "audio" || chunks[0].Ctrl.Label != assistantLabel {
		t.Fatalf("audio BOS = %#v", chunks[0])
	}
	if blob, ok := chunks[1].Part.(*genx.Blob); !ok || !bytes.Equal(blob.Data, []byte{0x44}) || chunks[1].IsEndOfStream() {
		t.Fatalf("audio chunk = %#v", chunks[1])
	}

	output = genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	if err := a.synthesizeTextSegment(context.Background(), "audio", "answer", "voice", " ", output, a.currentOutputEpoch(output), true); err != nil {
		t.Fatalf("synthesizeTextSegment(blank) error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done(blank) error = %v", err)
	}
	if chunks := drainChunks(t, output.Stream()); len(chunks) != 0 {
		t.Fatalf("blank segment chunks = %#v, want none", chunks)
	}
}

func TestWatchInputInterruptEmitsInterruptedEOSAndCancels(t *testing.T) {
	a := &agent{}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	epoch := a.setActiveOutput(output, "audio-1")
	if err := output.Add(
		textChunk(genx.RoleModel, assistantLabel, "audio-1", assistantLabel, "stale", false),
		audioChunk(assistantLabel, "audio-1", []byte{1, 2}, false),
	); err != nil {
		t.Fatalf("queue stale output: %v", err)
	}
	canceled := make(chan struct{}, 1)
	a.watchInputInterrupt(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("ignored")},
		genx.NewBeginOfStream("audio-2"),
	}}, output, "audio-1", epoch, func() {
		canceled <- struct{}{}
	})
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("cancel was not called")
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 2 {
		t.Fatalf("interrupt chunks = %#v, want text and audio EOS", chunks)
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "audio-1" || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != interruptedError {
			t.Fatalf("interrupt chunk = %#v", chunk)
		}
	}
}

func TestOggOpusFrameDecoderAcceptsPartialHeader(t *testing.T) {
	raw := buildOggPackets(t, opusHeadPacket(16000, 1), opusTagsPacket("test"), []byte{0x31})
	decoder := newOggOpusFrameDecoder()
	frames, err := decoder.Write(raw[:10])
	if err != nil {
		t.Fatalf("Write(partial header) error = %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("partial header frames = %#v, want none", frames)
	}
	frames, err = decoder.Write(raw[10:])
	if err != nil {
		t.Fatalf("Write(rest) error = %v", err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], []byte{0x31}) {
		t.Fatalf("frames = %#v, want one opus packet", frames)
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
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
	err := a.synthesize(context.Background(), "audio", "answer", "voice", "hello", output, a.currentOutputEpoch(output))
	if err == nil || !strings.Contains(err.Error(), "unsupported TTS audio MIME") {
		t.Fatalf("synthesize() error = %v", err)
	}
}

func TestDrainTTSOutputErrors(t *testing.T) {
	a := &agent{}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	wantErr := errors.New("tts read failed")
	err := a.drainTTSOutput(context.Background(), "audio", "answer", "voice", &sliceStream{err: wantErr}, output, a.currentOutputEpoch(output), true)
	if err == nil || !strings.Contains(err.Error(), "read TTS") || !errors.Is(err, wantErr) {
		t.Fatalf("drainTTSOutput(read error) = %v", err)
	}
	err = a.drainTTSOutput(context.Background(), "audio", "answer", "voice", &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("OggS")}},
	}}, output, a.currentOutputEpoch(output), true)
	if err == nil || !strings.Contains(err.Error(), "decode TTS ogg") {
		t.Fatalf("drainTTSOutput(truncated ogg) = %v", err)
	}
}

func TestInterruptOutputGuards(t *testing.T) {
	a := &agent{}
	if a.interruptOutput(nil, "audio", 0) {
		t.Fatal("interruptOutput(nil) = true")
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	if a.interruptOutput(output, "audio", 99) {
		t.Fatal("interruptOutput(stale epoch) = true")
	}
}

func TestInterruptQueuedOutputDiscardsCompletedAssistantOnly(t *testing.T) {
	a := &agent{}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	epoch := a.setActiveOutput(output, "audio")
	if err := output.Add(
		textChunk(genx.RoleUser, transcriptLabel, "audio", transcriptLabel, "hello", false),
		audioChunk(assistantLabel, "audio", []byte{1, 2}, false),
		audioChunk(assistantLabel, "audio", nil, true),
	); err != nil {
		t.Fatalf("queue output: %v", err)
	}
	if !a.interruptQueuedOutput(output, "audio", epoch) {
		t.Fatal("interruptQueuedOutput() = false")
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want transcript and two interrupted EOS chunks", chunks)
	}
	if chunks[0].Role != genx.RoleUser || chunks[0].Ctrl == nil || chunks[0].Ctrl.Label != transcriptLabel {
		t.Fatalf("preserved transcript = %#v", chunks[0])
	}
	for _, chunk := range chunks[1:] {
		if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != interruptedError {
			t.Fatalf("interrupted chunk = %#v", chunk)
		}
	}
}

func TestInterruptQueuedOutputSignalsConsumedAssistant(t *testing.T) {
	a := &agent{}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	epoch := a.setActiveOutput(output, "audio")
	if !a.interruptQueuedOutput(output, "audio", epoch) {
		t.Fatal("interruptQueuedOutput() = false")
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want two interrupted EOS chunks", chunks)
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != interruptedError {
			t.Fatalf("interrupted chunk = %#v", chunk)
		}
	}
}

func TestFinishCompletedOutputDoesNotInterruptEmptyTurn(t *testing.T) {
	a := &agent{}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	observations := newFlowcraftOutputObservations()
	epoch := a.setActiveOutput(output, "audio", observations)
	observations.begin("audio")
	if err := a.addOutput(output, epoch,
		textChunk(genx.RoleModel, assistantLabel, "audio", assistantLabel, "", true),
		audioChunk(assistantLabel, "audio", nil, true),
	); err != nil {
		t.Fatalf("queue empty completion: %v", err)
	}

	a.finishCompletedOutput(output, &flowcraftActiveTurn{streamID: "audio", epoch: epoch}, observations)

	if got := a.currentOutputEpoch(output); got != epoch {
		t.Fatalf("output epoch = %d, want unchanged %d", got, epoch)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want two successful EOS chunks", chunks)
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != "" {
			t.Fatalf("completion chunk = %#v, want successful EOS", chunk)
		}
	}
}

func TestOutputObservationsForwardOnlyDevicePulledAssistantChunks(t *testing.T) {
	observer := &recordingOutputObserver{}
	observations := newFlowcraftOutputObservations(observer)
	observations.begin("stream")
	text := textChunk(genx.RoleModel, assistantLabel, "stream", assistantLabel, "visible", false)
	interrupted := textChunk(genx.RoleModel, assistantLabel, "stream", assistantLabel, "", true)
	interrupted.Ctrl.Error = interruptedError

	observations.produce(text)
	if len(observer.chunks) != 0 {
		t.Fatalf("produced chunks observed before pull: %#v", observer.chunks)
	}
	observations.observe(text)
	observations.observe(interrupted)
	if len(observer.chunks) != 2 || observer.chunks[0] != text || observer.chunks[1] != interrupted {
		t.Fatalf("observed chunks = %#v", observer.chunks)
	}
}

func TestFinishCompletedOutputInterruptsUnobservedQueuedAssistant(t *testing.T) {
	observer := &recordingOutputObserver{}
	a := &agent{outputObserver: observer}
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	observations := newFlowcraftOutputObservations()
	epoch := a.setActiveOutput(output, "audio", observations)
	observations.begin("audio")
	if err := a.addOutput(output, epoch, audioChunk(assistantLabel, "audio", []byte{1, 2}, false)); err != nil {
		t.Fatalf("queue output: %v", err)
	}

	a.finishCompletedOutput(output, &flowcraftActiveTurn{streamID: "audio", epoch: epoch}, observations)
	if !reflect.DeepEqual(observer.interrupts, []string{"audio"}) {
		t.Fatalf("history interruptions = %#v", observer.interrupts)
	}

	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want two interrupted EOS chunks", chunks)
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != interruptedError {
			t.Fatalf("interrupted chunk = %#v", chunk)
		}
	}
}

func TestRealtimeInputInterruptsCurrent(t *testing.T) {
	tests := []struct {
		name    string
		current string
		input   string
		want    bool
	}{
		{name: "same stream", current: "audio-1", input: "audio-1", want: false},
		{name: "same realtime segment prefix", current: "audio-1:rt:1", input: "audio-1", want: false},
		{name: "different stream", current: "audio-1:rt:1", input: "audio-2", want: true},
		{name: "self start interrupted by input", current: selfStartStreamID, input: "audio-1", want: true},
		{name: "missing current is conservative", current: "", input: "audio-1", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := realtimeInputInterruptsCurrent(tt.current, tt.input); got != tt.want {
				t.Fatalf("realtimeInputInterruptsCurrent(%q, %q) = %t, want %t", tt.current, tt.input, got, tt.want)
			}
		})
	}
}

func TestAudioMIMEHelpers(t *testing.T) {
	if got := baseMIME(" audio/ogg; codecs=opus "); got != "audio/ogg" {
		t.Fatalf("baseMIME() = %q", got)
	}
	if !isAudioMIME("audio/opus") || !isAudioMIME("audio/pcm") || isAudioMIME("text/plain") {
		t.Fatalf("isAudioMIME() returned unexpected values")
	}
}

func TestRealtimeStreamIDHelpers(t *testing.T) {
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: " turn-a "}}
	if got := realtimeASRStreamID(chunk, "fallback"); got != "turn-a" {
		t.Fatalf("realtimeASRStreamID(chunk) = %q, want turn-a", got)
	}
	if got := realtimeASRStreamID(&genx.MessageChunk{}, " fallback "); got != "fallback" {
		t.Fatalf("realtimeASRStreamID(fallback) = %q, want fallback", got)
	}
	if got := realtimeASRStreamID(nil, " "); got != defaultInputStreamID {
		t.Fatalf("realtimeASRStreamID(default) = %q, want %q", got, defaultInputStreamID)
	}
	if got := realtimeTurnStreamID(" base ", 0); got != "base:rt:1" {
		t.Fatalf("realtimeTurnStreamID(base, 0) = %q, want base:rt:1", got)
	}
	eos := userAudioHistoryEOSChunk("", "")
	if eos.Role != genx.RoleUser || eos.Name != transcriptLabel || eos.Ctrl == nil || eos.Ctrl.StreamID != defaultInputStreamID || eos.Ctrl.Label != genx.HistoryUserAudioLabel || !eos.IsEndOfStream() {
		t.Fatalf("userAudioHistoryEOSChunk route = %#v", eos)
	}
	if blob, ok := eos.Part.(*genx.Blob); !ok || blob.MIMEType != "audio/pcm" {
		t.Fatalf("userAudioHistoryEOSChunk part = %#v, want audio/pcm blob", eos.Part)
	}
}

func TestOwnedRuntimeNilHistoryAndClose(t *testing.T) {
	if err := (ownedRuntime{}).CloseContext(context.Background()); err != nil {
		t.Fatalf("CloseContext(nil) error = %v", err)
	}
	if _, err := (ownedRuntime{}).History(t.Context(), 1); err == nil || !strings.Contains(err.Error(), "nil owned runtime") {
		t.Fatalf("History(nil) error = %v", err)
	}
}

func TestOwnedRuntimeResponseSurfacesTerminalChunkError(t *testing.T) {
	response := &ownedRuntimeResponse{stream: &sliceStream{chunks: []*genx.MessageChunk{{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: "owned runtime failed"},
	}}}}
	event, err := response.Next()
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != runtimeEventError || !event.IsError || event.Err != "owned runtime failed" {
		t.Fatalf("runtime event = %#v", event)
	}
}

func TestRunTurnReturnsClawEventError(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventError, Err: "boom", IsError: true},
		}},
		asrModel:     "asr",
		defaultVoice: "voice",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	err := a.runTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, output, a.currentOutputEpoch(output), "audio")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runTurn() error = %v", err)
	}
}

func TestRunTurnCompletesPartialLengthLimitedClawResponse(t *testing.T) {
	transformer := &recordingVoiceTransformer{}
	a := &agent{
		transformers: fakeTransformerProvider{transformer: transformer},
		runtime: fakeClaw{events: []runtimeEvent{
			{Type: runtimeEventToken, NodeID: "answer", Content: "月门亮起来了"},
			{Type: runtimeEventError, Err: `llm round "answer": stream error: bytedance: response incomplete: length`, IsError: true},
		}},
		defaultVoice: "voice",
		nodeVoices:   map[string]string{"answer": "voice-answer"},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	if err := a.runFlowcraftTextTurn(context.Background(), "开始", "audio", output, a.currentOutputEpoch(output)); err != nil {
		t.Fatalf("runFlowcraftTextTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	var gotText bool
	var gotAudioEOS bool
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && text == "月门亮起来了" {
			gotText = true
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/opus" && chunk.IsEndOfStream() {
			gotAudioEOS = true
		}
	}
	if !gotText || !gotAudioEOS {
		t.Fatalf("partial response chunks missing text/audio eos: text=%t audioEOS=%t chunks=%#v", gotText, gotAudioEOS, chunks)
	}
	if got := transformer.Texts(); !reflect.DeepEqual(got, []string{"月门亮起来了"}) {
		t.Fatalf("TTS input texts = %#v", got)
	}
}

func TestRunTranscriptTurnEmitsTranscriptAndAssistant(t *testing.T) {
	a := &agent{
		transformers: fakeTransformerProvider{transformer: fakeVoiceTransformer{}},
		runtime: fakeEchoClaw{
			nodeID: "answer",
			prefix: "回复:",
		},
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	if err := a.runTranscriptTurn(context.Background(), "  你好  ", "", output, a.currentOutputEpoch(output), true); err != nil {
		t.Fatalf("runTranscriptTurn() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	var transcript, assistant bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID != defaultInputStreamID {
			t.Fatalf("chunk stream id = %q, want default", chunk.Ctrl.StreamID)
		}
		if chunk.Ctrl.Label == transcriptLabel {
			if text, ok := chunk.Part.(genx.Text); ok && text == "你好" {
				transcript = true
			}
		}
		if chunk.Ctrl.Label == assistantLabel {
			if text, ok := chunk.Part.(genx.Text); ok && text == "回复:你好" {
				assistant = true
			}
		}
	}
	if !transcript || !assistant {
		t.Fatalf("missing transcript/assistant chunks transcript=%t assistant=%t chunks=%#v", transcript, assistant, chunks)
	}
	if err := a.runTranscriptTurn(context.Background(), " ", "audio", output, a.currentOutputEpoch(output), false); err == nil || !strings.Contains(err.Error(), "empty transcript") {
		t.Fatalf("runTranscriptTurn(empty) error = %v", err)
	}
}

func TestTTSSessionMethods(t *testing.T) {
	session := &ttsSession{
		input:    genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4),
		done:     make(chan error, 1),
		streamID: "audio",
		nodeID:   "answer",
	}
	session.done <- nil
	if err := session.AddText("hello"); err != nil {
		t.Fatalf("AddText() error = %v", err)
	}
	if err := session.CloseInput(); err != nil {
		t.Fatalf("CloseInput() error = %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if err := (*ttsSession)(nil).AddText("hello"); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("AddText(nil) error = %v", err)
	}
	if err := (*ttsSession)(nil).CloseInput(); err != nil {
		t.Fatalf("CloseInput(nil) error = %v", err)
	}
	if err := (*ttsSession)(nil).Wait(); err != nil {
		t.Fatalf("Wait(nil) error = %v", err)
	}
}

func TestTranscribeInputTurnUsesPrefetchedStreamID(t *testing.T) {
	const streamID = "client-turn"
	a := &agent{
		transformers: fakeTransformerProvider{transformer: patternTransformer{
			pattern: "model/asr",
			stream: &sliceStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("你好"), Ctrl: &genx.StreamCtrl{EndOfStream: true}},
			}},
		}},
		asrModel: "asr",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	transcript, gotStreamID, err := a.transcribeInputTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		{Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
	}}, output, a.currentOutputEpoch(output), "fallback")
	if err != nil {
		t.Fatalf("transcribeInputTurn() error = %v", err)
	}
	if transcript != "你好" || gotStreamID != streamID {
		t.Fatalf("transcript=%q streamID=%q", transcript, gotStreamID)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	var historyAudio, historyAudioEOS, transcriptText, transcriptEOS bool
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.Label == genx.HistoryUserAudioLabel || chunk.Ctrl.Label == transcriptLabel {
			if chunk.Ctrl.StreamID != streamID {
				t.Errorf("history chunk stream ID = %q, want %q: %#v", chunk.Ctrl.StreamID, streamID, chunk)
			}
		}
		if chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/pcm" && len(blob.Data) > 0 {
				historyAudio = true
			}
			if chunk.IsEndOfStream() {
				historyAudioEOS = true
			}
		}
		if chunk.Ctrl.Label == transcriptLabel {
			if text, ok := chunk.Part.(genx.Text); ok && string(text) == "你好" && !chunk.IsEndOfStream() {
				transcriptText = true
			}
			if chunk.IsEndOfStream() {
				transcriptEOS = true
			}
		}
	}
	if !historyAudio || !historyAudioEOS || !transcriptText || !transcriptEOS {
		t.Fatalf("chunks missing history/transcript flags audio=%t audioEOS=%t text=%t textEOS=%t chunks=%#v", historyAudio, historyAudioEOS, transcriptText, transcriptEOS, chunks)
	}
}

func TestTranscribeInputTurnDoesNotTreatASRHistoryAudioEOSAsTranscriptEOS(t *testing.T) {
	const streamID = "client-turn"
	a := &agent{
		transformers: fakeTransformerProvider{transformer: patternTransformer{
			pattern: "model/asr",
			stream: &sliceStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleUser, Name: transcriptLabel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel}},
				{Role: genx.RoleUser, Name: transcriptLabel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
				{Role: genx.RoleUser, Name: transcriptLabel, Part: genx.Text("你好"), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: transcriptLabel, EndOfStream: true}},
			}},
		}},
		asrModel: "asr",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	transcript, gotStreamID, err := a.transcribeInputTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{3, 4}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
	}}, output, a.currentOutputEpoch(output), "fallback")
	if err != nil {
		t.Fatalf("transcribeInputTurn() error = %v", err)
	}
	if transcript != "你好" || gotStreamID != streamID {
		t.Fatalf("transcript=%q streamID=%q", transcript, gotStreamID)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("Done() error = %v", err)
	}
	chunks := drainChunks(t, output.Stream())
	transcriptEOS := 0
	for _, chunk := range chunks {
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == transcriptLabel && chunk.IsEndOfStream() {
			transcriptEOS++
		}
	}
	if transcriptEOS != 1 {
		t.Fatalf("transcript EOS count = %d, want 1; chunks=%#v", transcriptEOS, chunks)
	}
}

func TestTranscribeInputTurnStartASRError(t *testing.T) {
	const streamID = "client-turn"
	want := errors.New("start failed")
	a := &agent{
		transformers: fakeTransformerProvider{transformer: errorTransformer{err: want}},
		asrModel:     "asr",
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	_, gotStreamID, err := a.transcribeInputTurn(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		{Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
	}}, output, a.currentOutputEpoch(output), "fallback")
	if err == nil || !strings.Contains(err.Error(), "start ASR") || !errors.Is(err, want) {
		t.Fatalf("transcribeInputTurn() error = %v", err)
	}
	if gotStreamID != streamID {
		t.Fatalf("transcribeInputTurn() stream ID = %q, want %q", gotStreamID, streamID)
	}
}

func TestFeedASRInputReturnsInputError(t *testing.T) {
	want := errors.New("input failed")
	input := &sliceStream{err: want}
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	result := feedASRInput(context.Background(), input, asrInput, defaultInputStreamID, nil)
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
	result := feedASRInput(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "audio"}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}, asrInput, defaultInputStreamID, nil)
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

func TestFeedRealtimeASRInputForwardsAudioAndDone(t *testing.T) {
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	state := &lockedString{}
	inputStarted := make(chan string, 1)
	result := feedRealtimeASRInput(context.Background(), &sliceStream{chunks: []*genx.MessageChunk{
		genx.NewBeginOfStream("audio-rt"),
		{Part: genx.Text("ignored"), Ctrl: &genx.StreamCtrl{StreamID: "audio-rt"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-rt"}},
		{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-rt", EndOfStream: true}},
	}}, asrInput, state, inputStarted)
	if result.err != nil || result.streamID != "audio-rt" || state.Get() != "audio-rt" {
		t.Fatalf("feedRealtimeASRInput() = %+v state=%q", result, state.Get())
	}
	if got := <-inputStarted; got != "audio-rt" {
		t.Fatalf("input started = %q, want audio-rt", got)
	}
	chunks := drainChunks(t, asrInput.Stream())
	if len(chunks) != 2 {
		t.Fatalf("ASR input chunks = %#v, want audio frame and EOS", chunks)
	}
	if blob, ok := chunks[0].Part.(*genx.Blob); !ok || !bytes.Equal(blob.Data, []byte{1}) {
		t.Fatalf("audio chunk = %#v", chunks[0])
	}
	if !chunks[1].IsEndOfStream() {
		t.Fatalf("EOS chunk = %#v", chunks[1])
	}

	errResult := feedRealtimeASRInput(context.Background(), nil, genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1), &lockedString{}, nil)
	if errResult.err == nil || !strings.Contains(errResult.err.Error(), "input stream") {
		t.Fatalf("feedRealtimeASRInput(nil) = %+v", errResult)
	}
}

func TestNormalizeInputModeAndRealtimeTurnStreamID(t *testing.T) {
	for _, in := range []string{"push", "push-to-talk", "ptt"} {
		if got := normalizeInputMode(in); got != inputModePushToTalk {
			t.Fatalf("normalizeInputMode(%q) = %q, want push_to_talk", in, got)
		}
	}
	for _, in := range []string{"realtime", "real_time", "real-time"} {
		if got := normalizeInputMode(in); got != inputModeRealtime {
			t.Fatalf("normalizeInputMode(%q) = %q, want realtime", in, got)
		}
	}
	if got := normalizeInputMode("unknown"); got != "" {
		t.Fatalf("normalizeInputMode(unknown) = %q, want empty", got)
	}
	if got := realtimeTurnStreamID("", 0); got != "audio:rt:1" {
		t.Fatalf("realtimeTurnStreamID(empty,0) = %q", got)
	}
	historyEOS := userAudioHistoryEOSChunk("", "")
	if historyEOS.Ctrl == nil || historyEOS.Ctrl.StreamID != defaultInputStreamID || historyEOS.Ctrl.Label != genx.HistoryUserAudioLabel || !historyEOS.IsEndOfStream() {
		t.Fatalf("userAudioHistoryEOSChunk(empty) = %#v", historyEOS)
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

func toolkitTestContext(t *testing.T, owner string, allowed []string) *agenthost.ToolkitContext {
	t.Helper()
	ctx := context.Background()
	ptr := func(value string) *string { return &value }
	store := &toolkit.Server{Store: kv.NewMemory(nil)}
	tool := toolkit.Tool{
		ID:             "system.music.play",
		Name:           ptr("play_music"),
		Description:    ptr("Play music"),
		Source:         toolkit.ToolSourceBuiltin,
		Enabled:        true,
		OwnerPublicKey: ptr(owner),
		InputSchema:    jsonschema.Schema{Type: "object"},
		Executor: toolkit.ToolExecutor{
			Kind: toolkit.ToolExecutorKindBuiltin,
			Name: ptr("music.play"),
		},
	}
	if _, err := store.PutTool(ctx, tool); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}
	return &agenthost.ToolkitContext{
		Builder:   &toolkit.Builder{Tools: store},
		Executors: toolkit.NewExecutorRegistry(),
		BuildRequest: toolkit.BuildRequest{
			OwnerPublicKey:  owner,
			AllowedToolIDs:  allowed,
			RestrictToolIDs: true,
		},
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

func TestAgentToolkitInvokesToolkit(t *testing.T) {
	ctx := context.Background()
	tools := toolkitTestContext(t, "peer-a", []string{"system.music.play"})
	if err := tools.Executors.Register("music.play", toolkit.ExecutorFunc(func(_ context.Context, call toolkit.Call) (toolkit.Result, error) {
		if call.SubjectID != "peer-a" {
			t.Fatalf("SubjectID = %q, want peer-a", call.SubjectID)
		}
		if call.Tool.ID != "system.music.play" {
			t.Fatalf("Tool.ID = %q, want system.music.play", call.Tool.ID)
		}
		if string(call.Args) != `{"query":"song"}` {
			t.Fatalf("Args = %s, want query song", call.Args)
		}
		return toolkit.Result{Data: json.RawMessage(`{"queued":true}`)}, nil
	})); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	agentToolkit, err := tools.BuildAgentToolkit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := agentToolkit.Invoke(ctx, commonagent.ToolCall{ID: "call-1", Name: "play_music", Arguments: json.RawMessage(`{"query":"song"}`)})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(result.Content) != `{"queued":true}` || result.ID != "call-1" {
		t.Fatalf("Invoke() = %#v, want queued JSON", result)
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

func flowcraftNonHistoryChunks(chunks []*genx.MessageChunk) []*genx.MessageChunk {
	out := chunks[:0]
	for _, chunk := range chunks {
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			continue
		}
		out = append(out, chunk)
	}
	return out
}

func countHistoryAudioChunks(chunks []*genx.MessageChunk, streamID string) int {
	count := 0
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != genx.HistoryUserAudioLabel || chunk.Ctrl.StreamID != streamID {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			count++
		}
	}
	return count
}

func nextChunkWithTimeout(t *testing.T, stream genx.Stream) *genx.MessageChunk {
	t.Helper()
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	done := make(chan result, 1)
	go func() {
		chunk, err := stream.Next()
		done <- result{chunk: chunk, err: err}
	}()
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("Next() error = %v", result.err)
		}
		return result.chunk
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for stream chunk")
		return nil
	}
}

func waitForClawCalls(t *testing.T, claw *interruptClaw, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if claw.Calls() >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("claw calls = %d, want at least %d", claw.Calls(), want)
		}
		time.Sleep(time.Millisecond)
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
			admin: t,
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
	admin  *recordingVoiceTransformer
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
				s.admin.appendText(string(text))
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

type blockingChunkStream struct {
	ch chan *genx.MessageChunk
}

func newBlockingChunkStream() *blockingChunkStream {
	return &blockingChunkStream{ch: make(chan *genx.MessageChunk, 8)}
}

func (s *blockingChunkStream) Push(chunk *genx.MessageChunk) {
	s.ch <- chunk
}

func (s *blockingChunkStream) Next() (*genx.MessageChunk, error) {
	chunk, ok := <-s.ch
	if !ok {
		return nil, genx.ErrDone
	}
	return chunk, nil
}

func (s *blockingChunkStream) Close() error {
	close(s.ch)
	return nil
}

func (s *blockingChunkStream) CloseWithError(error) error {
	close(s.ch)
	return nil
}

type realtimeASRTransformer struct {
	texts  []string
	chunks []*genx.MessageChunk
	err    error
}

func (t *realtimeASRTransformer) Transform(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if pattern != "model/asr?emit_interim=true" {
		return nil, errors.New("unexpected pattern: " + pattern)
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	go func() {
		defer func() { _ = output.Done(genx.Usage{}) }()
		texts := append([]string(nil), t.texts...)
		chunks := append([]*genx.MessageChunk(nil), t.chunks...)
		emittedChunks := false
		for {
			chunk, err := input.Next()
			if err != nil {
				if agenthost.IsStreamDone(err) {
					return
				}
				_ = output.Unexpected(genx.Usage{}, err)
				return
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || len(blob.Data) == 0 || chunk.IsEndOfStream() {
				continue
			}
			if len(chunks) > 0 && !emittedChunks {
				emittedChunks = true
				for _, next := range chunks {
					if err := output.Add(next); err != nil {
						return
					}
				}
				continue
			}
			if t.err != nil {
				_ = output.Unexpected(genx.Usage{}, t.err)
				return
			}
			if len(texts) == 0 {
				continue
			}
			text := texts[0]
			texts = texts[1:]
			streamID := realtimeASRFakeStreamID(chunk, len(t.texts)-len(texts))
			if err := output.Add(
				userAudioHistoryChunk(chunk, streamID),
				userAudioHistoryEOSChunk(streamID, "audio/pcm"),
				&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: transcriptLabel,
					Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: transcriptLabel},
				},
			); err != nil {
				return
			}
		}
	}()
	return output.Stream(), nil
}

func realtimeASRFakeStreamID(chunk *genx.MessageChunk, segment int) string {
	streamID := defaultInputStreamID
	if chunk != nil && chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) != "" {
		streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	if segment <= 1 {
		return streamID
	}
	return fmt.Sprintf("%s:asr:%d", streamID, segment)
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

type memoryHistoryLogStore struct {
	mu         sync.Mutex
	queryCalls int
	queries    []logstore.Query
}

func (*memoryHistoryLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}

func (s *memoryHistoryLogStore) Query(_ context.Context, query logstore.Query) (logstore.Page, error) {
	s.mu.Lock()
	s.queryCalls++
	s.queries = append(s.queries, query)
	s.mu.Unlock()
	return logstore.Page{}, nil
}

type recordingOutputObserver struct {
	chunks     []*genx.MessageChunk
	interrupts []string
}

func (*recordingOutputObserver) BeginOutput(string, string) {}

func (o *recordingOutputObserver) ObserveOutput(chunk *genx.MessageChunk) {
	o.chunks = append(o.chunks, chunk)
}

func (o *recordingOutputObserver) InterruptOutput(streamID string) {
	o.interrupts = append(o.interrupts, streamID)
}

func (*memoryHistoryLogStore) Replace(context.Context, logstore.Record) error {
	return logstore.ErrNotFound
}
func (*memoryHistoryLogStore) Delete(context.Context, logstore.RecordKey) error {
	return logstore.ErrNotFound
}
func (*memoryHistoryLogStore) Close() error { return nil }

type fakeClaw struct {
	events []runtimeEvent
	text   string
}

type inputRecordingClaw struct {
	mu       sync.Mutex
	requests []runtimeRequest
}

func (c *inputRecordingClaw) RoundTrip(req runtimeRequest) (runtimeResponse, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	c.mu.Unlock()
	return &fakeClawResponse{}, nil
}

func (*inputRecordingClaw) CloseContext(context.Context) error { return nil }

func (c *inputRecordingClaw) Requests() []runtimeRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]runtimeRequest(nil), c.requests...)
}

func (c fakeClaw) RoundTrip(req runtimeRequest) (runtimeResponse, error) {
	c.text = req.Text
	return &fakeClawResponse{events: append([]runtimeEvent(nil), c.events...)}, nil
}

func (fakeClaw) CloseContext(context.Context) error { return nil }

type fakeEchoClaw struct {
	nodeID string
	prefix string
}

func (c fakeEchoClaw) RoundTrip(req runtimeRequest) (runtimeResponse, error) {
	nodeID := c.nodeID
	if nodeID == "" {
		nodeID = "answer"
	}
	return &fakeClawResponse{events: []runtimeEvent{
		{Type: runtimeEventToken, NodeID: nodeID, Content: c.prefix + req.Text},
	}}, nil
}

func (fakeEchoClaw) CloseContext(context.Context) error { return nil }

type interruptClaw struct {
	mu    sync.Mutex
	calls int
}

func (c *interruptClaw) RoundTrip(req runtimeRequest) (runtimeResponse, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		return &blockingClawResponse{
			ctx:   req.Context,
			event: runtimeEvent{Type: runtimeEventToken, NodeID: "answer", Content: "旧回复"},
		}, nil
	}
	return &fakeClawResponse{events: []runtimeEvent{
		{Type: runtimeEventToken, NodeID: "answer", Content: "新回复"},
	}}, nil
}

func (c *interruptClaw) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (*interruptClaw) CloseContext(context.Context) error { return nil }

type blockingClawResponse struct {
	ctx  context.Context
	once sync.Once

	event runtimeEvent
	sent  bool
}

func (r *blockingClawResponse) Next() (runtimeEvent, error) {
	if !r.sent {
		r.sent = true
		return r.event, nil
	}
	if r.ctx == nil {
		select {}
	}
	<-r.ctx.Done()
	return runtimeEvent{}, r.ctx.Err()
}

type fakeDebugClaw struct {
	fakeClaw
	handler http.HandlerFunc
}

type debugHistoryResponse struct {
	Enabled  bool                `json:"enabled"`
	Messages []flowmodel.Message `json:"messages,omitempty"`
}

func (c fakeDebugClaw) serveHistory(w http.ResponseWriter, r *http.Request) {
	if c.handler == nil {
		http.NotFound(w, r)
		return
	}
	c.handler(w, r)
}

func (c fakeDebugClaw) History(ctx context.Context, _ int) ([]flowmodel.Message, error) {
	return historyFromTestHandler(ctx, c.serveHistory)
}

type recordingClaw struct {
	mu      sync.Mutex
	events  []runtimeEvent
	texts   []string
	handler http.HandlerFunc
}

func (c *recordingClaw) RoundTrip(req runtimeRequest) (runtimeResponse, error) {
	c.mu.Lock()
	c.texts = append(c.texts, req.Text)
	events := append([]runtimeEvent(nil), c.events...)
	c.mu.Unlock()
	return &fakeClawResponse{events: events}, nil
}

func (*recordingClaw) CloseContext(context.Context) error { return nil }

func (c *recordingClaw) serveHistory(w http.ResponseWriter, r *http.Request) {
	if c.handler == nil {
		http.NotFound(w, r)
		return
	}
	c.handler(w, r)
}

func (c *recordingClaw) History(ctx context.Context, _ int) ([]flowmodel.Message, error) {
	return historyFromTestHandler(ctx, c.serveHistory)
}

func historyFromTestHandler(ctx context.Context, handler http.HandlerFunc) ([]flowmodel.Message, error) {
	req := httptest.NewRequest(http.MethodGet, "/debug/history", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code < http.StatusOK || rec.Code >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("test history status: %s", rec.Result().Status)
	}
	var response debugHistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		return nil, err
	}
	if !response.Enabled {
		return nil, errors.New("flowcraft history is disabled")
	}
	return response.Messages, nil
}

func (c *recordingClaw) Texts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.texts...)
}

type fakeClawResponse struct {
	events []runtimeEvent
}

func (r *fakeClawResponse) Next() (runtimeEvent, error) {
	if len(r.events) == 0 {
		return runtimeEvent{}, io.EOF
	}
	ev := r.events[0]
	r.events = r.events[1:]
	return ev, nil
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
	if got := mergeTranscript("请用一句话回答，收到，并", "请用一句话回答，收到并继续"); got != "请用一句话回答，收到并继续" {
		t.Fatalf("punctuation-insensitive prefix merge = %q", got)
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

func TestVoiceForNodeUsesConfiguredMapBeforeDefault(t *testing.T) {
	a := &agent{defaultVoice: "default", nodeVoices: map[string]string{"answer": "answer-voice"}}
	if got, ok := a.voiceForNode("answer"); !ok || got != "answer-voice" {
		t.Fatalf("answer voice = %q ok=%t, want answer-voice true", got, ok)
	}
	if got, ok := a.voiceForNode("other"); ok || got != "" {
		t.Fatalf("unconfigured node voice = %q ok=%t, want false", got, ok)
	}
	a = &agent{defaultVoice: "default"}
	if got, ok := a.voiceForNode("other"); !ok || got != "default" {
		t.Fatalf("default voice = %q ok=%t, want default true", got, ok)
	}
}

func TestConfigHelperBranches(t *testing.T) {
	if got := mapString(map[string]any{"empty": " ", "name": "  value  "}, "empty", "name"); got != "value" {
		t.Fatalf("mapString() = %q, want value", got)
	}
	if got := firstString(nil, new(" "), new("x")); got != "x" {
		t.Fatalf("firstString() = %q, want x", got)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode test JSON: %v", err)
	}
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

type fakeModels struct {
	events       *[]string
	providerKind apitypes.ModelProviderKind
}

func testFlowcraftWorkflow(spec apitypes.FlowcraftWorkflowSpec) apitypes.Workflow {
	return apitypes.Workflow{
		Name: "test-workflow",
		Spec: apitypes.WorkflowSpec{
			Driver:    apitypes.WorkflowDriverFlowcraft,
			Flowcraft: &spec,
		},
	}
}

func appendEvent(events *[]string, event string) {
	if events != nil {
		*events = append(*events, event)
	}
}

func (f fakeModels) GetModel(_ context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	appendEvent(f.events, "get:model:"+request.Id)
	return adminhttp.GetModel200JSONResponse(f.model(request.Id)), nil
}

func (f fakeModels) ListModels(_ context.Context, request adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	appendEvent(f.events, "list:models")
	if request.Params.Cursor != nil && *request.Params.Cursor != "" {
		return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{}), nil
	}
	return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{
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
	providerData := apitypes.ModelProviderData{}
	switch providerKind {
	case apitypes.ModelProviderKindVolcTenant:
		if err := providerData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{
			UpstreamModel:        new("doubao-lite"),
			ThinkingParam:        new("thinking.type"),
			DefaultThinkingLevel: new("disabled"),
		}); err != nil {
			panic(err)
		}
	default:
		if err := providerData.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{
			UpstreamModel:        new("gpt-test"),
			ThinkingParam:        new("thinking.type"),
			DefaultThinkingLevel: new("disabled"),
		}); err != nil {
			panic(err)
		}
	}
	return apitypes.Model{
		Id:   id,
		Kind: kind,
		Provider: apitypes.ModelProvider{
			Kind: providerKind,
			Name: "main",
		},
		ProviderData: &providerData,
	}
}

type fakeVoices struct {
	events *[]string
}

func (f fakeVoices) GetVoice(_ context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	*f.events = append(*f.events, "get:voice:"+request.Id)
	return adminhttp.GetVoice200JSONResponse(apitypes.Voice{
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

func (f fakeCredentials) GetCredential(_ context.Context, request adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	*f.events = append(*f.events, "get:credential:"+request.Name)
	if request.Name == "volc-key" {
		return adminhttp.GetCredential200JSONResponse(apitypes.Credential{
			Name: request.Name,
			Body: testVolcCredentialBodyFromStrings(map[string]string{
				"speech_api_key": "speech-key",
				"ark_api_key":    "test-key",
			}),
		}), nil
	}
	return adminhttp.GetCredential200JSONResponse(apitypes.Credential{
		Name: request.Name,
		Body: testOpenAICredentialBody("test-key"),
	}), nil
}

type fakeTenants struct {
	events *[]string
}

type fakeMemoryStore struct {
	recall memory.RecallResult
	query  memory.Query
}

func (*fakeMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func (s *fakeMemoryStore) Recall(_ context.Context, query memory.Query) (memory.RecallResult, error) {
	s.query = query
	return s.recall, nil
}

func (*fakeMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}

func (*fakeMemoryStore) Delete(context.Context, memory.DeleteRequest) error { return nil }

func (f fakeTenants) GetOpenAITenant(_ context.Context, request adminhttp.GetOpenAITenantRequestObject) (adminhttp.GetOpenAITenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:openai:"+request.Name)
	baseURL := "https://llm.example/v1"
	return adminhttp.GetOpenAITenant200JSONResponse(apitypes.OpenAITenant{
		Name:           request.Name,
		CredentialName: "openai-key",
		BaseUrl:        &baseURL,
	}), nil
}

func (f fakeTenants) GetGeminiTenant(_ context.Context, request adminhttp.GetGeminiTenantRequestObject) (adminhttp.GetGeminiTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:gemini:"+request.Name)
	return adminhttp.GetGeminiTenant200JSONResponse(apitypes.GeminiTenant{
		Name:           request.Name,
		CredentialName: "gemini-key",
	}), nil
}

func (f fakeTenants) GetDashScopeTenant(context.Context, adminhttp.GetDashScopeTenantRequestObject) (adminhttp.GetDashScopeTenantResponseObject, error) {
	panic("unexpected dashscope tenant lookup")
}

func (f fakeTenants) GetMiniMaxTenant(context.Context, adminhttp.GetMiniMaxTenantRequestObject) (adminhttp.GetMiniMaxTenantResponseObject, error) {
	panic("unexpected minimax tenant lookup")
}

func (f fakeTenants) GetVolcTenant(_ context.Context, request adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	*f.events = append(*f.events, "get:tenant:volc:"+request.Name)
	region := "cn-beijing"
	return adminhttp.GetVolcTenant200JSONResponse(apitypes.VolcTenant{
		Name:           request.Name,
		CredentialName: "volc-key",
		Region:         &region,
	}), nil
}
