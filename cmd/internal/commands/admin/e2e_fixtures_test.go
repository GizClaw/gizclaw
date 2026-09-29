package admincmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

const e2eResourceFixtureRoot = "../../../../tests/gizclaw-e2e/testdata/resources"

var e2eResourceEnvReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-[^}]*)?\}`)

// E2E setup applies these fixtures to a live server, which can silently ignore
// fields the Schema no longer declares. Offline validation keeps them in sync.
func TestAdminValidateE2EResourceFixtures(t *testing.T) {
	var files []string
	err := filepath.WalkDir(e2eResourceFixtureRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".yaml", ".yml", ".json":
			if !entry.IsDir() {
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no resource fixtures found under %s", e2eResourceFixtureRoot)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range e2eResourceEnvReference.FindAllSubmatch(data, -1) {
			t.Setenv(string(match[1]), "e2e-placeholder")
		}
	}
	for _, file := range files {
		name, err := filepath.Rel(e2eResourceFixtureRoot, file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.ToSlash(name), func(t *testing.T) {
			cmd := NewCmd()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"validate", "-f", file})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("admin validate: %v", err)
			}
		})
	}
}
