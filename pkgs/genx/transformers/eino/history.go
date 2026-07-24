package eino

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/cloudwego/eino/schema"
)

const (
	historyStream  = "eino.history"
	historyKind    = "message"
	historyVersion = 1
)

type conversationHistory struct {
	config    *HistoryConfig
	agentID   string
	contextID string

	mu       sync.Mutex
	live     []*schema.Message
	lastTime time.Time
}

func (history *conversationHistory) load(ctx context.Context) ([]*schema.Message, error) {
	if history.config == nil || history.config.Store == nil {
		history.mu.Lock()
		defer history.mu.Unlock()
		messages := history.live
		limit := 50
		if history.config != nil {
			limit = history.config.Limit
		}
		if len(messages) > limit {
			messages = messages[len(messages)-limit:]
		}
		return cloneMessages(messages), nil
	}
	matchers := []logstore.AttributeMatcher{
		{Name: "agent_id", Op: logstore.MatchEqual, Value: history.agentID},
		{Name: "context_id", Op: logstore.MatchEqual, Value: history.contextID},
	}
	if history.config.Scope != "" {
		matchers = append(matchers, logstore.AttributeMatcher{
			Name: "scope", Op: logstore.MatchEqual, Value: history.config.Scope,
		})
	}
	page, err := history.config.Store.Query(ctx, logstore.Query{
		Streams: []string{historyStream}, Kinds: []string{historyKind},
		Matchers: matchers,
		Start:    time.Unix(0, 0).UTC(), End: time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC),
		Limit: history.config.Limit, Order: logstore.OrderDesc,
	})
	if err != nil {
		return nil, fmt.Errorf("eino: load History: %w", err)
	}
	sort.SliceStable(page.Records, func(i, j int) bool {
		if page.Records[i].Time.Equal(page.Records[j].Time) {
			return page.Records[i].ID < page.Records[j].ID
		}
		return page.Records[i].Time.Before(page.Records[j].Time)
	})
	messages := make([]*schema.Message, 0, len(page.Records))
	for _, record := range page.Records {
		var payload struct {
			Version int             `json:"version"`
			Message *schema.Message `json:"message"`
		}
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			return nil, fmt.Errorf("eino: decode History record %q: %w", record.ID, err)
		}
		if payload.Version != historyVersion || payload.Message == nil {
			return nil, fmt.Errorf("eino: unsupported History record %q", record.ID)
		}
		messages = append(messages, payload.Message)
	}
	return messages, nil
}

func (history *conversationHistory) append(ctx context.Context, messages []*schema.Message, interrupted bool) error {
	if len(messages) == 0 {
		return nil
	}
	owned := cloneMessages(messages)
	if history.config == nil || history.config.Store == nil {
		history.mu.Lock()
		history.live = append(history.live, owned...)
		limit := 50
		if history.config != nil {
			limit = history.config.Limit
		}
		if len(history.live) > limit {
			history.live = append([]*schema.Message(nil), history.live[len(history.live)-limit:]...)
		}
		history.mu.Unlock()
		return nil
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	now := time.Now().UTC()
	if !now.After(history.lastTime) {
		now = history.lastTime.Add(time.Nanosecond)
	}
	records := make([]logstore.Record, len(owned))
	for index, message := range owned {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return fmt.Errorf("eino: create History record ID: %w", err)
		}
		payload, err := json.Marshal(struct {
			Version int             `json:"version"`
			Message *schema.Message `json:"message"`
		}{Version: historyVersion, Message: message})
		if err != nil {
			return fmt.Errorf("eino: encode History: %w", err)
		}
		recordTime := now.Add(time.Duration(index))
		attributes := map[string]string{
			"agent_id": history.agentID, "context_id": history.contextID, "schema_version": "1",
		}
		if history.config.Scope != "" {
			attributes["scope"] = history.config.Scope
		}
		if interrupted && message.Role == schema.Assistant {
			attributes["interrupted"] = "true"
		}
		records[index] = logstore.Record{
			ID:   fmt.Sprintf("%016x%s", uint64(recordTime.UnixNano()), hex.EncodeToString(suffix[:])),
			Time: recordTime, Stream: historyStream, Kind: historyKind,
			Attributes: attributes, Payload: payload,
		}
	}
	history.lastTime = records[len(records)-1].Time
	keys, err := history.config.Store.Append(ctx, records)
	if err != nil {
		return fmt.Errorf("eino: append History: %w", err)
	}
	if len(keys) != len(records) {
		return fmt.Errorf("eino: History accepted %d of %d records", len(keys), len(records))
	}
	return nil
}
