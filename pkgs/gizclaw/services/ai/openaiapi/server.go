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
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/idy/ai-server-shell/backend"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/audiostream"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ModelLister interface {
	ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)
}

type VoiceLister interface {
	ListVoices(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error)
}

// ConversationWorkspaceRequest is the transport-neutral request for one new
// OpenAI-owned Workspace. The creator must authenticate ownership and execute
// Initialize before publishing the Workspace.
type ConversationWorkspaceRequest struct {
	Name         string
	Collection   string
	WorkflowName string
	Metadata     map[string]string
	Initialize   func(context.Context, workspace.Runtime) error
}

// ConversationWorkspaces provides owner-fenced Workspace and durable state
// access without starting an Agent for read operations.
type ConversationWorkspaces interface {
	CreateConversationWorkspace(context.Context, ConversationWorkspaceRequest) (apitypes.Workspace, error)
	GetConversationWorkspace(context.Context, string) (apitypes.Workspace, error)
	GetConversationRuntime(context.Context, string) (workspace.Runtime, error)
	AppendConversationHistory(context.Context, string, workspace.AppendHistoryRequest) (workspace.HistoryEntry, error)
}

// WorkspaceExecutor executes one request-scoped text turn against the shared
// canonical Workspace Agent. onDelta may be nil.
type WorkspaceExecutor interface {
	ExecuteWorkspaceText(context.Context, apitypes.Workspace, string, func(string) error) ([]workspace.HistoryEntry, error)
}

// ToolCallSupport reports whether the model selected by a Generator pattern
// can return function tool calls.
type ToolCallSupport interface {
	SupportsToolCalls(context.Context, string) (bool, error)
}

// VoiceListParams contains the pagination accepted by the RuntimeProfile-scoped
// OpenAI-compatible voice catalog.
type VoiceListParams struct {
	Cursor *string
	Limit  *int32
}

// Server maps the limited OpenAI-compatible surface to GizClaw resources and
// GenX. Wire parsing, validation, framing, and request IDs belong to the Shell.
type Server struct {
	Caller      giznet.PublicKey
	Models      ModelLister
	Voices      VoiceLister
	Generator   genx.Generator
	ToolCalls   ToolCallSupport
	Transformer genx.TransformerMux
	Workspaces  ConversationWorkspaces
	Executor    WorkspaceExecutor
	Responses   *ResponseRuntime
	Now         func() time.Time
}

var _ backend.Handler = (*Server)(nil)

func (s *Server) Handle(ctx context.Context, request backend.Request) (backend.Response, error) {
	if s == nil || s.Caller.IsZero() || request.Metadata.CallerID != s.Caller.String() {
		return backend.Response{}, unavailable("openai_backend_unavailable", "The OpenAI backend binding is unavailable.", nil)
	}
	switch {
	case request.Capability == backend.CapabilityModels && request.Operation == "listModels":
		return s.listModels(ctx)
	case request.Capability == backend.CapabilityChat && request.Operation == "createChatCompletion":
		return s.createChatCompletion(ctx, request)
	case request.Capability == backend.CapabilityAudio && request.Operation == "createSpeech":
		return s.createSpeech(ctx, request)
	case request.Capability == backend.CapabilityAudio && request.Operation == "createTranscription":
		return s.createTranscription(ctx, request)
	case request.Capability == backend.CapabilityConversations:
		return s.handleConversation(ctx, request)
	case request.Capability == backend.CapabilityResponses:
		return s.handleResponse(ctx, request)
	default:
		return backend.Response{}, &backend.Error{
			Kind: backend.ErrorUnsupported, Code: "operation_not_supported",
			Message: "This operation is not supported by GizClaw.",
		}
	}
}

