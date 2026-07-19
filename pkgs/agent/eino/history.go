package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

const historyKind = "agent.eino.message"

type historyPayload struct {
	Message     *schema.Message `json:"message"`
	Interrupted bool            `json:"interrupted,omitempty"`
}

type history struct {
	store  logstore.MutableStore
	stream string
	limit  int

	mu       sync.Mutex
	lastTime time.Time
	sequence uint64
}

type conversationHistory struct {
	store *history

	mu   sync.Mutex
	live []*schema.Message
}

func (h *conversationHistory) recent(ctx context.Context) ([]*schema.Message, error) {
	if h.store != nil {
		return h.store.recent(ctx)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	messages := make([]*schema.Message, len(h.live))
	for i := range h.live {
		messages[i] = cloneMessage(h.live[i])
	}
	return messages, nil
}

func (h *conversationHistory) append(ctx context.Context, message *schema.Message, interrupted bool) error {
	if h.store != nil {
		return h.store.append(ctx, message, interrupted)
	}
	h.mu.Lock()
	h.live = append(h.live, cloneMessage(message))
	if interrupted && len(h.live) > 0 {
		last := h.live[len(h.live)-1]
		if last.Extra == nil {
			last.Extra = make(map[string]any)
		}
		last.Extra["gizclaw.interrupted"] = true
	}
	h.mu.Unlock()
	return nil
}

func newHistory(config *HistoryConfig) (*history, error) {
	if config == nil {
		return nil, nil
	}
	if config.Store == nil {
		return nil, fmt.Errorf("agent/eino: history store is required")
	}
	stream := strings.TrimSpace(config.Stream)
	if stream == "" {
		return nil, fmt.Errorf("agent/eino: history stream is required")
	}
	limit := config.RecentLimit
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > logstore.MaxLimit {
		return nil, fmt.Errorf("agent/eino: history recent limit must be between 1 and %d", logstore.MaxLimit)
	}
	return &history{store: config.Store, stream: stream, limit: limit}, nil
}

func (h *history) recent(ctx context.Context) ([]*schema.Message, error) {
	if h == nil {
		return nil, nil
	}
	end := time.Now().UTC().Add(time.Millisecond).Truncate(time.Millisecond)
	page, err := h.store.Query(ctx, logstore.Query{
		Streams: []string{h.stream},
		Kinds:   []string{historyKind},
		Start:   time.Unix(0, 0).UTC(),
		End:     end,
		Limit:   h.limit,
		Order:   logstore.OrderDesc,
	})
	if err != nil {
		return nil, fmt.Errorf("agent/eino: query history: %w", err)
	}
	messages := make([]*schema.Message, 0, len(page.Records))
	for _, record := range page.Records {
		var payload historyPayload
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return nil, fmt.Errorf("agent/eino: decode history record %q: %w", record.ID, err)
		}
		if payload.Message == nil {
			return nil, fmt.Errorf("agent/eino: history record %q has no message", record.ID)
		}
		message := cloneMessage(payload.Message)
		if payload.Interrupted {
			if message.Extra == nil {
				message.Extra = make(map[string]any)
			}
			message.Extra["gizclaw.interrupted"] = true
		}
		messages = append(messages, message)
	}
	slices.Reverse(messages)
	return messages, nil
}

func (h *history) append(ctx context.Context, message *schema.Message, interrupted bool) error {
	if h == nil || message == nil {
		return nil
	}
	payload, err := json.Marshal(historyPayload{Message: cloneMessage(message), Interrupted: interrupted})
	if err != nil {
		return fmt.Errorf("agent/eino: encode history: %w", err)
	}

	h.mu.Lock()
	now := time.Now().UTC()
	if !now.After(h.lastTime) {
		now = h.lastTime.Add(time.Nanosecond)
	}
	h.lastTime = now
	h.sequence++
	id := fmt.Sprintf("%020d-%06d", now.UnixNano(), h.sequence)
	h.mu.Unlock()

	attributes := map[string]string{"role": string(message.Role)}
	if interrupted {
		attributes["interrupted"] = "true"
	}
	_, err = h.store.Append(ctx, []logstore.Record{{
		ID:         id,
		Time:       now,
		Stream:     h.stream,
		Kind:       historyKind,
		Message:    message.Content,
		Attributes: attributes,
		Payload:    payload,
	}})
	if err != nil {
		return fmt.Errorf("agent/eino: append history: %w", err)
	}
	return nil
}

