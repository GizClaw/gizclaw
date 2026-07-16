package asset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestRefRoundTripAndValidation(t *testing.T) {
	ref := Ref("asset://7d9c87aa1a224de6b93082026f30c77e")
	parsed, err := ParseRef(ref.String())
	if err != nil || parsed != ref {
		t.Fatalf("ParseRef() = %q, %v", parsed, err)
	}
	for _, invalid := range []string{
		"",
		"asset://7D9C87AA1A224DE6B93082026F30C77E",
		"asset://7d9c87aa1a224de6b93082026f30c77",
		"asset://7d9c87aa1a224de6b93082026f30c77e/extra",
		"asset://7d9c87aa1a224de6b93082026f30c77e?x=1",
		"https://example.com/icon.png",
	} {
		if _, err := ParseRef(invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ParseRef(%q) error = %v", invalid, err)
		}
	}
}

func TestNewRequiresStores(t *testing.T) {
	objects := &memoryObjectStore{data: make(map[string][]byte), deadlines: make(map[string]time.Time)}
	if _, err := New(nil, objects, Options{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("New(nil metadata) error = %v", err)
	}
	if _, err := New(kv.NewMemory(nil), nil, Options{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("New(nil objects) error = %v", err)
	}
}

func TestPutOpenAndMetadata(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	service, objects := newTestService(t, now, idSequence(idWithByte(1)))
	payload := bytes.Repeat([]byte("gizclaw-asset"), 8192)
	stored, err := service.Put(context.Background(), PutRequest{
		MediaType: "image/png; charset=binary",
		MaxBytes:  int64(len(payload)),
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	wantRef := Ref("asset://01010101010101010101010101010101")
	wantDigest := sha256.Sum256(payload)
	if stored.Metadata.Ref != wantRef || stored.Metadata.MediaType != "image/png" || stored.Metadata.SizeBytes != int64(len(payload)) || stored.Metadata.SHA256 != wantDigest || !stored.Metadata.CreatedAt.Equal(now) {
		t.Fatalf("Put() = %#v", stored)
	}
	opened, reader, err := service.Open(context.Background(), wantRef)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !bytes.Equal(got, payload) || !reflect.DeepEqual(opened.Metadata, stored.Metadata) {
		t.Fatalf("Open() metadata=%#v bytes=%d", opened.Metadata, len(got))
	}
	if _, ok := objects.data[objectName("01010101010101010101010101010101")]; !ok {
		t.Fatal("Put() did not write the sharded object key")
	}
}

func TestPutEmptyLimitReaderFailureAndRollback(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	t.Run("empty", func(t *testing.T) {
		service, _ := newTestService(t, now, idSequence(idWithByte(2)))
		stored, err := service.Put(context.Background(), PutRequest{MediaType: "application/octet-stream", MaxBytes: 1}, bytes.NewReader(nil))
		if err != nil || stored.Metadata.SizeBytes != 0 || stored.Metadata.SHA256 != sha256.Sum256(nil) {
			t.Fatalf("Put(empty) = %#v, %v", stored, err)
		}
	})
	t.Run("too large", func(t *testing.T) {
		service, objects := newTestService(t, now, idSequence(idWithByte(3)))
		_, err := service.Put(context.Background(), PutRequest{MediaType: "image/png", MaxBytes: 3}, bytes.NewBufferString("four"))
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("Put(too large) error = %v", err)
		}
		if len(objects.data) != 0 {
			t.Fatalf("Put(too large) objects = %v", objects.data)
		}
		if _, _, err := service.Open(context.Background(), Ref("asset://03030303030303030303030303030303")); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Open(rolled back) error = %v", err)
		}
	})
	t.Run("reader failure", func(t *testing.T) {
		service, objects := newTestService(t, now, idSequence(idWithByte(4)))
		_, err := service.Put(context.Background(), PutRequest{MediaType: "image/png", MaxBytes: 100}, failingReader{})
		if err == nil || len(objects.data) != 0 {
			t.Fatalf("Put(reader failure) error=%v objects=%v", err, objects.data)
		}
	})
	t.Run("object store failure", func(t *testing.T) {
		service, objects := newTestService(t, now, idSequence(idWithByte(13)))
		objectErr := errors.New("object store unavailable")
		objects.putErr = objectErr
		_, err := service.Put(context.Background(), PutRequest{MediaType: "image/png", MaxBytes: 100}, bytes.NewBufferString("payload"))
		if !errors.Is(err, objectErr) || len(objects.data) != 0 {
			t.Fatalf("Put(object store failure) error=%v objects=%v", err, objects.data)
		}
		if _, err := service.repo.asset(context.Background(), "0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("asset(rolled back object failure) error = %v", err)
		}
	})
	t.Run("metadata publish failure", func(t *testing.T) {
		metadata := &failingKVStore{
			Store:          kv.NewMemory(nil),
			failBatchSetAt: 2,
			err:            errors.New("metadata unavailable"),
		}
		objects := &memoryObjectStore{data: make(map[string][]byte), deadlines: make(map[string]time.Time)}
		service, err := New(metadata, objects, Options{IDGenerator: idSequence(idWithByte(14)), Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.Put(context.Background(), PutRequest{MediaType: "image/png", MaxBytes: 100}, bytes.NewBufferString("payload"))
		if !errors.Is(err, metadata.err) || len(objects.data) != 0 {
			t.Fatalf("Put(metadata publish failure) error=%v objects=%v", err, objects.data)
		}
		if _, err := metadata.Get(context.Background(), assetKey("0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e")); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("metadata(rolled back publish failure) error = %v", err)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		service, objects := newTestService(t, now, idSequence(idWithByte(5)))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := service.Put(ctx, PutRequest{MediaType: "image/png", MaxBytes: 100}, bytes.NewBufferString("payload"))
		if !errors.Is(err, context.Canceled) || len(objects.data) != 0 {
			t.Fatalf("Put(canceled) error=%v objects=%v", err, objects.data)
		}
	})
}

func TestPutValidationCollisionAndExpiration(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	clock := now
	service, objects := newTestService(t, now, idSequence(idWithByte(6), idWithByte(6), idWithByte(7)))
	service.now = func() time.Time { return clock }
	first, err := service.Put(context.Background(), PutRequest{MediaType: "text/plain", MaxBytes: 10}, bytes.NewBufferString("one"))
	if err != nil {
		t.Fatalf("Put(first) error = %v", err)
	}
	expiresAt := now.Add(time.Hour)
	second, err := service.Put(context.Background(), PutRequest{MediaType: "text/plain", MaxBytes: 10, ExpiresAt: &expiresAt}, bytes.NewBufferString("two"))
	if err != nil {
		t.Fatalf("Put(collision retry) error = %v", err)
	}
	if first.Metadata.Ref == second.Metadata.Ref || second.Metadata.Ref != Ref("asset://07070707070707070707070707070707") {
		t.Fatalf("collision refs = %s, %s", first.Metadata.Ref, second.Metadata.Ref)
	}
	if got := objects.deadlines[objectName("07070707070707070707070707070707")]; !got.Equal(expiresAt) {
		t.Fatalf("object deadline = %v", got)
	}
	clock = expiresAt
	if _, _, err := service.Open(context.Background(), second.Metadata.Ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open(expired) error = %v", err)
	}
	for _, request := range []PutRequest{
		{MediaType: "bad", MaxBytes: 1},
		{MediaType: "image/png", MaxBytes: 0},
		{MediaType: "image/png", MaxBytes: 1, ExpiresAt: &now},
	} {
		if _, err := service.Put(context.Background(), request, bytes.NewReader(nil)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Put(%#v) error = %v", request, err)
		}
	}
}

func TestDeleteRetriesPartialCleanupAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	service, objects := newTestService(t, now, idSequence(idWithByte(8)))
	stored := mustPut(t, service, "payload")
	objects.deleteErr = errors.New("delete unavailable")
	if err := service.Delete(context.Background(), stored.Metadata.Ref); err == nil {
		t.Fatal("Delete(partial) error = nil")
	}
	record, err := service.repo.asset(context.Background(), "08080808080808080808080808080808")
	if err != nil || record.State != stateDeleting {
		t.Fatalf("partial delete record = %#v, %v", record, err)
	}
	objects.deleteErr = nil
	if err := service.Delete(context.Background(), stored.Metadata.Ref); err != nil {
		t.Fatalf("Delete(retry) error = %v", err)
	}
	if _, _, err := service.Open(context.Background(), stored.Metadata.Ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open(deleted) error = %v", err)
	}
	if err := service.Delete(context.Background(), stored.Metadata.Ref); err != nil {
		t.Fatalf("Delete(idempotent) error = %v", err)
	}
}

func TestReconcileReportsMissingReadyObject(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	service, objects := newTestService(t, now, idSequence(idWithByte(9)))
	stored := mustPut(t, service, "payload")
	id, err := stored.Metadata.Ref.id()
	if err != nil {
		t.Fatal(err)
	}
	delete(objects.data, objectName(id))
	if err := service.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile(missing ready object) error = nil")
	}
}

func TestReconcileCleansInterruptedStates(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	service, objects := newTestService(t, now, idSequence(idWithByte(10)))
	records := []assetRecord{
		{SchemaVersion: assetSchemaVersion, ID: strings.Repeat("a", idHexLen), CreatedAt: now.Add(-stagingGrace), State: stateStaging},
		{SchemaVersion: assetSchemaVersion, ID: strings.Repeat("b", idHexLen), CreatedAt: now, State: stateDeleting},
	}
	for _, record := range records {
		if err := service.repo.putAsset(context.Background(), record); err != nil {
			t.Fatal(err)
		}
		objects.data[objectName(record.ID)] = []byte("partial")
	}
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, record := range records {
		if _, err := service.repo.asset(context.Background(), record.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("asset(%s) error = %v", record.ID, err)
		}
		if _, exists := objects.data[objectName(record.ID)]; exists {
			t.Fatalf("object %s still exists", record.ID)
		}
	}
}

func TestReconcileRejectsMalformedAssetID(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	service, _ := newTestService(t, now, idSequence(idWithByte(11)))
	record := assetRecord{
		SchemaVersion: assetSchemaVersion,
		ID:            "x",
		CreatedAt:     now.Add(-stagingGrace),
		State:         stateStaging,
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.repo.store.BatchSet(context.Background(), []kv.Entry{{Key: assetKey(record.ID), Value: data}}); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(context.Background()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Reconcile(malformed id) error = %v", err)
	}
}

func TestOpenAndReconcileRejectCorruptObject(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	service, objects := newTestService(t, now, idSequence(idWithByte(12)))
	stored := mustPut(t, service, "original")
	id, err := stored.Metadata.Ref.id()
	if err != nil {
		t.Fatal(err)
	}
	objects.mu.Lock()
	objects.data[objectName(id)] = []byte("corrupt")
	objects.mu.Unlock()
	_, reader, err := service.Open(context.Background(), stored.Metadata.Ref)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("ReadAll(corrupt) error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := service.Reconcile(context.Background()); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Reconcile(corrupt) error = %v", err)
	}
}

func newTestService(t *testing.T, now time.Time, generator IDGenerator) (*Service, *memoryObjectStore) {
	t.Helper()
	objects := &memoryObjectStore{data: make(map[string][]byte), deadlines: make(map[string]time.Time)}
	service, err := New(kv.NewMemory(nil), objects, Options{IDGenerator: generator, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service, objects
}

func mustPut(t *testing.T, service *Service, value string) Asset {
	t.Helper()
	stored, err := service.Put(context.Background(), PutRequest{MediaType: "application/octet-stream", MaxBytes: 1024}, bytes.NewBufferString(value))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	return stored
}

func idWithByte(value byte) [idBytes]byte {
	var id [idBytes]byte
	for i := range id {
		id[i] = value
	}
	return id
}

func idSequence(ids ...[idBytes]byte) IDGenerator {
	var mu sync.Mutex
	index := 0
	return func() ([idBytes]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(ids) {
			return [idBytes]byte{}, errors.New("id sequence exhausted")
		}
		id := ids[index]
		index++
		return id, nil
	}
}

type memoryObjectStore struct {
	mu        sync.Mutex
	data      map[string][]byte
	deadlines map[string]time.Time
	putErr    error
	getErr    error
	deleteErr error
}

type failingKVStore struct {
	kv.Store
	mu             sync.Mutex
	batchSetCalls  int
	failBatchSetAt int
	err            error
}

func (s *failingKVStore) BatchSet(ctx context.Context, entries []kv.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchSetCalls++
	if s.batchSetCalls == s.failBatchSetAt {
		return s.err
	}
	return s.Store.BatchSet(ctx, entries)
}

func (s *memoryObjectStore) Get(name string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	data, exists := s.data[name]
	if !exists {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), data...))), nil
}

func (s *memoryObjectStore) Put(name string, reader io.Reader) error {
	return s.put(name, reader, time.Time{})
}

func (s *memoryObjectStore) PutWithDeadline(name string, reader io.Reader, deadline time.Time) error {
	return s.put(name, reader, deadline)
}

func (s *memoryObjectStore) PutWithTTL(name string, reader io.Reader, ttl time.Duration) error {
	return s.put(name, reader, time.Now().Add(ttl))
}

func (s *memoryObjectStore) put(name string, reader io.Reader, deadline time.Time) error {
	if s.putErr != nil {
		return s.putErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[name] = data
	s.deadlines[name] = deadline
	return nil
}

func (s *memoryObjectStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.data, name)
	delete(s.deadlines, name)
	return nil
}

func (s *memoryObjectStore) DeletePrefix(prefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range s.data {
		if strings.HasPrefix(name, prefix) {
			delete(s.data, name)
			delete(s.deadlines, name)
		}
	}
	return nil
}

func (s *memoryObjectStore) List(prefix string) ([]objectstore.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]objectstore.ObjectInfo, 0)
	for name, data := range s.data {
		if strings.HasPrefix(name, prefix) {
			items = append(items, objectstore.ObjectInfo{Name: name, Size: int64(len(data)), Deadline: s.deadlines[name]})
		}
	}
	return items, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("reader failed")
}

var _ objectstore.ObjectStore = (*memoryObjectStore)(nil)
