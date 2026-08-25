//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	doubaospeech "github.com/GizClaw/doubao-speech-go"
	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoast"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtimeduplex"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
	einotransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	flowcrafttransformer "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/cloudwego/eino/components/model"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const liveInterruptPrompt = "Write a numbered list from 1 to 200. Put every item on its own line and do not summarize or stop early."

func TestEinoTransformerLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	client := openai.NewClient(option.WithAPIKey(firstEnv(einoAPIKeyEnv)))
	generator := &genx.OpenAIGenerator{Client: &client, Model: "gpt-4o-mini", TextOnly: true}
	transformer, err := einotransformer.New(t.Context(), einotransformer.Config{
		Agent:      einotransformer.AgentConfig{ID: "eino-interrupt-e2e", Name: "Eino Interrupt E2E"},
		Components: &einoE2EResolver{models: map[string]model.BaseChatModel{"chat": &genxChatModel{generator: generator}}},
		Limits:     einotransformer.Limits{MaxOutputBytes: 1 << 20},
		Graph: einotransformer.GraphDefinition{
			Name: "interrupt-chat",
			State: einotransformer.StateDefinition{Fields: []einotransformer.StateField{
				{Name: "messages", Type: einotransformer.StateMessages, Merge: einotransformer.MergeReplace},
				{Name: "answer", Type: einotransformer.StateString, Merge: einotransformer.MergeReplace},
			}},
			Nodes: []einotransformer.NodeDefinition{
				{
					ID: "prompt", Inputs: map[string]einotransformer.Binding{"text": {From: "input.text"}},
					Outputs: map[string]string{"messages": "messages"},
					Prompt: &einotransformer.PromptNode{
						Format: einotransformer.PromptFString,
						Messages: []einotransformer.PromptMessage{
							{Role: einotransformer.PromptSystem, Template: "Follow the user request exactly."},
							{Role: einotransformer.PromptUser, Template: "{text}"},
						},
					},
				},
				{
					ID: "chat", Inputs: map[string]einotransformer.Binding{"messages": {From: "messages"}},
					Outputs: map[string]string{"text": "answer"}, ChatModel: &einotransformer.ChatModelNode{Model: "chat"},
				},
			},
			Edges:   []einotransformer.EdgeDefinition{{From: "start", To: "prompt"}, {From: "prompt", To: "chat"}, {From: "chat", To: "end"}},
			Outputs: []einotransformer.OutputDefinition{{Node: "chat", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true}},
		},
	})
	if err != nil {
		t.Fatalf("eino.New() failed: %v", err)
	}
	runLiveTextRepeatedInterrupt(t, transformer, "eino")
}

func TestFlowcraftTransformerLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	client := openai.NewClient(option.WithAPIKey(firstEnv(flowcraftAPIKeyEnv)))
	generator := &genx.OpenAIGenerator{Client: &client, Model: "gpt-4o-mini", TextOnly: true}
	transformer, err := flowcrafttransformer.New(flowcrafttransformer.Config{
		ID: "flowcraft-interrupt-e2e", Name: "Flowcraft Interrupt E2E", Models: generator,
		Graph: flowgraph.GraphDefinition{Name: "interrupt-chat", Entry: "chat", Nodes: []flowgraph.NodeDefinition{{
			ID: "chat", Type: "llm", Config: map[string]any{"model": "chat", "system_prompt": "Follow the user request exactly."},
		}}},
		PublishNodes: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("flowcraft.New() failed: %v", err)
	}
	runLiveTextRepeatedInterrupt(t, transformer, "flowcraft")
}

func TestDoubaoRealtimeLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	client := liveDoubaoClient(t)
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client: client, Model: string(doubaospeech.RealtimeModelO20), Mode: doubaorealtime.ModePushToTalk,
		Instructions: "Reply with a detailed answer so the caller can interrupt you while speaking.", InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}
	runLiveAudioRepeatedInterrupt(t, transformer, "doubao-realtime", true, true, true, 1, false, false)
}

