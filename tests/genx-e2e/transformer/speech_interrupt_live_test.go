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
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoasr"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoast"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts"
	"github.com/GizClaw/minimax-go"
)

func TestDoubaoASTLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	pacing := false
	transformer, err := doubaoast.New(doubaoast.Config{
		Client: liveDoubaoClient(t), Mode: doubaospeech.ASTTranslateModeS2S,
		InputMode: doubaoast.InputModeRealtime, SourceLanguage: "zhen", TargetLanguage: "zhen",
		SourceLanguageDetect: true, SpeakerID: "zh_female_xiaohe_uranus_bigtts", RealtimePacing: &pacing,
	})
	if err != nil {
		t.Fatalf("doubaoast.New() failed: %v", err)
	}
	runLiveAudioRepeatedInterrupt(t, transformer, "doubao-ast", true, true, false, 10)
}

func TestDoubaoASRLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	pacing := false
	transformer, err := doubaoasr.New(doubaoasr.Config{
		Client: liveDoubaoClient(t), EmitInterim: true, RealtimePacing: &pacing,
	})
	if err != nil {
		t.Fatalf("doubaoasr.New() failed: %v", err)
	}
	runLiveTranscriptRepeatedInterrupt(t, transformer, "doubao-asr")
}

func TestChatroomLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	pacing := false
	asr, err := doubaoasr.New(doubaoasr.Config{
		Client: liveDoubaoClient(t), EmitInterim: true, RealtimePacing: &pacing,
	})
	if err != nil {
		t.Fatalf("doubaoasr.New() failed: %v", err)
	}
	mux := transformers.NewMux()
	if err := mux.Handle("asr?emit_interim=true", asr); err != nil {
		t.Fatalf("register ASR: %v", err)
	}
	transformer, err := chatroom.New(chatroom.Config{
		ASR: mux, TranscriptEnabled: true, ASRPattern: "asr", InputMode: chatroom.InputModeRealtime,
	})
	if err != nil {
		t.Fatalf("chatroom.New() failed: %v", err)
	}
	runLiveTranscriptRepeatedInterrupt(t, transformer, "chatroom")
}

func TestDoubaoSeedV2TTSLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	transformer, err := doubaotts.NewSeedV2(doubaotts.SeedV2Config{
		Client: liveDoubaoClient(t), Speaker: "zh_female_xiaohe_uranus_bigtts",
	})
	if err != nil {
		t.Fatalf("doubaotts.NewSeedV2() failed: %v", err)
	}
	runLiveTTSRepeatedInterrupt(t, transformer, "doubao-tts", "audio/ogg")
}

func TestMiniMaxTTSLiveRepeatedInterrupt(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(miniMaxAPIKeyEnv)
	baseURL := firstEnv(miniMaxBaseURLEnv)
	if apiKey == "" || baseURL == "" {
		t.Fatalf("set %s and %s in tests/genx-e2e/.env", miniMaxAPIKeyEnv, miniMaxBaseURLEnv)
	}
	client, err := minimax.NewClient(minimax.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		t.Fatalf("minimax.NewClient() failed: %v", err)
	}
	transformer, err := minimaxtts.New(minimaxtts.Config{Client: client, VoiceID: "female-shaonv"})
	if err != nil {
		t.Fatalf("minimaxtts.New() failed: %v", err)
	}
	runLiveTTSRepeatedInterrupt(t, transformer, "minimax-tts", "audio/mpeg")
}