func (s *Server) listModels(ctx context.Context) (backend.Response, error) {
	if s.Models == nil {
		return backend.Response{}, unavailable("models_unavailable", "The model service is unavailable.", nil)
	}
	models := make([]openAIModel, 0)
	var cursor *string
	limit := int32(200)
	for {
		response, err := s.Models.ListModels(ctx, adminhttp.ListModelsRequestObject{
			Params: adminhttp.ListModelsParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return backend.Response{}, internal(err)
		}
		list, err := modelListFromResponse(response)
		if err != nil {
			return backend.Response{}, internal(err)
		}
		for _, item := range list.Items {
			models = append(models, modelFromResource(item))
		}
		if !list.HasNext || list.NextCursor == nil || *list.NextCursor == "" {
			break
		}
		cursor = list.NextCursor
	}
	return jsonResponse(map[string]any{"object": "list", "data": models})
}

type thinkingOptions struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Level   *string `json:"level,omitempty"`
}

// chatCompletionRequest keeps the tool options raw so an explicit null is
// validated like any other present value instead of reading as absent.
type chatCompletionRequest struct {
	Messages          []map[string]any `json:"messages"`
	Model             string           `json:"model"`
	ParallelToolCalls json.RawMessage  `json:"parallel_tool_calls,omitempty"`
	Stream            *bool            `json:"stream,omitempty"`
	StreamOptions     json.RawMessage  `json:"stream_options,omitempty"`
	Temperature       *float32         `json:"temperature,omitempty"`
	Thinking          *thinkingOptions `json:"thinking,omitempty"`
	ToolChoice        json.RawMessage  `json:"tool_choice,omitempty"`
	Tools             []map[string]any `json:"tools,omitempty"`
}

func (s *Server) createChatCompletion(ctx context.Context, request backend.Request) (backend.Response, error) {
	var body chatCompletionRequest
	if err := decodeJSONProjection(request.Input.JSON, &body,
		"messages", "model", "parallel_tool_calls", "stream", "stream_options", "temperature", "thinking", "tool_choice", "tools",
	); err != nil {
		return backend.Response{}, err
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		return backend.Response{}, invalid("missing_model", "model", "The model field is required.")
	}
	streaming := body.Stream != nil && *body.Stream
	includeUsage, err := chatStreamOptions(body.StreamOptions, streaming)
	if err != nil {
		return backend.Response{}, err
	}
	modelContext, usesTools, err := buildModelContext(&body)
	if err != nil {
		if backendErr, ok := errors.AsType[*backend.Error](err); ok {
			return backend.Response{}, backendErr
		}
		return backend.Response{}, invalid("invalid_messages", "messages", "The messages field is invalid.")
	}
	if s.Generator == nil {
		return backend.Response{}, unavailable("generator_unavailable", "The model generator is unavailable.", nil)
	}
	if usesTools {
		if err := s.requireToolCalls(ctx, model); err != nil {
			return backend.Response{}, err
		}
	}
	stream, err := s.Generator.GenerateStream(ctx, "model/"+model, modelContext)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	if nilInterface(stream) {
		return backend.Response{}, unavailable("generator_unavailable", "The model generator is unavailable.", nil)
	}
	if streaming {
		return backend.Response{Stream: newChatEventStream(ctx, stream, model, s.now(), includeUsage)}, nil
	}
	output, err := readChatStream(stream)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	message := map[string]any{"content": output.text, "refusal": nil, "role": "assistant"}
	finishReason := "stop"
	if len(output.toolCalls) > 0 {
		message["tool_calls"] = output.toolCalls
		if output.text == "" {
			message["content"] = nil
		}
		finishReason = "tool_calls"
	}
	now := s.now()
	return jsonResponse(map[string]any{
		"id": idWithPrefix("chatcmpl", func() time.Time { return now }), "object": "chat.completion",
		"created": now.Unix(), "model": model,
		"choices": []any{map[string]any{
			"finish_reason": finishReason, "index": 0, "logprobs": nil, "message": message,
		}},
	})
}

