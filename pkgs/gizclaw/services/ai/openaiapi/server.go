package openaiapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/openaihttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ModelLister interface {
	ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)
}

type VoiceLister interface {
	ListVoices(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error)
}

type Server struct {
	Caller      giznet.PublicKey
	Models      ModelLister
	Voices      VoiceLister
	Generator   genx.Generator
	Transformer genx.TransformerMux
	Now         func() time.Time
}

var _ openaihttp.StrictServerInterface = (*Server)(nil)

func (s *Server) ListModels(ctx context.Context, _ openaihttp.ListModelsRequestObject) (openaihttp.ListModelsResponseObject, error) {
	if s == nil || s.Models == nil {
		return nil, errors.New("openaiapi: model service is not configured")
	}
	var out []openaihttp.Model
	cursor := (*string)(nil)
	limit := int32(200)
	for {
		resp, err := s.Models.ListModels(ctx, adminhttp.ListModelsRequestObject{
			Params: adminhttp.ListModelsParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return nil, err
		}
		list, err := modelListFromResponse(resp)
		if err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			out = append(out, openAIModel(item))
		}
		if !list.HasNext || list.NextCursor == nil || *list.NextCursor == "" {
			break
		}
		cursor = list.NextCursor
	}
	if out == nil {
		out = []openaihttp.Model{}
	}
	return openaihttp.ListModels200JSONResponse{Object: "list", Data: out}, nil
}

func (s *Server) ListVoices(ctx context.Context, params adminhttp.ListVoicesParams) (adminhttp.VoiceList, error) {
	if s == nil || s.Voices == nil {
		return adminhttp.VoiceList{}, errors.New("openaiapi: voice service is not configured")
	}
	resp, err := s.Voices.ListVoices(ctx, adminhttp.ListVoicesRequestObject{Params: params})
	if err != nil {
		return adminhttp.VoiceList{}, err
	}
	switch typed := resp.(type) {
	case adminhttp.ListVoices200JSONResponse:
		return adminhttp.VoiceList(typed), nil
	default:
		return adminhttp.VoiceList{}, fmt.Errorf("openaiapi: list voices response %T", resp)
	}
}

func (s *Server) CreateChatCompletion(ctx context.Context, request openaihttp.CreateChatCompletionRequestObject) (openaihttp.CreateChatCompletionResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("openaiapi: request body is required")
	}
	model := strings.TrimSpace(request.Body.Model)
	if model == "" {
		return nil, errors.New("openaiapi: model is required")
	}
	mctx, err := buildModelContext(request.Body)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Generator == nil {
		return nil, errors.New("openaiapi: generator is not configured")
	}
	stream, err := s.Generator.GenerateStream(ctx, "model/"+model, mctx)
	if err != nil {
		return nil, err
	}
	if request.Body.Stream != nil && *request.Body.Stream {
		pr, pw := io.Pipe()
		go func() {
			defer stream.Close()
			err := writeChatCompletionSSE(pw, stream, model, s.now())
			_ = pw.CloseWithError(err)
		}()
		return openaihttp.CreateChatCompletion200TexteventStreamResponse{Body: pr}, nil
	}
	text, err := readTextStream(stream)
	if err != nil {
		return nil, err
	}
	finish := "stop"
	now := s.now()
	return openaihttp.CreateChatCompletion200JSONResponse{
		Id:      idWithPrefix("chatcmpl", func() time.Time { return now }),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   model,
		Choices: []openaihttp.ChatCompletionChoice{
			{
				FinishReason: &finish,
				Index:        0,
				Message: openaihttp.ChatCompletionResponseMessage{
					Content: &text,
					Role:    openaihttp.ChatCompletionResponseMessageRoleAssistant,
				},
			},
		},
	}, nil
}

