//go:build gizclaw_genx_e2e

package transformer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoasr"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts"
	"github.com/GizClaw/minimax-go"
)

const (
	miniMaxAPIKeyEnv  = "GIZCLAW_GENX_E2E_MINIMAX_API_KEY"
	miniMaxBaseURLEnv = "GIZCLAW_GENX_E2E_MINIMAX_BASE_URL"
	miniMaxTTSModel   = "speech-2.6-hd"

	doubaoASRChunkDuration        = 100 * time.Millisecond
	doubaoASRChunkSize            = 3200
	doubaoASRShortUtteranceStart  = 640 * time.Millisecond
	doubaoASRShortUtteranceEnd    = 2560 * time.Millisecond
	doubaoASRDefiniteDeadline     = 5 * time.Second
	doubaoASRMinimumLatencySaving = time.Second
	doubaoASRTunedLatencyLimit    = 1500 * time.Millisecond
	doubaoASRLiveFrameDuration    = 20 * time.Millisecond
	doubaoASRLiveFrameSize        = 640
	doubaoASRLiveLatencyLimit     = 700 * time.Millisecond
	doubaoASRMinimumPacingSaving  = 200 * time.Millisecond
	doubaoASRPacingTrials         = 10
)

func TestDoubaoSAUCASR(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv)
	apiKey := firstEnv(doubaoAPIKeyEnv)
	if appID == "" || apiKey == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	realtimePacing := false
	transformer, err := doubaoasr.New(doubaoasr.Config{
		Client:         doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey)),
		RealtimePacing: &realtimePacing,
	})
	if err != nil {
		t.Fatalf("doubaoasr.New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	streamID := "doubao-sauc-e2e"
	pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/ogg"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
	})
	pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/ogg", Data: doubaoRealtimeDuplexPromptOgg},
		Ctrl: &genx.StreamCtrl{StreamID: streamID},
	})
	pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/ogg"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true},
	})
	if err := input.Close(); err != nil {
		t.Fatalf("close ASR input: %v", err)
	}

	var transcript strings.Builder
	tracker := newRouteLifecycleTracker()
	chunks := collectSpeechOutput(t, output)
	for _, chunk := range chunks {
		if err := speechChunkError(chunk); err != nil {
			t.Fatal(err)
		}
		observeRouteLifecycle(t, tracker, chunk)
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID {
			t.Fatalf("ASR chunk route = %#v, want stream %q", chunk.Ctrl, streamID)
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			transcript.WriteString(string(text))
		}
	}
	tracker.assertComplete(t)
	if strings.TrimSpace(transcript.String()) == "" {
		t.Fatal("Doubao SAUC returned no transcript")
	}
	transcriptRoute := tracker.route(streamID, "text/plain")
	if transcriptRoute == nil || transcriptRoute.dataChunks == 0 {
		t.Fatalf("Doubao SAUC transcript route = %#v, want BOS/data/EOS", transcriptRoute)
	}
	t.Logf("transcript=%q routes=%d chunks=%d", transcript.String(), len(tracker.routes), len(chunks))
}