func (s *Server) requireToolCalls(ctx context.Context, model string) error {
	if s.ToolCalls == nil {
		return unavailable("tool_calls_unavailable", "Tool call support is unavailable.", nil)
	}
	supported, err := s.ToolCalls.SupportsToolCalls(ctx, "model/"+model)
	if err != nil {
		return internal(err)
	}
	if !supported {
		return invalid("unsupported_option", "tools", "The model does not support tool calls.")
	}
	return nil
}

type speechRequest struct {
	Input          string   `json:"input"`
	Model          string   `json:"model"`
	ResponseFormat *string  `json:"response_format,omitempty"`
	Speed          *float32 `json:"speed,omitempty"`
	StreamFormat   *string  `json:"stream_format,omitempty"`
	Voice          string   `json:"voice"`
}

func (s *Server) createSpeech(ctx context.Context, request backend.Request) (backend.Response, error) {
	var body speechRequest
	if err := decodeJSONProjection(request.Input.JSON, &body, "input", "model", "response_format", "speed", "stream_format", "voice"); err != nil {
		return backend.Response{}, err
	}
	if strings.TrimSpace(body.Input) == "" {
		return backend.Response{}, invalid("missing_input", "input", "The input field is required.")
	}
	if body.Speed != nil && *body.Speed != 1 {
		return backend.Response{}, invalid("unsupported_option", "speed", "The speed option is not supported by GizClaw.")
	}
	pattern, err := speechPattern(&body)
	if err != nil {
		return backend.Response{}, invalid("missing_voice", "voice", "A model or voice is required.")
	}
	if s.Transformer == nil {
		return backend.Response{}, unavailable("transformer_unavailable", "The audio transformer is unavailable.", nil)
	}
	input := newTextStream(body.Input)
	stream, err := s.Transformer.Transform(ctx, pattern, input)
	if err != nil {
		_ = input.CloseWithError(err)
		return backend.Response{}, internal(err)
	}
	if nilInterface(stream) {
		_ = input.CloseWithError(errors.New("transformer returned a nil stream"))
		return backend.Response{}, unavailable("transformer_unavailable", "The audio transformer is unavailable.", nil)
	}
	contentType := speechContentType(&body)
	if body.StreamFormat != nil && *body.StreamFormat == "sse" {
		return backend.Response{Stream: newSpeechEventStream(ctx, stream, contentType)}, nil
	}
	audio, contentType, err := readBlobStreamWithMIME(stream, contentType)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	return backend.Response{MediaType: contentType, Body: io.NopCloser(bytes.NewReader(audio))}, nil
}

func (s *Server) createTranscription(ctx context.Context, request backend.Request) (backend.Response, error) {
	contentType := firstHeader(request.Metadata.Extensions, "Content-Type")
	_, parameters, err := mime.ParseMediaType(contentType)
	if err != nil || parameters["boundary"] == "" {
		return backend.Response{}, invalid("invalid_multipart", "file", "A valid multipart request is required.")
	}
	form, err := parseTranscriptionForm(multipart.NewReader(bytes.NewReader(request.Input.Bytes), parameters["boundary"]))
	if err != nil {
		return backend.Response{}, err
	}
	if transcriptionAcceptsEventStream(firstHeader(request.Metadata.Extensions, "Accept")) {
		form.stream = true
	}
	if s.Transformer == nil {
		return backend.Response{}, unavailable("transformer_unavailable", "The audio transformer is unavailable.", nil)
	}
	stream, err := s.Transformer.Transform(ctx, "model/"+form.model, form.input)
	if err != nil {
		_ = form.input.CloseWithError(err)
		return backend.Response{}, internal(err)
	}
	if nilInterface(stream) {
		_ = form.input.CloseWithError(errors.New("transformer returned a nil stream"))
		return backend.Response{}, unavailable("transformer_unavailable", "The audio transformer is unavailable.", nil)
	}
	if form.stream {
		return backend.Response{Stream: newTranscriptionEventStream(ctx, stream)}, nil
	}
	text, err := readTextStream(stream)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	return jsonResponse(map[string]string{"text": text})
}

