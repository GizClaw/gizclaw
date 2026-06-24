package openaiapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/openaiservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestListModelsFiltersDeniedModels(t *testing.T) {
	caller := mustKey(t)
	srv := &Server{
		Caller: caller.Public,
		Models: modelListerFunc(func(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
			return adminservice.ListModels200JSONResponse(adminservice.ModelList{
				Items: []apitypes.Model{
					testModel("allowed", "tenant-a"),
					testModel("denied", "tenant-b"),
				},
			}), nil
		}),
		Authorizer: authorizerFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
			if req.Subject.Id != caller.Public.String() {
				t.Fatalf("subject = %q, want caller public key", req.Subject.Id)
			}
			if req.Resource.Id == "denied" {
				return acl.ErrDenied
			}
			return nil
		}),
	}

	resp, err := srv.ListModels(context.Background(), openaiservice.ListModelsRequestObject{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	list, ok := resp.(openaiservice.ListModels200JSONResponse)
	if !ok {
		t.Fatalf("ListModels() response = %T", resp)
	}
	if len(list.Data) != 1 || list.Data[0].Id != "allowed" || list.Data[0].OwnedBy != "tenant-a" {
		t.Fatalf("ListModels() data = %#v", list.Data)
	}
}

func TestListModelsPaginationAndErrors(t *testing.T) {
	caller := mustKey(t)
	calls := 0
	srv := &Server{
		Caller: caller.Public,
		Models: modelListerFunc(func(_ context.Context, req adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
			calls++
			switch calls {
			case 1:
				next := "next"
				return adminservice.ListModels200JSONResponse(adminservice.ModelList{
					Items:      []apitypes.Model{testModel("first", "")},
					HasNext:    true,
					NextCursor: &next,
				}), nil
			default:
				if req.Params.Cursor == nil || *req.Params.Cursor != "next" {
					t.Fatalf("second cursor = %#v", req.Params.Cursor)
				}
				return adminservice.ListModels200JSONResponse(adminservice.ModelList{Items: []apitypes.Model{testModel("second", "")}}), nil
			}
		}),
		Authorizer: authorizerFunc(func(context.Context, acl.AuthorizeRequest) error { return nil }),
	}
	resp, err := srv.ListModels(context.Background(), openaiservice.ListModelsRequestObject{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	list := resp.(openaiservice.ListModels200JSONResponse)
	if len(list.Data) != 2 || list.Data[0].Id != "first" || list.Data[1].Id != "second" {
		t.Fatalf("ListModels() data = %#v", list.Data)
	}

	if _, err := (&Server{}).ListModels(context.Background(), openaiservice.ListModelsRequestObject{}); err == nil {
		t.Fatal("ListModels() without model service succeeded")
	}
	_, err = (&Server{
		Caller:     caller.Public,
		Authorizer: authorizerFunc(func(context.Context, acl.AuthorizeRequest) error { return nil }),
		Models: modelListerFunc(func(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
			return adminservice.ListModels500JSONResponse(apitypes.NewErrorResponse("ERR", "failed")), nil
		}),
	}).ListModels(context.Background(), openaiservice.ListModelsRequestObject{})
	if err == nil || !strings.Contains(err.Error(), "list models response") {
		t.Fatalf("ListModels() error = %v", err)
	}
}

func TestListVoicesReturnsPeerFilteredVoiceList(t *testing.T) {
	want := adminservice.ListVoicesParams{ProviderName: stringPtr("volc-main")}
	srv := &Server{
		Voices: voiceListerFunc(func(_ context.Context, req adminservice.ListVoicesRequestObject) (adminservice.ListVoicesResponseObject, error) {
			if req.Params.ProviderName == nil || *req.Params.ProviderName != *want.ProviderName {
				t.Fatalf("provider name = %#v, want %q", req.Params.ProviderName, *want.ProviderName)
			}
			return adminservice.ListVoices200JSONResponse(adminservice.VoiceList{
				Items: []apitypes.Voice{{Id: "voice-a", Name: stringPtr("Voice A")}},
			}), nil
		}),
	}

	list, err := srv.ListVoices(context.Background(), want)
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != "voice-a" {
		t.Fatalf("ListVoices() items = %#v", list.Items)
	}

	if _, err := (&Server{}).ListVoices(context.Background(), adminservice.ListVoicesParams{}); err == nil {
		t.Fatal("ListVoices() without service succeeded")
	}
	bad := &Server{Voices: voiceListerFunc(func(context.Context, adminservice.ListVoicesRequestObject) (adminservice.ListVoicesResponseObject, error) {
		return adminservice.ListVoices500JSONResponse(apitypes.NewErrorResponse("ERR", "bad")), nil
	})}
	if _, err := bad.ListVoices(context.Background(), adminservice.ListVoicesParams{}); err == nil {
		t.Fatal("ListVoices() with non-list response succeeded")
	}
}

func TestCreateChatCompletionStreamsGenXText(t *testing.T) {
	now := time.Unix(1700000000, 0)
	srv := &Server{
		Generator: generatorFunc(func(_ context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
			if pattern != "model/chat" {
				t.Fatalf("pattern = %q", pattern)
			}
			var sawUser bool
			for msg := range mctx.Messages() {
				if msg.Role == genx.RoleUser {
					sawUser = true
				}
			}
			if !sawUser {
				t.Fatal("model context did not include user message")
			}
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: genx.Text("hel")},
				{Role: genx.RoleModel, Part: genx.Text("lo")},
			}}, nil
		}),
		Now: func() time.Time { return now },
	}
	stream := true
	resp, err := srv.CreateChatCompletion(context.Background(), openaiservice.CreateChatCompletionRequestObject{
		Body: &openaiservice.CreateChatCompletionRequest{
			Model:  "chat",
			Stream: &stream,
			Messages: []map[string]interface{}{
				{"role": "user", "content": "hi"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion() error = %v", err)
	}
	out, ok := resp.(openaiservice.CreateChatCompletion200TexteventStreamResponse)
	if !ok {
		t.Fatalf("CreateChatCompletion() response = %T", resp)
	}
	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"content":"hel"`) || !strings.Contains(text, `"content":"lo"`) || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream body = %s", text)
	}
}

func TestCreateChatCompletionNonStreamConvertsOpenAIRequest(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString([]byte("wav"))
	srv := &Server{
		Generator: generatorFunc(func(_ context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
			if pattern != "model/chat" {
				t.Fatalf("pattern = %q", pattern)
			}
			if params := mctx.Params(); params == nil || params.Temperature != 0.3 || params.ExtraFields["reasoning_effort"] != "medium" {
				t.Fatalf("params = %#v", params)
			}
			var prompts, users, models int
			for prompt := range mctx.Prompts() {
				prompts++
				if prompt.Name != "system" || prompt.Text != "be brief" {
					t.Fatalf("prompt = %#v", prompt)
				}
			}
			for msg := range mctx.Messages() {
				switch msg.Role {
				case genx.RoleUser:
					users++
					parts := msg.Payload.(genx.Contents)
					if len(parts) != 2 {
						t.Fatalf("user parts = %#v", parts)
					}
				case genx.RoleModel:
					models++
				}
			}
			if prompts != 1 || users != 1 || models != 1 {
				t.Fatalf("prompts/users/models = %d/%d/%d", prompts, users, models)
			}
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: genx.Text("ok")},
			}}, nil
		}),
		Now: func() time.Time { return time.Unix(1700000100, 0) },
	}
	temperature := float32(0.3)
	level := "medium"
	resp, err := srv.CreateChatCompletion(context.Background(), openaiservice.CreateChatCompletionRequestObject{
		Body: &openaiservice.CreateChatCompletionRequest{
			Model:       "chat",
			Temperature: &temperature,
			Thinking:    &openaiservice.ThinkingOptions{Level: &level},
			Messages: []map[string]interface{}{
				{"role": "system", "content": "be brief"},
				{"role": "user", "content": []interface{}{
					map[string]interface{}{"type": "text", "text": "listen"},
					map[string]interface{}{"type": "input_audio", "input_audio": map[string]interface{}{"data": audio, "format": "wav"}},
				}},
				{"role": "assistant", "content": "previous"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion() error = %v", err)
	}
	out := resp.(openaiservice.CreateChatCompletion200JSONResponse)
	if out.Model != "chat" || len(out.Choices) != 1 || out.Choices[0].Message.Content == nil || *out.Choices[0].Message.Content != "ok" {
		t.Fatalf("CreateChatCompletion() response = %#v", out)
	}
}

func TestCreateChatCompletionPropagatesDenied(t *testing.T) {
	srv := &Server{
		Generator: generatorFunc(func(context.Context, string, genx.ModelContext) (genx.Stream, error) {
			return nil, peergenx.ErrDenied
		}),
	}
	_, err := srv.CreateChatCompletion(context.Background(), openaiservice.CreateChatCompletionRequestObject{
		Body: &openaiservice.CreateChatCompletionRequest{
			Model:    "chat",
			Messages: []map[string]interface{}{{"role": "user", "content": "hi"}},
		},
	})
	if !errors.Is(err, peergenx.ErrDenied) {
		t.Fatalf("CreateChatCompletion() error = %v, want denied", err)
	}
}

func TestCreateChatCompletionValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		srv  *Server
		body *openaiservice.CreateChatCompletionRequest
	}{
		{name: "nil body", srv: &Server{}},
		{name: "missing model", srv: &Server{}, body: &openaiservice.CreateChatCompletionRequest{}},
		{name: "unsupported role", srv: &Server{}, body: &openaiservice.CreateChatCompletionRequest{Model: "m", Messages: []map[string]interface{}{{"role": "tool", "content": "x"}}}},
		{name: "bad content", srv: &Server{}, body: &openaiservice.CreateChatCompletionRequest{Model: "m", Messages: []map[string]interface{}{{"role": "user", "content": 3}}}},
		{name: "bad audio", srv: &Server{}, body: &openaiservice.CreateChatCompletionRequest{Model: "m", Messages: []map[string]interface{}{{"role": "user", "content": []interface{}{map[string]interface{}{"type": "input_audio", "input_audio": map[string]interface{}{"data": "!", "format": "wav"}}}}}}},
		{name: "no generator", srv: &Server{}, body: &openaiservice.CreateChatCompletionRequest{Model: "m", Messages: []map[string]interface{}{{"role": "user", "content": "x"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.srv.CreateChatCompletion(context.Background(), openaiservice.CreateChatCompletionRequestObject{Body: tt.body})
			if err == nil {
				t.Fatal("CreateChatCompletion() succeeded")
			}
		})
	}
}

func TestCreateSpeechUsesVoiceTransformer(t *testing.T) {
	srv := &Server{
		Transformer: transformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			if pattern != "voice/cancan" {
				t.Fatalf("pattern = %q", pattern)
			}
			text, err := readTextStream(input)
			if err != nil {
				t.Fatalf("read input text: %v", err)
			}
			if text != "hello" {
				t.Fatalf("input text = %q", text)
			}
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("ogg")}},
			}}, nil
		}),
	}
	resp, err := srv.CreateSpeech(context.Background(), openaiservice.CreateSpeechRequestObject{
		Body: &openaiservice.CreateSpeechRequest{Model: "tts", Voice: "cancan", Input: "hello"},
	})
	if err != nil {
		t.Fatalf("CreateSpeech() error = %v", err)
	}
	out, ok := resp.(speechAudioResponse)
	if !ok {
		t.Fatalf("CreateSpeech() response = %T", resp)
	}
	if out.ContentType != "audio/ogg" {
		t.Fatalf("speech content type = %q, want audio/ogg", out.ContentType)
	}
	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read speech body: %v", err)
	}
	if string(body) != "ogg" {
		t.Fatalf("speech body = %q", body)
	}
}

func TestCreateSpeechNormalizesUsingBlobMIME(t *testing.T) {
	audio := append([]byte("ogg:"), openAITestID3Tag([]byte("valid-ogg-bytes"))...)
	audio = append(audio, []byte(":tail")...)
	srv := &Server{
		Transformer: transformerFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/ogg", Data: audio}},
			}}, nil
		}),
	}
	resp, err := srv.CreateSpeech(context.Background(), openaiservice.CreateSpeechRequestObject{
		Body: &openaiservice.CreateSpeechRequest{Model: "tts", Voice: "cancan", Input: "hello"},
	})
	if err != nil {
		t.Fatalf("CreateSpeech() error = %v", err)
	}
	out := resp.(speechAudioResponse)
	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read speech body: %v", err)
	}
	if !bytes.Equal(body, audio) {
		t.Fatalf("speech audio = %q, want original ogg payload", body)
	}
}

func TestSpeechContentTypeMapsResponseFormats(t *testing.T) {
	tests := []struct {
		format *string
		want   string
	}{
		{nil, "audio/mpeg"},
		{stringPtr("opus"), "audio/ogg"},
		{stringPtr("aac"), "audio/aac"},
		{stringPtr("flac"), "audio/flac"},
		{stringPtr("wav"), "audio/wav"},
		{stringPtr("pcm"), "audio/pcm"},
		{stringPtr("mp3"), "audio/mpeg"},
		{stringPtr("bad"), "audio/mpeg"},
	}
	for _, tt := range tests {
		body := &openaiservice.CreateSpeechRequest{ResponseFormat: tt.format}
		if got := speechContentType(body); got != tt.want {
			t.Fatalf("speechContentType(%v) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestCreateSpeechStreamsAndValidates(t *testing.T) {
	stream := true
	streamFormat := openaiservice.Sse
	srv := &Server{
		Transformer: transformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			if pattern != "model/tts" {
				t.Fatalf("pattern = %q", pattern)
			}
			if text, err := readTextStream(input); err != nil || text != "hello" {
				t.Fatalf("input text = %q err=%v", text, err)
			}
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("a")}},
				{Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("b")}},
			}}, nil
		}),
	}
	resp, err := srv.CreateSpeech(context.Background(), openaiservice.CreateSpeechRequestObject{
		Body: &openaiservice.CreateSpeechRequest{Model: "tts", Input: "hello", Stream: &stream},
	})
	if err != nil {
		t.Fatalf("CreateSpeech(stream) error = %v", err)
	}
	out := resp.(openaiservice.CreateSpeech200TexteventStreamResponse)
	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read speech stream: %v", err)
	}
	if text := string(body); !strings.Contains(text, "speech.audio.delta") || !strings.Contains(text, "speech.audio.done") {
		t.Fatalf("speech stream = %s", text)
	}

	resp, err = srv.CreateSpeech(context.Background(), openaiservice.CreateSpeechRequestObject{
		Body: &openaiservice.CreateSpeechRequest{Model: "tts", Input: "hello", StreamFormat: &streamFormat},
	})
	if err != nil {
		t.Fatalf("CreateSpeech(stream_format=sse) error = %v", err)
	}
	if _, ok := resp.(openaiservice.CreateSpeech200TexteventStreamResponse); !ok {
		t.Fatalf("CreateSpeech(stream_format=sse) response = %T", resp)
	}

	for _, req := range []*openaiservice.CreateSpeechRequest{
		nil,
		{Model: "tts", Voice: "v"},
		{Input: "hello"},
		{Model: "tts", Input: "hello"},
	} {
		_, err := (&Server{}).CreateSpeech(context.Background(), openaiservice.CreateSpeechRequestObject{Body: req})
		if err == nil {
			t.Fatalf("CreateSpeech(%#v) succeeded", req)
		}
	}
}

func TestCreateTranscriptionUsesModelTransformer(t *testing.T) {
	srv := &Server{
		Transformer: transformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			if pattern != "model/asr" {
				t.Fatalf("pattern = %q", pattern)
			}
			chunk, err := input.Next()
			if err != nil {
				t.Fatalf("input.Next() error = %v", err)
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || blob.MIMEType != "audio/wav" || string(blob.Data) != "wav" {
				t.Fatalf("input blob = %#v", chunk.Part)
			}
			return &sliceStream{chunks: []*genx.MessageChunk{{Part: genx.Text("text")}}}, nil
		}),
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("model", "asr")
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="input.wav"`)
	header.Set("Content-Type", "audio/wav")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreateFormFile error = %v", err)
	}
	_, _ = part.Write([]byte("wav"))
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), writer.Boundary())
	resp, err := srv.CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{Body: reader})
	if err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
	out, ok := resp.(openaiservice.CreateTranscription200JSONResponse)
	if !ok {
		t.Fatalf("CreateTranscription() response = %T", resp)
	}
	if out.Text != "text" {
		t.Fatalf("transcription text = %q", out.Text)
	}
}

