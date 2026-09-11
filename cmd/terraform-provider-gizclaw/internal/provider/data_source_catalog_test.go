package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestResolveCatalogSelectsProductClosureAndAppliesOverride(t *testing.T) {
	upstream := t.TempDir()
	override := t.TempDir()
	product := t.TempDir()

	writeCatalogTestFile(t, upstream, "credentials/base.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Credential
metadata: {id: provider-credential}
spec: {body: {api_key: "${API_KEY}"}}
`)
	writeCatalogTestFile(t, upstream, "tenants/base.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Tenant
metadata: {id: provider-tenant}
spec: {credential_id: provider-credential}
`)
	writeCatalogTestFile(t, upstream, "models/base.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Model
metadata: {id: chat-model}
spec:
  kind: llm
  provider: {id: provider-tenant}
`)
	writeCatalogTestFile(t, upstream, "memory-layouts/base.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata: {id: chat-memory}
spec:
  flowcraft: {}
  mem0: {}
  volc_mem0: {}
`)
	writeCatalogTestFile(t, upstream, "workflows/base.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: chat-workflow}
spec: {driver: upstream}
`)
	writeCatalogTestFile(t, upstream, "workflows/unused.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: unused-workflow}
spec: {driver: unused}
`)
	writeCatalogTestFile(t, override, "workflows/chat.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: chat-workflow}
spec: {driver: local}
`)
	writeCatalogTestFile(t, override, "firmwares/device.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Firmware
metadata: {id: device-firmware}
spec:
  description: Device declarative firmware channels
  slots: {stable: {}, beta: {}, develop: {}}
`)
	writeCatalogTestFile(t, product, "runtime-profiles/device.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: device}
spec:
  workflows:
    collections:
      assistants:
        chat: {resource_id: chat-workflow}
  resources:
    models:
      chat: {resource_id: chat-model}
    memories:
      chat: {layout_id: chat-memory, driver: flowcraft, connection: {type: flowcraft_bbh}}
    voices: {}

`)
	writeCatalogTestFile(t, product, "registration-tokens/device.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RegistrationToken
metadata: {id: device}
spec:
  token: device-token
  runtime_profile_id: device
  firmware_id: device-firmware
`)

	resolved, err := resolveCatalog([]string{upstream, override}, []string{product})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.overriddenIDs) != 1 || resolved.overriddenIDs[0] != "Workflow/chat-workflow" {
		t.Fatalf("overridden IDs = %#v", resolved.overriddenIDs)
	}
	for stage, id := range map[string]string{
		"credentials":         "Credential/provider-credential",
		"tenants":             "Tenant/provider-tenant",
		"models":              "Model/chat-model",
		"memory_layouts":      "MemoryLayout/chat-memory",
		"workflows":           "Workflow/chat-workflow",
		"firmwares":           "Firmware/device-firmware",
		"runtime_profiles":    "RuntimeProfile/device",
		"registration_tokens": "RegistrationToken/device",
	} {
		if _, exists := resolved.byStage[stage][id]; !exists {
			t.Fatalf("%s does not contain %s", stage, id)
		}
	}
	if _, exists := resolved.byStage["workflows"]["Workflow/unused-workflow"]; exists {
		t.Fatal("unselected workflow was included")
	}
	if got := resolved.byStage["workflows"]["Workflow/chat-workflow"]; !strings.Contains(got, `"driver":"local"`) {
		t.Fatalf("override was not selected: %s", got)
	}
}

func TestResolveCatalogRejectsMissingReference(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, product, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default}
spec:
  workflows:
    collections:
      assistants:
        missing: {resource_id: missing-workflow}
  resources: {models: {}, voices: {}}
`)
	if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil {
		t.Fatal("expected missing resource error")
	}
}

func TestReadCatalogManifestRejectsLegacyOrInvalidIdentity(t *testing.T) {
	tests := map[string]string{
		"legacy name": `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: workflow-id, name: legacy-name}
spec: {driver: chatroom}
`,
		"surrounding whitespace": `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: " workflow-id "}
spec: {driver: chatroom}
`,
		"dot segment": `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: ..}
