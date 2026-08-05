package localserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

//go:embed testdata/raids-v0.3.0.tar.gz
var raidsV030Archive []byte

//go:embed testdata/raids-v0.4.0.tar.gz
var raidsV040Archive []byte

const (
	raidsV030ArchiveSHA256 = "27bd688a4f61cac741685af4da871281994d4d7ec3d8103dc37d6d0d222497f9"
	raidsV040ArchiveSHA256 = "e475d93c4beb55d773dd9d3c52c1262a0b0dd413a7cf5b8e6b890548cc87a6bd"
)

// The source repository excludes these assets from redistribution. This
// metadata lets an explicitly gated smoke test pin the immutable responses
// without copying the restricted artwork into this repository.
var pinnedPIXARawAssets = map[string]struct {
	size   int
	sha256 string
}{
	"bsod.pixa":        {size: 216904, sha256: "f3230abe60a12e189c39dc8d24426af15c0b759c167e030348abf76d8f5ebb56"},
	"codex.pixa":       {size: 194782, sha256: "429d8a27b63ad259975a8adca3575d1419b5b57987b982293dee72fc9e514e0e"},
	"dewey.pixa":       {size: 181192, sha256: "fd29d0eb525ee614e3db3e74f7874cd553c7a809fee8bd48dca1ce26797ebf65"},
	"fireball.pixa":    {size: 231920, sha256: "ab8e7ea5836319eec6a04aeeb514bd171c8c961142a09aa8b0173eb22d803428"},
	"hoots.pixa":       {size: 288012, sha256: "f895bea0886806a9d6bcbf5418a184cd527dd53f39f7f19ca425faba6c41e21f"},
	"null-signal.pixa": {size: 152790, sha256: "2b996596ac93f75547aa4dbcc4724bfeccb69d9a1b33c5cd35470f20f5ee6162"},
	"rocky.pixa":       {size: 210370, sha256: "528c7d9a87a712c6275e54f9e86c0af8f5c9306a6b17ad5b0c11ff374a6a03be"},
	"seedy.pixa":       {size: 205884, sha256: "8748eea33bc935a27e6b63f186aeb112e8548217f1eff389cdf48777890e4cee"},
	"stacky.pixa":      {size: 168196, sha256: "3278be5f1e4d0b1477a2ca5dfc21562dc9a67947b3abd22dcae46bf271252747"},
}

func TestReadRaidsArchiveRejectsUnsafeAndAcceptsPackageFiles(t *testing.T) {
	archive := testRaidsArchive(t, []tar.Header{
		{Name: "raids-0.2/", Typeflag: tar.TypeDir},
		{Name: "raids-0.2/credentials/example.yaml", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/memory-layouts/default.yaml", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/.gitignore", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/.github/workflows/validate.yml", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/scripts/validate_catalog.py", Mode: 0o600, Size: 4},
		{Name: "raids-0.2/tests/test_validate_catalog.py", Mode: 0o600, Size: 4},
	}, [][]byte{nil, []byte("test"), []byte("test"), []byte("test"), []byte("test"), []byte("test"), []byte("test")})
	files, err := readRaidsArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(files["credentials/example.yaml"]); got != "test" {
		t.Fatalf("resource data = %q", got)
	}
	if got := string(files[".gitignore"]); got != "test" {
		t.Fatalf("root metadata = %q", got)
	}
	if got := string(files[".github/workflows/validate.yml"]); got != "test" {
		t.Fatalf("workflow metadata = %q", got)
	}
	if got := string(files["memory-layouts/default.yaml"]); got != "test" {
		t.Fatalf("MemoryLayout data = %q", got)
	}
	if got := string(files["scripts/validate_catalog.py"]); got != "test" {
		t.Fatalf("validation script = %q", got)
	}
	if got := string(files["tests/test_validate_catalog.py"]); got != "test" {
		t.Fatalf("validation test = %q", got)
	}
	unsafe := testRaidsArchive(t, []tar.Header{{Name: "raids-0.2/../escape.yaml", Mode: 0o600, Size: 4}}, [][]byte{[]byte("test")})
	if _, err := readRaidsArchive(unsafe); err == nil {
		t.Fatal("unsafe archive accepted")
	}
}

