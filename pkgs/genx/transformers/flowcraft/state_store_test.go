package flowcraft

import (
	"bytes"
	"context"
	"sync"
)

type memoryState struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newMemoryState() *memoryState { return &memoryState{values: make(map[string][]byte)} }
func (s *memoryState) LoadState(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.values[key]), nil
}
func (s *memoryState) SaveState(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = bytes.Clone(value)
	return nil
}
