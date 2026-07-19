package eino

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestAgentRunsEinoNativeToolsStrictlyInModelOrder(t *testing.T) {
	var mu sync.Mutex
	var calls []commonagent.ToolCall
	toolkit := commonagent.ToolkitFunc{
		List: func() []commonagent.Tool {
			return []commonagent.Tool{{Name: "first"}, {Name: "second"}}
		},
		InvokeFunc: func(_ context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
			mu.Lock()
			calls = append(calls, call)
			mu.Unlock()
			return commonagent.ToolResult{ID: call.ID, Content: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
	chatModel := &scriptedModel{respond: func(_ context.Context, input []*schema.Message) []*schema.Message {
		if input[len(input)-1].Role == schema.User {
			return []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{
				{ID: "call-2", Type: "function", Function: schema.FunctionCall{Name: "second", Arguments: `{"n":2}`}},
				{ID: "call-1", Type: "function", Function: schema.FunctionCall{Name: "first", Arguments: `{"n":1}`}},
			})}
		}
		return []*schema.Message{schema.AssistantMessage("done", nil)}
	}}
	agent, err := New(t.Context(), Config{Model: chatModel, Toolkit: toolkit, MaxToolCalls: 2})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := buffer.N[*genx.MessageChunk](3)
	addTextTurn(t, input, "user-1", "run tools")
	_ = input.CloseWrite()
	output, err := agent.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := readAll(t, output)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 || calls[0].ID != "call-2" || calls[1].ID != "call-1" {
		for i, chunk := range chunks {
			t.Logf("chunk[%d]: part=%#v ctrl=%#v", i, chunk.Part, chunk.Ctrl)
		}
		t.Fatalf("tool calls = %#v, want model order call-2 then call-1; model inputs=%#v", calls, chatModel.inputs())
	}
	if got := visibleText(chunks); got != "done" {
		t.Fatalf("visible text = %q, want done; chunks=%#v", got, chunks)
	}
	if len(chatModel.toolInfos()) != 2 {
		t.Fatalf("bound tools = %d, want 2", len(chatModel.toolInfos()))
	}
	for _, chunk := range chunks {
		if chunk.ToolCall != nil || chunk.Role == genx.RoleTool {
			t.Fatalf("internal tool traffic leaked: %#v", chunk)
		}
	}
}

func TestAgentReplacementInputInterruptsBufferedResponseAndKeepsPulledHistory(t *testing.T) {
	secondInput := make(chan []*schema.Message, 1)
	chatModel := &scriptedModel{respond: func(_ context.Context, input []*schema.Message) []*schema.Message {
		last := input[len(input)-1]
		if last.Content == "first" {
			return []*schema.Message{
				schema.AssistantMessage("visible", nil),
				schema.AssistantMessage("discarded", nil),
			}
		}
		secondInput <- cloneMessages(input)
		return []*schema.Message{schema.AssistantMessage("fresh", nil)}
	}}
	agent, err := New(t.Context(), Config{Model: chatModel, Toolkit: commonagent.EmptyToolkit()})
	if err != nil {
		t.Fatal(err)
	}
	input := buffer.N[*genx.MessageChunk](8)
	addTextTurn(t, input, "user-1", "first")
	output, err := agent.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}

	firstEmpty, err := output.Next()
	if err != nil || firstEmpty.Part != genx.Text("") {
		t.Fatalf("first output = %#v, %v", firstEmpty, err)
	}
	visible, err := output.Next()
	if err != nil || visible.Part != genx.Text("visible") {
		t.Fatalf("visible output = %#v, %v", visible, err)
	}
	firstStreamID := visible.Ctrl.StreamID
	addTextTurn(t, input, "user-2", "second")
	_ = input.CloseWrite()

	select {
	case messages := <-secondInput:
		if !containsInterruptedAssistant(messages, "visible") {
			t.Fatalf("second model history does not contain pulled interrupted response: %#v", messages)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second model input")
	}

	chunks := readAll(t, output)
	if slices.ContainsFunc(chunks, func(chunk *genx.MessageChunk) bool {
		text, _ := chunk.Part.(genx.Text)
		return text == "discarded"
	}) {
		t.Fatalf("unpulled response was not discarded: %#v", chunks)
	}
	if !slices.ContainsFunc(chunks, func(chunk *genx.MessageChunk) bool {
		return chunk.IsEndOfStream() && chunk.Ctrl.StreamID == firstStreamID && chunk.Ctrl.Error == commonagent.Interrupted
	}) {
		t.Fatalf("missing interrupted EOS for %q: %#v", firstStreamID, chunks)
	}
	if got := visibleText(chunks); got != "fresh" {
		t.Fatalf("remaining visible text = %q, want fresh; chunks=%#v", got, chunks)
	}
}

func TestAgentRecallsAndObservesInjectedMemory(t *testing.T) {
	store := &recordingMemoryStore{recall: memory.RecallResult{Matches: []memory.Match{{Fact: memory.Fact{ID: "fact-1", Text: "likes tea"}, Score: 0.9}}}}
	seen := make(chan []*schema.Message, 1)
	chatModel := &scriptedModel{respond: func(_ context.Context, input []*schema.Message) []*schema.Message {
		seen <- cloneMessages(input)
		return []*schema.Message{schema.AssistantMessage("answer", nil)}
	}}
	runtime, err := New(t.Context(), Config{Model: chatModel, Toolkit: commonagent.EmptyToolkit(), Memory: store, MemoryLimit: 3})
	if err != nil {
		t.Fatal(err)
	}
	input := buffer.N[*genx.MessageChunk](3)
	addTextTurn(t, input, "user-1", "what do I like?")
	_ = input.CloseWrite()
	output, err := runtime.Transform(t.Context(), "", input)
	if err != nil {
		t.Fatal(err)
	}
	if got := visibleText(readAll(t, output)); got != "answer" {
		t.Fatalf("visible text = %q, want answer", got)
	}
	messages := <-seen
	if !slices.ContainsFunc(messages, func(message *schema.Message) bool {
		return message.Role == schema.System && strings.Contains(message.Content, "likes tea")
	}) {
		t.Fatalf("model messages = %#v, want recalled system memory", messages)
	}
	query, observations := store.snapshot()
	if query.Text != "what do I like?" || query.Limit != 3 {
		t.Fatalf("memory query = %+v", query)
	}
	if len(observations) != 1 || len(observations[0].Turns) != 2 || observations[0].Turns[0].Text != "what do I like?" || observations[0].Turns[1].Text != "answer" {
		t.Fatalf("memory observations = %+v", observations)
	}
}

func TestPulledHistoryAcceptsPendingMemoryAndReportsObserveFailure(t *testing.T) {
	store := &recordingMemoryStore{observeResult: memory.ObserveResult{Operation: &memory.Operation{ID: "pending-1", Status: memory.OperationPending}}}
	var reported []error
	pulled := newPulledHistory(&conversationHistory{}, store, func(err error) { reported = append(reported, err) })
	pulled.track("response-1", "hello")
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "response-1"}})
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response-1", EndOfStream: true}})
	if len(reported) != 0 {
		t.Fatalf("pending memory operation reported errors: %v", reported)
	}

	store.setObserveResult(memory.ObserveResult{}, errors.New("memory unavailable"))
	pulled.track("response-2", "again")
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "response-2"}})
	pulled.observe(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "response-2", EndOfStream: true}})
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "memory unavailable") {
		t.Fatalf("reported errors = %v", reported)
	}
}

