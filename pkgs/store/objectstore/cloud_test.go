package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryCloudObject struct {
	data     []byte
	metadata map[string]string
}

type memoryCloudBackend struct {
	mu      sync.Mutex
	objects map[string]memoryCloudObject
	putErr  error
}

func newMemoryCloudBackend() *memoryCloudBackend {
	return &memoryCloudBackend{objects: map[string]memoryCloudObject{}}
}

func (b *memoryCloudBackend) get(_ context.Context, name string) (io.ReadCloser, cloudObjectAttrs, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	object, ok := b.objects[name]
	if !ok {
		return nil, cloudObjectAttrs{}, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(object.data)), cloudObjectAttrs{name: name, size: int64(len(object.data)), metadata: cloneMetadata(object.metadata)}, nil
}

func (b *memoryCloudBackend) put(_ context.Context, name string, reader io.Reader, metadata map[string]string) error {
	if b.putErr != nil {
		return b.putErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.objects[name] = memoryCloudObject{data: data, metadata: cloneMetadata(metadata)}
	b.mu.Unlock()
	return nil
}

func (b *memoryCloudBackend) delete(_ context.Context, name string) error {
	b.mu.Lock()
	delete(b.objects, name)
	b.mu.Unlock()
	return nil
}

func (b *memoryCloudBackend) list(_ context.Context, prefix string) ([]cloudObjectAttrs, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []cloudObjectAttrs
	for name, object := range b.objects {
		if strings.HasPrefix(name, prefix) {
			out = append(out, cloudObjectAttrs{name: name, size: int64(len(object.data)), metadata: cloneMetadata(object.metadata)})
		}
	}
	return out, nil
}

func cloneMetadata(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func TestCloudStoreConformance(t *testing.T) {
	backend := newMemoryCloudBackend()
	store, err := newCloudStore("test", backend)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", "/absolute", "../escape", ".objectstore-meta/private"} {
		if err := store.Put(name, strings.NewReader("bad")); err == nil {
			t.Fatalf("Put(%q) succeeded", name)
		}
	}
	if err := store.Put("demo/item", strings.NewReader("old")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("demo/item", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Get("demo/item")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil || string(data) != "new" {
		t.Fatalf("Get = %q, %v", data, err)
	}

	deadline := time.Now().Add(time.Hour).Round(0)
	if err := store.PutWithDeadline("demo/expiring", strings.NewReader("value"), deadline); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("demolition/other", strings.NewReader("keep")); err != nil {
		t.Fatal(err)
	}
	items, err := store.List("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "demo/expiring" || items[1].Name != "demo/item" || !items[0].Deadline.Equal(deadline) {
		t.Fatalf("List = %#v", items)
	}
	if err := store.DeletePrefix("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("demo/item"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("deleted Get error = %v", err)
	}
	if _, err := store.Get("demolition/other"); err != nil {
		t.Fatalf("neighbor Get error = %v", err)
	}
}

func TestCloudStoreExpirationAndErrors(t *testing.T) {
	backend := newMemoryCloudBackend()
	store, err := newCloudStore("test", backend)
	if err != nil {
		t.Fatal(err)
	}
	backend.objects["expired"] = memoryCloudObject{
		data: []byte("old"), metadata: map[string]string{deadlineMetadataKey: time.Now().Add(-time.Second).Format(time.RFC3339Nano)},
	}
	if _, err := store.Get("expired"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Get expired error = %v", err)
	}
	if _, ok := backend.objects["expired"]; ok {
		t.Fatal("expired object was not removed")
	}
	if err := store.PutWithTTL("ttl", strings.NewReader("x"), 0); err == nil {
		t.Fatal("zero TTL succeeded")
	}
	backend.putErr = errors.New("provider secret response")
	if err := store.Put("failed", strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), "put") {
		t.Fatalf("Put error = %v", err)
	}
	backend.putErr = nil
	backend.objects["malformed"] = memoryCloudObject{
		data: []byte("value"), metadata: map[string]string{deadlineMetadataKey: "secret-metadata-value"},
	}
	if _, err := store.Get("malformed"); err == nil || strings.Contains(err.Error(), "secret-metadata-value") {
		t.Fatalf("Get malformed metadata error = %v", err)
	}
}

var _ cloudBackend = (*memoryCloudBackend)(nil)
