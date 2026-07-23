package localserver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
)

const (
	RaidsVersion    = "v0.1"
	RaidsArchiveURL = "https://github.com/GizClaw/raids/archive/refs/tags/v0.1.tar.gz"

	maxRaidsArchiveBytes  = 8 << 20
	maxRaidsExpandedBytes = 16 << 20
	maxRaidsFileBytes     = 1 << 20
	maxRaidsFiles         = 2048
)

// CatalogResolver provides a fully validated catalog for one local Pod
// bootstrap or runtime-contract migration.
type CatalogResolver interface {
	Resolve(context.Context) (*Catalog, error)
}

// RaidsResolver loads the fixed public Raids archive and combines its selected
// resources with the one Desktop-owned RuntimeProfile.
type RaidsResolver struct {
	profile    fs.FS
	cacheDir   string
	archiveURL string
	httpClient *http.Client

	mu     sync.Mutex
	cached *Catalog
}

// NewRaidsResolver constructs a resolver without contacting the network.
func NewRaidsResolver(profile fs.FS, cacheDir string) (*RaidsResolver, error) {
	if profile == nil {
		return nil, errors.New("raids catalog: local RuntimeProfile filesystem is required")
	}
	if _, _, err := loadDefaultRuntimeProfile(profile); err != nil {
		return nil, err
	}
	return &RaidsResolver{
		profile:    profile,
		cacheDir:   cacheDir,
		archiveURL: RaidsArchiveURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// Resolve returns a cache-backed, immutable catalog. A failed candidate never
// replaces a previously valid archive.
func (r *RaidsResolver) Resolve(ctx context.Context) (*Catalog, error) {
	if r == nil || r.profile == nil || strings.TrimSpace(r.cacheDir) == "" {
		return nil, errors.New("raids catalog: resolver is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != nil {
		return r.cached, nil
	}
	if err := r.secureCacheDir(); err != nil {
		return nil, err
	}
	archive, cacheReadErr := r.readCache()
	var cacheErr error
	if cacheReadErr == nil {
		catalog, catalogErr := buildRaidsCatalog(r.profile, archive)
		if catalogErr == nil {
			r.cached = catalog
			return catalog, nil
		}
		cacheErr = fmt.Errorf("validate cached archive: %w", catalogErr)
	} else if !os.IsNotExist(cacheReadErr) {
		cacheErr = fmt.Errorf("read cached archive: %w", cacheReadErr)
	}
	candidate, downloadErr := r.download(ctx)
	if downloadErr != nil {
		if cacheErr != nil {
			return nil, fmt.Errorf("raids catalog: load %s: %w", RaidsVersion, errors.Join(cacheErr, downloadErr))
		}
		return nil, fmt.Errorf("raids catalog: load %s: %w", RaidsVersion, downloadErr)
	}
	catalog, validateErr := buildRaidsCatalog(r.profile, candidate)
	if validateErr != nil {
		return nil, fmt.Errorf("raids catalog: validate %s: %w", RaidsVersion, validateErr)
	}
	if writeErr := r.writeCache(candidate); writeErr != nil {
		return nil, writeErr
	}
	r.cached = catalog
	return catalog, nil
}

func (r *RaidsResolver) cacheFile() string { return filepath.Join(r.cacheDir, RaidsVersion+".tar.gz") }

func (r *RaidsResolver) readCache() ([]byte, error) {
	info, err := os.Lstat(r.cacheFile())
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("cache archive must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxRaidsArchiveBytes {
		return nil, fmt.Errorf("cache archive size %d is outside 1..%d", info.Size(), maxRaidsArchiveBytes)
	}
	return os.ReadFile(r.cacheFile())
}

func (r *RaidsResolver) writeCache(data []byte) error {
	if err := r.secureCacheDir(); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(r.cacheDir, "."+RaidsVersion+"-*.tmp")
	if err != nil {
		return fmt.Errorf("raids catalog: create cache candidate: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("raids catalog: secure cache candidate: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("raids catalog: write cache candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("raids catalog: sync cache candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("raids catalog: close cache candidate: %w", err)
	}
	cacheFile := r.cacheFile()
	backupName := temporaryName + ".backup"
	hadPrevious := false
	if info, err := os.Lstat(cacheFile); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("raids catalog: cache archive must be a regular file")
		}
		if err := os.Rename(cacheFile, backupName); err != nil {
			return fmt.Errorf("raids catalog: back up cache archive: %w", err)
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("raids catalog: inspect cache archive: %w", err)
	}
	defer os.Remove(backupName)
	if err := os.Rename(temporaryName, cacheFile); err != nil {
		if hadPrevious {
			return errors.Join(
				fmt.Errorf("raids catalog: activate cache candidate: %w", err),
				os.Rename(backupName, cacheFile),
			)
		}
		return fmt.Errorf("raids catalog: activate cache candidate: %w", err)
	}
	return nil
}

func (r *RaidsResolver) secureCacheDir() error {
	info, err := os.Lstat(r.cacheDir)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("raids catalog: cache directory %q must not be a symbolic link", r.cacheDir)
		}
		if !info.IsDir() {
			return fmt.Errorf("raids catalog: cache path %q is not a directory", r.cacheDir)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("raids catalog: inspect cache directory: %w", err)
	}
	if err := os.MkdirAll(r.cacheDir, 0o700); err != nil {
		return fmt.Errorf("raids catalog: create cache directory: %w", err)
	}
	info, err = os.Lstat(r.cacheDir)
	if err != nil {
		return fmt.Errorf("raids catalog: inspect cache directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("raids catalog: cache directory %q must not be a symbolic link", r.cacheDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("raids catalog: cache path %q is not a directory", r.cacheDir)
	}
	if err := os.Chmod(r.cacheDir, 0o700); err != nil {
		return fmt.Errorf("raids catalog: secure cache directory: %w", err)
	}
	return nil
}

func (r *RaidsResolver) download(ctx context.Context) ([]byte, error) {
	client := r.httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	url := r.archiveURL
	if url == "" {
		url = RaidsArchiveURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create archive request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download archive: HTTP %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRaidsArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	if len(data) == 0 || len(data) > maxRaidsArchiveBytes {
		return nil, fmt.Errorf("archive size %d is outside 1..%d", len(data), maxRaidsArchiveBytes)
	}
	return data, nil
}

type raidsCandidate struct {
	kind           string
	name           string
	data           []byte
	providerKind   string
	providerName   string
	credentialName string
}

func buildRaidsCatalog(profileFS fs.FS, archive []byte) (*Catalog, error) {
	profileData, profile, err := loadDefaultRuntimeProfile(profileFS)
	if err != nil {
		return nil, err
	}
	files, err := readRaidsArchive(archive)
	if err != nil {
		return nil, err
	}
	index := map[string]map[string]raidsCandidate{}
	for name, data := range files {
		category, ok := raidsResourceKind(name)
		if !ok {
			continue
		}
		candidate, err := parseRaidsCandidate(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if !matchesRaidsCategory(category, candidate.kind) {
			return nil, fmt.Errorf("%s has kind %s, incompatible with %s", name, candidate.kind, category)
		}
		if index[candidate.kind] == nil {
			index[candidate.kind] = map[string]raidsCandidate{}
		}
		if _, exists := index[candidate.kind][candidate.name]; exists {
			return nil, fmt.Errorf("duplicate %s/%s", candidate.kind, candidate.name)
		}
		index[candidate.kind][candidate.name] = candidate
	}
	selected, err := selectRaidsDependencies(profile, index)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowAliases(profile, selected); err != nil {
		return nil, err
	}
	mapFS := fstest.MapFS{
		"resources/07-runtime-profiles/00-default.yaml": &fstest.MapFile{Data: profileData, Mode: 0o444},
	}
	resources := make([]ResourceEntry, 0, len(selected)+1)
	requirements := map[string]EnvironmentRequirement{}
	for key, candidate := range selected {
		resourcePath := raidsCatalogPath(candidate, key)
		mapFS[resourcePath] = &fstest.MapFile{Data: candidate.data, Mode: 0o444}
		resources = append(resources, ResourceEntry{Path: resourcePath, Kind: candidate.kind, Name: candidate.name})
		if err := collectEnvironmentRequirements(candidate.data, requirements); err != nil {
			return nil, fmt.Errorf("raids catalog: collect environment requirements from %s/%s: %w", candidate.kind, candidate.name, err)
		}
	}
	resources = append(resources, ResourceEntry{Path: "resources/07-runtime-profiles/00-default.yaml", Kind: "RuntimeProfile", Name: "default"})
	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })
	result := &Catalog{FS: mapFS, Resources: resources}
	for _, requirement := range requirements {
		result.Requirements = append(result.Requirements, requirement)
	}
	sort.Slice(result.Requirements, func(i, j int) bool { return result.Requirements[i].Name < result.Requirements[j].Name })
	return result, nil
}

func loadDefaultRuntimeProfile(source fs.FS) ([]byte, apitypes.RuntimeProfileResource, error) {
	const profilePath = "resources/07-runtime-profiles/00-default.yaml"
	data, err := fs.ReadFile(source, profilePath)
	if err != nil {
		return nil, apitypes.RuntimeProfileResource{}, fmt.Errorf("raids catalog: read local RuntimeProfile: %w", err)
	}
	resource, header, err := decodeResource(data)
	if err != nil {
		return nil, apitypes.RuntimeProfileResource{}, fmt.Errorf("raids catalog: parse local RuntimeProfile: %w", err)
	}
	if header.Kind != "RuntimeProfile" || header.Name != "default" {
		return nil, apitypes.RuntimeProfileResource{}, fmt.Errorf("raids catalog: local resource must be RuntimeProfile/default, got %s/%s", header.Kind, header.Name)
	}
	profile, err := resource.AsRuntimeProfileResource()
	if err != nil {
		return nil, apitypes.RuntimeProfileResource{}, fmt.Errorf("raids catalog: decode local RuntimeProfile: %w", err)
	}
	return data, profile, nil
}

func readRaidsArchive(archive []byte) (map[string][]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	files := map[string][]byte{}
	root := ""
	var expanded int64
	for entries := 0; ; entries++ {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read tar: %w", nextErr)
		}
		if entries >= maxRaidsFiles {
			return nil, fmt.Errorf("archive exceeds %d entries", maxRaidsFiles)
		}
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}
		rawName := strings.TrimSuffix(header.Name, "/")
		for component := range strings.SplitSeq(rawName, "/") {
			if component == ".." {
				return nil, fmt.Errorf("unsafe archive path %q", header.Name)
			}
		}
		name := path.Clean(header.Name)
		if name == "." || path.IsAbs(name) || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if header.Typeflag == tar.TypeDir && !strings.Contains(name, "/") {
			if root == "" {
				root = name
			} else if root != name {
				return nil, fmt.Errorf("archive has multiple roots %q and %q", root, name)
			}
			continue
		}
		top, relative, found := strings.Cut(name, "/")
		if !found || top == "" || relative == "" {
			return nil, fmt.Errorf("archive path %q is outside a generated root", header.Name)
		}
		if root == "" {
			root = top
		} else if root != top {
			return nil, fmt.Errorf("archive has multiple roots %q and %q", root, top)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("archive path %q is not a regular file", header.Name)
		}
		if header.Size <= 0 || header.Size > maxRaidsFileBytes {
			return nil, fmt.Errorf("archive file %q size %d is outside 1..%d", header.Name, header.Size, maxRaidsFileBytes)
		}
		expanded += header.Size
		if expanded > maxRaidsExpandedBytes {
			return nil, fmt.Errorf("archive expands beyond %d bytes", maxRaidsExpandedBytes)
		}
		if !allowedRaidsPath(relative) {
			return nil, fmt.Errorf("archive file %q is outside the Raids package layout", header.Name)
		}
		if _, exists := files[relative]; exists {
			return nil, fmt.Errorf("duplicate archive path %q", relative)
		}
		data, readErr := io.ReadAll(io.LimitReader(tarReader, maxRaidsFileBytes+1))
		if readErr != nil {
			return nil, fmt.Errorf("read archive file %q: %w", header.Name, readErr)
		}
		if len(data) != int(header.Size) {
			return nil, fmt.Errorf("archive file %q is truncated", header.Name)
		}
		files[relative] = data
	}
	if root == "" || len(files) == 0 {
		return nil, errors.New("archive has no files")
	}
	return files, nil
}

func matchesRaidsCategory(category, kind string) bool {
	if category != "Tenant" {
		return category == kind
	}
	switch kind {
	case "DashScopeTenant", "DeepSeekTenant", "GeminiTenant", "MiniMaxTenant", "OpenAITenant", "VolcTenant":
		return true
	default:
		return false
	}
}

func allowedRaidsPath(name string) bool {
	switch name {
	case ".env.example", "LICENSE", "README.md", "runtime-profile.example.yaml":
		return true
	}
	for _, directory := range []string{"credentials/", "tenants/", "models/", "voices/", "workflows/"} {
		if strings.HasPrefix(name, directory) && (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			return true
		}
	}
	return false
}

func raidsResourceKind(name string) (string, bool) {
	switch {
	case strings.HasPrefix(name, "credentials/"):
		return "Credential", true
	case strings.HasPrefix(name, "tenants/"):
		return "Tenant", true
	case strings.HasPrefix(name, "models/"):
		return "Model", true
	case strings.HasPrefix(name, "voices/"):
		return "Voice", true
	case strings.HasPrefix(name, "workflows/"):
		return "Workflow", true
	default:
		return "", false
	}
}

type resourceHeader struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
	Name string
}

func decodeResource(data []byte) (apitypes.Resource, resourceHeader, error) {
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return apitypes.Resource{}, resourceHeader{}, err
	}
	var header resourceHeader
	if err := json.Unmarshal(jsonData, &header); err != nil {
		return apitypes.Resource{}, resourceHeader{}, err
	}
	header.Kind = strings.TrimSpace(header.Kind)
	header.Metadata.Name = strings.TrimSpace(header.Metadata.Name)
	header.Name = header.Metadata.Name
	if header.APIVersion != "gizclaw.admin/v1alpha1" || header.Kind == "" || header.Name == "" {
		return apitypes.Resource{}, resourceHeader{}, errors.New("missing or invalid apiVersion, kind, or metadata.name")
	}
	var resource apitypes.Resource
	if err := json.Unmarshal(jsonData, &resource); err != nil {
		return apitypes.Resource{}, resourceHeader{}, err
	}
	if err := validateResourceKind(resource, header.Kind); err != nil {
		return apitypes.Resource{}, resourceHeader{}, err
	}
	return resource, header, nil
}