func TestDoubaoRealtimePushToTalkLiveEmptyASRCompletesAndReusesTransformer(t *testing.T) {
	requireLiveDoubaoCredentials(t)
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client:         liveDoubaoClient(t),
		Model:          string(doubaospeech.RealtimeModelO20),
		Mode:           doubaorealtime.ModePushToTalk,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)
	events, outputErrors := collectDuplexOutput(output)

	const silentStreamID = "doubao-realtime-empty-ptt"
	feedDone := make(chan error, 1)
	silentPackets := liveSilentOpusPackets(t, 500*time.Millisecond)
	go func() {
		feedDone <- pushDuplexTurn(ctx, input, silentStreamID, silentPackets)
	}()
	waitLiveEmptyPTTCompletion(t, ctx, events, outputErrors, silentStreamID, feedDone)

	const semanticStreamID = "doubao-realtime-after-empty-ptt"
	feedDone = make(chan error, 1)
	semanticPackets := embeddedPromptOpusPackets(t)
	go func() {
		feedDone <- pushDuplexTurn(ctx, input, semanticStreamID, semanticPackets)
	}()
	result, err := waitDuplexRound(t, ctx, events, outputErrors, semanticStreamID, feedDone)
	if err != nil {
		t.Fatalf("semantic response after empty PTT failed: %v", err)
	}
	assertDuplexRound(t, 1, result)
}

func requireLiveDoubaoCredentials(t *testing.T) {
	t.Helper()
	for _, name := range []string{doubaoAppIDEnv, doubaoAPIKeyEnv} {
		value := strings.TrimSpace(os.Getenv(name))
		lower := strings.ToLower(value)
		if value == "" || strings.Contains(lower, "dummy") ||
			strings.Contains(lower, "example") ||
			strings.Contains(lower, "placeholder") ||
			strings.Contains(lower, "replace") ||
			strings.Contains(lower, "changeme") {
			t.Skipf("set a real %s for the live Doubao provider test", name)
		}
	}
}

func waitLiveEmptyPTTCompletion(
	t *testing.T,
	ctx context.Context,
	events <-chan *genx.MessageChunk,
	errs <-chan error,
	streamID string,
	feedDone <-chan error,
) {
	t.Helper()
	type routeKey struct {
		role     genx.Role
		label    string
		mimeType string
	}
	type routeState struct {
		begun bool
		ended bool
	}
	want := map[routeKey]*routeState{
		{role: genx.RoleUser, label: duplexTranscriptLabel, mimeType: "text/plain"}: {},
		{role: genx.RoleModel, label: duplexAssistantLabel, mimeType: "text/plain"}: {},
		{role: genx.RoleModel, label: duplexAssistantLabel, mimeType: "audio/*"}:    {},
	}
	assistantStreamID := ""
	inputDone := false
	allComplete := func() bool {
		for _, state := range want {
			if !state.begun || !state.ended {
				return false
			}
		}
		return true
	}
	for !inputDone || !allComplete() {
		select {
		case <-ctx.Done():
			t.Fatalf("wait empty PTT completion: %v; routes=%#v", ctx.Err(), want)
		case err := <-feedDone:
			feedDone = nil
			inputDone = true
			if err != nil {
				t.Fatalf("feed empty PTT audio: %v", err)
			}
		case err := <-errs:
			if err != nil {
				t.Fatalf("empty PTT output error: %v", err)
			}
		case chunk, ok := <-events:
			if !ok {
				t.Fatalf("provider output closed before empty PTT terminal; routes=%#v", want)
			}
			if chunk == nil || chunk.Ctrl == nil {
				t.Fatalf("empty PTT emitted an unowned chunk: %#v", chunk)
			}
			if chunk.Ctrl.Label != duplexTranscriptLabel && chunk.Ctrl.Label != duplexAssistantLabel {
				continue
			}
			switch chunk.Ctrl.Label {
			case duplexTranscriptLabel:
				if chunk.Ctrl.StreamID != streamID {
					t.Fatalf("empty PTT transcript StreamID = %q, want %q: %#v", chunk.Ctrl.StreamID, streamID, chunk)
				}
			case duplexAssistantLabel:
				if strings.TrimSpace(chunk.Ctrl.StreamID) == "" {
					t.Fatalf("empty PTT assistant route has no StreamID: %#v", chunk)
				}
				if assistantStreamID == "" && chunk.IsBeginOfStream() {
					assistantStreamID = chunk.Ctrl.StreamID
				}
				if chunk.Ctrl.StreamID != assistantStreamID {
					t.Fatalf("empty PTT assistant StreamID changed from %q to %q: %#v", assistantStreamID, chunk.Ctrl.StreamID, chunk)
				}
			}
			if chunk.Ctrl.Error != "" || routeChunkHasData(chunk) {
				t.Fatalf("empty PTT emitted data or error: %#v", chunk)
			}
			mimeType, ok := chunk.MIMEType()
			if !ok {
				t.Fatalf("empty PTT route has no MIME type: %#v", chunk)
			}
			if strings.HasPrefix(mimeType, "audio/") {
				mimeType = "audio/*"
			}
			key := routeKey{role: chunk.Role, label: chunk.Ctrl.Label, mimeType: mimeType}
			state := want[key]
			if state == nil {
				t.Fatalf("unexpected empty PTT route: %#v", chunk)
			}
			if chunk.IsBeginOfStream() {
				if state.begun || state.ended {
					t.Fatalf("duplicate empty PTT route BOS: %#v", chunk)
				}
				state.begun = true
			} else if !state.begun {
				t.Fatalf("empty PTT route emitted EOS before BOS: %#v", chunk)
			}
			if chunk.IsEndOfStream() {
				if state.ended {
					t.Fatalf("duplicate empty PTT route EOS: %#v", chunk)
				}
				state.ended = true
			}
		}
	}
}