func (s *Server) ListVoices(ctx context.Context, params VoiceListParams) (adminhttp.VoiceList, error) {
	if s == nil || s.Voices == nil {
		return adminhttp.VoiceList{}, errors.New("openaiapi: voice service is not configured")
	}
	response, err := s.Voices.ListVoices(ctx, adminhttp.ListVoicesRequestObject{Params: adminhttp.ListVoicesParams{
		Cursor: params.Cursor, Limit: params.Limit,
	}})
	if err != nil {
		return adminhttp.VoiceList{}, err
	}
	switch typed := response.(type) {
	case adminhttp.ListVoices200JSONResponse:
		return adminhttp.VoiceList(typed), nil
	default:
		return adminhttp.VoiceList{}, fmt.Errorf("openaiapi: list voices response %T", response)
	}
}

func decodeJSONProjection(input json.RawMessage, target any, allowed ...string) error {
	if len(input) == 0 {
		return invalid("missing_body", "", "A JSON request body is required.")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return invalid("invalid_json", "", "The JSON request body is invalid.")
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedSet[field]; !ok {
			return invalid("unsupported_option", "request", "A request option is not supported by GizClaw.")
		}
	}
	if err := json.Unmarshal(input, target); err != nil {
		return invalid("invalid_json", "", "The JSON request body is invalid.")
	}
	return nil
}

func jsonResponse(value any) (backend.Response, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	return backend.Response{JSON: data}, nil
}

func invalid(code, parameter, message string) error {
	return &backend.Error{Kind: backend.ErrorInvalid, Code: code, Param: parameter, Message: message}
}

func unavailable(code, message string, cause error) error {
	return &backend.Error{Kind: backend.ErrorUnavailable, Code: code, Message: message, Cause: cause}
}

