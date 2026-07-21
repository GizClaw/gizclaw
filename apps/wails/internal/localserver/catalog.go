package localserver

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
)

const maxBootstrapAssetBytes = 2 << 20

var bootstrapEnvPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// EnvironmentRequirement describes one process-style value referenced by the
// bundled resource catalog. Default is nil when the value is required.
type EnvironmentRequirement struct {
	Name    string
	Default *string
}

type ResourceEntry struct {
	Path string
	Kind string
	Name string
}

type PetDefPIXA struct {
	PetDef string
	PIXA   string
}

type VoiceSync struct {
	Provider string
	Tenant   string
}

// Catalog is a validated, read-only local Server bootstrap bundle.
type Catalog struct {
	FS fs.FS

	Resources    []ResourceEntry
	Requirements []EnvironmentRequirement
	PetDefPIXAs  []PetDefPIXA
	VoiceSyncs   []VoiceSync
}

// LoadCatalog validates the embedded catalog structure and returns its ordered
// resources and asset operations.
func LoadCatalog(source fs.FS) (*Catalog, error) {
	if source == nil {
		return nil, errors.New("local server catalog: filesystem is required")
	}
	catalog := &Catalog{FS: source}
	identities := map[string]map[string]bool{}
	requirements := map[string]EnvironmentRequirement{}
	err := fs.WalkDir(source, "resources", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (path.Ext(name) != ".yaml" && path.Ext(name) != ".yml") {
			return nil
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return fmt.Errorf("local server catalog: read %s: %w", name, err)
		}
		var header struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			I18n map[string]any `yaml:"i18n"`
			Spec map[string]any `yaml:"spec"`
		}
		if err := yaml.Unmarshal(data, &header); err != nil {
			return fmt.Errorf("local server catalog: parse %s: %w", name, err)
		}
		header.Kind = strings.TrimSpace(header.Kind)
		header.Metadata.Name = strings.TrimSpace(header.Metadata.Name)
		if header.Kind == "" || header.Metadata.Name == "" {
			return fmt.Errorf("local server catalog: %s must declare kind and metadata.name", name)
		}
		if !apitypes.ResourceKind(header.Kind).Valid() {
			return fmt.Errorf("local server catalog: %s has unsupported kind %q", name, header.Kind)
		}
		if header.Kind == "Workspace" {
			return fmt.Errorf("local server catalog: %s must not bundle client-created Workspace data", name)
		}
		if header.Kind == "PetDef" {
			if err := validateCatalogI18n(name, header.Kind, header.I18n); err != nil {
				return err
			}
		}
		if identities[header.Kind] == nil {
			identities[header.Kind] = map[string]bool{}
		}
		if identities[header.Kind][header.Metadata.Name] {
			return fmt.Errorf("local server catalog: duplicate %s/%s", header.Kind, header.Metadata.Name)
		}
		identities[header.Kind][header.Metadata.Name] = true
		catalog.Resources = append(catalog.Resources, ResourceEntry{Path: name, Kind: header.Kind, Name: header.Metadata.Name})
		for _, match := range bootstrapEnvPattern.FindAllSubmatch(data, -1) {
			variable := string(match[1])
			if variable == "input" {
				continue
			}
			requirement := EnvironmentRequirement{Name: variable}
			if len(match[2]) != 0 {
				value := string(match[3])
				requirement.Default = &value
			}
			if previous, ok := requirements[variable]; ok && !sameRequirement(previous, requirement) {
				return fmt.Errorf("local server catalog: environment %s has conflicting defaults", variable)
			}
			requirements[variable] = requirement
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(catalog.Resources) == 0 {
		return nil, errors.New("local server catalog: no resource files found")
	}
	sort.Slice(catalog.Resources, func(i, j int) bool { return catalog.Resources[i].Path < catalog.Resources[j].Path })
	for _, requirement := range requirements {
		catalog.Requirements = append(catalog.Requirements, requirement)
	}
	sort.Slice(catalog.Requirements, func(i, j int) bool { return catalog.Requirements[i].Name < catalog.Requirements[j].Name })

	declaredAssets := map[string]string{}
	declareAsset := func(name, owner string) error {
		if previous := declaredAssets[name]; previous != "" {
			return fmt.Errorf("local server catalog: %s and %s reuse asset path %s", previous, owner, name)
		}
		declaredAssets[name] = owner
		return nil
	}
	petRows, err := readManifest(source, "petdef-pixa.txt", 2)
	if err != nil {
		return nil, err
	}
	for _, row := range petRows {
		item := PetDefPIXA{PetDef: row[0], PIXA: row[1]}
		if !identities["PetDef"][item.PetDef] {
			return nil, fmt.Errorf("local server catalog: PetDef PIXA references missing PetDef/%s", item.PetDef)
		}
		if err := declareAsset(item.PIXA, "PetDef/"+item.PetDef+" PIXA"); err != nil {
			return nil, err
		}
		if err := validatePIXAAsset(source, item.PIXA, 0, 0); err != nil {
			return nil, err
		}
		catalog.PetDefPIXAs = append(catalog.PetDefPIXAs, item)
	}
	if err := requireExactManifest("PetDef", identities["PetDef"], petDefNames(catalog.PetDefPIXAs)); err != nil {
		return nil, err
	}

	voiceRows, err := readManifest(source, "voice-sync.txt", 2)
	if err != nil {
		return nil, err
	}
	for _, row := range voiceRows {
		var tenantKind string
		switch row[0] {
		case "minimax":
			tenantKind = "MiniMaxTenant"
		case "volc":
			tenantKind = "VolcTenant"
		default:
			return nil, fmt.Errorf("local server catalog: unsupported voice provider %q", row[0])
		}
		if !identities[tenantKind][row[1]] {
			return nil, fmt.Errorf("local server catalog: voice sync references missing %s/%s", tenantKind, row[1])
		}
		catalog.VoiceSyncs = append(catalog.VoiceSyncs, VoiceSync{Provider: row[0], Tenant: row[1]})
	}
	if err := rejectUndeclaredAssets(source, declaredAssets); err != nil {
		return nil, err
	}
	return catalog, nil
}

func rejectUndeclaredAssets(source fs.FS, declared map[string]string) error {
	return fs.WalkDir(source, "assets", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if declared[name] == "" {
			return fmt.Errorf("local server catalog: undeclared asset %s", name)
		}
		return nil
	})
}