func liveSilentOpusPackets(t *testing.T, duration time.Duration) [][]byte {
	t.Helper()
	const sampleRate = 16000
	const frameDuration = 20 * time.Millisecond
	frameSize := sampleRate / 50
	encoder, err := opus.NewEncoder(sampleRate, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("create Opus silence encoder: %v", err)
	}
	defer func() { _ = encoder.Close() }()
	frame := make([]int16, frameSize)
	packetCount := int((duration + frameDuration - 1) / frameDuration)
	packets := make([][]byte, 0, packetCount)
	for range packetCount {
		packet, err := encoder.Encode(frame, frameSize)
		if err != nil {
			t.Fatalf("encode Opus silence: %v", err)
		}
		packets = append(packets, packet)
	}
	return packets
}

func TestDoubaoRealtimeModeRealtimeLiveNaturalCompletion(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client:         liveDoubaoClient(t),
		Model:          string(doubaospeech.RealtimeModelO20),
		Mode:           doubaorealtime.ModeRealtime,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}
	runLiveDoubaoModeRealtimeNaturalCompletion(t, transformer)
}

func TestDoubaoRealtimeModeRealtimeLiveClientSilence(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client:         liveDoubaoClient(t),
		Model:          string(doubaospeech.RealtimeModelO20),
		Mode:           doubaorealtime.ModeRealtime,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}
	runLiveRealtimeClientSilence(t, transformer, "doubao-realtime-client-silence")
}

func TestDoubaoRealtimeModeRealtimeLiveConcurrentClientSilence(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client:         liveDoubaoClient(t),
		Model:          string(doubaospeech.RealtimeModelO20),
		Mode:           doubaorealtime.ModeRealtime,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}
	runLiveRealtimeClientSilenceConcurrent(t, transformer, "doubao-realtime-concurrent", 10)
}

func TestDoubaoRealtimeModeRealtimeLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	transformer := newLiveDoubaoRealtimeModeRealtimeTransformer(t)
	// Keep the outer Transformer input open across all three rounds. Each new
	// input BOS must close both assistant MIME routes from the previous round
	// before any replacement assistant BOS becomes visible.
	runLiveAudioRepeatedInterrupt(t, transformer, "doubao-realtime-mode-realtime", true, true, true, 1, true, true)
}

func TestDoubaoRealtimeModeRealtimeLiveRepeatedInterruptConcurrency10(t *testing.T) {
	loadGenXE2EEnv(t)
	for lane := 1; lane <= 10; lane++ {
		t.Run(fmt.Sprintf("lane-%02d", lane), func(t *testing.T) {
			t.Parallel()
			transformer := newLiveDoubaoRealtimeModeRealtimeTransformer(t)
			runLiveAudioRepeatedInterrupt(
				t,
				transformer,
				fmt.Sprintf("doubao-realtime-mode-realtime-lane-%02d", lane),
				true,
				true,
				true,
				1,
				true,
				true,
			)
		})
	}
}

func newLiveDoubaoRealtimeModeRealtimeTransformer(t *testing.T) genx.Transformer {
	t.Helper()
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client:         liveDoubaoClient(t),
		Model:          string(doubaospeech.RealtimeModelO20),
		Mode:           doubaorealtime.ModeRealtime,
		Instructions:   "Reply with a detailed answer so the caller can interrupt you while speaking.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}
	return transformer
}

