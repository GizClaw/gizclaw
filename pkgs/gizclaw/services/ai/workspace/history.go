package workspace

import (
	"bytes"
	"context"
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
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

// ErrInvalidHistoryCursor identifies a malformed history entry boundary.
var ErrInvalidHistoryCursor = errors.New("workspace history: invalid cursor")

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

var historyIDSeq atomic.Uint64

// HistoryStore persists structured workspace history in a memory.HistoryStore and keeps
// only binary assets in object storage.
type HistoryStore struct {
	Records      memory.HistoryStore
	Objects      objectstore.ObjectStore
	Workspace    string
	ObjectPrefix string
	Now          func() time.Time
	// RecordActivity, when set, runs after each durable Append so the owning
	// Workspace can advance its user-visible activity time. An error is
	// returned from Append together with the already stored entry.
	RecordActivity func(context.Context, HistoryEntry) error
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
	TurnID          string         `json:"turn_id,omitempty"`
	Interrupted     bool           `json:"interrupted,omitempty"`
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
	Type        string
	GearID      string
	Origin      string
	Name        string
	Text        string
	CreatedAt   time.Time
	Asset       *AppendHistoryAsset
	TurnID      string
	Interrupted bool
}

// AppendHistoryAsset is an optional binary asset attached to a history entry.
type AppendHistoryAsset struct {
	MIMEType string
	Data     []byte
}

// NewHistoryStore constructs a HistoryStore for one workspace runtime.
func NewHistoryStore(records memory.HistoryStore, objects objectstore.ObjectStore, workspace string) *HistoryStore {
	return &HistoryStore{
		Records:      records,
		Objects:      objects,
		Workspace:    workspace,
		ObjectPrefix: ObjectPrefix(workspace),
	}
}

// Append durably stores one entry and then reports it through RecordActivity.
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
		ID:          historyID(createdAt, now),
		Type:        strings.TrimSpace(req.Type),
		GearID:      strings.TrimSpace(req.GearID),
		Origin:      strings.TrimSpace(req.Origin),
		Name:        strings.TrimSpace(req.Name),
		Text:        req.Text,
		CreatedAt:   createdAt.UTC(),
		TurnID:      req.TurnID,
		Interrupted: req.Interrupted,
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
	err := s.Records.Append(ctx, []memory.HistoryEntry{s.memoryEntry(entry)})
	if err != nil {
		for _, name := range written {
			_ = s.Objects.Delete(name)
		}
		return HistoryEntry{}, fmt.Errorf("workspace history: append record: %w", err)
	}
	if s.RecordActivity != nil {
		if err := s.RecordActivity(ctx, entry); err != nil {
			return entry, fmt.Errorf("workspace history: record activity: %w", err)
		}
	}
	return entry, nil
}

func (s *HistoryStore) List(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return s.Search(ctx, req, HistoryFilter{})
}

// HistoryFilter narrows a History query. Zero fields leave that side open.
type HistoryFilter struct {
	// Text matches entry text literally.
	Text string
	// Start is the inclusive lower bound on entry creation time.
	Start time.Time
	// End is the exclusive upper bound on entry creation time.
	End time.Time
}

// HistoryTimeFilter converts optional [start, end) Unix-millisecond bounds and
// reports false when a bound is negative or the range is empty.
func HistoryTimeFilter(startMs, endMs *int64) (HistoryFilter, bool) {
	var filter HistoryFilter
	if startMs != nil {
		if *startMs < 0 {
			return HistoryFilter{}, false
		}
		filter.Start = time.UnixMilli(*startMs).UTC()
	}
	if endMs != nil {
		if *endMs < 0 {
			return HistoryFilter{}, false
		}
		filter.End = time.UnixMilli(*endMs).UTC()
	}
	if startMs != nil && endMs != nil && *startMs >= *endMs {
		return HistoryFilter{}, false
	}
	return filter, true
}