func (s *Server) CreateSpeech(ctx context.Context, request openaihttp.CreateSpeechRequestObject) (openaihttp.CreateSpeechResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("openaiapi: request body is required")
	}
	if strings.TrimSpace(request.Body.Input) == "" {
		return nil, errors.New("openaiapi: input is required")
	}
	pattern, err := speechPattern(request.Body)
	if err != nil {
		return nil, err
	}
	if s == nil || s.Transformer == nil {
		return nil, errors.New("openaiapi: transformer is not configured")
	}
	stream, err := s.Transformer.Transform(ctx, pattern, newTextStream(request.Body.Input))
	if err != nil {
		return nil, err
	}
	contentType := speechContentType(request.Body)
	if speechWantsSSE(request.Body) {
		pr, pw := io.Pipe()
		go func() {
			defer stream.Close()
			err := writeSpeechSSE(pw, stream, contentType)
			_ = pw.CloseWithError(err)
		}()
		return openaihttp.CreateSpeech200TexteventStreamResponse{Body: pr}, nil
	}
	audio, contentType, err := readBlobStreamWithMIME(stream, contentType)
	if err != nil {
		return nil, err
	}
	return speechAudioResponse{
		Body:          bytes.NewReader(audio),
		ContentLength: int64(len(audio)),
		ContentType:   contentType,
	}, nil
}

func speechWantsSSE(body *openaihttp.CreateSpeechRequest) bool {
	if body == nil {
		return false
	}
	if body.Stream != nil && *body.Stream {
		return true
	}
	return body.StreamFormat != nil && *body.StreamFormat == openaihttp.Sse
}

func speechContentType(body *openaihttp.CreateSpeechRequest) string {
	if body == nil || body.ResponseFormat == nil {
		return "audio/mpeg"
	}
	switch strings.ToLower(strings.TrimSpace(*body.ResponseFormat)) {
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/pcm"
	case "mp3", "":
		return "audio/mpeg"
	default:
		return "audio/mpeg"
	}
}

func (s *Server) CreateTranscription(ctx context.Context, request openaihttp.CreateTranscriptionRequestObject) (openaihttp.CreateTranscriptionResponseObject, error) {
	form, err := parseTranscriptionForm(request.Body)
	if err != nil {
		return nil, err
	}
	if request.Params.Accept != nil && transcriptionAcceptsEventStream(*request.Params.Accept) {
		form.stream = true
	}
	if s == nil || s.Transformer == nil {
		return nil, errors.New("openaiapi: transformer is not configured")
	}
	stream, err := s.Transformer.Transform(ctx, "model/"+form.model, form.input)
	if err != nil {
		_ = form.input.CloseWithError(err)
		return nil, err
	}
	if form.stream {
		pr, pw := io.Pipe()
		go func() {
			defer stream.Close()
			err := writeTranscriptionSSE(pw, stream)
			_ = pw.CloseWithError(err)
		}()
		return openaihttp.CreateTranscription200TexteventStreamResponse{Body: pr}, nil
	}
	text, err := readTextStream(stream)
	if err != nil {
		return nil, err
	}
	return openaihttp.CreateTranscription200JSONResponse{Text: text}, nil
}

func transcriptionAcceptsEventStream(accept string) bool {
	for _, part := range strings.Split(accept, ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if strings.EqualFold(mediaType, "text/event-stream") {
			return true
		}
	}
	return false
}

func modelListFromResponse(resp adminhttp.ListModelsResponseObject) (adminhttp.ModelList, error) {
	switch v := resp.(type) {
	case adminhttp.ListModels200JSONResponse:
		return adminhttp.ModelList(v), nil
	default:
		return adminhttp.ModelList{}, fmt.Errorf("openaiapi: list models response %T", resp)
	}
}

func openAIModel(model apitypes.Model) openaihttp.Model {
	owner := strings.TrimSpace(model.Provider.Name)
	if owner == "" {
		owner = strings.TrimSpace(string(model.Provider.Kind))
	}
	if owner == "" {
		owner = "gizclaw"
	}
	created := model.CreatedAt.Unix()
	if model.CreatedAt.IsZero() {
		created = 0
	}
	return openaihttp.Model{
		Id:      model.Id,
		Object:  openaihttp.ModelObjectModel,
		Created: created,
		OwnedBy: owner,
	}
}

