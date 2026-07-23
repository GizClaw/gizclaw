package localserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestReadRaidsArchiveRejectsUnsafeAndAcceptsPackageFiles(t *testing.T) {
	archive := testRaidsArchive(t, []tar.Header{
		{Name: "raids-0.1/", Typeflag: tar.TypeDir},
		{Name: "raids-0.1/credentials/example.yaml", Mode: 0o600, Size: 4},
	}, [][]byte{nil, []byte("test")})
	files, err := readRaidsArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(files["credentials/example.yaml"]); got != "test" {
		t.Fatalf("resource data = %q", got)
	}
	unsafe := testRaidsArchive(t, []tar.Header{{Name: "raids-0.1/../escape.yaml", Mode: 0o600, Size: 4}}, [][]byte{[]byte("test")})
	if _, err := readRaidsArchive(unsafe); err == nil {
		t.Fatal("unsafe archive accepted")
	}
}

func TestSelectRaidsDependenciesIncludesOnlyProfileClosure(t *testing.T) {
	models := map[string]apitypes.RuntimeProfileBinding{"chat": {ResourceId: "chat-model"}}
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": {ResourceId: "story-voice"}}
	profile := apitypes.RuntimeProfileResource{Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"stories": {"journey": {ResourceId: "journey"}},
		}},
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices},
	}}
	index := map[string]map[string]raidsCandidate{
		"Workflow":   {"journey": {kind: "Workflow", name: "journey"}},
		"Model":      {"chat-model": {kind: "Model", name: "chat-model", providerKind: "volc-tenant", providerName: "volc"}},
		"Voice":      {"story-voice": {kind: "Voice", name: "story-voice", providerKind: "volc-tenant", providerName: "volc"}},
		"VolcTenant": {"volc": {kind: "VolcTenant", name: "volc", credentialName: "volc-credential"}},
		"Credential": {"volc-credential": {kind: "Credential", name: "volc-credential"}},
	}
	selected, err := selectRaidsDependencies(profile, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 5 {
		t.Fatalf("selection = %#v", selected)
	}
	if _, exists := selected["Credential/volc-credential"]; !exists {
		t.Fatal("tenant credential is not selected")
	}
}

func TestRaidsResolverCachesValidatedArchive(t *testing.T) {
	profile := fstest.MapFS{
		"resources/07-runtime-profiles/00-default.yaml": {
			Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\nspec:\n  workflows: {collections: {}}\n  resources: {}\n"),
		},
	}
	archive := testRaidsArchive(t, []tar.Header{{Name: "raids-0.1/README.md", Mode: 0o600, Size: 4}}, [][]byte{[]byte("test")})
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		downloads.Add(1)
		if request.Method != http.MethodGet {
			t.Fatalf("request method = %s", request.Method)
		}
		_, _ = writer.Write(archive)
	}))
	cacheDir := t.TempDir()
	resolver, err := NewRaidsResolver(profile, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	resolver.archiveURL = server.URL
	first, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Resources) != 1 || first.Resources[0].Kind != "RuntimeProfile" {
		t.Fatalf("first catalog = %#v", first.Resources)
	}
	server.Close()

	cachedResolver, err := NewRaidsResolver(profile, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	cachedResolver.archiveURL = server.URL
	if _, err := cachedResolver.Resolve(context.Background()); err != nil {
		t.Fatalf("cached Resolve() = %v", err)
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("downloads = %d, want 1", got)
	}
}

func TestRaidsResolverReplacesExistingCacheArchive(t *testing.T) {
	cacheDir := t.TempDir()
	resolver := &RaidsResolver{cacheDir: cacheDir}
	if err := os.WriteFile(resolver.cacheFile(), []byte("invalid archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := []byte("replacement archive")
	if err := resolver.writeCache(archive); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(resolver.cacheFile())
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(archive) {
		t.Fatalf("cache archive = %q, want %q", actual, archive)
	}
}

func TestRaidsResolverReportsInvalidCacheWhenDownloadFails(t *testing.T) {
	profile := fstest.MapFS{
		"resources/07-runtime-profiles/00-default.yaml": {
			Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\nspec:\n  workflows: {collections: {}}\n  resources: {}\n"),
		},
	}
	cacheDir := t.TempDir()
	resolver, err := NewRaidsResolver(profile, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resolver.cacheFile(), []byte("invalid archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	offline := errors.New("offline")
	resolver.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, offline
	})}
	_, err = resolver.Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "validate cached archive") || !errors.Is(err, offline) {
		t.Fatalf("Resolve() error = %v, want invalid cache and download failures", err)
	}
}

func TestRaidsResolverRejectsSymlinkedCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	profile := fstest.MapFS{
		"resources/07-runtime-profiles/00-default.yaml": {
			Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\nspec:\n  workflows: {collections: {}}\n  resources: {}\n"),
		},
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
	resolver, err := NewRaidsResolver(profile, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background()); err == nil || !strings.Contains(err.Error(), "must not be a symbolic link") {
		t.Fatalf("Resolve() error = %v, want cache directory symlink rejection", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target received cache files: %v", entries)
	}
}

func TestWorkflowAliasesIncludesFlowcraftGraphAndMemoryModels(t *testing.T) {
	models, voices, err := workflowAliases([]byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {name: flowcraft-example}
spec:
  driver: flowcraft
  flowcraft:
    agent:
      graph:
        nodes:
          - config: {model: chat}
          - config: {model: extraction}
    memory:
      extract: {model: extraction}
    voice_adapter:
      asr_model: asr
      default_voice: narrator
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := models; len(got) != 3 || got[0] != "asr" || got[1] != "chat" || got[2] != "extraction" {
		t.Fatalf("models = %v", got)
	}
	if got := voices; len(got) != 1 || got[0] != "narrator" {
		t.Fatalf("voices = %v", got)
	}
}

func TestCollectEnvironmentRequirementsRejectsConflictingDefaults(t *testing.T) {
	requirements := map[string]EnvironmentRequirement{}
	if err := collectEnvironmentRequirements([]byte("one: ${RAIDS_TOKEN:-first}"), requirements); err != nil {
		t.Fatal(err)
	}
	if err := collectEnvironmentRequirements([]byte("two: ${RAIDS_TOKEN:-second}"), requirements); err == nil {
		t.Fatal("collectEnvironmentRequirements() error = nil")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func testRaidsArchive(t *testing.T, headers []tar.Header, contents [][]byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	for i := range headers {
		if err := tarWriter.WriteHeader(&headers[i]); err != nil {
			t.Fatal(err)
		}
		if len(contents[i]) != 0 {
			if _, err := tarWriter.Write(contents[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}
