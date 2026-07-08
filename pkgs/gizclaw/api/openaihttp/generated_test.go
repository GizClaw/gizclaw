package openaihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

var _ StrictServerInterface = (*Stub)(nil)

type Stub struct{}

func RegisterStubHandlers(router fiber.Router, baseURL string) {
	RegisterHandlersWithOptions(router, NewStrictHandler(&Stub{}, nil), FiberServerOptions{
		BaseURL: baseURL,
	})
}

func (s *Stub) ListModels(context.Context, ListModelsRequestObject) (ListModelsResponseObject, error) {
	return ListModels200JSONResponse{
		Object: "list",
		Data: []Model{
			{
				Id:      "genx-stub",
				Object:  ModelObjectModel,
				Created: 0,
				OwnedBy: "genx",
			},
		},
	}, nil
}

func (s *Stub) CreateChatCompletion(_ context.Context, request CreateChatCompletionRequestObject) (CreateChatCompletionResponseObject, error) {
	model := "genx-stub"
	if request.Body != nil && request.Body.Stream != nil && *request.Body.Stream {
		chunk, err := json.Marshal(CreateChatCompletionStreamResponse{
			Id:      "chatcmpl-genx-stub",
			Object:  ChatCompletionChunk,
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: chatCompletionStreamDelta("stub"),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		body := []byte(fmt.Sprintf("data: %s\n\ndata: [DONE]\n\n", chunk))
		return CreateChatCompletion200TexteventStreamResponse{
			Body:          bytes.NewReader(body),
			ContentLength: int64(len(body)),
		}, nil
	}

	return CreateChatCompletion200JSONResponse{
		Id:      "chatcmpl-genx-stub",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{
			{
				FinishReason: stringPtr("stop"),
				Index:        0,
				Message:      chatCompletionMessage("genx openai-compatible stub"),
			},
		},
	}, nil
}

func (s *Stub) CreateSpeech(_ context.Context, request CreateSpeechRequestObject) (CreateSpeechResponseObject, error) {
	if request.Body != nil && request.Body.Stream != nil && *request.Body.Stream {
		audio := "Z2VueCBzcGVlY2ggc3R1Yg=="
		done := true
		body, err := serverSentEvents(
			CreateSpeechResponseStreamEvent{Type: "speech.audio.delta", Audio: &audio},
			CreateSpeechResponseStreamEvent{Type: "speech.audio.done", Done: &done},
		)
		if err != nil {
			return nil, err
		}
		return CreateSpeech200TexteventStreamResponse{
			Body:          bytes.NewReader(body),
			ContentLength: int64(len(body)),
		}, nil
	}
	body := []byte("genx speech stub\n")
	return CreateSpeech200ApplicationoctetStreamResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
	}, nil
}

func (s *Stub) CreateTranscription(_ context.Context, request CreateTranscriptionRequestObject) (CreateTranscriptionResponseObject, error) {
	if transcriptionRequestWantsStream(request) {
		delta := "genx "
		text := "genx transcription stub"
		body, err := serverSentEvents(
			CreateTranscriptionResponseStreamEvent{Type: "transcript.text.delta", Delta: &delta},
			CreateTranscriptionResponseStreamEvent{Type: "transcript.text.done", Text: &text},
		)
		if err != nil {
			return nil, err
		}
		return CreateTranscription200TexteventStreamResponse{
			Body:          bytes.NewReader(body),
			ContentLength: int64(len(body)),
		}, nil
	}
	return CreateTranscription200JSONResponse{Text: "genx transcription stub"}, nil
}

func transcriptionRequestWantsStream(request CreateTranscriptionRequestObject) bool {
	if request.Body == nil {
		return false
	}
	for {
		part, err := request.Body.NextPart()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return false
		}
		if part.FormName() != "stream" {
			continue
		}
		body, err := io.ReadAll(part)
		return err == nil && string(body) == "true"
	}
}

func serverSentEvents(events ...any) ([]byte, error) {
	var body bytes.Buffer
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(&body, "data: %s\n\n", payload)
	}
	return body.Bytes(), nil
}

func chatCompletionMessage(content string) ChatCompletionResponseMessage {
	return ChatCompletionResponseMessage{
		Content: &content,
		Role:    ChatCompletionResponseMessageRoleAssistant,
	}
}

