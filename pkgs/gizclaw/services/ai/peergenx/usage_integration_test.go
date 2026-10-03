package peergenx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerusage"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/coder/websocket"
	"github.com/jmoiron/sqlx"
)

func TestConfiguredGeneratorPersistsProviderUsage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" || request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected provider request: %s %s", request.Method, request.URL.Path)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode provider request: %v", err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if body["model"] != "billing-chat" {
			t.Errorf("provider model = %v, want billing-chat", body["model"])
		}
		if body["stream"] == true {
			options, _ := body["stream_options"].(map[string]any)
			if options["include_usage"] != true {
				t.Error("streaming request did not ask for provider usage")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, err := io.WriteString(w, "data: "+`{"id":"stream","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":""}]}`+"\n\n"+
				"data: "+`{"id":"stream","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n"+
				"data: "+`{"id":"stream","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"prompt_tokens_details":{"cached_tokens":3}}}`+"\n\n"+
				"data: [DONE]\n\n")
			if err != nil {
				t.Errorf("write provider stream: %v", err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"id":"invoke","object":"chat.completion","created":1,"model":"billing-chat","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11,"prompt_tokens_details":{"cached_tokens":2}}}`); err != nil {
			t.Errorf("write provider response: %v", err)
		}
	}))
	defer server.Close()
	store, meter, peer, hour := newConfiguredUsageStore(t)
	service := New(Service{
		Models: responseModels{response: adminhttp.GetModel200JSONResponse(apitypes.Model{
			Id:       "catalog-chat",
			Kind:     apitypes.ModelKindLlm,
			Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindOpenaiTenant, Id: "tenant"},
			ProviderData: mustOpenAIModelProviderData(t, apitypes.OpenAITenantModelProviderData{
				UpstreamModel:     "billing-chat",
				SupportJsonOutput: new(true),
			}),
		})},
		Credentials: responseCredentials{response: adminhttp.GetCredential200JSONResponse(apitypes.Credential{
			Id: "key", Body: testOpenAICredentialBody("test-key"),
		})},
		ProviderTenants: responseTenants{openai: adminhttp.GetOpenAITenant200JSONResponse(apitypes.OpenAITenant{
			Id: "tenant", CredentialId: "key", BaseUrl: new(server.URL + "/"),
		})},
		Usage: meter.Handler(peer),
	})
	var input genx.ModelContextBuilder
	input.UserText("user", "hello")
	stream, err := service.Generator().GenerateStream(ctx, "model/catalog-chat", input.Build())
	if err != nil {
		t.Fatal(err)
	}
	text, _ := consumeConfiguredUsageStream(t, stream)
	if text != "hello" {
		t.Fatalf("generator text = %q", text)
	}
	assertConfiguredUsage(t, ctx, store, meter, peer, hour, "billing-chat", 14)
	tool := genx.MustNewFuncTool[struct {
		OK bool `json:"ok"`
	}]("answer", "Return an answer")
	_, call, err := service.Generator().Invoke(ctx, "model/catalog-chat", input.Build(), tool)
	if err != nil {
		t.Fatal(err)
	}
	if call == nil || call.Arguments != `{"ok":true}` {
		t.Fatalf("generator call = %+v", call)
	}
	assertConfiguredUsage(t, ctx, store, meter, peer, hour, "billing-chat", 25)
}

func TestConfiguredTransformerAndSpeechPersistProviderUsage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	server := newConfiguredUsageSpeechServer(t)
	defer server.Close()
	store, meter, peer, hour := newConfiguredUsageStore(t)
	service := New(Service{
		Voices: configuredUsageVoices{voice: apitypes.Voice{
			Id:       "catalog-voice",
			Provider: apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindMinimaxTenant, Id: "tenant"},
			ProviderData: mustMiniMaxVoiceProviderData(t, apitypes.MiniMaxTenantVoiceProviderData{
				VoiceId: new("speaker-id"), Model: new("billing-speech"),
			}),
		}},
		Credentials: responseCredentials{response: adminhttp.GetCredential200JSONResponse(apitypes.Credential{
			Id: "key", Body: testMiniMaxCredentialBody("test-key"),
		})},
		ProviderTenants: responseTenants{minimax: adminhttp.GetMiniMaxTenant200JSONResponse(apitypes.MiniMaxTenant{
			Id: "tenant", CredentialId: "key", BaseUrl: new(server.URL),
		})},
		Usage: meter.Handler(peer),
	})
	stream, err := service.Transformer().Transform(ctx, "voice/narrator?format=pcm", newTextStream("hello"))
	if err != nil {
		t.Fatal(err)
	}
	_, audio := consumeConfiguredUsageStream(t, stream)
	if !bytes.Equal(audio, []byte{1, 2, 3, 4}) {
		t.Fatalf("transformer audio = %x", audio)
	}
	assertConfiguredUsage(t, ctx, store, meter, peer, hour, "billing-speech", 37)
	result, err := service.Synthesize(ctx, "narrator", "hello", []string{"audio/pcm"})
	if err != nil {
		t.Fatal(err)
	}
	_, audio = consumeConfiguredUsageStream(t, result.Stream)
	if result.ContentType != "audio/pcm" || !bytes.Equal(audio, []byte{1, 2, 3, 4}) {
		t.Fatalf("speech content type = %q, audio = %x", result.ContentType, audio)
	}
	assertConfiguredUsage(t, ctx, store, meter, peer, hour, "billing-speech", 74)
}

