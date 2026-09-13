package dashscoperealtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

type dashScopeToolInvocation struct {
	name      string
	arguments string
}

type dashScopeTestToolInvoker struct {
	mu          sync.Mutex
	definitions []genx.ToolDefinition
	resolveErr  error
	invoke      func(context.Context, string, json.RawMessage) (json.RawMessage, error)
	calls       []dashScopeToolInvocation
}

func (i *dashScopeTestToolInvoker) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	if i.resolveErr != nil {
		return nil, i.resolveErr
	}
	return i.definitions, nil
}

func (i *dashScopeTestToolInvoker) InvokeTool(
	ctx context.Context,
	name string,
	arguments json.RawMessage,
) (json.RawMessage, error) {
	i.mu.Lock()
	i.calls = append(i.calls, dashScopeToolInvocation{name: name, arguments: string(arguments)})
	i.mu.Unlock()
	if i.invoke != nil {
		return i.invoke(ctx, name, arguments)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (i *dashScopeTestToolInvoker) invocations() []dashScopeToolInvocation {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]dashScopeToolInvocation(nil), i.calls...)
}

func dashScopeToolDefinitions() []genx.ToolDefinition {
	return []genx.ToolDefinition{{
		Name:        "get_weather",
		Description: "Look up weather.",
		Argument: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"city": {Type: "string"},
			},
			Required:             []string{"city"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
	}}
}

func TestResolveDashScopeTools(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
	tools, err := resolveDashScopeTools(t.Context(), invoker)
	if err != nil {
		t.Fatalf("resolveDashScopeTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Type != dashscope.ToolTypeFunction ||
		tool.Function.Name != "get_weather" ||
		tool.Function.Description != "Look up weather." ||
		tool.Function.Parameters == nil {
		t.Fatalf("tool = %#v", tool)
	}
	parameters := tool.Function.Parameters
	if parameters.Type != "object" ||
		parameters.Properties["city"] == nil ||
		parameters.Properties["city"].Type != "string" ||
		parameters.AdditionalProperties == nil ||
		*parameters.AdditionalProperties {
		t.Fatalf("tool parameters = %#v", parameters)
	}
}

func TestResolveDashScopeToolsRejectsUnsupportedSchema(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{definitions: []genx.ToolDefinition{{
		Name: "pattern_tool",
		Argument: &jsonschema.Schema{
			Type:    "string",
			Pattern: "^supported-by-genx-only$",
		},
	}}}
	_, err := resolveDashScopeTools(t.Context(), invoker)
	if err == nil || !strings.Contains(err.Error(), `convert tool "pattern_tool" schema`) {
		t.Fatalf("resolveDashScopeTools() error = %v, want unsupported schema error", err)
	}
}

func TestDashScopeRealtimeExplicitlyClearsToolsWithoutInvoker(t *testing.T) {
	session := &fakeDashScopeSession{}
	transformer := newTransformer(nil)
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), emptyDashScopeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer stream.Close()
	config, _, _ := session.toolState()
	if config == nil || config.Tools == nil || len(config.Tools) != 0 {
		t.Fatalf("session tools = %#v, want explicit empty slice", config)
	}
}

func TestDashScopeRealtimeRejectsFunctionCallWithoutInvoker(t *testing.T) {
	session := newDashScopeToolSession(dashScopeSingleToolEvents("call-1"))
	transformer := newTransformer(nil)
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_, err = collectDashScopeToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "ToolInvoker is not configured") {
		t.Fatalf("output error = %v, want missing ToolInvoker", err)
	}
	_, submitted, creates := session.toolState()
	if len(submitted) != 0 || creates != 0 {
		t.Fatalf("submitted/creates = %#v/%d, want none", submitted, creates)
	}
}