func internal(cause error) error {
	if errors.Is(cause, context.Canceled) {
		return &backend.Error{Kind: backend.ErrorCanceled, Code: "request_canceled", Message: "The request was canceled.", Cause: cause}
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return &backend.Error{Kind: backend.ErrorTimeout, Code: "request_timeout", Message: "The request timed out.", Cause: cause}
	}
	return &backend.Error{Kind: backend.ErrorInternal, Code: "backend_error", Message: "The GizClaw backend failed.", Cause: cause}
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func modelListFromResponse(response adminhttp.ListModelsResponseObject) (adminhttp.ModelList, error) {
	switch typed := response.(type) {
	case adminhttp.ListModels200JSONResponse:
		return adminhttp.ModelList(typed), nil
	default:
		return adminhttp.ModelList{}, fmt.Errorf("openaiapi: list models response %T", response)
	}
}

func modelFromResource(model apitypes.Model) openAIModel {
	owner := strings.TrimSpace(model.Provider.Id)
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
	return openAIModel{ID: model.Id, Object: "model", Created: created, OwnedBy: owner}
}

// buildModelContext maps a chat request to GenX and reports whether it
// declares tools or replays tool calls, which requires model support.
func buildModelContext(body *chatCompletionRequest) (genx.ModelContext, bool, error) {
	var builder genx.ModelContextBuilder
	if body.Temperature != nil {
		builder.Params = &genx.ModelParams{Temperature: *body.Temperature}
	}
	if body.Thinking != nil {
		if builder.Params == nil {
			builder.Params = &genx.ModelParams{}
		}
		builder.Params.Thinking = thinkingParams(body.Thinking)
	}
	tools, err := chatTools(body)
	if err != nil {
		return nil, false, err
	}
	for _, tool := range tools {
		builder.AddTool(tool)
	}
	usesTools := len(tools) > 0
	for _, message := range body.Messages {
		role, _ := message["role"].(string)
		name, _ := message["name"].(string)
		allowed := []string{"role", "name", "content"}
		switch role {
		case "assistant":
			allowed = append(allowed, "tool_calls")
		case "tool":
			allowed = []string{"role", "content", "tool_call_id"}
		}
		if err := rejectUnknownFields(message, "messages", allowed...); err != nil {
			return nil, false, err
		}
		text, blobs, err := parseMessageContent(message["content"], role == "assistant")
		if err != nil {
			return nil, false, err
		}
		switch role {
		case "system", "developer":
			if strings.TrimSpace(text) != "" {
				builder.PromptText(role, text)
			}
		case "user":
			if text != "" {
				builder.UserText(name, text)
			}
			for _, blob := range blobs {
				builder.UserBlob(name, blob.MIMEType, blob.Data)
			}
		case "assistant":
			if len(blobs) > 0 {
				return nil, false, invalid("unsupported_option", "messages.content", "Assistant audio content is not supported by GizClaw.")
			}
			if text != "" {
				builder.ModelText(name, text)
			}
			calls, err := parseAssistantToolCalls(message["tool_calls"])
			if err != nil {
				return nil, false, err
			}
			for _, call := range calls {
				builder.AddMessage(&genx.Message{Role: genx.RoleModel, Name: name, Payload: call})
			}
			usesTools = usesTools || len(calls) > 0
		case "tool":
			id, _ := message["tool_call_id"].(string)
			if strings.TrimSpace(id) == "" {
				return nil, false, invalid("invalid_messages", "messages.tool_call_id", "A tool message requires tool_call_id.")
			}
			if len(blobs) > 0 {
				return nil, false, invalid("unsupported_option", "messages.content", "Tool message content must be text.")
			}
			builder.AddMessage(&genx.Message{Role: genx.RoleTool, Payload: &genx.ToolResult{ID: id, Result: text}})
			usesTools = true
		default:
			return nil, false, invalid("unsupported_option", "messages.role", "This message role is not supported by GizClaw.")
		}
	}
	return builder.Build(), usesTools, nil
}

func thinkingParams(options *thinkingOptions) *genx.ThinkingParams {
	if options == nil {
		return nil
	}
	result := &genx.ThinkingParams{Enabled: options.Enabled}
	if options.Level != nil {
		result.Level = strings.TrimSpace(*options.Level)
	}
	if result.Enabled == nil && result.Level == "" {
		return nil
	}
	return result
}

// parseMessageContent reads text and audio parts. Assistant history text
// parts may echo the rest of an earlier response message: @openai/agents
// replays role, refusal, tool_calls and provider fields beside type and text.
// Those echoes carry no instruction, so only type and text are read there.
func parseMessageContent(value any, assistantHistory bool) (string, []*genx.Blob, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil, nil
	case []any:
		var text strings.Builder
		var blobs []*genx.Blob
		for _, raw := range typed {
			part, ok := raw.(map[string]any)
			if !ok {
				return "", nil, invalid("invalid_messages", "messages.content", "A message content part is invalid.")
			}
			switch part["type"] {
			case "text":
				if !assistantHistory {
					if err := requireFields(part, "type", "text"); err != nil {
						return "", nil, err
					}
				}
				if value, ok := part["text"].(string); ok {
					text.WriteString(value)
				}
			case "input_audio":
				if err := requireFields(part, "type", "input_audio"); err != nil {
					return "", nil, err
				}
				blob, err := parseInputAudio(part["input_audio"])
				if err != nil {
					return "", nil, err
				}
				blobs = append(blobs, blob)
			default:
				return "", nil, invalid("unsupported_option", "messages.content", "This message content type is not supported by GizClaw.")
			}
		}
		return text.String(), blobs, nil
	case nil:
		return "", nil, nil
	default:
		return "", nil, invalid("invalid_messages", "messages.content", "A message content value is invalid.")
	}
}

func requireFields(value map[string]any, allowed ...string) error {
	return rejectUnknownFields(value, "messages.content", allowed...)
}

