package peergenx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			starts := make(chan map[string]any, 1)
			server := newMiniMaxLanguageTestServer(t, starts)
			defer server.Close()

			_, params, err := splitPatternParams(test.pattern)
			if err != nil {
				t.Fatalf("splitPatternParams() error = %v", err)
			}
			transformer, err := (DefaultBuilder{}).BuildTransformer(ctx, TransformerConfig{
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
			output, err := transformer.Transform(ctx, &miniMaxLanguageTestStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("今日は東京駅で会いましょう。"), Ctrl: &genx.StreamCtrl{StreamID: "tts"}},
				{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "tts", EndOfStream: true}},
			}})
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			defer output.Close()
			for {
				if _, err := output.Next(); err != nil {
					if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
						t.Fatal(err)
					}
					break
				}
			}
			var start map[string]any
			select {
			case start = <-starts:
			case <-ctx.Done():
				t.Fatal("provider did not receive a speech request")
			}
			if start["language_boost"] != test.want {
				t.Fatalf("language_boost = %#v, want %#v", start["language_boost"], test.want)
			}
		})
	}
}

func newMiniMaxLanguageTestServer(t *testing.T, starts chan<- map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(io.LimitReader(request.Body, 1<<20)).Decode(&body); err != nil {
			t.Errorf("decode speech request: %v", err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		options, _ := body["stream_options"].(map[string]any)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/t2a_v2" || body["stream"] != true || options["exclude_aggregated_audio"] != true {
			t.Errorf("unexpected speech request: %s %s %v", request.Method, request.URL.Path, body)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		starts <- body
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"data\":{\"audio\":\"0102\",\"status\":1}}\n\ndata: {\"data\":{\"status\":2}}\n\n"); err != nil {
			t.Errorf("write speech event: %v", err)
		}
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
