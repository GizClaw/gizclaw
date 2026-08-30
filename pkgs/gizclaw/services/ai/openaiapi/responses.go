package openaiapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/idy/ai-server-shell/backend"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

type createResponseRequest struct {
	Conversation json.RawMessage   `json:"conversation"`
	Input        json.RawMessage   `json:"input"`
	Model        *string           `json:"model"`
	Stream       *bool             `json:"stream"`
	Store        *bool             `json:"store"`
	Background   *bool             `json:"background"`
	Metadata     map[string]string `json:"metadata"`
}

func (s *Server) handleResponse(ctx context.Context, request backend.Request) (backend.Response, error) {
	switch request.Operation {
	case "createResponse":
		return s.createResponse(ctx, request)
	case "getResponse":
		return s.getResponse(ctx, request)
	case "listInputItems":
		return s.listResponseInputItems(ctx, request)
	case "cancelResponse":
		return s.cancelResponse(ctx, request)
	default:
		return backend.Response{}, unsupportedOperation()
	}
}

func (s *Server) createResponse(ctx context.Context, request backend.Request) (backend.Response, error) {
	if s.Workspaces == nil || s.Executor == nil {
		return backend.Response{}, unavailable("workspace_agent_unavailable", "The Workspace Agent service is unavailable.", nil)
	}
	var body createResponseRequest
	if err := decodeJSONProjection(request.Input.JSON, &body, "conversation", "input", "model", "stream", "store", "background", "metadata"); err != nil {
		return backend.Response{}, err
	}
	conversationID, err := decodeConversationID(body.Conversation)
	if err != nil {
		return backend.Response{}, invalid("invalid_conversation", "conversation", err.Error())
	}
	text, err := decodeResponseText(body.Input)
	if err != nil {
		return backend.Response{}, invalid("unsupported_option", "input", err.Error())
	}
	if body.Store != nil && !*body.Store {
		return backend.Response{}, invalid("unsupported_option", "store", "store:false is not supported by GizClaw.")
	}
	streaming := body.Stream != nil && *body.Stream
	background := body.Background != nil && *body.Background
	if streaming && background {
		return backend.Response{}, invalid("unsupported_option", "background", "background:true cannot be combined with stream:true.")
	}
	workspaceName, runtime, conversation, resolveErr := s.resolveConversation(ctx, conversationID)
	if resolveErr != nil {
		return backend.Response{}, resolveErr
	}
	model := strings.TrimSpace(conversation.Metadata["workflow_name"])
	if body.Model != nil && strings.TrimSpace(*body.Model) != "" && strings.TrimSpace(*body.Model) != model {
		return backend.Response{}, invalid("unsupported_option", "model", "model must match the Conversation workflow_name.")
	}
	workspaceItem, err := s.Workspaces.GetConversationWorkspace(ctx, workspaceName)
	if err != nil {
		return backend.Response{}, notFound("conversation_not_found", "Conversation not found.")
	}
	responseID, err := responseIDForWorkspace(workspaceName)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	responseRuntime := s.responseRuntime()
	active, err := responseRuntime.acquire(workspaceItem.Id, responseID)
	if err != nil {
		return backend.Response{}, &backend.Error{Kind: backend.ErrorConflict, Code: "response_conflict", Message: err.Error()}
	}
	createdAt := s.now()
	existing, err := runtime.OpenAI.Items(ctx)
	if err != nil {
		responseRuntime.release(workspaceItem.Id, active)
		return backend.Response{}, internal(err)
	}
	userItemID := newItemID("msg")
	userHistory, err := s.Workspaces.AppendConversationHistory(ctx, workspaceItem.Id, workspace.AppendHistoryRequest{
		Type: "gear", GearID: userItemID, Origin: workspace.HistoryOriginOpenAI, Name: "user", Text: text, CreatedAt: createdAt,
	})
	if err != nil {
		responseRuntime.release(workspaceItem.Id, active)
		return backend.Response{}, internal(err)
	}
	userItem := workspace.OpenAIItem{ID: userItemID, HistoryID: userHistory.ID, Role: "user", Status: "completed", Sequence: uint64(len(existing)), CreatedAt: userHistory.CreatedAt}
	if err := runtime.OpenAI.PutItem(ctx, userItem); err != nil {
		responseRuntime.release(workspaceItem.Id, active)
		return backend.Response{}, internal(err)
	}
	inputIDs := make([]string, 0, len(existing)+1)
	for _, item := range existing {
		inputIDs = append(inputIDs, item.ID)
	}
	inputIDs = append(inputIDs, userItem.ID)
	outputItemID := newItemID("msg")
	record := workspace.OpenAIResponse{
		ID: responseID, ConversationID: conversation.ID, WorkspaceID: workspaceItem.Id, Model: model,
		Status: "in_progress", Metadata: cloneStringMap(body.Metadata), InputItemIDs: inputIDs,
		OutputItemIDs: []string{outputItemID}, CreatedAt: createdAt,
	}
	if err := runtime.OpenAI.PutResponse(ctx, record); err != nil {
		responseRuntime.release(workspaceItem.Id, active)
		return backend.Response{}, internal(err)
	}
	job := responseJob{server: s, workspace: workspaceItem, runtime: runtime, record: record, outputItemID: outputItemID, input: text, active: active}
	if background {
		go func() { _, _ = job.run(context.WithoutCancel(ctx), nil) }()
		return s.responseJSON(ctx, runtime, record)
	}
	if streaming {
		return backend.Response{Stream: newResponseEventStream(ctx, &job)}, nil
	}
	record, err = job.run(ctx, nil)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	return s.responseJSON(ctx, runtime, record)
}

