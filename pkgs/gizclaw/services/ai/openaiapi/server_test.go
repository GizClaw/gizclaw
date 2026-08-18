package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/idy/ai-server-shell/backend"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestHandleListsAllModels(t *testing.T) {
	key := mustKey(t)
	calls := 0
	server := &Server{
		Caller: key.Public,
		Models: modelListerFunc(func(_ context.Context, request adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
			calls++
			if request.Params.Limit == nil || *request.Params.Limit != 200 {
				t.Fatalf("limit = %v", request.Params.Limit)
			}
			if calls == 1 {
				next := "next"
				return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{
					Items: []apitypes.Model{testModel("first", "owner")}, HasNext: true, NextCursor: &next,
				}), nil
			}
			return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: []apitypes.Model{testModel("second", "owner")}}), nil
		}),
	}
	response, err := server.Handle(context.Background(), requestFor(key.Public, backend.CapabilityModels, "listModels", nil))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	var list struct {
		Object string        `json:"object"`
		Data   []openAIModel `json:"data"`
	}
	if err := json.Unmarshal(response.JSON, &list); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if list.Object != "list" || len(list.Data) != 2 || list.Data[1].ID != "second" || calls != 2 {
		t.Fatalf("response = %#v calls=%d", list, calls)
	}
}

func TestHandleChatPreservesProjectionAndStreams(t *testing.T) {
	key := mustKey(t)
	now := time.Unix(100, 0)
	server := &Server{
		Caller: key.Public,
		Now:    func() time.Time { return now },
		Generator: generatorFunc(func(_ context.Context, pattern string, modelContext genx.ModelContext) (genx.Stream, error) {
			if pattern != "model/chat" {
				t.Fatalf("pattern = %q", pattern)
			}
			params := modelContext.Params()
			if params == nil || params.Temperature != 0.25 || params.Thinking == nil || params.Thinking.Level != "high" {
				t.Fatalf("params = %#v", params)
			}
			return newTextStream("hello"), nil
		}),
	}
	body := json.RawMessage(`{"model":"chat","messages":[{"role":"system","content":"prompt"},{"role":"user","content":"hi"}],"temperature":0.25,"thinking":{"level":"high"},"stream":true}`)
	response, err := server.Handle(context.Background(), requestFor(key.Public, backend.CapabilityChat, "createChatCompletion", body))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	events := collectEvents(t, response.Stream)
	if len(events) != 3 || !bytes.Contains(events[0].Data, []byte(`"content":"hello"`)) || string(events[2].Data) != "[DONE]" {
		t.Fatalf("events = %#v", events)
	}
}

func TestHandleChatRejectsUnsupportedOption(t *testing.T) {
	key := mustKey(t)
	for _, test := range []struct {
		body  string
		param string
	}{
		{body: `{"model":"chat","messages":[],"top_p":0.5}`, param: "request"},
		{body: `{"model":"chat","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}]}`, param: "messages.content"},
		{body: `{"model":"chat","messages":[{"role":"assistant","content":"ok","tool_calls":[]}]}`, param: "messages"},
	} {
		_, err := (&Server{Caller: key.Public}).Handle(context.Background(), requestFor(
			key.Public, backend.CapabilityChat, "createChatCompletion", json.RawMessage(test.body),
		))
		var backendErr *backend.Error
		if !errors.As(err, &backendErr) || backendErr.Code != "unsupported_option" || backendErr.Param != test.param {
			t.Fatalf("body=%s error = %#v", test.body, err)
		}
	}
}

