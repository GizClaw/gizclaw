package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const (
	defaultHistoryListLimit = 50
	maxHistoryListLimit     = 200
	historyEntryTypeGear    = "gear"
	historyEntryTypeAgent   = "agent"
	historyRecordVersion    = 1
	historyStreamPrefix     = "workspace.history/"
	// HistoryOriginAgentHost marks entries durably written by the authenticated
	// AgentHost path. Missing or other origins are never reward-eligible.
	HistoryOriginAgentHost = "agenthost"
	// HistoryOriginOpenAI marks user turns accepted by the OpenAI Responses adapter.
	HistoryOriginOpenAI = "openai.responses"
)

var historyIDSeq uint64

// HistoryStore persists structured workspace history in a LogStore and keeps
// only binary assets in object storage.
type HistoryStore struct {
	Records      logstore.MutableRecordStore
	Objects      objectstore.ObjectStore
	Workspace    string
	ObjectPrefix string
	Now          func() time.Time
}

// HistoryEntry is the internal persisted history shape.
type HistoryEntry struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	GearID          string         `json:"gear_id,omitempty"`
	Origin          string         `json:"origin,omitempty"`
	Name            string         `json:"name"`
	Text            string         `json:"text"`
	CreatedAt       time.Time      `json:"created_at"`
	ReplayAvailable bool           `json:"replay_available"`
	Assets          []HistoryAsset `json:"assets,omitempty"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
}

// HistoryAsset references a stored history asset.
type HistoryAsset struct {
	Name      string     `json:"name"`
	MIMEType  string     `json:"mime_type"`
	Bytes     int64      `json:"bytes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AppendHistoryRequest describes one entry to append.
type AppendHistoryRequest struct {
	Type      string
	GearID    string
	Origin    string
	Name      string
	Text      string
	CreatedAt time.Time
	Asset     *AppendHistoryAsset
}

// AppendHistoryAsset is an optional binary asset attached to a history entry.
type AppendHistoryAsset struct {
	MIMEType string
	Data     []byte
}

// NewHistoryStore constructs a HistoryStore for one workspace runtime.
func NewHistoryStore(records logstore.MutableRecordStore, objects objectstore.ObjectStore, workspace string) *HistoryStore {
	return &HistoryStore{
		Records:      records,
		Objects:      objects,
		Workspace:    workspace,
		ObjectPrefix: ObjectPrefix(workspace),
	}
}

func (s *HistoryStore) Append(ctx context.Context, req AppendHistoryRequest) (HistoryEntry, error) {
	if err := ctxErr(ctx); err != nil {
		return HistoryEntry{}, err
	}
	if err := s.validate(); err != nil {
		return HistoryEntry{}, err
	}
	now := s.now()
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	entry := HistoryEntry{
		ID:        historyID(createdAt, now),
		Type:      strings.TrimSpace(req.Type),
		GearID:    strings.TrimSpace(req.GearID),
		Origin:    strings.TrimSpace(req.Origin),
		Name:      strings.TrimSpace(req.Name),
		Text:      req.Text,
		CreatedAt: createdAt.UTC(),
	}
	if entry.Type == "" {
		entry.Type = historyEntryTypeAgent
	}
	if entry.Name == "" {
		entry.Name = entry.Type
	}
	if err := validateHistoryEntry(entry); err != nil {
		return HistoryEntry{}, err
	}
	var written []string
	hasReplayContent := false
	if req.Asset != nil && len(req.Asset.Data) > 0 {
		asset, err := s.writeAsset(entry.ID, *req.Asset)
		if err != nil {
			return HistoryEntry{}, err
		}
		written = append(written, asset.Name)
		entry.Assets = append(entry.Assets, asset)
		hasReplayContent = true
	}
	if strings.TrimSpace(entry.Text) != "" {
		hasReplayContent = true
	}
	entry.ReplayAvailable = hasReplayContent
	record, err := historyRecord(entry, s.stream())
	if err != nil {
		for _, name := range written {
			_ = s.Objects.Delete(name)
		}
		return HistoryEntry{}, err
	}
	keys, err := s.Records.Append(ctx, []logstore.Record{record})
	if err != nil {
		for _, name := range written {
			_ = s.Objects.Delete(name)
		}
		return HistoryEntry{}, fmt.Errorf("workspace history: append record: %w", err)
	}
	if len(keys) != 1 || keys[0] != record.Key() {
		for _, name := range written {
			_ = s.Objects.Delete(name)
		}
		return HistoryEntry{}, fmt.Errorf("workspace history: LogStore accepted %d of 1 records", len(keys))
	}
	return entry, nil
}