// TestDoubaoSAUCLivePCMFrameAggregation exercises the device-facing cadence:
// 20 ms PCM frames arrive in real time, while the transformer aggregates them
// into the provider's 100 ms packet cadence before finalization.
func TestDoubaoSAUCLivePCMFrameAggregation(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv)
	apiKey := firstEnv(doubaoAPIKeyEnv)
	if appID == "" || apiKey == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	var pcm bytes.Buffer
	if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(doubaoRealtimeDuplexPromptOgg), opus.SampleRate16K); err != nil {
		t.Fatalf("decode live ASR fixture: %v", err)
	}
	audio := shortASRPCM(t, pcm.Bytes(), doubaoASRShortUtteranceStart, doubaoASRShortUtteranceEnd)
	client := doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey))
	for _, packetSize := range []int{doubaoASRChunkSize, 6400} {
		t.Run(fmt.Sprintf("unpaced_packet_%dms", packetSize*1000/(16000*2)), func(t *testing.T) {
			latencies := make([]time.Duration, 0, 5)
			for trial := range 5 {
				latencies = append(latencies, runDoubaoASRLivePCMTrial(t, client, audio, packetSize, false, trial))
			}
			slices.Sort(latencies)
			median := latencies[len(latencies)/2]
			t.Logf("packet_bytes=%d final transcript latencies=%v p50=%s", packetSize, latencies, median)
			if median > doubaoASRLiveLatencyLimit {
				t.Fatalf("live PCM final transcript p50 = %s, want at most %s", median, doubaoASRLiveLatencyLimit)
			}
		})
	}

	t.Run("pacing_comparison_100ms", func(t *testing.T) {
		latencies := map[bool][]time.Duration{false: {}, true: {}}
		for trial := range doubaoASRPacingTrials {
			// Alternate the order so provider drift cannot systematically favor one mode.
			order := []bool{false, true}
			if trial%2 != 0 {
				slices.Reverse(order)
			}
			for _, realtimePacing := range order {
				latencies[realtimePacing] = append(latencies[realtimePacing], runDoubaoASRLivePCMTrial(t, client, audio, doubaoASRChunkSize, realtimePacing, trial))
			}
		}
		for realtimePacing := range latencies {
			slices.Sort(latencies[realtimePacing])
		}
		unpacedMedian := latencies[false][len(latencies[false])/2]
		pacedMedian := latencies[true][len(latencies[true])/2]
		t.Logf("unpaced_latencies=%v paced_latencies=%v unpaced_p50=%s paced_p50=%s", latencies[false], latencies[true], unpacedMedian, pacedMedian)
		if unpacedMedian > doubaoASRLiveLatencyLimit {
			t.Fatalf("unpaced final transcript p50 = %s, want at most %s", unpacedMedian, doubaoASRLiveLatencyLimit)
		}
		if saving := pacedMedian - unpacedMedian; saving < doubaoASRMinimumPacingSaving {
			t.Fatalf("disabling duplicate realtime pacing saved %s, want at least the %s production-change threshold", saving, doubaoASRMinimumPacingSaving)
		}
	})
}

func runDoubaoASRLivePCMTrial(t *testing.T, client *doubaospeech.Client, audio []byte, packetSize int, realtimePacing bool, trial int) time.Duration {
	t.Helper()
	transformer, err := doubaoasr.New(doubaoasr.Config{
		Client:         client,
		ChunkSize:      packetSize,
		RealtimePacing: &realtimePacing,
	})
	if err != nil {
		t.Fatalf("doubaoasr.New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	transcripts := make(chan doubaoASRTranscript, 1)
	go receiveFirstASRTranscript(output, transcripts)
	streamID := fmt.Sprintf("doubao-sauc-live-pcm-%d-%t-%d", packetSize, realtimePacing, trial)
	for offset := 0; offset < len(audio); offset += doubaoASRLiveFrameSize {
		end := min(offset+doubaoASRLiveFrameSize, len(audio))
		pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "audio/pcm", Data: audio[offset:end]},
			Ctrl: &genx.StreamCtrl{StreamID: streamID},
		})
		waitSpeechInterval(t, ctx, doubaoASRLiveFrameDuration)
	}
	speechEndedAt := time.Now()
	pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true},
	})
	if err := input.Close(); err != nil {
		t.Fatalf("close ASR input: %v", err)
	}

	select {
	case transcript := <-transcripts:
		if transcript.err != nil {
			t.Fatalf("read ASR transcript: %v", transcript.err)
		}
		latency := transcript.receivedAt.Sub(speechEndedAt)
		t.Logf("trial=%d realtime_pacing=%t transcript_present=%t final_latency=%s", trial+1, realtimePacing, strings.TrimSpace(transcript.text) != "", latency)
		return latency
	case <-ctx.Done():
		t.Fatalf("Doubao ASR live PCM context ended: %v", ctx.Err())
		return 0
	}
}

func TestDoubaoSAUCEndpointingReducesShortUtteranceLatency(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv)
	apiKey := firstEnv(doubaoAPIKeyEnv)
	if appID == "" || apiKey == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	var pcm bytes.Buffer
	if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(doubaoRealtimeDuplexPromptOgg), opus.SampleRate16K); err != nil {
		t.Fatalf("decode short ASR fixture: %v", err)
	}
	audio := shortASRPCM(t, pcm.Bytes(), doubaoASRShortUtteranceStart, doubaoASRShortUtteranceEnd)
	client := doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey))

	baseline := runDoubaoASREndpointingTrial(t, client, audio, nil, nil)
	endWindowSize := 200
	forceToSpeechTime := 0
	tuned := runDoubaoASREndpointingTrial(t, client, audio, &endWindowSize, &forceToSpeechTime)

	saving := baseline.latency - tuned.latency
	t.Logf(
		"Doubao ASR short-utterance latency: default=%s tuned=%s saving=%s default_text=%q tuned_text=%q",
		baseline.latency,
		tuned.latency,
		saving,
		baseline.text,
		tuned.text,
	)
	if tuned.text != baseline.text {
		t.Fatalf("tuned transcript = %q, want baseline transcript %q", tuned.text, baseline.text)
	}
	if saving < doubaoASRMinimumLatencySaving {
		t.Fatalf("endpointing parameters saved %s, want at least %s", saving, doubaoASRMinimumLatencySaving)
	}
	if tuned.latency > doubaoASRTunedLatencyLimit {
		t.Fatalf("tuned endpointing latency = %s, want at most %s", tuned.latency, doubaoASRTunedLatencyLimit)
	}
}

