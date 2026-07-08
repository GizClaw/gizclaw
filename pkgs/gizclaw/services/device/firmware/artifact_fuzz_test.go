package firmware

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func FuzzWriteArtifactPackage(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("not a tar archive"),
		fuzzTarPayload(f, []fuzzTarEntry{{name: "firmware.bin", body: "payload", kind: tar.TypeReg}}),
		fuzzTarPayload(f, []fuzzTarEntry{{name: "assets/readme.txt", body: "hello", kind: tar.TypeReg}}),
		fuzzTarPayload(f, []fuzzTarEntry{{name: "../bad.bin", body: "payload", kind: tar.TypeReg}}),
		fuzzTarPayload(f, []fuzzTarEntry{{name: "assets", kind: tar.TypeDir}}),
		fuzzTarPayload(f, []fuzzTarEntry{{name: "assets", body: "payload", kind: tar.TypeReg}, {name: "assets/readme.txt", body: "child", kind: tar.TypeReg}}),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 16384 {
			return
		}
		assets := newFuzzObjectStore()
		artifact, err := writeArtifactPackage(context.Background(), assets, "devkit", "stable", bytes.NewReader(data), time.Unix(0, 0))
		if err != nil {
			if !IsInvalidArtifactError(err) {
				t.Fatalf("writeArtifactPackage() error = %v, want invalid artifact or success", err)
			}
			return
		}
		if artifact.TarPath == "" || artifact.ManifestPath == "" || artifact.FilesPath == "" || artifact.Sha256 == "" {
			t.Fatalf("artifact metadata has empty path/hash fields: %+v", artifact)
		}
		manifestData, ok := assets.bytes(artifact.ManifestPath)
		if !ok {
			t.Fatalf("manifest object %q was not written", artifact.ManifestPath)
		}
		var manifest artifactManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			t.Fatalf("manifest json error = %v", err)
		}
		if len(manifest.Entries) == 0 {
			t.Fatal("accepted artifact has no manifest entries")
		}
		for _, entry := range manifest.Entries {
			normalized, err := normalizeArtifactPath(entry.Path, false)
			if err != nil {
				t.Fatalf("manifest path %q is invalid: %v", entry.Path, err)
			}
			if normalized != entry.Path {
				t.Fatalf("manifest path = %q, want normalized %q", entry.Path, normalized)
			}
			if strings.HasPrefix(entry.Path, "/") || strings.Contains(entry.Path, "..") || strings.Contains(entry.Path, "\x00") {
				t.Fatalf("unsafe manifest path accepted: %q", entry.Path)
			}
			if entry.Type == apitypes.FirmwareArtifactEntryTypeFile {
				objectPath := path.Join(artifact.FilesPath, entry.Path)
				if _, ok := assets.bytes(objectPath); !ok {
					t.Fatalf("file entry %q missing object %q", entry.Path, objectPath)
				}
			}
		}
	})
}

type fuzzTarEntry struct {
	name string
	body string
	kind byte
}

func fuzzTarPayload(tb testing.TB, entries []fuzzTarEntry) []byte {
	tb.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, entry := range entries {
		data := []byte(entry.body)
		header := &tar.Header{Name: entry.name, Mode: 0644, ModTime: modTime, Typeflag: entry.kind}
		if entry.kind == tar.TypeDir {
			header.Mode = 0755
		} else {
			header.Size = int64(len(data))
		}
		if err := tw.WriteHeader(header); err != nil {
			tb.Fatalf("WriteHeader(%s): %v", entry.name, err)
		}
		if entry.kind != tar.TypeDir {
			if _, err := tw.Write(data); err != nil {
				tb.Fatalf("Write(%s): %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatalf("Close tar: %v", err)
	}
	return buf.Bytes()
}

type fuzzObjectStore struct {
	data map[string][]byte
}

var _ objectstore.ObjectStore = (*fuzzObjectStore)(nil)

func newFuzzObjectStore() *fuzzObjectStore {
	return &fuzzObjectStore{data: make(map[string][]byte)}
}

func (s *fuzzObjectStore) bytes(name string) ([]byte, bool) {
	data, ok := s.data[name]
	return append([]byte(nil), data...), ok
}

func (s *fuzzObjectStore) Get(name string) (io.ReadCloser, error) {
	data, ok := s.bytes(name)
	if !ok {
		return nil, errArtifactNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fuzzObjectStore) Put(name string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.data[name] = append([]byte(nil), data...)
	return nil
}

func (s *fuzzObjectStore) PutWithDeadline(name string, r io.Reader, _ time.Time) error {
	return s.Put(name, r)
}

func (s *fuzzObjectStore) PutWithTTL(name string, r io.Reader, _ time.Duration) error {
	return s.Put(name, r)
}

func (s *fuzzObjectStore) Delete(name string) error {
	delete(s.data, name)
	return nil
}

func (s *fuzzObjectStore) DeletePrefix(prefix string) error {
	prefix = strings.TrimRight(prefix, "/")
	for name := range s.data {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			delete(s.data, name)
		}
	}
	return nil
}

func (s *fuzzObjectStore) List(prefix string) ([]objectstore.ObjectInfo, error) {
	prefix = strings.TrimRight(prefix, "/")
	out := make([]objectstore.ObjectInfo, 0)
	for name, data := range s.data {
		if prefix == "" || name == prefix || strings.HasPrefix(name, prefix+"/") {
			out = append(out, objectstore.ObjectInfo{Name: name, Size: int64(len(data))})
		}
	}
	return out, nil
}