func TestCreateTranscriptionSniffsAudioMIME(t *testing.T) {
	const mp3 = "ID3\x04\x00\x00\x00\x00\x00\x00audio"
	srv := &Server{
		Transformer: transformerFunc(func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
			chunk, err := input.Next()
			if err != nil {
				t.Fatalf("input.Next() error = %v", err)
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok {
				t.Fatalf("input chunk part = %T, want *genx.Blob", chunk.Part)
			}
			if blob.MIMEType != "audio/mpeg" {
				t.Fatalf("input MIME type = %q, want audio/mpeg", blob.MIMEType)
			}
			if string(blob.Data) != mp3 {
				t.Fatalf("input audio = %q, want mp3 bytes", blob.Data)
			}
			return &sliceStream{chunks: []*genx.MessageChunk{{Part: genx.Text("text")}}}, nil
		}),
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("model", "asr")
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="speech"`)
	header.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart error = %v", err)
	}
	_, _ = part.Write([]byte(mp3))
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}

	resp, err := srv.CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{
		Body: multipart.NewReader(bytes.NewReader(body.Bytes()), writer.Boundary()),
	})
	if err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
	out := resp.(openaiservice.CreateTranscription200JSONResponse)
	if out.Text != "text" {
		t.Fatalf("transcription text = %q", out.Text)
	}
}

func TestCreateTranscriptionStreamsMultipartFile(t *testing.T) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	releaseWriter := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseWriter) })
	}
	writerDone := make(chan error, 1)
	go func() {
		defer pw.Close()
		if err := writer.WriteField("model", "asr"); err != nil {
			writerDone <- err
			return
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="file"; filename="input.ogg"`)
		header.Set("Content-Type", "audio/ogg")
		part, err := writer.CreatePart(header)
		if err != nil {
			writerDone <- err
			return
		}
		if _, err := part.Write([]byte("first")); err != nil {
			writerDone <- err
			return
		}
		<-releaseWriter
		if _, err := part.Write([]byte("second")); err != nil {
			writerDone <- err
			return
		}
		writerDone <- writer.Close()
	}()

	firstChunk := make(chan string, 1)
	srv := &Server{
		Transformer: transformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			if pattern != "model/asr" {
				t.Errorf("pattern = %q", pattern)
			}
			chunk, err := input.Next()
			if err != nil {
				t.Errorf("input.Next() error = %v", err)
				release()
				return newTextStream(""), nil
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok {
				t.Errorf("input chunk part = %T, want *genx.Blob", chunk.Part)
				release()
				return newTextStream(""), nil
			}
			if blob.MIMEType != "audio/ogg" {
				t.Errorf("input MIME type = %q, want audio/ogg", blob.MIMEType)
			}
			firstChunk <- string(blob.Data)
			release()
			for {
				_, err := input.Next()
				if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Errorf("drain input: %v", err)
					break
				}
			}
			return newTextStream("text"), nil
		}),
	}

	respDone := make(chan error, 1)
	go func() {
		resp, err := srv.CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{
			Body: multipart.NewReader(pr, writer.Boundary()),
		})
		if err != nil {
			respDone <- err
			return
		}
		out, ok := resp.(openaiservice.CreateTranscription200JSONResponse)
		if !ok {
			respDone <- errors.New("unexpected transcription response type")
			return
		}
		if out.Text != "text" {
			respDone <- errors.New("unexpected transcription text")
			return
		}
		respDone <- nil
	}()

	select {
	case got := <-firstChunk:
		if got != "first" {
			t.Fatalf("first audio chunk = %q, want first", got)
		}
	case <-time.After(time.Second):
		_ = pw.CloseWithError(errors.New("test timed out waiting for streaming chunk"))
		release()
		t.Fatal("timed out waiting for first streaming audio chunk")
	}
	if err := <-writerDone; err != nil {
		t.Fatalf("write multipart: %v", err)
	}
	if err := <-respDone; err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
}