type doubaoASREndpointingResult struct {
	text    string
	latency time.Duration
}

type doubaoASRTranscript struct {
	text       string
	receivedAt time.Time
	err        error
}

func runDoubaoASREndpointingTrial(
	t *testing.T,
	client *doubaospeech.Client,
	audio []byte,
	endWindowSize *int,
	forceToSpeechTime *int,
) doubaoASREndpointingResult {
	t.Helper()
	realtimePacing := false
	transformer, err := doubaoasr.New(doubaoasr.Config{
		Client:            client,
		RealtimePacing:    &realtimePacing,
		EndWindowSize:     endWindowSize,
		ForceToSpeechTime: forceToSpeechTime,
	})
	if err != nil {
		t.Fatalf("doubaoasr.New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	transcripts := make(chan doubaoASRTranscript, 1)
	go receiveFirstASRTranscript(output, transcripts)
	streamID := "doubao-sauc-endpointing-e2e"
	pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
	})
	for offset := 0; offset < len(audio); offset += doubaoASRChunkSize {
		end := min(offset+doubaoASRChunkSize, len(audio))
		pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "audio/pcm", Data: audio[offset:end]},
			Ctrl: &genx.StreamCtrl{StreamID: streamID},
		})
		waitSpeechInterval(t, ctx, doubaoASRChunkDuration)
	}

	speechEndedAt := time.Now()
	silence := make([]byte, doubaoASRChunkSize)
	deadline := time.NewTimer(doubaoASRDefiniteDeadline)
	defer deadline.Stop()
	ticker := time.NewTicker(doubaoASRChunkDuration)
	defer ticker.Stop()
	for {
		select {
		case transcript := <-transcripts:
			if transcript.err != nil {
				t.Fatalf("read ASR transcript: %v", transcript.err)
			}
			latency := transcript.receivedAt.Sub(speechEndedAt)
			if latency < 0 {
				t.Fatalf("ASR transcript arrived before short utterance ended: %s", latency)
			}
			return doubaoASREndpointingResult{text: transcript.text, latency: latency}
		case <-ticker.C:
			pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: &genx.Blob{MIMEType: "audio/pcm", Data: silence},
				Ctrl: &genx.StreamCtrl{StreamID: streamID},
			})
		case <-deadline.C:
			t.Fatalf("Doubao ASR returned no non-empty definite transcript within %s without audio EOS", doubaoASRDefiniteDeadline)
		case <-ctx.Done():
			t.Fatalf("Doubao ASR endpointing context ended: %v", ctx.Err())
		}
	}
}

func receiveFirstASRTranscript(output genx.Stream, transcripts chan<- doubaoASRTranscript) {
	for {
		chunk, err := output.Next()
		if err != nil {
			transcripts <- doubaoASRTranscript{err: err}
			return
		}
		if chunk == nil {
			continue
		}
		if err := speechChunkError(chunk); err != nil {
			transcripts <- doubaoASRTranscript{err: err}
			return
		}
		text, ok := chunk.Part.(genx.Text)
		trimmed := strings.TrimSpace(string(text))
		if ok && trimmed != "" {
			transcripts <- doubaoASRTranscript{text: trimmed, receivedAt: time.Now()}
			return
		}
	}
}

func shortASRPCM(t *testing.T, audio []byte, start, end time.Duration) []byte {
	t.Helper()
	const bytesPerSecond = 16000 * 2
	startOffset := int(int64(start) * bytesPerSecond / int64(time.Second))
	endOffset := int(int64(end) * bytesPerSecond / int64(time.Second))
	if startOffset < 0 || startOffset >= endOffset || endOffset > len(audio) {
		t.Fatalf("short ASR fixture range [%s, %s] is outside %d-byte PCM input", start, end, len(audio))
	}
	return audio[startOffset:endOffset]
}

