//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoasr"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts"
	"github.com/GizClaw/minimax-go"
)

const (
	miniMaxAPIKeyEnv  = "GIZCLAW_GENX_E2E_MINIMAX_API_KEY"
	miniMaxBaseURLEnv = "GIZCLAW_GENX_E2E_MINIMAX_BASE_URL"
	miniMaxVoiceIDEnv = "GIZCLAW_GENX_E2E_MINIMAX_VOICE_ID"
)

func TestDoubaoSAUCASR(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv, "GIZCLAW_E2E_DOUBAO_APP_ID")
	apiKey := firstEnv(doubaoAPIKeyEnv, "GIZCLAW_E2E_DOUBAO_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skipf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
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
	seenEOS := false
	for _, chunk := range collectSpeechOutput(t, output) {
		if err := speechChunkError(chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID {
			t.Fatalf("ASR chunk route = %#v, want stream %q", chunk.Ctrl, streamID)
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			transcript.WriteString(string(text))
		}
		if chunk.IsEndOfStream() {
			seenEOS = true
		}
	}
	if strings.TrimSpace(transcript.String()) == "" {
		t.Fatal("Doubao SAUC returned no transcript")
	}
	if !seenEOS {
		t.Fatal("Doubao SAUC returned no terminal EOS")
	}
	t.Logf("transcript=%q", transcript.String())
}

func TestDoubaoSeedV2TTS(t *testing.T) {
	loadGenXE2EEnv(t)
	appID := firstEnv(doubaoAppIDEnv, "GIZCLAW_E2E_DOUBAO_APP_ID")
	apiKey := firstEnv(doubaoAPIKeyEnv, "GIZCLAW_E2E_DOUBAO_API_KEY")
	if appID == "" || apiKey == "" {
		t.Skipf("set %s and %s in tests/genx-e2e/.env to run this provider e2e test", doubaoAppIDEnv, doubaoAPIKeyEnv)
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
	apiKey := firstEnv(miniMaxAPIKeyEnv, "GIZCLAW_E2E_MINIMAX_GLOBAL_API_KEY", "GIZCLAW_E2E_MINIMAX_API_KEY")
	if apiKey == "" {
		t.Skipf("set %s in tests/genx-e2e/.env to run this provider e2e test", miniMaxAPIKeyEnv)
	}
	baseURL := firstEnv(miniMaxBaseURLEnv, "GIZCLAW_E2E_MINIMAX_GLOBAL_VOICE_BASE_URL", "GIZCLAW_E2E_MINIMAX_VOICE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.io"
	}
	voiceID := firstEnv(miniMaxVoiceIDEnv)
	if voiceID == "" {
		voiceID = "female-shaonv"
	}
	client, err := minimax.NewClient(minimax.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		t.Fatalf("minimax.NewClient() failed: %v", err)
	}
	transformer, err := minimaxtts.New(minimaxtts.Config{Client: client, VoiceID: voiceID})
	if err != nil {
		t.Fatalf("minimaxtts.New() failed: %v", err)
	}
	runTTSE2E(t, transformer, "minimax-tts-e2e", "你好，这是一条 MiniMax 语音合成端到端测试。", "audio/mpeg")
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

	pushSpeechChunk(t, ctx, input, &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: "assistant",
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", EndOfStream: true},
	})
	if err := input.Close(); err != nil {
		t.Fatalf("close TTS input: %v", err)
	}

	audioBytes := 0
	seenEOS := false
	for _, chunk := range collectSpeechOutput(t, output) {
		if err := speechChunkError(chunk); err != nil {
			t.Fatal(err)
		}
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
		if chunk.IsEndOfStream() {
			seenEOS = true
		}
	}
	if audioBytes == 0 {
		t.Fatal("TTS returned no audio bytes")
	}
	if !seenEOS {
		t.Fatal("TTS returned no terminal EOS")
	}
	t.Logf("audio_bytes=%d mime=%s", audioBytes, wantMIME)
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