func TestRaidsV030FixtureIsRejectedAsLegacyNameBasedCatalog(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(raidsV030Archive)); got != raidsV030ArchiveSHA256 {
		t.Fatalf("Raids v0.3.0 fixture SHA-256 = %s, want %s", got, raidsV030ArchiveSHA256)
	}
	if _, err := buildRaidsCatalog(nil, raidsV030Archive); err == nil || !strings.Contains(err.Error(), "metadata.id") {
		t.Fatalf("buildRaidsCatalog(v0.3.0) error = %v, want metadata.id rejection", err)
	}
}

func TestRaidsV040FixtureBuildsCallerDefinedIDCatalog(t *testing.T) {
	if got := fmt.Sprintf("%x", sha256.Sum256(raidsV040Archive)); got != raidsV040ArchiveSHA256 {
		t.Fatalf("Raids v0.4.0 fixture SHA-256 = %s, want %s", got, raidsV040ArchiveSHA256)
	}
	catalog, err := buildRaidsCatalog(func(_ string, width, height uint16) ([]byte, error) {
		return testPIXA(width, height), nil
	}, raidsV040Archive)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.DefaultRegistrationToken != expectedDefaultRegistrationToken {
		t.Fatalf("default RegistrationToken = %q", catalog.DefaultRegistrationToken)
	}
	want := map[string]bool{
		"Credential/volc-credential":                                    false,
		"VolcTenant/volc-cn-beijing":                                    false,
		"Model/doubao-seed-2-0-lite":                                    false,
		"Voice/volc-tenant:volc-cn-beijing:zh_female_vv_jupiter_bigtts": false,
		"Workflow/flowcraft-chat-assistant":                             false,
		"MemoryLayout/user-chat-with-assistant":                         false,
		"PetDef/petdef-codex":                                           false,
		"RuntimeProfile/default":                                        false,
		"RegistrationToken/default-runtime":                             false,
	}
	for _, entry := range catalog.Resources {
		key := entry.Kind + "/" + entry.ID
		if _, ok := want[key]; ok {
			want[key] = true
		}
		data, readErr := fs.ReadFile(catalog.FS, entry.Path)
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Path, readErr)
		}
		if bytes.Contains(data, []byte("\n  name:")) {
			t.Fatalf("selected Raids resource %s contains legacy metadata.name", key)
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("selected Raids catalog is missing %s", key)
		}
	}
}

