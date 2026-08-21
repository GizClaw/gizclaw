package giztest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func discover(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("at least one file or directory is required")
	}
	seen := map[string]struct{}{}
	var paths []string
	for _, input := range inputs {
		abs, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink is not allowed: %s", input)
		}
		root := abs
		if !info.IsDir() {
			if !strings.HasSuffix(info.Name(), ".giztest.yaml") {
				return nil, fmt.Errorf("not a .giztest.yaml file: %s", input)
			}
			if _, ok := seen[abs]; !ok {
				seen[abs] = struct{}{}
				paths = append(paths, abs)
			}
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == root {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink is not allowed: %s", path)
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".giztest.yaml") {
				return nil
			}
			clean := filepath.Clean(path)
			rel, err := filepath.Rel(root, clean)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("path escapes root: %s", path)
			}
			if _, ok := seen[clean]; !ok {
				seen[clean] = struct{}{}
				paths = append(paths, clean)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .giztest.yaml files selected")
	}
	return paths, nil
}