spec: {driver: chatroom}
`,
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "resource.yaml")
			if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := readCatalogManifest(path); err == nil {
				t.Fatal("invalid caller-defined ID was accepted")
			}
		})
	}
}

func TestResolveCatalogRejectsWhitespacePaddedForeignID(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, catalog, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default-profile}
spec:
  workflows: {collections: {}}
  resources: {models: {}, voices: {}}
`)
	writeCatalogTestFile(t, product, "registration-tokens/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RegistrationToken
metadata: {id: default-token}
spec: {token: default-token, runtime_profile_id: " default-profile "}
`)

	if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil ||
		!strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("error = %v, want exact foreign ID validation", err)
	}
}

func TestResolveCatalogRejectsInvalidRuntimeProfileBindingID(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, product, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default-profile}
spec:
  workflows:
    collections:
      assistants:
        invalid: {resource_id: " workflow-id "}
  resources: {models: {}, voices: {}}
`)

	if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil ||
		!strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("error = %v, want exact RuntimeProfile binding ID validation", err)
	}
}

func TestResolveCatalogSelectsWorkflowMemoryLayoutDependency(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, catalog, "memory-layouts/chat.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata: {id: chat-memory}
spec: {flowcraft: {}, mem0: {}, volc_mem0: {}}
`)
	writeCatalogTestFile(t, catalog, "workflows/chat.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: chat-workflow}
spec: {driver: flowcraft, memory: chat-memory, flowcraft: {}}
`)
	writeCatalogTestFile(t, product, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default}
spec:
  workflows:
    collections:
      assistants:
        chat: {resource_id: chat-workflow}
  resources: {models: {}, voices: {}}
`)

	resolved, err := resolveCatalog([]string{catalog}, []string{product})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := resolved.byStage["memory_layouts"]["MemoryLayout/chat-memory"]; !exists {
		t.Fatal("workflow memory layout dependency was not selected")
	}
}

func TestResolveCatalogAllowsProductTokenToSelectPublicProfile(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, catalog, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default}
spec:
  workflows: {collections: {}}
  resources: {models: {}, voices: {}}
`)
	writeCatalogTestFile(t, product, "registration-tokens/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RegistrationToken
metadata: {id: default}
spec: {token: default-token, runtime_profile_id: default}
`)

	resolved, err := resolveCatalog([]string{catalog}, []string{product})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := resolved.byStage["runtime_profiles"]["RuntimeProfile/default"]; !exists {
		t.Fatal("public RuntimeProfile/default was not selected")
	}
	if _, exists := resolved.byStage["registration_tokens"]["RegistrationToken/default"]; !exists {
		t.Fatal("product RegistrationToken/default was not selected")
	}
}

func TestResolveCatalogRejectsCatalogResourceInsideProduct(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, product, "models/model.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Model
metadata: {id: product-model}
spec: {kind: llm}
`)

	if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil {
		t.Fatal("expected product ownership error")
	}
}

func TestCatalogStagesApplyDependenciesBeforeConsumers(t *testing.T) {
	indexes := make(map[string]int, len(catalogStages))
	for index, stage := range catalogStages {
		indexes[stage.name] = index
	}
	if indexes["firmwares"] >= indexes["runtime_profiles"] {
		t.Fatal("firmwares must precede runtime_profiles")
	}
	if indexes["memory_layouts"] >= indexes["workflows"] {
		t.Fatal("memory_layouts must precede workflows")
	}
	if indexes["memory_layouts"] >= indexes["runtime_profiles"] {
		t.Fatal("memory_layouts must precede runtime_profiles")
	}
	if indexes["firmwares"] >= indexes["registration_tokens"] {
		t.Fatal("firmwares must precede registration_tokens")
	}
}

func TestResolveCatalogRejectsMissingFirmwareReference(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()
	writeCatalogTestFile(t, catalog, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default}
spec:
  workflows: {collections: {}}
  resources: {models: {}, voices: {}}
`)
	writeCatalogTestFile(t, product, "registration-tokens/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RegistrationToken
metadata: {id: default}
spec:
  token: default-token
  runtime_profile_id: default
  firmware_id: missing-firmware
`)

	if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil ||
		!strings.Contains(err.Error(), "Firmware/missing-firmware") {
		t.Fatalf("error = %v, want missing Firmware reference", err)
	}
}