func buildModelContext(body *openaihttp.CreateChatCompletionRequest) (genx.ModelContext, error) {
	var b genx.ModelContextBuilder
	if body.Temperature != nil {
		b.Params = &genx.ModelParams{Temperature: *body.Temperature}
	}
	if body.Thinking != nil {
		if b.Params == nil {
			b.Params = &genx.ModelParams{}
		}
		b.Params.ExtraFields = thinkingExtraFields(body.Thinking)
	}
	for _, msg := range body.Messages {
		role, _ := msg["role"].(string)
		name, _ := msg["name"].(string)
		text, blobs, err := parseMessageContent(msg["content"])
		if err != nil {
			return nil, err
		}
		switch role {
		case "system", "developer":
			if strings.TrimSpace(text) != "" {
				b.PromptText(role, text)
			}
		case "user":
			if text != "" {
				b.UserText(name, text)
			}
			for _, blob := range blobs {
				b.UserBlob(name, blob.MIMEType, blob.Data)
			}
		case "assistant":
			if text != "" {
				b.ModelText(name, text)
			}
		default:
			return nil, fmt.Errorf("openaiapi: unsupported chat message role %q", role)
		}
	}
	return b.Build(), nil
}

func thinkingExtraFields(options *openaihttp.ThinkingOptions) map[string]any {
	out := map[string]any{}
	if options.Enabled != nil {
		out["enable_thinking"] = *options.Enabled
	}
	if options.Level != nil && strings.TrimSpace(*options.Level) != "" {
		out["reasoning_effort"] = strings.TrimSpace(*options.Level)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseMessageContent(value any) (string, []*genx.Blob, error) {
	switch v := value.(type) {
	case string:
		return v, nil, nil
	case []interface{}:
		var text strings.Builder
		var blobs []*genx.Blob
		for _, raw := range v {
			part, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			switch part["type"] {
			case "text":
				if s, ok := part["text"].(string); ok {
					text.WriteString(s)
				}
			case "input_audio":
				blob, err := parseInputAudio(part["input_audio"])
				if err != nil {
					return "", nil, err
				}
				blobs = append(blobs, blob)
			}
		}
		return text.String(), blobs, nil
	case nil:
		return "", nil, nil
	default:
		return "", nil, fmt.Errorf("openaiapi: unsupported message content type %T", value)
	}
}

func parseInputAudio(value any) (*genx.Blob, error) {
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil, errors.New("openaiapi: input_audio must be an object")
	}
	data, _ := m["data"].(string)
	format, _ := m["format"].(string)
	if data == "" || format == "" {
		return nil, errors.New("openaiapi: input_audio data and format are required")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("openaiapi: decode input_audio: %w", err)
	}
	return &genx.Blob{MIMEType: "audio/" + format, Data: decoded}, nil
}

func speechPattern(body *openaihttp.CreateSpeechRequest) (string, error) {
	voice := strings.TrimSpace(body.Voice)
	if voice != "" {
		return "voice/" + voice, nil
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		return "", errors.New("openaiapi: model or voice is required")
	}
	return "model/" + model, nil
}

type transcriptionForm struct {
	model    string
	stream   bool
	input    genx.Stream
	mimeType string
}

func parseTranscriptionForm(r *multipart.Reader) (transcriptionForm, error) {
	if r == nil {
		return transcriptionForm{}, errors.New("openaiapi: multipart body is required")
	}
	var out transcriptionForm
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return transcriptionForm{}, err
		}
		switch part.FormName() {
		case "model":
			body, err := io.ReadAll(part)
			if err != nil {
				return transcriptionForm{}, err
			}
			out.model = strings.TrimSpace(string(body))
		case "stream":
			body, err := io.ReadAll(part)
			if err != nil {
				return transcriptionForm{}, err
			}
			out.stream = strings.TrimSpace(string(body)) == "true"
		case "file":
			out.mimeType = part.Header.Get("Content-Type")
			out.mimeType = transcriptionAudioMIME(out.mimeType, part.FileName(), nil)
			if out.model != "" {
				if out.mimeType == "" {
					out.mimeType = "application/octet-stream"
				}
				out.input = newReaderBlobStream(part, out.mimeType, part.FileName())
				return out, nil
			}
			body, err := io.ReadAll(part)
			if err != nil {
				return transcriptionForm{}, err
			}
			out.mimeType = transcriptionAudioMIME(out.mimeType, part.FileName(), body)
			out.input = newBlobStream(out.mimeType, body)
		}
	}
	if out.model == "" {
		return transcriptionForm{}, errors.New("openaiapi: model is required")
	}
	if out.input == nil {
		return transcriptionForm{}, errors.New("openaiapi: file is required")
	}
	if out.mimeType == "" {
		out.mimeType = "application/octet-stream"
	}
	return out, nil
}