func TestDashScopeRealtimeInvokesToolsInOrderAndKeepsControlInternal(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{
		definitions: dashScopeToolDefinitions(),
		invoke: func(_ context.Context, _ string, arguments json.RawMessage) (json.RawMessage, error) {
			var value struct {
				City string `json:"city"`
			}
			if err := json.Unmarshal(arguments, &value); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"city": value.City})
		},
	}
	session := newDashScopeToolSession([]*dashscope.RealtimeEvent{
		{
			Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
			CallID:    "call-1",
			Name:      "get_weather",
			Arguments: `{"city":"深圳"}`,
		},
		{
			Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
			CallID:    "call-2",
			Name:      "get_weather",
			Arguments: `{"city":"上海"}`,
		},
		{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "response-1", Delta: "完成"},
		{Type: dashscope.EventTypeResponseTextDone, ResponseID: "response-1"},
	})
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks, err := collectDashScopeToolOutput(stream)
	if err != nil {
		t.Fatalf("collect output: %v", err)
	}
	config, submitted, creates := session.toolState()
	if config == nil || len(config.Tools) != 1 {
		t.Fatalf("provider tools = %#v", config)
	}
	if len(submitted) != 2 ||
		submitted[0] != (dashScopeSubmittedToolResult{callID: "call-1", output: `{"city":"深圳"}`}) ||
		submitted[1] != (dashScopeSubmittedToolResult{callID: "call-2", output: `{"city":"上海"}`}) {
		t.Fatalf("submitted results = %#v", submitted)
	}
	if creates != 2 {
		t.Fatalf("CreateResponse calls = %d, want 2", creates)
	}
	calls := invoker.invocations()
	if len(calls) != 2 ||
		calls[0].arguments != `{"city":"深圳"}` ||
		calls[1].arguments != `{"city":"上海"}` {
		t.Fatalf("tool calls = %#v", calls)
	}
	for _, chunk := range chunks {
		if chunk.ToolCall != nil {
			t.Fatalf("provider ToolCall leaked to public stream: %#v", chunk)
		}
	}
}

func TestDashScopeRealtimeSubmitsEarlierResultBeforeCallLimitFailure(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
	session := newDashScopeToolSession([]*dashscope.RealtimeEvent{
		{
			Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
			CallID:    "call-1",
			Name:      "get_weather",
			Arguments: `{"city":"深圳"}`,
		},
		{
			Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
			CallID:    "call-2",
			Name:      "get_weather",
			Arguments: `{"city":"上海"}`,
		},
	})
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withMaxToolCalls(1),
	)
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_, err = collectDashScopeToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "ToolCall limit exceeded") {
		t.Fatalf("output error = %v, want call limit", err)
	}
	_, submitted, creates := session.toolState()
	if len(submitted) != 1 || submitted[0].callID != "call-1" || creates != 1 {
		t.Fatalf("submitted/creates = %#v/%d, want first result continued before failure", submitted, creates)
	}
}

func TestDashScopeRealtimeRejectsInvalidToolResultJSON(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{
		definitions: dashScopeToolDefinitions(),
		invoke: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`not-json`), nil
		},
	}
	session := newDashScopeToolSession(dashScopeSingleToolEvents("call-1"))
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_, err = collectDashScopeToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "invalid tool result") {
		t.Fatalf("output error = %v, want invalid result", err)
	}
	_, submitted, creates := session.toolState()
	if len(submitted) != 0 || creates != 0 {
		t.Fatalf("submitted/creates = %#v/%d, want none", submitted, creates)
	}
}

func TestDashScopeRealtimeReturnsToolResolutionErrorBeforeConnecting(t *testing.T) {
	wantErr := errors.New("resolution failed")
	invoker := &dashScopeTestToolInvoker{resolveErr: wantErr}
	opener := &fakeDashScopeOpener{}
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = opener
	_, err := transformer.Transform(t.Context(), emptyDashScopeStream{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Transform() error = %v, want %v", err, wantErr)
	}
	if opener.count() != 0 {
		t.Fatalf("Connect calls = %d, want 0", opener.count())
	}
}

func TestDashScopeRealtimeSubmitsModelVisibleBusinessFailure(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{
		definitions: dashScopeToolDefinitions(),
		invoke: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":false,"error":"city unavailable"}`), nil
		},
	}
	session := newDashScopeToolSession(dashScopeSingleToolEvents("call-1"))
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := collectDashScopeToolOutput(stream); err != nil {
		t.Fatalf("collect output: %v", err)
	}
	_, submitted, _ := session.toolState()
	if len(submitted) != 1 || submitted[0].output != `{"ok":false,"error":"city unavailable"}` {
		t.Fatalf("submitted results = %#v", submitted)
	}
}

