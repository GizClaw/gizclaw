package petdefscmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestOpenPixaUploadRequiresFile(t *testing.T) {
	if _, _, err := openPixaUpload(&cobra.Command{}, ""); err == nil || !strings.Contains(err.Error(), "--file") {
		t.Fatalf("openPixaUpload() error = %v", err)
	}
}

func TestOpenPixaUploadReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pet.pixa")
	if err := os.WriteFile(path, []byte("PIXA"), 0644); err != nil {
		t.Fatal(err)
	}
	body, closeFn, err := openPixaUpload(&cobra.Command{}, path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PIXA" {
		t.Fatalf("openPixaUpload() = %q", got)
	}
}

func TestOpenPixaUploadReadsStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("PIXA"))
	body, closeFn, err := openPixaUpload(cmd, "-")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PIXA" {
		t.Fatalf("openPixaUpload() = %q", got)
	}
}
