package flowcraft

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const (
	historyStream  = "flowcraft-history"
	historyKind    = "message"
	historyVersion = 1
)

type historyStore struct {
	store        logstore.MutableStore
	workspace    string
	conversation string

	mu       sync.Mutex
	lastTime time.Time
	sequence uint64
}

type conversationHistory struct {
	store *historyStore

	mu   sync.Mutex
	live []flowmodel.Message
}

func (h *conversationHistory) recent(ctx context.Context, limit int) ([]flowmodel.Message, error) {
	if h.store != nil {
		return h.store.recent(ctx, limit)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneMessages(h.live)
}

func (h *conversationHistory) append(ctx context.Context, messages []flowmodel.Message, interrupted bool) error {
	owned := flowmodel.CloneMessages(messages)
	if interrupted {
		for index := range owned {
			owned[index].Parts = append(owned[index].Parts, flowmodel.Part{
				Type: flowmodel.PartData,
				Data: &flowmodel.DataRef{
					MimeType: "application/vnd.gizclaw.interruption+json",
					Value:    map[string]any{"interrupted": true},
				},
			})
		}
	}
	if h.store != nil {
		return h.store.append(ctx, owned, interrupted)
	}
	h.mu.Lock()
	h.live = append(h.live, owned...)
	h.mu.Unlock()
	return nil
}

func cloneMessages(messages []flowmodel.Message) ([]flowmodel.Message, error) {
	data, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("agent/flowcraft: clone history: %w", err)
	}
	var owned []flowmodel.Message
	if err := json.Unmarshal(data, &owned); err != nil {
		return nil, fmt.Errorf("agent/flowcraft: clone history: %w", err)
	}
	return owned, nil
}

func newHistoryStore(store logstore.MutableStore, workspace, conversation string) (*historyStore, error) {
	if store == nil {
		return nil, nil
	}
	workspace = strings.TrimSpace(workspace)
	conversation = strings.TrimSpace(conversation)
	if workspace == "" || conversation == "" {
		return nil, fmt.Errorf("agent/flowcraft: history workspace and conversation are required")
	}
	return &historyStore{store: store, workspace: workspace, conversation: conversation}, nil
}

func (h *historyStore) recent(ctx context.Context, limit int) ([]flowmodel.Message, error) {
	if h == nil {
		return nil, nil
	}
	if limit <= 0 || limit > logstore.MaxLimit {
		limit = logstore.MaxLimit
	}
	page, err := h.store.Query(ctx, logstore.Query{
		Streams: []string{historyStream},
		Kinds:   []string{historyKind},
		Matchers: []logstore.AttributeMatcher{
			{Name: "workspace_name", Op: logstore.MatchEqual, Value: h.workspace},
			{Name: "conversation_id", Op: logstore.MatchEqual, Value: h.conversation},
		},
		Start: time.Unix(0, 0).UTC(),
		End:   time.Date(2262, time.January, 1, 0, 0, 0, 0, time.UTC),
		Limit: limit,
		Order: logstore.OrderDesc,
	})
	if err != nil {
		return nil, fmt.Errorf("agent/flowcraft: query history: %w", err)
	}
	messages := make([]flowmodel.Message, 0, len(page.Records))
	for _, record := range page.Records {
		if record.Attributes["schema_version"] != "1" {
			return nil, fmt.Errorf("agent/flowcraft: unsupported history schema %q", record.Attributes["schema_version"])
		}
		var payload struct {
			Version int               `json:"version"`
			Message flowmodel.Message `json:"message"`
		}
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return nil, fmt.Errorf("agent/flowcraft: decode history record %q: %w", record.ID, err)
		}
		if payload.Version != historyVersion {
			return nil, fmt.Errorf("agent/flowcraft: unsupported history payload version %d", payload.Version)
		}
		messages = append(messages, payload.Message)
	}
	slices.Reverse(messages)
	return messages, nil
}

func (h *historyStore) append(ctx context.Context, messages []flowmodel.Message, interrupted bool) error {
	if h == nil || len(messages) == 0 {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	if !now.After(h.lastTime) {
		now = h.lastTime.Add(time.Nanosecond)
	}
	records := make([]logstore.Record, len(messages))
	for i, message := range messages {
		recordTime := now.Add(time.Duration(i))
		h.sequence++
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return fmt.Errorf("agent/flowcraft: create history ID: %w", err)
		}
		payload, err := json.Marshal(struct {
			Version int               `json:"version"`
			Message flowmodel.Message `json:"message"`
		}{Version: historyVersion, Message: message})
		if err != nil {
			return fmt.Errorf("agent/flowcraft: encode history: %w", err)
		}
		attributes := map[string]string{
			"workspace_name": h.workspace, "conversation_id": h.conversation, "schema_version": "1",
		}
		if interrupted {
			attributes["interrupted"] = "true"
		}
		records[i] = logstore.Record{
			ID:   fmt.Sprintf("%016x%s", uint64(recordTime.UnixNano()), hex.EncodeToString(random[:])),
			Time: recordTime, Stream: historyStream, Kind: historyKind,
			Attributes: attributes, Payload: payload,
		}
	}
	h.lastTime = records[len(records)-1].Time
	keys, err := h.store.Append(ctx, records)
	if err != nil {
		return fmt.Errorf("agent/flowcraft: append history: %w", err)
	}
	if len(keys) != len(records) {
		return fmt.Errorf("agent/flowcraft: history append returned %d keys for %d records", len(keys), len(records))
	}
	return nil
}

type pulledHistory struct {
	history *conversationHistory
	memory  memory.Store
	mu      sync.Mutex
	states  map[string]*pulledState
	users   map[string]string
	report  func(error)
}

type pulledState struct {
	content   strings.Builder
	committed bool
}

func newPulledHistory(history *conversationHistory, memoryStore memory.Store, report func(error)) *pulledHistory {
	return &pulledHistory{history: history, memory: memoryStore, states: make(map[string]*pulledState), users: make(map[string]string), report: report}
}

func (p *pulledHistory) track(streamID, user string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.users[streamID] = user
	p.mu.Unlock()
}

func (p *pulledHistory) observe(chunk *genx.MessageChunk) {
	if p == nil || chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil {
		return
	}
	p.mu.Lock()
	state := p.states[chunk.Ctrl.StreamID]
	if state == nil {
		state = &pulledState{}
		p.states[chunk.Ctrl.StreamID] = state
	}
	if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
		state.content.WriteString(string(text))
	}
	commit := chunk.IsEndOfStream() && !state.committed
	content := state.content.String()
	user := p.users[chunk.Ctrl.StreamID]
	interrupted := chunk.Ctrl.Error == commonagent.Interrupted
	if commit {
		state.committed = true
	}
	p.mu.Unlock()
	if commit {
		p.append(content, interrupted)
		if !interrupted {
			p.observeMemory(chunk.Ctrl.StreamID, user, content)
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
		p.reportError(fmt.Errorf("agent/flowcraft: observe memory: %w", err))
		return
	}
	if result.Operation != nil && result.Operation.Status == memory.OperationFailed {
		p.reportError(fmt.Errorf("agent/flowcraft: memory operation %q failed: %s", result.Operation.ID, result.Operation.Error))
	}
}

func (p *pulledHistory) commitInterrupted(streamID string) {
	if p == nil || streamID == "" {
		return
	}
	p.mu.Lock()
	state := p.states[streamID]
	if state == nil {
		state = &pulledState{}
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
	err := p.history.append(context.Background(), []flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleAssistant, content)}, interrupted)
	if err != nil {
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
	slog.Error("flowcraft agent background operation failed", "error", err)
}