func rejectUnknownFields(value map[string]any, parameter string, allowed ...string) error {
	for field := range value {
		if !slices.Contains(allowed, field) {
			return invalid("unsupported_option", parameter, "A request option is not supported by GizClaw.")
		}
	}
	return nil
}

func parseInputAudio(value any) (*genx.Blob, error) {
	input, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("input_audio must be an object")
	}
	if err := requireFields(input, "data", "format"); err != nil {
		return nil, err
	}
	data, _ := input["data"].(string)
	format, _ := input["format"].(string)
	if data == "" || format == "" {
		return nil, errors.New("input_audio data and format are required")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode input_audio: %w", err)
	}
	return &genx.Blob{MIMEType: "audio/" + format, Data: decoded}, nil
}

func speechPattern(body *speechRequest) (string, error) {
	format := speechTransformerFormat(body)
	if voice := strings.TrimSpace(body.Voice); voice != "" {
		return "voice/" + voice + "?format=" + format, nil
	}
	if model := strings.TrimSpace(body.Model); model != "" {
		return "model/" + model + "?format=" + format, nil
	}
	return "", errors.New("model or voice is required")
}

func speechTransformerFormat(body *speechRequest) string {
	if body == nil || body.ResponseFormat == nil {
		return "mp3"
	}
	switch format := strings.ToLower(strings.TrimSpace(*body.ResponseFormat)); format {
	case "opus":
		return "ogg_opus"
	case "aac", "flac", "mp3", "pcm", "wav":
		return format
	default:
		return "mp3"
	}
}

func speechContentType(body *speechRequest) string {
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
	default:
		return "audio/mpeg"
	}
}

type transcriptionForm struct {
	model  string
	stream bool
	input  genx.Stream
}

func parseTranscriptionForm(reader *multipart.Reader) (transcriptionForm, error) {
	if reader == nil {
		return transcriptionForm{}, invalid("invalid_multipart", "file", "A valid multipart request is required.")
	}
	var result transcriptionForm
	var file []byte
	var filename, contentType string
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return transcriptionForm{}, invalid("invalid_multipart", "file", "A valid multipart request is required.")
		}
		name := part.FormName()
		filenameValue := part.FileName()
		partContentType := part.Header.Get("Content-Type")
		body, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return transcriptionForm{}, internal(err)
		}
		switch name {
		case "model":
			result.model = strings.TrimSpace(string(body))
		case "stream":
			value, err := strconv.ParseBool(strings.TrimSpace(string(body)))
			if err != nil {
				return transcriptionForm{}, invalid("invalid_stream", "stream", "The stream field must be a boolean.")
			}
			result.stream = value
		case "response_format":
			if value := strings.TrimSpace(string(body)); value != "" && value != "json" {
				return transcriptionForm{}, invalid("unsupported_option", "response_format", "Only the json response format is supported by GizClaw.")
			}
		case "file":
			file = append([]byte(nil), body...)
			filename = filenameValue
			contentType = partContentType
		case "":
		default:
			return transcriptionForm{}, invalid("unsupported_option", "request", "A transcription option is not supported by GizClaw.")
		}
	}
	if result.model == "" {
		return transcriptionForm{}, invalid("missing_model", "model", "The model field is required.")
	}
	if file == nil {
		return transcriptionForm{}, invalid("missing_file", "file", "The file field is required.")
	}
	contentType = transcriptionAudioMIME(contentType, filename, file)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	result.input = newBlobStream(contentType, file)
	return result, nil
}

func transcriptionAcceptsEventStream(accept string) bool {
	for part := range strings.SplitSeq(accept, ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err == nil && strings.EqualFold(mediaType, "text/event-stream") {
			return true
		}
	}
	return false
}

