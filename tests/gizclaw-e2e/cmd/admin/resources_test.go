//go:build gizclaw_e2e

package admin_test

import (
	"os"
	"path/filepath"
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
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write resource file: %v", err)
	}

	apply := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	apply.MustSucceed(t)
	if !strings.Contains(apply.Stdout, `"action":"created"`) || !strings.Contains(apply.Stdout, `"name":"minimax-main"`) {
		t.Fatalf("admin apply create output unexpected:\n%s", apply.Stdout)
	}

	missing := h.RunCLI("admin", "show", "Credential", "missing", "--context", "admin-a")
	if missing.Err == nil {
		t.Fatal("admin show missing resource should fail")
	}
	if !strings.Contains(missing.Stderr, "RESOURCE_NOT_FOUND") {
		t.Fatalf("admin show missing stderr = %s", missing.Stderr)
	}

	show := h.RunCLI("admin", "show", "Credential", "minimax-main", "--context", "admin-a")
	show.MustSucceed(t)
	if !strings.Contains(show.Stdout, `"kind":"Credential"`) || !strings.Contains(show.Stdout, `"name":"minimax-main"`) {
		t.Fatalf("admin show output unexpected:\n%s", show.Stdout)
	}

	if err := os.WriteFile(resourcePath, []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"description": "updated credential",
			"body": {"api_key": "secret"}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write updated resource file: %v", err)
	}
	update := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	update.MustSucceed(t)
	if !strings.Contains(update.Stdout, `"action":"updated"`) {
		t.Fatalf("admin apply update output unexpected:\n%s", update.Stdout)
	}

	deleted := h.RunCLI("admin", "delete", "Credential", "minimax-main", "--context", "admin-a")
	deleted.MustSucceed(t)
	if !strings.Contains(deleted.Stdout, `"kind":"Credential"`) || !strings.Contains(deleted.Stdout, `"name":"minimax-main"`) {
		t.Fatalf("admin delete output unexpected:\n%s", deleted.Stdout)
	}

	resourceList := h.RunCLI("admin", "show", "ResourceList", "bundle", "--context", "admin-a")
	if resourceList.Err == nil {
		t.Fatal("admin show ResourceList should fail before server lookup")
	}
	if !strings.Contains(resourceList.Stderr, `resource kind "ResourceList" cannot be addressed by name`) {
		t.Fatalf("admin show ResourceList stderr = %s", resourceList.Stderr)
	}
}

func TestAdminResourceListAppliesModelAndVoice(t *testing.T) {
	h := clitest.NewHarness(t, "509-admin-resources")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	resourcePath := filepath.Join(h.SandboxDir, "model-voice-resources.json")
	if err := os.WriteFile(resourcePath, []byte(`{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "ResourceList",
		"metadata": {"name": "model-voice-bundle"},
		"spec": {
			"items": [
				{
					"apiVersion": "gizclaw.admin/v1alpha1",
					"kind": "Model",
					"metadata": {"name": "openai-main-chat"},
					"spec": {
						"kind": "llm",
						"source": "manual",
						"provider": {
							"kind": "openai-tenant",
							"name": "openai-main"
						},
						"name": "OpenAI main chat",
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
					"metadata": {"name": "openai-main-alloy"},
					"spec": {
						"source": "manual",
						"provider": {
							"kind": "openai-tenant",
							"name": "openai-main"
						},
						"name": "OpenAI Alloy",
						"description": "OpenAI-compatible voice from resource apply",
						"provider_data": {
							"voice_id": "alloy"
						}
					}
				}
			]
		}
	}`), 0o644); err != nil {
		t.Fatalf("write resource list file: %v", err)
	}

	apply := h.RunCLI("admin", "apply", "-f", resourcePath, "--context", "admin-a")
	apply.MustSucceed(t)
	for _, want := range []string{
		`"kind":"ResourceList"`,
		`"name":"model-voice-bundle"`,
		`"action":"applied"`,
		`"kind":"Model"`,
		`"name":"openai-main-chat"`,
		`"kind":"Voice"`,
		`"name":"openai-main-alloy"`,
	} {
		if !strings.Contains(apply.Stdout, want) {
			t.Fatalf("admin apply resource list missing %s:\n%s", want, apply.Stdout)
		}
	}

	showModel := h.RunCLI("admin", "show", "Model", "openai-main-chat", "--context", "admin-a")
	showModel.MustSucceed(t)
	for _, want := range []string{
		`"kind":"Model"`,
		`"name":"openai-main-chat"`,
		`"kind":"openai-tenant"`,
		`"upstream_model":"gpt-4o-mini"`,
	} {
		if !strings.Contains(showModel.Stdout, want) {
			t.Fatalf("admin show Model missing %s:\n%s", want, showModel.Stdout)
		}
	}

	showVoice := h.RunCLI("admin", "show", "Voice", "openai-main-alloy", "--context", "admin-a")
	showVoice.MustSucceed(t)
	for _, want := range []string{
		`"kind":"Voice"`,
		`"name":"openai-main-alloy"`,
		`"kind":"openai-tenant"`,
		`"voice_id":"alloy"`,
	} {
		if !strings.Contains(showVoice.Stdout, want) {
			t.Fatalf("admin show Voice missing %s:\n%s", want, showVoice.Stdout)
		}
	}

	deleteModel := h.RunCLI("admin", "delete", "Model", "openai-main-chat", "--context", "admin-a")
	deleteModel.MustSucceed(t)
	if !strings.Contains(deleteModel.Stdout, `"kind":"Model"`) || !strings.Contains(deleteModel.Stdout, `"name":"openai-main-chat"`) {
		t.Fatalf("admin delete Model output unexpected:\n%s", deleteModel.Stdout)
	}

	deleteVoice := h.RunCLI("admin", "delete", "Voice", "openai-main-alloy", "--context", "admin-a")
	deleteVoice.MustSucceed(t)
	if !strings.Contains(deleteVoice.Stdout, `"kind":"Voice"`) || !strings.Contains(deleteVoice.Stdout, `"name":"openai-main-alloy"`) {
		t.Fatalf("admin delete Voice output unexpected:\n%s", deleteVoice.Stdout)
	}
}