func TestDashScopeRealtimeReturnsMalformedAndUnknownCallErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		event     *dashscope.RealtimeEvent
		wantError string
	}{
		{
			name: "malformed arguments",
			event: &dashscope.RealtimeEvent{
				Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
				CallID:    "call-1",
				Name:      "get_weather",
				Arguments: `{`,
			},
			wantError: "malformed arguments",
		},
		{
			name: "unknown tool",
			event: &dashscope.RealtimeEvent{
				Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
				CallID:    "call-1",
				Name:      "unknown",
				Arguments: `{}`,
			},
			wantError: "unknown tool",
		},
		{
			name: "missing call ID",
			event: &dashscope.RealtimeEvent{
				Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
				Name:      "get_weather",
				Arguments: `{}`,
			},
			wantError: "call ID is required",
		},
		{
			name: "missing function name",
			event: &dashscope.RealtimeEvent{
				Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
				CallID:    "call-1",
				Arguments: `{}`,
			},
			wantError: "function name is required",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			invoker := &dashScopeTestToolInvoker{
				definitions: dashScopeToolDefinitions(),
				invoke: func(_ context.Context, name string, arguments json.RawMessage) (json.RawMessage, error) {
					if name != "get_weather" {
						return nil, errors.New("unknown tool")
					}
					if !json.Valid(arguments) {
						return nil, errors.New("malformed arguments")
					}
					return json.RawMessage(`{"ok":true}`), nil
				},
			}
			session := newDashScopeToolSession([]*dashscope.RealtimeEvent{test.event})
			transformer := newTransformer(nil, withToolInvoker(invoker))
			transformer.realtime = &dashScopeFixedOpener{session: session}
			stream, err := transformer.Transform(
				t.Context(),
				dashScopeToolInput{done: session.eventsDrained},
			)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			_, err = collectDashScopeToolOutput(stream)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("output error = %v, want %q", err, test.wantError)
			}
			_, submitted, _ := session.toolState()
			if len(submitted) != 0 {
				t.Fatalf("submitted results = %#v, want none", submitted)
			}
		})
	}
}

func TestDashScopeRealtimeRejectsDuplicateProviderCallID(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
	session := newDashScopeToolSession([]*dashscope.RealtimeEvent{
		dashScopeSingleToolEvent("call-1"),
		dashScopeSingleToolEvent("call-1"),
	})
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	_, err = collectDashScopeToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "duplicate ToolCall ID") {
		t.Fatalf("output error = %v, want duplicate call ID", err)
	}
	_, submitted, creates := session.toolState()
	if len(submitted) != 1 || creates != 1 {
		t.Fatalf("submitted/creates = %#v/%d, want first call only", submitted, creates)
	}
}

