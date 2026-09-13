package doubaorealtimeduplex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolrun"
	"github.com/google/jsonschema-go/jsonschema"
)

type doubaoToolInvocation struct {
	name      string
	arguments string
}

type doubaoTestToolInvoker struct {
	mu          sync.Mutex
	definitions []genx.ToolDefinition
	resolveErr  error
	invoke      func(context.Context, string, json.RawMessage) (json.RawMessage, error)
	calls       []doubaoToolInvocation
}

func (i *doubaoTestToolInvoker) ResolveTools(context.Context) ([]genx.ToolDefinition, error) {
	if i.resolveErr != nil {
		return nil, i.resolveErr
	}
	return i.definitions, nil
}

func (i *doubaoTestToolInvoker) InvokeTool(
	ctx context.Context,
	name string,
	arguments json.RawMessage,
) (json.RawMessage, error) {
	i.mu.Lock()
	i.calls = append(i.calls, doubaoToolInvocation{name: name, arguments: string(arguments)})
	i.mu.Unlock()
	if i.invoke != nil {
		return i.invoke(ctx, name, arguments)
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (i *doubaoTestToolInvoker) invocations() []doubaoToolInvocation {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]doubaoToolInvocation(nil), i.calls...)
}

func doubaoToolDefinitions() []genx.ToolDefinition {
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

func TestResolveDoubaoRealtimeDuplexTools(t *testing.T) {
	invoker := &doubaoTestToolInvoker{definitions: doubaoToolDefinitions()}
	tools, err := resolveDoubaoRealtimeDuplexTools(t.Context(), invoker)
	if err != nil {
		t.Fatalf("resolveDoubaoRealtimeDuplexTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Type != "function" || tool.Name != "get_weather" ||
		tool.Description != "Look up weather." || tool.Parameters == nil {
		t.Fatalf("tool = %#v", tool)
	}
	if tool.Parameters.Type != "object" ||
		tool.Parameters.Properties["city"] == nil ||
		tool.Parameters.Properties["city"].Type != "string" ||
		tool.Parameters.AdditionalProperties == nil ||
		*tool.Parameters.AdditionalProperties {
		t.Fatalf("tool parameters = %#v", tool.Parameters)
	}
}

func TestResolveDoubaoRealtimeDuplexToolsRejectsUnsupportedSchema(t *testing.T) {
	invoker := &doubaoTestToolInvoker{definitions: []genx.ToolDefinition{{
		Name: "pattern_tool",
		Argument: &jsonschema.Schema{
			Type:    "string",
			Pattern: "^supported-by-genx-only$",
		},
	}}}
	_, err := resolveDoubaoRealtimeDuplexTools(t.Context(), invoker)
	if err == nil || !strings.Contains(err.Error(), `convert tool "pattern_tool" schema`) {
		t.Fatalf("resolveDoubaoRealtimeDuplexTools() error = %v, want unsupported schema error", err)
	}
}

func TestDoubaoRealtimeDuplexInvokesToolsInOrderAndKeepsControlInternal(t *testing.T) {
	invoker := &doubaoTestToolInvoker{
		definitions: doubaoToolDefinitions(),
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
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{
				Type: doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
				FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{
					{CallID: "call-1", Name: "get_weather", Arguments: `{"city":"深圳"}`},
					{CallID: "call-2", Name: "get_weather", Arguments: `{"city":"上海"}`},
				},
			},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDelta, ResponseID: "response-1", Delta: "完成"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDone, ResponseID: "response-1"},
			{Type: doubaospeech.RealtimeDuplexEventSessionClosed},
		},
	}
	opener := &fakeDoubaoRealtimeDuplexOpener{session: session}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(opener),
	)
	stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks, err := collectDoubaoToolOutput(stream)
	if err != nil {
		t.Fatalf("collect output: %v", err)
	}
	if opener.config == nil || len(opener.config.Session.Tools) != 1 {
		t.Fatalf("provider tools = %#v", opener.config)
	}
	calls := invoker.invocations()
	if len(calls) != 2 ||
		calls[0].arguments != `{"city":"深圳"}` ||
		calls[1].arguments != `{"city":"上海"}` {
		t.Fatalf("tool calls = %#v", calls)
	}
	outputs := session.functionOutputs()
	if len(outputs) != 2 ||
		outputs[0].CallID != "call-1" ||
		outputs[0].Output != `{"city":"深圳"}` ||
		outputs[1].CallID != "call-2" ||
		outputs[1].Output != `{"city":"上海"}` {
		t.Fatalf("function outputs = %#v", outputs)
	}
	for _, output := range outputs {
		if strings.Contains(output.Output, "gizclaw-internal-fake") {
			t.Fatalf("legacy fake result was submitted: %#v", output)
		}
	}
	for _, chunk := range chunks {
		if chunk.ToolCall != nil {
			t.Fatalf("provider ToolCall leaked to public stream: %#v", chunk)
		}
		if text, ok := chunk.Part.(genx.Text); ok &&
			strings.Contains(string(text), "gizclaw-internal-fake") {
			t.Fatalf("legacy fake result leaked to public stream: %#v", chunk)
		}
	}
}

