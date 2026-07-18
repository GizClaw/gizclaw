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

func TestBootstrapperAppliesResourcesSyncsVoicesUploadsAssetsAndCreatesRegistrationToken(t *testing.T) {
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
			"resources/00-credentials/a.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: a\n")},
			"resources/00-credentials/b.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: b\n")},
			"assets/workflows/a.png":          {Data: []byte("png")},
			"assets/workflows/a.pixa":         {Data: []byte("pixa")},
			"assets/pets/a.pixa":              {Data: []byte("pet")},
		},
		Resources: []ResourceEntry{
			{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"},
			{Path: "resources/00-credentials/b.yaml", Kind: "Credential", Name: "b"},
		},
		Requirements: []EnvironmentRequirement{
			{Name: "BOOTSTRAP_SAVED"},
			{Name: "BOOTSTRAP_DEFAULT", Default: &defaultValue},
		},
		VoiceSyncs:    []VoiceSync{{Provider: "volc", Tenant: "volc-main"}},
		WorkflowIcons: []WorkflowIcon{{Workflow: "workflow-a", PNG: "assets/workflows/a.png", PIXA: "assets/workflows/a.pixa"}},
		PetDefPIXAs:   []PetDefPIXA{{PetDef: "pet-a", PIXA: "assets/pets/a.pixa"}},
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
		Run: func(_ context.Context, executable string, args, environment []string) error {
			if len(args) >= 2 && args[0] == "admin" && args[1] == "apply" {
				data, err := os.ReadFile(args[len(args)-1])
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "kind: ResourceList") {
					t.Fatalf("batched apply document = %s", data)
				}
				if strings.Contains(args[len(args)-1], "desktop-bootstrap-resources") {
					first, second := strings.Index(string(data), "name: a"), strings.Index(string(data), "name: b")
					if first < 0 || second <= first {
						t.Fatalf("resource batch order = %s", data)
					}
				}
			}
			checkCommand(executable, args, environment)
			return nil
		},
		RunOutput: func(_ context.Context, executable string, args, environment []string) ([]byte, error) {
			checkCommand(executable, args, environment)
			request, err := os.ReadFile(args[len(args)-1])
			if err != nil {
				t.Fatal(err)
			}
			if got := string(request); !strings.Contains(got, `"name":"desktop-local"`) || !strings.Contains(got, `"firmware_name":"desktop-local"`) || !strings.Contains(got, `"runtime_profile_name":"desktop-local"`) {
				t.Fatalf("RegistrationToken request = %s", got)
			}
			return []byte(`{"name":"desktop-local","firmware_name":"desktop-local","runtime_profile_name":"desktop-local","token":"registration-secret"}`), nil
		},
	}
	if err := bootstrapper.Apply(context.Background(), podDir, map[string]string{"BOOTSTRAP_SAVED": "desktop"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 6 {
		t.Fatalf("commands = %d: %v", len(commands), commands)
	}
	if !strings.Contains(commands[0], "admin apply") || !strings.Contains(commands[1], "volc-tenants sync-voices volc-main") {
		t.Fatalf("resource/sync order = %v", commands[:2])
	}
	if !strings.Contains(commands[2], "upload-icon workflow-a --format png") || !strings.Contains(commands[3], "upload-icon workflow-a --format pixa") || !strings.Contains(commands[4], "upload-pixa pet-a") {
		t.Fatalf("asset commands = %v", commands[2:5])
	}
	if !strings.Contains(commands[5], "registration-tokens create --context local") {
		t.Fatalf("RegistrationToken command = %q", commands[5])
	}
	tokenPath := filepath.Join(podDir, "workspace", RegistrationTokenFile)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "registration-secret" {
		t.Fatalf("registration token = %q", token)
	}
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("registration token mode = %o", info.Mode().Perm())
	}
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
		FS:        fstest.MapFS{"resources/00-credentials/a.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: a\nspec:\n  provider: openai\n  body:\n    api_key: secret\n")}},
		Resources: []ResourceEntry{{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"}},
	}
	bootstrapper := &Bootstrapper{
		Catalog:    catalog,
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(context.Context, string, []string, []string) error {
			return errors.New("exit status 1")
		},
	}
	err := bootstrapper.Apply(context.Background(), podDir, nil)
	if err == nil || !strings.Contains(err.Error(), "Credential/a") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("Apply() error = %v", err)
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
	err := runBootstrapCommand(context.Background(), os.Args[0], []string{"-test.run=TestRunBootstrapCommandReturnsRedactedDiagnostic"}, environment)
	if err == nil || !strings.Contains(err.Error(), "request rejected") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("runBootstrapCommand() error = %v", err)
	}
}

func TestRunBootstrapOperationRetriesTransientDialFailure(t *testing.T) {
	var attempts int
	run := func(context.Context, string, []string, []string) error {
		attempts++
		if attempts == 1 {
			return errors.New("exit status 1: Error: gizclaw: dial: gizwebrtc: wait for packet channel: context deadline exceeded")
		}
		return nil
	}
	if err := runBootstrapOperation(context.Background(), run, "gizclaw", []string{"admin", "apply"}, nil); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRunBootstrapOperationDoesNotRetryApplyRejection(t *testing.T) {
	var attempts int
	run := func(context.Context, string, []string, []string) error {
		attempts++
		return errors.New("exit status 1: INVALID_CREDENTIAL")
	}
	err := runBootstrapOperation(context.Background(), run, "gizclaw", []string{"admin", "apply"}, nil)
	if err == nil || attempts != 1 {
		t.Fatalf("error = %v, attempts = %d", err, attempts)
	}
}