func TestHistoryReopensInAppendOrder(t *testing.T) {
	store := &recordingLogStore{}
	first, err := newHistory(&HistoryConfig{Store: store, Stream: "conversation", RecentLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.append(t.Context(), schema.UserMessage("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := first.append(t.Context(), schema.AssistantMessage("second", nil), false); err != nil {
		t.Fatal(err)
	}
	reopened, err := newHistory(&HistoryConfig{Store: store, Stream: "conversation", RecentLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := reopened.recent(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "first" || messages[1].Content != "second" {
		t.Fatalf("reopened history = %#v", messages)
	}
}

func TestToolBudgetRejectsDuplicateCallIdentity(t *testing.T) {
	ctx := withToolBudget(t.Context(), 4)
	if err := consumeToolBudget(ctx, "call-1"); err != nil {
		t.Fatal(err)
	}
	if err := consumeToolBudget(ctx, "call-1"); !errors.Is(err, commonagent.ErrInvalidToolCall) {
		t.Fatalf("consumeToolBudget(duplicate) error = %v, want ErrInvalidToolCall", err)
	}
}

type scriptedModel struct {
	mu      sync.Mutex
	tools   []*schema.ToolInfo
	seen    [][]*schema.Message
	respond func(context.Context, []*schema.Message) []*schema.Message
}

type recordingMemoryStore struct {
	mu            sync.Mutex
	recall        memory.RecallResult
	query         memory.Query
	observations  []memory.Observation
	observeResult memory.ObserveResult
	observeErr    error
}

func (s *recordingMemoryStore) Observe(_ context.Context, observation memory.Observation) (memory.ObserveResult, error) {
	s.mu.Lock()
	s.observations = append(s.observations, observation)
	result, err := s.observeResult, s.observeErr
	s.mu.Unlock()
	return result, err
}

func (s *recordingMemoryStore) setObserveResult(result memory.ObserveResult, err error) {
	s.mu.Lock()
	s.observeResult = result
	s.observeErr = err
	s.mu.Unlock()
}

func (s *recordingMemoryStore) Recall(_ context.Context, query memory.Query) (memory.RecallResult, error) {
	s.mu.Lock()
	s.query = query
	s.mu.Unlock()
	return s.recall, nil
}

func (*recordingMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}

func (*recordingMemoryStore) Delete(context.Context, memory.DeleteRequest) error { return nil }

func (s *recordingMemoryStore) snapshot() (memory.Query, []memory.Observation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query, slices.Clone(s.observations)
}

type recordingLogStore struct {
	mu      sync.Mutex
	records []logstore.Record
}

func (s *recordingLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		s.records = append(s.records, record)
		keys[index] = record.Key()
	}
	return keys, nil
}

func (s *recordingLogStore) Query(_ context.Context, query logstore.Query) (logstore.Page, error) {
	s.mu.Lock()
	records := slices.Clone(s.records)
	s.mu.Unlock()
	slices.SortFunc(records, func(left, right logstore.Record) int { return left.Time.Compare(right.Time) })
	if query.Order == logstore.OrderDesc {
		slices.Reverse(records)
	}
	if query.Limit > 0 && len(records) > query.Limit {
		records = records[:query.Limit]
	}
	return logstore.Page{Records: records}, nil
}

func (*recordingLogStore) Replace(context.Context, logstore.Record) error   { return nil }
func (*recordingLogStore) Delete(context.Context, logstore.RecordKey) error { return nil }
func (*recordingLogStore) Close() error                                     { return nil }

func (m *scriptedModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.mu.Lock()
	m.tools = slices.Clone(tools)
	m.mu.Unlock()
	return m, nil
}

func (m *scriptedModel) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.record(input)
	messages := m.respond(ctx, cloneMessages(input))
	if len(messages) == 0 {
		return nil, io.EOF
	}
	return messages[len(messages)-1], nil
}

func (m *scriptedModel) Stream(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.record(input)
	return schema.StreamReaderFromArray(m.respond(ctx, cloneMessages(input))), nil
}

func (m *scriptedModel) record(input []*schema.Message) {
	m.mu.Lock()
	m.seen = append(m.seen, cloneMessages(input))
	m.mu.Unlock()
}

func (m *scriptedModel) inputs() [][]*schema.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.seen)
}

func (m *scriptedModel) toolInfos() []*schema.ToolInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.tools)
}

func addTextTurn(t *testing.T, input *buffer.Buffer[*genx.MessageChunk], id, content string) {
	t.Helper()
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}},
		{Role: genx.RoleUser, Part: genx.Text(content), Ctrl: &genx.StreamCtrl{StreamID: id}},
		{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}},
	} {
		if err := input.Add(chunk); err != nil {
			t.Fatal(err)
		}
	}
}

func readAll(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return chunks
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

func visibleText(chunks []*genx.MessageChunk) string {
	var text strings.Builder
	for _, chunk := range chunks {
		if chunk.IsEndOfStream() {
			continue
		}
		part, ok := chunk.Part.(genx.Text)
		if ok {
			text.WriteString(string(part))
		}
	}
	return text.String()
}

func containsInterruptedAssistant(messages []*schema.Message, content string) bool {
	for _, message := range messages {
		if message.Role == schema.Assistant && message.Content == content && message.Extra["gizclaw.interrupted"] == true {
			return true
		}
	}
	return false
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	clones := make([]*schema.Message, len(messages))
	for i := range messages {
		clones[i] = cloneMessage(messages[i])
	}
	return clones
}