func TestDashScopeRealtimeCancelsBlockedToolInvocation(t *testing.T) {
	started := make(chan struct{})
	invoker := &dashScopeTestToolInvoker{
		definitions: dashScopeToolDefinitions(),
		invoke: func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	session := newDashScopeToolSession(dashScopeSingleToolEvents("call-1"))
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := transformer.Transform(ctx, dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	<-started
	cancel()
	_, err = collectDashScopeToolOutput(stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("output error = %v, want context cancellation", err)
	}
	_, submitted, _ := session.toolState()
	if len(submitted) != 0 {
		t.Fatalf("submitted results = %#v, want none", submitted)
	}
}

func TestDashScopeRealtimeTimesOutBlockedToolInvocation(t *testing.T) {
	started := make(chan struct{})
	invoker := &dashScopeTestToolInvoker{
		definitions: dashScopeToolDefinitions(),
		invoke: func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	session := newDashScopeToolSession(dashScopeSingleToolEvents("call-1"))
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeFixedOpener{session: session}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := transformer.Transform(ctx, dashScopeToolInput{done: session.eventsDrained})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	<-started
	_, err = collectDashScopeToolOutput(stream)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("output error = %v, want deadline exceeded", err)
	}
	_, submitted, _ := session.toolState()
	if len(submitted) != 0 {
		t.Fatalf("submitted results = %#v, want none", submitted)
	}
}

func TestDashScopeRealtimeReturnsResultSubmissionAndContinuationErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		submitErr error
		createErr error
		want      error
	}{
		{name: "submit", submitErr: errors.New("submit failed"), want: errors.New("submit failed")},
		{name: "continue", createErr: errors.New("continue failed"), want: errors.New("continue failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
			session := newDashScopeToolSession(dashScopeSingleToolEvents("call-1"))
			session.submitErr = test.submitErr
			session.createErr = test.createErr
			transformer := newTransformer(nil, withToolInvoker(invoker))
			transformer.realtime = &dashScopeFixedOpener{session: session}
			stream, err := transformer.Transform(t.Context(), dashScopeToolInput{done: session.eventsDrained})
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			_, err = collectDashScopeToolOutput(stream)
			if err == nil || !strings.Contains(err.Error(), test.want.Error()) {
				t.Fatalf("output error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDashScopeRealtimeAllowsSameProviderCallIDAcrossConcurrentTransforms(t *testing.T) {
	invoker := &dashScopeTestToolInvoker{definitions: dashScopeToolDefinitions()}
	sessions := []*fakeDashScopeSession{
		newDashScopeToolSession(dashScopeSingleToolEvents("shared-call")),
		newDashScopeToolSession(dashScopeSingleToolEvents("shared-call")),
	}
	transformer := newTransformer(nil, withToolInvoker(invoker))
	transformer.realtime = &dashScopeSequenceOpener{sessions: sessions}
	allEventsDone := make(chan struct{})
	go func() {
		for _, session := range sessions {
			<-session.eventsDrained
		}
		close(allEventsDone)
	}()
	var wg sync.WaitGroup
	errs := make(chan error, len(sessions))
	for range sessions {
		wg.Go(func() {
			stream, err := transformer.Transform(
				t.Context(),
				dashScopeToolInput{done: allEventsDone},
			)
			if err == nil {
				_, err = collectDashScopeToolOutput(stream)
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent transform error = %v", err)
		}
	}
	for index, session := range sessions {
		_, submitted, _ := session.toolState()
		if len(submitted) != 1 || submitted[0].callID != "shared-call" {
			t.Fatalf("session %d submitted = %#v", index, submitted)
		}
	}
}

type dashScopeFixedOpener struct {
	session *fakeDashScopeSession
}

func (o *dashScopeFixedOpener) Connect(
	context.Context,
	*dashscope.RealtimeConfig,
) (dashScopeRealtimeSession, error) {
	return o.session, nil
}

type dashScopeSequenceOpener struct {
	mu       sync.Mutex
	sessions []*fakeDashScopeSession
	next     int
}

func (o *dashScopeSequenceOpener) Connect(
	context.Context,
	*dashscope.RealtimeConfig,
) (dashScopeRealtimeSession, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	session := o.sessions[o.next]
	o.next++
	return session, nil
}

func (s *fakeDashScopeSession) toolState() (
	*dashscope.SessionConfig,
	[]dashScopeSubmittedToolResult,
	int,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var config *dashscope.SessionConfig
	if s.updateConfig != nil {
		clone := *s.updateConfig
		if s.updateConfig.Tools != nil {
			clone.Tools = append(
				make([]dashscope.FunctionTool, 0, len(s.updateConfig.Tools)),
				s.updateConfig.Tools...,
			)
		}
		config = &clone
	}
	return config, append([]dashScopeSubmittedToolResult(nil), s.submitted...), s.toolResponseCreates
}

func dashScopeSingleToolEvents(callID string) []*dashscope.RealtimeEvent {
	return []*dashscope.RealtimeEvent{dashScopeSingleToolEvent(callID)}
}

func dashScopeSingleToolEvent(callID string) *dashscope.RealtimeEvent {
	return &dashscope.RealtimeEvent{
		Type:      dashscope.EventTypeResponseFunctionCallArgumentsDone,
		CallID:    callID,
		Name:      "get_weather",
		Arguments: `{"city":"深圳"}`,
	}
}

func newDashScopeToolSession(events []*dashscope.RealtimeEvent) *fakeDashScopeSession {
	return &fakeDashScopeSession{
		events:        events,
		eventsDrained: make(chan struct{}),
	}
}

type dashScopeToolInput struct {
	done <-chan struct{}
}

func (i dashScopeToolInput) Next() (*genx.MessageChunk, error) {
	<-i.done
	return nil, io.EOF
}

func (dashScopeToolInput) Close() error               { return nil }
func (dashScopeToolInput) CloseWithError(error) error { return nil }

func collectDashScopeToolOutput(stream genx.Stream) ([]*genx.MessageChunk, error) {
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		switch {
		case errors.Is(err, io.EOF), errors.Is(err, genx.ErrDone):
			return chunks, nil
		case err != nil:
			return chunks, err
		case chunk != nil:
			chunks = append(chunks, chunk)
		}
	}
}
