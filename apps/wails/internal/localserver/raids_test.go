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
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestReadRaidsArchiveRejectsUnsafeAndAcceptsPackageFiles(t *testing.T) {
	archive := testRaidsArchive(t, []tar.Header{
		{Name: "raids-0.2/", Typeflag: tar.TypeDir},
		{Name: "raids-0.2/credentials/example.yaml", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/.github/workflows/validate.yml", Mode: 0o600, Size: 4},
	}, [][]byte{nil, []byte("test"), []byte("test")})
	files, err := readRaidsArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(files["credentials/example.yaml"]); got != "test" {
		t.Fatalf("resource data = %q", got)
	}
	if got := string(files[".github/workflows/validate.yml"]); got != "test" {
		t.Fatalf("workflow metadata = %q", got)
	}
	unsafe := testRaidsArchive(t, []tar.Header{{Name: "raids-0.2/../escape.yaml", Mode: 0o600, Size: 4}}, [][]byte{[]byte("test")})
	if _, err := readRaidsArchive(unsafe); err == nil {
		t.Fatal("unsafe archive accepted")
	}
}

func TestRaidsReleaseUsesCommitAddressedArchive(t *testing.T) {
	if RaidsVersion != "v0.2.2" {
		t.Fatalf("RaidsVersion = %q", RaidsVersion)
	}
	if len(RaidsCommit) != 40 || RaidsArchiveURL != "https://github.com/GizClaw/raids/archive/"+RaidsCommit+".tar.gz" {
		t.Fatalf("Raids archive pin = %q at %q", RaidsCommit, RaidsArchiveURL)
	}
}

func TestSelectRaidsDependenciesIncludesOnlyProfileClosure(t *testing.T) {
	models := map[string]apitypes.RuntimeProfileBinding{"chat": {ResourceId: "chat-model"}}
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": {ResourceId: "story-voice"}}
	petDefs := map[string]apitypes.RuntimeProfileBinding{"pet": {ResourceId: "petdef-codex"}}
	profile := apitypes.RuntimeProfileResource{Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"stories": {"journey": {ResourceId: "journey"}},
		}, System: apitypes.RuntimeProfileSystemWorkflows{
			FriendChatroom: "chatroom",
			GroupChatroom:  "chatroom",
			Pet:            "chatroom",
		}},
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices, PetDefs: &petDefs},
	}}
	index := map[string]map[string]raidsCandidate{
		"Workflow":   {"journey": {kind: "Workflow", name: "journey"}, "chatroom": {kind: "Workflow", name: "chatroom"}},
		"Model":      {"chat-model": {kind: "Model", name: "chat-model", providerKind: "volc-tenant", providerName: "volc"}},
		"Voice":      {"story-voice": {kind: "Voice", name: "story-voice", providerKind: "volc-tenant", providerName: "volc"}},
		"PetDef":     {"petdef-codex": {kind: "PetDef", name: "petdef-codex"}},
		"VolcTenant": {"volc": {kind: "VolcTenant", name: "volc", credentialName: "volc-credential"}},
		"Credential": {"volc-credential": {kind: "Credential", name: "volc-credential"}},
	}
	selected, err := selectRaidsDependencies(profile, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 7 {
		t.Fatalf("selection = %#v", selected)
	}
	if _, exists := selected["Credential/volc-credential"]; !exists {
		t.Fatal("tenant credential is not selected")
	}
}