func TestTranscriptionAudioMIMEFallbacks(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		filename    string
		data        []byte
		want        string
	}{
		{name: "header", contentType: "audio/webm; codecs=opus", filename: "ignored", data: nil, want: "audio/webm"},
		{name: "extension", contentType: "application/octet-stream", filename: "input.mp3", data: nil, want: "audio/mpeg"},
		{name: "ogg magic", contentType: "application/octet-stream", data: []byte("OggSdata"), want: "audio/ogg"},
		{name: "wav magic", data: []byte("RIFFxxxxWAVEdata"), want: "audio/wav"},
		{name: "flac magic", data: []byte("fLaCdata"), want: "audio/flac"},
		{name: "mp4 magic", data: []byte("\x00\x00\x00\x18ftypmp42"), want: "audio/mp4"},
		{name: "unknown", contentType: "application/octet-stream", data: []byte("???"), want: "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transcriptionAudioMIME(tt.contentType, tt.filename, tt.data); got != tt.want {
				t.Fatalf("transcriptionAudioMIME() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateTranscriptionStreamsAndValidates(t *testing.T) {
	srv := &Server{
		Transformer: transformerFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("he")},
				{Part: genx.Text("llo")},
			}}, nil
		}),
	}
	reader := newTranscriptionMultipart(t, map[string]string{"model": "asr", "stream": "true"}, []byte("ogg"))
	resp, err := srv.CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{Body: reader})
	if err != nil {
		t.Fatalf("CreateTranscription(stream) error = %v", err)
	}
	out := resp.(openaiservice.CreateTranscription200TexteventStreamResponse)
	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read transcription stream: %v", err)
	}
	if text := string(body); !strings.Contains(text, "transcript.text.delta") || !strings.Contains(text, "transcript.text.done") {
		t.Fatalf("transcription stream = %s", text)
	}

	if _, err := (&Server{}).CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{}); err == nil {
		t.Fatal("CreateTranscription(nil body) succeeded")
	}
	if _, err := (&Server{}).CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{Body: newTranscriptionMultipart(t, map[string]string{}, []byte("x"))}); err == nil {
		t.Fatal("CreateTranscription(missing model) succeeded")
	}
	if _, err := (&Server{}).CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{Body: newTranscriptionMultipart(t, map[string]string{"model": "asr"}, nil)}); err == nil {
		t.Fatal("CreateTranscription(missing file) succeeded")
	}
	if _, err := (&Server{}).CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{Body: newTranscriptionMultipart(t, map[string]string{"model": "asr"}, []byte("x"))}); err == nil {
		t.Fatal("CreateTranscription(no transformer) succeeded")
	}
}

