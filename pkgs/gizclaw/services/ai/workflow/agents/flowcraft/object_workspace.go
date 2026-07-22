package flowcraft

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	flowworkspace "github.com/GizClaw/flowcraft/sdk/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

// objectWorkspace adapts a Workspace-scoped ObjectStore prefix to the
// Flowcraft persistence interface used by the memory backend. It is not made
// available to script nodes.
type objectWorkspace struct {
	objects objectstore.ObjectStore
	prefix  string
	mu      sync.Mutex
}

func newObjectWorkspace(objects objectstore.ObjectStore, prefix string) (*objectWorkspace, error) {
	if objects == nil {
		return nil, fmt.Errorf("flowcraft: memory object store is required")
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return nil, fmt.Errorf("flowcraft: memory object prefix is required")
	}
	return &objectWorkspace{objects: objects, prefix: prefix}, nil
}

func (w *objectWorkspace) Read(ctx context.Context, name string) ([]byte, error) {
	key, err := w.key(ctx, name)
	if err != nil {
		return nil, err
	}
	r, err := w.objects.Get(key)
	if err != nil {
		return nil, workspaceError(name, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: read memory object %q: %w", name, err)
	}
	return data, nil
}

func (w *objectWorkspace) Write(ctx context.Context, name string, data []byte) error {
	key, err := w.key(ctx, name)
	if err != nil {
		return err
	}
	if err := w.objects.Put(key, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("flowcraft: write memory object %q: %w", name, err)
	}
	return nil
}

func (w *objectWorkspace) Append(ctx context.Context, name string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	existing, err := w.Read(ctx, name)
	if err != nil && !errors.Is(err, flowworkspace.ErrNotFound) {
		return err
	}
	existing = append(existing, data...)
	return w.Write(ctx, name, existing)
}

func (w *objectWorkspace) Rename(ctx context.Context, source, destination string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := w.Read(ctx, source)
	if err != nil {
		return err
	}
	if err := w.Write(ctx, destination, data); err != nil {
		return err
	}
	return w.Delete(ctx, source)
}

func (w *objectWorkspace) Delete(ctx context.Context, name string) error {
	key, err := w.key(ctx, name)
	if err != nil {
		return err
	}
	if err := w.objects.Delete(key); err != nil {
		return fmt.Errorf("flowcraft: delete memory object %q: %w", name, err)
	}
	return nil
}

func (w *objectWorkspace) RemoveAll(ctx context.Context, name string) error {
	key, err := w.key(ctx, name)
	if err != nil {
		return err
	}
	if err := w.objects.DeletePrefix(strings.TrimRight(key, "/") + "/"); err != nil {
		return fmt.Errorf("flowcraft: delete memory object prefix %q: %w", name, err)
	}
	return w.objects.Delete(key)
}

func (w *objectWorkspace) List(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	key, err := w.key(ctx, dir)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimRight(key, "/") + "/"
	objects, err := w.objects.List(prefix)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: list memory object prefix %q: %w", dir, err)
	}
	entries := make(map[string]objectEntry)
	for _, object := range objects {
		relative := strings.TrimPrefix(object.Name, prefix)
		if relative == "" {
			continue
		}
		name, rest, _ := strings.Cut(relative, "/")
		entry := objectEntry{name: name, size: object.Size}
		if rest != "" {
			entry.dir = true
			entry.size = 0
		}
		entries[name] = entry
	}
	result := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result, nil
}

func (w *objectWorkspace) Exists(ctx context.Context, name string) (bool, error) {
	_, err := w.Stat(ctx, name)
	if errors.Is(err, flowworkspace.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (w *objectWorkspace) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	key, err := w.key(ctx, name)
	if err != nil {
		return nil, err
	}
	objects, err := w.objects.List(key)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: stat memory object %q: %w", name, err)
	}
	for _, object := range objects {
		if object.Name == key {
			return objectEntry{name: path.Base(name), size: object.Size}, nil
		}
		if strings.HasPrefix(object.Name, strings.TrimRight(key, "/")+"/") {
			return objectEntry{name: path.Base(name), dir: true}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", flowworkspace.ErrNotFound, name)
}

func (w *objectWorkspace) key(ctx context.Context, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	clean := path.Clean(name)
	if name == "" || clean == "." {
		return w.prefix, nil
	}
	if strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", flowworkspace.ErrPathTraversal
	}
	return w.prefix + "/" + clean, nil
}

func workspaceError(name string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", flowworkspace.ErrNotFound, name)
	}
	return fmt.Errorf("flowcraft: access memory object %q: %w", name, err)
}

type objectEntry struct {
	name string
	size int64
	dir  bool
}

func (e objectEntry) Name() string { return e.name }
func (e objectEntry) IsDir() bool  { return e.dir }
func (e objectEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e objectEntry) Info() (fs.FileInfo, error) { return e, nil }
func (e objectEntry) Size() int64                { return e.size }
func (e objectEntry) Mode() fs.FileMode {
	if e.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (e objectEntry) ModTime() time.Time { return time.Time{} }
func (e objectEntry) Sys() any           { return nil }

var _ flowworkspace.Workspace = (*objectWorkspace)(nil)