func firstHeader(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func transcriptionAudioMIME(contentType, filename string, data []byte) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}
	if extensionType := mime.TypeByExtension(filepath.Ext(filename)); extensionType != "" {
		extensionType = strings.TrimSpace(strings.Split(extensionType, ";")[0])
		if extensionType != "application/octet-stream" {
			return extensionType
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

type eventStream struct {
	events      chan backend.Event
	cancel      context.CancelFunc
	closeSource func() error
	closeOnce   sync.Once
}

type eventSender func(json.RawMessage) bool

func newEventStream(ctx context.Context, source genx.Stream, produce func(context.Context, genx.Stream, eventSender)) backend.Stream {
	streamCtx, cancel := context.WithCancel(ctx)
	result := &eventStream{events: make(chan backend.Event, 1), cancel: cancel}
	result.closeSource = sync.OnceValue(source.Close)
	stopContextClose := context.AfterFunc(streamCtx, func() {
		_ = result.closeSource()
	})
	go func() {
		defer close(result.events)
		defer result.closeSource()
		defer stopContextClose()
		send := func(data json.RawMessage) bool {
			select {
			case result.events <- backend.Event{Data: data}:
				return true
			case <-streamCtx.Done():
				return false
			}
		}
		produce(streamCtx, source, send)
	}()
	return result
}

func (s *eventStream) Events() <-chan backend.Event { return s.events }

func (s *eventStream) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.closeSource()
	})
	return nil
}

func sendJSON(send eventSender, value any) bool {
	data, err := json.Marshal(value)
	return err == nil && send(data)
}

func newChatEventStream(ctx context.Context, source genx.Stream, model string, now time.Time, includeUsage bool) backend.Stream {
	id := idWithPrefix("chatcmpl", func() time.Time { return now })
	created := now.Unix()
	return newEventStream(ctx, source, func(streamCtx context.Context, source genx.Stream, send eventSender) {
		sentRole := false
		toolCalls := 0
		sendChoice := func(choice map[string]any) bool {
			return sendJSON(send, map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []any{choice},
			})
		}
		sendDelta := func(delta map[string]any) bool {
			if !sentRole {
				delta["role"] = "assistant"
				sentRole = true
			}
			return sendChoice(map[string]any{"index": 0, "delta": delta})
		}
		var doneErr error
		for {
			chunk, err := source.Next()
			if streamDone(err) {
				if streamCtx.Err() != nil {
					return
				}
				doneErr = err
				break
			}
			if err != nil {
				sendJSON(send, streamErrorEvent())
				return
			}
			if chunk == nil {
				continue
			}
			if call := chunk.ToolCall; call != nil && call.FuncCall != nil {
				toolCall := chatToolCall(call)
				toolCall["index"] = toolCalls
				toolCalls++
				if !sendDelta(map[string]any{"tool_calls": []any{toolCall}}) {
					return
				}
			}
			if chunk.IsEndOfStream() {
				continue
			}
			text, ok := chunk.Part.(genx.Text)
			if !ok || text == "" {
				continue
			}
			if !sendDelta(map[string]any{"content": string(text)}) {
				return
			}
		}
		finishReason := "stop"
		if toolCalls > 0 {
			finishReason = "tool_calls"
		}
		if !sendChoice(map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finishReason}) {
			return
		}
		if usage, ok := chatUsage(doneErr); includeUsage && ok {
			if !sendJSON(send, map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []any{}, "usage": usage,
			}) {
				return
			}
		}
		send(json.RawMessage("[DONE]"))
	})
}

func newSpeechEventStream(ctx context.Context, source genx.Stream, contentType string) backend.Stream {
	return newEventStream(ctx, source, func(streamCtx context.Context, source genx.Stream, send eventSender) {
		var normalizer *audiostream.Normalizer
		for {
			chunk, err := source.Next()
			if streamDone(err) {
				if streamCtx.Err() != nil {
					return
				}
				break
			}
			if err != nil || chunk != nil && chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
				sendJSON(send, streamErrorEvent())
				return
			}
			if chunk == nil || chunk.IsEndOfStream() {
				continue
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || len(blob.Data) == 0 {
				continue
			}
			if normalizer == nil {
				normalizer = audiostream.NewNormalizer(blobContentType(blob, contentType))
			}
			if !sendSpeechDelta(send, normalizer.Normalize(blob.Data)) {
				return
			}
		}
		if normalizer != nil && !sendSpeechDelta(send, normalizer.Flush()) {
			return
		}
		sendJSON(send, map[string]any{"type": "speech.audio.done", "done": true})
	})
}