func TestCreateTranscriptionAcceptHeaderSelectsEventStream(t *testing.T) {
	srv := &Server{
		Transformer: transformerFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
			return &sliceStream{chunks: []*genx.MessageChunk{
				{Part: genx.Text("ordered")},
			}}, nil
		}),
	}
	reader := newTranscriptionMultipartOrdered(t, []transcriptionMultipartPart{
		{field: "model", value: "asr"},
		{file: []byte("ogg")},
		{field: "stream", value: "true"},
	})
	resp, err := srv.CreateTranscription(context.Background(), openaiservice.CreateTranscriptionRequestObject{
		Accept: "application/json, text/event-stream",
		Body:   reader,
	})
	if err != nil {
		t.Fatalf("CreateTranscription() error = %v", err)
	}
	out, ok := resp.(openaiservice.CreateTranscription200TexteventStreamResponse)
	if !ok {
		t.Fatalf("CreateTranscription() response = %T, want event stream", resp)
	}
	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read transcription stream: %v", err)
	}
	if text := string(body); !strings.Contains(text, "ordered") || !strings.Contains(text, "transcript.text.done") {
		t.Fatalf("transcription stream = %s", text)
	}
}

func TestOpenAIAPIStreamHelpers(t *testing.T) {
	var buf bytes.Buffer
	err := writeSpeechSSE(&buf, &sliceStream{err: errors.New("boom")}, "audio/mpeg")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("writeSpeechSSE() error = %v", err)
	}
	buf.Reset()
	err = writeTranscriptionSSE(&buf, &sliceStream{err: errors.New("boom")})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("writeTranscriptionSSE() error = %v", err)
	}
	buf.Reset()
	if err := writeSSEError(&buf, "ERR", errors.New("broken")); err != nil {
		t.Fatalf("writeSSEError() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"code":"ERR"`) {
		t.Fatalf("writeSSEError() body = %s", buf.String())
	}
	stream := newTextStream("x")
	if err := stream.CloseWithError(errors.New("closed")); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	if _, err := stream.Next(); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Next() after CloseWithError error = %v", err)
	}
}