type configuredUsageVoices struct{ voice apitypes.Voice }

func (v configuredUsageVoices) ResolveVoiceAlias(alias string) (string, bool) {
	return v.voice.Id, alias == "narrator"
}

func (v configuredUsageVoices) GetVoice(_ context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	if request.Id != v.voice.Id {
		return adminhttp.GetVoice404JSONResponse{}, nil
	}
	return adminhttp.GetVoice200JSONResponse(v.voice), nil
}

func newConfiguredUsageSpeechServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		conn, err := websocket.Accept(w, request, nil)
		if err != nil {
			t.Errorf("accept speech websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
		defer cancel()
		write := func(value map[string]any) bool {
			data, err := json.Marshal(value)
			if err == nil {
				err = conn.Write(ctx, websocket.MessageText, data)
			}
			if err != nil {
				t.Errorf("write speech event: %v", err)
			}
			return err == nil
		}
		read := func(event string) map[string]any {
			_, data, err := conn.Read(ctx)
			var value map[string]any
			if err == nil {
				err = json.Unmarshal(data, &value)
			}
			if err != nil || value["event"] != event {
				t.Errorf("speech event = %v, error = %v, want %s", value, err, event)
				return nil
			}
			return value
		}
		ok := map[string]any{"status_code": 0, "status_msg": "success"}
		if !write(map[string]any{"event": "connected_success", "base_resp": ok}) {
			return
		}
		start := read("task_start")
		if start == nil {
			return
		}
		voice, _ := start["voice_setting"].(map[string]any)
		if start["model"] != "billing-speech" || voice["voice_id"] != "speaker-id" {
			t.Errorf("speech model = %v, voice = %v", start["model"], voice)
		}
		if !write(map[string]any{"event": "task_started", "base_resp": ok}) {
			return
		}
		continued := read("task_continue")
		if continued == nil {
			return
		}
		if continued["text"] != "hello" {
			t.Errorf("speech text = %v", continued["text"])
		}
		if read("task_finish") == nil {
			return
		}
		if !write(map[string]any{"event": "task_result", "data": map[string]any{"audio": "01020304"}, "base_resp": ok}) {
			return
		}
		// Deliberately differs from the input's character count: billing must
		// use the provider-reported quantity rather than estimate from text.
		if !write(map[string]any{"event": "task_continued", "is_final": true, "data": map[string]any{"audio": ""}, "extra_info": map[string]any{"usage_characters": 37}, "base_resp": ok}) {
			return
		}
		write(map[string]any{"event": "task_finished", "base_resp": ok})
	}))
}

func newConfiguredUsageStore(t *testing.T) (*peerusage.Store, *peerusage.Recorder, giznet.PublicKey, time.Time) {
	t.Helper()
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := peerusage.NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	store.Now = func() time.Time { return hour }
	meter := peerusage.NewRecorder(store)
	meter.Now = store.Now
	return store, meter, giznet.PublicKey{42}, hour
}

func consumeConfiguredUsageStream(t *testing.T, stream genx.Stream) (string, []byte) {
	t.Helper()
	defer stream.Close()
	var text, audio []byte
	for {
		chunk, err := stream.Next()
		if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
			return string(text), audio
		}
		if err != nil {
			t.Fatal(err)
		}
		if chunk == nil {
			continue
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			text = append(text, string(part)...)
		case *genx.Blob:
			audio = append(audio, part.Data...)
		}
	}
}

func assertConfiguredUsage(t *testing.T, ctx context.Context, store *peerusage.Store, meter *peerusage.Recorder, peer giznet.PublicKey, hour time.Time, model string, quantity int64) {
	t.Helper()
	if err := meter.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Query(ctx, peer, "", hour, hour.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Peer != peer || rows[0].ModelID != model || !rows[0].Hour.Equal(hour) || rows[0].Quantity != quantity {
		t.Fatalf("persisted rows = %+v, want %s quantity %d", rows, model, quantity)
	}
	other, err := store.Query(ctx, giznet.PublicKey{43}, "", hour, hour.Add(time.Hour))
	if err != nil || len(other) != 0 {
		t.Fatalf("another Peer's rows = %+v, error = %v", other, err)
	}
}