func runLiveTranscriptRepeatedInterrupt(t *testing.T, transformer genx.Transformer, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	packets := embeddedPromptOpusPackets(t)
	ended := make(map[string]string)
	startTurn := func(round int) string {
		t.Helper()
		streamID := fmt.Sprintf("%s-input-%d", prefix, round)
		if err := input.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser, Part: &genx.Blob{MIMEType: duplexInputMIME},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true},
		}); err != nil {
			t.Fatalf("push round %d BOS: %v", round, err)
		}
		for _, packet := range packets {
			if err := input.Push(ctx, &genx.MessageChunk{
				Role: genx.RoleUser, Part: &genx.Blob{MIMEType: duplexInputMIME, Data: append([]byte(nil), packet...)},
				Ctrl: &genx.StreamCtrl{StreamID: streamID},
			}); err != nil {
				t.Fatalf("push round %d audio: %v", round, err)
			}
		}
		return streamID
	}
	waitText := func(streamID, previousID string) {
		t.Helper()
		for {
			chunk, err := output.Next()
			if err != nil {
				t.Fatalf("read transcript output: %v", err)
			}
			if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != "transcript" {
				continue
			}
			if chunk.IsEndOfStream() {
				ended[chunk.Ctrl.StreamID] = chunk.Ctrl.Error
				continue
			}
			if chunk.IsBeginOfStream() && previousID != "" && ended[previousID] != "interrupted" {
				t.Fatalf("replacement transcript BOS %q arrived before %q interrupted EOS; ended=%#v", chunk.Ctrl.StreamID, previousID, ended)
			}
			text, ok := chunk.Part.(genx.Text)
			if ok && strings.TrimSpace(string(text)) != "" && chunk.Ctrl.StreamID == streamID {
				return
			}
		}
	}

	firstID := startTurn(1)
	waitText(firstID, "")
	secondID := startTurn(2)
	waitText(secondID, firstID)
	thirdID := startTurn(3)
	waitText(thirdID, secondID)
	if err := input.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleUser, Part: &genx.Blob{MIMEType: duplexInputMIME},
		Ctrl: &genx.StreamCtrl{StreamID: thirdID, EndOfStream: true},
	}); err != nil {
		t.Fatalf("push final EOS: %v", err)
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
			t.Fatalf("drain transcript output: %v", err)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" && chunk.IsEndOfStream() {
			ended[chunk.Ctrl.StreamID] = chunk.Ctrl.Error
		}
	}
	if ended[firstID] != "interrupted" || ended[secondID] != "interrupted" {
		t.Fatalf("transcript interruption errors = %q/%q, want interrupted/interrupted", ended[firstID], ended[secondID])
	}
	if errorText, ok := ended[thirdID]; !ok || errorText != "" {
		t.Fatalf("final transcript EOS = %q, present=%t", errorText, ok)
	}
	t.Logf("transcript routes=%q,%q,%q", firstID, secondID, thirdID)
}

func runLiveTTSRepeatedInterrupt(t *testing.T, transformer genx.Transformer, prefix, wantMIME string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	ended := make(map[string]string)
	startTurn := func(round int) string {
		t.Helper()
		streamID := fmt.Sprintf("%s-input-%d", prefix, round)
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("这是一段用于验证语音合成中断边界的较长文本。现在开始逐项说明第一点、第二点、第三点，并继续生成足够长的音频。"), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"}},
		} {
			if err := input.Push(ctx, chunk); err != nil {
				t.Fatalf("push TTS round %d: %v", round, err)
			}
		}
		return streamID
	}
	waitAudio := func(streamID, previousID string) {
		t.Helper()
		for {
			chunk, err := output.Next()
			if err != nil {
				t.Fatalf("read TTS output: %v", err)
			}
			if chunk == nil || chunk.Ctrl == nil {
				continue
			}
			if chunk.IsEndOfStream() {
				ended[chunk.Ctrl.StreamID] = chunk.Ctrl.Error
				continue
			}
			if chunk.IsBeginOfStream() && previousID != "" && chunk.Ctrl.StreamID != previousID && ended[previousID] != "interrupted" {
				t.Fatalf("replacement TTS BOS %q arrived before %q interrupted EOS; ended=%#v", chunk.Ctrl.StreamID, previousID, ended)
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if ok && chunk.Ctrl.StreamID == streamID && blob.MIMEType == wantMIME && len(blob.Data) > 0 {
				return
			}
		}
	}
	interrupt := func(streamID string) {
		t.Helper()
		if err := input.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", Error: "interrupted"},
		}); err != nil {
			t.Fatalf("interrupt TTS route %q: %v", streamID, err)
		}
	}

	firstID := startTurn(1)
	waitAudio(firstID, "")
	interrupt(firstID)
	secondID := startTurn(2)
	waitAudio(secondID, firstID)
	interrupt(secondID)
	thirdID := startTurn(3)
	waitAudio(thirdID, secondID)
	if err := input.Push(ctx, &genx.MessageChunk{
		Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: thirdID, Label: "assistant", EndOfStream: true},
	}); err != nil {
		t.Fatalf("push final TTS EOS: %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close TTS input: %v", err)
	}
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			break
		}
		if err != nil {
			t.Fatalf("drain TTS output: %v", err)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.IsEndOfStream() {
			ended[chunk.Ctrl.StreamID] = chunk.Ctrl.Error
		}
	}
	if ended[firstID] != "interrupted" || ended[secondID] != "interrupted" {
		t.Fatalf("TTS interruption errors = %q/%q, want interrupted/interrupted", ended[firstID], ended[secondID])
	}
	if errorText, ok := ended[thirdID]; !ok || errorText != "" {
		t.Fatalf("final TTS EOS = %q, present=%t", errorText, ok)
	}
	t.Logf("TTS routes=%q,%q,%q mime=%s", firstID, secondID, thirdID, wantMIME)
}