func TestPinnedPIXARawMediaSmoke(t *testing.T) {
	if os.Getenv("GIZCLAW_TEST_PINNED_PIXA_MEDIA") != "1" {
		t.Skip("set GIZCLAW_TEST_PINNED_PIXA_MEDIA=1 to verify the pinned GitHub media responses")
	}
	pixa, err := newPIXAResolver(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, fixture := range pinnedPIXARawAssets {
		data, err := pixa.resolve(t.Context(), name, 96, 104)
		if err != nil {
			t.Errorf("resolve %s: %v", name, err)
			continue
		}
		if len(data) != fixture.size {
			t.Errorf("pinned raw PIXA %s size = %d, want %d", name, len(data), fixture.size)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != fixture.sha256 {
			t.Errorf("pinned raw PIXA %s SHA-256 = %s, want %s", name, got, fixture.sha256)
		}
	}
}

func TestRaidsReleaseUsesCommitAddressedArchive(t *testing.T) {
	if RaidsVersion != "v0.4.0" {
		t.Fatalf("RaidsVersion = %q", RaidsVersion)
	}
	if RaidsCommit != "8ddaf0ba14c98a94638f323670e47188d6beb435" || RaidsArchiveURL != "https://github.com/GizClaw/raids/archive/"+RaidsCommit+".tar.gz" {
		t.Fatalf("Raids archive pin = %q at %q", RaidsCommit, RaidsArchiveURL)
	}
	if len(PIXACommit) != 40 || PIXAAssetBaseURL != "https://media.githubusercontent.com/media/GizClaw/pixa/"+PIXACommit+"/assets/codex-pets/" {
		t.Fatalf("PIXA asset pin = %q at %q", PIXACommit, PIXAAssetBaseURL)
	}
}

func TestSelectRaidsDependenciesIncludesOnlyProfileClosure(t *testing.T) {
	models := map[string]apitypes.RuntimeProfileBinding{"chat": {ResourceId: "chat-model"}}
	voices := map[string]apitypes.RuntimeProfileBinding{"narrator": {ResourceId: "story-voice"}}
	petDefs := map[string]apitypes.RuntimeProfileBinding{"pet": {ResourceId: "petdef-codex"}}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{"memory": testFlowcraftBBHBinding(t, "pet-memory")}
	profile := apitypes.RuntimeProfileResource{Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"stories": {"journey": {ResourceId: "journey"}},
		}, System: apitypes.RuntimeProfileSystemWorkflows{
			FriendChatroom: "chatroom",
			GroupChatroom:  "chatroom",
			Pet:            "chatroom",
		}},
		Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices, PetDefs: &petDefs, Memories: &memories},
	}}
	index := map[string]map[string]raidsCandidate{
		"Workflow":     {"journey": {kind: "Workflow", id: "journey"}, "chatroom": {kind: "Workflow", id: "chatroom"}},
		"Model":        {"chat-model": {kind: "Model", id: "chat-model", providerKind: "volc-tenant", providerID: "volc"}},
		"Voice":        {"story-voice": {kind: "Voice", id: "story-voice", providerKind: "volc-tenant", providerID: "volc"}},
		"PetDef":       {"petdef-codex": {kind: "PetDef", id: "petdef-codex"}},
		"MemoryLayout": {"pet-memory": {kind: "MemoryLayout", id: "pet-memory"}},
		"VolcTenant":   {"volc": {kind: "VolcTenant", id: "volc", credentialName: "volc-credential"}},
		"Credential":   {"volc-credential": {kind: "Credential", id: "volc-credential"}},
	}
	selected, err := selectRaidsDependencies(profile, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 8 {
		t.Fatalf("selection = %#v", selected)
	}
	if _, exists := selected["Credential/volc-credential"]; !exists {
		t.Fatal("tenant credential is not selected")
	}
}

