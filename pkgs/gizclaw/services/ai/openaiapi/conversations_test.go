package openaiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/idy/ai-server-shell/backend"
	shellopenai "github.com/idy/ai-server-shell/openai"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestConversationCreateReturnsNotFoundForUnknownWorkflowAlias(t *testing.T) {
	key := mustKey(t)
	fake := &fakeConversationWorkspaces{
		createErr: &workspace.PeerWorkspaceCreateError{
			Kind: workspace.PeerWorkspaceCreateNotFound,
			Err:  errors.New("workflow not found"),
		},
	}
	server := &Server{Caller: key.Public, Workspaces: fake}
	request := requestFor(key.Public, backend.CapabilityConversations, "createConversation", json.RawMessage(`{
		"metadata":{"collection":"assistants","workflow_name":"missing"},"items":[]
	}`))
	_, err := server.Handle(t.Context(), request)
	var backendErr *backend.Error
	if !errors.As(err, &backendErr) || backendErr.Kind != backend.ErrorNotFound || backendErr.Code != "workflow_not_found" {
		t.Fatalf("create Conversation error = %#v, want workflow_not_found", err)
	}
}

func TestConversationsResponsesThreeTurnsAndImmutableInputSnapshot(t *testing.T) {
	key := mustKey(t)
	objects := testOpenAIObjectStore(t)
	fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, objects), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
	server := &Server{Caller: key.Public, Workspaces: fake, Executor: fake, Responses: NewResponseRuntime()}
	created := handleJSON(t, server, key.Public, backend.CapabilityConversations, "createConversation", `{
		"metadata":{"collection":"assistants","workflow_name":"story","purpose":"test"},"items":[]
	}`, nil)
	conversationID := jsonString(t, created, "id")
	if conversationID == "" || jsonStringMap(t, created, "metadata")["purpose"] != "test" {
		t.Fatalf("Conversation = %s", created)
	}
	second := handleJSON(t, server, key.Public, backend.CapabilityConversations, "createConversation", `{
		"metadata":{"collection":"assistants","workflow_name":"story","purpose":"test"},"items":[]
	}`, nil)
	if secondID := jsonString(t, second, "id"); secondID == "" || secondID == conversationID {
		t.Fatalf("identical Conversation creates returned %q and %q", conversationID, secondID)
	}

	var responseIDs []string
	for turn := 1; turn <= 3; turn++ {
		body := fmt.Sprintf(`{"conversation":%q,"input":%q,"model":"story"}`, conversationID, fmt.Sprintf("turn-%d", turn))
		response := handleJSON(t, server, key.Public, backend.CapabilityResponses, "createResponse", body, nil)
		if jsonString(t, response, "status") != "completed" {
			t.Fatalf("turn %d Response = %s", turn, response)
		}
		responseIDs = append(responseIDs, jsonString(t, response, "id"))
	}

	items := handleJSON(t, server, key.Public, backend.CapabilityConversations, "listConversationItems", "", map[string]string{"conversation_id": conversationID})
	var list struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(items, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 6 {
		t.Fatalf("Conversation items = %d, want 6: %s", len(list.Data), items)
	}
	inputItems := handleJSON(t, server, key.Public, backend.CapabilityResponses, "listInputItems", "", map[string]string{"response_id": responseIDs[0]})
	if err := json.Unmarshal(inputItems, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 {
		t.Fatalf("first Response input items = %d, want immutable 1: %s", len(list.Data), inputItems)
	}
	lastInputItems := handleJSON(t, server, key.Public, backend.CapabilityResponses, "listInputItems", "", map[string]string{"response_id": responseIDs[2]})
	if err := json.Unmarshal(lastInputItems, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 5 {
		t.Fatalf("third Response input items = %d, want 5: %s", len(list.Data), lastInputItems)
	}
	reloaded := &Server{Caller: key.Public, Workspaces: fake, Executor: fake, Responses: NewResponseRuntime()}
	if got := handleJSON(t, reloaded, key.Public, backend.CapabilityConversations, "getConversation", "", map[string]string{"conversation_id": conversationID}); jsonString(t, got, "id") != conversationID {
		t.Fatalf("reloaded Conversation = %s", got)
	}
	if got := handleJSON(t, reloaded, key.Public, backend.CapabilityResponses, "getResponse", "", map[string]string{"response_id": responseIDs[0]}); jsonString(t, got, "status") != "completed" {
		t.Fatalf("reloaded Response = %s", got)
	}
	if fake.executionCount() != 3 {
		t.Fatalf("read-only reload paths executed Agent %d times, want 3 prior creates only", fake.executionCount())
	}
}

func TestConversationResponseObjectsPassFrozenShellValidation(t *testing.T) {
	key := mustKey(t)
	objects := testOpenAIObjectStore(t)
	fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, objects), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
	server := &Server{Caller: key.Public, Workspaces: fake, Executor: fake, Responses: NewResponseRuntime()}
	dispatch := backend.HandlerFunc(func(ctx context.Context, request backend.Request) (backend.Response, error) {
		request.Metadata.CallerID = key.Public.String()
		return server.Handle(ctx, request)
	})
	services, err := backend.NewServices(backend.WithConversations(dispatch), backend.WithResponses(dispatch))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := shellopenai.NewHandler(services, shellopenai.WithAuthenticator(shellopenai.AuthenticatorFunc(
		func(context.Context, shellopenai.Credential) (shellopenai.Principal, error) {
			return shellopenai.Principal{ID: key.Public.String()}, nil
		},
	)))
	if err != nil {
		t.Fatal(err)
	}
	conversationRecorder := httptest.NewRecorder()
	conversationRequest := httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(`{"metadata":{"collection":"assistants","workflow_name":"story"}}`))
	conversationRequest.Header.Set("Content-Type", "application/json")
	conversationRequest.Header.Set("Authorization", "Bearer test")
	handler.ServeHTTP(conversationRecorder, conversationRequest)
	if conversationRecorder.Code != http.StatusOK {
		t.Fatalf("Conversation status = %d body=%s", conversationRecorder.Code, conversationRecorder.Body.String())
	}
	conversationID := jsonString(t, conversationRecorder.Body.Bytes(), "id")
	responseRecorder := httptest.NewRecorder()
	responseRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"conversation":%q,"input":"hello"}`, conversationID)))
	responseRequest.Header.Set("Content-Type", "application/json")
	responseRequest.Header.Set("Authorization", "Bearer test")
	handler.ServeHTTP(responseRecorder, responseRequest)
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("Response status = %d body=%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	if jsonString(t, responseRecorder.Body.Bytes(), "status") != "completed" {
		t.Fatalf("Response body = %s", responseRecorder.Body.String())
	}
}

func TestResponsePreservesMultipleWorkspaceOutputsInHistoryOrder(t *testing.T) {
	key := mustKey(t)
	objects := testOpenAIObjectStore(t)
	fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, objects), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
	executor := &multipleOutputExecutor{workspaces: fake}
	server := &Server{Caller: key.Public, Workspaces: fake, Executor: executor, Responses: NewResponseRuntime()}
	created := handleJSON(t, server, key.Public, backend.CapabilityConversations, "createConversation", `{"metadata":{"collection":"assistants","workflow_name":"story"}}`, nil)
	conversationID := jsonString(t, created, "id")
	response := handleJSON(t, server, key.Public, backend.CapabilityResponses, "createResponse", fmt.Sprintf(`{"conversation":%q,"input":"route"}`, conversationID), nil)
	var object struct {
		Status string `json:"status"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(response, &object); err != nil {
		t.Fatal(err)
	}
	if object.Status != "completed" || len(object.Output) != 2 || object.Output[0].Content[0].Text != "first route" || object.Output[1].Content[0].Text != "second route" {
		t.Fatalf("multi-output Response = %s", response)
	}

	request := requestFor(key.Public, backend.CapabilityResponses, "createResponse", json.RawMessage(fmt.Sprintf(`{"conversation":%q,"input":"stream routes","stream":true}`, conversationID)))
	streamed, err := server.Handle(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	var doneIndexes []int
	for event := range streamed.Stream.Events() {
		if event.Type != "response.output_item.done" {
			continue
		}
		var data struct {
			OutputIndex int `json:"output_index"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatal(err)
		}
		doneIndexes = append(doneIndexes, data.OutputIndex)
	}
	if fmt.Sprint(doneIndexes) != "[0 1]" {
		t.Fatalf("stream output indexes = %v, want [0 1]", doneIndexes)
	}
}

func TestBackgroundCancelAndStreamingAbortReleaseConversation(t *testing.T) {
	key := mustKey(t)
	objects := testOpenAIObjectStore(t)
	fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, objects), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
	blocker := &blockingWorkspaceExecutor{started: make(chan struct{}, 1)}
	server := &Server{Caller: key.Public, Workspaces: fake, Executor: blocker, Responses: NewResponseRuntime()}
	created := handleJSON(t, server, key.Public, backend.CapabilityConversations, "createConversation", `{"metadata":{"collection":"assistants","workflow_name":"story"}}`, nil)
	conversationID := jsonString(t, created, "id")
	background := handleJSON(t, server, key.Public, backend.CapabilityResponses, "createResponse", fmt.Sprintf(`{"conversation":%q,"input":"cancel me","background":true}`, conversationID), nil)
	responseID := jsonString(t, background, "id")
	select {
	case <-blocker.started:
	case <-time.After(time.Second):
		t.Fatal("background executor did not start")
	}
	cancelled := handleJSON(t, server, key.Public, backend.CapabilityResponses, "cancelResponse", "", map[string]string{"response_id": responseID})
	if jsonString(t, cancelled, "status") != "cancelled" {
		t.Fatalf("cancelled Response = %s", cancelled)
	}
	server.Executor = fake
	recovered := handleJSON(t, server, key.Public, backend.CapabilityResponses, "createResponse", fmt.Sprintf(`{"conversation":%q,"input":"recover"}`, conversationID), nil)
	if jsonString(t, recovered, "status") != "completed" {
		t.Fatalf("recovery Response = %s", recovered)
	}

	streamBlocker := &blockingWorkspaceExecutor{started: make(chan struct{}, 1), delta: "partial"}
	server.Executor = streamBlocker
	request := requestFor(key.Public, backend.CapabilityResponses, "createResponse", json.RawMessage(fmt.Sprintf(`{"conversation":%q,"input":"abort","stream":true}`, conversationID)))
	response, err := server.Handle(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	for event := range response.Stream.Events() {
		if event.Type == "response.output_text.delta" {
			break
		}
	}
	if err := response.Stream.Close(); err != nil {
		t.Fatal(err)
	}
	server.Executor = fake
	deadline := time.Now().Add(time.Second)
	for {
		request = requestFor(key.Public, backend.CapabilityResponses, "createResponse", json.RawMessage(fmt.Sprintf(`{"conversation":%q,"input":"after abort"}`, conversationID)))
		response, err = server.Handle(t.Context(), request)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Workspace slot was not released: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if jsonString(t, response.JSON, "status") != "completed" {
		t.Fatalf("post-abort Response = %s", response.JSON)
	}
}

func TestResponseIDRoundTripsWorkspaceLocator(t *testing.T) {
	for _, name := range []string{"oai-0123456789abcdef01234567", "oai-ffffffffffffffffffffffff"} {
		id, err := responseIDForWorkspace(name)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := workspaceNameFromResponseID(id); !ok || got != name {
			t.Fatalf("workspaceNameFromResponseID(%q) = %q, %v", id, got, ok)
		}
	}
	for _, id := range []string{"resp_../conversation", "resp_b2FpLWRlbW8.deadbeef", "resp_b2FpLWZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZmZm.deadbeef"} {
		if name, ok := workspaceNameFromResponseID(id); ok {
			t.Fatalf("workspaceNameFromResponseID(%q) = %q, true", id, name)
		}
	}
}

func TestResponseRetrieveRecoversStaleInProgressRecord(t *testing.T) {
	key := mustKey(t)
	objects := testOpenAIObjectStore(t)
	fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, objects), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
	server := &Server{Caller: key.Public, Workspaces: fake, Executor: fake, Responses: NewResponseRuntime()}
	created := handleJSON(t, server, key.Public, backend.CapabilityConversations, "createConversation", `{"metadata":{"collection":"assistants","workflow_name":"story"}}`, nil)
	conversationID := jsonString(t, created, "id")
	workspaceName := strings.TrimPrefix(conversationID, "conv_")
	item, err := fake.GetConversationWorkspace(t.Context(), workspaceName)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := fake.GetConversationRuntime(t.Context(), item.Id)
	if err != nil {
		t.Fatal(err)
	}
	responseID, err := responseIDForWorkspace(workspaceName)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.OpenAI.PutResponse(t.Context(), workspace.OpenAIResponse{
		ID: responseID, ConversationID: conversationID, WorkspaceID: item.Id,
		Model: "story", Status: "in_progress", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	response := handleJSON(t, server, key.Public, backend.CapabilityResponses, "getResponse", "", map[string]string{"response_id": responseID})
	if jsonString(t, response, "status") != "failed" {
		t.Fatalf("recovered Response = %s", response)
	}
	var recovered struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.Error.Code != "server_restarted" {
		t.Fatalf("recovered Response error = %s", response)
	}
}

func TestUnsupportedResponseInputsFailBeforeHistoryMutation(t *testing.T) {
	key := mustKey(t)
	objects := testOpenAIObjectStore(t)
	fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, objects), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
	server := &Server{Caller: key.Public, Workspaces: fake, Executor: fake, Responses: NewResponseRuntime()}
	created := handleJSON(t, server, key.Public, backend.CapabilityConversations, "createConversation", `{"metadata":{"collection":"assistants","workflow_name":"story"}}`, nil)
	conversationID := jsonString(t, created, "id")
	workspaceName := strings.TrimPrefix(conversationID, "conv_")
	item, err := fake.GetConversationWorkspace(t.Context(), workspaceName)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := fake.GetConversationRuntime(t.Context(), item.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		fmt.Sprintf(`{"conversation":%q,"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`, conversationID),
		fmt.Sprintf(`{"conversation":%q,"input":"hello","model":"another"}`, conversationID),
		fmt.Sprintf(`{"conversation":%q,"input":"hello","store":false}`, conversationID),
		fmt.Sprintf(`{"conversation":%q,"input":"hello","temperature":0.2}`, conversationID),
	} {
		request := requestFor(key.Public, backend.CapabilityResponses, "createResponse", json.RawMessage(body))
		if _, err := server.Handle(t.Context(), request); err == nil {
			t.Fatalf("unsupported request succeeded: %s", body)
		}
	}
	items, err := runtime.OpenAI.Items(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unsupported requests persisted %d items", len(items))
	}
}

type fakeConversationWorkspaces struct {
	mu           sync.Mutex
	createErr    error
	runtimeStore workspace.ObjectRuntimeStore
	items        map[string]apitypes.Workspace
	runtimes     map[string]workspace.Runtime
	sequence     int
	executions   int
}

type blockingWorkspaceExecutor struct {
	started chan struct{}
	delta   string
}

type multipleOutputExecutor struct {
	workspaces *fakeConversationWorkspaces
}

func (e *multipleOutputExecutor) ExecuteWorkspaceText(ctx context.Context, item apitypes.Workspace, _ string, _ func(string) error) ([]workspace.HistoryEntry, error) {
	runtime, err := e.workspaces.GetConversationRuntime(ctx, item.Id)
	if err != nil {
		return nil, err
	}
	entries := make([]workspace.HistoryEntry, 0, 2)
	for _, text := range []string{"first route", "second route"} {
		entry, appendErr := runtime.History.Append(ctx, workspace.AppendHistoryRequest{Type: "agent", Origin: workspace.HistoryOriginAgentHost, Name: "assistant", Text: text})
		if appendErr != nil {
			return nil, appendErr
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (e *blockingWorkspaceExecutor) ExecuteWorkspaceText(ctx context.Context, _ apitypes.Workspace, _ string, delta func(string) error) ([]workspace.HistoryEntry, error) {
	select {
	case e.started <- struct{}{}:
	default:
	}
	if e.delta != "" && delta != nil {
		if err := delta(e.delta); err != nil {
			return nil, err
		}
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeConversationWorkspaces) CreateConversationWorkspace(ctx context.Context, request ConversationWorkspaceRequest) (apitypes.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return apitypes.Workspace{}, f.createErr
	}
	f.sequence++
	id := fmt.Sprintf("workspace-%d", f.sequence)
	runtime, err := f.runtimeStore.PrepareWorkspace(ctx, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if err := request.Initialize(ctx, runtime); err != nil {
		return apitypes.Workspace{}, err
	}
	system := false
	labels := map[string]string{"collection": request.Collection, "openai.conversation": "true"}
	item := apitypes.Workspace{Id: id, Name: request.Name, WorkflowId: request.WorkflowName, Labels: &labels, System: &system, CreatedAt: time.Now(), UpdatedAt: time.Now(), LastActiveAt: time.Now()}
	f.items[item.Name] = item
	f.runtimes[id] = runtime
	return item, nil
}

func (f *fakeConversationWorkspaces) GetConversationWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.items[name]
	if !ok {
		return apitypes.Workspace{}, os.ErrNotExist
	}
	return item, nil
}

func (f *fakeConversationWorkspaces) GetConversationRuntime(_ context.Context, id string) (workspace.Runtime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	runtime, ok := f.runtimes[id]
	if !ok {
		return workspace.Runtime{}, os.ErrNotExist
	}
	return runtime, nil
}

func (f *fakeConversationWorkspaces) AppendConversationHistory(ctx context.Context, id string, request workspace.AppendHistoryRequest) (workspace.HistoryEntry, error) {
	runtime, err := f.GetConversationRuntime(ctx, id)
	if err != nil {
		return workspace.HistoryEntry{}, err
	}
	return runtime.History.Append(ctx, request)
}

func (f *fakeConversationWorkspaces) ExecuteWorkspaceText(ctx context.Context, item apitypes.Workspace, input string, delta func(string) error) ([]workspace.HistoryEntry, error) {
	f.mu.Lock()
	f.executions++
	f.mu.Unlock()
	text := "reply:" + input
	if delta != nil {
		if err := delta(text); err != nil {
			return nil, err
		}
	}
	runtime, err := f.GetConversationRuntime(ctx, item.Id)
	if err != nil {
		return nil, err
	}
	entry, err := runtime.History.Append(ctx, workspace.AppendHistoryRequest{Type: "agent", Origin: workspace.HistoryOriginAgentHost, Name: "assistant", Text: text})
	if err != nil {
		return nil, err
	}
	return []workspace.HistoryEntry{entry}, nil
}

func (f *fakeConversationWorkspaces) executionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.executions
}

func testOpenAIObjectStore(t *testing.T) *objectstore.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	store, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testOpenAIRuntimeStore(t *testing.T, objects objectstore.ObjectStore) workspace.ObjectRuntimeStore {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	history, err := logstore.NewSQLStoreWithDB(db, "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	return workspace.NewObjectRuntimeStore(objects, history, objects)
}

func handleJSON(t *testing.T, server *Server, caller [32]byte, capability backend.Capability, operation, body string, parameters map[string]string) json.RawMessage {
	t.Helper()
	rawParameters := make(map[string]json.RawMessage, len(parameters))
	for key, value := range parameters {
		rawParameters[key], _ = json.Marshal(value)
	}
	request := requestFor(caller, capability, operation, json.RawMessage(body))
	request.Parameters = rawParameters
	response, err := server.Handle(t.Context(), request)
	if err != nil {
		if backendErr, ok := err.(*backend.Error); ok && backendErr.Cause != nil {
			t.Fatalf("%s: %v: %v", operation, err, backendErr.Cause)
		}
		t.Fatalf("%s: %v (%#v)", operation, err, err)
	}
	return response.JSON
}

func jsonString(t *testing.T, raw json.RawMessage, key string) string {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	result, _ := value[key].(string)
	return result
}

func jsonStringMap(t *testing.T, raw json.RawMessage, key string) map[string]string {
	t.Helper()
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(value[key], &result); err != nil {
		t.Fatal(err)
	}
	return result
}