func (s *HistoryStore) List(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	entries, hasNext, nextCursor, err := s.listInternal(ctx, req)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	items := make([]apitypes.PeerRunHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entry.Public())
	}
	return apitypes.PeerRunHistoryListResponse{
		Available:  true,
		Items:      items,
		HasNext:    hasNext,
		NextCursor: nextCursor,
	}, nil
}

// HistoryEntryPage is an internal-history page with origin and authoritative
// entry identity preserved. It is not an HTTP or RPC response type.
type HistoryEntryPage struct {
	Entries    []HistoryEntry
	HasNext    bool
	NextCursor string
}

// ListPage returns one authoritative internal History page with the same
// ordering, cursor, and limit semantics as the public History list.
func (s *HistoryStore) ListPage(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (HistoryEntryPage, error) {
	entries, hasNext, next, err := s.listInternal(ctx, req)
	if err != nil {
		return HistoryEntryPage{}, err
	}
	nextCursor := ""
	if next != nil {
		nextCursor = *next
	}
	return HistoryEntryPage{Entries: entries, HasNext: hasNext, NextCursor: nextCursor}, nil
}

// ListEntries returns internal persisted entries in ascending ID order after
// the exclusive cursor and, when provided, through the inclusive high-water.
func (s *HistoryStore) ListEntries(ctx context.Context, after, through string, limit int) (HistoryEntryPage, error) {
	after = strings.TrimSpace(after)
	through = strings.TrimSpace(through)
	order := apitypes.PeerRunHistoryListRequestOrderAsc
	req := apitypes.PeerRunHistoryListRequest{Order: &order}
	if after != "" {
		req.Cursor = &after
	}
	if limit > 0 {
		req.Limit = &limit
	}
	entries, hasNext, next, err := s.listInternal(ctx, req)
	if err != nil {
		return HistoryEntryPage{}, err
	}
	if through != "" {
		for i, entry := range entries {
			if entry.ID > through {
				entries = entries[:i]
				hasNext = false
				next = nil
				break
			}
		}
		if len(entries) > 0 && entries[len(entries)-1].ID == through {
			hasNext = false
			next = nil
		}
	}
	nextCursor := ""
	if next != nil {
		nextCursor = *next
	} else if hasNext && len(entries) > 0 {
		nextCursor = entries[len(entries)-1].ID
	}
	return HistoryEntryPage{Entries: entries, HasNext: hasNext, NextCursor: nextCursor}, nil
}

// LatestEntry returns the newest retained internal History entry.
func (s *HistoryStore) LatestEntry(ctx context.Context) (HistoryEntry, bool, error) {
	order := apitypes.PeerRunHistoryListRequestOrderDesc
	limit := 1
	entries, _, _, err := s.listInternal(ctx, apitypes.PeerRunHistoryListRequest{
		Limit: &limit,
		Order: &order,
	})
	if err != nil {
		return HistoryEntry{}, false, err
	}
	if len(entries) == 0 {
		return HistoryEntry{}, false, nil
	}
	return entries[0], true, nil
}

// LatestEntryBefore returns the newest retained entry strictly before the
// supplied activation boundary. It lets a new post-processor establish a
// non-retroactive checkpoint without losing entries appended after activation.
func (s *HistoryStore) LatestEntryBefore(ctx context.Context, before time.Time) (HistoryEntry, bool, error) {
	if before.IsZero() {
		return HistoryEntry{}, false, fmt.Errorf("workspace history: before is required")
	}
	order := apitypes.PeerRunHistoryListRequestOrderDesc
	limit := maxHistoryListLimit
	var cursor *string
	for {
		entries, hasNext, next, err := s.listInternal(ctx, apitypes.PeerRunHistoryListRequest{
			Cursor: cursor,
			Limit:  &limit,
			Order:  &order,
		})
		if err != nil {
			return HistoryEntry{}, false, err
		}
		for _, entry := range entries {
			if entry.CreatedAt.Before(before) {
				return entry, true, nil
			}
		}
		if !hasNext || next == nil {
			return HistoryEntry{}, false, nil
		}
		cursor = next
	}
}

func (s *HistoryStore) Get(ctx context.Context, id string) (HistoryEntry, error) {
	if err := ctxErr(ctx); err != nil {
		return HistoryEntry{}, err
	}
	if err := s.validate(); err != nil {
		return HistoryEntry{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return HistoryEntry{}, fmt.Errorf("workspace history: history_id is required")
	}
	record, err := s.Records.Get(ctx, logstore.RecordKey{Stream: s.stream(), ID: id})
	if errors.Is(err, logstore.ErrNotFound) {
		return HistoryEntry{}, fs.ErrNotExist
	}
	if err != nil {
		return HistoryEntry{}, err
	}
	entry, err := historyEntryFromRecord(record, s.stream())
	if err != nil {
		return HistoryEntry{}, err
	}
	if s.entryExpired(entry) {
		if err := s.deleteEntry(ctx, entry); err != nil {
			return HistoryEntry{}, err
		}
		return HistoryEntry{}, fs.ErrNotExist
	}
	return entry, nil
}

func (s *HistoryStore) ReadAsset(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("workspace history: asset name is required")
	}
	if !strings.HasPrefix(name, s.historyPrefix()+"/assets/") {
		return nil, fmt.Errorf("workspace history: asset %q is outside workspace history", name)
	}
	return s.Objects.Get(name)
}

func (s *HistoryStore) CleanupExpired(ctx context.Context) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if err := s.validate(); err != nil {
		return err
	}
	return s.scanRecords(ctx, logstore.OrderAsc, func(entry HistoryEntry) error {
		if entry.ExpiresAt == nil || s.now().Before(*entry.ExpiresAt) {
			return nil
		}
		return s.deleteEntry(ctx, entry)
	})
}

