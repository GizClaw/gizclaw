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

func TestBootstrapperAppliesResourcesSyncsVoicesUploadsPetAssetsAndCreatesRegistrationToken(t *testing.T) {
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
		VoiceSyncs:  []VoiceSync{{Provider: "volc", Tenant: "volc-main"}},
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
			if got := string(request); !strings.Contains(got, `"name":"app:com.gizclaw.opensource"`) || strings.Contains(got, `"firmware_name"`) || !strings.Contains(got, `"runtime_profile_name":"default"`) {
				t.Fatalf("RegistrationToken request = %s", got)
			}
			return []byte(`{"name":"app:com.gizclaw.opensource","runtime_profile_name":"default","token":"registration-secret"}`), nil
		},
	}
	if err := bootstrapper.Apply(context.Background(), podDir, map[string]string{"BOOTSTRAP_SAVED": "desktop"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 {
		t.Fatalf("commands = %d: %v", len(commands), commands)
	}
	if !strings.Contains(commands[0], "admin apply") || !strings.Contains(commands[1], "volc-tenants sync-voices volc-main") {
		t.Fatalf("resource/sync order = %v", commands[:2])
	}
	if !strings.Contains(commands[2], "upload-pixa pet-a") {
		t.Fatalf("asset command = %v", commands[2])
	}
	if !strings.Contains(commands[3], "registration-tokens create --context local") {
		t.Fatalf("RegistrationToken command = %q", commands[3])
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

func TestBootstrapperRecoversRegistrationTokenWithoutReapplyingCatalog(t *testing.T) {
	podDir := t.TempDir()
	contextDir := filepath.Join(podDir, "admin_context", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), []byte("context"), 0o600); err != nil {
		t.Fatal(err)
	}
	var commands []string
	bootstrapper := &Bootstrapper{
		Catalog:    &Catalog{},
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(_ context.Context, _ string, args, _ []string) error {
			commands = append(commands, strings.Join(args, " "))
			return errors.New("existing token may not exist")
		},
		RunOutput: func(_ context.Context, _ string, args, _ []string) ([]byte, error) {
			commands = append(commands, strings.Join(args, " "))
			return []byte(`{"token":"replacement-secret"}`), nil
		},
	}
	if err := bootstrapper.RecoverRegistrationToken(context.Background(), podDir, nil); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 || !strings.Contains(commands[0], "registration-tokens delete app:com.gizclaw.opensource") || !strings.Contains(commands[1], "registration-tokens create") {
		t.Fatalf("recovery commands = %v", commands)
	}
	token, err := os.ReadFile(filepath.Join(podDir, "workspace", RegistrationTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "replacement-secret" {
		t.Fatalf("replacement token = %q", token)
	}
}

func TestBootstrapperMigratesOnlyDefaultRuntimeContract(t *testing.T) {
	podDir := t.TempDir()
	contextDir := filepath.Join(podDir, "admin_context", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), []byte("context"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := &Catalog{
		FS: fstest.MapFS{
			"resources/00-credentials/a.yaml":               {Data: []byte("kind: Credential\nmetadata: {name: a}\n")},
			"resources/07-runtime-profiles/00-default.yaml": {Data: []byte("kind: RuntimeProfile\nmetadata: {name: default}\n")},
		},
		Resources: []ResourceEntry{
			{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"},
			{Path: "resources/07-runtime-profiles/00-default.yaml", Kind: "RuntimeProfile", Name: "default"},
		},
	}
	var commands []string
	bootstrapper := &Bootstrapper{
		Catalog:    catalog,
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(_ context.Context, _ string, args, _ []string) error {
			commands = append(commands, strings.Join(args, " "))
			if args[1] == "apply" {
				data, err := os.ReadFile(args[len(args)-1])
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(data), "Credential") || !strings.Contains(string(data), "RuntimeProfile") {
					t.Fatalf("migration apply = %s", data)
				}
			}
			return nil
		},
		RunOutput: func(_ context.Context, _ string, args, _ []string) ([]byte, error) {
			commands = append(commands, strings.Join(args, " "))
			return []byte(`{"token":"migrated-secret"}`), nil
		},
	}
	if err := bootstrapper.MigrateRuntimeContract(context.Background(), podDir); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 3 || !strings.Contains(commands[0], "admin apply") || !strings.Contains(commands[1], "registration-tokens delete app:com.gizclaw.opensource") || !strings.Contains(commands[2], "registration-tokens create") {
		t.Fatalf("migration commands = %v", commands)
	}
	token, err := os.ReadFile(filepath.Join(podDir, "workspace", RegistrationTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "migrated-secret" {
		t.Fatalf("migration token = %q", token)
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
