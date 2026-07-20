package minimaxtts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/internal/streamkit"
	"github.com/GizClaw/minimax-go"
)

func TestConfigValidationDefaultsAndPointerCopies(t *testing.T) {
	if _, err := New(Config{VoiceID: "voice"}); err == nil {
		t.Fatal("New() accepted a nil client")
	}
	client, err := minimax.NewClient(minimax.Config{APIKey: "test"})
	if err != nil {
		t.Fatalf("minimax.NewClient() error = %v", err)
	}
	if _, err := New(Config{Client: client}); err == nil {
		t.Fatal("New() accepted an empty voice ID")
	}
	transformer, err := New(Config{Client: client, VoiceID: " voice "})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if transformer.voiceID != "voice" || transformer.model != "speech-2.6-hd" || transformer.format != "mp3" || transformer.sampleRate != 32000 || transformer.bitrate != 128000 {
		t.Fatalf("defaults = %#v", transformer)
	}
	if transformer.speed != 1 || transformer.vol != 1 || transformer.pitch != 0 {
		t.Fatalf("voice defaults = %v/%v/%v", transformer.speed, transformer.vol, transformer.pitch)
	}

	speed := 0.0
	volume := 0.0
	pitch := -2
	configured, err := New(Config{Client: client, VoiceID: "voice", Speed: &speed, Volume: &volume, Pitch: &pitch})
	if err != nil {
		t.Fatalf("New(explicit values) error = %v", err)
	}
	speed, volume, pitch = 2, 2, 2
	if configured.speed != 0 || configured.vol != 0 || configured.pitch != -2 {
		t.Fatalf("configured values changed after source pointer mutation: %#v", configured)
	}
}

func TestSynthesizeMapsTypedConfigToProviderRequest(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_resp":{"status_code":0,"status_msg":"ok"},"data":{"audio_hex":"0102"}}`))
	}))
	defer server.Close()

	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("minimax.NewClient() error = %v", err)
	}
	speed := 0.75
	volume := 1.5
	pitch := -2
	transformer, err := New(Config{
		Client:     client,
		VoiceID:    "voice",
		Model:      "speech-model",
		Speed:      &speed,
		Volume:     &volume,
		Pitch:      &pitch,
		Emotion:    "happy",
		Format:     "pcm",
		SampleRate: 16000,
		BitRate:    64000,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var audio []byte
	if err := transformer.synthesize(context.Background(), "hello", streamkit.TTSMeta{}, "audio/pcm", func(data []byte) error {
		audio = append(audio, data...)
		return nil
	}); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if len(audio) == 0 {
		t.Fatal("synthesize() emitted no audio")
	}

	body := <-requestBody
	if body["model"] != "speech-model" || body["text"] != "hello" {
		t.Fatalf("request identity = %#v", body)
	}
	voice, ok := body["voice_setting"].(map[string]any)
	if !ok || voice["voice_id"] != "voice" || voice["emotion"] != "happy" || voice["speed"] != speed || voice["vol"] != volume || voice["pitch"] != float64(pitch) {
		t.Fatalf("voice_setting = %#v", body["voice_setting"])
	}
	audioSetting, ok := body["audio_setting"].(map[string]any)
	if !ok || audioSetting["format"] != "pcm" || audioSetting["sample_rate"] != float64(16000) || audioSetting["bitrate"] != float64(64000) {
		t.Fatalf("audio_setting = %#v", body["audio_setting"])
	}
}

func TestTransformerConcurrentCallsAreIndependent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"base_resp":{"status_code":0,"status_msg":"ok"},"data":{"audio_hex":"0102"}}`))
	}))
	defer server.Close()

	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, APIKey: "test", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("minimax.NewClient() error = %v", err)
	}
	transformer, err := New(Config{Client: client, VoiceID: "voice"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for index := range 2 {
		wg.Go(func() {
			streamID := fmt.Sprintf("tts-%d", index)
			input := &configTestStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("hello."), Ctrl: &genx.StreamCtrl{StreamID: streamID}},
				{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
			}}
			output, transformErr := transformer.Transform(context.Background(), input)
			if transformErr != nil {
				errors <- transformErr
				return
			}
			for chunkIndex := range 2 {
				chunk, nextErr := output.Next()
				if nextErr != nil {
					errors <- fmt.Errorf("%s chunk %d: %w", streamID, chunkIndex, nextErr)
					return
				}
				if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID {
					errors <- fmt.Errorf("%s chunk %d control = %#v", streamID, chunkIndex, chunk.Ctrl)
					return
				}
			}
			if _, nextErr := output.Next(); nextErr != io.EOF {
				errors <- fmt.Errorf("%s terminal error = %v", streamID, nextErr)
			}
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

type configTestStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func (s *configTestStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (*configTestStream) Close() error               { return nil }
func (*configTestStream) CloseWithError(error) error { return nil }
