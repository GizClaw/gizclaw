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
	"path"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

const (
	defaultHistoryListLimit = 50
	maxHistoryListLimit     = 200
	defaultHistoryAssetTTL  = 7 * 24 * time.Hour
	historyEntryTypeGear    = "gear"
	historyEntryTypeAgent   = "agent"
)

var historyIDSeq uint64

// HistoryStore persists workspace history metadata and assets in object storage.
type HistoryStore struct {
	Objects        objectstore.ObjectStore
	Workspace      string
	ObjectPrefix   string
	Now            func() time.Time
	AssetRetention time.Duration
}

// HistoryEntry is the internal persisted history shape.
type HistoryEntry struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	GearID          string         `json:"gear_id,omitempty"`
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
	Name      string
	Text      string
	CreatedAt time.Time
	Asset     *AppendHistoryAsset
}

// AppendHistoryAsset is an optional binary asset attached to a history entry.
type AppendHistoryAsset struct {
	MIMEType string
	Data     []byte
	TTL      time.Duration
}

// NewHistoryStore constructs a HistoryStore for one workspace runtime.
func NewHistoryStore(objects objectstore.ObjectStore, workspace string) *HistoryStore {
	return &HistoryStore{
		Objects:        objects,
		Workspace:      workspace,
		ObjectPrefix:   ObjectPrefix(workspace),
		AssetRetention: defaultHistoryAssetTTL,
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
		asset, err := s.writeAsset(entry.ID, *req.Asset, now)
		if err != nil {
			return HistoryEntry{}, err
		}
		written = append(written, asset.Name)
		entry.Assets = append(entry.Assets, asset)
		hasReplayContent = true
		entry.ExpiresAt = asset.ExpiresAt
	}
	if strings.TrimSpace(entry.Text) != "" {
		hasReplayContent = true
	}
	entry.ReplayAvailable = hasReplayContent
	data, err := json.Marshal(entry)
	if err != nil {
		for _, name := range written {
			_ = s.Objects.Delete(name)
		}
		return HistoryEntry{}, fmt.Errorf("workspace history: encode entry: %w", err)
	}
	if err := s.Objects.Put(s.entryObjectName(entry.ID), bytes.NewReader(data)); err != nil {
		for _, name := range written {
			_ = s.Objects.Delete(name)
		}
		return HistoryEntry{}, fmt.Errorf("workspace history: write entry: %w", err)
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
	entry, err := s.readEntry(s.entryObjectName(id))
	if err != nil {
		return HistoryEntry{}, err
	}
	if s.entryExpired(entry) {
		if err := s.deleteEntry(entry); err != nil {
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
	objects, err := s.Objects.List(s.entryPrefix())
	if err != nil {
		return err
	}
	now := s.now()
	for _, obj := range objects {
		entry, err := s.readEntry(obj.Name)
		if err != nil {
			return err
		}
		if entry.ExpiresAt == nil || now.Before(*entry.ExpiresAt) {
			continue
		}
		if err := s.deleteEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

func (e HistoryEntry) Public() apitypes.PeerRunHistoryEntry {
	item := apitypes.PeerRunHistoryEntry{
		Id:              e.ID,
		Type:            apitypes.PeerRunHistoryEntryType(e.Type),
		Name:            e.Name,
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
	objects, err := s.Objects.List(s.entryPrefix())
	if err != nil {
		return nil, false, nil, err
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Name < objects[j].Name
	})
	out := make([]HistoryEntry, 0, limit)
	var afterCursor bool
	hasCursor := cursor != ""
	for _, obj := range objects {
		id := strings.TrimSuffix(path.Base(obj.Name), ".json")
		if hasCursor && !afterCursor {
			afterCursor = id == cursor
			continue
		}
		entry, err := s.readEntry(obj.Name)
		if err != nil {
			return nil, false, nil, err
		}
		if s.entryExpired(entry) {
			if err := s.deleteEntry(entry); err != nil {
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
	return out, false, nil, nil
}

func (s *HistoryStore) writeAsset(id string, asset AppendHistoryAsset, now time.Time) (HistoryAsset, error) {
	ttl := asset.TTL
	if ttl <= 0 {
		ttl = s.AssetRetention
	}
	if ttl <= 0 {
		ttl = defaultHistoryAssetTTL
	}
	deadline := now.Add(ttl).UTC()
	name := s.assetObjectName(id, asset.MIMEType)
	if err := s.Objects.PutWithDeadline(name, bytes.NewReader(asset.Data), deadline); err != nil {
		return HistoryAsset{}, fmt.Errorf("workspace history: write asset: %w", err)
	}
	return HistoryAsset{
		Name:      name,
		MIMEType:  strings.TrimSpace(asset.MIMEType),
		Bytes:     int64(len(asset.Data)),
		ExpiresAt: &deadline,
	}, nil
}

func (s *HistoryStore) readEntry(name string) (HistoryEntry, error) {
	r, err := s.Objects.Get(name)
	if err != nil {
		return HistoryEntry{}, err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return HistoryEntry{}, err
	}
	var entry HistoryEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return HistoryEntry{}, fmt.Errorf("workspace history: decode %s: %w", name, err)
	}
	if err := validateHistoryEntry(entry); err != nil {
		return HistoryEntry{}, fmt.Errorf("workspace history: decode %s: %w", name, err)
	}
	return entry, nil
}

func (s *HistoryStore) deleteEntry(entry HistoryEntry) error {
	for _, asset := range entry.Assets {
		if err := s.Objects.Delete(asset.Name); err != nil && !isNotExist(err) {
			return err
		}
	}
	if err := s.Objects.Delete(s.entryObjectName(entry.ID)); err != nil && !isNotExist(err) {
		return err
	}
	return nil
}

func (s *HistoryStore) entryExpired(entry HistoryEntry) bool {
	return entry.ExpiresAt != nil && !s.now().Before(*entry.ExpiresAt)
}

func (s *HistoryStore) validate() error {
	if s == nil || s.Objects == nil {
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

func (s *HistoryStore) entryPrefix() string {
	return s.historyPrefix() + "/entries"
}

func (s *HistoryStore) entryObjectName(id string) string {
	return s.entryPrefix() + "/" + url.PathEscape(id) + ".json"
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