func chatCompletionStreamDelta(content string) ChatCompletionStreamResponseDelta {
	role := ChatCompletionStreamResponseDeltaRoleAssistant
	return ChatCompletionStreamResponseDelta{
		Content: &content,
		Role:    &role,
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestOpenAISDKAgainstMockServer(t *testing.T) {
	client := newOpenAITestClient(t)
	ctx := context.Background()

	t.Run("models", func(t *testing.T) {
		models, err := client.Models.List(ctx)
		requireNoOpenAIError(t, err)
		if len(models.Data) != 1 {
			t.Fatalf("models length = %d, want 1", len(models.Data))
		}
		if got := models.Data[0].ID; got != "genx-stub" {
			t.Fatalf("model id = %q, want %q", got, "genx-stub")
		}
	})

	t.Run("chat completion", func(t *testing.T) {
		completion, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    shared.ChatModelGPT4oMini,
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
		})
		requireNoOpenAIError(t, err)
		if completion.ID == "" {
			t.Fatal("completion id is empty")
		}
		if len(completion.Choices) != 1 {
			t.Fatalf("choices length = %d, want 1", len(completion.Choices))
		}
		if got := completion.Choices[0].Message.Content; got != "genx openai-compatible stub" {
			t.Fatalf("completion content = %q, want %q", got, "genx openai-compatible stub")
		}
	})

	t.Run("chat completion stream", func(t *testing.T) {
		stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Model:    shared.ChatModelGPT4oMini,
			Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")},
		})
		defer stream.Close()

		var content bytes.Buffer
		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}
			content.WriteString(chunk.Choices[0].Delta.Content)
		}
		requireNoOpenAIError(t, stream.Err())
		if got := content.String(); got != "stub" {
			t.Fatalf("stream content = %q, want %q", got, "stub")
		}
	})

	t.Run("tts", func(t *testing.T) {
		resp, err := client.Audio.Speech.New(ctx, openai.AudioSpeechNewParams{
			Input:          "hello",
			Model:          openai.SpeechModelTTS1,
			Voice:          openai.AudioSpeechNewParamsVoiceAlloy,
			ResponseFormat: openai.AudioSpeechNewParamsResponseFormatMP3,
		})
		requireNoOpenAIError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read speech body: %v", err)
		}
		if got, want := string(body), "genx speech stub\n"; got != want {
			t.Fatalf("speech body = %q, want %q", got, want)
		}
	})

	t.Run("asr", func(t *testing.T) {
		transcription, err := client.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
			File:           bytes.NewReader([]byte("fake wav data")),
			Model:          openai.AudioModelWhisper1,
			ResponseFormat: openai.AudioResponseFormatJSON,
		})
		requireNoOpenAIError(t, err)
		if got := transcription.Text; got != "genx transcription stub" {
			t.Fatalf("transcription text = %q, want %q", got, "genx transcription stub")
		}
	})

	t.Run("asr stream", func(t *testing.T) {
		stream := client.Audio.Transcriptions.NewStreaming(ctx, openai.AudioTranscriptionNewParams{
			File:           bytes.NewReader([]byte("fake wav data")),
			Model:          openai.AudioModelGPT4oTranscribe,
			ResponseFormat: openai.AudioResponseFormatJSON,
		})
		defer stream.Close()

		var delta, done string
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "transcript.text.delta":
				delta += event.Delta
			case "transcript.text.done":
				done = event.Text
			}
		}
		requireNoOpenAIError(t, stream.Err())
		if got := delta; got != "genx " {
			t.Fatalf("transcription stream delta = %q, want %q", got, "genx ")
		}
		if got := done; got != "genx transcription stub" {
			t.Fatalf("transcription stream done = %q, want %q", got, "genx transcription stub")
		}
	})
}

func TestOpenAIEventStreamResponses(t *testing.T) {
	app := fiber.New()
	RegisterStubHandlers(app, "/v1")

	assertEventStream(t, app, newJSONRequest(
		http.MethodPost,
		"/v1/chat/completions",
		`{"model":"m","messages":[],"stream":true}`,
	), "chat.completion.chunk")
	assertEventStream(t, app, newJSONRequest(
		http.MethodPost,
		"/v1/audio/speech",
		`{"model":"tts-1","input":"hello","voice":"alloy","stream":true}`,
	), "speech.audio.delta")
	assertEventStream(t, app, newMultipartRequest(t, "/v1/audio/transcriptions", map[string]string{
		"model":           "gpt-4o-transcribe",
		"response_format": "json",
		"stream":          "true",
	}), "transcript.text.delta")
}