type responseJob struct {
	server       *Server
	workspace    apitypes.Workspace
	runtime      workspace.Runtime
	record       workspace.OpenAIResponse
	outputItemID string
	input        string
	active       *activeResponse
}

func (j *responseJob) run(parent context.Context, delta func(string) error) (workspace.OpenAIResponse, error) {
	runtime := j.server.responseRuntime()
	ctx, cancel := runtime.context(parent, j.active)
	defer cancel()
	defer runtime.release(j.workspace.Id, j.active)
	entries, err := j.server.Executor.ExecuteWorkspaceText(ctx, j.workspace, j.input, delta)
	now := j.server.now()
	result := j.record
	result.CompletedAt = &now
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			result.Status = "cancelled"
		} else {
			slog.ErrorContext(parent, "gizclaw: OpenAI Workspace Agent failed",
				"workspace_id", j.workspace.Id,
				"error_class", "workspace_agent_execution",
			)
			result.Status = "failed"
			result.ErrorCode = "server_error"
			result.ErrorMessage = "The Workspace Agent failed to complete the Response."
		}
		if persistErr := j.runtime.OpenAI.PutResponse(context.WithoutCancel(parent), result); persistErr != nil {
			return result, fmt.Errorf("persist terminal Response: %w", persistErr)
		}
		return result, nil
	}
	if len(entries) == 0 {
		result.Status = "failed"
		result.ErrorCode = "server_error"
		result.ErrorMessage = "The Workspace Agent produced no textual output."
		if persistErr := j.runtime.OpenAI.PutResponse(context.WithoutCancel(parent), result); persistErr != nil {
			return result, fmt.Errorf("persist empty-output Response: %w", persistErr)
		}
		return result, nil
	}
	result.OutputItemIDs = result.OutputItemIDs[:0]
	for index, entry := range entries {
		itemID := j.outputItemID
		if index > 0 {
			itemID = newItemID("msg")
		}
		item := workspace.OpenAIItem{ID: itemID, HistoryID: entry.ID, Role: "assistant", Status: "completed", Sequence: uint64(len(result.InputItemIDs) + index), CreatedAt: entry.CreatedAt}
		if err := j.runtime.OpenAI.PutItem(ctx, item); err != nil {
			result.Status = "failed"
			result.ErrorCode = "server_error"
			result.ErrorMessage = "The Response could not be persisted."
			if persistErr := j.runtime.OpenAI.PutResponse(context.WithoutCancel(parent), result); persistErr != nil {
				return result, fmt.Errorf("persist failed Response after item error: %w", persistErr)
			}
			return result, nil
		}
		result.OutputItemIDs = append(result.OutputItemIDs, itemID)
	}
	result.Status = "completed"
	if err := j.runtime.OpenAI.PutResponse(ctx, result); err != nil {
		result.Status = "failed"
		result.ErrorCode = "server_error"
		result.ErrorMessage = "The Response could not be persisted."
		if persistErr := j.runtime.OpenAI.PutResponse(context.WithoutCancel(parent), result); persistErr != nil {
			return result, fmt.Errorf("persist failed Response after terminal error: %w", persistErr)
		}
	}
	return result, nil
}

