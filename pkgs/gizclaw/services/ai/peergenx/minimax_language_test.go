package peergenx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/coder/websocket"
)

// TestDefaultBuilderSendsMiniMaxLanguageBoostFromVoicePattern checks that a
// Voice pattern language parameter, as sent by AST translation for its target
// language, reaches MiniMax as language_boost.
func TestDefaultBuilderSendsMiniMaxLanguageBoostFromVoicePattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    any
	}{
		{pattern: "voice/translator?language=ja", want: "Japanese"},
		{pattern: "voice/translator?language=fr", want: "French"},
		{pattern: "voice/translator?language=es", want: "Spanish"},
		{pattern: "voice/translator", want: nil},
		{pattern: "voice/translator?language=xx", want: nil},
	}
	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			starts := make(chan map[string]any, 1)
			server := newMiniMaxLanguageTestServer(t, starts)
			defer server.Close()

			_, params, err := splitPatternParams(test.pattern)
			if err != nil {
				t.Fatalf("splitPatternParams() error = %v", err)
			}
			transformer, err := (DefaultBuilder{}).BuildTransformer(context.Background(), TransformerConfig{
				Pattern: test.pattern,
				Voice: &apitypes.Voice{
					Id: "translator",
					ProviderData: mustMiniMaxVoiceProviderData(t, apitypes.MiniMaxTenantVoiceProviderData{
						VoiceId: new("voice-id"),
						Model:   new("speech-2.8-turbo"),
					}),
				},
				Tenant: Tenant{
					Kind:    string(apitypes.VoiceProviderKindMinimaxTenant),
					MiniMax: &apitypes.MiniMaxTenant{Id: "main", BaseUrl: new(server.URL)},
				},
				Credential: apitypes.Credential{Id: "minimax-key", Body: testMiniMaxCredentialBody("sk-test")},
				Params:     params,
			})
			if err != nil {
				t.Fatalf("BuildTransformer() error = %v", err)
			}
			output, err := transformer.Transform(context.Background(), &miniMaxLanguageTestStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("今日は東京駅で会いましょう。"), Ctrl: &genx.StreamCtrl{StreamID: "tts"}},
				{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "tts", EndOfStream: true}},
			}})
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			defer output.Close()
			for {
				if _, err := output.Next(); err != nil {
					break
				}
			}
			start := <-starts
			if start["language_boost"] != test.want {
				t.Fatalf("language_boost = %#v, want %#v", start["language_boost"], test.want)
			}
		})
	}
}

func newMiniMaxLanguageTestServer(t *testing.T, starts chan<- map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(w, request, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		ctx := request.Context()
		ok := map[string]any{"status_code": 0, "status_msg": "success"}
		write := func(value map[string]any) bool {
			data, _ := json.Marshal(value)
			return conn.Write(ctx, websocket.MessageText, data) == nil
		}
		read := func() (map[string]any, bool) {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return nil, false
			}
			var value map[string]any
			return value, json.Unmarshal(data, &value) == nil
		}
		if !write(map[string]any{"event": "connected_success", "base_resp": ok}) {
			return
		}
		start, valid := read()
		if !valid {
			t.Errorf("read start message")
			return
		}
		starts <- start
		if !write(map[string]any{"event": "task_started", "base_resp": ok}) {
			return
		}
		if _, valid := read(); !valid { // task_continue
			return
		}
		if _, valid := read(); !valid { // task_finish
			return
		}
		_ = write(map[string]any{"event": "task_result", "data": map[string]any{"audio": "0102"}, "base_resp": ok})
		_ = write(map[string]any{"event": "task_finished", "base_resp": ok})
	}))
}

type miniMaxLanguageTestStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func (s *miniMaxLanguageTestStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (*miniMaxLanguageTestStream) Close() error               { return nil }
func (*miniMaxLanguageTestStream) CloseWithError(error) error { return nil }