func TestSelectPetDefPIXAsCopiesSelectedLocalAsset(t *testing.T) {
	target := fstest.MapFS{}
	selected := map[string]raidsCandidate{
		"PetDef/petdef-codex": {
			kind: "PetDef",
			id:   "petdef-codex",
			data: []byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: PetDef
metadata: {id: petdef-codex}
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
	var loadedName string
	assets, err := selectPetDefPIXAs(func(name string, width, height uint16) ([]byte, error) {
		loadedName = name
		return testPIXA(width, height), nil
	}, selected, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].PetDef != "petdef-codex" || assets[0].PIXA != "assets/pet-defs/codex.pixa" {
		t.Fatalf("PetDef PIXA assets = %#v", assets)
	}
	if _, ok := target["assets/pet-defs/codex.pixa"]; !ok {
		t.Fatal("selected PIXA was not copied into the composed catalog")
	}
	if loadedName != "codex.pixa" {
		t.Fatalf("loaded PIXA name = %q", loadedName)
	}

	candidate := selected["PetDef/petdef-codex"]
	candidate.data = bytes.ReplaceAll(candidate.data, []byte("asset://codex/pets/codex.pixa"), []byte("https://example.com/codex.pixa"))
	selected["PetDef/petdef-codex"] = candidate
	if _, err := selectPetDefPIXAs(nil, selected, fstest.MapFS{}); err == nil ||
		!strings.Contains(err.Error(), "unsupported PIXA asset_ref") {
		t.Fatalf("unsupported PIXA asset_ref error = %v", err)
	}

	candidate.data = bytes.ReplaceAll(candidate.data, []byte("https://example.com/codex.pixa"), []byte("asset://codex/pets/codex.pixa"))
	selected["PetDef/petdef-codex"] = candidate
	_, err = selectPetDefPIXAs(func(string, uint16, uint16) ([]byte, error) {
		return nil, errors.New("HTTP 404 Not Found")
	}, selected, fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "PetDef/petdef-codex") ||
		!strings.Contains(err.Error(), PIXACommit+"/codex.pixa") || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("missing PIXA source context error = %v", err)
	}
}

func TestRaidsResolverCachesValidatedArchive(t *testing.T) {
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
	resolver, err := NewRaidsResolver(cacheDir, t.TempDir())
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

	cachedResolver, err := NewRaidsResolver(cacheDir, t.TempDir())
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
	validToken := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RegistrationToken\nmetadata:\n  id: default-runtime\nspec:\n  token: " + expectedDefaultRegistrationToken + "\n  runtime_profile_id: default\n")
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
			token:   bytes.ReplaceAll(validToken, []byte("id: default-runtime"), []byte("id: another-runtime")),
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
			token:   bytes.ReplaceAll(validToken, []byte("runtime_profile_id: default"), []byte("runtime_profile_id: another")),
			want:    "targets RuntimeProfile/another",
		},
		{
			name:    "unresolved profile dependency",
			profile: bytes.ReplaceAll(validProfile, []byte(": chatroom"), []byte(": missing-workflow")),
			token:   validToken,
			want:    "references missing Raids Workflow/missing-workflow",
		},
		{
			name:    "whitespace-normalized profile dependency",
			profile: bytes.ReplaceAll(validProfile, []byte(": chatroom"), []byte(": ' chatroom '")),
			token:   validToken,
			want:    "invalid Workflow resource_id",
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
			if _, err := buildRaidsCatalog(nil, archive); err == nil || !strings.Contains(err.Error(), test.want) {
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
	cacheDir := t.TempDir()
	resolver, err := NewRaidsResolver(cacheDir, t.TempDir())
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
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(root, "cache")
	if err := os.Symlink(target, cacheDir); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewRaidsResolver(cacheDir, t.TempDir())
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

func TestWorkflowAliasesIncludesFlattenedFlowcraftGraphVoiceAndMemoryAliases(t *testing.T) {
	models, voices, memoryAlias, err := workflowAliases([]byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: flowcraft-example}
spec:
  driver: flowcraft
  memory: pet-memory
  flowcraft:
    graph:
      nodes:
        - config: {model: chat}
    voice_adapter:
      asr_model: asr
      default_voice: narrator
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := models; len(got) != 2 || got[0] != "asr" || got[1] != "chat" {
		t.Fatalf("models = %v", got)
	}
	if got := voices; len(got) != 1 || got[0] != "narrator" {
		t.Fatalf("voices = %v", got)
	}
	if memoryAlias != "pet-memory" {
		t.Fatalf("memory alias = %q", memoryAlias)
	}
}

func TestWorkflowAliasesIncludesRealtimeVoices(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		driver string
		field  string
	}{
		{name: "dashscope", driver: "dashscope-realtime", field: "dashscope_realtime"},
		{name: "doubao duplex", driver: "doubao-realtime-duplex", field: "doubao_realtime_duplex"},
	} {
		t.Run(test.name, func(t *testing.T) {
			models, voices, memoryAlias, err := workflowAliases([]byte("spec:\n  driver: " + test.driver +
				"\n  " + test.field + ":\n    model: realtime\n    voice: assistant\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(models) != 1 || models[0] != "realtime" ||
				len(voices) != 1 || voices[0] != "assistant" || memoryAlias != "" {
				t.Fatalf("aliases = models:%v voices:%v memory:%q", models, voices, memoryAlias)
			}
		})
	}
}

func TestValidateMemoryLayoutAliasesAcceptsPortableFlowcraftBBH(t *testing.T) {
	models := map[string]apitypes.RuntimeProfileBinding{
		"extraction": {ResourceId: "extraction-model"},
		"embedding":  {ResourceId: "embedding-model"},
		"rerank":     {ResourceId: "rerank-model"},
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{
		"pet-memory": testFlowcraftBBHBinding(t, "pet-memory"),
	}
	profile := apitypes.RuntimeProfileResource{Spec: apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{Models: &models, Memories: &memories},
	}}
	layout := []byte(`
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata: {id: pet-memory}
spec:
  flowcraft:
    extraction: {model: extraction, mode: two_pass}
    embedding: {model: embedding}
    rerank: {model: rerank}
    bbh: {search_overfetch: 2}
    lanes: [{name: facts, kind: note}]
    write: {mode: sync, tier: general}
  mem0: {custom_instructions: "Keep pet facts."}
  volc_mem0:
    strategies: [{name: pet, type: user_preference, custom_instructions: "Keep pet facts."}]
`)
	selected := map[string]raidsCandidate{
		"MemoryLayout/pet-memory": {kind: "MemoryLayout", id: "pet-memory", data: layout},
	}
	if err := validateMemoryLayoutAliases(profile, selected); err != nil {
		t.Fatal(err)
	}
	delete(models, "embedding")
	if err := validateMemoryLayoutAliases(profile, selected); err == nil || !strings.Contains(err.Error(), `missing model alias "embedding"`) {
		t.Fatalf("validateMemoryLayoutAliases() error = %v", err)
	}
}

func TestWorkflowAliasesIncludesChatroomAndNestedPetAliases(t *testing.T) {
	t.Run("chatroom", func(t *testing.T) {
		models, voices, memoryAlias, err := workflowAliases([]byte(`
spec:
  driver: chatroom
  chatroom:
    transcript: {enabled: true, asr_model: asr}
`))
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 1 || models[0] != "asr" || len(voices) != 0 || memoryAlias != "" {
			t.Fatalf("aliases = models:%v voices:%v memory:%q", models, voices, memoryAlias)
		}
	})
	t.Run("pet", func(t *testing.T) {
		models, voices, memoryAlias, err := workflowAliases([]byte(`
spec:
  driver: pet
  memory: pet-memory
  pet:
    driver: flowcraft
    flowcraft:
      graph:
        nodes:
          - config: {model: pet-chat}
      voice_adapter:
        default_voice: cute-pet
`))
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 1 || models[0] != "pet-chat" {
			t.Fatalf("models = %v", models)
		}
		if len(voices) != 1 || voices[0] != "cute-pet" {
			t.Fatalf("voices = %v", voices)
		}
		if memoryAlias != "pet-memory" {
			t.Fatalf("memory alias = %q", memoryAlias)
		}
	})
}

func TestValidateWorkflowAliasesRejectsMissingMemoryBinding(t *testing.T) {
	profile := apitypes.RuntimeProfileResource{}
	selected := map[string]raidsCandidate{
		"Workflow/assistant": {
			kind: "Workflow",
			id:   "assistant",
			data: []byte(`
spec:
  driver: flowcraft
  memory: missing-memory
  flowcraft:
    graph: {nodes: []}
`),
		},
	}
	err := validateWorkflowAliases(profile, selected)
	if err == nil || !strings.Contains(err.Error(), `missing memory alias "missing-memory"`) {
		t.Fatalf("validateWorkflowAliases() error = %v", err)
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

func testRuntimeProfileFS() fstest.MapFS {
	return fstest.MapFS{
		"resources/07-runtime-profiles/00-default.yaml": {
			Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  id: default\nspec:\n  workflows:\n    system: {friend_chatroom: chatroom, group_chatroom: chatroom, pet: chatroom}\n    collections: {}\n  resources: {}\n"),
		},
	}
}

func testFlowcraftBBHBinding(t *testing.T, layoutID string) apitypes.RuntimeProfileMemoryBinding {
	t.Helper()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftBBHConnection(apitypes.RuntimeProfileFlowcraftBBHConnection{
		Type: apitypes.RuntimeProfileFlowcraftBBHConnectionTypeFlowcraftBbh,
	}); err != nil {
		t.Fatal(err)
	}
	return apitypes.RuntimeProfileMemoryBinding{
		LayoutId:   layoutID,
		Driver:     apitypes.RuntimeProfileMemoryDriverFlowcraft,
		Connection: connection,
	}
}

func testMinimalRaidsArchive(t *testing.T) []byte {
	t.Helper()
	profile := testRuntimeProfileFS()["resources/07-runtime-profiles/00-default.yaml"].Data
	token := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RegistrationToken\nmetadata:\n  id: default-runtime\nspec:\n  token: " + expectedDefaultRegistrationToken + "\n  runtime_profile_id: default\n")
	return testMinimalRaidsArchiveWithRoots(t, profile, token, nil)
}

func testMinimalRaidsArchiveWithRoots(t *testing.T, profile, token []byte, extra map[string][]byte) []byte {
	t.Helper()
	workflow := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Workflow\nmetadata:\n  id: chatroom\nspec:\n  driver: chatroom\n  chatroom:\n    history: {}\n")
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