func runLiveDoubaoModeRealtimeNaturalCompletion(t *testing.T, transformer genx.Transformer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	events, outputErrors := collectDuplexOutput(output)
	const streamID = "doubao-realtime-natural"
	feedDone := make(chan error, 1)
	packets := embeddedPromptOpusPackets(t)
	go func() {
		feedDone <- pushDuplexTurn(ctx, input, streamID, packets)
	}()
	result, err := waitDuplexRound(t, ctx, events, outputErrors, streamID, feedDone)
	if err != nil {
		if result.terminalComplete() {
			result.lifecycles.assertComplete(t)
		}
		t.Fatalf("ModeRealtime natural completion: %v", err)
	}
	assertDuplexRound(t, 1, result)
	t.Logf(
		"ModeRealtime transcript=%q assistant=%q assistant_audio_bytes=%d",
		result.transcript.String(), result.assistantText.String(), result.assistantAudioBytes,
	)
}

func TestDoubaoRealtimeDuplexLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
		Client:         liveDoubaoClient(t),
		Model:          doubaospeech.RealtimeDuplexModelDefault,
		InputTranscode: &transcode,
		Instructions:   "Reply with a detailed answer so the caller can interrupt you while speaking.",
	})
	if err != nil {
		t.Fatalf("doubaorealtimeduplex.New() failed: %v", err)
	}
	runLiveAudioRepeatedInterrupt(t, transformer, "doubao-duplex", true, true, true, 1, false, false)
}

func TestDoubaoRealtimeDuplexLiveOpenInputRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
		Client:         liveDoubaoClient(t),
		Model:          "1.2.6.1",
		InputTranscode: &transcode,
		Instructions:   "Reply with a detailed answer so the caller can interrupt you while speaking.",
	})
	if err != nil {
		t.Fatalf("doubaorealtimeduplex.New() failed: %v", err)
	}
	runLiveAudioRepeatedInterrupt(t, transformer, "doubao-duplex-open", true, true, true, 1, true, true)
}

func TestDoubaoRealtimeDuplexLiveOpenInputRepeatedInterruptConcurrency10(t *testing.T) {
	loadGenXE2EEnv(t)
	for lane := 1; lane <= 10; lane++ {
		lane := lane
		t.Run(fmt.Sprintf("lane-%02d", lane), func(t *testing.T) {
			t.Parallel()
			transcode := false
			transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
				Client:         liveDoubaoClient(t),
				Model:          "1.2.6.1",
				InputTranscode: &transcode,
				Instructions:   "Reply with a detailed answer so the caller can interrupt you while speaking.",
			})
			if err != nil {
				t.Fatalf("doubaorealtimeduplex.New() failed: %v", err)
			}
			runLiveAudioRepeatedInterrupt(
				t,
				transformer,
				fmt.Sprintf("doubao-duplex-open-lane-%02d", lane),
				true,
				true,
				true,
				1,
				true,
				true,
			)
		})
	}
}

func TestDoubaoRealtimeDuplexLiveClientSilence(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
		Client:         liveDoubaoClient(t),
		Model:          "1.2.6.1",
		InputTranscode: &transcode,
		Instructions:   "Reply in one short English sentence.",
	})
	if err != nil {
		t.Fatalf("doubaorealtimeduplex.New() failed: %v", err)
	}
	runLiveRealtimeClientSilence(t, transformer, "doubao-realtime-duplex-client-silence")
}

func TestDoubaoRealtimeDuplexLiveConcurrentClientSilence(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
		Client:         liveDoubaoClient(t),
		Model:          "1.2.6.1",
		InputTranscode: &transcode,
		Instructions:   "Reply in one short English sentence.",
	})
	if err != nil {
		t.Fatalf("doubaorealtimeduplex.New() failed: %v", err)
	}
	runLiveRealtimeClientSilenceConcurrent(t, transformer, "doubao-realtime-duplex-concurrent", 10)
}

func runLiveRealtimeClientSilence(t *testing.T, transformer genx.Transformer, streamID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	events, outputErrors := collectDuplexOutput(output)
	feedDone := make(chan error, 1)
	packets := embeddedPromptOpusPackets(t)
	go func() {
		feedDone <- pushRealtimeSpeechThenIdle(ctx, input, streamID, packets)
	}()
	result, err := waitDuplexRound(t, ctx, events, outputErrors, streamID, feedDone)
	if err != nil {
		if result.terminalComplete() {
			result.lifecycles.assertComplete(t)
		}
		t.Fatalf("client-silence response failed: %v", err)
	}
	assertDuplexRound(t, 1, result)
	const providerIdleWindow = 70 * time.Second
	assertNoRealtimeOutput(t, ctx, events, outputErrors, providerIdleWindow)

	nextStreamID := streamID + "-after-idle"
	feedDone = make(chan error, 1)
	go func() {
		feedDone <- pushRealtimeSpeechThenIdle(ctx, input, nextStreamID, packets)
	}()
	nextResult, err := waitDuplexRound(t, ctx, events, outputErrors, nextStreamID, feedDone)
	if err != nil {
		t.Fatalf("response after %s provider idle failed: %v", providerIdleWindow, err)
	}
	assertDuplexRound(t, 2, nextResult)
}