func TestDoubaoRealtimeDuplexSubmitsEarlierResultBeforeCallLimitFailure(t *testing.T) {
	invoker := &doubaoTestToolInvoker{definitions: doubaoToolDefinitions()}
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{{
			Type: doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
			FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{
				{CallID: "call-1", Name: "get_weather", Arguments: `{"city":"深圳"}`},
				{CallID: "call-2", Name: "get_weather", Arguments: `{"city":"上海"}`},
			},
		}},
	}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withMaxToolCalls(1),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	_, err = collectDoubaoToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "ToolCall limit exceeded") {
		t.Fatalf("output error = %v, want call limit", err)
	}
	outputs := session.functionOutputs()
	if len(outputs) != 1 || outputs[0].CallID != "call-1" {
		t.Fatalf("function outputs = %#v, want first result submitted before failure", outputs)
	}
}

func TestDoubaoRealtimeDuplexRejectsInvalidToolResultJSON(t *testing.T) {
	invoker := &doubaoTestToolInvoker{
		definitions: doubaoToolDefinitions(),
		invoke: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`not-json`), nil
		},
	}
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{{
			Type: doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
			FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{{
				CallID: "call-1", Name: "get_weather", Arguments: `{"city":"深圳"}`,
			}},
		}},
	}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	_, err = collectDoubaoToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "invalid tool result") {
		t.Fatalf("output error = %v, want invalid result", err)
	}
	if outputs := session.functionOutputs(); len(outputs) != 0 {
		t.Fatalf("function outputs = %#v, want none", outputs)
	}
}

func TestDoubaoRealtimeDuplexReturnsFunctionCallOutputError(t *testing.T) {
	wantErr := errors.New("send function output failed")
	session := &fakeDoubaoRealtimeDuplexSession{
		events:          doubaoSingleToolEvents("call-1"),
		functionCallErr: wantErr,
	}
	invoker := &doubaoTestToolInvoker{definitions: doubaoToolDefinitions()}
	transformer := newTransformer(nil, withToolInvoker(invoker))
	input := newDoubaoRealtimeDuplexInputReader(emptyRealtimeStream{})
	defer input.Close()
	_, err := transformer.processLoop(
		t.Context(),
		input,
		newBufferStream(1),
		session,
		toolrun.New(invoker, 0),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processLoop() error = %v, want %v", err, wantErr)
	}
	if !session.waitClosed(time.Second) {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeDuplexReturnsToolResolutionErrorBeforeOpeningSession(t *testing.T) {
	wantErr := errors.New("resolution failed")
	invoker := &doubaoTestToolInvoker{resolveErr: wantErr}
	opener := &fakeDoubaoRealtimeDuplexOpener{}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(opener),
	)
	_, err := transformer.transform(t.Context(), emptyRealtimeStream{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("transform() error = %v, want %v", err, wantErr)
	}
	if opener.openCount() != 0 {
		t.Fatalf("OpenSession calls = %d, want 0", opener.openCount())
	}
}

func TestDoubaoRealtimeDuplexSubmitsModelVisibleBusinessFailure(t *testing.T) {
	invoker := &doubaoTestToolInvoker{
		definitions: doubaoToolDefinitions(),
		invoke: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":false,"error":"city unavailable"}`), nil
		},
	}
	session := &fakeDoubaoRealtimeDuplexSession{events: doubaoSingleToolEvents("call-1")}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	if _, err := collectDoubaoToolOutput(stream); err != nil {
		t.Fatalf("collect output: %v", err)
	}
	outputs := session.functionOutputs()
	if len(outputs) != 1 || outputs[0].Output != `{"ok":false,"error":"city unavailable"}` {
		t.Fatalf("function outputs = %#v", outputs)
	}
}

func TestDoubaoRealtimeDuplexReturnsMalformedAndUnknownCallErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		call      doubaospeech.RealtimeDuplexFunctionCall
		wantError string
	}{
		{
			name:      "malformed arguments",
			call:      doubaospeech.RealtimeDuplexFunctionCall{CallID: "call-1", Name: "get_weather", Arguments: `{`},
			wantError: "malformed arguments",
		},
		{
			name:      "unknown tool",
			call:      doubaospeech.RealtimeDuplexFunctionCall{CallID: "call-1", Name: "unknown", Arguments: `{}`},
			wantError: "unknown tool",
		},
		{
			name:      "missing call ID",
			call:      doubaospeech.RealtimeDuplexFunctionCall{Name: "get_weather", Arguments: `{}`},
			wantError: "call ID is required",
		},
		{
			name:      "missing function name",
			call:      doubaospeech.RealtimeDuplexFunctionCall{CallID: "call-1", Arguments: `{}`},
			wantError: "function name is required",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			invoker := &doubaoTestToolInvoker{
				definitions: doubaoToolDefinitions(),
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
			session := &fakeDoubaoRealtimeDuplexSession{
				events: []*doubaospeech.RealtimeDuplexEvent{{
					Type:          doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
					FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{test.call},
				}},
			}
			transformer := newTransformer(
				nil,
				withToolInvoker(invoker),
				withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
			)
			stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
			if err != nil {
				t.Fatalf("transform() error = %v", err)
			}
			_, err = collectDoubaoToolOutput(stream)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("output error = %v, want %q", err, test.wantError)
			}
			if outputs := session.functionOutputs(); len(outputs) != 0 {
				t.Fatalf("function outputs = %#v, want none", outputs)
			}
		})
	}
}