func (s *Server) getResponse(ctx context.Context, request backend.Request) (backend.Response, error) {
	_, runtime, record, err := s.resolveResponse(ctx, parameterString(request, "response_id"))
	if err != nil {
		return backend.Response{}, err
	}
	return s.responseJSON(ctx, runtime, record)
}

func (s *Server) listResponseInputItems(ctx context.Context, request backend.Request) (backend.Response, error) {
	_, runtime, record, err := s.resolveResponse(ctx, parameterString(request, "response_id"))
	if err != nil {
		return backend.Response{}, err
	}
	items := make([]workspace.OpenAIItem, 0, len(record.InputItemIDs))
	for _, id := range record.InputItemIDs {
		item, getErr := runtime.OpenAI.Item(ctx, id)
		if getErr != nil {
			return backend.Response{}, internal(getErr)
		}
		items = append(items, item)
	}
	page := paginateOpenAIItems(items, request)
	data := make([]any, 0, len(page.items))
	for _, item := range page.items {
		object, objectErr := s.itemObject(ctx, runtime, item)
		if objectErr != nil {
			return backend.Response{}, objectErr
		}
		data = append(data, object)
	}
	return jsonResponse(listObject(data, page.hasMore))
}

func (s *Server) cancelResponse(ctx context.Context, request backend.Request) (backend.Response, error) {
	workspaceItem, runtime, record, err := s.resolveResponse(ctx, parameterString(request, "response_id"))
	if err != nil {
		return backend.Response{}, err
	}
	if record.Status != "in_progress" {
		return backend.Response{}, &backend.Error{Kind: backend.ErrorConflict, Code: "response_not_cancellable", Message: "The Response is not cancellable."}
	}
	done, cancelled := s.responseRuntime().cancelResponse(workspaceItem.Id, record.ID)
	if !cancelled {
		return backend.Response{}, &backend.Error{Kind: backend.ErrorConflict, Code: "response_not_cancellable", Message: "The Response is not cancellable."}
	}
	select {
	case <-done:
	case <-ctx.Done():
		return backend.Response{}, internal(ctx.Err())
	}
	stored, err := runtime.OpenAI.Response(ctx, record.ID)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	return s.responseJSON(ctx, runtime, stored)
}

func (s *Server) resolveResponse(ctx context.Context, id string) (apitypes.Workspace, workspace.Runtime, workspace.OpenAIResponse, error) {
	workspaceName, ok := workspaceNameFromResponseID(id)
	if !ok {
		return apitypes.Workspace{}, workspace.Runtime{}, workspace.OpenAIResponse{}, notFound("response_not_found", "Response not found.")
	}
	_, runtime, _, err := s.resolveConversation(ctx, "conv_"+workspaceName)
	if err != nil {
		return apitypes.Workspace{}, workspace.Runtime{}, workspace.OpenAIResponse{}, notFound("response_not_found", "Response not found.")
	}
	item, getErr := s.Workspaces.GetConversationWorkspace(ctx, workspaceName)
	if getErr != nil {
		return apitypes.Workspace{}, workspace.Runtime{}, workspace.OpenAIResponse{}, notFound("response_not_found", "Response not found.")
	}
	record, getErr := runtime.OpenAI.Response(ctx, id)
	if getErr != nil || record.WorkspaceID != item.Id {
		return apitypes.Workspace{}, workspace.Runtime{}, workspace.OpenAIResponse{}, notFound("response_not_found", "Response not found.")
	}
	if record.Status == "in_progress" && !s.responseRuntime().isActive(item.Id, record.ID) {
		now := s.now()
		record.Status = "failed"
		record.CompletedAt = &now
		record.ErrorCode = "server_restarted"
		record.ErrorMessage = "The Server restarted before the Response completed."
		if putErr := runtime.OpenAI.PutResponse(ctx, record); putErr != nil {
			return apitypes.Workspace{}, workspace.Runtime{}, workspace.OpenAIResponse{}, internal(putErr)
		}
	}
	return item, runtime, record, nil
}

