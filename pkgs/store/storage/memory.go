package storage

import (
	"errors"
	"fmt"
)

// Memory marks a process-local physical slot. Logical Store constructors use
// the marker to create independent in-memory backends.
type Memory struct{}

// Memory returns the named process-local marker.
func (s *Storage) Memory(name string) (Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Memory{}, errors.New("storage: registry is closed")
	}
	if cfg, ok := s.configs[name]; !ok || cfg.storageKind() != KindMemory {
		return Memory{}, fmt.Errorf("storage: memory %q not found", name)
	}
	return Memory{}, nil
}
