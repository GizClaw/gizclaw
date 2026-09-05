//go:build gizclaw_e2e

package admin_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminResourcesUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "509-admin-resources")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	resourcePath := filepath.Join(h.SandboxDir, "credential-resource.json")
	if err := os.WriteFile(resourcePath, []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write resource file: %v", err)
	}

	apply := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	apply.MustSucceed(t)
	if !strings.Contains(apply.Stdout, `"action":"created"`) || !strings.Contains(apply.Stdout, `"id":"minimax-main"`) {
		t.Fatalf("admin apply create output unexpected:\n%s", apply.Stdout)
	}
	credentialID := adminAppliedResourceID(t, apply.Stdout)

	missing := h.RunCLI("admin", "show", "Credential", "missing", "--context", "admin-a")
	if missing.Err == nil {
		t.Fatal("admin show missing resource should fail")
	}
	if !strings.Contains(missing.Stderr, "RESOURCE_NOT_FOUND") {
		t.Fatalf("admin show missing stderr = %s", missing.Stderr)
	}

	show := h.RunCLI("admin", "show", "Credential", credentialID, "--context", "admin-a")
	show.MustSucceed(t)
	if !strings.Contains(show.Stdout, `"kind":"Credential"`) || !strings.Contains(show.Stdout, `"id":"minimax-main"`) {
		t.Fatalf("admin show output unexpected:\n%s", show.Stdout)
	}

	if err := os.WriteFile(resourcePath, fmt.Appendf(nil, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": %q},
		"spec": {
			"provider": "minimax",
			"description": "updated credential",
			"body": {"api_key": "secret"}
		}
	}`, credentialID), 0o644); err != nil {
		t.Fatalf("write updated resource file: %v", err)
	}
	update := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	update.MustSucceed(t)
	if !strings.Contains(update.Stdout, `"action":"updated"`) {
		t.Fatalf("admin apply update output unexpected:\n%s", update.Stdout)
	}

	deleted := h.RunCLI("admin", "delete", "Credential", credentialID, "--context", "admin-a")
	deleted.MustSucceed(t)
	if !strings.Contains(deleted.Stdout, `"kind":"Credential"`) || !strings.Contains(deleted.Stdout, `"id":"minimax-main"`) {
		t.Fatalf("admin delete output unexpected:\n%s", deleted.Stdout)
	}

	resourceList := h.RunCLI("admin", "show", "ResourceList", "bundle", "--context", "admin-a")
	if resourceList.Err == nil {
		t.Fatal("admin show ResourceList should fail before server lookup")
	}
	if !strings.Contains(resourceList.Stderr, `resource kind "ResourceList" cannot be addressed by ID`) {
		t.Fatalf("admin show ResourceList stderr = %s", resourceList.Stderr)
	}
}

func TestAdminResourceListAppliesModelAndVoice(t *testing.T) {
	h := clitest.NewHarness(t, "509-admin-resources")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	credentialPath := filepath.Join(h.SandboxDir, "openai-credential.json")
	writeAdminFixture(t, credentialPath, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Credential",
		"metadata":{"id":"openai-main-credential"},
		"spec":{"provider":"openai","body":{"api_key":"secret"}}
	}`)
	credential := h.RunCLI("admin", "apply", "-f", credentialPath, "--context", "admin-a")
	credential.MustSucceed(t)
	credentialID := adminAppliedResourceID(t, credential.Stdout)
	tenantPath := filepath.Join(h.SandboxDir, "openai-tenant.json")
	writeAdminFixture(t, tenantPath, fmt.Sprintf(`{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"OpenAITenant",
		"metadata":{"id":"openai-main"},
		"spec":{"kind":"compatible","credential_id":%q,"base_url":"https://api.openai.com/v1","api_mode":"chat_completions"}
	}`, credentialID))
	tenant := h.RunCLI("admin", "apply", "-f", tenantPath, "--context", "admin-a")
	tenant.MustSucceed(t)
	tenantID := adminAppliedResourceID(t, tenant.Stdout)

	resourcePath := filepath.Join(h.SandboxDir, "model-voice-resources.json")
	if err := os.WriteFile(resourcePath, fmt.Appendf(nil, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"spec": {
			"items": [
				{
					"apiVersion": "gizclaw.admin/v1alpha1",
					"kind": "Model",
					"metadata": {"id": "openai-main-chat"},
					"spec": {
						"kind": "llm",
						"source": "manual",
						"provider": {
							"kind": "openai-tenant",
							"id": %q
						},
						"display_name": "OpenAI main chat",
						"description": "OpenAI-compatible chat model from resource apply",
						"provider_data": {
							"upstream_model": "gpt-4o-mini",
							"support_json_output": true,
							"support_tool_calls": true
						}
					}
				},
				{
					"apiVersion": "gizclaw.admin/v1alpha1",
					"kind": "Voice",
					"metadata": {"id": "openai-main-alloy"},
					"spec": {
						"source": "manual",
						"provider": {
							"kind": "openai-tenant",
							"id": %q
						},
						"display_name": "OpenAI Alloy",
						"description": "OpenAI-compatible voice from resource apply",
						"provider_data": {
							"voice_id": "alloy"
						}
					}
				}
			]
		}
	}`, tenantID, tenantID), 0o644); err != nil {
		t.Fatalf("write resource list file: %v", err)
	}

	apply := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	apply.MustSucceed(t)
	for _, want := range []string{
		`"kind":"ResourceList"`,
		`"action":"applied"`,
		`"kind":"Model"`,
		`"id":"openai-main-chat"`,
		`"kind":"Voice"`,
		`"id":"openai-main-alloy"`,
	} {
		if !strings.Contains(apply.Stdout, want) {
			t.Fatalf("admin apply resource list missing %s:\n%s", want, apply.Stdout)
		}
	}
	resourceIDs := adminAppliedResourceIDs(t, apply.Stdout)
	modelID := resourceIDs["openai-main-chat"]
	voiceID := resourceIDs["openai-main-alloy"]
	if modelID == "" || voiceID == "" {
		t.Fatalf("admin ResourceList apply did not return model and voice IDs: %s", apply.Stdout)
	}

	showModel := h.RunCLI("admin", "show", "Model", modelID, "--context", "admin-a")
	showModel.MustSucceed(t)
	for _, want := range []string{
		`"kind":"Model"`,
		`"id":"openai-main-chat"`,
		`"kind":"openai-tenant"`,
		`"upstream_model":"gpt-4o-mini"`,
	} {
		if !strings.Contains(showModel.Stdout, want) {
			t.Fatalf("admin show Model missing %s:\n%s", want, showModel.Stdout)
		}
	}

	showVoice := h.RunCLI("admin", "show", "Voice", voiceID, "--context", "admin-a")
	showVoice.MustSucceed(t)
	for _, want := range []string{
		`"kind":"Voice"`,
		`"id":"openai-main-alloy"`,
		`"kind":"openai-tenant"`,
		`"voice_id":"alloy"`,
	} {
		if !strings.Contains(showVoice.Stdout, want) {
			t.Fatalf("admin show Voice missing %s:\n%s", want, showVoice.Stdout)
		}
	}

	t.Run("batch_show", func(t *testing.T) {
		// More than eight entries exercise multiple worker iterations over the
		// real CLI transport, with mixed kinds and duplicate positions.
		type reference struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		}
		refs := make([]reference, 11)
		for i := range refs {
			refs[i] = reference{Kind: "Model", ID: modelID}
			if i%2 == 1 {
				refs[i] = reference{Kind: "Voice", ID: voiceID}
			}
		}
		path := filepath.Join(h.SandboxDir, "show-references.json")
		for _, partial := range []bool{false, true} {
			missingIndex := -1
			if partial {
				missingIndex = 4
				refs[missingIndex] = reference{Kind: "Model", ID: "batch-show-missing"}
			}
			input, err := json.Marshal(refs)
			if err != nil {
				t.Fatal(err)
			}
			writeAdminFixture(t, path, string(input))
			result := h.RunCLI("admin", "show", "-f", path, "--context", "admin-a")
			if partial {
				var exitErr *exec.ExitError
				if !errors.As(result.Err, &exitErr) || exitErr.ExitCode() != 1 {
					t.Fatalf("partial batch exit = %v, want exit code 1", result.Err)
				}
				for _, want := range []string{"[4] Model/batch-show-missing", "RESOURCE_NOT_FOUND"} {
					if !strings.Contains(result.Stderr, want) {
						t.Fatalf("partial batch stderr missing %q: %s", want, result.Stderr)
					}
				}
			} else {
				result.MustSucceed(t)
			}
			var resources []json.RawMessage
			if err := json.Unmarshal([]byte(result.Stdout), &resources); err != nil {
				t.Fatalf("batch stdout must be exactly one JSON array: %v; output=%s", err, result.Stdout)
			}
			if len(resources) != len(refs) {
				t.Fatalf("batch returned %d entries, want %d", len(resources), len(refs))
			}
			for i, resource := range resources {
				if i == missingIndex {
					if string(resource) != "null" {
						t.Fatalf("missing resource [%d] = %s, want null", i, resource)
					}
					continue
				}
				// Compare the full resource with the existing single-show response,
				// rather than accepting a reference-only object as a lookup result.
				want := showModel.Stdout
				if refs[i].Kind == "Voice" {
					want = showVoice.Stdout
				}
				var gotValue, wantValue any
				if err := json.Unmarshal(resource, &gotValue); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(gotValue, wantValue) {
					t.Fatalf("resource [%d] does not match single show: got %s, want %s", i, resource, want)
				}
			}
		}
	})

	deleteModel := h.RunCLI("admin", "delete", "Model", modelID, "--context", "admin-a")
	deleteModel.MustSucceed(t)
	if !strings.Contains(deleteModel.Stdout, `"kind":"Model"`) || !strings.Contains(deleteModel.Stdout, `"id":"openai-main-chat"`) {
		t.Fatalf("admin delete Model output unexpected:\n%s", deleteModel.Stdout)
	}

	deleteVoice := h.RunCLI("admin", "delete", "Voice", voiceID, "--context", "admin-a")
	deleteVoice.MustSucceed(t)
	if !strings.Contains(deleteVoice.Stdout, `"kind":"Voice"`) || !strings.Contains(deleteVoice.Stdout, `"id":"openai-main-alloy"`) {
		t.Fatalf("admin delete Voice output unexpected:\n%s", deleteVoice.Stdout)
	}
}
