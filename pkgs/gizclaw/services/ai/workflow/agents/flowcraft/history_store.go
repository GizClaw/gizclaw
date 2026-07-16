package flowcraft

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	memoryhistory "github.com/GizClaw/flowcraft/memory/history"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

const (
	flowcraftHistoryStream        = "flowcraft-history"
	flowcraftHistoryKind          = "message"
	flowcraftHistorySchemaVersion = 1
	flowcraftHistoryPageLimit     = logstore.MaxLimit
)

var (
	flowcraftHistoryStart = time.Unix(0, 0).UTC()
	flowcraftHistoryEnd   = time.Date(2262, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// HistoryStore adapts one workspace scope of a MutableStore to Flowcraft
// short-term message history. It does not own or close the underlying store.
type HistoryStore struct {
	store     logstore.MutableStore
	workspace string
	now       func() time.Time
	mutation  sync.Mutex
}

// NewHistoryStore creates a workspace-scoped Flowcraft history adapter.
func NewHistoryStore(store logstore.MutableStore, workspaceName string) (*HistoryStore, error) {
	if store == nil {
		return nil, errors.New("flowcraft: history store is nil")
	}
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return nil, errors.New("flowcraft: history workspace name is required")
	}
	return &HistoryStore{store: store, workspace: workspaceName, now: time.Now}, nil
}

// GetMessages returns every message in chronological record order.
func (store *HistoryStore) GetMessages(ctx context.Context, conversationID string) ([]flowmodel.Message, error) {
	records, err := store.records(ctx, conversationID, logstore.OrderAsc, 0)
	if err != nil {
		return nil, err
	}
	return store.decodeRecords(conversationID, records)
}

// GetRecentMessages returns at most limit messages in chronological order.
func (store *HistoryStore) GetRecentMessages(ctx context.Context, conversationID string, limit int) ([]flowmodel.Message, error) {
	if limit <= 0 {
		return []flowmodel.Message{}, nil
	}
	records, err := store.records(ctx, conversationID, logstore.OrderDesc, limit)
	if err != nil {
		return nil, err
	}
	messages, err := store.decodeRecords(conversationID, records)
	if err != nil {
		return nil, err
	}
	slices.Reverse(messages)
	return messages, nil
}

// AppendMessages appends only the supplied new messages.
func (store *HistoryStore) AppendMessages(ctx context.Context, conversationID string, messages []flowmodel.Message) error {
	if len(messages) == 0 {
		return nil
	}
	conversationID, err := validateHistoryConversation(conversationID)
	if err != nil {
		return err
	}
	store.mutation.Lock()
	defer store.mutation.Unlock()
	existing, err := store.records(ctx, conversationID, logstore.OrderDesc, 1)
	if err != nil {
		return err
	}
	var after time.Time
	if len(existing) > 0 {
		after = existing[0].Time
	}
	return store.appendMessagesAfter(ctx, conversationID, messages, after)
}

func (store *HistoryStore) appendMessagesAfter(
	ctx context.Context,
	conversationID string,
	messages []flowmodel.Message,
	after time.Time,
) error {
	if len(messages) == 0 {
		return nil
	}
	conversationID, err := validateHistoryConversation(conversationID)
	if err != nil {
		return err
	}
	records, err := store.encodeMessages(conversationID, messages, after)
	if err != nil {
		return fmt.Errorf("flowcraft: encode history append for workspace %q conversation %q: %w", store.workspace, conversationID, err)
	}
	keys, err := store.store.Append(ctx, records)
	if err != nil {
		return fmt.Errorf("flowcraft: append history for workspace %q conversation %q: %w", store.workspace, conversationID, err)
	}
	if len(keys) != len(records) {
		return fmt.Errorf(
			"flowcraft: append history for workspace %q conversation %q returned %d keys for %d records",
			store.workspace,
			conversationID,
			len(keys),
			len(records),
		)
	}
	for index, key := range keys {
		if key != records[index].Key() {
			return fmt.Errorf(
				"flowcraft: append history for workspace %q conversation %q returned mismatched key at index %d",
				store.workspace,
				conversationID,
				index,
			)
		}
	}
	return nil
}

// SaveMessages converges the stored records on the complete supplied slice.
func (store *HistoryStore) SaveMessages(ctx context.Context, conversationID string, messages []flowmodel.Message) error {
	conversationID, err := validateHistoryConversation(conversationID)
	if err != nil {
		return err
	}
	store.mutation.Lock()
	defer store.mutation.Unlock()
	existing, err := store.records(ctx, conversationID, logstore.OrderAsc, 0)
	if err != nil {
		return err
	}
	overlap := min(len(existing), len(messages))
	for index := range overlap {
		record, err := store.encodeMessage(conversationID, messages[index], existing[index].ID, existing[index].Time)
		if err != nil {
			return fmt.Errorf(
				"flowcraft: encode history replacement for workspace %q conversation %q index %d: %w",
				store.workspace,
				conversationID,
				index,
				err,
			)
		}
		if err := store.store.Replace(ctx, record); err != nil {
			return fmt.Errorf(
				"flowcraft: replace history for workspace %q conversation %q index %d: %w",
				store.workspace,
				conversationID,
				index,
				err,
			)
		}
	}
	for index := overlap; index < len(existing); index++ {
		if err := store.store.Delete(ctx, existing[index].Key()); err != nil && !errors.Is(err, logstore.ErrNotFound) {
			return fmt.Errorf(
				"flowcraft: delete surplus history for workspace %q conversation %q index %d: %w",
				store.workspace,
				conversationID,
				index,
				err,
			)
		}
	}
	if len(messages) > overlap {
		var after time.Time
		if len(existing) > 0 {
			after = existing[len(existing)-1].Time
		}
		if err := store.appendMessagesAfter(ctx, conversationID, messages[overlap:], after); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMessages removes every record in the conversation scope.
func (store *HistoryStore) DeleteMessages(ctx context.Context, conversationID string) error {
	conversationID, err := validateHistoryConversation(conversationID)
	if err != nil {
		return err
	}
	store.mutation.Lock()
	defer store.mutation.Unlock()
	records, err := store.records(ctx, conversationID, logstore.OrderAsc, 0)
	if err != nil {
		return err
	}
	for index, record := range records {
		if err := store.store.Delete(ctx, record.Key()); err != nil && !errors.Is(err, logstore.ErrNotFound) {
			return fmt.Errorf(
				"flowcraft: delete history for workspace %q conversation %q index %d: %w",
				store.workspace,
				conversationID,
				index,
				err,
			)
		}
	}
	return nil
}

func (store *HistoryStore) records(
	ctx context.Context,
	conversationID string,
	order logstore.Order,
	maxRecords int,
) ([]logstore.Record, error) {
	conversationID, err := validateHistoryConversation(conversationID)
	if err != nil {
		return nil, err
	}
	if store == nil || store.store == nil {
		return nil, errors.New("flowcraft: history store is not initialized")
	}
	query := logstore.Query{
		Streams: []string{flowcraftHistoryStream},
		Kinds:   []string{flowcraftHistoryKind},
		Matchers: []logstore.AttributeMatcher{
			{Name: "workspace_name", Op: logstore.MatchEqual, Value: store.workspace},
			{Name: "conversation_id", Op: logstore.MatchEqual, Value: conversationID},
		},
		Start: flowcraftHistoryStart,
		End:   flowcraftHistoryEnd,
		Limit: flowcraftHistoryPageLimit,
		Order: order,
	}
	records := make([]logstore.Record, 0)
	seenKeys := make(map[logstore.RecordKey]struct{})
	for {
		page, err := store.store.Query(ctx, query)
		if err != nil {
			return nil, fmt.Errorf(
				"flowcraft: query history for workspace %q conversation %q: %w",
				store.workspace,
				conversationID,
				err,
			)
		}
		if err := logstore.ValidatePage(page, query.Limit); err != nil {
			return nil, fmt.Errorf(
				"flowcraft: query history for workspace %q conversation %q returned an invalid page: %w",
				store.workspace,
				conversationID,
				err,
			)
		}
		for _, record := range page.Records {
			if err := store.validateRecordScope(conversationID, record); err != nil {
				return nil, err
			}
			if _, duplicate := seenKeys[record.Key()]; duplicate {
				return nil, fmt.Errorf(
					"flowcraft: duplicate history key for workspace %q conversation %q: stream %q id %q",
					store.workspace,
					conversationID,
					record.Stream,
					record.ID,
				)
			}
			seenKeys[record.Key()] = struct{}{}
			records = append(records, record)
			if maxRecords > 0 && len(records) >= maxRecords {
				return records[:maxRecords], nil
			}
		}
		if !page.HasNext {
			return records, nil
		}
		if page.NextCursor == query.Cursor {
			return nil, fmt.Errorf(
				"flowcraft: history query for workspace %q conversation %q returned a non-advancing cursor",
				store.workspace,
				conversationID,
			)
		}
		query.Cursor = page.NextCursor
	}
}

func (store *HistoryStore) encodeMessages(
	conversationID string,
	messages []flowmodel.Message,
	after time.Time,
) ([]logstore.Record, error) {
	now := time.Now
	if store.now != nil {
		now = store.now
	}
	base := now().UTC()
	if !after.IsZero() && !base.After(after) {
		base = after.UTC().Add(time.Nanosecond)
	}
	records := make([]logstore.Record, len(messages))
	for index, message := range messages {
		recordTime := base.Add(time.Duration(index))
		id, err := newHistoryRecordID(recordTime)
		if err != nil {
			return nil, err
		}
		record, err := store.encodeMessage(conversationID, message, id, recordTime)
		if err != nil {
			return nil, err
		}
		records[index] = record
	}
	return records, nil
}

func (store *HistoryStore) encodeMessage(
	conversationID string,
	message flowmodel.Message,
	id string,
	recordTime time.Time,
) (logstore.Record, error) {
	payload, err := json.Marshal(struct {
		Version int               `json:"version"`
		Message flowmodel.Message `json:"message"`
	}{
		Version: flowcraftHistorySchemaVersion,
		Message: message,
	})
	if err != nil {
		return logstore.Record{}, err
	}
	record := logstore.Record{
		ID:      id,
		Time:    recordTime.UTC(),
		Stream:  flowcraftHistoryStream,
		Kind:    flowcraftHistoryKind,
		Message: "",
		Attributes: map[string]string{
			"workspace_name":  store.workspace,
			"conversation_id": conversationID,
			"schema_version":  "1",
		},
		Payload: payload,
	}
	if err := logstore.ValidateRecord(record); err != nil {
		return logstore.Record{}, err
	}
	return record, nil
}

func (store *HistoryStore) decodeRecords(
	conversationID string,
	records []logstore.Record,
) ([]flowmodel.Message, error) {
	messages := make([]flowmodel.Message, len(records))
	for index, record := range records {
		message, err := decodeHistoryMessage(record.Payload)
		if err != nil {
			return nil, fmt.Errorf(
				"flowcraft: decode history for workspace %q conversation %q record %q: %w",
				store.workspace,
				conversationID,
				record.ID,
				err,
			)
		}
		messages[index] = message
	}
	return messages, nil
}

func (store *HistoryStore) validateRecordScope(conversationID string, record logstore.Record) error {
	if err := logstore.ValidateRecord(record); err != nil {
		return fmt.Errorf(
			"flowcraft: invalid history record for workspace %q conversation %q: %w",
			store.workspace,
			conversationID,
			err,
		)
	}
	if record.Stream != flowcraftHistoryStream ||
		record.Kind != flowcraftHistoryKind ||
		record.Attributes["workspace_name"] != store.workspace ||
		record.Attributes["conversation_id"] != conversationID {
		return fmt.Errorf(
			"flowcraft: history record %q is outside workspace %q conversation %q",
			record.ID,
			store.workspace,
			conversationID,
		)
	}
	if record.Attributes["schema_version"] != "1" {
		return fmt.Errorf(
			"flowcraft: history record %q has unsupported schema version %q",
			record.ID,
			record.Attributes["schema_version"],
		)
	}
	return nil
}

func decodeHistoryMessage(payload json.RawMessage) (flowmodel.Message, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return flowmodel.Message{}, fmt.Errorf("decode envelope: %w", err)
	}
	var version int
	if err := json.Unmarshal(envelope["version"], &version); err != nil {
		return flowmodel.Message{}, fmt.Errorf("decode version: %w", err)
	}
	if version != flowcraftHistorySchemaVersion {
		return flowmodel.Message{}, fmt.Errorf("unsupported payload version %d", version)
	}
	rawMessage, exists := envelope["message"]
	if !exists {
		return flowmodel.Message{}, errors.New("message is missing")
	}
	var message flowmodel.Message
	if err := json.Unmarshal(rawMessage, &message); err != nil {
		return flowmodel.Message{}, fmt.Errorf("decode message: %w", err)
	}
	return message, nil
}

func validateHistoryConversation(conversationID string) (string, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return "", errors.New("flowcraft: history conversation id is required")
	}
	return conversationID, nil
}

func newHistoryRecordID(observedAt time.Time) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("create history record id: %w", err)
	}
	return fmt.Sprintf("%016x%s", uint64(observedAt.UnixNano()), hex.EncodeToString(random[:])), nil
}

var (
	_ memoryhistory.Store           = (*HistoryStore)(nil)
	_ memoryhistory.MessageAppender = (*HistoryStore)(nil)
	_ memoryhistory.RecentReader    = (*HistoryStore)(nil)
)
