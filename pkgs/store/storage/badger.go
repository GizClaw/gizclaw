package storage

import (
	"errors"
	"fmt"
	"os"

	"github.com/dgraph-io/badger/v4"
)

// Badger returns the named physical Badger DB.
func (s *Storage) Badger(name string) (*badger.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	st, ok := s.badgers[name]
	if !ok {
		return nil, fmt.Errorf("storage: badger %q not found", name)
	}
	return st, nil
}

func newBadger(name, dir string) (*badger.DB, error) {
	if dir == "" {
		return nil, fmt.Errorf("storage: badger %q requires dir", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: badger %q mkdir: %w", name, err)
	}
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nil))
	if err != nil {
		return nil, &externalOperationError{operation: fmt.Sprintf("storage: badger %q open", name), err: err}
	}
	return db, nil
}
