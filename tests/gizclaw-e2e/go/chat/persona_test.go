//go:build gizclaw_e2e

package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func TestOpusPacketsFromOggSkipsHeaders(t *testing.T) {
	var buf bytes.Buffer
	writer, err := ogg.NewStreamWriter(&buf, 123)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	for _, packet := range [][]byte{
		[]byte("OpusHeadxxxx"),
		[]byte("OpusTagsyyyy"),
		{0x11, 0x22, 0x33},
		{0x44, 0x55},
	} {
		if _, err := writer.WritePacket(packet, 960, false); err != nil {
			t.Fatalf("WritePacket: %v", err)
		}
	}
	packets, err := opusPacketsFromOgg(buf.Bytes())
	if err != nil {
		t.Fatalf("opusPacketsFromOgg() error = %v", err)
	}
	if len(packets) != 2 {
		t.Fatalf("packets len = %d, want 2", len(packets))
	}
	if !bytes.Equal(packets[0], []byte{0x11, 0x22, 0x33}) || !bytes.Equal(packets[1], []byte{0x44, 0x55}) {
		t.Fatalf("packets = %#v", packets)
	}
}

func TestRoundtripUtteranceWrapsIndex(t *testing.T) {
	first := roundtripUtterance(1)
	if first == "" || roundtripUtterance(0) != first || roundtripUtterance(4) != first {
		t.Fatalf("roundtrip utterance wrap = %q/%q/%q", first, roundtripUtterance(0), roundtripUtterance(4))
	}
}

func TestUseRoundtripUtterancesPrefersConfiguredUtterances(t *testing.T) {
	driver := &personaDriver{cfg: config{Utterances: []string{"今天天气怎么样", "你好测试"}}}
	driver.useRoundtripUtterances()
	first, err := driver.generateUtterance(context.Background(), 1)
	if err != nil {
		t.Fatalf("generate first utterance: %v", err)
	}
	third, err := driver.generateUtterance(context.Background(), 3)
	if err != nil {
		t.Fatalf("generate third utterance: %v", err)
	}
	if first != "今天天气怎么样" || third != "今天天气怎么样" {
		t.Fatalf("configured utterances = %q/%q", first, third)
	}
}

func TestOpusPacketsFromOggErrors(t *testing.T) {
	if _, err := opusPacketsFromOgg([]byte("not ogg")); err == nil {
		t.Fatal("invalid ogg succeeded")
	}
	var buf bytes.Buffer
	writer, err := ogg.NewStreamWriter(&buf, 123)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	if _, err := writer.WritePacket([]byte("OpusHeadxxxx"), 0, false); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if _, err := writer.WritePacket([]byte("OpusTagsyyyy"), 0, false); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	if _, err := opusPacketsFromOgg(buf.Bytes()); err == nil {
		t.Fatal("header-only ogg succeeded")
	}
}

func TestTextHelpers(t *testing.T) {
	if got := cleanUtterance(" “你好\n测试” "); got != "你好测试" {
		t.Fatalf("cleanUtterance() = %q", got)
	}
	if got := cleanAssistantSpokenText(`你是3号平民，轮到你发言。<seed:_tool_call><function werewolf><parameter name="event_type">night</parameter></function>`); got != "你是3号平民，轮到你发言。" {
		t.Fatalf("cleanAssistantSpokenText(tool call) = %q", got)
	}
	if got := cleanAssistantSpokenText(`<node id="answer">不要念出来</node>请继续。`); got != "请继续。" {
		t.Fatalf("cleanAssistantSpokenText(xml block) = %q", got)
	}
	if got := normalizeTranscript("A-猫，12!"); got != "a猫12" {
		t.Fatalf("normalizeTranscript() = %q", got)
	}
	if err := assertTextSimilar("same", "你好测试", "你好，测试。", 1); err != nil {
		t.Fatalf("assertTextSimilar() error = %v", err)
	}
	if err := assertTextSimilar("contained", "第一段自动切分测试", "the third paragraph 第一段自动切分测试。", 1); err != nil {
		t.Fatalf("assertTextSimilar(contained) error = %v", err)
	}
	if err := assertTextSimilar("auto split short replay asr", "第二段自动切分测试", "第二段。", 0.30); err != nil {
		t.Fatalf("assertTextSimilar(auto split short replay asr) error = %v", err)
	}
	if err := assertTextSimilar("different", "你好测试", "天气不错", 0.9); err == nil {
		t.Fatal("assertTextSimilar() succeeded for unrelated text")
	}
	if got := eventLabel(apitypes.PeerStreamEvent{}); got != "" {
		t.Fatalf("eventLabel(nil) = %q", got)
	}
	label := " Assistant "
	if got := eventLabel(apitypes.PeerStreamEvent{Label: &label}); got != "assistant" {
		t.Fatalf("eventLabel() = %q", got)
	}
	if got := runeCount("猫a"); got != 2 {
		t.Fatalf("runeCount() = %d", got)
	}
	if got := mergeTranscriptText("你好能听到我说话吗", "你好，能听到我说话吗？"); got != "你好，能听到我说话吗？" {
		t.Fatalf("mergeTranscriptText(punctuated replacement) = %q", got)
	}
	if got := mergeTranscriptText("嗯今天天气怎么样我想出门走走", "今天天气怎么样？我想出门走走。"); got != "今天天气怎么样？我想出门走走。" {
		t.Fatalf("mergeTranscriptText(asr replacement) = %q", got)
	}
	if got := mergeTranscriptText("你好", "世界"); got != "你好世界" {
		t.Fatalf("mergeTranscriptText(delta) = %q", got)
	}
	line := encodeJSONLine(map[string]string{"a": "b"})
	if !strings.Contains(line, `"a":"b"`) {
		t.Fatalf("encodeJSONLine() = %q", line)
	}
}