func runLiveRealtimeClientSilenceConcurrent(
	t *testing.T,
	transformer genx.Transformer,
	prefix string,
	concurrency int,
) {
	t.Helper()
	for lane := 1; lane <= concurrency; lane++ {
		lane := lane
		t.Run(fmt.Sprintf("lane-%02d", lane), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
			defer input.CloseWithError(context.Canceled)
			output, err := transformer.Transform(ctx, input)
			if err != nil {
				t.Fatalf("Transform() failed: %v", err)
			}
			defer output.CloseWithError(context.Canceled)

			events, outputErrors := collectDuplexOutput(output)
			streamID := fmt.Sprintf("%s-%02d", prefix, lane)
			feedDone := make(chan error, 1)
			packets := embeddedPromptOpusPackets(t)
			go func() {
				feedDone <- pushRealtimeSpeechThenIdle(ctx, input, streamID, packets)
			}()
			result, err := waitDuplexRound(t, ctx, events, outputErrors, streamID, feedDone)
			if err != nil {
				t.Fatalf("concurrent client-silence response failed: %v", err)
			}
			assertDuplexRound(t, 1, result)
			assertNoRealtimeOutput(t, ctx, events, outputErrors, 5*time.Second)
		})
	}
}

func assertNoRealtimeOutput(
	t *testing.T,
	ctx context.Context,
	events <-chan *genx.MessageChunk,
	errs <-chan error,
	duration time.Duration,
) {
	t.Helper()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatalf("wait for %s provider idle: %v", duration, ctx.Err())
	case err := <-errs:
		if err == nil {
			t.Fatal("provider error channel returned nil")
		}
		t.Fatalf("provider failed while client audio was idle: %v", err)
	case chunk, ok := <-events:
		if !ok {
			t.Fatal("provider output closed while client audio was idle")
		}
		t.Fatalf("provider emitted an unexpected event while client audio was idle: %#v", chunk)
	case <-timer.C:
	}
}

func pushRealtimeSpeechThenIdle(
	ctx context.Context,
	input *genx.RealtimeStream,
	streamID string,
	packets [][]byte,
) error {
	chunks := duplexTurnInputChunks(streamID, packets)
	for _, chunk := range chunks[:len(chunks)-2] {
		if err := input.Push(ctx, chunk); err != nil {
			return err
		}
		blob, hasAudio := chunk.Part.(*genx.Blob)
		if !hasAudio || len(blob.Data) == 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil
}

func TestDashScopeRealtimeLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(dashScopeAPIKeyEnv)
	if apiKey == "" {
		t.Fatalf("set %s in tests/genx-e2e/.env", dashScopeAPIKeyEnv)
	}
	transformer, err := dashscoperealtime.New(dashscoperealtime.Config{
		Client: dashscope.NewClient(apiKey), Model: dashscope.ModelQwen35OmniPlusRealtime,
		VAD:          dashscope.VADModeDisabled,
		Instructions: "Reply with a detailed answer so the caller can interrupt you while speaking.",
	})
	if err != nil {
		t.Fatalf("dashscoperealtime.New() failed: %v", err)
	}
	runLiveAudioRepeatedInterrupt(t, transformer, "dashscope", false, false, true, 1, false, false)
}

func TestASTTranslateLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	client := liveDoubaoClient(t)
	pacing := false
	ast, err := doubaoast.New(doubaoast.Config{
		Client: client, Mode: doubaospeech.ASTTranslateModeS2T, InputMode: doubaoast.InputModePushToTalk,
		SourceLanguage: "zhen", TargetLanguage: "zhen", SourceLanguageDetect: true, RealtimePacing: &pacing,
	})
	if err != nil {
		t.Fatalf("doubaoast.New() failed: %v", err)
	}
	tts, err := doubaotts.NewSeedV2(doubaotts.SeedV2Config{
		Client: client, Speaker: "zh_female_xiaohe_uranus_bigtts",
	})
	if err != nil {
		t.Fatalf("doubaotts.NewSeedV2() failed: %v", err)
	}
	mux := transformers.NewMux()
	if err := mux.Handle("model/#", ast); err != nil {
		t.Fatalf("register AST: %v", err)
	}
	if err := mux.Handle("voice/#", tts); err != nil {
		t.Fatalf("register TTS: %v", err)
	}
	transformer, err := asttranslate.New(asttranslate.Config{
		Transformer: mux, Model: "live-ast", Params: map[string]any{"lang_pair": "auto"}, ExternalVoice: "live-voice",
	})
	if err != nil {
		t.Fatalf("asttranslate.New() failed: %v", err)
	}
	runLiveAudioRepeatedInterrupt(t, transformer, "ast-translate", true, true, true, 1, false, false)
}

