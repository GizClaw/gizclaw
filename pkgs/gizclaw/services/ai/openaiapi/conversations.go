package openaiapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"strconv"
	"strings"

	"github.com/idy/ai-server-shell/backend"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

type createConversationRequest struct {
	Metadata map[string]string `json:"metadata"`
	Items    json.RawMessage   `json:"items"`
}

func (s *Server) handleConversation(ctx context.Context, request backend.Request) (backend.Response, error) {
	switch request.Operation {
	case "createConversation":
		return s.createConversation(ctx, request)
	case "getConversation":
		return s.getConversation(ctx, request)
	case "listConversationItems":
		return s.listConversationItems(ctx, request)
	case "getConversationItem":
		return s.getConversationItem(ctx, request)
	default:
		return backend.Response{}, unsupportedOperation()
	}
}

func (s *Server) createConversation(ctx context.Context, request backend.Request) (backend.Response, error) {
	if s.Workspaces == nil {
		return backend.Response{}, unavailable("workspaces_unavailable", "The Workspace service is unavailable.", nil)
	}
	var body createConversationRequest
	if err := decodeJSONProjection(request.Input.JSON, &body, "metadata", "items"); err != nil {
		return backend.Response{}, err
	}
	if nonEmptyJSONArray(body.Items) {
		return backend.Response{}, invalid("unsupported_option", "items", "Initial Conversation items are not supported by GizClaw.")
	}
	collection := strings.TrimSpace(body.Metadata["collection"])
	workflowName := strings.TrimSpace(body.Metadata["workflow_name"])
	if collection == "" {
		return backend.Response{}, invalid("missing_required_parameter", "metadata.collection", "metadata.collection is required.")
	}
	if workflowName == "" {
		return backend.Response{}, invalid("missing_required_parameter", "metadata.workflow_name", "metadata.workflow_name is required.")
	}
	now := s.now()
	name, err := randomOpenAIName("oai")
	if err != nil {
		return backend.Response{}, internal(err)
	}
	metadata := cloneStringMap(body.Metadata)
	var conversation workspace.OpenAIConversation
	_, err = s.Workspaces.CreateConversationWorkspace(ctx, ConversationWorkspaceRequest{
		Name: name, Collection: collection, WorkflowName: workflowName, Metadata: metadata,
		Initialize: func(initCtx context.Context, runtime workspace.Runtime) error {
			if runtime.OpenAI == nil {
				return errors.New("OpenAI state store is unavailable")
			}
			conversation = workspace.OpenAIConversation{ID: "conv_" + name, Metadata: metadata, CreatedAt: now}
			return runtime.OpenAI.PutConversation(initCtx, conversation)
		},
	})
	if err != nil {
		return backend.Response{}, mapWorkspaceError(err)
	}
	return jsonResponse(conversationObject(conversation))
}

func (s *Server) getConversation(ctx context.Context, request backend.Request) (backend.Response, error) {
	_, _, conversation, err := s.resolveConversation(ctx, parameterString(request, "conversation_id"))
	if err != nil {
		return backend.Response{}, err
	}
	return jsonResponse(conversationObject(conversation))
}

func (s *Server) listConversationItems(ctx context.Context, request backend.Request) (backend.Response, error) {
	_, runtime, _, err := s.resolveConversation(ctx, parameterString(request, "conversation_id"))
	if err != nil {
		return backend.Response{}, err
	}
	items, err := runtime.OpenAI.Items(ctx)
	if err != nil {
		return backend.Response{}, internal(err)
	}
	page := paginateOpenAIItems(items, request)
	data := make([]any, 0, len(page.items))
	for _, item := range page.items {
		object, err := s.itemObject(ctx, runtime, item)
		if err != nil {
			return backend.Response{}, err
		}
		data = append(data, object)
	}
	return jsonResponse(listObject(data, page.hasMore))
}

func (s *Server) getConversationItem(ctx context.Context, request backend.Request) (backend.Response, error) {
	_, runtime, _, err := s.resolveConversation(ctx, parameterString(request, "conversation_id"))
	if err != nil {
		return backend.Response{}, err
	}
	itemID := parameterString(request, "item_id")
	if !isGeneratedOpenAIID(itemID, "msg_") {
		return backend.Response{}, notFound("conversation_item_not_found", "Conversation item not found.")
	}
	item, err := runtime.OpenAI.Item(ctx, itemID)
	if err != nil {
		return backend.Response{}, notFound("conversation_item_not_found", "Conversation item not found.")
	}
	object, err := s.itemObject(ctx, runtime, item)
	if err != nil {
		return backend.Response{}, err
	}
	return jsonResponse(object)
}