func writeCatalogTestFile(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCatalogDeploysRaidTesterWithoutProfileBinding(t *testing.T) {
	catalog := t.TempDir()
	product := t.TempDir()

	writeCatalogTestFile(t, catalog, "workflows/adventure-demo/flowcraft.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: flowcraft-adventure-demo}
spec: {driver: flowcraft}
`)
	writeCatalogTestFile(t, catalog, "workflows/adventure-demo/test.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: adventure-demo-test}
spec: {driver: eino}
`)
	writeCatalogTestFile(t, catalog, "workflows/adventure-demo/raid.json", `{
  "schema": "raids.raid/v1alpha1",
  "id": "adventure-demo",
  "implementations": {"flowcraft": {"workflow_id": "flowcraft-adventure-demo"}},
  "tester": {"workflow_id": "adventure-demo-test"}
}`)
	writeCatalogTestFile(t, catalog, "workflows/adventure-unused/flowcraft.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: flowcraft-adventure-unused}
spec: {driver: flowcraft}
`)
	writeCatalogTestFile(t, catalog, "workflows/adventure-unused/test.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: adventure-unused-test}
spec: {driver: eino}
`)
	writeCatalogTestFile(t, catalog, "workflows/adventure-unused/raid.json", `{
  "schema": "raids.raid/v1alpha1",
  "id": "adventure-unused",
  "implementations": {"flowcraft": {"workflow_id": "flowcraft-adventure-unused"}},
  "tester": {"workflow_id": "adventure-unused-test"}
}`)
	writeCatalogTestFile(t, product, "runtime-profiles/demo.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: demo}
spec:
  workflows:
    collections:
      raids:
        adventure-demo: {resource_id: flowcraft-adventure-demo}
  resources: {}
`)

	resolved, err := resolveCatalog([]string{catalog}, []string{product})
	if err != nil {
		t.Fatalf("resolveCatalog: %v", err)
	}
	workflows := resolved.byStage["workflows"]
	if _, ok := workflows["Workflow/flowcraft-adventure-demo"]; !ok {
		t.Error("selected raid implementation Workflow is missing")
	}
	if _, ok := workflows["Workflow/adventure-demo-test"]; !ok {
		t.Error("tester Workflow of a selected raid must be deployed")
	}
	if _, ok := workflows["Workflow/adventure-unused-test"]; ok {
		t.Error("tester Workflow of an unselected raid must not be deployed")
	}
	if _, ok := resolved.raids["adventure-demo"]; !ok {
		t.Error("selected raid descriptor is missing from the raids output")
	}
	if _, ok := resolved.raids["adventure-unused"]; ok {
		t.Error("unselected raid descriptor must not be published")
	}
}

func TestResolveCatalogRejectsRemovedRuntimeProfileFields(t *testing.T) {
	for _, field := range []string{
		"  gameplay: {}\n",
		"  workflows: {system: {pet: pet-care}, collections: {}}\n",
		"  resources: {pet_defs: {}}\n",
		"  resources: {game_defs: {}}\n",
		"  resources: {badge_defs: {}}\n",
	} {
		t.Run(strings.TrimSpace(field), func(t *testing.T) {
			catalog, product := t.TempDir(), t.TempDir()
			writeCatalogTestFile(t, product, "runtime-profiles/old.yaml", "apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata: {id: old}\nspec:\n"+field)
			if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil || !strings.Contains(err.Error(), "removed in Runtime 0.17.0") {
				t.Fatalf("removed profile field must fail before Admin effects: %v", err)
			}
		})
	}
}

func TestResolveCatalogRejectsSelectedPetDriver(t *testing.T) {
	catalog, product := t.TempDir(), t.TempDir()
	writeCatalogTestFile(t, catalog, "workflows/pet.yaml", "apiVersion: gizclaw.admin/v1alpha1\nkind: Workflow\nmetadata: {id: pet-care}\nspec: {driver: pet}\n")
	writeCatalogTestFile(t, product, "runtime-profiles/old.yaml", "apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata: {id: old}\nspec:\n  workflows: {collections: {assistants: {pet: {resource_id: pet-care}}}}\n")
	if _, err := resolveCatalog([]string{catalog}, []string{product}); err == nil || !strings.Contains(err.Error(), "pet driver removed") {
		t.Fatalf("selected Pet driver must fail before Admin effects: %v", err)
	}
}

// The encoding is the compatibility contract with configurations that
// jsondecode these values: envelope fields in declaration order, spec keys
// sorted, numbers and placeholders as written, YAML 1.1 booleans such as yes
// decoded as true, raid.json bytes unchanged.
func TestResolveCatalogEncodingIsStable(t *testing.T) {
	catalog, product := t.TempDir(), t.TempDir()
	writeCatalogTestFile(t, catalog, "workflows/demo/flowcraft.yaml", `
spec:
  zeta: {size: 1048576, ratio: 0.5, enabled: yes, list: [b, a]}
  driver: flowcraft
  prompt: "${PROMPT:-hello}"
metadata: {id: demo-flowcraft}
kind: Workflow
apiVersion: gizclaw.admin/v1alpha1
`)
	raid := "{\n  \"schema\": \"raids.raid/v1alpha1\",\n  \"id\": \"demo\",\n  \"implementations\": {\"flowcraft\": {\"workflow_id\": \"demo-flowcraft\"}}\n}\n"
	writeCatalogTestFile(t, catalog, "workflows/demo/raid.json", raid)
	writeCatalogTestFile(t, product, "runtime-profiles/demo.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: demo}
spec:
  workflows: {collections: {raids: {demo: {resource_id: demo-flowcraft}}}}
`)

	resolved, err := resolveCatalog([]string{catalog}, []string{product})
	if err != nil {
		t.Fatal(err)
	}
	for got, want := range map[string]string{
		resolved.byStage["workflows"]["Workflow/demo-flowcraft"]:    `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workflow","metadata":{"id":"demo-flowcraft"},"spec":{"driver":"flowcraft","prompt":"${PROMPT:-hello}","zeta":{"enabled":true,"list":["b","a"],"ratio":0.5,"size":1048576}}}`,
		resolved.byStage["runtime_profiles"]["RuntimeProfile/demo"]: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"RuntimeProfile","metadata":{"id":"demo"},"spec":{"workflows":{"collections":{"raids":{"demo":{"resource_id":"demo-flowcraft"}}}}}}`,
		resolved.raids["demo"]: raid,
	} {
		if got != want {
			t.Errorf("encoded = %s\nwant      %s", got, want)
		}
	}
}

func TestCatalogDataSourceReadSetsState(t *testing.T) {
	catalog, product := t.TempDir(), t.TempDir()
	writeCatalogTestFile(t, catalog, "workflows/chat.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: chat}
spec: {driver: eino}
`)
	override := t.TempDir()
	writeCatalogTestFile(t, override, "workflows/chat.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: Workflow
metadata: {id: chat}
spec: {driver: flowcraft}
`)
	writeCatalogTestFile(t, product, "runtime-profiles/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata: {id: default}
spec:
  workflows: {collections: {assistants: {chat: {resource_id: chat}}}}
`)
	writeCatalogTestFile(t, product, "registration-tokens/default.yaml", `
apiVersion: gizclaw.admin/v1alpha1
kind: RegistrationToken
metadata: {id: default}
spec: {token: default-token, runtime_profile_id: default}
`)

	ctx := context.Background()
	// The data source is constructed without Configure: reads must not need
	// provider data or a Server connection.
	ds := newCatalogDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	objectType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attrType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	values["sources"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, catalog),
		tftypes.NewValue(tftypes.String, override),
	})
	values["product_sources"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, product),
	})
	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)}}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	ds.Read(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}

	var state catalogDataSourceModel
	if diagnostics := resp.State.Get(ctx, &state); diagnostics.HasError() {
		t.Fatalf("state diagnostics = %v", diagnostics)
	}
	stringMap := func(value types.Map) map[string]string {
		t.Helper()
		out := map[string]string{}
		if diagnostics := value.ElementsAs(ctx, &out, false); diagnostics.HasError() {
			t.Fatalf("map diagnostics = %v", diagnostics)
		}
		return out
	}
	if got := stringMap(state.Workflows)["Workflow/chat"]; !strings.Contains(got, `"driver":"flowcraft"`) {
		t.Fatalf("workflows = %v", stringMap(state.Workflows))
	}
	if _, ok := stringMap(state.RegistrationTokens)["RegistrationToken/default"]; !ok {
		t.Fatalf("registration_tokens = %v", stringMap(state.RegistrationTokens))
	}
	if _, ok := stringMap(state.RuntimeProfiles)["RuntimeProfile/default"]; !ok {
		t.Fatalf("runtime_profiles = %v", stringMap(state.RuntimeProfiles))
	}
	for name, value := range map[string]types.Map{"credentials": state.Credentials, "raids": state.Raids} {
		if value.IsNull() || value.IsUnknown() || len(value.Elements()) != 0 {
			t.Fatalf("%s = %v, want known empty map", name, value)
		}
	}
	var overridden []string
	if diagnostics := state.OverriddenIDs.ElementsAs(ctx, &overridden, false); diagnostics.HasError() || !reflect.DeepEqual(overridden, []string{"Workflow/chat"}) {
		t.Fatalf("overridden_ids = %v (%v)", overridden, diagnostics)
	}

	values["product_sources"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, filepath.Join(product, "missing")),
	})
	req.Config.Raw = tftypes.NewValue(objectType, values)
	resp = datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	ds.Read(ctx, req, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("missing product source was accepted")
	}
}