func transcriptionAudioMIME(contentType, filename string, data []byte) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}
	if extType := mime.TypeByExtension(filepath.Ext(filename)); extType != "" {
		extType = strings.TrimSpace(strings.Split(extType, ";")[0])
		if extType != "" && extType != "application/octet-stream" {
			return extType
		}
	}
	switch {
	case len(data) >= 3 && string(data[:3]) == "ID3":
		return "audio/mpeg"
	case len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		return "audio/mpeg"
	case len(data) >= 4 && string(data[:4]) == "OggS":
		return "audio/ogg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return "audio/wav"
	case len(data) >= 4 && string(data[:4]) == "fLaC":
		return "audio/flac"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return "audio/mp4"
	default:
		return contentType
	}
}

type readerBlobStream struct {
	reader   io.Reader
	closer   io.Closer
	mimeType string
	filename string
	buf      []byte
	done     bool
	err      error
}

func newReaderBlobStream(r io.Reader, mimeType, filename string) genx.Stream {
	stream := &readerBlobStream{
		reader:   r,
		mimeType: mimeType,
		filename: filename,
		buf:      make([]byte, 32*1024),
	}
	if closer, ok := r.(io.Closer); ok {
		stream.closer = closer
	}
	return stream
}

func (s *readerBlobStream) Next() (*genx.MessageChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.done {
		return nil, genx.ErrDone
	}
	for {
		n, err := s.reader.Read(s.buf)
		if n > 0 {
			data := append([]byte(nil), s.buf[:n]...)
			s.mimeType = transcriptionAudioMIME(s.mimeType, s.filename, data)
			if s.mimeType == "" {
				s.mimeType = "application/octet-stream"
			}
			return &genx.MessageChunk{Part: &genx.Blob{MIMEType: s.mimeType, Data: data}}, nil
		}
		if errors.Is(err, io.EOF) {
			s.done = true
			if s.mimeType == "" {
				s.mimeType = "application/octet-stream"
			}
			return genx.NewEndOfStream(s.mimeType), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *readerBlobStream) Close() error {
	s.done = true
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

func (s *readerBlobStream) CloseWithError(err error) error {
	s.err = err
	return s.Close()
}

func writeChatCompletionSSE(w io.Writer, stream genx.Stream, model string, now time.Time) error {
	id := idWithPrefix("chatcmpl", func() time.Time { return now })
	created := now.Unix()
	sentRole := false
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				break
			}
			return writeSSEError(w, "STREAM_ERROR", err)
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok || text == "" {
			continue
		}
		delta := openaihttp.ChatCompletionStreamResponseDelta{Content: stringPtr(string(text))}
		if !sentRole {
			role := openaihttp.ChatCompletionStreamResponseDeltaRoleAssistant
			delta.Role = &role
			sentRole = true
		}
		if err := writeSSEData(w, openaihttp.CreateChatCompletionStreamResponse{
			Id:      id,
			Object:  openaihttp.ChatCompletionChunk,
			Created: created,
			Model:   model,
			Choices: []openaihttp.ChatCompletionChunkChoice{{Index: 0, Delta: delta}},
		}); err != nil {
			return err
		}
	}
	finish := "stop"
	if err := writeSSEData(w, openaihttp.CreateChatCompletionStreamResponse{
		Id:      id,
		Object:  openaihttp.ChatCompletionChunk,
		Created: created,
		Model:   model,
		Choices: []openaihttp.ChatCompletionChunkChoice{{Index: 0, FinishReason: &finish}},
	}); err != nil {
		return err
	}
	_, err := io.WriteString(w, "data: [DONE]\n\n")
	return err
}

func writeSpeechSSE(w io.Writer, stream genx.Stream, contentType string) error {
	var normalizer *transformers.TTSAudioNormalizer
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			continue
		}
		if normalizer == nil {
			normalizer = transformers.NewTTSAudioNormalizer(blobContentType(blob, contentType))
		}
		if err := writeSpeechAudioDelta(w, normalizer.Write(blob.Data)); err != nil {
			return err
		}
	}
	if normalizer != nil {
		if err := writeSpeechAudioDelta(w, normalizer.Flush()); err != nil {
			return err
		}
	}
	done := true
	return writeSSEData(w, openaihttp.CreateSpeechResponseStreamEvent{
		Type: "speech.audio.done",
		Done: &done,
	})
}

