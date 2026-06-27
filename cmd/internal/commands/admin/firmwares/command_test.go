package firmwarescmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenArtifactUploadFileCloseAfterClientClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.tar")
	if err := os.WriteFile(path, []byte("tar"), 0644); err != nil {
		t.Fatal(err)
	}

	r, closeFn, err := openArtifactUpload(nil, path, "")
	if err != nil {
		t.Fatal(err)
	}
	rc, ok := r.(io.Closer)
	if !ok {
		t.Fatalf("upload reader does not implement io.Closer: %T", r)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("cleanup after client close returned error: %v", err)
	}
}