func validateCatalogI18n(name, kind string, catalog map[string]any) error {
	defaultLocale, _ := catalog["default_locale"].(string)
	if defaultLocale != "en" {
		return fmt.Errorf("local server catalog: %s %s i18n.default_locale must be en", name, kind)
	}
	for _, locale := range []string{"en", "zh-CN"} {
		item, ok := stringMap(catalog[locale])
		if !ok {
			return fmt.Errorf("local server catalog: %s %s must define i18n.%s", name, kind, locale)
		}
		nameField := "name"
		if kind == "PetDef" {
			nameField = "display_name"
		}
		if !nonEmptyString(item[nameField]) || !nonEmptyString(item["description"]) {
			return fmt.Errorf("local server catalog: %s %s i18n.%s is incomplete", name, kind, locale)
		}
	}
	return nil
}

func stringMap(value any) (map[string]any, bool) {
	switch item := value.(type) {
	case map[string]any:
		return item, true
	case map[any]any:
		converted := make(map[string]any, len(item))
		for key, value := range item {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			converted[name] = value
		}
		return converted, true
	default:
		return nil, false
	}
}

func nonEmptyString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func sameRequirement(left, right EnvironmentRequirement) bool {
	if left.Default == nil || right.Default == nil {
		return left.Default == nil && right.Default == nil
	}
	return *left.Default == *right.Default
}

func readManifest(source fs.FS, name string, fieldCount int) ([][]string, error) {
	data, err := fs.ReadFile(source, name)
	if err != nil {
		return nil, fmt.Errorf("local server catalog: read %s: %w", name, err)
	}
	var rows [][]string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != fieldCount {
			return nil, fmt.Errorf("local server catalog: %s:%d has %d fields, want %d", name, line, len(fields), fieldCount)
		}
		for _, field := range fields[1:] {
			if path.IsAbs(field) || path.Clean(field) != field || strings.HasPrefix(field, "../") {
				return nil, fmt.Errorf("local server catalog: %s:%d has unsafe path %q", name, line, field)
			}
		}
		rows = append(rows, fields)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("local server catalog: scan %s: %w", name, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("local server catalog: %s is empty", name)
	}
	return rows, nil
}

func validatePIXAAsset(source fs.FS, name string, expectedWidth, expectedHeight uint16) error {
	data, err := readAsset(source, name)
	if err != nil {
		return err
	}
	if len(data) < 40 || string(data[:4]) != "PIXA" {
		return fmt.Errorf("local server catalog: %s is not a PIXA asset", name)
	}
	if binary.LittleEndian.Uint16(data[4:6]) != 1 || binary.LittleEndian.Uint16(data[6:8]) < 40 {
		return fmt.Errorf("local server catalog: %s has an unsupported PIXA header", name)
	}
	width := binary.LittleEndian.Uint16(data[8:10])
	height := binary.LittleEndian.Uint16(data[10:12])
	if width == 0 || height == 0 {
		return fmt.Errorf("local server catalog: %s has an empty PIXA canvas", name)
	}
	if expectedWidth != 0 && (width != expectedWidth || height != expectedHeight) {
		return fmt.Errorf("local server catalog: %s has PIXA dimensions %dx%d, want %dx%d", name, width, height, expectedWidth, expectedHeight)
	}
	return nil
}

func readAsset(source fs.FS, name string) ([]byte, error) {
	info, err := fs.Stat(source, name)
	if err != nil {
		return nil, fmt.Errorf("local server catalog: stat %s: %w", name, err)
	}
	if info.Size() == 0 || info.Size() > maxBootstrapAssetBytes {
		return nil, fmt.Errorf("local server catalog: %s size %d is outside 1..%d", name, info.Size(), maxBootstrapAssetBytes)
	}
	data, err := fs.ReadFile(source, name)
	if err != nil {
		return nil, fmt.Errorf("local server catalog: read %s: %w", name, err)
	}
	return data, nil
}

func requireExactManifest(kind string, resources map[string]bool, manifest []string) error {
	if len(resources) != len(manifest) {
		return fmt.Errorf("local server catalog: %s asset manifest has %d entries, want %d", kind, len(manifest), len(resources))
	}
	seen := map[string]bool{}
	for _, name := range manifest {
		if seen[name] {
			return fmt.Errorf("local server catalog: duplicate %s asset mapping %s", kind, name)
		}
		seen[name] = true
	}
	for name := range resources {
		if !seen[name] {
			return fmt.Errorf("local server catalog: missing %s asset mapping %s", kind, name)
		}
	}
	return nil
}

func petDefNames(items []PetDefPIXA) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.PetDef)
	}
	return names
}
