package storage

import (
	"errors"
	"fmt"
	"os"
)

// FilesystemDir returns the named rooted filesystem handle.
func (s *Storage) FilesystemDir(name string) (*os.Root, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	root, ok := s.dirs[name]
	if !ok {
		return nil, fmt.Errorf("storage: filesystem.dir %q not found", name)
	}
	return root, nil
}

func newFilesystemDir(name string, cfg FilesystemDirConfig) (*os.Root, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("storage: filesystem.dir %q requires dir", name)
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: filesystem.dir %q mkdir: %w", name, err)
	}
	return os.OpenRoot(cfg.Dir)
}
