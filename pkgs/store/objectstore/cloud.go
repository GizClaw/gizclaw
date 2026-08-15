package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"
)

const (
	cloudOperationTimeout = 30 * time.Second
	deadlineMetadataKey   = "gizclaw-deadline"
)

type cloudObjectAttrs struct {
	name     string
	size     int64
	metadata map[string]string
}

type cloudBackend interface {
	get(context.Context, string) (io.ReadCloser, cloudObjectAttrs, error)
	put(context.Context, string, io.Reader, map[string]string) error
	delete(context.Context, string) error
	list(context.Context, string) ([]cloudObjectAttrs, error)
}

type cloudStore struct {
	provider string
	backend  cloudBackend
}

func newCloudStore(provider string, backend cloudBackend) (*cloudStore, error) {
	if backend == nil {
		return nil, fmt.Errorf("objectstore: %s backend is nil", provider)
	}
	return &cloudStore{provider: provider, backend: backend}, nil
}

func (s *cloudStore) Get(name string) (io.ReadCloser, error) {
	name, err := cleanName(name, false)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudOperationTimeout)
	body, attrs, err := s.backend.get(ctx, name)
	if err != nil {
		cancel()
		return nil, s.wrap("get", name, err)
	}
	deadline, err := parseCloudDeadline(attrs.metadata)
	if err != nil {
		_ = body.Close()
		cancel()
		return nil, s.wrap("get metadata", name, err)
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		_ = body.Close()
		_ = s.backend.delete(ctx, name)
		cancel()
		return nil, fs.ErrNotExist
	}
	return &cancelReadCloser{ReadCloser: body, cancel: cancel}, nil
}

func (s *cloudStore) Put(name string, reader io.Reader) error {
	return s.put(name, reader, time.Time{})
}

func (s *cloudStore) PutWithDeadline(name string, reader io.Reader, deadline time.Time) error {
	return s.put(name, reader, deadline)
}

func (s *cloudStore) PutWithTTL(name string, reader io.Reader, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("objectstore: ttl must be positive")
	}
	return s.put(name, reader, time.Now().Add(ttl))
}

func (s *cloudStore) put(name string, reader io.Reader, deadline time.Time) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	if reader == nil {
		return errors.New("objectstore: reader is nil")
	}
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		return errors.New("objectstore: deadline must be in the future")
	}
	metadata := map[string]string{}
	if !deadline.IsZero() {
		metadata[deadlineMetadataKey] = deadline.UTC().Format(time.RFC3339Nano)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudOperationTimeout)
	defer cancel()
	if err := s.backend.put(ctx, name, reader, metadata); err != nil {
		return s.wrap("put", name, err)
	}
	return nil
}

func (s *cloudStore) Delete(name string) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloudOperationTimeout)
	defer cancel()
	if err := s.backend.delete(ctx, name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return s.wrap("delete", name, err)
	}
	return nil
}

func (s *cloudStore) DeletePrefix(prefix string) error {
	prefix, err := cleanName(prefix, true)
	if err != nil || prefix == "" {
		return err
	}
	items, err := s.list(prefix)
	if err != nil {
		return err
	}
	var errs []error
	for _, item := range items {
		if err := s.Delete(item.Name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *cloudStore) List(prefix string) ([]ObjectInfo, error) {
	prefix, err := cleanName(prefix, true)
	if err != nil {
		return nil, err
	}
	return s.list(prefix)
}

func (s *cloudStore) list(prefix string) ([]ObjectInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cloudOperationTimeout)
	defer cancel()
	attrs, err := s.backend.list(ctx, prefix)
	if err != nil {
		return nil, s.wrap("list", prefix, err)
	}
	now := time.Now()
	out := make([]ObjectInfo, 0, len(attrs))
	for _, attr := range attrs {
		if prefix != "" && attr.name != prefix && !strings.HasPrefix(attr.name, prefix+"/") {
			continue
		}
		name, err := cleanName(attr.name, false)
		if err != nil {
			return nil, s.wrap("list name", attr.name, err)
		}
		deadline, err := parseCloudDeadline(attr.metadata)
		if err != nil {
			return nil, s.wrap("list metadata", name, err)
		}
		if !deadline.IsZero() && !now.Before(deadline) {
			_ = s.backend.delete(ctx, name)
			continue
		}
		out = append(out, ObjectInfo{Name: name, Size: attr.size, Deadline: deadline})
	}
	slices.SortFunc(out, func(a, b ObjectInfo) int { return cmpString(a.Name, b.Name) })
	return out, nil
}

func parseCloudDeadline(metadata map[string]string) (time.Time, error) {
	value := metadata[deadlineMetadataKey]
	if value == "" {
		return time.Time{}, nil
	}
	deadline, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.New("invalid deadline metadata")
	}
	return deadline, nil
}

func (s *cloudStore) wrap(operation, name string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("objectstore: %s %s %q: %w", s.provider, operation, name, fs.ErrNotExist)
	}
	return fmt.Errorf("objectstore: %s %s %q: %w", s.provider, operation, name, err)
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

var _ ObjectStore = (*cloudStore)(nil)

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}