func (s *Server) responseJSON(ctx context.Context, runtime workspace.Runtime, record workspace.OpenAIResponse) (backend.Response, error) {
	object, err := s.responseObject(ctx, runtime, record)
	if err != nil {
		return backend.Response{}, err
	}
	return jsonResponse(object)
}

func (s *Server) responseObject(ctx context.Context, runtime workspace.Runtime, record workspace.OpenAIResponse) (map[string]any, error) {
	output := make([]any, 0, len(record.OutputItemIDs))
	if record.Status == "completed" {
		for _, id := range record.OutputItemIDs {
			item, err := runtime.OpenAI.Item(ctx, id)
			if err != nil {
				return nil, internal(err)
			}
			object, err := s.itemObject(ctx, runtime, item)
			if err != nil {
				return nil, err
			}
			output = append(output, object)
		}
	}
	var responseError any
	if record.ErrorCode != "" {
		responseError = map[string]any{"code": record.ErrorCode, "message": record.ErrorMessage}
	}
	object := map[string]any{
		"id": record.ID, "object": "response", "created_at": record.CreatedAt.Unix(), "status": record.Status,
		"completed_at": nil, "error": responseError, "incomplete_details": nil, "instructions": nil,
		"model": record.Model, "output": output, "parallel_tool_calls": true, "metadata": cloneStringMap(record.Metadata),
		"tool_choice": "auto", "tools": []any{}, "temperature": 1.0, "top_p": 1.0,
		"previous_response_id": nil, "reasoning": map[string]any{"effort": nil, "summary": nil},
		"store": true, "text": map[string]any{"format": map[string]any{"type": "text"}}, "truncation": "disabled",
		"conversation": map[string]any{"id": record.ConversationID},
	}
	if record.CompletedAt != nil {
		object["completed_at"] = record.CompletedAt.Unix()
	}
	return object, nil
}

func decodeConversationID(raw json.RawMessage) (string, error) {
	var id string
	if json.Unmarshal(raw, &id) == nil && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id), nil
	}
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil && strings.TrimSpace(object.ID) != "" {
		return strings.TrimSpace(object.ID), nil
	}
	return "", errors.New("conversation is required")
}

func decodeResponseText(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		text = strings.TrimSpace(text)
		if text != "" {
			return text, nil
		}
		return "", errors.New("input text must not be empty")
	}
	var items []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &items) != nil || len(items) != 1 || items[0].Type != "message" || items[0].Role != "user" || len(items[0].Content) != 1 || items[0].Content[0].Type != "input_text" {
		return "", errors.New("exactly one user message with one input_text part is required")
	}
	text = strings.TrimSpace(items[0].Content[0].Text)
	if text == "" {
		return "", errors.New("input text must not be empty")
	}
	return text, nil
}

func responseIDForWorkspace(name string) (string, error) {
	random, err := randomOpenAIName("r")
	if err != nil {
		return "", err
	}
	return "resp_" + base64.RawURLEncoding.EncodeToString([]byte(name)) + "." + strings.TrimPrefix(random, "r-"), nil
}

