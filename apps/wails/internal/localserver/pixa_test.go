package localserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPIXAResolverDownloadsCachesAndReusesPinnedAsset(t *testing.T) {
	asset := testPIXA(96, 104)
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		downloads.Add(1)
		if request.URL.Path != "/codex.pixa" {
			t.Errorf("asset path = %q", request.URL.Path)
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		_, _ = writer.Write(asset)
	}))
	cacheDir := t.TempDir()
	resolver, err := newPIXAResolver(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	resolver.assetBaseURL = server.URL + "/"
	actual, err := resolver.resolve(context.Background(), "codex.pixa", 96, 104)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, asset) {
		t.Fatal("downloaded PIXA changed")
	}
	commitInfo, err := os.Stat(filepath.Join(cacheDir, PIXACommit))
	if err != nil || commitInfo.Mode().Perm() != 0o700 {
		t.Fatalf("commit cache stat = %v/%v", commitInfo, err)
	}
	assetInfo, err := os.Stat(filepath.Join(cacheDir, PIXACommit, "codex.pixa"))
	if err != nil || assetInfo.Mode().Perm() != 0o600 {
		t.Fatalf("asset cache stat = %v/%v", assetInfo, err)
	}
	server.Close()

	cached, err := newPIXAResolver(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	cached.assetBaseURL = server.URL + "/"
	actual, err = cached.resolve(context.Background(), "codex.pixa", 96, 104)
	if err != nil {
		t.Fatalf("cached resolve: %v", err)
	}
	if !bytes.Equal(actual, asset) || downloads.Load() != 1 {
		t.Fatalf("cached asset/downloads = %d/%d", len(actual), downloads.Load())
	}
}

func TestPIXAResolverRejectsUnsafeNames(t *testing.T) {
	resolver, err := newPIXAResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "../codex.pixa", "pets/codex.pixa", `pets\codex.pixa`, "codex.PIXA", "codex.png", ".pixa"} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolver.resolve(context.Background(), name, 96, 104); err == nil || !strings.Contains(err.Error(), "unsafe asset name") {
				t.Fatalf("resolve(%q) error = %v", name, err)
			}
		})
	}
}

func TestPIXAResolverRejectsInvalidCandidateWithoutReplacingCache(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := newPIXAResolver(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.secureCacheDir(); err != nil {
		t.Fatal(err)
	}
	old := testPIXA(96, 104)
	if err := os.WriteFile(resolver.cacheFile("codex.pixa"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	resolver.assetBaseURL = server.URL + "/"
	_, err = resolver.resolve(context.Background(), "codex.pixa", 95, 104)
	if err == nil || !strings.Contains(err.Error(), "dimensions 96x104, want 95x104") || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("resolve error = %v", err)
	}
	actual, readErr := os.ReadFile(resolver.cacheFile("codex.pixa"))
	if readErr != nil || !bytes.Equal(actual, old) {
		t.Fatalf("cache after failed candidate = %q/%v", actual, readErr)
	}
}

func TestPIXAResolverClassifiesCandidateValidationAfterCacheError(t *testing.T) {
	cacheDir := t.TempDir()
	resolver, err := newPIXAResolver(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.secureCacheDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resolver.cacheFile("codex.pixa"), testPIXA(96, 104), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(testPIXA(94, 104))
	}))
	defer server.Close()
	resolver.assetBaseURL = server.URL + "/"
	_, err = resolver.resolve(context.Background(), "codex.pixa", 95, 104)
	if err == nil || !strings.Contains(err.Error(), "pixa assets: validate codex.pixa") ||
		!strings.Contains(err.Error(), "dimensions 96x104, want 95x104") ||
		!strings.Contains(err.Error(), "dimensions 94x104, want 95x104") {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestPIXAResolverRejectsMalformedOversizedAndWrongDimensions(t *testing.T) {
	truncatedHeader := testPIXA(96, 104)
	binary.LittleEndian.PutUint16(truncatedHeader[6:8], uint16(len(truncatedHeader)+1))
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "LFS pointer", data: []byte("version https://git-lfs.github.com/spec/v1\n"), want: "not a PIXA asset"},
		{name: "truncated header", data: truncatedHeader, want: "header size 41 larger than file size 40"},
		{name: "wrong dimensions", data: testPIXA(95, 104), want: "dimensions 95x104, want 96x104"},
		{name: "oversized", data: make([]byte, maxBootstrapAssetBytes+1), want: "asset size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write(test.data)
			}))
			defer server.Close()
			resolver, err := newPIXAResolver(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			resolver.assetBaseURL = server.URL + "/"
			if _, err := resolver.resolve(context.Background(), "codex.pixa", 96, 104); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("resolve error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPIXAResolverPropagatesCancellation(t *testing.T) {
	resolver, err := newPIXAResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resolver.httpClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.resolve(ctx, "codex.pixa", 96, 104); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve error = %v, want context cancellation", err)
	}
}

func TestPIXAResolverRejectsSymlinkedCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(root, "cache")
	if err := os.Symlink(target, cacheDir); err != nil {
		t.Fatal(err)
	}
	resolver, err := newPIXAResolver(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.resolve(context.Background(), "codex.pixa", 96, 104); err == nil || !strings.Contains(err.Error(), "must not be a symbolic link") {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestPIXAResolverRejectsSymlinkedCacheEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	resolver, err := newPIXAResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.secureCacheDir(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "codex.pixa")
	if err := os.WriteFile(target, testPIXA(96, 104), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, resolver.cacheFile("codex.pixa")); err != nil {
		t.Fatal(err)
	}
	resolver.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	if _, err := resolver.resolve(context.Background(), "codex.pixa", 96, 104); err == nil ||
		!strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestPIXAResolverRecoversInterruptedCacheReplacement(t *testing.T) {
	resolver, err := newPIXAResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.secureCacheDir(); err != nil {
		t.Fatal(err)
	}
	old := testPIXA(96, 104)
	if err := os.WriteFile(resolver.cacheBackupFile("codex.pixa"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	actual, err := resolver.resolve(context.Background(), "codex.pixa", 96, 104)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, old) {
		t.Fatal("recovered PIXA changed")
	}
	if _, err := os.Stat(resolver.cacheFile("codex.pixa")); err != nil {
		t.Fatalf("active cache after recovery: %v", err)
	}
	if _, err := os.Stat(resolver.cacheBackupFile("codex.pixa")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup after recovery: %v", err)
	}
}

func TestPIXAResolverPreservesDiscoverableBackupWhenActivationAndRestoreFail(t *testing.T) {
	resolver, err := newPIXAResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.secureCacheDir(); err != nil {
		t.Fatal(err)
	}
	old := testPIXA(96, 104)
	if err := os.WriteFile(resolver.cacheFile("codex.pixa"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	err = resolver.writeCacheWithRename("codex.pixa", testPIXA(95, 104), func(oldPath, newPath string) error {
		calls++
		switch calls {
		case 1:
			return os.Rename(oldPath, newPath)
		case 2:
			return errors.New("activation failed")
		case 3:
			return errors.New("restore failed")
		default:
			t.Errorf("unexpected rename call %d", calls)
			return nil
		}
	})
	if err == nil || !strings.Contains(err.Error(), "activation failed") ||
		!strings.Contains(err.Error(), "restore previous cache entry") ||
		!strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("writeCacheWithRename() error = %v", err)
	}
	preserved, err := os.ReadFile(resolver.cacheBackupFile("codex.pixa"))
	if err != nil || !bytes.Equal(preserved, old) {
		t.Fatalf("preserved cache backup = %d bytes, %v", len(preserved), err)
	}
	resolver.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	recovered, err := resolver.resolve(context.Background(), "codex.pixa", 96, 104)
	if err != nil || !bytes.Equal(recovered, old) {
		t.Fatalf("recovered cache = %d bytes, %v", len(recovered), err)
	}
}

func testPIXA(width, height uint16) []byte {
	data := make([]byte, 40)
	copy(data, "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], 40)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
	return data
}
