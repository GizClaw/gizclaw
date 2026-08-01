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
	RaidsVersion    = "v0.3.0"
	RaidsCommit     = "2d9a18d2d8096d6e063bde1ce3229d2898c80983"
	RaidsArchiveURL = "https://github.com/GizClaw/raids/archive/" + RaidsCommit + ".tar.gz"

	defaultRegistrationTokenName     = "default-runtime"
	expectedDefaultRegistrationToken = "28c4e4e9-a05f-5a7e-815e-9cf9afb6878f"

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
// resources with commit-addressed PIXA assets.
type RaidsResolver struct {
	cacheDir   string
	archiveURL string
	httpClient *http.Client
	pixa       *pixaResolver

	mu     sync.Mutex
	cached *Catalog
}

// NewRaidsResolver constructs a resolver without contacting the network.
func NewRaidsResolver(cacheDir, pixaCacheDir string) (*RaidsResolver, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return nil, errors.New("raids catalog: cache directory is required")
	}
	pixa, err := newPIXAResolver(pixaCacheDir)
	if err != nil {
		return nil, err
	}
	return &RaidsResolver{
		cacheDir:   cacheDir,
		archiveURL: RaidsArchiveURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		pixa:       pixa,
	}, nil
}

// Resolve returns a cache-backed, immutable catalog. A failed candidate never
// replaces a previously valid archive.
func (r *RaidsResolver) Resolve(ctx context.Context) (*Catalog, error) {
	if r == nil || r.pixa == nil || strings.TrimSpace(r.cacheDir) == "" {
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
		catalog, catalogErr := r.buildCatalog(ctx, archive)
		if catalogErr == nil {
			r.cached = catalog
			return catalog, nil
		}
		var assetErr *pixaAssetError
		if errors.As(catalogErr, &assetErr) {
			return nil, catalogErr
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
	catalog, validateErr := r.buildCatalog(ctx, candidate)
	if validateErr != nil {
		return nil, fmt.Errorf("raids catalog: validate %s: %w", RaidsVersion, validateErr)
	}
	if writeErr := r.writeCache(candidate); writeErr != nil {
		return nil, writeErr
	}
	r.cached = catalog
	return catalog, nil
}

func (r *RaidsResolver) buildCatalog(ctx context.Context, archive []byte) (*Catalog, error) {
	return buildRaidsCatalog(func(name string, width, height uint16) ([]byte, error) {
		return r.pixa.resolve(ctx, name, width, height)
	}, archive)
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

func buildRaidsCatalog(loadPIXA func(string, uint16, uint16) ([]byte, error), archive []byte) (*Catalog, error) {
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
	profileCandidate, ok := index["RuntimeProfile"][defaultRuntimeProfileName]
	if !ok {
		return nil, fmt.Errorf("Raids RuntimeProfile/%s is missing", defaultRuntimeProfileName)
	}
	profileResource, _, err := decodeResource(profileCandidate.data)
	if err != nil {
		return nil, fmt.Errorf("decode RuntimeProfile/%s: %w", defaultRuntimeProfileName, err)
	}
	profile, err := profileResource.AsRuntimeProfileResource()
	if err != nil {
		return nil, fmt.Errorf("decode RuntimeProfile/%s: %w", defaultRuntimeProfileName, err)
	}
	tokenCandidate, ok := index["RegistrationToken"][defaultRegistrationTokenName]
	if !ok {
		return nil, fmt.Errorf("Raids RegistrationToken/%s is missing", defaultRegistrationTokenName)
	}
	tokenResource, _, err := decodeResource(tokenCandidate.data)
	if err != nil {
		return nil, fmt.Errorf("decode RegistrationToken/%s: %w", defaultRegistrationTokenName, err)
	}
	token, err := tokenResource.AsRegistrationTokenResource()
	if err != nil {
		return nil, fmt.Errorf("decode RegistrationToken/%s: %w", defaultRegistrationTokenName, err)
	}
	if token.Spec.RuntimeProfileName != defaultRuntimeProfileName {
		return nil, fmt.Errorf("RegistrationToken/%s targets RuntimeProfile/%s, want %s", defaultRegistrationTokenName, token.Spec.RuntimeProfileName, defaultRuntimeProfileName)
	}
	if token.Spec.Token != expectedDefaultRegistrationToken {
		return nil, fmt.Errorf("RegistrationToken/%s has unexpected public token", defaultRegistrationTokenName)
	}
	selected, err := selectRaidsDependencies(profile, index)
	if err != nil {
		return nil, err
	}
	if err := validateWorkflowAliases(profile, selected); err != nil {
		return nil, err
	}
	if err := validateMemoryLayoutAliases(profile, selected); err != nil {
		return nil, err
	}
	mapFS := fstest.MapFS{}
	resources := make([]ResourceEntry, 0, len(selected)+2)
	requirements := map[string]EnvironmentRequirement{}
	for key, candidate := range selected {
		resourcePath := raidsCatalogPath(candidate, key)
		mapFS[resourcePath] = &fstest.MapFile{Data: candidate.data, Mode: 0o444}
		resources = append(resources, ResourceEntry{Path: resourcePath, Kind: candidate.kind, Name: candidate.name})
		if err := collectEnvironmentRequirements(candidate.data, requirements); err != nil {
			return nil, fmt.Errorf("raids catalog: collect environment requirements from %s/%s: %w", candidate.kind, candidate.name, err)
		}
	}
	petDefPIXAs, err := selectPetDefPIXAs(loadPIXA, selected, mapFS)
	if err != nil {
		return nil, err
	}
	for _, candidate := range []raidsCandidate{profileCandidate, tokenCandidate} {
		key := candidate.kind + "/" + candidate.name
		resourcePath := raidsCatalogPath(candidate, key)
		mapFS[resourcePath] = &fstest.MapFile{Data: candidate.data, Mode: 0o444}
		resources = append(resources, ResourceEntry{Path: resourcePath, Kind: candidate.kind, Name: candidate.name})
		if err := collectEnvironmentRequirements(candidate.data, requirements); err != nil {
			return nil, fmt.Errorf("raids catalog: collect environment requirements from %s/%s: %w", candidate.kind, candidate.name, err)
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })
	result := &Catalog{
		FS:                       mapFS,
		Resources:                resources,
		PetDefPIXAs:              petDefPIXAs,
		DefaultRegistrationToken: token.Spec.Token,
	}
	for _, requirement := range requirements {
		result.Requirements = append(result.Requirements, requirement)
	}
	sort.Slice(result.Requirements, func(i, j int) bool { return result.Requirements[i].Name < result.Requirements[j].Name })
	return result, nil
}

type pixaAssetError struct{ err error }

func (e *pixaAssetError) Error() string { return e.err.Error() }
func (e *pixaAssetError) Unwrap() error { return e.err }

func selectPetDefPIXAs(loadPIXA func(string, uint16, uint16) ([]byte, error), selected map[string]raidsCandidate, target fstest.MapFS) ([]PetDefPIXA, error) {
	names := make([]string, 0)
	for _, candidate := range selected {
		if candidate.kind == "PetDef" {
			names = append(names, candidate.name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)

	result := make([]PetDefPIXA, 0, len(names))
	assetOwners := map[string]string{}
	for _, name := range names {
		candidate := selected["PetDef/"+name]
		resource, _, err := decodeResource(candidate.data)
		if err != nil {
			return nil, fmt.Errorf("raids catalog: decode selected PetDef/%s: %w", name, err)
		}
		petDef, err := resource.AsPetDefResource()
		if err != nil {
			return nil, fmt.Errorf("raids catalog: decode selected PetDef/%s: %w", name, err)
		}
		const assetPrefix = "asset://codex/pets/"
		assetName := strings.TrimPrefix(petDef.Spec.Visual.Pixa.AssetRef, assetPrefix)
		if assetName == petDef.Spec.Visual.Pixa.AssetRef ||
			assetName != path.Base(assetName) ||
			path.Ext(assetName) != ".pixa" {
			return nil, fmt.Errorf("raids catalog: PetDef/%s has unsupported PIXA asset_ref %q", name, petDef.Spec.Visual.Pixa.AssetRef)
		}
		if owner := assetOwners[assetName]; owner != "" {
			return nil, fmt.Errorf("raids catalog: PetDef/%s and PetDef/%s reuse PIXA asset %s", owner, name, assetName)
		}
		assetOwners[assetName] = name
		assetPath := path.Join("assets/pet-defs", assetName)
		width := petDef.Spec.Visual.Pixa.Metadata.Canvas.Width
		height := petDef.Spec.Visual.Pixa.Metadata.Canvas.Height
		if width <= 0 || width > 1<<16-1 || height <= 0 || height > 1<<16-1 {
			return nil, fmt.Errorf("raids catalog: PetDef/%s has unsupported PIXA dimensions %dx%d", name, width, height)
		}
		if loadPIXA == nil {
			return nil, &pixaAssetError{err: fmt.Errorf("raids catalog: PIXA loader is required for PetDef/%s", name)}
		}
		data, err := loadPIXA(assetName, uint16(width), uint16(height))
		if err != nil {
			return nil, &pixaAssetError{err: fmt.Errorf("raids catalog: load PIXA for PetDef/%s from GizClaw/pixa@%s/%s: %w", name, PIXACommit, assetName, err)}
		}
		if err := validatePIXAData(data, assetName, uint16(width), uint16(height)); err != nil {
			return nil, &pixaAssetError{err: fmt.Errorf("raids catalog: validate PIXA for PetDef/%s from GizClaw/pixa@%s/%s: %w", name, PIXACommit, assetName, err)}
		}
		target[assetPath] = &fstest.MapFile{Data: data, Mode: 0o444}
		result = append(result, PetDefPIXA{PetDef: name, PIXA: assetPath})
	}
	return result, nil
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
	case ".env.example", ".gitignore", "LICENSE", "README.md", "runtime-profile.example.yaml",
		"scripts/validate_catalog.py", "tests/test_validate_catalog.py":
		return true
	}
	if strings.HasPrefix(name, ".github/workflows/") &&
		(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
		return true
	}
	for _, directory := range []string{"credentials/", "tenants/", "models/", "voices/", "workflows/", "memory-layouts/", "petdefs/", "runtime-profiles/", "registration-tokens/"} {
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
	case strings.HasPrefix(name, "memory-layouts/"):
		return "MemoryLayout", true
	case strings.HasPrefix(name, "petdefs/"):
		return "PetDef", true
	case strings.HasPrefix(name, "runtime-profiles/"):
		return "RuntimeProfile", true
	case strings.HasPrefix(name, "registration-tokens/"):
		return "RegistrationToken", true
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
	case "Credential", "Workflow", "MemoryLayout", "PetDef", "RuntimeProfile", "RegistrationToken":
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
	case "MemoryLayout":
		_, err = resource.AsMemoryLayoutResource()
	case "PetDef":
		_, err = resource.AsPetDefResource()
	case "RuntimeProfile":
		_, err = resource.AsRuntimeProfileResource()
	case "RegistrationToken":
		_, err = resource.AsRegistrationTokenResource()
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
	for _, resourceID := range []string{
		profile.Spec.Workflows.System.FriendChatroom,
		profile.Spec.Workflows.System.GroupChatroom,
		profile.Spec.Workflows.System.Pet,
	} {
		pending = append(pending, struct{ kind, name string }{"Workflow", resourceID})
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
	if profile.Spec.Resources.PetDefs != nil {
		for _, binding := range *profile.Spec.Resources.PetDefs {
			pending = append(pending, struct{ kind, name string }{"PetDef", binding.ResourceId})
		}
	}
	if profile.Spec.Resources.Memories != nil {
		for _, binding := range *profile.Spec.Resources.Memories {
			pending = append(pending, struct{ kind, name string }{"MemoryLayout", binding.LayoutId})
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
	memories := map[string]bool{}
	if profile.Spec.Resources.Memories != nil {
		for alias := range *profile.Spec.Resources.Memories {
			memories[alias] = true
		}
	}
	for _, candidate := range selected {
		if candidate.kind != "Workflow" {
			continue
		}
		modelAliases, voiceAliases, memoryAlias, err := workflowAliases(candidate.data)
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
		if memoryAlias != "" && !memories[memoryAlias] {
			return fmt.Errorf("Workflow/%s references missing memory alias %q", candidate.name, memoryAlias)
		}
	}
	return nil
}

func validateMemoryLayoutAliases(profile apitypes.RuntimeProfileResource, selected map[string]raidsCandidate) error {
	if profile.Spec.Resources.Memories == nil {
		return nil
	}
	models := map[string]bool{}
	if profile.Spec.Resources.Models != nil {
		for alias := range *profile.Spec.Resources.Models {
			models[alias] = true
		}
	}
	for alias, binding := range *profile.Spec.Resources.Memories {
		connectionType, err := binding.Connection.Discriminator()
		if err != nil {
			return fmt.Errorf("RuntimeProfile/default memory alias %q has invalid connection: %w", alias, err)
		}
		switch binding.Driver {
		case apitypes.RuntimeProfileMemoryDriverFlowcraft:
			if connectionType != "flowcraft_bbh" && connectionType != "flowcraft_object_store" && connectionType != "flowcraft_postgresql" {
				return fmt.Errorf("RuntimeProfile/default memory alias %q uses flowcraft with incompatible connection type %q", alias, connectionType)
			}
		case apitypes.RuntimeProfileMemoryDriverMem0:
			if connectionType != "mem0" {
				return fmt.Errorf("RuntimeProfile/default memory alias %q uses mem0 with incompatible connection type %q", alias, connectionType)
			}
		case apitypes.RuntimeProfileMemoryDriverVolcMem0:
			if connectionType != "volc_mem0" {
				return fmt.Errorf("RuntimeProfile/default memory alias %q uses volc_mem0 with incompatible connection type %q", alias, connectionType)
			}
		default:
			return fmt.Errorf("RuntimeProfile/default memory alias %q has unsupported driver %q", alias, binding.Driver)
		}
		if binding.Driver != apitypes.RuntimeProfileMemoryDriverFlowcraft {
			continue
		}
		candidate, exists := selected["MemoryLayout/"+strings.TrimSpace(binding.LayoutId)]
		if !exists {
			return fmt.Errorf("RuntimeProfile/default memory alias %q references missing MemoryLayout/%s", alias, binding.LayoutId)
		}
		resource, _, err := decodeResource(candidate.data)
		if err != nil {
			return fmt.Errorf("decode MemoryLayout/%s: %w", binding.LayoutId, err)
		}
		layout, err := resource.AsMemoryLayoutResource()
		if err != nil {
			return fmt.Errorf("decode MemoryLayout/%s: %w", binding.LayoutId, err)
		}
		aliases := []string{layout.Spec.Flowcraft.Extraction.Model}
		if layout.Spec.Flowcraft.Embedding != nil {
			aliases = append(aliases, layout.Spec.Flowcraft.Embedding.Model)
		}
		if layout.Spec.Flowcraft.Rerank != nil {
			aliases = append(aliases, layout.Spec.Flowcraft.Rerank.Model)
		}
		for _, modelAlias := range aliases {
			if !models[modelAlias] {
				return fmt.Errorf("MemoryLayout/%s references missing model alias %q", binding.LayoutId, modelAlias)
			}
		}
	}
	return nil
}

func workflowAliases(data []byte) ([]string, []string, string, error) {
	var document struct {
		Spec map[string]any `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, "", err
	}
	return workflowSpecAliases(document.Spec)
}

func workflowSpecAliases(spec map[string]any) ([]string, []string, string, error) {
	models := map[string]bool{}
	voices := map[string]bool{}
	memoryAlias, _ := spec["memory"].(string)
	memoryAlias = strings.TrimSpace(memoryAlias)
	add := func(set map[string]bool, value any) {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			set[text] = true
		}
	}
	driver, _ := spec["driver"].(string)
	switch driver {
	case "dashscope-realtime":
		config, ok := anyMap(spec["dashscope_realtime"])
		if !ok {
			return nil, nil, "", errors.New("dashscope-realtime workflow has no configuration")
		}
		add(models, config["model"])
		add(voices, config["voice"])
	case "doubao-realtime":
		config, ok := anyMap(spec["doubao_realtime"])
		if !ok {
			return nil, nil, "", errors.New("doubao-realtime workflow has no configuration")
		}
		add(models, config["model"])
		if audio, ok := anyMap(config["audio"]); ok {
			if output, ok := anyMap(audio["output"]); ok {
				add(voices, output["voice"])
			}
		}
	case "doubao-realtime-duplex":
		config, ok := anyMap(spec["doubao_realtime_duplex"])
		if !ok {
			return nil, nil, "", errors.New("doubao-realtime-duplex workflow has no configuration")
		}
		add(models, config["model"])
		add(voices, config["voice"])
	case "eino":
		config, ok := anyMap(spec["eino"])
		if !ok {
			return nil, nil, "", errors.New("eino workflow has no configuration")
		}
		if graph, ok := anyMap(config["graph"]); ok {
			collectEinoGraphModelAliases(graph, models)
		}
	case "ast-translate":
		config, ok := anyMap(spec["ast_translate"])
		if !ok {
			return nil, nil, "", errors.New("ast-translate workflow has no configuration")
		}
		add(models, config["translation_model"])
		if voice, ok := anyMap(config["voice"]); ok {
			add(voices, voice["tts_voice"])
		}
	case "flowcraft":
		config, ok := anyMap(spec["flowcraft"])
		if !ok {
			return nil, nil, "", errors.New("flowcraft workflow has no configuration")
		}
		if graph, ok := anyMap(config["graph"]); ok {
			for _, node := range anySlice(graph["nodes"]) {
				if node, ok := anyMap(node); ok {
					if config, ok := anyMap(node["config"]); ok {
						add(models, config["model"])
					}
				}
			}
		}
		if adapter, ok := anyMap(config["voice_adapter"]); ok {
			add(models, adapter["asr_model"])
			add(voices, adapter["default_voice"])
			if nodeVoices, ok := anyMap(adapter["node_voices"]); ok {
				for _, value := range nodeVoices {
					add(voices, value)
				}
			}
		}
	case "chatroom":
		config, ok := anyMap(spec["chatroom"])
		if !ok {
			return nil, nil, "", errors.New("chatroom workflow has no configuration")
		}
		if transcript, ok := anyMap(config["transcript"]); ok {
			add(models, transcript["asr_model"])
		}
	case "pet":
		nested, ok := anyMap(spec["pet"])
		if !ok {
			return nil, nil, "", errors.New("pet workflow has no nested workflow")
		}
		nestedModels, nestedVoices, nestedMemory, err := workflowSpecAliases(nested)
		if err != nil {
			return nil, nil, "", err
		}
		if nestedMemory != "" {
			return nil, nil, "", errors.New("pet nested workflow must not declare memory")
		}
		return nestedModels, nestedVoices, memoryAlias, nil
	default:
		return nil, nil, "", fmt.Errorf("unsupported workflow driver %q", driver)
	}
	return sortedAliases(models), sortedAliases(voices), memoryAlias, nil
}

func collectEinoGraphModelAliases(graph map[string]any, models map[string]bool) {
	for _, value := range anySlice(graph["nodes"]) {
		node, ok := anyMap(value)
		if !ok {
			continue
		}
		switch node["type"] {
		case "chat_model":
			if alias, ok := node["model"].(string); ok && strings.TrimSpace(alias) != "" {
				models[alias] = true
			}
		case "batch", "subgraph":
			if nested, ok := anyMap(node["graph"]); ok {
				collectEinoGraphModelAliases(nested, models)
			}
		case "race":
			for _, branchValue := range anySlice(node["branches"]) {
				if branch, ok := anyMap(branchValue); ok {
					if nested, ok := anyMap(branch["graph"]); ok {
						collectEinoGraphModelAliases(nested, models)
					}
				}
			}
		}
	}
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
		"Credential":        "00-credentials",
		"DashScopeTenant":   "01-tenants",
		"DeepSeekTenant":    "01-tenants",
		"GeminiTenant":      "01-tenants",
		"MiniMaxTenant":     "01-tenants",
		"OpenAITenant":      "01-tenants",
		"VolcTenant":        "01-tenants",
		"Model":             "02-models",
		"Voice":             "03-voices",
		"Workflow":          "04-workflows",
		"MemoryLayout":      "05-memory-layouts",
		"PetDef":            "05-pet-defs",
		"RuntimeProfile":    "07-runtime-profiles",
		"RegistrationToken": "08-registration-tokens",
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