func TestHandleSpeechBinaryAndSSE(t *testing.T) {
	key := mustKey(t)
	server := &Server{
		Caller: key.Public,
		Transformer: transformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			if pattern != "voice/alloy?format=mp3" {
				t.Fatalf("pattern = %q", pattern)
			}
			text, err := readTextStream(input)
			if err != nil || text != "hello" {
				t.Fatalf("input = %q error=%v", text, err)
			}
			return newBlobStream("audio/mpeg", []byte("audio")), nil
		}),
	}
	request := requestFor(key.Public, backend.CapabilityAudio, "createSpeech", json.RawMessage(`{"model":"tts","voice":"alloy","input":"hello","stream_format":"audio"}`))
	response, err := server.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle(binary) error = %v", err)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "audio" || response.MediaType != "audio/mpeg" {
		t.Fatalf("binary response type=%q data=%q error=%v", response.MediaType, data, err)
	}
	request.Input.JSON = json.RawMessage(`{"model":"tts","voice":"alloy","input":"hello","stream_format":"sse"}`)
	response, err = server.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle(SSE) error = %v", err)
	}
	events := collectEvents(t, response.Stream)
	if len(events) != 2 || !bytes.Contains(events[0].Data, []byte(`"type":"speech.audio.delta"`)) || !bytes.Contains(events[1].Data, []byte(`"type":"speech.audio.done"`)) {
		t.Fatalf("events = %#v", events)
	}
}

func TestHandleSpeechRejectsNonStandardStreamAndUnsupportedSpeed(t *testing.T) {
	key := mustKey(t)
	for _, body := range []string{
		`{"model":"tts","voice":"alloy","input":"hello","stream":true}`,
		`{"model":"tts","voice":"alloy","input":"hello","speed":1.5}`,
	} {
		_, err := (&Server{Caller: key.Public}).Handle(context.Background(), requestFor(
			key.Public, backend.CapabilityAudio, "createSpeech", json.RawMessage(body),
		))
		var backendErr *backend.Error
		if !errors.As(err, &backendErr) || backendErr.Code != "unsupported_option" {
			t.Fatalf("body=%s error=%#v", body, err)
		}
	}
}

func TestHandleTranscriptionJSONAndSSE(t *testing.T) {
	key := mustKey(t)
	server := &Server{
		Caller: key.Public,
		Transformer: transformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			if pattern != "model/asr" {
				t.Fatalf("pattern = %q", pattern)
			}
			data, _, err := readBlobStreamWithMIME(input, "")
			if err != nil || string(data) != "audio" {
				t.Fatalf("input = %q error=%v", data, err)
			}
			return newTextStream("words"), nil
		}),
	}
	body, contentType := multipartBody(t, map[string]string{"model": "asr", "response_format": "json"}, []byte("audio"))
	request := requestFor(key.Public, backend.CapabilityAudio, "createTranscription", nil)
	request.Input = backend.Input{MediaType: "multipart/form-data", Bytes: body}
	request.Metadata.Extensions = map[string][]string{"Content-Type": {contentType}}
	response, err := server.Handle(context.Background(), request)
	if err != nil || string(response.JSON) != `{"text":"words"}` {
		t.Fatalf("JSON response=%s error=%v", response.JSON, err)
	}
	body, contentType = multipartBody(t, map[string]string{"model": "asr", "stream": "true"}, []byte("audio"))
	request.Input.Bytes = body
	request.Metadata.Extensions = map[string][]string{"Content-Type": {contentType}}
	response, err = server.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle(SSE) error = %v", err)
	}
	events := collectEvents(t, response.Stream)
	if len(events) != 2 || !bytes.Contains(events[1].Data, []byte(`"text":"words"`)) {
		t.Fatalf("events = %#v", events)
	}
}

func TestHandleFailsClosedOnCallerAndOperation(t *testing.T) {
	key := mustKey(t)
	other := mustKey(t)
	server := &Server{Caller: key.Public}
	_, err := server.Handle(context.Background(), requestFor(other.Public, backend.CapabilityModels, "listModels", nil))
	var backendErr *backend.Error
	if !errors.As(err, &backendErr) || backendErr.Kind != backend.ErrorUnavailable {
		t.Fatalf("caller error = %#v", err)
	}
	_, err = server.Handle(context.Background(), requestFor(key.Public, backend.CapabilityAudio, "createTranslation", nil))
	if !errors.As(err, &backendErr) || backendErr.Kind != backend.ErrorUnsupported {
		t.Fatalf("operation error = %#v", err)
	}
}

