//go:build gizclaw_e2e

package terraform_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

const modelNoteEnv = "GIZCLAW_E2E_TF_MODEL_NOTE"

// lifecycleCase is one gizclaw_resource managed through every lifecycle step.
// spec builds the configured spec for a variant; probe is the spec path whose
// value differs between variants, nil when the kind has no mutable field.
type lifecycleCase struct {
	tier  int
	kind  string
	id    string
	spec  func(variant string) map[string]any
	probe []string
	// secret marks kinds whose spec the Server never returns, so the provider
	// keeps the configured spec and out-of-band changes stay invisible.
	secret bool
	// noReuse marks kinds whose deleted IDs the Server refuses to reuse.
	noReuse bool
}

func (c lifecycleCase) key() string { return c.kind + "/" + c.id }

func (c lifecycleCase) address() string {
	return fmt.Sprintf("gizclaw_resource.t%d[%q]", c.tier, c.key())
}

// lifecycleConfig is the state of terraform.tfvars.json for one step.
type lifecycleConfig struct {
	variants       map[string]string // key => variant
	inputRevisions map[string]string
	apiVersions    map[string]string
	renamed        map[string]string // key => replacement resource_id
	extra          map[string]map[string]any
}

func TestTerraformProviderResourceLifecycle(t *testing.T) {
	// Out-of-band writes go through `gizclaw admin apply`, which expands the
	// same placeholders from the process environment.
	t.Setenv(apiKeyEnv, "tf-e2e-secret")
	t.Setenv(modelNoteEnv, "noted by terraform e2e")
	env := newTerraformEnv(t)
	h := env.h
	enableSFU(t, h)
	h.CreateContext("tf-peer").MustSucceed(t)
	h.RegisterContext("tf-peer", "--sn", "tf-peer-sn").MustSucceed(t)
	cases := lifecycleCases(h.ContextPublicKey(adminContext), h.ContextPublicKey("tf-peer"))
	tf := env.root(t, "lifecycle")

	config := lifecycleConfig{variants: map[string]string{}}
	for _, c := range cases {
		config.variants[c.key()] = "a"
	}
	writeLifecycleTiers(t, tf.rootDir, cases, config)

	if !t.Run("create applies every kind", func(t *testing.T) {
		tf.apply(t)
		requireVariants(t, h, cases, config.variants)
		model := showResource(t, h, "Model/tf-lc-chat")
		if got := specString(t, model, "description"); got != "noted by terraform e2e" {
			t.Fatalf("Model description = %q, want the expanded %s", got, modelNoteEnv)
		}
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("update changes every mutable kind in place", func(t *testing.T) {
		for _, c := range cases {
			if c.probe != nil {
				config.variants[c.key()] = "b"
			}
		}
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		requireActions(t, tf.plannedActions(t), cases, func(c lifecycleCase) string {
			if c.probe == nil {
				return ""
			}
			return "update"
		})
		tf.apply(t)
		requireVariants(t, h, cases, config.variants)
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("refresh detects out-of-band changes and apply restores them", func(t *testing.T) {
		for _, c := range cases {
			if c.probe != nil {
				applyOutOfBand(t, h, c.kind, c.id, c.spec("c"))
			}
		}
		requireActions(t, tf.plannedActions(t), cases, func(c lifecycleCase) string {
			if c.probe == nil || c.secret {
				return ""
			}
			return "update"
		})
		tf.apply(t)
		for _, c := range cases {
			if c.probe != nil && !c.secret {
				requireVariant(t, h, c, "b")
			}
		}
		tf.requirePlanExitCode(t, 0)
		// A secret spec keeps the configured value, so only a configuration
		// change (or input_revision) writes it back.
		for _, c := range cases {
			if c.secret {
				requireVariant(t, h, c, "c")
			}
		}
	}) {
		return
	}

	if !t.Run("refresh drops resources deleted out of band and apply recreates them", func(t *testing.T) {
		for tier := 5; tier >= 1; tier-- {
			for _, c := range cases {
				if c.tier == tier && !c.noReuse {
					deleteOutOfBand(t, h, c.key())
				}
			}
		}
		requireActions(t, tf.plannedActions(t), cases, func(c lifecycleCase) string {
			if c.noReuse {
				return ""
			}
			return "create"
		})
		tf.apply(t)
		requireVariants(t, h, cases, config.variants)
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("import adopts every kind", func(t *testing.T) {
		for _, c := range cases {
			tf.mustRun(t, "state", "rm", "-no-color", c.address())
		}
		for _, c := range cases {
			tf.mustRun(t, "import", "-input=false", "-no-color", c.address(), c.key())
		}
		tf.apply(t)
		requireVariants(t, h, cases, config.variants)
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("input_revision re-applies an unchanged spec", func(t *testing.T) {
		credential := findCase(t, cases, "Credential/tf-lc-openai")
		// The Credential still holds the out-of-band value; a new input
		// revision writes the configured spec back.
		config.inputRevisions = map[string]string{credential.key(): strings.Repeat("a", 64)}
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		actions := tf.plannedActions(t)
		if len(actions) != 1 || actions[credential.address()] != "update" {
			t.Fatalf("planned actions = %v, want only %s update", actions, credential.address())
		}
		tf.apply(t)
		requireVariant(t, h, credential, "b")
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("explicit api_version keeps the resource", func(t *testing.T) {
		tool := findCase(t, cases, "Tool/tf-lc-weather")
		config.apiVersions = map[string]string{tool.key(): "gizclaw.admin/v1alpha1"}
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		tf.requirePlanExitCode(t, 0)
		config.apiVersions[tool.key()] = "gizclaw.admin/v2"
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		tf.mustFail(t, "gizclaw.admin/v1alpha1", "plan", "-input=false", "-no-color")
		delete(config.apiVersions, tool.key())
		writeLifecycleTiers(t, tf.rootDir, cases, config)
	}) {
		return
	}

	if !t.Run("changing resource_id replaces the resource", func(t *testing.T) {
		tool := findCase(t, cases, "Tool/tf-lc-weather")
		config.renamed = map[string]string{tool.key(): "tf-lc-weather-renamed"}
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		actions := tf.plannedActions(t)
		if len(actions) != 1 || !strings.Contains(actions[tool.address()], "delete") || !strings.Contains(actions[tool.address()], "create") {
			t.Fatalf("planned actions = %v, want %s replaced", actions, tool.address())
		}
		tf.apply(t)
		requireAbsent(t, h, tool.key())
		showResource(t, h, "Tool/tf-lc-weather-renamed")
		tf.requirePlanExitCode(t, 0)
		config.renamed = nil
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		tf.apply(t)
		requireAbsent(t, h, "Tool/tf-lc-weather-renamed")
		requireVariant(t, h, tool, config.variants[tool.key()])
	}) {
		return
	}

	if !t.Run("an unset environment placeholder fails the apply", func(t *testing.T) {
		model := findCase(t, cases, "Model/tf-lc-chat")
		config.variants[model.key()] = "c"
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		tf.withEnv(modelNoteEnv).mustFail(t, "environment variable "+modelNoteEnv+" is required",
			"apply", "-input=false", "-no-color", "-auto-approve")
		requireVariant(t, h, model, "b")
		config.variants[model.key()] = "b"
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		// The failed run refreshed without the variable, so the placeholder
		// could not be matched and state recorded the Server value. The next
		// run re-applies the same content once, then converges.
		actions := tf.plannedActions(t)
		if len(actions) != 1 || actions[model.address()] != "update" {
			t.Fatalf("planned actions = %v, want only %s update", actions, model.address())
		}
		tf.apply(t)
		requireVariant(t, h, model, "b")
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("invalid configuration fails before changing the Server", func(t *testing.T) {
		for name, tc := range map[string]struct {
			entry  map[string]any
			want   string
			absent string
		}{
			"ResourceList kind":    {map[string]any{"kind": "ResourceList", "resource_id": "tf-lc-list", "spec": `{}`}, "cannot be addressed by ID", ""},
			"unknown kind":         {map[string]any{"kind": "Gadget", "resource_id": "tf-lc-gadget", "spec": `{}`}, `unknown resource kind "Gadget"`, ""},
			"padded resource_id":   {map[string]any{"kind": "Tool", "resource_id": " tf-lc-padded", "spec": `{}`}, "surrounding whitespace", "Tool/tf-lc-padded"},
			"bad input_revision":   {map[string]any{"kind": "Tool", "resource_id": "tf-lc-rev", "spec": `{}`, "input_revision": "not-a-digest"}, "SHA-256", "Tool/tf-lc-rev"},
			"non-object spec":      {map[string]any{"kind": "Tool", "resource_id": "tf-lc-array", "spec": `["x"]`}, "spec must be a JSON object", "Tool/tf-lc-array"},
			"Peer-owned Workspace": {map[string]any{"kind": "Workspace", "resource_id": "tf-lc-workspace", "spec": `{"name":"tf-lc-workspace","workflow_id":"tf-lc-echo"}`}, "UNSUPPORTED_WORKSPACE_APPLY", "Workspace/tf-lc-workspace"},
			"Server rejects spec":  {map[string]any{"kind": "Workflow", "resource_id": "tf-lc-invalid", "spec": `{"driver":"missing-driver"}`}, "INVALID_WORKFLOW_RESOURCE: unsupported driver", "Workflow/tf-lc-invalid"},
		} {
			t.Run(name, func(t *testing.T) {
				config.extra = map[string]map[string]any{"Invalid/entry": tc.entry}
				writeLifecycleTiers(t, tf.rootDir, cases, config)
				tf.mustFail(t, tc.want, "apply", "-input=false", "-no-color", "-auto-approve")
				if tc.absent != "" {
					requireAbsent(t, h, tc.absent)
				}
			})
		}
		config.extra = nil
		writeLifecycleTiers(t, tf.rootDir, cases, config)
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("a restarted Server is reached by the next run", func(t *testing.T) {
		h.RestartServer()
		// Peers live in the fixture's in-memory store; register them again.
		h.RegisterContext(adminContext, "--sn", "tf-admin-sn").MustSucceed(t)
		h.RegisterContext("tf-peer", "--sn", "tf-peer-sn").MustSucceed(t)
		tf.apply(t)
		requireVariants(t, h, cases, config.variants)
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("destroy deletes every kind and tolerates already absent resources", func(t *testing.T) {
		deleteOutOfBand(t, h, "RegistrationToken/tf-lc-device")
		tf.mustRun(t, "destroy", "-input=false", "-no-color", "-auto-approve")
		for _, c := range cases {
			requireAbsent(t, h, c.key())
		}
	}) {
		return
	}

	t.Run("version 0 state upgrades to resource_id", func(t *testing.T) {
		spec := toolSpec("tf_legacy_tool", "Legacy tool")
		applyOutOfBand(t, h, "Tool", "tf-legacy-tool", spec)
		encoded, _ := json.Marshal(spec)
		legacy := env.root(t, "legacy", "TF_VAR_spec="+string(encoded))
		writeLegacyState(t, legacy.rootDir, string(encoded))
		legacy.requirePlanExitCode(t, 0)
		legacy.mustRun(t, "apply", "-refresh-only", "-input=false", "-no-color", "-auto-approve")
		data, err := os.ReadFile(filepath.Join(legacy.rootDir, "terraform.tfstate"))
		if err != nil {
			t.Fatal(err)
		}
		var state struct {
			Resources []struct {
				Instances []struct {
					SchemaVersion int            `json:"schema_version"`
					Attributes    map[string]any `json:"attributes"`
				} `json:"instances"`
			} `json:"resources"`
		}
		if err := json.Unmarshal(data, &state); err != nil || len(state.Resources) != 1 || len(state.Resources[0].Instances) != 1 {
			t.Fatalf("decode upgraded state: %v\n%s", err, data)
		}
		instance := state.Resources[0].Instances[0]
		if _, hasName := instance.Attributes["name"]; instance.SchemaVersion != 1 || instance.Attributes["resource_id"] != "tf-legacy-tool" || hasName {
			t.Fatalf("upgraded state instance = %+v", instance)
		}
		legacy.mustRun(t, "destroy", "-input=false", "-no-color", "-auto-approve")
		requireAbsent(t, h, "Tool/tf-legacy-tool")
	})

	t.Run("gizclaw_catalog reads without a reachable Server", func(t *testing.T) {
		h.StopServer()
		catalogs := filepath.Join(env.workDir, "catalogs")
		sources, _ := json.Marshal([]string{filepath.Join(catalogs, "base"), filepath.Join(catalogs, "overrides")})
		products, _ := json.Marshal([]string{filepath.Join(env.workDir, "products", "device")})
		catalog := env.root(t, "catalog-only",
			"TF_VAR_catalog_sources="+string(sources),
			"TF_VAR_product_sources="+string(products),
		)
		catalog.apply(t)
		var profiles []string
		catalog.outputJSON(t, "runtime_profiles", &profiles)
		if strings.Join(profiles, ",") != "RuntimeProfile/tf-device" {
			t.Fatalf("runtime_profiles = %v", profiles)
		}
	})
}

func lifecycleCases(adminKey, peerKey string) []lifecycleCase {
	i18n := func(name string) map[string]any {
		return map[string]any{"en": map[string]any{"display_name": name}, "zh-CN": map[string]any{"display_name": name}}
	}
	credential := func(id, provider, secretField string) lifecycleCase {
		return lifecycleCase{tier: 1, kind: "Credential", id: id, probe: []string{"description"}, secret: true,
			spec: func(v string) map[string]any {
				return map[string]any{
					"provider":    provider,
					"description": "Terraform credential " + v,
					"body":        map[string]any{secretField: "${" + apiKeyEnv + "}"},
				}
			}}
	}
	tenant := func(kind, id, credentialID string, extra map[string]any) lifecycleCase {
		return lifecycleCase{tier: 2, kind: kind, id: id, probe: []string{"description"},
			spec: func(v string) map[string]any {
				spec := map[string]any{"credential_id": credentialID, "description": "Terraform tenant " + v}
				for key, value := range extra {
					spec[key] = value
				}
				return spec
			}}
	}
	owner, peer := adminKey, peerKey
	if peer < owner {
		owner, peer = peer, owner
	}
	return []lifecycleCase{
		credential("tf-lc-openai", "openai", "api_key"),
		credential("tf-lc-deepseek", "deepseek", "api_key"),
		credential("tf-lc-gemini", "gemini", "api_key"),
		credential("tf-lc-dashscope", "dashscope", "api_key"),
		credential("tf-lc-minimax", "minimax", "api_key"),
		credential("tf-lc-volc", "volc", "ark_api_key"),
		tenant("OpenAITenant", "tf-lc-openai", "tf-lc-openai", map[string]any{
			"kind": "compatible", "base_url": "https://api.openai.com/v1", "api_mode": "chat_completions",
		}),
		tenant("DeepSeekTenant", "tf-lc-deepseek", "tf-lc-deepseek", nil),
		tenant("GeminiTenant", "tf-lc-gemini", "tf-lc-gemini", nil),
		tenant("DashScopeTenant", "tf-lc-dashscope", "tf-lc-dashscope", nil),
		tenant("MiniMaxTenant", "tf-lc-minimax", "tf-lc-minimax", nil),
		tenant("VolcTenant", "tf-lc-volc", "tf-lc-volc", nil),
		{tier: 3, kind: "Model", id: "tf-lc-chat", probe: []string{"display_name"},
			spec: func(v string) map[string]any {
				return map[string]any{
					"kind": "llm", "source": "manual",
					"provider":      map[string]any{"kind": "openai-tenant", "id": "tf-lc-openai"},
					"display_name":  "Terraform chat " + v,
					"description":   "${" + modelNoteEnv + "}",
					"provider_data": map[string]any{"upstream_model": "gpt-4o-mini"},
				}
			}},
		{tier: 3, kind: "Voice", id: "tf-lc-alloy", probe: []string{"display_name"},
			spec: func(v string) map[string]any {
				return map[string]any{
					"source":        "manual",
					"provider":      map[string]any{"kind": "openai-tenant", "id": "tf-lc-openai"},
					"display_name":  "Terraform Alloy " + v,
					"provider_data": map[string]any{"voice_id": "alloy"},
				}
			}},
		{tier: 1, kind: "MemoryLayout", id: "tf-lc-memory", probe: []string{"mem0", "custom_instructions"},
			spec: func(v string) map[string]any {
				return map[string]any{
					"flowcraft": map[string]any{
						"extraction": map[string]any{"model": "tf-lc-chat", "mode": "two_pass"},
						"lanes":      []any{map[string]any{"name": "owner_profile", "kind": "note"}},
						"write":      map[string]any{"mode": "sync", "tier": "general"},
					},
					// The trailing newline is what a YAML block scalar produces;
					// the Server trims it and refresh must still see no change.
					"mem0": map[string]any{"custom_instructions": "Keep stable preferences " + v + ".\n"},
					"volc_mem0": map[string]any{"strategies": []any{map[string]any{
						"name": "tf-facts", "type": "user_preference", "custom_instructions": "Keep facts.",
					}}},
				}
			}},
		{tier: 1, kind: "Workflow", id: "tf-lc-echo", probe: []string{"flowcraft", "graph", "name"},
			spec: func(v string) map[string]any {
				return map[string]any{"driver": "flowcraft", "flowcraft": map[string]any{"graph": map[string]any{
					"name":  "tf-lc-echo-" + v,
					"entry": "passthrough",
					"nodes": []any{map[string]any{"id": "passthrough", "type": "passthrough", "publish": true}},
					"edges": []any{map[string]any{"from": "passthrough", "to": "__end__"}},
				}}}
			}},
		{tier: 1, kind: "Tool", id: "tf-lc-weather", probe: []string{"description"},
			spec: func(v string) map[string]any { return toolSpec("tf_lc_weather", "Terraform tool "+v) }},
		{tier: 1, kind: "Firmware", id: "tf-lc-devkit", probe: []string{"description"},
			spec: func(v string) map[string]any {
				return map[string]any{
					"description": "Terraform firmware " + v,
					"slots":       map[string]any{"stable": map[string]any{}, "beta": map[string]any{}, "develop": map[string]any{}},
				}
			}},
		{tier: 4, kind: "RuntimeProfile", id: "tf-lc-device",
			probe: []string{"workflows", "collections", "assistants", "echo", "i18n", "en", "display_name"},
			spec: func(v string) map[string]any {
				return map[string]any{
					"workflows": map[string]any{"collections": map[string]any{"assistants": map[string]any{
						"echo": map[string]any{"resource_id": "tf-lc-echo", "i18n": i18n("Echo " + v)},
					}}},
					"resources": map[string]any{
						"models": map[string]any{"chat": map[string]any{"resource_id": "tf-lc-chat", "i18n": i18n("Chat")}},
						"voices": map[string]any{"assistant": map[string]any{"resource_id": "tf-lc-alloy", "i18n": i18n("Alloy")}},
					},
				}
			}},
		{tier: 5, kind: "RegistrationToken", id: "tf-lc-device", probe: []string{"token"},
			spec: func(v string) map[string]any {
				return map[string]any{"token": "tf-lc-token-" + v, "runtime_profile_id": "tf-lc-device", "firmware_id": "tf-lc-devkit"}
			}},
		{tier: 1, kind: "Contact", id: "tf-lc-contact", probe: []string{"display_name"},
			spec: func(v string) map[string]any {
				return map[string]any{"owner_public_key": adminKey, "name": "tf-lc-contact-name", "display_name": "Terraform contact " + v}
			}},
		{tier: 1, kind: "Friend", id: owner + ":" + peer,
			spec: func(string) map[string]any {
				return map[string]any{"owner_public_key": owner, "peer_public_key": peer}
			}},
		{tier: 1, kind: "FriendGroup", id: "tf-lc-group", probe: []string{"display_name"}, noReuse: true,
			spec: func(v string) map[string]any {
				return map[string]any{
					"owner_public_key": adminKey, "name": "tf-lc-social-group",
					"display_name": "Terraform group " + v, "description": "Managed by Terraform",
				}
			}},
		{tier: 2, kind: "FriendGroupMember", id: customid.MembershipName("tf-lc-group", peerKey),
			spec: func(string) map[string]any {
				return map[string]any{"friend_group_id": "tf-lc-group", "name": "tf-lc-social-group", "peer_public_key": peerKey, "role": "member"}
			}},
		{tier: 2, kind: "FriendGroupInviteToken", id: "tf-lc-group", probe: []string{"invite_token"},
			spec: func(v string) map[string]any {
				return map[string]any{"friend_group_id": "tf-lc-group", "invite_token": "tf-lc-invite-" + v, "expires_at": "2099-01-01T00:00:00Z"}
			}},
	}
}

// toolSpec is written in the Server's normalized form: gizclaw_resource
// compares the stored spec literally, so omitting the Server defaults for
// enabled, http.headers, and http.success_status_codes would plan a change on
// every refresh.
func toolSpec(invokeName, description string) map[string]any {
	return map[string]any{
		"type":        "http_request",
		"invoke_name": invokeName,
		"description": description,
		"input_schema": map[string]any{
			"type": "object", "required": []any{"city"},
			"properties": map[string]any{"city": map[string]any{"type": "string"}},
		},
		"http": map[string]any{
			"url": "https://weather.example/v1", "method": "GET",
			"auth": map[string]any{"method": "none"}, "timeout": "5s", "max_response_bytes": 4096,
		},
	}
}

func writeLifecycleTiers(t *testing.T, rootDir string, cases []lifecycleCase, config lifecycleConfig) {
	t.Helper()
	tiers := map[string]map[string]any{}
	for _, c := range cases {
		spec, err := json.Marshal(c.spec(config.variants[c.key()]))
		if err != nil {
			t.Fatal(err)
		}
		entry := map[string]any{"kind": c.kind, "resource_id": c.id, "spec": string(spec)}
		if revision := config.inputRevisions[c.key()]; revision != "" {
			entry["input_revision"] = revision
		}
		if version := config.apiVersions[c.key()]; version != "" {
			entry["api_version"] = version
		}
		if renamed := config.renamed[c.key()]; renamed != "" {
			entry["resource_id"] = renamed
		}
		tier := fmt.Sprintf("t%d", c.tier)
		if tiers[tier] == nil {
			tiers[tier] = map[string]any{}
		}
		tiers[tier][c.key()] = entry
	}
	for key, entry := range config.extra {
		tiers["t1"][key] = entry
	}
	data, err := json.MarshalIndent(map[string]any{"tiers": tiers}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootDir, "terraform.tfvars.json"), string(data))
}

// writeLegacyState writes local state the way a schema-version-0 provider
// stored gizclaw_resource: the resource ID in `name`.
func writeLegacyState(t *testing.T, rootDir, spec string) {
	t.Helper()
	state := map[string]any{
		"version":           4,
		"terraform_version": "1.5.0",
		"serial":            1,
		"lineage":           "6f0c3b8e-8d1c-4f8e-9c1a-2b5d7e9f0a11",
		"outputs":           map[string]any{},
		"resources": []any{map[string]any{
			"mode":     "managed",
			"type":     "gizclaw_resource",
			"name":     "legacy",
			"provider": `provider["gizclaw.local/gizclaw/gizclaw"]`,
			"instances": []any{map[string]any{
				"schema_version": 0,
				"attributes": map[string]any{
					"id":          "Tool/tf-legacy-tool",
					"api_version": "gizclaw.admin/v1alpha1",
					"kind":        "Tool",
					"name":        "tf-legacy-tool",
					"spec":        spec,
				},
				"sensitive_attributes": []any{},
			}},
		}},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootDir, "terraform.tfstate"), string(data))
}

func requireVariants(t *testing.T, h *clitest.Harness, cases []lifecycleCase, variants map[string]string) {
	t.Helper()
	for _, c := range cases {
		requireVariant(t, h, c, variants[c.key()])
	}
}

// requireVariant reads the resource from the Server and, when the kind has a
// probe, requires its value to be the one of variant.
func requireVariant(t *testing.T, h *clitest.Harness, c lifecycleCase, variant string) {
	t.Helper()
	raw := showResource(t, h, c.key())
	if c.probe == nil {
		return
	}
	want := fmt.Sprint(lookupPath(c.spec(variant), c.probe))
	got := fmt.Sprint(specValue(t, raw, c.probe))
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("%s spec.%s = %q, want %q (variant %s)", c.key(), strings.Join(c.probe, "."), got, want, variant)
	}
}

// requireActions compares the planned actions with want(case); "" means no
// change is planned for that case.
func requireActions(t *testing.T, actions map[string]string, cases []lifecycleCase, want func(lifecycleCase) string) {
	t.Helper()
	expected := map[string]string{}
	for _, c := range cases {
		if action := want(c); action != "" {
			expected[c.address()] = action
		}
	}
	var problems []string
	for address, action := range expected {
		if actions[address] != action {
			problems = append(problems, fmt.Sprintf("%s: planned %q, want %q", address, actions[address], action))
		}
	}
	for address, action := range actions {
		if _, ok := expected[address]; !ok {
			problems = append(problems, fmt.Sprintf("%s: unexpected %q", address, action))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("planned actions differ:\n%s", strings.Join(problems, "\n"))
	}
}

func findCase(t *testing.T, cases []lifecycleCase, key string) lifecycleCase {
	t.Helper()
	for _, c := range cases {
		if c.key() == key {
			return c
		}
	}
	t.Fatalf("no lifecycle case %s", key)
	return lifecycleCase{}
}

// enableSFU gives the Server an SFU URL so Friend and Friend Group resources
// can bind their Room identity. Nothing listens there; these resources only
// mint the binding.
func enableSFU(t *testing.T, h *clitest.Harness) {
	t.Helper()
	keyFile := filepath.Join(h.SandboxDir, "sfu-api-key")
	secretFile := filepath.Join(h.SandboxDir, "sfu-api-secret")
	writeFile(t, keyFile, "tf-e2e-sfu-key\n")
	writeFile(t, secretFile, "tf-e2e-sfu-secret-0123456789abcdef0123456789\n")
	configPath := filepath.Join(h.ServerWorkspace, "config.yaml")
	replaceInFile(t, configPath, "\nservices:\n", fmt.Sprintf(
		"\nservices:\n  sfu:\n    url: ws://127.0.0.1:7880\n    api_key_file: %s\n    api_secret_file: %s\n", keyFile, secretFile))
	h.RestartServer()
}
