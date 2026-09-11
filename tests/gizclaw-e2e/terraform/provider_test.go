//go:build gizclaw_e2e

// Package terraform_test drives the released Terraform provider shape against a
// real GizClaw Server: the provider binary is installed from a filesystem
// mirror, and `terraform` itself runs init, apply, plan, and destroy.
package terraform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerraformProviderAppliesCatalogSelection(t *testing.T) {
	env := newTerraformEnv(t)
	h := env.h
	catalogs := filepath.Join(env.workDir, "catalogs")
	product := filepath.Join(env.workDir, "products", "device")
	sources, _ := json.Marshal([]string{filepath.Join(catalogs, "base"), filepath.Join(catalogs, "overrides")})
	products, _ := json.Marshal([]string{product})
	tf := env.root(t, "root",
		"TF_VAR_catalog_sources="+string(sources),
		"TF_VAR_product_sources="+string(products),
	)

	if !t.Run("apply selects and creates the product closure", func(t *testing.T) {
		tf.mustRun(t, "apply", "-input=false", "-no-color", "-auto-approve")
		for _, id := range []string{
			"Credential/tf-openai",
			"OpenAITenant/tf-openai",
			"Model/tf-chat",
			"Voice/tf-alloy",
			"Workflow/tf-echo",
			"Workflow/tf-raid-flowcraft",
			"Workflow/tf-raid-test",
			"Firmware/tf-devkit",
			"RuntimeProfile/tf-device",
			"RegistrationToken/tf-device",
		} {
			showResource(t, h, id)
		}
		for _, id := range []string{"Credential/tf-unused", "Workflow/tf-unused"} {
			requireAbsent(t, h, id)
		}
		if got := specString(t, showResource(t, h, "Model/tf-chat"), "display_name"); got != "Terraform override chat" {
			t.Fatalf("Model/tf-chat display_name = %q, want the override source", got)
		}
		workflow := showResource(t, h, "Workflow/tf-echo")
		if !strings.Contains(string(workflow), `"name":"tf-echo-override"`) {
			t.Fatalf("Workflow/tf-echo does not carry the override graph:\n%s", workflow)
		}
		token := showResource(t, h, "RegistrationToken/tf-device")
		for _, want := range []string{`"runtime_profile_id":"tf-device"`, `"firmware_id":"tf-devkit"`} {
			if !strings.Contains(string(token), want) {
				t.Fatalf("RegistrationToken/tf-device missing %s:\n%s", want, token)
			}
		}

		var overridden []string
		tf.outputJSON(t, "overridden_ids", &overridden)
		if strings.Join(overridden, ",") != "Model/tf-chat,Workflow/tf-echo" {
			t.Fatalf("overridden_ids = %v", overridden)
		}
		var raids []string
		tf.outputJSON(t, "raids", &raids)
		if strings.Join(raids, ",") != "tf-demo-raid" {
			t.Fatalf("raids = %v", raids)
		}
	}) {
		return
	}

	if !t.Run("refresh after apply plans no changes", func(t *testing.T) {
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("catalog edits update the Server", func(t *testing.T) {
		modelPath := filepath.Join(catalogs, "overrides", "models", "chat.yaml")
		replaceInFile(t, modelPath, "Terraform override chat", "Terraform edited chat")
		tf.requirePlanExitCode(t, 2)
		tf.mustRun(t, "apply", "-input=false", "-no-color", "-auto-approve")
		if got := specString(t, showResource(t, h, "Model/tf-chat"), "display_name"); got != "Terraform edited chat" {
			t.Fatalf("Model/tf-chat display_name = %q after edit", got)
		}
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	if !t.Run("resources leaving the selection are deleted", func(t *testing.T) {
		if err := os.Remove(filepath.Join(product, "registration-tokens", "device.yaml")); err != nil {
			t.Fatal(err)
		}
		tf.mustRun(t, "apply", "-input=false", "-no-color", "-auto-approve")
		requireAbsent(t, h, "RegistrationToken/tf-device")
		requireAbsent(t, h, "Firmware/tf-devkit")
		showResource(t, h, "RuntimeProfile/tf-device")
		tf.requirePlanExitCode(t, 0)
	}) {
		return
	}

	t.Run("destroy removes every managed resource", func(t *testing.T) {
		tf.mustRun(t, "destroy", "-input=false", "-no-color", "-auto-approve")
		for _, id := range []string{
			"RuntimeProfile/tf-device",
			"Workflow/tf-echo",
			"Workflow/tf-raid-flowcraft",
			"Workflow/tf-raid-test",
			"Model/tf-chat",
			"Voice/tf-alloy",
			"OpenAITenant/tf-openai",
			"Credential/tf-openai",
		} {
			requireAbsent(t, h, id)
		}
	})
}
