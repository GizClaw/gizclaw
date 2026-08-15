// Package objectstore defines a generic object storage abstraction.
package objectstore

import (
	"io"
	"time"
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Name     string
	Size     int64
	Deadline time.Time
}

// ObjectStore provides provider-neutral, prefix-addressable object storage.
//
// Object names are normalized relative slash-separated keys such as
// "demo/main/stable/manifest.json". Implementations stream writes, replace an
// exact object, hide expired objects, and return List results in lexical order.
type ObjectStore interface {
	// Get opens an object for reading.
	// Returns an error wrapping fs.ErrNotExist/os.ErrNotExist if absent.
	Get(name string) (io.ReadCloser, error)

	// Put writes or replaces an object from the provided reader.
	Put(name string, r io.Reader) error

	// PutWithDeadline writes or replaces an object and makes it expire at the
	// given deadline. A zero deadline means the object does not expire.
	PutWithDeadline(name string, r io.Reader, deadline time.Time) error

	// PutWithTTL writes or replaces an object and makes it expire after ttl.
	PutWithTTL(name string, r io.Reader, ttl time.Duration) error

	// Delete removes a single object. Returns nil if absent.
	Delete(name string) error

	// DeletePrefix removes all objects under the given prefix. A scoped Store
	// may interpret an empty prefix as its complete logical namespace.
	DeletePrefix(prefix string) error

	// List returns all objects at the prefix or below it in lexical name order.
	List(prefix string) ([]ObjectInfo, error)
}

// LocalDirProvider is implemented by object stores backed by a local
// filesystem directory.
type LocalDirProvider interface {
	LocalDir() (string, bool)
}
