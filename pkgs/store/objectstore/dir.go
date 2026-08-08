package objectstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dir stores objects on the local filesystem rooted at the given directory.
// It is a path-based convenience adapter; each operation delegates to the same
// rooted implementation used by Storage-backed ObjectStores.
//
// Object names are slash-separated keys. Directories are implementation detail;
// callers should treat this as object storage, not as a general filesystem.
type Dir string

var _ ObjectStore = Dir("")

func (d Dir) Get(name string) (reader io.ReadCloser, err error) {
	name, err = cleanName(name, false)
	if err != nil {
		return nil, err
	}
	store, root, err := d.open(false)
	if err != nil {
		return nil, err
	}
	reader, err = store.Get(name)
	if err != nil {
		if closeErr := root.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	return &dirReadCloser{ReadCloser: reader, root: root}, nil
}

type dirReadCloser struct {
	io.ReadCloser
	root *os.Root
}

func (r *dirReadCloser) Close() error {
	return errors.Join(r.ReadCloser.Close(), r.root.Close())
}

func (d Dir) Put(name string, reader io.Reader) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	return d.use(true, func(store *Root) error {
		return store.Put(name, reader)
	})
}

func (d Dir) PutWithDeadline(name string, reader io.Reader, deadline time.Time) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		return errors.New("objectstore: deadline must be in the future")
	}
	return d.use(true, func(store *Root) error {
		return store.PutWithDeadline(name, reader, deadline)
	})
}

func (d Dir) PutWithTTL(name string, reader io.Reader, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("objectstore: ttl must be positive")
	}
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	return d.use(true, func(store *Root) error {
		return store.PutWithTTL(name, reader, ttl)
	})
}

func (d Dir) Delete(name string) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	err = d.use(false, func(store *Root) error {
		return store.Delete(name)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (d Dir) DeletePrefix(prefix string) error {
	prefix, err := cleanName(prefix, true)
	if err != nil || prefix == "" {
		return err
	}
	err = d.use(false, func(store *Root) error {
		return store.DeletePrefix(prefix)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (d Dir) List(prefix string) (items []ObjectInfo, err error) {
	prefix, err = cleanName(prefix, true)
	if err != nil {
		return nil, err
	}
	store, root, err := d.open(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()
	return store.List(prefix)
}

func (d Dir) LocalDir() (string, bool) { return d.root(), true }

func (d Dir) use(create bool, operation func(*Root) error) (err error) {
	store, root, err := d.open(create)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()
	return operation(store)
}

func (d Dir) open(create bool) (*Root, *os.Root, error) {
	dir := d.root()
	if create {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	store, err := NewRoot(root)
	if err != nil {
		_ = root.Close()
		return nil, nil, err
	}
	return store, root, nil
}

func (d Dir) root() string {
	if d == "" {
		return "."
	}
	return string(d)
}

func (d Dir) join(name string) string {
	if name == "" {
		return d.root()
	}
	return filepath.Join(d.root(), filepath.FromSlash(name))
}

func (d Dir) metadataRoot() string { return d.join(metadataRoot) }

func (d Dir) metadataPath(name string) string { return d.join(metadataName(name)) }

func (d Dir) writeMetadata(name string, deadline time.Time) error {
	return d.use(true, func(store *Root) error {
		return store.writeMetadata(name, deadline)
	})
}

func cleanName(name string, allowEmpty bool) (string, error) {
	if name == "" {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("objectstore: object name is empty")
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("objectstore: invalid absolute object name %q", name)
	}

	parts := strings.Split(filepath.ToSlash(name), "/")
	out := parts[:0]
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("objectstore: invalid object name %q", name)
		default:
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("objectstore: object name is empty")
	}
	name = strings.Join(out, "/")
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("objectstore: invalid object name %q", name)
	}
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("objectstore: invalid absolute object name %q", name)
	}
	if name == metadataRoot || strings.HasPrefix(name, metadataRoot+"/") {
		return "", fmt.Errorf("objectstore: reserved object name %q", name)
	}
	return name, nil
}