func TestWaitFlowcraftHistoryProgressAcceptsCappedContentChange(t *testing.T) {
	oldItems := testHistoryEntries("旧回复一", "旧回复二")
	newItems := testHistoryEntries("旧回复一", "新回复二")
	driver := &personaDriver{
		cfg: config{
			Agent: "flowcraft",
		},
		runtimeClient: &fakeRunControl{
			history: &rpcapi.ServerListRunWorkspaceHistoryResponse{
				Available: true,
				Items:     newItems,
			},
		},
		runtimeHistoryItems: len(oldItems),
		runtimeHistorySig:   flowcraftHistoryProgressSignature(oldItems),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := driver.waitFlowcraftHistoryProgress(ctx, "capped history"); err != nil {
		t.Fatalf("waitFlowcraftHistoryProgress() error = %v", err)
	}
	if driver.runtimeHistoryItems != len(newItems) {
		t.Fatalf("runtimeHistoryItems = %d, want %d", driver.runtimeHistoryItems, len(newItems))
	}
	if got, want := driver.runtimeHistorySig, flowcraftHistoryProgressSignature(newItems); got != want {
		t.Fatalf("runtimeHistorySig = %q, want %q", got, want)
	}
}

func TestWaitFlowcraftHistoryProgressAllowsAvailableHistoryWithoutAdvance(t *testing.T) {
	items := testHistoryEntries("已有回复")
	driver := &personaDriver{
		cfg: config{
			Agent:     "flowcraft",
			Workspace: "history-stable",
		},
		runtimeClient: &fakeRunControl{
			history: &rpcapi.ServerListRunWorkspaceHistoryResponse{
				Available: true,
				Items:     items,
			},
		},
		runtimeHistoryItems: len(items),
		runtimeHistorySig:   flowcraftHistoryProgressSignature(items),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := driver.waitFlowcraftHistoryProgress(ctx, "stable history"); err != nil {
		t.Fatalf("waitFlowcraftHistoryProgress() error = %v", err)
	}
}

func TestWaitFlowcraftHistoryProgressRejectsShorterHistoryWithoutAdvance(t *testing.T) {
	items := testHistoryEntries("旧回复")
	driver := &personaDriver{
		cfg: config{
			Agent:     "flowcraft",
			Workspace: "history-shorter",
		},
		runtimeClient: &fakeRunControl{
			history: &rpcapi.ServerListRunWorkspaceHistoryResponse{
				Available: true,
				Items:     items,
			},
		},
		runtimeHistoryItems: len(items) + 1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := driver.waitFlowcraftHistoryProgress(ctx, "shorter history"); err == nil {
		t.Fatalf("waitFlowcraftHistoryProgress() error = nil, want failure")
	}
}

func TestPrepareConversationRequiresConfiguredSelfStart(t *testing.T) {
	driver := &personaDriver{
		cfg: config{
			Agent:     "flowcraft",
			Workspace: "self-start-required",
			Timeout:   "20ms",
			timeout:   20 * time.Millisecond,
			Workflow: workflowConfig{
				Flowcraft: map[string]interface{}{
					"conversation": map[string]interface{}{"starts": "self"},
				},
			},
		},
		transport: &chatTransport{
			events:      make(chan timedPeerEvent),
			opusPackets: make(chan timedPeerPacket),
			errs:        make(chan error),
		},
	}

	_, err := driver.prepareConversation(context.Background(), conversationMode{})
	if err == nil || !strings.Contains(err.Error(), "self-start did not emit output") {
		t.Fatalf("prepareConversation() error = %v, want missing self-start", err)
	}
}

func TestPersonaDriverRunUsesPeerTransportContract(t *testing.T) {
	events := make(chan timedPeerEvent, 8)
	opusPackets := make(chan timedPeerPacket, 8)
	stream := newFakePeerStream()
	var sentFrames [][]byte
	reloadCount := 0
	roundIndex := 0
	assistantLabel := "assistant"
	stream.push = func(chunk *genx.MessageChunk) error {
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			return nil
		}
		sentFrames = append(sentFrames, append([]byte(nil), blob.Data...))
		events <- timedTextEvent("transcript", "你好测试")
		events <- timedTranscriptDoneEvent()
		events <- timedTextEvent("assistant", "收到，继续。")
		events <- timedTextDoneEvent("assistant")
		for j := 0; j < 4; j++ {
			opusPackets <- newTimedPeerPacket([]byte{0x44, byte(roundIndex), byte(j)})
		}
		events <- newTimedPeerEvent(apitypes.PeerStreamEvent{
			Type:  apitypes.PeerStreamEventTypeEos,
			Label: &assistantLabel,
		})
		roundIndex++
		return nil
	}
	driver := &personaDriver{
		cfg: config{
			Rounds:    2,
			OutputDir: t.TempDir(),
		},
		transport: &chatTransport{
			stream:      stream,
			events:      events,
			opusPackets: opusPackets,
			errs:        make(chan error, 1),
		},
		generateUtterance: func(context.Context, int) (string, error) {
			return "你好测试", nil
		},
		synthesizeAudio: func(context.Context, string) ([]byte, [][]byte, error) {
			return []byte("ogg-audio"), [][]byte{{0x11, 0x22}}, nil
		},
		transcribeAudioFile: func(_ context.Context, path string) (string, error) {
			if strings.Contains(path, "assistant") {
				return "收到继续", nil
			}
			return "你好测试", nil
		},
		reloadAgent: func(context.Context) error {
			reloadCount++
			return nil
		},
	}

	stats, err := driver.runPushToTalkRoundtrip(context.Background())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(stats) != 2 || len(driver.history) != 2 {
		t.Fatalf("stats/history len = %d/%d", len(stats), len(driver.history))
	}
	if len(sentFrames) != 2 {
		t.Fatalf("sent frames = %d, want 2", len(sentFrames))
	}
	if reloadCount != 1 {
		t.Fatalf("reload count = %d, want 1", reloadCount)
	}
	for i, frame := range sentFrames {
		if !bytes.Equal(frame, []byte{0x11, 0x22}) {
			t.Fatalf("sent frame %d = %v", i, frame)
		}
	}
}

func testHistoryEntries(texts ...string) []rpcapi.PeerRunHistoryEntry {
	items := make([]rpcapi.PeerRunHistoryEntry, 0, len(texts))
	for i, text := range texts {
		items = append(items, rpcapi.PeerRunHistoryEntry{
			Id:              fmt.Sprintf("ctx:%06d", i),
			CreatedAt:       time.Now().Add(time.Duration(i) * time.Second),
			Name:            "agent",
			ReplayAvailable: true,
			Text:            text,
			Type:            rpcapi.PeerRunHistoryEntryTypeAgent,
		})
	}
	return items
}

func TestPersonaDriverWriteRoundAudioCreatesOutputDir(t *testing.T) {
	driver := &personaDriver{cfg: config{OutputDir: filepath.Join(t.TempDir(), "out")}}
	path, err := driver.writeRoundAudio(3, []byte("audio"))
	if err != nil {
		t.Fatalf("writeRoundAudio() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audio: %v", err)
	}
	if string(data) != "audio" || !strings.HasSuffix(path, "round-03-input.ogg") {
		t.Fatalf("path/data = %q/%q", path, data)
	}
}

func TestPersonaDriverDefaultOpenAIPaths(t *testing.T) {
	oggAudio := testOggOpus(t, [][]byte{
		[]byte("OpusHeadxxxx"),
		[]byte("OpusTagsyyyy"),
		{0x11, 0x22},
	})
	var sawChat, sawSpeech, sawTranscription bool
	var transcriptionBody []byte
	client := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.HasSuffix(req.URL.Path, "/chat/completions"):
				sawChat = true
				return jsonResponse(`{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"chat","choices":[{"index":0,"message":{"role":"assistant","content":" “你好\n测试” "},"finish_reason":"stop"}]}`), nil
			case strings.HasSuffix(req.URL.Path, "/audio/speech"):
				sawSpeech = true
				return binaryResponse("audio/ogg", oggAudio), nil
			case strings.HasSuffix(req.URL.Path, "/audio/transcriptions"):
				sawTranscription = true
				var err error
				transcriptionBody, err = io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read transcription request: %v", err)
				}
				return jsonResponse(`{"text":"你好测试"}`), nil
			default:
				t.Fatalf("unexpected OpenAI path %s", req.URL.Path)
				return nil, nil
			}
		})}),
	)
	driver := &personaDriver{
		cfg: config{
			Models: modelConfig{LLM: "chat", TTS: "tts", ASR: "asr"},
			Voice:  "voice",
		},
		client: client,
	}

	utterance, err := driver.nextUtterance(context.Background(), 1)
	if err != nil {
		t.Fatalf("nextUtterance() error = %v", err)
	}
	if utterance != "你好测试" {
		t.Fatalf("utterance = %q", utterance)
	}
	audio, packets, err := driver.synthesizeOpus(context.Background(), "你好测试")
	if err != nil {
		t.Fatalf("synthesizeOpus() error = %v", err)
	}
	if !bytes.Equal(audio, oggAudio) || len(packets) != 1 || !bytes.Equal(packets[0], []byte{0x11, 0x22}) {
		t.Fatalf("audio/packets = %d/%#v", len(audio), packets)
	}
	path := filepath.Join(t.TempDir(), "input.ogg")
	if err := os.WriteFile(path, audio, 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	transcript, err := driver.transcribe(context.Background(), path)
	if err != nil {
		t.Fatalf("transcribe() error = %v", err)
	}
	if transcript != "你好测试" {
		t.Fatalf("transcript = %q", transcript)
	}
	if !sawChat || !sawSpeech || !sawTranscription {
		t.Fatalf("saw chat/speech/transcription = %t/%t/%t", sawChat, sawSpeech, sawTranscription)
	}
	if bytes.Contains(transcriptionBody, []byte(`name="language"`)) {
		t.Fatalf("input transcription unexpectedly set language: %s", transcriptionBody)
	}
}