func TestEventStreamCloseUnblocksProducer(t *testing.T) {
	key := mustKey(t)
	source := newBlockingStream()
	server := &Server{
		Caller: key.Public,
		Generator: generatorFunc(func(context.Context, string, genx.ModelContext) (genx.Stream, error) {
			return source, nil
		}),
	}
	response, err := server.Handle(context.Background(), requestFor(
		key.Public, backend.CapabilityChat, "createChatCompletion",
		json.RawMessage(`{"model":"chat","messages":[],"stream":true}`),
	))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if err := response.Stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-source.closed:
	case <-time.After(time.Second):
		t.Fatal("source did not close")
	}
	select {
	case _, open := <-response.Stream.Events():
		if open {
			t.Fatal("events remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("event producer did not exit")
	}
}

func TestEventStreamContextCancellationUnblocksProducer(t *testing.T) {
	key := mustKey(t)
	source := newBlockingStream()
	server := &Server{
		Caller: key.Public,
		Generator: generatorFunc(func(context.Context, string, genx.ModelContext) (genx.Stream, error) {
			return source, nil
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	response, err := server.Handle(ctx, requestFor(
		key.Public, backend.CapabilityChat, "createChatCompletion",
		json.RawMessage(`{"model":"chat","messages":[],"stream":true}`),
	))
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	cancel()
	select {
	case <-source.closed:
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not close source")
	}
	select {
	case _, open := <-response.Stream.Events():
		if open {
			t.Fatal("events remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("event producer did not exit")
	}
}

func requestFor(caller giznet.PublicKey, capability backend.Capability, operation string, body json.RawMessage) backend.Request {
	return backend.Request{
		Capability: capability, Operation: operation,
		Metadata: backend.Metadata{CallerID: caller.String()},
		Input:    backend.Input{MediaType: "application/json", JSON: body, Bytes: append([]byte(nil), body...)},
	}
}

func collectEvents(t *testing.T, stream backend.Stream) []backend.Event {
	t.Helper()
	defer stream.Close()
	var result []backend.Event
	for event := range stream.Events() {
		result = append(result, event)
	}
	return result
}

func multipartBody(t *testing.T, fields map[string]string, file []byte) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
	}
	part, err := writer.CreateFormFile("file", "audio.mp3")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(file); err != nil {
		t.Fatalf("Write(file) error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buffer.Bytes(), writer.FormDataContentType()
}

type modelListerFunc func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)

func (f modelListerFunc) ListModels(ctx context.Context, request adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	return f(ctx, request)
}

type generatorFunc func(context.Context, string, genx.ModelContext) (genx.Stream, error)

func (f generatorFunc) GenerateStream(ctx context.Context, pattern string, modelContext genx.ModelContext) (genx.Stream, error) {
	return f(ctx, pattern, modelContext)
}

func (generatorFunc) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not implemented")
}

type transformerFunc func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f transformerFunc) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return f(ctx, pattern, input)
}

func testModel(id, providerID string) apitypes.Model {
	return apitypes.Model{
		Id: id, Kind: apitypes.ModelKindLlm, CreatedAt: time.Unix(100, 0),
		Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindOpenaiTenant, Id: providerID},
	}
}

func mustKey(t *testing.T) *giznet.KeyPair {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	return key
}

type blockingStream struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingStream() *blockingStream { return &blockingStream{closed: make(chan struct{})} }

func (s *blockingStream) Next() (*genx.MessageChunk, error) {
	<-s.closed
	return nil, genx.ErrDone
}

func (s *blockingStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *blockingStream) CloseWithError(error) error { return s.Close() }

func TestTranscriptionAudioMIME(t *testing.T) {
	if got := transcriptionAudioMIME("application/octet-stream", "recording.wav", nil); got != "audio/wav" && got != "audio/x-wav" {
		t.Fatalf("extension MIME = %q", got)
	}
	if got := transcriptionAudioMIME("", "recording", []byte("ID3audio")); got != "audio/mpeg" {
		t.Fatalf("sniffed MIME = %q", got)
	}
	if !transcriptionAcceptsEventStream("application/json, text/event-stream; q=1") {
		t.Fatal("Accept did not select event stream")
	}
	if strings.TrimSpace(firstHeader(map[string][]string{"accept": {"value"}}, "Accept")) != "value" {
		t.Fatal("header lookup is not case insensitive")
	}
}
