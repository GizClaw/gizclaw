package localserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBootstrapperAppliesResourcesThenRuntimeProfileAndRegistrationToken(t *testing.T) {
	podDir := t.TempDir()
	contextDir := filepath.Join(podDir, "admin_context", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), []byte("context"), 0o600); err != nil {
		t.Fatal(err)
	}
	defaultValue := "default"
	catalog := &Catalog{
		FS: fstest.MapFS{
			"resources/00-credentials/a.yaml":               {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  id: a\nspec: {}\n")},
			"resources/00-credentials/b.yaml":               {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  id: b\nspec: {}\n")},
			"resources/05-pet-defs/pet-a.yaml":              {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: PetDef\nmetadata:\n  id: pet-a\nspec: {}\n")},
			"resources/07-runtime-profiles/00-default.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  id: default\nspec: {}\n")},
			"resources/08-registration-tokens/default.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RegistrationToken\nmetadata:\n  id: default-runtime\nspec:\n  token: public-token\n  runtime_profile_id: default\n")},
			"assets/pets/a.pixa":                            {Data: []byte("pet")},
		},
		Resources: []ResourceEntry{
			{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"},
			{Path: "resources/00-credentials/b.yaml", Kind: "Credential", Name: "b"},
			{Path: "resources/05-pet-defs/pet-a.yaml", Kind: "PetDef", Name: "pet-a"},
			{Path: "resources/07-runtime-profiles/00-default.yaml", Kind: "RuntimeProfile", Name: "default"},
			{Path: "resources/08-registration-tokens/default.yaml", Kind: "RegistrationToken", Name: "default-runtime"},
		},
		Requirements: []EnvironmentRequirement{
			{Name: "BOOTSTRAP_SAVED"},
			{Name: "BOOTSTRAP_DEFAULT", Default: &defaultValue},
		},
		PetDefPIXAs: []PetDefPIXA{{PetDef: "pet-a", PIXA: "assets/pets/a.pixa"}},
	}
	t.Setenv("BOOTSTRAP_SAVED", "process")
	var commands []string
	checkCommand := func(executable string, args, environment []string) {
		if executable != "/fake/gizclaw" {
			t.Fatalf("executable = %q", executable)
		}
		joinedEnvironment := strings.Join(environment, "\n")
		if !strings.Contains(joinedEnvironment, "BOOTSTRAP_SAVED=desktop") || !strings.Contains(joinedEnvironment, "input=${input}") {
			t.Fatalf("environment does not contain resolved values")
		}
		var xdgConfigHome, appData string
		for _, entry := range environment {
			name, value, _ := strings.Cut(entry, "=")
			switch name {
			case "XDG_CONFIG_HOME":
				xdgConfigHome = value
			case "AppData":
				appData = value
			}
		}
		if xdgConfigHome == "" || appData != xdgConfigHome {
			t.Fatalf("CLI config roots = XDG_CONFIG_HOME %q, AppData %q", xdgConfigHome, appData)
		}
		if data, err := os.ReadFile(filepath.Join(appData, "gizclaw", "local", "config.yaml")); err != nil || string(data) != "context" {
			t.Fatalf("Windows CLI context = %q, %v", data, err)
		}
		commands = append(commands, strings.Join(args, " "))
	}
	bootstrapper := &Bootstrapper{
		Catalog:    catalog,
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(_ context.Context, executable string, args, environment []string) ([]byte, error) {
			var result []byte
			if len(args) >= 2 && args[0] == "admin" && args[1] == "apply" {
				data, err := os.ReadFile(args[len(args)-1])
				if err != nil {
					t.Fatal(err)
				}
				kind, name := "", ""
				for _, candidate := range []struct{ kind, name string }{
					{"Credential", "a"}, {"Credential", "b"}, {"PetDef", "pet-a"},
					{"RuntimeProfile", "default"}, {"RegistrationToken", "default-runtime"},
				} {
					if strings.Contains(string(data), "kind: "+candidate.kind) && strings.Contains(string(data), "id: "+candidate.name) {
						kind, name = candidate.kind, candidate.name
						break
					}
				}
				if kind == "" {
					t.Fatalf("unexpected apply document = %s", data)
				}
				result = fmt.Appendf(nil, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":%q,"id":%q,"action":"created"}`, kind, name)
			}
			checkCommand(executable, args, environment)
			return result, nil
		},
	}
	if err := bootstrapper.Apply(context.Background(), podDir, map[string]string{"BOOTSTRAP_SAVED": "desktop"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 6 {
		t.Fatalf("commands = %d: %v", len(commands), commands)
	}
	if !strings.Contains(commands[0], "admin apply") {
		t.Fatalf("resource apply = %v", commands)
	}
	if !strings.Contains(commands[3], "admin pet-defs upload-pixa pet-a") {
		t.Fatalf("PetDef PIXA upload = %q", commands[3])
	}
	if !strings.Contains(commands[4], "admin apply") {
		t.Fatalf("RuntimeProfile apply = %q", commands[4])
	}
	if !strings.Contains(commands[5], "admin apply") {
		t.Fatalf("RegistrationToken command = %q", commands[5])
	}
}

type catalogResolverFunc func(context.Context) (*Catalog, error)

func (resolve catalogResolverFunc) Resolve(ctx context.Context) (*Catalog, error) {
	return resolve(ctx)
}

func TestSetCommandEnvironmentReplacesWindowsNameCaseInsensitively(t *testing.T) {
	environment := setCommandEnvironmentForOS([]string{"APPDATA=old", "OTHER=value"}, "AppData", "new", "windows")
	if got := strings.Join(environment, "\n"); got != "AppData=new\nOTHER=value" {
		t.Fatalf("environment = %q", got)
	}
}

func TestBootstrapperIdentifiesFailingResourceWithoutEnvironmentValues(t *testing.T) {
	podDir := t.TempDir()
	contextDir := filepath.Join(podDir, "admin_context", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), []byte("context"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := &Catalog{
		FS:        fstest.MapFS{"resources/00-credentials/a.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  id: a\nspec:\n  provider: openai\n  body:\n    api_key: secret\n")}},
		Resources: []ResourceEntry{{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"}},
	}
	bootstrapper := &Bootstrapper{
		Catalog:    catalog,
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(context.Context, string, []string, []string) ([]byte, error) {
			return nil, errors.New("exit status 1")
		},
	}
	err := bootstrapper.Apply(context.Background(), podDir, nil)
	if err == nil || !strings.Contains(err.Error(), "Credential/a") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestBootstrapperIdentifiesRejectedPetDefPIXA(t *testing.T) {
	catalog := &Catalog{
		FS:          fstest.MapFS{"assets/pet-defs/codex.pixa": {Data: []byte("pet")}},
		PetDefPIXAs: []PetDefPIXA{{PetDef: "petdef-codex", PIXA: "assets/pet-defs/codex.pixa"}},
	}
	bootstrapper := &Bootstrapper{Catalog: catalog}
	err := bootstrapper.uploadPetDefPIXAs(
		context.Background(),
		catalog,
		t.TempDir(),
		"/fake/gizclaw",
		nil,
		func(context.Context, string, []string, []string) ([]byte, error) {
			return nil, errors.New("visible outer-border pixel")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "PetDef/petdef-codex") ||
		!strings.Contains(err.Error(), "assets/pet-defs/codex.pixa") ||
		!strings.Contains(err.Error(), "visible outer-border pixel") {
		t.Fatalf("uploadPetDefPIXAs() error = %v", err)
	}
}

func TestRunBootstrapCommandReturnsRedactedDiagnostic(t *testing.T) {
	if os.Getenv("GIZCLAW_BOOTSTRAP_HELPER_PROCESS") == "1" {
		_, _ = fmt.Fprintln(os.Stderr, "request rejected for secret-token")
		os.Exit(1)
	}
	environment := append(os.Environ(),
		"GIZCLAW_BOOTSTRAP_HELPER_PROCESS=1",
		"GIZCLAW_MINIMAX_CN_API_KEY=secret-token",
	)
	_, err := runBootstrapCommand(context.Background(), os.Args[0], []string{"-test.run=TestRunBootstrapCommandReturnsRedactedDiagnostic"}, environment)
	if err == nil || !strings.Contains(err.Error(), "request rejected") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("runBootstrapCommand() error = %v", err)
	}
}

func TestRunBootstrapOperationRetriesTransientDialFailure(t *testing.T) {
	var attempts int
	run := func(context.Context, string, []string, []string) ([]byte, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("exit status 1: Error: gizclaw: dial: gizwebrtc: wait for packet channel: context deadline exceeded")
		}
		return nil, nil
	}
	if _, err := runBootstrapOperation(context.Background(), run, "gizclaw", []string{"admin", "apply"}, nil); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRunBootstrapOperationDoesNotRetryApplyRejection(t *testing.T) {
	var attempts int
	run := func(context.Context, string, []string, []string) ([]byte, error) {
		attempts++
		return nil, errors.New("exit status 1: INVALID_CREDENTIAL")
	}
	_, err := runBootstrapOperation(context.Background(), run, "gizclaw", []string{"admin", "apply"}, nil)
	if err == nil || attempts != 1 {
		t.Fatalf("error = %v, attempts = %d", err, attempts)
	}
}
