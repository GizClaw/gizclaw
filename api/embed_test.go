package api

import (
	"bytes"
	"io/fs"
	"os"
	"testing"
)

func TestFilesContainCompleteAPISourceTrees(t *testing.T) {
	source := os.DirFS(".")
	for _, root := range []string{"http", "proto"} {
		err := fs.WalkDir(source, root, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			want, err := fs.ReadFile(source, name)
			if err != nil {
				return err
			}
			got, err := Files.ReadFile(name)
			if err != nil {
				t.Errorf("Files.ReadFile(%q): %v", name, err)
				return nil
			}
			if !bytes.Equal(got, want) {
				t.Errorf("Files.ReadFile(%q) does not match the source file", name)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s API sources: %v", root, err)
		}
	}
}