func TestGeneratedHandlerErrorPaths(t *testing.T) {
	t.Run("server errors", func(t *testing.T) {
		app := fiber.New()
		RegisterHandlersWithOptions(app, NewStrictHandler(&errorServer{}, nil), FiberServerOptions{BaseURL: "/v1"})

		assertStatus(t, app, httptest.NewRequest(http.MethodGet, "/v1/models", nil), http.StatusBadRequest)
		assertStatus(t, app, newJSONRequest(http.MethodPost, "/v1/chat/completions", `{"model":"m","messages":[]}`), http.StatusBadRequest)
		assertStatus(t, app, newJSONRequest(http.MethodPost, "/v1/audio/speech", `{"model":"m","input":"x","voice":"v"}`), http.StatusBadRequest)
		assertStatus(t, app, httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader([]byte("multipart body"))), http.StatusBadRequest)
	})

	t.Run("middleware", func(t *testing.T) {
		app := fiber.New()
		calls := 0
		RegisterHandlersWithOptions(app, NewStrictHandler(&Stub{}, nil), FiberServerOptions{
			BaseURL: "/v1",
			Middlewares: []MiddlewareFunc{
				func(c *fiber.Ctx) error {
					calls++
					return c.Next()
				},
			},
		})

		assertStatus(t, app, httptest.NewRequest(http.MethodGet, "/v1/models", nil), http.StatusOK)
		if calls != 1 {
			t.Fatalf("middleware calls = %d, want 1", calls)
		}
	})

	t.Run("strict middleware", func(t *testing.T) {
		app := fiber.New()
		calls := 0
		RegisterHandlersWithOptions(app, NewStrictHandler(&Stub{}, []StrictMiddlewareFunc{
			func(next StrictHandlerFunc, operationID string) StrictHandlerFunc {
				return func(ctx *fiber.Ctx, args interface{}) (interface{}, error) {
					calls++
					if operationID != "ListModels" {
						t.Fatalf("operationID = %q, want %q", operationID, "ListModels")
					}
					return next(ctx, args)
				}
			},
		}), FiberServerOptions{BaseURL: "/v1"})

		assertStatus(t, app, httptest.NewRequest(http.MethodGet, "/v1/models", nil), http.StatusOK)
		if calls != 1 {
			t.Fatalf("strict middleware calls = %d, want 1", calls)
		}
	})

	t.Run("invalid json bodies", func(t *testing.T) {
		app := fiber.New()
		RegisterStubHandlers(app, "/v1")

		assertStatus(t, app, newJSONRequest(http.MethodPost, "/v1/chat/completions", "{"), http.StatusBadRequest)
		assertStatus(t, app, newJSONRequest(http.MethodPost, "/v1/audio/speech", "{"), http.StatusBadRequest)
	})

	t.Run("unexpected response types", func(t *testing.T) {
		app := fiber.New()
		unexpectedMiddleware := func(StrictHandlerFunc, string) StrictHandlerFunc {
			return func(*fiber.Ctx, interface{}) (interface{}, error) {
				return unexpectedResponse{}, nil
			}
		}
		RegisterHandlersWithOptions(app, NewStrictHandler(&Stub{}, []StrictMiddlewareFunc{unexpectedMiddleware}), FiberServerOptions{BaseURL: "/v1"})

		assertStatus(t, app, httptest.NewRequest(http.MethodGet, "/v1/models", nil), http.StatusInternalServerError)
		assertStatus(t, app, newJSONRequest(http.MethodPost, "/v1/chat/completions", `{"model":"m","messages":[]}`), http.StatusInternalServerError)
		assertStatus(t, app, newJSONRequest(http.MethodPost, "/v1/audio/speech", `{"model":"m","input":"x","voice":"v"}`), http.StatusInternalServerError)
		assertStatus(t, app, httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader([]byte("multipart body"))), http.StatusInternalServerError)
	})

	t.Run("response visitor errors", func(t *testing.T) {
		app := fiber.New()
		RegisterHandlersWithOptions(app, NewStrictHandler(&badBodyServer{}, nil), FiberServerOptions{BaseURL: "/v1"})

		assertRequestError(t, app, newJSONRequest(http.MethodPost, "/v1/audio/speech", `{"model":"m","input":"x","voice":"v"}`), "read failed")
		assertRequestError(t, app, newJSONRequest(http.MethodPost, "/v1/chat/completions", `{"model":"m","messages":[],"stream":true}`), "read failed")
	})

	t.Run("transcription event stream", func(t *testing.T) {
		app := fiber.New()
		RegisterHandlersWithOptions(app, NewStrictHandler(&transcriptionStreamServer{}, nil), FiberServerOptions{BaseURL: "/v1"})

		assertStatus(t, app, httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader([]byte("multipart body"))), http.StatusOK)
	})
}

func TestGeneratedDefaultRegistration(t *testing.T) {
	app := fiber.New()
	RegisterHandlers(app, NewStrictHandler(&Stub{}, nil))
	assertStatus(t, app, httptest.NewRequest(http.MethodGet, "/models", nil), http.StatusOK)
}