func TestDoubaoRealtimeDuplexRejectsDuplicateProviderCallID(t *testing.T) {
	invoker := &doubaoTestToolInvoker{definitions: doubaoToolDefinitions()}
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{
				Type:          doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
				FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{doubaoSingleToolCall("call-1")},
			},
			{
				Type:          doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
				FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{doubaoSingleToolCall("call-1")},
			},
		},
	}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	_, err = collectDoubaoToolOutput(stream)
	if err == nil || !strings.Contains(err.Error(), "duplicate ToolCall ID") {
		t.Fatalf("output error = %v, want duplicate call ID", err)
	}
	if outputs := session.functionOutputs(); len(outputs) != 1 {
		t.Fatalf("function outputs = %#v, want first call only", outputs)
	}
}

func TestDoubaoRealtimeDuplexCancelsBlockedToolInvocation(t *testing.T) {
	started := make(chan struct{})
	invoker := &doubaoTestToolInvoker{
		definitions: doubaoToolDefinitions(),
		invoke: func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	session := &fakeDoubaoRealtimeDuplexSession{events: doubaoSingleToolEvents("call-1")}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := transformer.transform(ctx, emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	<-started
	cancel()
	_, err = collectDoubaoToolOutput(stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("output error = %v, want context cancellation", err)
	}
	if outputs := session.functionOutputs(); len(outputs) != 0 {
		t.Fatalf("function outputs = %#v, want none", outputs)
	}
}

func TestDoubaoRealtimeDuplexTimesOutBlockedToolInvocation(t *testing.T) {
	started := make(chan struct{})
	invoker := &doubaoTestToolInvoker{
		definitions: doubaoToolDefinitions(),
		invoke: func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	session := &fakeDoubaoRealtimeDuplexSession{events: doubaoSingleToolEvents("call-1")}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := transformer.transform(ctx, emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	<-started
	_, err = collectDoubaoToolOutput(stream)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("output error = %v, want deadline exceeded", err)
	}
	if outputs := session.functionOutputs(); len(outputs) != 0 {
		t.Fatalf("function outputs = %#v, want none", outputs)
	}
}

func TestDoubaoRealtimeDuplexAllowsSameProviderCallIDAcrossConcurrentTransforms(t *testing.T) {
	invoker := &doubaoTestToolInvoker{definitions: doubaoToolDefinitions()}
	sessions := []*fakeDoubaoRealtimeDuplexSession{
		{events: doubaoSingleToolEvents("shared-call")},
		{events: doubaoSingleToolEvents("shared-call")},
	}
	transformer := newTransformer(
		nil,
		withToolInvoker(invoker),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{sessions: sessions}),
	)
	var wg sync.WaitGroup
	errs := make(chan error, len(sessions))
	for range sessions {
		wg.Go(func() {
			stream, err := transformer.transform(t.Context(), emptyRealtimeStream{})
			if err == nil {
				_, err = collectDoubaoToolOutput(stream)
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
		outputs := session.functionOutputs()
		if len(outputs) != 1 || outputs[0].CallID != "shared-call" {
			t.Fatalf("session %d outputs = %#v", index, outputs)
		}
	}
}

func doubaoSingleToolEvents(callID string) []*doubaospeech.RealtimeDuplexEvent {
	return []*doubaospeech.RealtimeDuplexEvent{
		{
			Type:          doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
			FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{doubaoSingleToolCall(callID)},
		},
		{Type: doubaospeech.RealtimeDuplexEventSessionClosed},
	}
}

func doubaoSingleToolCall(callID string) doubaospeech.RealtimeDuplexFunctionCall {
	return doubaospeech.RealtimeDuplexFunctionCall{
		CallID: callID, Name: "get_weather", Arguments: `{"city":"深圳"}`,
	}
}

func collectDoubaoToolOutput(stream genx.Stream) ([]*genx.MessageChunk, error) {
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