func liveDoubaoClient(t *testing.T) *doubaospeech.Client {
	t.Helper()
	appID := firstEnv(doubaoAppIDEnv)
	apiKey := firstEnv(doubaoAPIKeyEnv)
	if appID == "" || apiKey == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}
	return doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey))
}

func runLiveAudioRepeatedInterrupt(
	t *testing.T,
	transformer genx.Transformer,
	prefix string,
	paced, explicitRoute, inputEOS bool,
	packetRepeats int,
	consumeThroughMixer bool,
	keepRealtimeInputOpen bool,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	providerOutput, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer providerOutput.CloseWithError(context.Canceled)
	output := providerOutput
	if consumeThroughMixer {
		output = consumeLiveOutputThroughMixer(t, ctx, providerOutput)
	}
	defer output.CloseWithError(context.Canceled)

	tracker := newRouteLifecycleTracker()
	terminalErrors := make(map[routeLifecycleKey]string)
	media := make(map[string]map[string]bool)
	seenResponses := make(map[string]bool)
	packets := embeddedPromptOpusPackets(t)
	if packetRepeats > 1 {
		repeated := make([][]byte, 0, len(packets)*packetRepeats)
		for range packetRepeats {
			repeated = append(repeated, packets...)
		}
		packets = repeated
	}
	var feedDone <-chan error
	var cancelFeed context.CancelFunc
	startTurn := func(round int) {
		t.Helper()
		streamID := fmt.Sprintf("%s-input-%d", prefix, round)
		chunks := duplexTurnInputChunks(streamID, packets)
		if keepRealtimeInputOpen {
			chunks = make([]*genx.MessageChunk, 0, len(packets)+1)
			chunks = append(chunks, &genx.MessageChunk{
				Part: &genx.Blob{MIMEType: duplexInputMIME},
				Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
			})
			for _, packet := range packets {
				chunks = append(chunks, &genx.MessageChunk{
					Part: &genx.Blob{MIMEType: duplexInputMIME, Data: append([]byte(nil), packet...)},
					Ctrl: &genx.StreamCtrl{StreamID: streamID},
				})
			}
		} else if !inputEOS && round < 3 {
			chunks = chunks[:len(chunks)-1]
		}
		if !keepRealtimeInputOpen && !explicitRoute {
			chunks = make([]*genx.MessageChunk, 0, len(packets))
			for index, packet := range packets {
				chunks = append(chunks, &genx.MessageChunk{
					Role: genx.RoleUser,
					Part: &genx.Blob{MIMEType: duplexInputMIME, Data: append([]byte(nil), packet...)},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: index == 0, EndOfStream: index == len(packets)-1},
				})
			}
		}
		if err := input.Push(ctx, chunks[0]); err != nil {
			t.Fatalf("push round %d BOS: %v", round, err)
		}
		// Push() queues the control boundary. Give the Transformer input loop a
		// scheduling window to observe barge-in before reading queued provider EOS.
		time.Sleep(20 * time.Millisecond)
		feedCtx, cancel := context.WithCancel(ctx)
		cancelFeed = cancel
		done := make(chan error, 1)
		feedDone = done
		go func() {
			for _, chunk := range chunks[1:] {
				if err := input.Push(feedCtx, chunk); err != nil {
					done <- err
					return
				}
				if paced && routeChunkHasData(chunk) {
					select {
					case <-feedCtx.Done():
						done <- feedCtx.Err()
						return
					case <-time.After(20 * time.Millisecond):
					}
				}
			}
			done <- nil
		}()
	}
	observe := func(chunk *genx.MessageChunk) {
		t.Helper()
		observeRouteLifecycle(t, tracker, chunk)
		if chunk == nil || chunk.Ctrl == nil || chunk.Role != genx.RoleModel || chunk.Part == nil {
			return
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			return
		}
		key := routeLifecycleKey{streamID: strings.TrimSpace(chunk.Ctrl.StreamID), mimeType: mimeType}
		if chunk.IsEndOfStream() {
			terminalErrors[key] = chunk.Ctrl.Error
		}
		if !routeChunkHasData(chunk) {
			return
		}
		kind := ""
		switch {
		case strings.HasPrefix(mimeType, "text/"):
			kind = "text"
		case strings.HasPrefix(mimeType, "audio/"):
			kind = "audio"
		}
		if kind != "" {
			if media[key.streamID] == nil {
				media[key.streamID] = make(map[string]bool)
			}
			media[key.streamID][kind] = true
		}
	}
	next := func() *genx.MessageChunk {
		t.Helper()
		chunk, err := output.Next()
		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("wait live audio output: %v", ctx.Err())
			}
			t.Fatalf("live audio output closed before three rounds completed: %v", err)
		}
		observe(chunk)
		return chunk
	}
	waitResponse := func() string {
		t.Helper()
		for {
			next()
			textID := ""
			audioID := ""
			for streamID, got := range media {
				if seenResponses[streamID] {
					continue
				}
				if got["text"] && textID == "" {
					textID = streamID
				}
				if got["audio"] && audioID == "" {
					audioID = streamID
				}
			}
			if textID != "" && audioID != "" {
				seenResponses[textID] = true
				seenResponses[audioID] = true
				return audioID
			}
		}
	}
	waitFeed := func(interrupt bool) {
		t.Helper()
		if feedDone == nil {
			return
		}
		if interrupt && cancelFeed != nil {
			cancelFeed()
		}
		select {
		case err := <-feedDone:
			feedDone = nil
			cancelFeed = nil
			if err != nil && !(interrupt && errors.Is(err, context.Canceled)) {
				t.Fatalf("feed live audio: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("wait live audio feed: %v", ctx.Err())
		}
	}

	responseIDs := make([]string, 0, 3)
	startTurn(1)
	for round := 1; round <= 3; round++ {
		responseID := waitResponse()
		responseIDs = append(responseIDs, responseID)
		if round == 3 {
			break
		}
		open := make(map[routeLifecycleKey]bool)
		for key, state := range tracker.routes {
			if state.role == genx.RoleModel && state.begun && !state.ended {
				open[key] = true
			}
		}
		if len(open) == 0 {
			t.Fatalf("round %d response %q completed before it could be interrupted", round, responseID)
		}
		oldStreamIDs := make(map[string]bool)
		for key := range open {
			oldStreamIDs[key.streamID] = true
		}
		waitFeed(!inputEOS)
		startTurn(round + 1)
		interruptedRoutes := 0
		for len(open) != 0 {
			chunk := next()
			if chunk == nil || chunk.Ctrl == nil {
				continue
			}
			if chunk.Role == genx.RoleModel && chunk.IsBeginOfStream() {
				streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
				if !oldStreamIDs[streamID] {
					t.Fatalf("round %d replacement BOS %q arrived before prior model routes closed: %#v", round, streamID, open)
				}
			}
			for key := range open {
				state := tracker.routes[key]
				if state != nil && state.ended {
					if terminalErrors[key] == "interrupted" {
						interruptedRoutes++
					}
					delete(open, key)
				}
			}
		}
		if interruptedRoutes == 0 {
			t.Fatalf("round %d closed all prior routes without an interrupted EOS", round)
		}
	}
	waitFeed(false)
	if err := input.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	for !tracker.allComplete() {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			break
		}
		if err != nil {
			t.Fatalf("drain live audio output: %v", err)
		}
		observe(chunk)
	}
	if err := output.CloseWithError(context.Canceled); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("close live audio output: %v", err)
	}
	tracker.assertComplete(t)
	if len(responseIDs) != 3 || responseIDs[0] == responseIDs[1] || responseIDs[1] == responseIDs[2] || responseIDs[0] == responseIDs[2] {
		t.Fatalf("assistant response IDs = %#v, want three unique IDs", responseIDs)
	}
	t.Logf("responses=%q routes=%d", responseIDs, len(tracker.routes))
}