func TestPersonaDriverSynthesizeRetriesRetryableAPIError(t *testing.T) {
	oggAudio := testOggOpus(t, [][]byte{
		[]byte("OpusHeadxxxx"),
		[]byte("OpusTagsyyyy"),
		{0x11, 0x22},
	})
	attempts := 0
	client := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(req.URL.Path, "/audio/speech") {
				t.Fatalf("unexpected OpenAI path %s", req.URL.Path)
			}
			attempts++
			if attempts == 1 {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"speech backend rejected request","type":"invalid_request_error","code":"speech_failed"}}`)),
				}, nil
			}
			return binaryResponse("audio/ogg", oggAudio), nil
		})}),
	)
	driver := &personaDriver{
		cfg: config{
			Models: modelConfig{TTS: "tts"},
			Voice:  "voice",
		},
		client: client,
	}

	audio, packets, err := driver.synthesizeOpus(context.Background(), "你好测试")
	if err != nil {
		t.Fatalf("synthesizeOpus() error = %v", err)
	}
	if attempts != 2 || !bytes.Equal(audio, oggAudio) || len(packets) != 1 || !bytes.Equal(packets[0], []byte{0x11, 0x22}) {
		t.Fatalf("synthesizeOpus() attempts/audio/packets = %d/%d/%#v", attempts, len(audio), packets)
	}
}

func TestPersonaDriverSynthesizeRetriesEmptyAudio(t *testing.T) {
	oggAudio := testOggOpus(t, [][]byte{
		[]byte("OpusHeadxxxx"),
		[]byte("OpusTagsyyyy"),
		{0x11, 0x22},
	})
	attempts := 0
	client := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(req.URL.Path, "/audio/speech") {
				t.Fatalf("unexpected OpenAI path %s", req.URL.Path)
			}
			attempts++
			if attempts == 1 {
				return binaryResponse("audio/ogg", nil), nil
			}
			return binaryResponse("audio/ogg", oggAudio), nil
		})}),
	)
	driver := &personaDriver{
		cfg: config{
			Models: modelConfig{TTS: "tts"},
			Voice:  "voice",
		},
		client: client,
	}

	audio, packets, err := driver.synthesizeOpus(context.Background(), "你好测试")
	if err != nil {
		t.Fatalf("synthesizeOpus() error = %v", err)
	}
	if attempts != 2 || !bytes.Equal(audio, oggAudio) || len(packets) != 1 || !bytes.Equal(packets[0], []byte{0x11, 0x22}) {
		t.Fatalf("synthesizeOpus() attempts/audio/packets = %d/%d/%#v", attempts, len(audio), packets)
	}
}

func TestPersonaDriverTranscribeRetriesRetryableAPIError(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "input.ogg")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	attempts := 0
	client := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("http://gizclaw/v1"),
		option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(req.URL.Path, "/audio/transcriptions") {
				t.Fatalf("unexpected OpenAI path %s", req.URL.Path)
			}
			attempts++
			if attempts == 1 {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"audio decode failed","type":"invalid_request_error","code":"bad_audio"}}`)),
				}, nil
			}
			return jsonResponse(`{"text":"你好测试"}`), nil
		})}),
	)
	driver := &personaDriver{
		cfg:    config{Models: modelConfig{ASR: "asr"}},
		client: client,
	}
	got, err := driver.transcribe(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("transcribe() error = %v", err)
	}
	if got != "你好测试" || attempts != 2 {
		t.Fatalf("transcribe() = %q after %d attempts", got, attempts)
	}
}