func TestWriteSpeechSSEStripsMP3ID3Tags(t *testing.T) {
	var buf bytes.Buffer
	err := writeSpeechSSE(&buf, &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/mpeg", Data: append(openAITestID3Tag([]byte("tag-a")), []byte("frame-a")...)}},
		{Part: &genx.Blob{MIMEType: "audio/mpeg", Data: append(openAITestID3Tag([]byte("tag-b")), []byte("frame-b")...)}},
	}}, "audio/mpeg")
	if err != nil {
		t.Fatalf("writeSpeechSSE() error = %v", err)
	}

	var audio []byte
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") || strings.Contains(line, "speech.audio.done") {
			continue
		}
		var event struct {
			Audio string `json:"audio"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		chunk, err := base64.StdEncoding.DecodeString(event.Audio)
		if err != nil {
			t.Fatalf("decode audio: %v", err)
		}
		audio = append(audio, chunk...)
	}
	if bytes.Contains(audio, []byte("ID3")) {
		t.Fatalf("speech audio still contains ID3: %q", audio)
	}
	if string(audio) != "frame-aframe-b" {
		t.Fatalf("speech audio = %q, want frame-aframe-b", audio)
	}
}

func TestWriteSpeechSSEUsesBlobMIME(t *testing.T) {
	audio := append([]byte("ogg:"), openAITestID3Tag([]byte("valid-ogg-bytes"))...)
	audio = append(audio, []byte(":tail")...)
	var buf bytes.Buffer
	err := writeSpeechSSE(&buf, &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/ogg", Data: audio}},
	}}, "audio/mpeg")
	if err != nil {
		t.Fatalf("writeSpeechSSE() error = %v", err)
	}

	var got []byte
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") || strings.Contains(line, "speech.audio.done") {
			continue
		}
		var event struct {
			Audio string `json:"audio"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		chunk, err := base64.StdEncoding.DecodeString(event.Audio)
		if err != nil {
			t.Fatalf("decode audio: %v", err)
		}
		got = append(got, chunk...)
	}
	if !bytes.Equal(got, audio) {
		t.Fatalf("speech stream audio = %q, want original ogg payload", got)
	}
}