func sendSpeechDelta(send eventSender, data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return sendJSON(send, map[string]any{"type": "speech.audio.delta", "audio": base64.StdEncoding.EncodeToString(data)})
}

func newTranscriptionEventStream(ctx context.Context, source genx.Stream) backend.Stream {
	return newEventStream(ctx, source, func(streamCtx context.Context, source genx.Stream, send eventSender) {
		var full strings.Builder
		for {
			chunk, err := source.Next()
			if streamDone(err) {
				if streamCtx.Err() != nil {
					return
				}
				break
			}
			if err != nil {
				sendJSON(send, streamErrorEvent())
				return
			}
			if chunk == nil || chunk.IsEndOfStream() {
				continue
			}
			text, ok := chunk.Part.(genx.Text)
			if !ok || text == "" {
				continue
			}
			full.WriteString(string(text))
			if !sendJSON(send, map[string]any{"type": "transcript.text.delta", "delta": string(text)}) {
				return
			}
		}
		sendJSON(send, map[string]any{"type": "transcript.text.done", "text": full.String()})
	})
}

func streamErrorEvent() map[string]any {
	return map[string]any{"error": map[string]string{
		"code": "stream_error", "message": "The GizClaw backend stream failed.", "type": "server_error",
	}}
}

func streamDone(err error) bool {
	return errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF)
}

func readTextStream(stream genx.Stream) (string, error) {
	defer stream.Close()
	var result strings.Builder
	for {
		chunk, err := stream.Next()
		if streamDone(err) {
			return result.String(), nil
		}
		if err != nil {
			return "", err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			result.WriteString(string(text))
		}
	}
}

func readBlobStreamWithMIME(stream genx.Stream, contentType string) ([]byte, string, error) {
	defer stream.Close()
	var result bytes.Buffer
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var normalizer *audiostream.Normalizer
	for {
		chunk, err := stream.Next()
		if streamDone(err) {
			if normalizer != nil {
				result.Write(normalizer.Flush())
			}
			return result.Bytes(), contentType, nil
		}
		if err != nil {
			return nil, "", err
		}
		if chunk == nil {
			continue
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Error) != "" {
			return nil, "", errors.New("audio stream failed")
		}
		if chunk.IsEndOfStream() {
			continue
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			if result.Len() == 0 && strings.TrimSpace(blob.MIMEType) != "" {
				contentType = strings.TrimSpace(blob.MIMEType)
			}
			if normalizer == nil {
				normalizer = audiostream.NewNormalizer(contentType)
			}
			result.Write(normalizer.Normalize(blob.Data))
		}
	}
}

func blobContentType(blob *genx.Blob, fallback string) string {
	if blob != nil && strings.TrimSpace(blob.MIMEType) != "" {
		return strings.TrimSpace(blob.MIMEType)
	}
	if strings.TrimSpace(fallback) == "" {
		return "application/octet-stream"
	}
	return strings.TrimSpace(fallback)
}

type sliceStream struct {
	chunks []*genx.MessageChunk
	err    error
}

func newTextStream(text string) genx.Stream {
	return &sliceStream{chunks: []*genx.MessageChunk{{Part: genx.Text(text)}, genx.NewTextEndOfStream()}}
}

func newBlobStream(mimeType string, data []byte) genx.Stream {
	return &sliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: mimeType, Data: data}}, genx.NewEndOfStream(mimeType),
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