func (s *Server) resolveConversation(ctx context.Context, id string) (workspaceName string, runtime workspace.Runtime, conversation workspace.OpenAIConversation, backendErr error) {
	if s.Workspaces == nil || !strings.HasPrefix(id, "conv_") {
		return "", workspace.Runtime{}, workspace.OpenAIConversation{}, notFound("conversation_not_found", "Conversation not found.")
	}
	workspaceName = strings.TrimPrefix(id, "conv_")
	if !isGeneratedOpenAIID(workspaceName, "oai-") {
		return "", workspace.Runtime{}, workspace.OpenAIConversation{}, notFound("conversation_not_found", "Conversation not found.")
	}
	item, err := s.Workspaces.GetConversationWorkspace(ctx, workspaceName)
	if err != nil || item.System == nil || *item.System || !hasWorkspaceLabel(item, "openai.conversation", "true") {
		return "", workspace.Runtime{}, workspace.OpenAIConversation{}, notFound("conversation_not_found", "Conversation not found.")
	}
	runtime, err = s.Workspaces.GetConversationRuntime(ctx, item.Id)
	if err != nil || runtime.OpenAI == nil {
		return "", workspace.Runtime{}, workspace.OpenAIConversation{}, notFound("conversation_not_found", "Conversation not found.")
	}
	conversation, err = runtime.OpenAI.Conversation(ctx)
	if err != nil || conversation.ID != id {
		return "", workspace.Runtime{}, workspace.OpenAIConversation{}, notFound("conversation_not_found", "Conversation not found.")
	}
	return workspaceName, runtime, conversation, nil
}

func hasWorkspaceLabel(item apitypes.Workspace, key, value string) bool {
	return item.Labels != nil && (*item.Labels)[key] == value
}

func (s *Server) itemObject(ctx context.Context, runtime workspace.Runtime, item workspace.OpenAIItem) (any, error) {
	entry, err := runtime.History.Get(ctx, item.HistoryID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, notFound("conversation_item_not_found", "Conversation item not found.")
		}
		return nil, internal(err)
	}
	if item.Role == "assistant" {
		return map[string]any{
			"id": item.ID, "type": "message", "role": "assistant", "status": item.Status,
			"content": []any{map[string]any{"type": "output_text", "text": entry.Text, "annotations": []any{}, "logprobs": []any{}}},
		}, nil
	}
	return map[string]any{
		"id": item.ID, "type": "message", "role": "user", "status": item.Status,
		"content": []any{map[string]any{"type": "input_text", "text": entry.Text}},
	}, nil
}

func conversationObject(value workspace.OpenAIConversation) map[string]any {
	return map[string]any{
		"id": value.ID, "object": "conversation", "metadata": cloneStringMap(value.Metadata),
		"created_at": value.CreatedAt.Unix(),
	}
}

type itemPage struct {
	items   []workspace.OpenAIItem
	hasMore bool
}

func paginateOpenAIItems(items []workspace.OpenAIItem, request backend.Request) itemPage {
	order := parameterString(request, "order")
	if order == "desc" {
		for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
			items[left], items[right] = items[right], items[left]
		}
	}
	after := parameterString(request, "after")
	if after != "" {
		for index, item := range items {
			if item.ID == after {
				items = items[index+1:]
				break
			}
		}
	}
	limit := 20
	if raw := parameterString(request, "limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > len(items) {
		limit = len(items)
	}
	return itemPage{items: items[:limit], hasMore: limit < len(items)}
}

func listObject(data []any, hasMore bool) map[string]any {
	first, last := "", ""
	if len(data) != 0 {
		if object, ok := data[0].(map[string]any); ok {
			first, _ = object["id"].(string)
		}
		if object, ok := data[len(data)-1].(map[string]any); ok {
			last, _ = object["id"].(string)
		}
	}
	return map[string]any{"object": "list", "data": data, "has_more": hasMore, "first_id": first, "last_id": last}
}

func parameterString(request backend.Request, name string) string {
	raw := request.Parameters[name]
	if len(raw) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func nonEmptyJSONArray(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var items []json.RawMessage
	return json.Unmarshal(raw, &items) != nil || len(items) != 0
}

func randomOpenAIName(prefix string) (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("openaiapi: generate id: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(value[:]), nil
}

func isGeneratedOpenAIID(value, prefix string) bool {
	suffix := strings.TrimPrefix(value, prefix)
	if suffix == value || len(suffix) != 24 {
		return false
	}
	for _, char := range suffix {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	maps.Copy(result, source)
	return result
}

func mapWorkspaceError(err error) error {
	var createErr *workspace.PeerWorkspaceCreateError
	if !errors.As(err, &createErr) {
		return internal(err)
	}
	switch createErr.Kind {
	case workspace.PeerWorkspaceCreateInvalid:
		return invalid("invalid_workspace", "metadata", createErr.Error())
	case workspace.PeerWorkspaceCreateNotFound:
		return notFound("workflow_not_found", createErr.Error())
	case workspace.PeerWorkspaceCreateConflict:
		return &backend.Error{Kind: backend.ErrorConflict, Code: "workspace_already_exists", Message: createErr.Error()}
	default:
		return internal(err)
	}
}

func unsupportedOperation() error {
	return &backend.Error{Kind: backend.ErrorUnsupported, Code: "operation_not_supported", Message: "This operation is not supported by GizClaw."}
}

func notFound(code, message string) error {
	return &backend.Error{Kind: backend.ErrorNotFound, Code: code, Message: message}
}
