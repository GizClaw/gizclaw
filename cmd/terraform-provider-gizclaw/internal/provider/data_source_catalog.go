package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"sigs.k8s.io/yaml"
)

// catalogStages lists the manifest directories in dependency order. name is
// the computed attribute, dir the directory under a source, catalog and
// product whether reusable catalog or product sources may define it.
var catalogStages = []struct {
	name    string
	dir     string
	catalog bool
	product bool
}{
	{name: "credentials", dir: "credentials", catalog: true},
	{name: "tenants", dir: "tenants", catalog: true},
	{name: "voices", dir: "voices", catalog: true},
	{name: "models", dir: "models", catalog: true},
	{name: "memory_layouts", dir: "memory-layouts", catalog: true},
	{name: "workflows", dir: "workflows", catalog: true},
	{name: "firmwares", dir: "firmwares", catalog: true},
	{name: "runtime_profiles", dir: "runtime-profiles", catalog: true, product: true},
	{name: "registration_tokens", dir: "registration-tokens", product: true},
}

// catalogDataSource resolves manifests from local directories only. It has no
// Configure method and never uses the Admin client, so reads open no Server
// connection.
type catalogDataSource struct{}

type catalogDataSourceModel struct {
	Sources            types.List `tfsdk:"sources"`
	ProductSources     types.List `tfsdk:"product_sources"`
	Credentials        types.Map  `tfsdk:"credentials"`
	Tenants            types.Map  `tfsdk:"tenants"`
	Voices             types.Map  `tfsdk:"voices"`
	Models             types.Map  `tfsdk:"models"`
	MemoryLayouts      types.Map  `tfsdk:"memory_layouts"`
	Workflows          types.Map  `tfsdk:"workflows"`
	Firmwares          types.Map  `tfsdk:"firmwares"`
	RuntimeProfiles    types.Map  `tfsdk:"runtime_profiles"`
	RegistrationTokens types.Map  `tfsdk:"registration_tokens"`
	Raids              types.Map  `tfsdk:"raids"`
	OverriddenIDs      types.Set  `tfsdk:"overridden_ids"`
}

// catalogManifest is the envelope each output value encodes. Spec stays an
// untyped map so the JSON encoding keeps every field as written, sorted by
// key; typed Resource models would drop or normalize fields.
type catalogManifest struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   catalogMetadata `json:"metadata"`
	Spec       map[string]any  `json:"spec"`
}

type catalogMetadata struct {
	ID   string          `json:"id"`
	Name json.RawMessage `json:"name,omitempty"`
}

type catalogEntry struct {
	manifest catalogManifest
	stage    string
	encoded  string
}

type resolvedCatalog struct {
	byStage       map[string]map[string]string
	raids         map[string]string
	overriddenIDs []string
}

// raidDescriptor is the subset of a Raids raid.json package descriptor the
// catalog needs. Implementations name the Workflow a RuntimeProfile may bind;
// the tester names the Workflow Raids runs its own suites through, which is
// deployed with the raid but never bound by a product RuntimeProfile.
type raidDescriptor struct {
	Schema          string `json:"schema"`
	ID              string `json:"id"`
	Implementations map[string]struct {
		WorkflowID string `json:"workflow_id"`
	} `json:"implementations"`
	Tester struct {
		WorkflowID string `json:"workflow_id"`
	} `json:"tester"`
}

func newCatalogDataSource() datasource.DataSource { return &catalogDataSource{} }

func (d *catalogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog"
}

