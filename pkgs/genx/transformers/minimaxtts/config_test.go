package minimaxtts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/minimax-go"
	"github.com/coder/websocket"
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
	server := newMiniMaxTTSTestServer(t, requestBody)
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
	emissions := 0
	if err := transformer.synthesize(context.Background(), "hello", streamkit.TTSMeta{}, "audio/pcm", func(data []byte) error {
		if len(data) > 0 {
			emissions++
		}
		audio = append(audio, data...)
		return nil
	}); err != nil {
		t.Fatalf("synthesize() error = %v", err)
	}
	if !bytes.Equal(audio, []byte{1, 2}) || emissions != 2 {
		t.Fatalf("streamed audio = %v in %d emissions, want [1 2] in 2", audio, emissions)
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
	server := newMiniMaxTTSTestServer(t, nil)
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

func newMiniMaxTTSTestServer(t *testing.T, requests chan<- map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(w, request, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		if err := writeMiniMaxWebSocketJSON(conn, map[string]any{
			"event":     "connected_success",
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		}); err != nil {
			t.Errorf("write connected event: %v", err)
			return
		}
		start, err := readMiniMaxWebSocketJSON(conn)
		if err != nil {
			t.Errorf("read start message: %v", err)
			return
		}
		if err := writeMiniMaxWebSocketJSON(conn, map[string]any{
			"event":     "task_started",
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		}); err != nil {
			t.Errorf("write started event: %v", err)
			return
		}
		continueMessage, err := readMiniMaxWebSocketJSON(conn)
		if err != nil {
			t.Errorf("read continue message: %v", err)
			return
		}
		start["text"] = continueMessage["text"]
		if requests != nil {
			requests <- start
		}
		if _, err := readMiniMaxWebSocketJSON(conn); err != nil {
			t.Errorf("read finish message: %v", err)
			return
		}
		for _, audio := range []string{"01", "02"} {
			if err := writeMiniMaxWebSocketJSON(conn, map[string]any{
				"event":     "task_result",
				"data":      map[string]any{"audio": audio},
				"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
			}); err != nil {
				t.Errorf("write audio event: %v", err)
				return
			}
		}
		if err := writeMiniMaxWebSocketJSON(conn, map[string]any{
			"event":     "task_finished",
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		}); err != nil {
			t.Errorf("write finished event: %v", err)
		}
	}))
}

func writeMiniMaxWebSocketJSON(conn *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.Write(context.Background(), websocket.MessageText, data)
}

func readMiniMaxWebSocketJSON(conn *websocket.Conn) (map[string]any, error) {
	_, data, err := conn.Read(context.Background())
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
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