func TestPersonaDriverSkipsAssistantAudioASRForJapaneseASTTarget(t *testing.T) {
	driver := &personaDriver{
		cfg: config{
			Agent: "ast-translate",
			Workflow: workflowConfig{
				Parameters: workspaceParameterConfig{LangPair: "zh/jp"},
			},
		},
	}
	if got := driver.assistantAudioASRSkipReason(conversationMode{}); got != "ast-translate-target-jp-human-review" {
		t.Fatalf("assistantAudioASRSkipReason() = %q", got)
	}
	driver.cfg.Workflow.Parameters.LangPair = "zh/en"
	if got := driver.assistantAudioASRSkipReason(conversationMode{}); got != "" {
		t.Fatalf("assistantAudioASRSkipReason(zh/en) = %q", got)
	}
	if got := driver.assistantAudioASRSkipReason(conversationMode{Realtime: true}); got != "ast-translate-realtime-provider-segmented-audio" {
		t.Fatalf("assistantAudioASRSkipReason(zh/en realtime) = %q", got)
	}
	if got := driver.assistantAudioASRSkipReason(conversationMode{SkipAssistantAudioASR: true}); got != "human-review" {
		t.Fatalf("assistantAudioASRSkipReason(human-review) = %q", got)
	}
}

func TestChatTransportClose(t *testing.T) {
	closed := false
	stream := newFakePeerStream()
	stream.close = func() error {
		closed = true
		return nil
	}
	transport := &chatTransport{
		stream: stream,
	}
	transport.close()
	if !closed {
		t.Fatal("transport did not close stream")
	}
}