// DeleteAll removes every history record and referenced asset for this
// Workspace. It is idempotent and scoped to the Workspace history stream.
func (s *HistoryStore) DeleteAll(ctx context.Context) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if err := s.validate(); err != nil {
		return err
	}
	return s.scanRecords(ctx, logstore.OrderAsc, func(entry HistoryEntry) error {
		return s.deleteEntry(ctx, entry)
	})
}

// Empty reports whether the Workspace has any retained history records.
func (s *HistoryStore) Empty(ctx context.Context) (bool, error) {
	limit := 1
	page, err := s.ListPage(ctx, apitypes.PeerRunHistoryListRequest{Limit: &limit})
	if err != nil {
		return false, err
	}
	return len(page.Entries) == 0, nil
}

func (e HistoryEntry) Public() apitypes.PeerRunHistoryEntry {
	item := apitypes.PeerRunHistoryEntry{
		Name:            e.ID,
		Type:            apitypes.PeerRunHistoryEntryType(e.Type),
		ActorName:       e.Name,
		Text:            e.Text,
		CreatedAt:       e.CreatedAt,
		ReplayAvailable: e.ReplayAvailable,
	}
	if e.GearID != "" {
		item.GearId = &e.GearID
	}
	return item
}

func (s *HistoryStore) listInternal(ctx context.Context, req apitypes.PeerRunHistoryListRequest) ([]HistoryEntry, bool, *string, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, false, nil, err
	}
	if err := s.validate(); err != nil {
		return nil, false, nil, err
	}
	limit := defaultHistoryListLimit
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit <= 0 {
		limit = defaultHistoryListLimit
	}
	if limit > maxHistoryListLimit {
		limit = maxHistoryListLimit
	}
	cursor := ""
	if req.Cursor != nil {
		cursor = strings.TrimSpace(*req.Cursor)
	}
	order := apitypes.PeerRunHistoryListRequestOrderAsc
	if req.Order != nil {
		order = *req.Order
	}
	if !order.Valid() {
		return nil, false, nil, fmt.Errorf("workspace history: unsupported order %q", order)
	}
	queryOrder := logstore.OrderAsc
	if order == apitypes.PeerRunHistoryListRequestOrderDesc {
		queryOrder = logstore.OrderDesc
	}
	start, end, err := historyQueryBounds(cursor, queryOrder)
	if err != nil {
		return nil, false, nil, err
	}
	out := make([]HistoryEntry, 0, limit)
	storeCursor := ""
	for {
		page, err := s.Records.Query(ctx, logstore.Query{
			Streams: []string{s.stream()},
			Kinds:   []string{historyEntryTypeGear, historyEntryTypeAgent},
			Start:   start,
			End:     end,
			Limit:   logstore.MaxLimit,
			Order:   queryOrder,
			Cursor:  storeCursor,
		})
		if err != nil {
			return nil, false, nil, fmt.Errorf("workspace history: query records: %w", err)
		}
		for _, record := range page.Records {
			entry, err := historyEntryFromRecord(record, s.stream())
			if err != nil {
				return nil, false, nil, err
			}
			if cursor != "" {
				if queryOrder == logstore.OrderDesc && entry.ID >= cursor {
					continue
				}
				if queryOrder == logstore.OrderAsc && entry.ID <= cursor {
					continue
				}
			}
			if s.entryExpired(entry) {
				if err := s.deleteEntry(ctx, entry); err != nil {
					return nil, false, nil, err
				}
				continue
			}
			out = append(out, entry)
			if len(out) == limit+1 {
				next := out[limit-1].ID
				return out[:limit], true, &next, nil
			}
		}
		if !page.HasNext {
			return out, false, nil, nil
		}
		storeCursor = page.NextCursor
	}
}