func waitSpeechInterval(t *testing.T, ctx context.Context, interval time.Duration) {
	t.Helper()
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatalf("speech interval context ended: %v", ctx.Err())
	}
}

func TestDoubaoSeedV2TTS(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv)
	apiKey := firstEnv(doubaoAPIKeyEnv)
	if appID == "" || apiKey == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
	}

	transformer, err := doubaotts.NewSeedV2(doubaotts.SeedV2Config{
		Client:  doubaospeech.NewClient(appID, doubaospeech.WithAPIKey(apiKey)),
		Speaker: "zh_female_xiaohe_uranus_bigtts",
	})
	if err != nil {
		t.Fatalf("doubaotts.NewSeedV2() failed: %v", err)
	}
	runTTSE2E(t, transformer, "doubao-seed-v2-e2e", "你好，这是一条豆包语音合成端到端测试。", "audio/ogg")
}

func TestMiniMaxTTS(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(miniMaxAPIKeyEnv)
	baseURL := firstEnv(miniMaxBaseURLEnv)
	if apiKey == "" || baseURL == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", miniMaxAPIKeyEnv, miniMaxBaseURLEnv)
	}
	voiceID := "female-shaonv"
	client, err := minimax.NewClient(minimax.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		t.Fatalf("minimax.NewClient() failed: %v", err)
	}
	transformer, err := minimaxtts.New(minimaxtts.Config{Client: client, VoiceID: voiceID, Model: miniMaxTTSModel})
	if err != nil {
		t.Fatalf("minimaxtts.New() failed: %v", err)
	}
	runTTSE2E(t, transformer, "minimax-tts-e2e", "你好，这是一条 MiniMax 语音合成端到端测试。", "audio/mpeg")
}

// TestMiniMaxOggOpusTTS exercises the #933 path end to end against the live
// MiniMax provider: MiniMax has no Opus container, so the transformer requests
// PCM and encodes Ogg/Opus locally. The emitted bytes must decode as Ogg/Opus,
// be 16 kHz mono (Volc parity), and — when the response spans several
// synthesized segments — form a valid chained stream with distinct serials.
func TestMiniMaxOggOpusTTS(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(miniMaxAPIKeyEnv)
	baseURL := firstEnv(miniMaxBaseURLEnv)
	if apiKey == "" || baseURL == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", miniMaxAPIKeyEnv, miniMaxBaseURLEnv)
	}
	client, err := minimax.NewClient(minimax.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		t.Fatalf("minimax.NewClient() failed: %v", err)
	}
	transformer, err := minimaxtts.New(minimaxtts.Config{Client: client, VoiceID: "female-shaonv", Model: miniMaxTTSModel, Format: minimaxtts.FormatOggOpus})
	if err != nil {
		t.Fatalf("minimaxtts.New() failed: %v", err)
	}
	// Two sentences so the shared TTS pipeline produces multiple segments and
	// the output is a chained Ogg stream.
	encoded := collectTTSAudioE2E(t, transformer, "minimax-ogg-e2e",
		"你好，这是第一句。这是第二句，用于验证分段拼接。", "audio/ogg")

	// The concatenation must be valid Ogg: every OpusHead is 16 kHz mono, and
	// each BOS page carries a distinct serial (a valid chained physical stream).
	pages, err := ogg.ReadAllPages(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode chained ogg pages: %v", err)
	}
	serials := map[uint32]struct{}{}
	heads := 0
	for _, page := range pages {
		if page.HasBOS() {
			serials[page.BitstreamSerial] = struct{}{}
		}
	}
	for packet, err := range ogg.Packets(bytes.NewReader(encoded)) {
		if err != nil {
			t.Fatalf("reconstruct chained ogg packets: %v", err)
		}
		if codecconv.IsOpusHeadPacket(packet.Data) {
			sampleRate, channels, err := codecconv.ParseOpusHeadPacket(packet.Data)
			if err != nil {
				t.Fatalf("ParseOpusHeadPacket() error = %v", err)
			}
			if sampleRate != 16000 || channels != 1 {
				t.Fatalf("OpusHead = %d Hz %d ch, want 16000 Hz mono (Volc parity)", sampleRate, channels)
			}
			heads++
		}
	}
	if heads == 0 {
		t.Fatal("no OpusHead in MiniMax ogg output")
	}
	if len(serials) != heads {
		t.Fatalf("distinct BOS serials = %d, want %d (one per logical stream)", len(serials), heads)
	}

	// The audio packets must decode as Opus.
	opusPackets := 0
	for _, err := range codecconv.OggOpusPackets(bytes.NewReader(encoded)) {
		if err != nil {
			t.Fatalf("decode chained opus packets: %v", err)
		}
		opusPackets++
	}
	if opusPackets == 0 {
		t.Fatal("MiniMax ogg output decoded to zero Opus audio packets")
	}
	t.Logf("ogg_bytes=%d logical_streams=%d opus_packets=%d", len(encoded), heads, opusPackets)
}