func openAITestID3Tag(payload []byte) []byte {
	header := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
	size := len(payload)
	header[6] = byte((size >> 21) & 0x7f)
	header[7] = byte((size >> 14) & 0x7f)
	header[8] = byte((size >> 7) & 0x7f)
	header[9] = byte(size & 0x7f)
	return append(header, payload...)
}

type modelListerFunc func(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error)

func (f modelListerFunc) ListModels(ctx context.Context, req adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
	return f(ctx, req)
}

type voiceListerFunc func(context.Context, adminservice.ListVoicesRequestObject) (adminservice.ListVoicesResponseObject, error)

func (f voiceListerFunc) ListVoices(ctx context.Context, req adminservice.ListVoicesRequestObject) (adminservice.ListVoicesResponseObject, error) {
	return f(ctx, req)
}

type authorizerFunc func(context.Context, acl.AuthorizeRequest) error

func (f authorizerFunc) Authorize(ctx context.Context, req acl.AuthorizeRequest) error {
	return f(ctx, req)
}

type generatorFunc func(context.Context, string, genx.ModelContext) (genx.Stream, error)

func (f generatorFunc) GenerateStream(ctx context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
	return f(ctx, pattern, mctx)
}

func (f generatorFunc) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not implemented")
}

type transformerFunc func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f transformerFunc) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return f(ctx, pattern, input)
}

