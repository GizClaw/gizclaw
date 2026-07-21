package flowcraft

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

const (
	historyStream  = "flowcraft.history"
	historyKind    = "message"
	historyVersion = 1
	historyWindow  = 50
)

type conversationHistory struct {
	store     logstore.MutableStore
	agentID   string
	contextID string

	mu       sync.Mutex
	live     []flowmodel.Message
	lastTime time.Time
}

func (h *conversationHistory) load(ctx context.Context) ([]flowmodel.Message, error) {
	if h.store == nil {
		h.mu.Lock()
		defer h.mu.Unlock()
		live := h.live
		if len(live) > historyWindow {
			live = live[len(live)-historyWindow:]
		}
		return flowmodel.CloneMessages(live), nil
	}
	page, err := h.store.Query(ctx, logstore.Query{
		Streams: []string{historyStream}, Kinds: []string{historyKind},
		Matchers: []logstore.AttributeMatcher{
			{Name: "agent_id", Op: logstore.MatchEqual, Value: h.agentID},
			{Name: "context_id", Op: logstore.MatchEqual, Value: h.contextID},
		},
		Start: time.Unix(0, 0).UTC(), End: time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC),
		Limit: logstore.MaxLimit, Order: logstore.OrderDesc,
	})
	if err != nil {
		return nil, fmt.Errorf("flowcraft: load History: %w", err)
	}
	sort.SliceStable(page.Records, func(i, j int) bool {
		if page.Records[i].Time.Equal(page.Records[j].Time) {
			return page.Records[i].ID < page.Records[j].ID
		}
		return page.Records[i].Time.Before(page.Records[j].Time)
	})
	if len(page.Records) > historyWindow {
		page.Records = page.Records[len(page.Records)-historyWindow:]
	}
	messages := make([]flowmodel.Message, 0, len(page.Records))
	for _, record := range page.Records {
		var payload struct {
			Version int               `json:"version"`
			Message flowmodel.Message `json:"message"`
		}
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return nil, fmt.Errorf("flowcraft: decode History record %q: %w", record.ID, err)
		}
		if payload.Version != historyVersion {
			return nil, fmt.Errorf("flowcraft: unsupported History version %d", payload.Version)
		}
		messages = append(messages, payload.Message)
	}
	return messages, nil
}

func (h *conversationHistory) append(ctx context.Context, messages []flowmodel.Message, interrupted bool) error {
	if len(messages) == 0 {
		return nil
	}
	owned := flowmodel.CloneMessages(messages)
	if interrupted {
		for index := range owned {
			if owned[index].Role != flowmodel.RoleAssistant {
				continue
			}
			owned[index].Parts = append(owned[index].Parts, flowmodel.Part{Type: flowmodel.PartData, Data: &flowmodel.DataRef{
				MimeType: "application/vnd.genx.interruption+json", Value: map[string]any{"interrupted": true},
			}})
		}
	}
	if h.store == nil {
		h.mu.Lock()
		h.live = append(h.live, owned...)
		h.mu.Unlock()
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	if !now.After(h.lastTime) {
		now = h.lastTime.Add(time.Nanosecond)
	}
	records := make([]logstore.Record, len(owned))
	for index, message := range owned {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return fmt.Errorf("flowcraft: create History record ID: %w", err)
		}
		payload, err := json.Marshal(struct {
			Version int               `json:"version"`
			Message flowmodel.Message `json:"message"`
		}{Version: historyVersion, Message: message})
		if err != nil {
			return fmt.Errorf("flowcraft: encode History: %w", err)
		}
		recordTime := now.Add(time.Duration(index))
		attributes := map[string]string{"agent_id": h.agentID, "context_id": h.contextID, "schema_version": "1"}
		if interrupted && message.Role == flowmodel.RoleAssistant {
			attributes["interrupted"] = "true"
		}
		records[index] = logstore.Record{
			ID:   fmt.Sprintf("%016x%s", uint64(recordTime.UnixNano()), hex.EncodeToString(suffix[:])),
			Time: recordTime, Stream: historyStream, Kind: historyKind, Attributes: attributes, Payload: payload,
		}
	}
	h.lastTime = records[len(records)-1].Time
	keys, err := h.store.Append(ctx, records)
	if err != nil {
		return fmt.Errorf("flowcraft: append History: %w", err)
	}
	if len(keys) != len(records) {
		return fmt.Errorf("flowcraft: History accepted %d of %d records", len(keys), len(records))
	}
	return nil
}