func TestPersonaDriverRunRoundFailsWhenResponseIsIncomplete(t *testing.T) {
	events := make(chan timedPeerEvent, 2)
	opusPackets := make(chan timedPeerPacket, 1)
	var publish sync.Once
	stream := newFakePeerStream()
	stream.push = func(chunk *genx.MessageChunk) error {
		if _, ok := chunk.Part.(*genx.Blob); !ok {
			return nil
		}
		publish.Do(func() {
			events <- timedTextEvent("transcript", "你好测试")
			opusPackets <- newTimedPeerPacket([]byte{0x44})
		})
		return nil
	}
	driver := &personaDriver{
		cfg: config{OutputDir: t.TempDir()},
		transport: &chatTransport{
			stream:      stream,
			events:      events,
			opusPackets: opusPackets,
			errs:        make(chan error, 1),
		},
		generateUtterance: func(context.Context, int) (string, error) {
			return "你好测试", nil
		},
		synthesizeAudio: func(context.Context, string) ([]byte, [][]byte, error) {
			return []byte("ogg-audio"), [][]byte{{0x11}}, nil
		},
		transcribeAudioFile: func(context.Context, string) (string, error) {
			return "你好测试", nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := driver.runRound(ctx, 1, conversationMode{})
	if err == nil || !strings.Contains(err.Error(), "wait response") {
		t.Fatalf("runRound() error = %v", err)
	}
}

func TestPersonaDriverRunRoundVerifiesAssistantAudio(t *testing.T) {
	events := make(chan timedPeerEvent, 5)
	label := "assistant"
	opusPackets := make(chan timedPeerPacket, 1)
	var publish sync.Once
	stream := newFakePeerStream()
	stream.push = func(chunk *genx.MessageChunk) error {
		if _, ok := chunk.Part.(*genx.Blob); !ok {
			return nil
		}
		publish.Do(func() {
			responseStreamID := "response-stream-1"
			events <- timedTextEvent("transcript", "你好测试")
			events <- timedTranscriptDoneEvent()
			events <- newTimedPeerEvent(labeledTextEventWithStream("assistant", responseStreamID, "回复文本"))
			events <- timedTextDoneEventWithStream("assistant", responseStreamID)
			events <- newTimedPeerEvent(apitypes.PeerStreamEvent{
				Type:     apitypes.PeerStreamEventTypeEos,
				Label:    &label,
				StreamId: &responseStreamID,
			})
			opusPackets <- newTimedPeerPacket([]byte{0x44})
		})
		return nil
	}
	driver := &personaDriver{
		cfg: config{OutputDir: t.TempDir()},
		transport: &chatTransport{
			stream:      stream,
			events:      events,
			opusPackets: opusPackets,
			errs:        make(chan error, 1),
		},
		generateUtterance: func(context.Context, int) (string, error) {
			return "你好测试", nil
		},
		synthesizeAudio: func(context.Context, string) ([]byte, [][]byte, error) {
			return []byte("ogg-audio"), [][]byte{{0x11}}, nil
		},
		transcribeAudioFile: func(_ context.Context, path string) (string, error) {
			if strings.Contains(path, "assistant") {
				return "回复文本", nil
			}
			return "你好测试", nil
		},
	}
	stat, err := driver.runRound(context.Background(), 1, conversationMode{})
	if err != nil {
		t.Fatalf("runRound() error = %v", err)
	}
	if stat.AssistantText != "回复文本" {
		t.Fatalf("assistant text = %q, want 回复文本", stat.AssistantText)
	}
	if stat.AssistantAudioASR != "回复文本" {
		t.Fatalf("assistant audio asr = %q, want 回复文本", stat.AssistantAudioASR)
	}
	if stat.DownlinkPackets != 1 {
		t.Fatalf("downlink packets = %d, want 1", stat.DownlinkPackets)
	}
}

func TestPersonaDriverRunRoundLightweightSkipsSemanticASR(t *testing.T) {
	events := make(chan timedPeerEvent, 5)
	label := "assistant"
	opusPackets := make(chan timedPeerPacket, 1)
	var publish sync.Once
	stream := newFakePeerStream()
	stream.push = func(chunk *genx.MessageChunk) error {
		if _, ok := chunk.Part.(*genx.Blob); !ok {
			return nil
		}
		publish.Do(func() {
			responseStreamID := "response-stream-1"
			events <- timedTextEvent("transcript", "完全不同的识别文本")
			events <- timedTranscriptDoneEvent()
			events <- newTimedPeerEvent(labeledTextEventWithStream("assistant", responseStreamID, "回复文本"))
			events <- timedTextDoneEventWithStream("assistant", responseStreamID)
			events <- newTimedPeerEvent(apitypes.PeerStreamEvent{
				Type:     apitypes.PeerStreamEventTypeEos,
				Label:    &label,
				StreamId: &responseStreamID,
			})
			opusPackets <- newTimedPeerPacket([]byte{0x44})
		})
		return nil
	}
	driver := &personaDriver{
		cfg: config{OutputDir: t.TempDir()},
		transport: &chatTransport{
			stream:      stream,
			events:      events,
			opusPackets: opusPackets,
			errs:        make(chan error, 1),
		},
		generateUtterance: func(context.Context, int) (string, error) {
			return "你好测试", nil
		},
		synthesizeAudio: func(context.Context, string) ([]byte, [][]byte, error) {
			return []byte("ogg-audio"), [][]byte{{0x11}}, nil
		},
		transcribeAudioFile: func(context.Context, string) (string, error) {
			t.Fatal("lightweight round should not call ASR")
			return "", nil
		},
	}
	stat, err := driver.runRound(context.Background(), 1, conversationMode{
		SkipInputASR:             true,
		SkipTranscriptSimilarity: true,
		SkipAssistantAudioASR:    true,
		AssistantAudioASRReason:  "history-replay",
	})
	if err != nil {
		t.Fatalf("runRound() error = %v", err)
	}
	if stat.InputASR != "skipped: lightweight-behavior" {
		t.Fatalf("input asr = %q", stat.InputASR)
	}
	if stat.Transcript != "完全不同的识别文本" {
		t.Fatalf("transcript = %q", stat.Transcript)
	}
	if stat.AssistantAudioASR != "skipped: history-replay" {
		t.Fatalf("assistant audio asr = %q", stat.AssistantAudioASR)
	}
}

func TestAssistantASRFrameChunksMergesShortTail(t *testing.T) {
	tests := []struct {
		name       string
		frameCount int
		want       []frameChunkRange
	}{
		{
			name:       "single short audio",
			frameCount: 24,
			want:       []frameChunkRange{{start: 0, end: 24}},
		},
		{
			name:       "exact chunks",
			frameCount: 1200,
			want:       []frameChunkRange{{start: 0, end: 600}, {start: 600, end: 1200}},
		},
		{
			name:       "short tail merged",
			frameCount: 3018,
			want:       []frameChunkRange{{start: 0, end: 600}, {start: 600, end: 1200}, {start: 1200, end: 1800}, {start: 1800, end: 2400}, {start: 2400, end: 3018}},
		},
		{
			name:       "large enough tail remains separate",
			frameCount: 3130,
			want:       []frameChunkRange{{start: 0, end: 600}, {start: 600, end: 1200}, {start: 1200, end: 1800}, {start: 1800, end: 2400}, {start: 2400, end: 3000}, {start: 3000, end: 3130}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assistantASRFrameChunks(tt.frameCount)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("assistantASRFrameChunks(%d) = %+v, want %+v", tt.frameCount, got, tt.want)
			}
		})
	}
}

func TestVerifyAssistantAudioASRSplitsFailedLargeChunk(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	frames, err := opusPacketsFromPCM16LE(
		silencePCM16Mono16K(time.Duration(assistantASRMinRetryFrames*2)*20*time.Millisecond),
		16000,
		1,
	)
	if err != nil {
		t.Fatalf("opusPacketsFromPCM16LE: %v", err)
	}
	if len(frames) != assistantASRMinRetryFrames*2 {
		t.Fatalf("frames = %d, want %d", len(frames), assistantASRMinRetryFrames*2)
	}
	var sawBase, sawLeft, sawRight bool
	driver := &personaDriver{
		cfg: config{OutputDir: t.TempDir()},
		transcribeAudioFile: func(_ context.Context, path string) (string, error) {
			switch filepath.Base(path) {
			case "round-01-assistant.wav":
				sawBase = true
				return "", errors.New("transcription failed")
			case "round-01-assistanta.wav":
				sawLeft = true
				return "你好", nil
			case "round-01-assistantb.wav":
				sawRight = true
				return "测试", nil
			default:
				t.Fatalf("unexpected transcription path %s", path)
				return "", nil
			}
		},
	}
	got, err := driver.verifyAssistantAudioASR(context.Background(), 1, "assistant", "你好测试", frames)
	if err != nil {
		t.Fatalf("verifyAssistantAudioASR() error = %v", err)
	}
	if got != "你好 测试" {
		t.Fatalf("verifyAssistantAudioASR() = %q", got)
	}
	if !sawBase || !sawLeft || !sawRight {
		t.Fatalf("saw base/left/right = %t/%t/%t", sawBase, sawLeft, sawRight)
	}
}

func TestChatTransportReadEventsAndSendAudioTurn(t *testing.T) {
	stream := newFakePeerStream()
	packets := make(chan timedPeerPacket, 4)

	transport := &chatTransport{
		stream:      stream,
		events:      make(chan timedPeerEvent, 4),
		opusPackets: packets,
		errs:        make(chan error, 1),
	}
	go transport.readChunks(packets)

	text := "你好"
	stream.chunks <- &genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{Label: "assistant"}}
	got := <-transport.events
	if got.event.Text == nil || *got.event.Text != text {
		t.Fatalf("event = %+v", got)
	}
	stream.chunks <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{Label: "assistant", EndOfStream: true, Error: "interrupted"}}
	got = <-transport.events
	if got.event.Error == nil || *got.event.Error != "interrupted" {
		t.Fatalf("eos event error = %+v", got.event)
	}

	stream.chunks <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}}
	gotPacket := <-transport.opusPackets
	if !bytes.Equal(gotPacket.frame, []byte{1, 2, 3}) || gotPacket.receivedAt.IsZero() {
		t.Fatalf("forwarded packet = %+v", gotPacket)
	}
	if gotPacket.since(gotPacket.receivedAt.Add(-time.Millisecond)) <= 0 {
		t.Fatalf("forwarded packet since returned non-positive duration")
	}

	stream.chunks <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{4, 5, 6}}, Ctrl: &genx.StreamCtrl{Label: "assistant", EndOfStream: true}}
	gotPacket = <-transport.opusPackets
	if !bytes.Equal(gotPacket.frame, []byte{4, 5, 6}) {
		t.Fatalf("forwarded eos packet = %+v", gotPacket)
	}
	got = <-transport.events
	if got.event.Type != apitypes.PeerStreamEventTypeEos || eventLabel(got.event) != "assistant" {
		t.Fatalf("audio data eos event = %+v", got.event)
	}
}

