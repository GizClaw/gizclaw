//go:build gizclaw_e2e

// Package terraform_test drives the released Terraform provider shape against a
// real GizClaw Server: the provider binary is installed from a filesystem
// mirror, and `terraform` itself runs init, apply, plan, and destroy.
package terraform_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

const (
	providerVersion = "0.0.1"
	adminContext    = "tf-admin"
	apiKeyEnv       = "GIZCLAW_E2E_TF_API_KEY"
)

type terraformRun struct {
	h       *clitest.Harness
	binary  string
	rootDir string
	env     []string
}

func TestTerraformProviderAppliesCatalogSelection(t *testing.T) {
	terraformBinary := os.Getenv("GIZCLAW_E2E_TERRAFORM")
	if terraformBinary == "" {
		terraformBinary = "terraform"
	}
	terraformBinary, err := exec.LookPath(terraformBinary)
	if err != nil {
		t.Fatalf("terraform CLI is required for the provider e2e (set GIZCLAW_E2E_TERRAFORM or put terraform on PATH): %v", err)
	}

	h := clitest.NewHarnessForRoot(t, "tests/gizclaw-e2e/terraform", "testdata")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext(adminContext).MustSucceed(t)
	h.RegisterContext(adminContext, "--sn", "tf-admin-sn").MustSucceed(t)

	workDir := filepath.Join(h.SandboxDir, "terraform")
	copyTree(t, filepath.Join(h.StoryDir), workDir)
	mirror := filepath.Join(h.SandboxDir, "provider-mirror")
	buildProvider(t, h.RepoRoot, mirror)
	cliConfig := filepath.Join(h.SandboxDir, "terraformrc")
	writeFile(t, cliConfig, fmt.Sprintf(`provider_installation {
  filesystem_mirror {
    path    = %q
    include = ["gizclaw.local/*/*"]
  }
  direct {
    exclude = ["gizclaw.local/*/*"]
  }
}
`, mirror))

	catalogs := filepath.Join(workDir, "catalogs")
	product := filepath.Join(workDir, "products", "device")
	sources, _ := json.Marshal([]string{filepath.Join(catalogs, "base"), filepath.Join(catalogs, "overrides")})
	products, _ := json.Marshal([]string{product})
	tf := &terraformRun{
		h:       h,
		binary:  terraformBinary,
		rootDir: filepath.Join(workDir, "root"),
		env: append(os.Environ(),
			"HOME="+h.HomeDir,
			"XDG_CONFIG_HOME="+h.XDGConfigHome,
			"GIZCLAW_CONTEXT=",
			"GIZCLAW_ENDPOINT=",
			"TF_CLI_CONFIG_FILE="+cliConfig,
			"TF_DATA_DIR="+filepath.Join(h.SandboxDir, "terraform-data"),
			"TF_IN_AUTOMATION=1",
			"CHECKPOINT_DISABLE=1",
			"TF_VAR_context="+adminContext,
			"TF_VAR_catalog_sources="+string(sources),
			"TF_VAR_product_sources="+string(products),
			apiKeyEnv+"=tf-e2e-secret",
		),
	}

	tf.mustRun(t, "init", "-input=false", "-no-color")
	if lock, err := os.ReadFile(filepath.Join(tf.rootDir, ".terraform.lock.hcl")); err != nil ||
		!strings.Contains(string(lock), `provider "gizclaw.local/gizclaw/gizclaw"`) ||
		!strings.Contains(string(lock), `"`+providerVersion+`"`) {
		t.Fatalf("terraform init did not lock the mirrored provider: %v\n%s", err, lock)
	}

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

func (r *terraformRun) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(r.binary, args...)
	cmd.Dir = r.rootDir
	cmd.Env = r.env
	out, err := cmd.CombinedOutput()
	logName := fmt.Sprintf("terraform-%s.log", strings.ReplaceAll(t.Name(), "/", "_"))
	if f, openErr := os.OpenFile(filepath.Join(r.h.LogsDir, logName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); openErr == nil {
		_, _ = fmt.Fprintf(f, "$ terraform %s\n%s\n", strings.Join(args, " "), out)
		_ = f.Close()
	}
	return string(out), err
}

func (r *terraformRun) mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := r.run(t, args...)
	if err != nil {
		t.Fatalf("terraform %s: %v\n%s\nserver log: %s", strings.Join(args, " "), err, out, r.h.ServerLogPath)
	}
	return out
}

// requirePlanExitCode runs a refreshing plan; with -detailed-exitcode, 0 means
// no changes and 2 means changes are pending.
func (r *terraformRun) requirePlanExitCode(t *testing.T, want int) {
	t.Helper()
	out, err := r.run(t, "plan", "-input=false", "-no-color", "-detailed-exitcode")
	got := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("terraform plan: %v\n%s", err, out)
		}
		got = exitErr.ExitCode()
	}
	if got != want {
		t.Fatalf("terraform plan exit code = %d, want %d\n%s", got, want, out)
	}
}

func (r *terraformRun) outputJSON(t *testing.T, name string, target any) {
	t.Helper()
	cmd := exec.Command(r.binary, "output", "-json", name)
	cmd.Dir = r.rootDir
	cmd.Env = r.env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("terraform output %s: %v", name, err)
	}
	if err := json.Unmarshal(out, target); err != nil {
		t.Fatalf("decode terraform output %s: %v\n%s", name, err, out)
	}
}

func buildProvider(t *testing.T, repoRoot, mirror string) {
	t.Helper()
	dir := filepath.Join(mirror, "gizclaw.local", "gizclaw", "gizclaw", providerVersion, runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-X main.version="+providerVersion,
		"-o", filepath.Join(dir, "terraform-provider-gizclaw_v"+providerVersion),
		"./cmd/terraform-provider-gizclaw")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build terraform provider: %v\n%s", err, out)
	}
}

func showResource(t *testing.T, h *clitest.Harness, id string) json.RawMessage {
	t.Helper()
	kind, resourceID, _ := strings.Cut(id, "/")
	result := h.RunCLI("admin", "show", kind, resourceID, "--context", adminContext)
	result.MustSucceed(t)
	return json.RawMessage(result.Stdout)
}

func requireAbsent(t *testing.T, h *clitest.Harness, id string) {
	t.Helper()
	kind, resourceID, _ := strings.Cut(id, "/")
	result := h.RunCLI("admin", "show", kind, resourceID, "--context", adminContext)
	if result.Err == nil || !strings.Contains(result.Stderr, "_NOT_FOUND") {
		t.Fatalf("%s should be absent: err=%v\nstdout:\n%s\nstderr:\n%s", id, result.Err, result.Stdout, result.Stderr)
	}
}

func specString(t *testing.T, raw json.RawMessage, field string) string {
	t.Helper()
	var resource struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatalf("decode resource: %v\n%s", err, raw)
	}
	value, _ := resource.Spec[field].(string)
	return value
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatalf("copy %s: %v", from, err)
	}
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	writeFile(t, path, strings.ReplaceAll(string(data), old, replacement))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