func (s *HistoryStore) writeAsset(id string, asset AppendHistoryAsset) (HistoryAsset, error) {
	name := s.assetObjectName(id, asset.MIMEType)
	if err := s.Objects.Put(name, bytes.NewReader(asset.Data)); err != nil {
		return HistoryAsset{}, fmt.Errorf("workspace history: write asset: %w", err)
	}
	return HistoryAsset{
		Name:     name,
		MIMEType: strings.TrimSpace(asset.MIMEType),
		Bytes:    int64(len(asset.Data)),
	}, nil
}

type historyPayload struct {
	Version         int            `json:"version"`
	GearID          string         `json:"gear_id,omitempty"`
	Origin          string         `json:"origin,omitempty"`
	Name            string         `json:"name"`
	ReplayAvailable bool           `json:"replay_available"`
	Assets          []HistoryAsset `json:"assets,omitempty"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
}

func historyRecord(entry HistoryEntry, stream string) (logstore.Record, error) {
	payload, err := json.Marshal(historyPayload{
		Version: historyRecordVersion, GearID: entry.GearID, Origin: entry.Origin,
		Name: entry.Name, ReplayAvailable: entry.ReplayAvailable,
		Assets: entry.Assets, ExpiresAt: entry.ExpiresAt,
	})
	if err != nil {
		return logstore.Record{}, fmt.Errorf("workspace history: encode record: %w", err)
	}
	attributes := map[string]string{
		"actor_name":     entry.Name,
		"schema_version": strconv.Itoa(historyRecordVersion),
	}
	if entry.GearID != "" {
		attributes["gear_id"] = entry.GearID
	}
	if entry.Origin != "" {
		attributes["origin"] = entry.Origin
	}
	record := logstore.Record{
		ID: entry.ID, Time: entry.CreatedAt, Stream: stream, Kind: entry.Type,
		Message: entry.Text, Attributes: attributes, Payload: payload,
	}
	if err := logstore.ValidateRecord(record); err != nil {
		return logstore.Record{}, fmt.Errorf("workspace history: encode record: %w", err)
	}
	return record, nil
}

func historyEntryFromRecord(record logstore.Record, stream string) (HistoryEntry, error) {
	if record.Stream != stream {
		return HistoryEntry{}, fmt.Errorf("workspace history: record %q belongs to stream %q", record.ID, record.Stream)
	}
	var payload historyPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return HistoryEntry{}, fmt.Errorf("workspace history: decode record %q: %w", record.ID, err)
	}
	if payload.Version != historyRecordVersion {
		return HistoryEntry{}, fmt.Errorf("workspace history: record %q has unsupported version %d", record.ID, payload.Version)
	}
	entry := HistoryEntry{
		ID: record.ID, Type: record.Kind, GearID: payload.GearID,
		Origin: payload.Origin, Name: payload.Name, Text: record.Message,
		CreatedAt: record.Time.UTC(), ReplayAvailable: payload.ReplayAvailable,
		Assets: payload.Assets, ExpiresAt: payload.ExpiresAt,
	}
	if err := validateHistoryEntry(entry); err != nil {
		return HistoryEntry{}, fmt.Errorf("workspace history: decode record %q: %w", record.ID, err)
	}
	return entry, nil
}

func historyQueryBounds(cursor string, order logstore.Order) (time.Time, time.Time, error) {
	start := time.Unix(0, 0).UTC()
	end := time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC)
	if cursor == "" {
		return start, end, nil
	}
	boundary, err := historyIDTime(cursor)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("workspace history: invalid cursor: %w", err)
	}
	if order == logstore.OrderDesc {
		end = boundary.Truncate(time.Millisecond).Add(time.Millisecond)
	} else {
		start = boundary.Truncate(time.Millisecond)
	}
	return start, end, nil
}

func historyIDTime(id string) (time.Time, error) {
	const timestampLength = len("20060102T150405.000000000Z")
	if len(id) <= timestampLength || id[timestampLength] != '-' {
		return time.Time{}, fmt.Errorf("invalid history ID %q", id)
	}
	parsed, err := time.Parse("20060102T150405.000000000Z", id[:timestampLength])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid history ID %q", id)
	}
	return parsed.UTC(), nil
}

func (s *HistoryStore) scanRecords(ctx context.Context, order logstore.Order, visit func(HistoryEntry) error) error {
	cursor := ""
	for {
		page, err := s.Records.Query(ctx, logstore.Query{
			Streams: []string{s.stream()},
			Kinds:   []string{historyEntryTypeGear, historyEntryTypeAgent},
			Start:   time.Unix(0, 0).UTC(),
			End:     time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC),
			Limit:   logstore.MaxLimit,
			Order:   order,
			Cursor:  cursor,
		})
		if err != nil {
			return fmt.Errorf("workspace history: query records: %w", err)
		}
		for _, record := range page.Records {
			entry, err := historyEntryFromRecord(record, s.stream())
			if err != nil {
				return err
			}
			if err := visit(entry); err != nil {
				return err
			}
		}
		if !page.HasNext {
			return nil
		}
		cursor = page.NextCursor
	}
}

func (s *HistoryStore) deleteEntry(ctx context.Context, entry HistoryEntry) error {
	for _, asset := range entry.Assets {
		if err := s.Objects.Delete(asset.Name); err != nil && !isNotExist(err) {
			return err
		}
	}
	if err := s.Records.Delete(ctx, logstore.RecordKey{Stream: s.stream(), ID: entry.ID}); err != nil && !errors.Is(err, logstore.ErrNotFound) {
		return err
	}
	return nil
}

func (s *HistoryStore) entryExpired(entry HistoryEntry) bool {
	return entry.ExpiresAt != nil && !s.now().Before(*entry.ExpiresAt)
}

func (s *HistoryStore) validate() error {
	if s == nil || s.Records == nil {
		return fmt.Errorf("workspace history: mutable record store is required")
	}
	if s.Objects == nil {
		return fmt.Errorf("workspace history: object store is required")
	}
	if strings.TrimSpace(s.ObjectPrefix) == "" {
		return fmt.Errorf("workspace history: object prefix is required")
	}
	return nil
}

func validateHistoryEntry(entry HistoryEntry) error {
	if strings.TrimSpace(entry.ID) == "" {
		return fmt.Errorf("id is required")
	}
	switch entry.Type {
	case historyEntryTypeGear:
		if strings.TrimSpace(entry.GearID) == "" {
			return fmt.Errorf("gear_id is required for gear history")
		}
	case historyEntryTypeAgent:
		if strings.TrimSpace(entry.GearID) != "" {
			return fmt.Errorf("gear_id must be empty for agent history")
		}
	default:
		return fmt.Errorf("unsupported history type %q", entry.Type)
	}
	if strings.TrimSpace(entry.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(entry.Origin) > 64 {
		return fmt.Errorf("origin must be at most 64 bytes")
	}
	if entry.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	return nil
}

func (s *HistoryStore) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *HistoryStore) historyPrefix() string {
	return strings.Trim(strings.TrimSpace(s.ObjectPrefix), "/") + "/history"
}

func (s *HistoryStore) stream() string {
	return historyStreamPrefix + strings.TrimSpace(s.Workspace)
}

func (s *HistoryStore) assetObjectName(id string, mimeType string) string {
	ext := "bin"
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "audio/opus":
		ext = "opus"
	case "audio/ogg", "audio/ogg; codecs=opus":
		ext = "ogg"
	case "audio/mpeg", "audio/mp3":
		ext = "mp3"
	}
	return s.historyPrefix() + "/assets/" + url.PathEscape(id) + "/audio." + ext
}

func historyID(createdAt, now time.Time) string {
	if createdAt.IsZero() {
		createdAt = now
	}
	seq := atomic.AddUint64(&historyIDSeq, 1)
	return createdAt.UTC().Format("20060102T150405.000000000Z") + "-" + strconv.FormatUint(seq, 36)
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
