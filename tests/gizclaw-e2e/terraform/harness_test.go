//go:build gizclaw_e2e

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

// terraformEnv is one isolated Server, its admin context, and a filesystem
// mirror holding a freshly built provider.
type terraformEnv struct {
	h       *clitest.Harness
	binary  string
	workDir string
	env     []string
}

// terraformRun runs terraform in one root module with its own data directory.
type terraformRun struct {
	h       *clitest.Harness
	binary  string
	rootDir string
	env     []string
}

func newTerraformEnv(t *testing.T) *terraformEnv {
	t.Helper()
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
	copyTree(t, h.StoryDir, workDir)
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
	return &terraformEnv{
		h:       h,
		binary:  terraformBinary,
		workDir: workDir,
		env: append(os.Environ(),
			"HOME="+h.HomeDir,
			"XDG_CONFIG_HOME="+h.XDGConfigHome,
			"GIZCLAW_CONTEXT=",
			"GIZCLAW_ENDPOINT=",
			"TF_CLI_CONFIG_FILE="+cliConfig,
			"TF_IN_AUTOMATION=1",
			"CHECKPOINT_DISABLE=1",
			"TF_VAR_context="+adminContext,
			apiKeyEnv+"=tf-e2e-secret",
		),
	}
}

// root initializes the root module testdata/<name> and checks that init
// locked the mirrored provider.
func (e *terraformEnv) root(t *testing.T, name string, extraEnv ...string) *terraformRun {
	t.Helper()
	env := append(append([]string{}, e.env...), "TF_DATA_DIR="+filepath.Join(e.h.SandboxDir, "terraform-data", name))
	tf := &terraformRun{h: e.h, binary: e.binary, rootDir: filepath.Join(e.workDir, name), env: append(env, extraEnv...)}
	tf.mustRun(t, "init", "-input=false", "-no-color")
	lock, err := os.ReadFile(filepath.Join(tf.rootDir, ".terraform.lock.hcl"))
	if err != nil ||
		!strings.Contains(string(lock), `provider "gizclaw.local/gizclaw/gizclaw"`) ||
		!strings.Contains(string(lock), `"`+providerVersion+`"`) {
		t.Fatalf("terraform init did not lock the mirrored provider: %v\n%s", err, lock)
	}
	return tf
}

// withEnv returns a copy of r whose process environment has the given
// entries appended (later entries win) or, for "NAME" without "=", removed.
func (r *terraformRun) withEnv(entries ...string) *terraformRun {
	env := make([]string, 0, len(r.env)+len(entries))
	for _, kv := range r.env {
		name, _, _ := strings.Cut(kv, "=")
		removed := false
		for _, entry := range entries {
			if !strings.Contains(entry, "=") && entry == name {
				removed = true
			}
		}
		if !removed {
			env = append(env, kv)
		}
	}
	for _, entry := range entries {
		if strings.Contains(entry, "=") {
			env = append(env, entry)
		}
	}
	copied := *r
	copied.env = env
	return &copied
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

// mustFail runs terraform expecting failure and requires out to contain want.
func (r *terraformRun) mustFail(t *testing.T, want string, args ...string) string {
	t.Helper()
	out, err := r.run(t, args...)
	if err == nil {
		t.Fatalf("terraform %s succeeded, want failure containing %q\n%s", strings.Join(args, " "), want, out)
	}
	if !strings.Contains(strings.Join(strings.Fields(out), " "), want) {
		t.Fatalf("terraform %s failed without %q:\n%s", strings.Join(args, " "), want, out)
	}
	return out
}

func (r *terraformRun) apply(t *testing.T) {
	t.Helper()
	r.mustRun(t, "apply", "-input=false", "-no-color", "-auto-approve")
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

// plannedActions returns the planned actions keyed by resource address for
// every resource whose plan is not a no-op.
func (r *terraformRun) plannedActions(t *testing.T) map[string]string {
	t.Helper()
	planFile := filepath.Join(r.h.SandboxDir, "lifecycle.tfplan")
	r.mustRun(t, "plan", "-input=false", "-no-color", "-out="+planFile)
	out := r.outputCommand(t, "show", "-json", planFile)
	var plan struct {
		ResourceChanges []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if err := json.Unmarshal(out, &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	actions := map[string]string{}
	for _, change := range plan.ResourceChanges {
		joined := strings.Join(change.Change.Actions, ",")
		if joined != "no-op" && joined != "read" {
			actions[change.Address] = joined
		}
	}
	return actions
}

func (r *terraformRun) outputCommand(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(r.binary, args...)
	cmd.Dir = r.rootDir
	cmd.Env = r.env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("terraform %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func (r *terraformRun) outputJSON(t *testing.T, name string, target any) {
	t.Helper()
	out := r.outputCommand(t, "output", "-json", name)
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

// applyOutOfBand writes a resource directly through the gizclaw CLI, as an
// operator changing the Server outside Terraform would.
func applyOutOfBand(t *testing.T, h *clitest.Harness, kind, id string, spec any) {
	t.Helper()
	manifest, err := json.Marshal(map[string]any{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind":       kind,
		"metadata":   map[string]any{"id": id},
		"spec":       spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.SandboxDir, "out-of-band.json")
	writeFile(t, path, string(manifest))
	h.RunCLI("admin", "apply", "-f", path, "--context", adminContext).MustSucceed(t)
}

func deleteOutOfBand(t *testing.T, h *clitest.Harness, id string) {
	t.Helper()
	kind, resourceID, _ := strings.Cut(id, "/")
	h.RunCLI("admin", "delete", kind, resourceID, "--context", adminContext).MustSucceed(t)
}

func specString(t *testing.T, raw json.RawMessage, field string) string {
	t.Helper()
	value, _ := specValue(t, raw, []string{field}).(string)
	return value
}

func specValue(t *testing.T, raw json.RawMessage, path []string) any {
	t.Helper()
	var resource struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatalf("decode resource: %v\n%s", err, raw)
	}
	return lookupPath(resource.Spec, path)
}

func lookupPath(value any, path []string) any {
	for _, key := range path {
		object, _ := value.(map[string]any)
		value = object[key]
	}
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