func testModel(id, providerName string) apitypes.Model {
	return apitypes.Model{
		Id:        id,
		Kind:      apitypes.ModelKindLlm,
		CreatedAt: time.Unix(100, 0),
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Name: providerName,
		},
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

func newTranscriptionMultipart(t *testing.T, fields map[string]string, file []byte) *multipart.Reader {
	t.Helper()
	var parts []transcriptionMultipartPart
	for key, value := range fields {
		parts = append(parts, transcriptionMultipartPart{field: key, value: value})
	}
	if file != nil {
		parts = append(parts, transcriptionMultipartPart{file: file})
	}
	return newTranscriptionMultipartOrdered(t, parts)
}

type transcriptionMultipartPart struct {
	field string
	value string
	file  []byte
}

func newTranscriptionMultipartOrdered(t *testing.T, parts []transcriptionMultipartPart) *multipart.Reader {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, partSpec := range parts {
		if partSpec.field != "" {
			if err := writer.WriteField(partSpec.field, partSpec.value); err != nil {
				t.Fatalf("WriteField(%s) error = %v", partSpec.field, err)
			}
			continue
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="file"; filename="input.ogg"`)
		header.Set("Content-Type", "audio/ogg")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("CreatePart error = %v", err)
		}
		_, _ = part.Write(partSpec.file)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}
	return multipart.NewReader(bytes.NewReader(body.Bytes()), writer.Boundary())
}