func (d *catalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"sources": schema.ListAttribute{
			Required:    true,
			ElementType: types.StringType,
			Description: "Ordered reusable catalog directories from lowest to highest precedence.",
		},
		"product_sources": schema.ListAttribute{
			Required:    true,
			ElementType: types.StringType,
			Description: "Selected product directories containing RuntimeProfile and RegistrationToken resources.",
		},
		"raids": schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Raid package descriptors (raid.json) of the selected raids, keyed by raid id.",
		},
		"overridden_ids": schema.SetAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Kind/id identities replaced by a higher-precedence catalog source.",
		},
	}
	for _, stage := range catalogStages {
		attributes[stage.name] = schema.MapAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "Selected " + stage.name + " manifests encoded as JSON and keyed by Kind/id.",
		}
	}
	resp.Schema = schema.Schema{
		Description: "Resolve layered GizClaw catalogs and the dependency closure selected by product definitions.",
		Attributes:  attributes,
	}
}

func (d *catalogDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config catalogDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var sources []string
	resp.Diagnostics.Append(config.Sources.ElementsAs(ctx, &sources, false)...)
	var productSources []string
	resp.Diagnostics.Append(config.ProductSources.ElementsAs(ctx, &productSources, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resolved, err := resolveCatalog(sources, productSources)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve GizClaw catalog", err.Error())
		return
	}

	state := config
	stageValues := map[string]*types.Map{
		"credentials":         &state.Credentials,
		"tenants":             &state.Tenants,
		"voices":              &state.Voices,
		"models":              &state.Models,
		"memory_layouts":      &state.MemoryLayouts,
		"workflows":           &state.Workflows,
		"firmwares":           &state.Firmwares,
		"runtime_profiles":    &state.RuntimeProfiles,
		"registration_tokens": &state.RegistrationTokens,
	}
	for _, stage := range catalogStages {
		value, diagnostics := types.MapValueFrom(ctx, types.StringType, resolved.byStage[stage.name])
		resp.Diagnostics.Append(diagnostics...)
		*stageValues[stage.name] = value
	}
	raids, raidDiagnostics := types.MapValueFrom(ctx, types.StringType, resolved.raids)
	resp.Diagnostics.Append(raidDiagnostics...)
	state.Raids = raids
	overridden, diagnostics := types.SetValueFrom(ctx, types.StringType, resolved.overriddenIDs)
	resp.Diagnostics.Append(diagnostics...)
	state.OverriddenIDs = overridden
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func resolveCatalog(sources, productSources []string) (resolvedCatalog, error) {
	if len(sources) == 0 {
		return resolvedCatalog{}, fmt.Errorf("at least one catalog source is required")
	}
	if len(productSources) == 0 {
		return resolvedCatalog{}, fmt.Errorf("at least one product source is required")
	}

	effective := make(map[string]catalogEntry)
	overridden := make(map[string]struct{})
	if _, err := loadManifestSources(sources, effective, overridden, false); err != nil {
		return resolvedCatalog{}, err
	}
	productIDs, err := loadManifestSources(productSources, effective, nil, true)
	if err != nil {
		return resolvedCatalog{}, err
	}
	if len(productIDs) == 0 {
		return resolvedCatalog{}, fmt.Errorf("selected product sources contain no RuntimeProfile or RegistrationToken resources")
	}

	selected := make(map[string]struct{})
	profileIDs := make(map[string]struct{})
	for id := range productIDs {
		entry := effective[id]
		switch entry.stage {
		case "runtime_profiles":
			selected[id] = struct{}{}
			profileIDs[id] = struct{}{}
		case "registration_tokens":
			selected[id] = struct{}{}
			profileResourceID, _ := entry.manifest.Spec["runtime_profile_id"].(string)
			if profileResourceID == "" {
				return resolvedCatalog{}, fmt.Errorf("%s must define spec.runtime_profile_id", id)
			}
			if err := customid.ValidateResourceID(profileResourceID); err != nil {
				return resolvedCatalog{}, fmt.Errorf("%s spec.runtime_profile_id: %w", id, err)
			}
			profileIDs["RuntimeProfile/"+profileResourceID] = struct{}{}
			firmwareID, _ := entry.manifest.Spec["firmware_id"].(string)
			if firmwareID != "" {
				if err := customid.ValidateResourceID(firmwareID); err != nil {
					return resolvedCatalog{}, fmt.Errorf("%s spec.firmware_id: %w", id, err)
				}
				selected["Firmware/"+firmwareID] = struct{}{}
			}
		default:
			return resolvedCatalog{}, fmt.Errorf("product source resource %s must be a RuntimeProfile or RegistrationToken", id)
		}
	}

	for profileID := range profileIDs {
		profile, exists := effective[profileID]
		if !exists {
			return resolvedCatalog{}, fmt.Errorf("selected product references missing %s", profileID)
		}
		if profile.stage != "runtime_profiles" {
			return resolvedCatalog{}, fmt.Errorf("%s was loaded from %s, expected runtime-profiles", profileID, profile.stage)
		}
		if err := validateRuntimeProfile017(profile.manifest.Spec); err != nil {
			return resolvedCatalog{}, fmt.Errorf("%s: %w", profileID, err)
		}
		selected[profileID] = struct{}{}
		for _, id := range runtimeProfileResourceIDs(profile.manifest.Spec) {
			_, resourceID, _ := strings.Cut(id, "/")
			if err := customid.ValidateResourceID(resourceID); err != nil {
				return resolvedCatalog{}, fmt.Errorf("%s reference %s: %w", profileID, id, err)
			}
			selected[id] = struct{}{}
		}
	}
	for id := range cloneStringSet(selected) {
		entry, exists := effective[id]
		if !exists || entry.stage != "workflows" {
			continue
		}
		memoryID, _ := entry.manifest.Spec["memory"].(string)
		if memoryID != "" {
			if err := customid.ValidateResourceID(memoryID); err != nil {
				return resolvedCatalog{}, fmt.Errorf("%s spec.memory: %w", id, err)
			}
			selected["MemoryLayout/"+memoryID] = struct{}{}
		}
	}
	for id := range selected {
		if _, exists := effective[id]; !exists {
			return resolvedCatalog{}, fmt.Errorf("selected product references missing catalog resource %s", id)
		}
	}

	tenantByID := make(map[string]string)
	for id, entry := range effective {
		if strings.HasSuffix(entry.manifest.Kind, "Tenant") {
			tenantByID[entry.manifest.Metadata.ID] = id
		}
	}
	for id := range cloneStringSet(selected) {
		entry := effective[id]
		if entry.stage != "models" && entry.stage != "voices" {
			continue
		}
		provider, _ := entry.manifest.Spec["provider"].(map[string]any)
		tenantResourceID, _ := provider["id"].(string)
		if err := customid.ValidateResourceID(tenantResourceID); err != nil {
			return resolvedCatalog{}, fmt.Errorf("%s spec.provider.id: %w", id, err)
		}
		tenantID, exists := tenantByID[tenantResourceID]
		if !exists {
			return resolvedCatalog{}, fmt.Errorf("%s requires missing Tenant %q", id, tenantResourceID)
		}
		selected[tenantID] = struct{}{}
	}
	for id := range cloneStringSet(selected) {
		entry := effective[id]
		if entry.stage != "tenants" {
			continue
		}
		credentialResourceID, _ := entry.manifest.Spec["credential_id"].(string)
		if credentialResourceID == "" {
			continue
		}
		if err := customid.ValidateResourceID(credentialResourceID); err != nil {
			return resolvedCatalog{}, fmt.Errorf("%s spec.credential_id: %w", id, err)
		}
		credentialID := "Credential/" + credentialResourceID
		if _, exists := effective[credentialID]; !exists {
			return resolvedCatalog{}, fmt.Errorf("%s requires missing %s", id, credentialID)
		}
		selected[credentialID] = struct{}{}
	}

	descriptors, err := loadRaidDescriptors(sources)
	if err != nil {
		return resolvedCatalog{}, err
	}
	selectedRaids := make(map[string]string)
	for _, descriptor := range descriptors {
		used := false
		for _, implementation := range descriptor.raid.Implementations {
			if implementation.WorkflowID == "" {
				continue
			}
			if _, exists := selected["Workflow/"+implementation.WorkflowID]; exists {
				used = true
				break
			}
		}
		if !used {
			continue
		}
		selectedRaids[descriptor.raid.ID] = descriptor.encoded
		testerID := strings.TrimSpace(descriptor.raid.Tester.WorkflowID)
		if testerID == "" {
			continue
		}
		// The tester Workflow ships with the raid so Raids can exercise it on a
		// deployed cluster. It is deployed, never bound by a product RuntimeProfile.
		id := "Workflow/" + testerID
		if _, exists := effective[id]; !exists {
			return resolvedCatalog{}, fmt.Errorf("raid %q tester references missing %s", descriptor.raid.ID, id)
		}
		selected[id] = struct{}{}
	}

	result := resolvedCatalog{byStage: make(map[string]map[string]string), raids: selectedRaids}
	for _, stage := range catalogStages {
		result.byStage[stage.name] = make(map[string]string)
	}
	for id := range selected {
		entry := effective[id]
		if entry.stage == "workflows" && entry.manifest.Spec["driver"] == "pet" {
			return resolvedCatalog{}, fmt.Errorf("%s uses the pet driver removed in Runtime 0.17.0", id)
		}
		result.byStage[entry.stage][id] = entry.encoded
	}
	for id := range overridden {
		result.overriddenIDs = append(result.overriddenIDs, id)
	}
	sort.Strings(result.overriddenIDs)
	return result, nil
}

func loadManifestSources(
	sources []string,
	effective map[string]catalogEntry,
	overridden map[string]struct{},
	product bool,
) (map[string]struct{}, error) {
	loaded := make(map[string]struct{})
	for _, source := range sources {
		info, err := os.Stat(source)
		if err != nil {
			return nil, fmt.Errorf("manifest source %q: %w", source, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("manifest source %q is not a directory", source)
		}
		seen := make(map[string]string)
		for _, stage := range catalogStages {
			if (product && !stage.product) || (!product && !stage.catalog) {
				continue
			}
			root := filepath.Join(source, stage.dir)
			if _, err := os.Stat(root); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("manifest directory %q: %w", root, err)
			}
			err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
					return nil
				}
				manifest, encoded, err := readCatalogManifest(path)
				if err != nil {
					return err
				}
				if product {
					expectedKind := map[string]string{
						"runtime_profiles":    "RuntimeProfile",
						"registration_tokens": "RegistrationToken",
					}[stage.name]
					if manifest.Kind != expectedKind {
						return fmt.Errorf("product manifest %s has kind %q, expected %q", path, manifest.Kind, expectedKind)
					}
				}
				id := manifest.Kind + "/" + manifest.Metadata.ID
				if previous, exists := seen[id]; exists {
					return fmt.Errorf("catalog source %q defines %s more than once: %s and %s", source, id, previous, path)
				}
				seen[id] = path
				if _, exists := effective[id]; exists {
					if product {
						return fmt.Errorf("product source %q conflicts with existing resource %s", source, id)
					}
					overridden[id] = struct{}{}
				}
				effective[id] = catalogEntry{manifest: manifest, stage: stage.name, encoded: encoded}
				loaded[id] = struct{}{}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return loaded, nil
}

func readCatalogManifest(path string) (catalogManifest, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return catalogManifest{}, "", fmt.Errorf("read %s: %w", path, err)
	}
	var manifest catalogManifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return catalogManifest{}, "", fmt.Errorf("decode %s: %w", path, err)
	}
	if len(manifest.Metadata.Name) != 0 {
		return catalogManifest{}, "", fmt.Errorf("%s uses legacy metadata.name", path)
	}
	if manifest.APIVersion == "" || manifest.Kind == "" || manifest.Metadata.ID == "" || manifest.Spec == nil {
		return catalogManifest{}, "", fmt.Errorf("%s must define apiVersion, kind, metadata.id, and spec", path)
	}
	if err := customid.ValidateResourceID(manifest.Metadata.ID); err != nil {
		return catalogManifest{}, "", fmt.Errorf("%s metadata.id: %w", path, err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return catalogManifest{}, "", fmt.Errorf("encode %s: %w", path, err)
	}
	return manifest, string(encoded), nil
}

func runtimeProfileResourceIDs(spec map[string]any) []string {
	ids := make(map[string]struct{})
	workflows, _ := spec["workflows"].(map[string]any)
	collections, _ := workflows["collections"].(map[string]any)
	for _, rawCollection := range collections {
		collection, _ := rawCollection.(map[string]any)
		for _, rawEntry := range collection {
			if name := bindingResourceID(rawEntry); name != "" {
				ids["Workflow/"+name] = struct{}{}
			}
		}
	}
	resources, _ := spec["resources"].(map[string]any)
	for key, kind := range map[string]string{
		"models": "Model", "voices": "Voice",
	} {
		bindings, _ := resources[key].(map[string]any)
		for _, rawEntry := range bindings {
			if name := bindingResourceID(rawEntry); name != "" {
				ids[kind+"/"+name] = struct{}{}
			}
		}
	}
	memories, _ := resources["memories"].(map[string]any)
	for _, rawEntry := range memories {
		entry, _ := rawEntry.(map[string]any)
		if name, _ := entry["layout_id"].(string); name != "" {
			ids["MemoryLayout/"+name] = struct{}{}
		}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func bindingResourceID(value any) string {
	entry, _ := value.(map[string]any)
	id, _ := entry["resource_id"].(string)
	return id
}

func cloneStringSet(input map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(input))
	for key := range input {
		result[key] = struct{}{}
	}
	return result
}

var _ datasource.DataSource = (*catalogDataSource)(nil)

type loadedRaid struct {
	raid    raidDescriptor
	encoded string
}

// loadRaidDescriptors reads every raid.json package descriptor under the
// workflows directory of each catalog source. Later sources win, matching
// manifest precedence.
func loadRaidDescriptors(sources []string) ([]loadedRaid, error) {
	byID := make(map[string]loadedRaid)
	var order []string
	for _, source := range sources {
		root := filepath.Join(source, "workflows")
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("raid package directory %q: %w", root, err)
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || entry.Name() != "raid.json" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			var descriptor raidDescriptor
			if err := json.Unmarshal(raw, &descriptor); err != nil {
				return fmt.Errorf("decode %s: %w", path, err)
			}
			if err := customid.ValidateResourceID(descriptor.ID); err != nil {
				return fmt.Errorf("%s metadata id: %w", path, err)
			}
			if _, exists := byID[descriptor.ID]; !exists {
				order = append(order, descriptor.ID)
			}
			byID[descriptor.ID] = loadedRaid{raid: descriptor, encoded: string(raw)}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(order)
	loaded := make([]loadedRaid, 0, len(order))
	for _, id := range order {
		loaded = append(loaded, byID[id])
	}
	return loaded, nil
}

// Only selected profiles are checked: the pinned upstream catalog can retain
// unselected pre-0.17.0 manifests, but none may reach the Admin API.
func validateRuntimeProfile017(spec map[string]any) error {
	if _, exists := spec["gameplay"]; exists {
		return fmt.Errorf("spec.gameplay was removed in Runtime 0.17.0")
	}
	workflows, _ := spec["workflows"].(map[string]any)
	if _, exists := workflows["system"]; exists {
		return fmt.Errorf("spec.workflows.system was removed in Runtime 0.17.0")
	}
	resources, _ := spec["resources"].(map[string]any)
	for _, key := range []string{"pet_defs", "game_defs", "badge_defs"} {
		if _, exists := resources[key]; exists {
			return fmt.Errorf("spec.resources.%s was removed in Runtime 0.17.0", key)
		}
	}
	return nil
}