func writeSpeechAudioDelta(w io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	audio := base64.StdEncoding.EncodeToString(data)
	return writeSSEData(w, openaihttp.CreateSpeechResponseStreamEvent{
		Type:  "speech.audio.delta",
		Audio: &audio,
	})
}

func writeTranscriptionSSE(w io.Writer, stream genx.Stream) error {
	var full strings.Builder
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok || text == "" {
			continue
		}
		delta := string(text)
		full.WriteString(delta)
		if err := writeSSEData(w, openaihttp.CreateTranscriptionResponseStreamEvent{
			Type:  "transcript.text.delta",
			Delta: &delta,
		}); err != nil {
			return err
		}
	}
	done := full.String()
	return writeSSEData(w, openaihttp.CreateTranscriptionResponseStreamEvent{
		Type: "transcript.text.done",
		Text: &done,
	})
}

func writeSSEData(w io.Writer, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func writeSSEError(w io.Writer, code string, err error) error {
	message := "upstream stream failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return writeSSEData(w, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
			"type":    "upstream_error",
		},
	})
}

func readTextStream(stream genx.Stream) (string, error) {
	defer stream.Close()
	var out strings.Builder
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return out.String(), nil
			}
			return "", err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			out.WriteString(string(text))
		}
	}
}

func readBlobStream(stream genx.Stream) ([]byte, error) {
	audio, _, err := readBlobStreamWithMIME(stream, "application/octet-stream")
	return audio, err
}

func readBlobStreamWithMIME(stream genx.Stream, contentType string) ([]byte, string, error) {
	defer stream.Close()
	var out bytes.Buffer
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var normalizer *transformers.TTSAudioNormalizer
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				if normalizer != nil {
					out.Write(normalizer.Flush())
				}
				return out.Bytes(), contentType, nil
			}
			return nil, "", err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			if out.Len() == 0 && strings.TrimSpace(blob.MIMEType) != "" {
				contentType = strings.TrimSpace(blob.MIMEType)
			}
			if normalizer == nil {
				normalizer = transformers.NewTTSAudioNormalizer(contentType)
			}
			out.Write(normalizer.Write(blob.Data))
		}
	}
}

func blobContentType(blob *genx.Blob, fallback string) string {
	if blob != nil {
		if mimeType := strings.TrimSpace(blob.MIMEType); mimeType != "" {
			return mimeType
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "application/octet-stream"
	}
	return fallback
}

type speechAudioResponse struct {
	Body          io.Reader
	ContentLength int64
	ContentType   string
}

func (response speechAudioResponse) VisitCreateSpeechResponse(ctx *fiber.Ctx) error {
	contentType := strings.TrimSpace(response.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx.Response().Header.Set("Content-Type", contentType)
	if response.ContentLength != 0 {
		ctx.Response().Header.Set("Content-Length", fmt.Sprint(response.ContentLength))
	}
	ctx.Status(200)

	if closer, ok := response.Body.(io.ReadCloser); ok {
		defer closer.Close()
	}
	_, err := io.Copy(ctx.Response().BodyWriter(), response.Body)
	return err
}

type sliceStream struct {
	chunks []*genx.MessageChunk
	err    error
}

func newTextStream(text string) genx.Stream {
	return &sliceStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text(text)},
		genx.NewTextEndOfStream(),
	}}
}

func newBlobStream(mimeType string, data []byte) genx.Stream {
	return &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: mimeType, Data: data}},
		genx.NewEndOfStream(mimeType),
	}}
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.chunks) == 0 {
		return nil, genx.ErrDone
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *sliceStream) CloseWithError(err error) error {
	s.err = err
	s.chunks = nil
	return nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func idWithPrefix(prefix string, now func() time.Time) string {
	return prefix + "-" + strconv.FormatInt(now().UnixNano(), 36)
}

func stringPtr(value string) *string {
	return &value
}