// Search returns a page of persisted history matching filter.
func (s *HistoryStore) Search(ctx context.Context, req apitypes.PeerRunHistoryListRequest, filter HistoryFilter) (apitypes.PeerRunHistoryListResponse, error) {
	entries, hasNext, nextCursor, err := s.listMatching(ctx, req, filter)
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
	record, err := s.Records.Get(ctx, memory.HistoryKey{ConversationID: s.Workspace, ID: id})
	if errors.Is(err, memory.ErrNotFound) {
		return HistoryEntry{}, fs.ErrNotExist
	}
	if err != nil {
		return HistoryEntry{}, err
	}
	entry := historyEntryFromMemory(record)
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
	return s.cleanupRecords(ctx, s.now())
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
	return s.cleanupRecords(ctx, time.Time{})
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
	return s.listMatching(ctx, req, HistoryFilter{})
}

func (s *HistoryStore) listMatching(ctx context.Context, req apitypes.PeerRunHistoryListRequest, filter HistoryFilter) ([]HistoryEntry, bool, *string, error) {
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
	descending := order == apitypes.PeerRunHistoryListRequestOrderDesc
	start, end, err := historyQueryBounds(cursor, descending)
	if err != nil {
		return nil, false, nil, err
	}
	if !filter.Start.IsZero() && filter.Start.After(start) {
		start = filter.Start
	}
	if !filter.End.IsZero() && filter.End.Before(end) {
		end = filter.End
	}
	if !start.Before(end) {
		return nil, false, nil, nil
	}
	out := make([]HistoryEntry, 0, limit)
	var storeCursor *memory.HistoryPosition
	if cursor != "" {
		at, err := historyIDTime(cursor)
		if err != nil {
			return nil, false, nil, err
		}
		storeCursor = &memory.HistoryPosition{CreatedAt: at, ID: cursor}
	}
	for {
		page, err := s.Records.Query(ctx, memory.HistoryQuery{
			ConversationID: s.Workspace,
			Text:           filter.Text,
			Start:          start,
			End:            end,
			// One extra record detects has_next; entries skipped below are
			// refilled by the next store page.
			Limit:      limit + 1,
			Descending: descending,
			Cursor:     storeCursor,
		})
		if err != nil {
			return nil, false, nil, fmt.Errorf("workspace history: query records: %w", err)
		}
		for _, record := range page.Entries {
			entry := historyEntryFromMemory(record)
			if cursor != "" {
				if descending && entry.ID >= cursor {
					continue
				}
				if !descending && entry.ID <= cursor {
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
		if page.Next == nil {
			return out, false, nil, nil
		}
		storeCursor = page.Next
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

func historyQueryBounds(cursor string, descending bool) (time.Time, time.Time, error) {
	start := time.Unix(0, 0).UTC()
	end := time.Date(2262, 1, 1, 0, 0, 0, 0, time.UTC)
	if cursor == "" {
		return start, end, nil
	}
	boundary, err := historyIDTime(cursor)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %v", ErrInvalidHistoryCursor, err)
	}
	if descending {
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

func (s *HistoryStore) cleanupRecords(ctx context.Context, before time.Time) error {
	return memory.CleanupHistory(ctx, s.Records, memory.HistoryCleanupQuery{ConversationID: s.Workspace, Before: before}, func(_ memory.HistoryKey, asset memory.HistoryAsset) error {
		if err := s.Objects.Delete(asset.Name); err != nil && !isNotExist(err) {
			return err
		}
		return nil
	})
}

func (s *HistoryStore) deleteEntry(ctx context.Context, entry HistoryEntry) error {
	for _, asset := range entry.Assets {
		if err := s.Objects.Delete(asset.Name); err != nil && !isNotExist(err) {
			return err
		}
	}
	if err := s.Records.Delete(ctx, memory.HistoryKey{ConversationID: s.Workspace, ID: entry.ID}); err != nil && !errors.Is(err, memory.ErrNotFound) {
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
	seq := historyIDSeq.Add(1)
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