type liveMixerTrackCreator struct {
	mixer *pcm.Mixer
}

func (c liveMixerTrackCreator) CreateAudioTrack(opts ...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	return c.mixer.CreateTrack(opts...)
}

func consumeLiveOutputThroughMixer(t *testing.T, ctx context.Context, providerOutput genx.Stream) genx.Stream {
	t.Helper()
	mixer := pcm.NewMixer(pcm.L16Mono16K)
	observed := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	var closeOnce sync.Once
	closeObserved := func(err error) {
		closeOnce.Do(func() {
			if err != nil {
				_ = observed.CloseWithError(err)
				return
			}
			_ = observed.Close()
		})
	}

	consumeDone := make(chan error, 1)
	go func() {
		err := (agenthost.MixerOutput{
			Tracks:            liveMixerTrackCreator{mixer: mixer},
			WaitForAudioDrain: true,
			Observe: func(chunk *genx.MessageChunk) error {
				return observed.Push(ctx, chunk)
			},
		}).ConsumeAgentOutput(ctx, providerOutput)
		closeObserved(err)
		consumeDone <- err
	}()

	drainDone := make(chan error, 1)
	go func() {
		frame := make([]byte, mixer.Output().BytesInDuration(20*time.Millisecond))
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				drainDone <- ctx.Err()
				return
			case <-ticker.C:
			}
			if _, err := mixer.Read(frame); err != nil {
				drainDone <- err
				return
			}
		}
	}()

	t.Cleanup(func() {
		_ = mixer.Close()
		select {
		case err := <-consumeDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("MixerOutput.ConsumeAgentOutput() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("MixerOutput.ConsumeAgentOutput() did not stop")
		}
		select {
		case err := <-drainDone:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("drain live mixer error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("live mixer drain did not stop")
		}
	})
	return observed
}

func runLiveTextRepeatedInterrupt(t *testing.T, transformer genx.Transformer, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	tracker := newRouteLifecycleTracker()
	ended := make(map[string]string)
	pushTurn := func(round int) {
		t.Helper()
		streamID := fmt.Sprintf("%s-interrupt-input-%d", prefix, round)
		for _, chunk := range completeTextRoute(genx.RoleUser, "", "", streamID, liveInterruptPrompt) {
			if err := input.Push(ctx, chunk); err != nil {
				t.Fatalf("push round %d: %v", round, err)
			}
		}
	}
	nextChunk := func() *genx.MessageChunk {
		t.Helper()
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("read live interrupt output: %v", err)
		}
		observeRouteLifecycle(t, tracker, chunk)
		if chunk.Ctrl != nil && chunk.IsEndOfStream() {
			ended[chunk.Ctrl.StreamID] = chunk.Ctrl.Error
		}
		return chunk
	}
	waitResponseData := func(previousID string) string {
		t.Helper()
		for {
			chunk := nextChunk()
			if chunk.Ctrl == nil || chunk.Role != genx.RoleModel {
				continue
			}
			streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
			if previousID != "" && streamID != previousID && chunk.IsBeginOfStream() {
				if errorText, ok := ended[previousID]; !ok || errorText != "interrupted" {
					t.Fatalf("replacement BOS %q arrived before %q interrupted EOS; ended=%#v", streamID, previousID, ended)
				}
			}
			text, ok := chunk.Part.(genx.Text)
			if !ok || chunk.IsEndOfStream() || strings.TrimSpace(string(text)) == "" || streamID == previousID {
				continue
			}
			if previousID != "" {
				if errorText, ok := ended[previousID]; !ok || errorText != "interrupted" {
					t.Fatalf("response %q produced data before %q interrupted EOS; ended=%#v", streamID, previousID, ended)
				}
			}
			return streamID
		}
	}

	pushTurn(1)
	firstID := waitResponseData("")
	pushTurn(2)
	secondID := waitResponseData(firstID)
	pushTurn(3)
	thirdID := waitResponseData(secondID)
	if firstID == secondID || secondID == thirdID || firstID == thirdID {
		t.Fatalf("assistant response IDs are not unique: %q %q %q", firstID, secondID, thirdID)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			break
		}
		if err != nil {
			t.Fatalf("drain output: %v", err)
		}
		observeRouteLifecycle(t, tracker, chunk)
		if chunk.Ctrl != nil && chunk.IsEndOfStream() {
			ended[chunk.Ctrl.StreamID] = chunk.Ctrl.Error
		}
	}
	tracker.assertComplete(t)
	if ended[firstID] != "interrupted" || ended[secondID] != "interrupted" {
		t.Fatalf("interrupted response errors = %q/%q, want interrupted/interrupted", ended[firstID], ended[secondID])
	}
	if _, ok := ended[thirdID]; !ok {
		t.Fatalf("final response %q did not close", thirdID)
	}
	t.Logf("responses=%q,%q,%q routes=%d", firstID, secondID, thirdID, len(tracker.routes))
}
