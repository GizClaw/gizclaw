package localserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
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
