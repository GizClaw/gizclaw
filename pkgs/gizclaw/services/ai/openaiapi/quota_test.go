package openaiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerquota"
	"github.com/idy/ai-server-shell/backend"
	shellopenai "github.com/idy/ai-server-shell/openai"
)

func quotaHTTPHandler(t *testing.T, server *Server) http.Handler {
	t.Helper()
	dispatch := backend.HandlerFunc(func(ctx context.Context, request backend.Request) (backend.Response, error) {
		request.Metadata.CallerID = server.Caller.String()
		return server.Handle(ctx, request)
	})
	services, err := backend.NewServices(backend.WithChat(dispatch), backend.WithConversations(dispatch), backend.WithResponses(dispatch))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := shellopenai.NewHandler(services, shellopenai.WithAuthenticator(shellopenai.AuthenticatorFunc(func(context.Context, shellopenai.Credential) (shellopenai.Principal, error) {
		return shellopenai.Principal{ID: server.Caller.String()}, nil
	})))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func quotaRequest(handler http.Handler, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer test")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestQuotaChatHTTPAndSSEContract(t *testing.T) {
	for _, test := range []struct {
		cause               error
		status              int
		code, kind, message string
	}{
		{peerquota.ErrDenied, 403, "quota_exhausted", "permission_error", "Quota exhausted."},
		{peerquota.ErrUnavailable, 503, "quota_unavailable", "service_unavailable_error", "Quota unavailable."},
		{peerquota.ErrClosed, 503, "quota_unavailable", "service_unavailable_error", "Quota unavailable."},
	} {
		t.Run(test.code+test.cause.Error(), func(t *testing.T) {
			cause := fmt.Errorf("secret provider URL: %w", fmt.Errorf("%w: %w", peergenx.ErrDenied, test.cause))
			server := &Server{Caller: mustKey(t).Public, Generator: generatorFunc(func(context.Context, string, genx.ModelContext) (genx.Stream, error) { return nil, cause })}
			handler := quotaHTTPHandler(t, server)
			for _, streaming := range []bool{false, true} {
				response := quotaRequest(handler, "/v1/chat/completions", fmt.Sprintf(`{"model":"chat","messages":[{"role":"user","content":"hello"}],"stream":%t}`, streaming))
				var body struct {
					Error struct{ Code, Type, Message string }
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if response.Code != test.status || body.Error.Code != test.code || body.Error.Type != test.kind || body.Error.Message != test.message {
					t.Fatalf("response = %d %s", response.Code, response.Body)
				}
			}
			server.Generator = generatorFunc(func(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
				builder := genx.NewStreamBuilder(modelContext, 2)
				_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial")})
				_ = builder.Abort(cause)
				return builder.Stream(), nil
			})
			response := quotaRequest(handler, "/v1/chat/completions", `{"model":"chat","messages":[{"role":"user","content":"hello"}],"stream":true}`)
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) || !strings.Contains(response.Body.String(), `"message":"`+test.message+`"`) || !strings.Contains(response.Body.String(), `"type":"`+test.kind+`"`) || strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("SSE = %d %s", response.Code, response.Body)
			}
			server.Generator = generatorFunc(func(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
				return quotaControlStream(modelContext, cause), nil
			})
			for _, streaming := range []bool{false, true} {
				response := quotaRequest(handler, "/v1/chat/completions", fmt.Sprintf(`{"model":"chat","messages":[{"role":"user","content":"hello"}],"stream":%t}`, streaming))
				status := test.status
				if streaming {
					status = 200
				}
				if response.Code != status || !strings.Contains(response.Body.String(), test.code) || !strings.Contains(response.Body.String(), test.message) || strings.Contains(response.Body.String(), "secret") {
					t.Fatalf("control EOS response = %d %s", response.Code, response.Body)
				}
			}
			for _, stream := range []backend.Stream{
				newSpeechEventStream(t.Context(), quotaControlStream((&genx.ModelContextBuilder{}).Build(), cause), "audio/pcm"),
				newTranscriptionEventStream(t.Context(), quotaControlStream((&genx.ModelContextBuilder{}).Build(), cause)),
			} {
				var events strings.Builder
				for event := range stream.Events() {
					events.Write(event.Data)
				}
				_ = stream.Close()
				if !strings.Contains(events.String(), test.code) || !strings.Contains(events.String(), test.message) {
					t.Fatalf("control EOS SSE = %s", events.String())
				}
			}

		})
	}
	if quotaBackendError(fmt.Errorf("resource: %w", peergenx.ErrDenied)) != nil || quotaBackendError(errors.New("QUOTA_EXHAUSTED")) != nil {
		t.Fatal("mislabeled non-quota error")
	}
}

type quotaWorkspaceExecutor struct{ cause error }

func (e quotaWorkspaceExecutor) ExecuteWorkspaceText(context.Context, apitypes.Workspace, string, func(string) error) ([]workspace.HistoryEntry, error) {
	return nil, e.cause
}

func TestQuotaResponsesHTTPStreamingAndPersistence(t *testing.T) {
	for _, cause := range []error{peerquota.ErrDenied, peerquota.ErrUnavailable} {
		t.Run(cause.Error(), func(t *testing.T) {
			fake := &fakeConversationWorkspaces{runtimeStore: testOpenAIRuntimeStore(t, testOpenAIObjectStore(t)), items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{}}
			server := &Server{Caller: mustKey(t).Public, Workspaces: fake, Executor: quotaWorkspaceExecutor{cause: fmt.Errorf("private runtime: %w", cause)}, Responses: NewResponseRuntime()}
			handler := quotaHTTPHandler(t, server)
			conversation := quotaRequest(handler, "/v1/conversations", `{"metadata":{"workflow_name":"story"}}`)
			id := jsonString(t, conversation.Body.Bytes(), "id")
			want := quotaBackendError(cause)
			response := quotaRequest(handler, "/v1/responses", fmt.Sprintf(`{"conversation":%q,"input":"hello"}`, id))
			status := 403
			if want.Kind == backend.ErrorUnavailable {
				status = 503
			}
			if response.Code != status || !strings.Contains(response.Body.String(), want.Code) || !strings.Contains(response.Body.String(), want.Message) {
				t.Fatalf("Response = %d %s", response.Code, response.Body)
			}
			response = quotaRequest(handler, "/v1/responses", fmt.Sprintf(`{"conversation":%q,"input":"hello","stream":true}`, id))
			if response.Code != 200 || !strings.Contains(response.Body.String(), "response.failed") || !strings.Contains(response.Body.String(), want.Code) || !strings.Contains(response.Body.String(), want.Message) || strings.Contains(response.Body.String(), "private") {
				t.Fatalf("Response SSE = %d %s", response.Code, response.Body)
			}
		})
	}
}

func quotaControlStream(modelContext genx.ModelContext, cause error) genx.Stream {
	builder := genx.NewStreamBuilder(modelContext, 2)
	chunk := genx.NewTextEndOfStream()
	genx.SetStreamError(chunk.Ctrl, cause)
	_ = builder.Add(chunk)
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}