func TestGeneratedEnumValidity(t *testing.T) {
	assertValidEnum(t, ChatCompletionResponseMessageRoleAssistant.Valid(), ChatCompletionResponseMessageRole("invalid").Valid())
	assertValidEnum(t, ChatCompletionStreamResponseDeltaRoleAssistant.Valid(), ChatCompletionStreamResponseDeltaRole("invalid").Valid())
	assertValidEnum(t, ChatCompletionChunk.Valid(), CreateChatCompletionStreamResponseObject("invalid").Valid())
	assertValidEnum(t, ModelObjectModel.Valid(), ModelObject("invalid").Valid())
}

func assertValidEnum(t *testing.T, valid, invalid bool) {
	t.Helper()
	if !valid {
		t.Fatal("known enum value should be valid")
	}
	if invalid {
		t.Fatal("unknown enum value should be invalid")
	}
}

func newOpenAITestClient(t *testing.T) openai.Client {
	t.Helper()

	app := fiber.New()
	RegisterStubHandlers(app, "/v1")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Listener(ln)
	}()

	t.Cleanup(func() {
		_ = app.Shutdown()
		select {
		case err := <-errCh:
			if err != nil {
				t.Logf("mock server stopped: %v", err)
			}
		case <-time.After(time.Second):
			t.Log("mock server did not stop within 1s")
		}
	})

	return openai.NewClient(
		option.WithBaseURL("http://"+ln.Addr().String()+"/v1"),
		option.WithAPIKey("test-api-key"),
	)
}

func assertStatus(t *testing.T, app *fiber.App, req *http.Request, want int) {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != want {
		t.Fatalf("%s %s status = %d, want %d", req.Method, req.URL.Path, resp.StatusCode, want)
	}
}

func assertRequestError(t *testing.T, app *fiber.App, req *http.Request, want string) {
	t.Helper()
	_, err := app.Test(req)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("request error = %v, want containing %q", err, want)
	}
}

func assertEventStream(t *testing.T, app *fiber.App, req *http.Request, wantBody string) {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s status = %d, want %d", req.Method, req.URL.Path, resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("%s %s content-type = %q, want text/event-stream", req.Method, req.URL.Path, got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read event stream body: %v", err)
	}
	if !bytes.Contains(body, []byte("data: ")) {
		t.Fatalf("event stream body %q does not contain SSE data lines", body)
	}
	if !bytes.Contains(body, []byte(wantBody)) {
		t.Fatalf("event stream body %q does not contain %q", body, wantBody)
	}
}

func newJSONRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newMultipartRequest(t *testing.T, path string, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := file.Write([]byte("fake wav data")); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write multipart field %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type errorServer struct{}

func (s *errorServer) ListModels(context.Context, ListModelsRequestObject) (ListModelsResponseObject, error) {
	return nil, errors.New("models failed")
}

func (s *errorServer) CreateChatCompletion(context.Context, CreateChatCompletionRequestObject) (CreateChatCompletionResponseObject, error) {
	return nil, errors.New("chat failed")
}

func (s *errorServer) CreateSpeech(context.Context, CreateSpeechRequestObject) (CreateSpeechResponseObject, error) {
	return nil, errors.New("speech failed")
}

func (s *errorServer) CreateTranscription(context.Context, CreateTranscriptionRequestObject) (CreateTranscriptionResponseObject, error) {
	return nil, errors.New("transcription failed")
}

type unexpectedResponse struct{}

type badBodyServer struct {
	Stub
}

func (s *badBodyServer) CreateSpeech(context.Context, CreateSpeechRequestObject) (CreateSpeechResponseObject, error) {
	return CreateSpeech200ApplicationoctetStreamResponse{Body: errReader{}}, nil
}

func (s *badBodyServer) CreateChatCompletion(context.Context, CreateChatCompletionRequestObject) (CreateChatCompletionResponseObject, error) {
	return CreateChatCompletion200TexteventStreamResponse{Body: errReader{}}, nil
}

type errReader struct{}

func (r errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

type transcriptionStreamServer struct {
	Stub
}

func (s *transcriptionStreamServer) CreateTranscription(context.Context, CreateTranscriptionRequestObject) (CreateTranscriptionResponseObject, error) {
	body := []byte("data: {\"type\":\"transcript.text.done\",\"text\":\"genx transcription stub\"}\n\n")
	return CreateTranscription200TexteventStreamResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
	}, nil
}

func requireNoOpenAIError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}

	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		t.Log(string(apiErr.DumpRequest(true)))
	}
	t.Fatalf("unexpected OpenAI SDK error: %v", err)
}
