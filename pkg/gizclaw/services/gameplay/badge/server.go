package badge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

var rootKey = kv.Key{"by-id"}

type Server struct {
	Store  kv.Store
	Assets objectstore.ObjectStore
	Now    func() time.Time
}

func (s *Server) Put(ctx context.Context, id string, spec apitypes.BadgeSpec) (apitypes.Badge, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Badge{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.Badge{}, errors.New("badge id is required")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return apitypes.Badge{}, errors.New("badge name is required")
	}
	now := s.now()
	current, err := Get(ctx, store, id)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return apitypes.Badge{}, err
	}
	out := apitypes.Badge{
		Id:          id,
		Name:        name,
		Description: strings.TrimSpace(spec.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err == nil {
		out.CreatedAt = current.CreatedAt
		out.IconPath = current.IconPath
	}
	if spec.IconPath != nil {
		out.IconPath = strings.TrimSpace(*spec.IconPath)
	}
	if err := Write(ctx, store, out); err != nil {
		return apitypes.Badge{}, err
	}
	return out, nil
}

func (s *Server) Get(ctx context.Context, id string) (apitypes.Badge, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Badge{}, err
	}
	return Get(ctx, store, id)
}

func (s *Server) Delete(ctx context.Context, id string) (apitypes.Badge, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Badge{}, err
	}
	item, err := Get(ctx, store, id)
	if err != nil {
		return apitypes.Badge{}, err
	}
	if err := store.Delete(ctx, badgeKey(id)); err != nil {
		return apitypes.Badge{}, err
	}
	if item.IconPath != "" && s.Assets != nil {
		if err := s.Assets.Delete(item.IconPath); err != nil {
			return apitypes.Badge{}, err
		}
	}
	return item, nil
}

func (s *Server) List(ctx context.Context, cursor string, limit int) ([]apitypes.Badge, bool, *string, error) {
	store, err := s.store()
	if err != nil {
		return nil, false, nil, err
	}
	normalizedCursor, normalizedLimit := normalizeListParams(cursor, limit)
	entries, err := kv.ListAfter(ctx, store, rootKey, cursorAfterKey(normalizedCursor), normalizedLimit+1)
	if err != nil {
		return nil, false, nil, err
	}
	hasNext := len(entries) > normalizedLimit
	if hasNext {
		entries = entries[:normalizedLimit]
	}
	items := make([]apitypes.Badge, 0, len(entries))
	for _, entry := range entries {
		var item apitypes.Badge
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	var next *string
	if hasNext && len(entries) > 0 {
		v := unescapeStoreSegment(entries[len(entries)-1].Key[len(entries[len(entries)-1].Key)-1])
		next = &v
	}
	return items, hasNext, next, nil
}

func (s *Server) UploadIcon(ctx context.Context, id string, r io.Reader) (apitypes.Badge, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Badge{}, err
	}
	assets, err := s.assets()
	if err != nil {
		return apitypes.Badge{}, err
	}
	item, err := Get(ctx, store, id)
	if err != nil {
		return apitypes.Badge{}, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return apitypes.Badge{}, err
	}
	if item.IconPath == "" {
		item.IconPath = item.Id + "/icon"
	}
	if err := assets.Put(item.IconPath, bytes.NewReader(data)); err != nil {
		return apitypes.Badge{}, err
	}
	item.UpdatedAt = s.now()
	if err := Write(ctx, store, item); err != nil {
		return apitypes.Badge{}, err
	}
	return item, nil
}

func (s *Server) DownloadIcon(ctx context.Context, id string) (io.ReadCloser, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	assets, err := s.assets()
	if err != nil {
		return nil, err
	}
	item, err := Get(ctx, store, id)
	if err != nil {
		return nil, err
	}
	if item.IconPath == "" {
		return nil, fmt.Errorf("badge %q has no icon file", id)
	}
	return assets.Get(item.IconPath)
}

func Get(ctx context.Context, store kv.Store, id string) (apitypes.Badge, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.Badge{}, errors.New("badge id is required")
	}
	data, err := store.Get(ctx, badgeKey(id))
	if err != nil {
		return apitypes.Badge{}, err
	}
	var item apitypes.Badge
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.Badge{}, err
	}
	return item, nil
}

func Write(ctx context.Context, store kv.Store, item apitypes.Badge) error {
	if strings.TrimSpace(item.Id) == "" {
		return errors.New("badge id is required")
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return store.Set(ctx, badgeKey(item.Id), data)
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("badge service not configured")
	}
	return s.Store, nil
}

func (s *Server) assets() (objectstore.ObjectStore, error) {
	if s == nil || s.Assets == nil {
		return nil, errors.New("badge asset store not configured")
	}
	return s.Assets, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func badgeKey(id string) kv.Key {
	return append(append(kv.Key{}, rootKey...), escapeStoreSegment(strings.TrimSpace(id)))
}

func normalizeListParams(cursor string, limit int) (string, int) {
	normalizedCursor := escapeStoreSegment(strings.TrimSpace(cursor))
	normalizedLimit := defaultListLimit
	if limit > 0 {
		normalizedLimit = limit
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	return normalizedCursor, normalizedLimit
}

func cursorAfterKey(cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	return append(append(kv.Key{}, rootKey...), cursor)
}

func escapeStoreSegment(value string) string {
	return url.QueryEscape(value)
}

func unescapeStoreSegment(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