func TestTimedPeerSinceFallsBackWhenTimestampIsZero(t *testing.T) {
	start := time.Now().Add(-time.Millisecond)
	if got := (timedPeerEvent{}).since(start); got <= 0 {
		t.Fatalf("timedPeerEvent.since() = %s", got)
	}
	if got := (timedPeerPacket{}).since(start); got <= 0 {
		t.Fatalf("timedPeerPacket.since() = %s", got)
	}
}

func TestSendAudioTurnFramesEvents(t *testing.T) {
	stream := newFakePeerStream()
	transport := &chatTransport{
		stream: stream,
	}
	if err := transport.sendAudioTurn(context.Background(), "s1", [][]byte{{1, 2, 3}}); err != nil {
		t.Fatalf("sendAudioTurn() error = %v", err)
	}
	if len(stream.pushed) != 3 {
		t.Fatalf("pushed chunks = %d, want 3", len(stream.pushed))
	}
	if !stream.pushed[0].IsBeginOfStream() || !stream.pushed[2].IsEndOfStream() {
		t.Fatalf("stream boundaries = %#v / %#v", stream.pushed[0].Ctrl, stream.pushed[2].Ctrl)
	}
	blob := stream.pushed[1].Part.(*genx.Blob)
	if blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{1, 2, 3}) {
		t.Fatalf("audio chunk = %#v", stream.pushed[1])
	}
}

func TestSendAudioTurnErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	transport := &chatTransport{
		stream: newFakePeerStream(),
	}
	if err := transport.sendAudioTurn(canceled, "s1", [][]byte{{1}}); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sendAudioTurn() error = %v", err)
	}
	wantErr := errors.New("write packet failed")
	stream := newFakePeerStream()
	stream.push = func(chunk *genx.MessageChunk) error {
		if _, ok := chunk.Part.(*genx.Blob); ok {
			return wantErr
		}
		return nil
	}
	transport = &chatTransport{
		stream: stream,
	}
	if err := transport.sendAudioTurn(context.Background(), "s1", [][]byte{{1}}); !errors.Is(err, wantErr) {
		t.Fatalf("write error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	writes := 0
	stream = newFakePeerStream()
	stream.push = func(chunk *genx.MessageChunk) error {
		if _, ok := chunk.Part.(*genx.Blob); ok {
			writes++
			cancel()
		}
		return nil
	}
	transport = &chatTransport{
		packetInterval: time.Hour,
		stream:         stream,
	}
	if err := transport.sendAudioTurn(ctx, "s1", [][]byte{{1}, {2}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("paced canceled sendAudioTurn() error = %v", err)
	}
	if writes != 1 {
		t.Fatalf("paced writes = %d, want 1", writes)
	}
}

func labeledTextEvent(label, text string) apitypes.PeerStreamEvent {
	return apitypes.PeerStreamEvent{
		Type:  apitypes.PeerStreamEventTypeTextDelta,
		Label: &label,
		Text:  &text,
	}
}

func labeledTextEventWithStream(label, streamID, text string) apitypes.PeerStreamEvent {
	event := labeledTextEvent(label, text)
	event.StreamId = &streamID
	return event
}

func timedTextEvent(label, text string) timedPeerEvent {
	return newTimedPeerEvent(labeledTextEvent(label, text))
}

func timedTranscriptDoneEvent() timedPeerEvent {
	return timedTextDoneEvent("transcript")
}

func timedTextDoneEvent(label string) timedPeerEvent {
	return newTimedPeerEvent(apitypes.PeerStreamEvent{
		Type:  apitypes.PeerStreamEventTypeTextDone,
		Label: &label,
	})
}

func timedTextDoneEventWithStream(label, streamID string) timedPeerEvent {
	return newTimedPeerEvent(apitypes.PeerStreamEvent{
		Type:     apitypes.PeerStreamEventTypeTextDone,
		Label:    &label,
		StreamId: &streamID,
	})
}

type closeBuffer struct {
	bytes.Buffer
}

func (b *closeBuffer) Close() error {
	return nil
}

func (b *closeBuffer) CloseWithError(error) error { return nil }
func (b *closeBuffer) Next() (*genx.MessageChunk, error) {
	return nil, io.EOF
}
func (b *closeBuffer) Push(context.Context, *genx.MessageChunk) error {
	return nil
}

type fakePeerStream struct {
	chunks chan *genx.MessageChunk
	pushed []*genx.MessageChunk
	push   func(*genx.MessageChunk) error
	close  func() error
}

func newFakePeerStream() *fakePeerStream {
	return &fakePeerStream{chunks: make(chan *genx.MessageChunk, 16)}
}

func (s *fakePeerStream) Next() (*genx.MessageChunk, error) {
	chunk, ok := <-s.chunks
	if !ok {
		return nil, io.EOF
	}
	return chunk, nil
}

func (s *fakePeerStream) Push(_ context.Context, chunk *genx.MessageChunk) error {
	s.pushed = append(s.pushed, chunk)
	if s.push != nil {
		return s.push(chunk)
	}
	return nil
}

func (s *fakePeerStream) Close() error {
	if s.close != nil {
		return s.close()
	}
	return nil
}

func (s *fakePeerStream) CloseWithError(err error) error {
	return s.Close()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func binaryResponse(contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func testOggOpus(t *testing.T, packets [][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer, err := ogg.NewStreamWriter(&buf, 123)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	for _, packet := range packets {
		if _, err := writer.WritePacket(packet, 960, false); err != nil {
			t.Fatalf("WritePacket: %v", err)
		}
	}
	return buf.Bytes()
}