type pulledHistory struct {
	history *conversationHistory
	memory  memory.Store

	mu     sync.Mutex
	states map[string]*pulledResponse
	users  map[string]string
	report func(error)
}

type pulledResponse struct {
	content   strings.Builder
	committed bool
}

func newPulledHistory(h *conversationHistory, memoryStore memory.Store, report func(error)) *pulledHistory {
	return &pulledHistory{history: h, memory: memoryStore, states: make(map[string]*pulledResponse), users: make(map[string]string), report: report}
}

func (p *pulledHistory) track(streamID, user string) {
	p.mu.Lock()
	p.users[streamID] = user
	p.mu.Unlock()
}

func (p *pulledHistory) observe(chunk *genx.MessageChunk) {
	if p == nil || chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil {
		return
	}
	streamID := chunk.Ctrl.StreamID
	if streamID == "" {
		return
	}
	p.mu.Lock()
	state := p.states[streamID]
	if state == nil {
		state = &pulledResponse{}
		p.states[streamID] = state
	}
	if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
		state.content.WriteString(string(text))
	}
	shouldCommit := chunk.IsEndOfStream() && !state.committed
	interrupted := chunk.Ctrl.Error == commonagent.Interrupted
	content := state.content.String()
	user := p.users[streamID]
	if shouldCommit {
		state.committed = true
	}
	p.mu.Unlock()
	if shouldCommit {
		p.append(content, interrupted)
		if !interrupted {
			p.observeMemory(streamID, user, content)
		}
	}
}

func (p *pulledHistory) observeMemory(streamID, user, assistant string) {
	if p.memory == nil || strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
		return
	}
	now := time.Now().UTC()
	result, err := p.memory.Observe(context.Background(), memory.Observation{
		ID: streamID, ObservedAt: now,
		Turns: []memory.Turn{
			{ID: streamID + ":user", Role: memory.RoleUser, Text: user, ObservedAt: now},
			{ID: streamID + ":assistant", Role: memory.RoleAssistant, Text: assistant, ObservedAt: now},
		},
	})
	if err != nil {
		p.reportError(fmt.Errorf("agent/eino: observe memory: %w", err))
		return
	}
	if result.Operation != nil && result.Operation.Status == memory.OperationFailed {
		p.reportError(fmt.Errorf("agent/eino: memory operation %q failed: %s", result.Operation.ID, result.Operation.Error))
	}
}

func (p *pulledHistory) commitInterrupted(streamID string) {
	if p == nil || streamID == "" {
		return
	}
	p.mu.Lock()
	state := p.states[streamID]
	if state == nil {
		state = &pulledResponse{}
		p.states[streamID] = state
	}
	if state.committed {
		p.mu.Unlock()
		return
	}
	state.committed = true
	content := state.content.String()
	p.mu.Unlock()
	p.append(content, true)
}

func (p *pulledHistory) append(content string, interrupted bool) {
	if content == "" && !interrupted {
		return
	}
	if err := p.history.append(context.Background(), schema.AssistantMessage(content, nil), interrupted); err != nil {
		p.reportError(err)
	}
}

func (p *pulledHistory) reportError(err error) {
	if err == nil {
		return
	}
	if p.report != nil {
		p.report(err)
		return
	}
	slog.Error("eino agent background operation failed", "error", err)
}

func cloneMessage(message *schema.Message) *schema.Message {
	if message == nil {
		return nil
	}
	data, err := json.Marshal(message)
	if err != nil {
		return &schema.Message{Role: message.Role, Content: message.Content}
	}
	var clone schema.Message
	if err := json.Unmarshal(data, &clone); err != nil {
		return &schema.Message{Role: message.Role, Content: message.Content}
	}
	return &clone
}