// collectTTSAudioE2E drives one TTS turn and returns the concatenated audio
// bytes, asserting the route lifecycle and MIME like runTTSE2E.
func collectTTSAudioE2E(t *testing.T, transformer genx.Transformer, streamID, text, wantMIME string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", EndOfStream: true}},
	} {
		pushSpeechChunk(t, ctx, input, chunk)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close TTS input: %v", err)
	}

	var audio []byte
	tracker := newRouteLifecycleTracker()
	for _, chunk := range collectSpeechOutput(t, output) {
		if err := speechChunkError(chunk); err != nil {
			t.Fatal(err)
		}
		observeRouteLifecycle(t, tracker, chunk)
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			if blob.MIMEType != wantMIME {
				t.Fatalf("TTS MIME = %q, want %q", blob.MIMEType, wantMIME)
			}
			audio = append(audio, blob.Data...)
		}
	}
	tracker.assertComplete(t)
	if len(audio) == 0 {
		t.Fatal("TTS returned no audio bytes")
	}
	return audio
}

func runTTSE2E(t *testing.T, transformer genx.Transformer, streamID, text, wantMIME string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	for _, chunk := range []*genx.MessageChunk{
		{
			Role: genx.RoleModel,
			Name: "assistant",
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true},
		},
		{
			Role: genx.RoleModel,
			Name: "assistant",
			Part: genx.Text(text),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"},
		},
		{
			Role: genx.RoleModel,
			Name: "assistant",
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", EndOfStream: true},
		},
	} {
		pushSpeechChunk(t, ctx, input, chunk)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close TTS input: %v", err)
	}

	audioBytes := 0
	tracker := newRouteLifecycleTracker()
	chunks := collectSpeechOutput(t, output)
	for _, chunk := range chunks {
		if err := speechChunkError(chunk); err != nil {
			t.Fatal(err)
		}
		observeRouteLifecycle(t, tracker, chunk)
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != "assistant" {
			t.Fatalf("TTS chunk route = %#v, want stream %q label assistant", chunk.Ctrl, streamID)
		}
		if chunk.Role != genx.RoleModel || chunk.Name != "assistant" {
			t.Fatalf("TTS chunk metadata = role %q name %q", chunk.Role, chunk.Name)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			if blob.MIMEType != wantMIME {
				t.Fatalf("TTS MIME = %q, want %q", blob.MIMEType, wantMIME)
			}
			audioBytes += len(blob.Data)
		}
	}
	tracker.assertComplete(t)
	if audioBytes == 0 {
		t.Fatal("TTS returned no audio bytes")
	}
	audioRoute := tracker.route(streamID, wantMIME)
	if audioRoute == nil || audioRoute.dataChunks == 0 {
		t.Fatalf("TTS audio route = %#v, want BOS/data/EOS", audioRoute)
	}
	t.Logf("audio_bytes=%d mime=%s routes=%d chunks=%d", audioBytes, wantMIME, len(tracker.routes), len(chunks))
}

func pushSpeechChunk(t *testing.T, ctx context.Context, input *genx.RealtimeStream, chunk *genx.MessageChunk) {
	t.Helper()
	if err := input.Push(ctx, chunk); err != nil {
		t.Fatalf("push input chunk: %v", err)
	}
}

func collectSpeechOutput(t *testing.T, output genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
				return chunks
			}
			t.Fatalf("read transformer output: %v", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}

func speechChunkError(chunk *genx.MessageChunk) error {
	if chunk == nil || chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.Error) == "" {
		return nil
	}
	return fmt.Errorf("stream %q returned error: %s", chunk.Ctrl.StreamID, chunk.Ctrl.Error)
}