func parseRaidsCandidate(data []byte) (raidsCandidate, error) {
	_, header, err := decodeResource(data)
	if err != nil {
		return raidsCandidate{}, err
	}
	candidate := raidsCandidate{kind: header.Kind, name: header.Name, data: data}
	switch header.Kind {
	case "Credential", "Workflow":
	case "Model", "Voice":
		var spec struct {
			Provider struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"provider"`
		}
		if err := json.Unmarshal(header.Spec, &spec); err != nil {
			return raidsCandidate{}, fmt.Errorf("decode provider: %w", err)
		}
		candidate.providerKind = spec.Provider.Kind
		candidate.providerName = spec.Provider.Name
		if candidate.providerKind == "" || candidate.providerName == "" {
			return raidsCandidate{}, fmt.Errorf("%s/%s has no provider reference", header.Kind, header.Name)
		}
	case "DashScopeTenant", "DeepSeekTenant", "GeminiTenant", "MiniMaxTenant", "OpenAITenant", "VolcTenant":
		var spec struct {
			CredentialName string `json:"credential_name"`
		}
		if err := json.Unmarshal(header.Spec, &spec); err != nil {
			return raidsCandidate{}, fmt.Errorf("decode tenant: %w", err)
		}
		candidate.credentialName = spec.CredentialName
		if candidate.credentialName == "" {
			return raidsCandidate{}, fmt.Errorf("%s/%s has no credential_name", header.Kind, header.Name)
		}
	default:
		return raidsCandidate{}, fmt.Errorf("unsupported Raids resource kind %s", header.Kind)
	}
	return candidate, nil
}

func validateResourceKind(resource apitypes.Resource, kind string) error {
	var err error
	switch kind {
	case "Credential":
		_, err = resource.AsCredentialResource()
	case "DashScopeTenant":
		_, err = resource.AsDashScopeTenantResource()
	case "DeepSeekTenant":
		_, err = resource.AsDeepSeekTenantResource()
	case "GeminiTenant":
		_, err = resource.AsGeminiTenantResource()
	case "MiniMaxTenant":
		_, err = resource.AsMiniMaxTenantResource()
	case "OpenAITenant":
		_, err = resource.AsOpenAITenantResource()
	case "VolcTenant":
		_, err = resource.AsVolcTenantResource()
	case "Model":
		_, err = resource.AsModelResource()
	case "Voice":
		_, err = resource.AsVoiceResource()
	case "Workflow":
		_, err = resource.AsWorkflowResource()
	case "RuntimeProfile":
		_, err = resource.AsRuntimeProfileResource()
	default:
		return fmt.Errorf("unsupported resource kind %q", kind)
	}
	return err
}

func selectRaidsDependencies(profile apitypes.RuntimeProfileResource, index map[string]map[string]raidsCandidate) (map[string]raidsCandidate, error) {
	selected := map[string]raidsCandidate{}
	pending := make([]struct{ kind, name string }, 0)
	for _, collection := range profile.Spec.Workflows.Collections {
		for _, binding := range collection {
			pending = append(pending, struct{ kind, name string }{"Workflow", binding.ResourceId})
		}
	}
	if profile.Spec.Resources.Models != nil {
		for _, binding := range *profile.Spec.Resources.Models {
			pending = append(pending, struct{ kind, name string }{"Model", binding.ResourceId})
		}
	}
	if profile.Spec.Resources.Voices != nil {
		for _, binding := range *profile.Spec.Resources.Voices {
			pending = append(pending, struct{ kind, name string }{"Voice", binding.ResourceId})
		}
	}
	for len(pending) != 0 {
		current := pending[0]
		pending = pending[1:]
		current.name = strings.TrimSpace(current.name)
		if current.name == "" {
			return nil, fmt.Errorf("RuntimeProfile/default has an empty %s resource_id", current.kind)
		}
		key := current.kind + "/" + current.name
		if _, exists := selected[key]; exists {
			continue
		}
		candidate, exists := index[current.kind][current.name]
		if !exists {
			return nil, fmt.Errorf("RuntimeProfile/default references missing Raids %s/%s", current.kind, current.name)
		}
		selected[key] = candidate
		if candidate.providerName != "" {
			tenantKind, ok := tenantResourceKind(candidate.providerKind)
			if !ok {
				return nil, fmt.Errorf("%s/%s has unsupported provider kind %q", candidate.kind, candidate.name, candidate.providerKind)
			}
			pending = append(pending, struct{ kind, name string }{tenantKind, candidate.providerName})
		}
		if candidate.credentialName != "" {
			pending = append(pending, struct{ kind, name string }{"Credential", candidate.credentialName})
		}
	}
	return selected, nil
}

func validateWorkflowAliases(profile apitypes.RuntimeProfileResource, selected map[string]raidsCandidate) error {
	models := map[string]bool{}
	if profile.Spec.Resources.Models != nil {
		for alias := range *profile.Spec.Resources.Models {
			models[alias] = true
		}
	}
	voices := map[string]bool{}
	if profile.Spec.Resources.Voices != nil {
		for alias := range *profile.Spec.Resources.Voices {
			voices[alias] = true
		}
	}
	for _, candidate := range selected {
		if candidate.kind != "Workflow" {
			continue
		}
		modelAliases, voiceAliases, err := workflowAliases(candidate.data)
		if err != nil {
			return fmt.Errorf("parse Workflow/%s aliases: %w", candidate.name, err)
		}
		for _, alias := range modelAliases {
			if !models[alias] {
				return fmt.Errorf("Workflow/%s references missing model alias %q", candidate.name, alias)
			}
		}
		for _, alias := range voiceAliases {
			if !voices[alias] {
				return fmt.Errorf("Workflow/%s references missing Voice alias %q", candidate.name, alias)
			}
		}
	}
	return nil
}

func workflowAliases(data []byte) ([]string, []string, error) {
	var document struct {
		Spec struct {
			Driver         string         `yaml:"driver"`
			DoubaoRealtime map[string]any `yaml:"doubao_realtime"`
			ASTTranslate   map[string]any `yaml:"ast_translate"`
			Flowcraft      map[string]any `yaml:"flowcraft"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, err
	}
	models := map[string]bool{}
	voices := map[string]bool{}
	add := func(set map[string]bool, value any) {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			set[text] = true
		}
	}
	switch document.Spec.Driver {
	case "doubao-realtime":
		add(models, document.Spec.DoubaoRealtime["model"])
		if audio, ok := anyMap(document.Spec.DoubaoRealtime["audio"]); ok {
			if output, ok := anyMap(audio["output"]); ok {
				add(voices, output["voice"])
			}
		}
	case "ast-translate":
		add(models, document.Spec.ASTTranslate["translation_model"])
		if voice, ok := anyMap(document.Spec.ASTTranslate["voice"]); ok {
			add(voices, voice["tts_voice"])
		}
	case "flowcraft":
		if settings, ok := anyMap(document.Spec.Flowcraft["settings"]); ok {
			add(models, settings["extract_model"])
			add(models, settings["generate_model"])
		}
		if agent, ok := anyMap(document.Spec.Flowcraft["agent"]); ok {
			if graph, ok := anyMap(agent["graph"]); ok {
				for _, node := range anySlice(graph["nodes"]) {
					if node, ok := anyMap(node); ok {
						if config, ok := anyMap(node["config"]); ok {
							add(models, config["model"])
						}
					}
				}
			}
		}
		if memory, ok := anyMap(document.Spec.Flowcraft["memory"]); ok {
			if extract, ok := anyMap(memory["extract"]); ok {
				add(models, extract["model"])
			}
		}
		if adapter, ok := anyMap(document.Spec.Flowcraft["voice_adapter"]); ok {
			add(models, adapter["asr_model"])
			add(voices, adapter["default_voice"])
			if nodeVoices, ok := anyMap(adapter["node_voices"]); ok {
				for _, value := range nodeVoices {
					add(voices, value)
				}
			}
		}
	default:
		return nil, nil, fmt.Errorf("unsupported workflow driver %q", document.Spec.Driver)
	}
	return sortedAliases(models), sortedAliases(voices), nil
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

func anyMap(value any) (map[string]any, bool) {
	switch item := value.(type) {
	case map[string]any:
		return item, true
	case map[any]any:
		result := make(map[string]any, len(item))
		for key, value := range item {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			result[name] = value
		}
		return result, true
	default:
		return nil, false
	}
}

func sortedAliases(values map[string]bool) []string {
	aliases := make([]string, 0, len(values))
	for alias := range values {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func tenantResourceKind(providerKind string) (string, bool) {
	switch providerKind {
	case "dashscope-tenant":
		return "DashScopeTenant", true
	case "deepseek-tenant":
		return "DeepSeekTenant", true
	case "gemini-tenant":
		return "GeminiTenant", true
	case "minimax-tenant":
		return "MiniMaxTenant", true
	case "openai-tenant":
		return "OpenAITenant", true
	case "volc-tenant":
		return "VolcTenant", true
	default:
		return "", false
	}
}

func raidsCatalogPath(candidate raidsCandidate, key string) string {
	directory := map[string]string{
		"Credential":      "00-credentials",
		"DashScopeTenant": "01-tenants",
		"DeepSeekTenant":  "01-tenants",
		"GeminiTenant":    "01-tenants",
		"MiniMaxTenant":   "01-tenants",
		"OpenAITenant":    "01-tenants",
		"VolcTenant":      "01-tenants",
		"Model":           "02-models",
		"Voice":           "03-voices",
		"Workflow":        "04-workflows",
	}[candidate.kind]
	digest := sha256.Sum256([]byte(key))
	return path.Join("resources", directory, fmt.Sprintf("%x.yaml", digest[:]))
}

func collectEnvironmentRequirements(data []byte, requirements map[string]EnvironmentRequirement) error {
	for _, match := range bootstrapEnvPattern.FindAllSubmatch(data, -1) {
		name := string(match[1])
		if name == "input" {
			continue
		}
		requirement := EnvironmentRequirement{Name: name}
		if len(match[2]) != 0 {
			value := string(match[3])
			requirement.Default = &value
		}
		if previous, exists := requirements[name]; exists && !sameRequirement(previous, requirement) {
			return fmt.Errorf("environment %s has conflicting defaults", name)
		}
		requirements[name] = requirement
	}
	return nil
}