func workspaceNameFromResponseID(id string) (string, bool) {
	if !strings.HasPrefix(id, "resp_") {
		return "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(id, "resp_"), ".", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	name := string(decoded)
	return name, err == nil && isGeneratedOpenAIID(name, "oai-") && isGeneratedOpenAIID("r-"+parts[1], "r-")
}

func newItemID(prefix string) string {
	value, err := randomOpenAIName(prefix)
	if err != nil {
		return fmt.Sprintf("%s_%024x", prefix, uint64(time.Now().UnixNano()))
	}
	return strings.Replace(value, "-", "_", 1)
}

func (s *Server) responseRuntime() *ResponseRuntime {
	if s.Responses != nil {
		return s.Responses
	}
	return defaultResponseRuntime
}

var defaultResponseRuntime = NewResponseRuntime()

type responseEventStream struct {
	events chan backend.Event
	cancel context.CancelFunc
	once   sync.Once
}

func newResponseEventStream(parent context.Context, job *responseJob) backend.Stream {
	ctx, cancel := context.WithCancel(parent)
	stream := &responseEventStream{events: make(chan backend.Event, 8), cancel: cancel}
	go stream.produce(ctx, job)
	return stream
}

func (s *responseEventStream) Events() <-chan backend.Event { return s.events }
func (s *responseEventStream) Close() error {
	s.once.Do(s.cancel)
	return nil
}

func (s *responseEventStream) produce(ctx context.Context, job *responseJob) {
	defer close(s.events)
	sequence := 0
	send := func(eventType string, value map[string]any) bool {
		value["type"] = eventType
		value["sequence_number"] = sequence
		sequence++
		data, _ := json.Marshal(value)
		select {
		case s.events <- backend.Event{Type: eventType, Data: data}:
			return true
		case <-ctx.Done():
			return false
		}
	}
	initial, _ := job.server.responseObject(ctx, job.runtime, job.record)
	if !send("response.created", map[string]any{"response": initial}) ||
		!send("response.in_progress", map[string]any{"response": initial}) {
		job.active.cancel()
		_, _ = job.run(ctx, nil)
		return
	}
	item := map[string]any{"id": job.outputItemID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
	if !send("response.output_item.added", map[string]any{"output_index": 0, "item": item}) {
		job.active.cancel()
		_, _ = job.run(ctx, nil)
		return
	}
	part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}
	if !send("response.content_part.added", map[string]any{"item_id": job.outputItemID, "output_index": 0, "content_index": 0, "part": part}) {
		job.active.cancel()
		_, _ = job.run(ctx, nil)
		return
	}
	var output strings.Builder
	record, runErr := job.run(ctx, func(delta string) error {
		output.WriteString(delta)
		if !send("response.output_text.delta", map[string]any{
			"response_id": job.record.ID, "item_id": job.outputItemID, "output_index": 0, "content_index": 0,
			"delta": delta, "logprobs": []any{},
		}) {
			return context.Canceled
		}
		return nil
	})
	if runErr != nil {
		return
	}
	if record.Status != "completed" {
		terminal, _ := job.server.responseObject(context.WithoutCancel(ctx), job.runtime, record)
		typeName := "response.failed"
		if record.Status == "cancelled" {
			typeName = "response.incomplete"
		}
		send(typeName, map[string]any{"response": terminal})
		return
	}
	for outputIndex, itemID := range record.OutputItemIDs {
		text := ""
		if outputIndex == 0 {
			text = output.String()
		}
		storedItem, itemErr := job.runtime.OpenAI.Item(context.WithoutCancel(ctx), itemID)
		if itemErr == nil {
			if storedHistory, historyErr := job.runtime.History.Get(context.WithoutCancel(ctx), storedItem.HistoryID); historyErr == nil {
				text = storedHistory.Text
			}
		}
		if outputIndex > 0 {
			item = map[string]any{"id": itemID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
			if !send("response.output_item.added", map[string]any{"output_index": outputIndex, "item": item}) {
				return
			}
			part = map[string]any{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}
			if !send("response.content_part.added", map[string]any{"item_id": itemID, "output_index": outputIndex, "content_index": 0, "part": part}) ||
				!send("response.output_text.delta", map[string]any{"response_id": job.record.ID, "item_id": itemID, "output_index": outputIndex, "content_index": 0, "delta": text, "logprobs": []any{}}) {
				return
			}
		}
		part = map[string]any{"type": "output_text", "text": text, "annotations": []any{}, "logprobs": []any{}}
		item = map[string]any{"id": itemID, "type": "message", "role": "assistant", "status": "completed", "content": []any{part}}
		if !send("response.output_text.done", map[string]any{"item_id": itemID, "output_index": outputIndex, "content_index": 0, "text": text, "logprobs": []any{}}) ||
			!send("response.content_part.done", map[string]any{"item_id": itemID, "output_index": outputIndex, "content_index": 0, "part": part}) ||
			!send("response.output_item.done", map[string]any{"output_index": outputIndex, "item": item}) {
			return
		}
	}
	terminal, _ := job.server.responseObject(ctx, job.runtime, record)
	send("response.completed", map[string]any{"response": terminal})
}