func TestSelectPetDefPIXAsCopiesSelectedLocalAsset(t *testing.T) {
	source, err := desktopresources.LocalServer()
	if err != nil {
		t.Fatal(err)
	}
	target := fstest.MapFS{}
	selected := map[string]raidsCandidate{
		"PetDef/petdef-codex": {
			kind: "PetDef",
			name: "petdef-codex",
			data: []byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: PetDef
metadata: {name: petdef-codex}
spec:
  character: {prompt: coding mascot}
  voice: {prompt: concise}
  visual:
    refs: {images: [], videos: []}
    bindings:
      behaviors: {feed: waiting, bathe: jumping, play: running, heal: waving}
      states: {idle: idle, sick: failed, dead: failed}
    pixa:
      asset_ref: asset://codex/pets/codex.pixa
      metadata:
        version: "1"
        canvas: {width: 96, height: 104}
        clips:
          - {id: idle, pixa_clip_name: idle}
`),
		},
	}
	assets, err := selectPetDefPIXAs(source, selected, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].PetDef != "petdef-codex" || assets[0].PIXA != "assets/pet-defs/codex.pixa" {
		t.Fatalf("PetDef PIXA assets = %#v", assets)
	}
	if _, ok := target["assets/pet-defs/codex.pixa"]; !ok {
		t.Fatal("selected PIXA was not copied into the composed catalog")
	}

	candidate := selected["PetDef/petdef-codex"]
	candidate.data = bytes.ReplaceAll(candidate.data, []byte("asset://codex/pets/codex.pixa"), []byte("https://example.com/codex.pixa"))
	selected["PetDef/petdef-codex"] = candidate
	if _, err := selectPetDefPIXAs(source, selected, fstest.MapFS{}); err == nil ||
		!strings.Contains(err.Error(), "unsupported PIXA asset_ref") {
		t.Fatalf("unsupported PIXA asset_ref error = %v", err)
	}
}

func TestRaidsResolverCachesValidatedArchive(t *testing.T) {
	profile := testRuntimeProfileFS()
	archive := testMinimalRaidsArchive(t)
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
	if len(first.Resources) != 3 || first.Resources[0].Kind != "Workflow" || first.Resources[1].Kind != "RuntimeProfile" || first.Resources[2].Kind != "RegistrationToken" {
		t.Fatalf("first catalog = %#v", first.Resources)
	}
	if first.DefaultRegistrationToken != expectedDefaultRegistrationToken {
		t.Fatalf("default RegistrationToken = %q", first.DefaultRegistrationToken)
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

func TestBuildRaidsCatalogRejectsInvalidDefaultContract(t *testing.T) {
	validProfile := testRuntimeProfileFS()["resources/07-runtime-profiles/00-default.yaml"].Data
	validToken := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RegistrationToken\nmetadata:\n  name: default-runtime\nspec:\n  token: " + expectedDefaultRegistrationToken + "\n  runtime_profile_name: default\n")
	tests := []struct {
		name    string
		profile []byte
		token   []byte
		extra   map[string][]byte
		want    string
	}{
		{name: "missing profile", token: validToken, want: "RuntimeProfile/default is missing"},
		{name: "missing token", profile: validProfile, want: "RegistrationToken/default-runtime is missing"},
		{
			name:    "wrong token identity",
			profile: validProfile,
			token:   bytes.ReplaceAll(validToken, []byte("name: default-runtime"), []byte("name: another-runtime")),
			want:    "RegistrationToken/default-runtime is missing",
		},
		{
			name:    "wrong token value",
			profile: validProfile,
			token:   bytes.ReplaceAll(validToken, []byte(expectedDefaultRegistrationToken), []byte("wrong-token")),
			want:    "unexpected public token",
		},
		{
			name:    "wrong profile target",
			profile: validProfile,
			token:   bytes.ReplaceAll(validToken, []byte("runtime_profile_name: default"), []byte("runtime_profile_name: another")),
			want:    "targets RuntimeProfile/another",
		},
		{
			name:    "unresolved profile dependency",
			profile: bytes.ReplaceAll(validProfile, []byte(": chatroom"), []byte(": missing-workflow")),
			token:   validToken,
			want:    "references missing Raids Workflow/missing-workflow",
		},
		{
			name:    "duplicate token identity",
			profile: validProfile,
			token:   validToken,
			extra:   map[string][]byte{"registration-tokens/duplicate.yaml": validToken},
			want:    "duplicate RegistrationToken/default-runtime",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := testMinimalRaidsArchiveWithRoots(t, test.profile, test.token, test.extra)
			if _, err := buildRaidsCatalog(testRuntimeProfileFS(), archive); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("buildRaidsCatalog() error = %v, want %q", err, test.want)
			}
		})
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
	profile := testRuntimeProfileFS()
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
	profile := testRuntimeProfileFS()
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

func TestWorkflowAliasesIncludesChatroomAndNestedPetAliases(t *testing.T) {
	t.Run("chatroom", func(t *testing.T) {
		models, voices, err := workflowAliases([]byte(`
spec:
  driver: chatroom
  chatroom:
    transcript: {enabled: true, asr_model: asr}
`))
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 1 || models[0] != "asr" || len(voices) != 0 {
			t.Fatalf("aliases = models:%v voices:%v", models, voices)
		}
	})
	t.Run("pet", func(t *testing.T) {
		models, voices, err := workflowAliases([]byte(`
spec:
  driver: pet
  pet:
    driver: flowcraft
    flowcraft:
      agent:
        graph:
          nodes:
            - config: {model: pet-chat}
      memory:
        extract: {model: pet-extract}
      voice_adapter:
        default_voice: cute-pet
`))
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 2 || models[0] != "pet-chat" || models[1] != "pet-extract" {
			t.Fatalf("models = %v", models)
		}
		if len(voices) != 1 || voices[0] != "cute-pet" {
			t.Fatalf("voices = %v", voices)
		}
	})
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

func testRuntimeProfileFS() fstest.MapFS {
	return fstest.MapFS{
		"resources/07-runtime-profiles/00-default.yaml": {
			Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\nspec:\n  workflows:\n    system: {friend_chatroom: chatroom, group_chatroom: chatroom, pet: chatroom}\n    collections: {}\n  resources: {}\n"),
		},
	}
}

func testMinimalRaidsArchive(t *testing.T) []byte {
	t.Helper()
	profile := testRuntimeProfileFS()["resources/07-runtime-profiles/00-default.yaml"].Data
	token := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RegistrationToken\nmetadata:\n  name: default-runtime\nspec:\n  token: " + expectedDefaultRegistrationToken + "\n  runtime_profile_name: default\n")
	return testMinimalRaidsArchiveWithRoots(t, profile, token, nil)
}

func testMinimalRaidsArchiveWithRoots(t *testing.T, profile, token []byte, extra map[string][]byte) []byte {
	t.Helper()
	workflow := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Workflow\nmetadata:\n  name: chatroom\nspec:\n  driver: chatroom\n  chatroom:\n    history: {}\n")
	headers := []tar.Header{
		{Name: "raids-0.2/", Typeflag: tar.TypeDir},
		{Name: "raids-0.2/README.md", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/workflows/chatroom/social.yaml", Mode: 0o600, Size: int64(len(workflow))},
	}
	contents := [][]byte{nil, []byte("test"), workflow}
	if profile != nil {
		headers = append(headers, tar.Header{Name: "raids-0.2/runtime-profiles/default.yaml", Mode: 0o600, Size: int64(len(profile))})
		contents = append(contents, profile)
	}
	if token != nil {
		headers = append(headers, tar.Header{Name: "raids-0.2/registration-tokens/default.yaml", Mode: 0o600, Size: int64(len(token))})
		contents = append(contents, token)
	}
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := extra[name]
		headers = append(headers, tar.Header{Name: "raids-0.2/" + name, Mode: 0o600, Size: int64(len(data))})
		contents = append(contents, data)
	}
	return testRaidsArchive(t, headers, contents)
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
