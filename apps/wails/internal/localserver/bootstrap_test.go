package localserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
)

func TestBootstrapperGeneratesURLSafeRegistrationTokenFrom32Bytes(t *testing.T) {
	token, err := (&Bootstrapper{}).registrationToken()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("registration token is not raw URL-safe base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("registration token bytes = %d, want 32", len(decoded))
	}
}

func TestBootstrapperAppliesResourcesThenRuntimeProfileAndConfiguresRegistrationToken(t *testing.T) {
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
			"resources/00-credentials/a.yaml":               {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: a\n")},
			"resources/00-credentials/b.yaml":               {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: b\n")},
			"resources/07-runtime-profiles/00-default.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\n")},
			"assets/pets/a.pixa":                            {Data: []byte("pet")},
		},
		Resources: []ResourceEntry{
			{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"},
			{Path: "resources/00-credentials/b.yaml", Kind: "Credential", Name: "b"},
			{Path: "resources/07-runtime-profiles/00-default.yaml", Kind: "RuntimeProfile", Name: "default"},
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
			if len(args) >= 3 && args[0] == "admin" && args[1] == "registration-tokens" && args[2] == "put" {
				request, err := os.ReadFile(args[len(args)-1])
				if err != nil {
					t.Fatal(err)
				}
				if got := string(request); !strings.Contains(got, `"name":"app:com.gizclaw.opensource"`) || !strings.Contains(got, `"token":"registration-token"`) || strings.Contains(got, `"firmware_name"`) || !strings.Contains(got, `"runtime_profile_name":"default"`) {
					t.Fatalf("RegistrationToken request = %s", got)
				}
			}
			checkCommand(executable, args, environment)
			return nil
		},
		NewRegistrationToken: func() (string, error) { return "registration-token", nil },
	}
	if err := bootstrapper.Apply(context.Background(), podDir, map[string]string{"BOOTSTRAP_SAVED": "desktop"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 {
		t.Fatalf("commands = %d: %v", len(commands), commands)
	}
	if !strings.Contains(commands[0], "admin apply") {
		t.Fatalf("resource apply = %v", commands)
	}
	if !strings.Contains(commands[1], "admin pet-defs upload-pixa pet-a") {
		t.Fatalf("PetDef PIXA upload = %q", commands[1])
	}
	if !strings.Contains(commands[2], "admin apply") {
		t.Fatalf("RuntimeProfile apply = %q", commands[2])
	}
	if !strings.Contains(commands[3], "registration-tokens put app:com.gizclaw.opensource --context local") {
		t.Fatalf("RegistrationToken command = %q", commands[3])
	}
	tokenPath := filepath.Join(podDir, "workspace", RegistrationTokenFile)
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "registration-token" {
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
	resolverCalled := false
	bootstrapper := &Bootstrapper{
		Resolver: catalogResolverFunc(func(context.Context) (*Catalog, error) {
			resolverCalled = true
			return nil, errors.New("Raids archive is unavailable")
		}),
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(_ context.Context, _ string, args, _ []string) error {
			commands = append(commands, strings.Join(args, " "))
			return nil
		},
		NewRegistrationToken: func() (string, error) { return "replacement-token", nil },
	}
	if err := bootstrapper.RecoverRegistrationToken(context.Background(), podDir, nil); err != nil {
		t.Fatal(err)
	}
	if resolverCalled {
		t.Fatal("token recovery resolved the Raids catalog")
	}
	if len(commands) != 1 || !strings.Contains(commands[0], "registration-tokens put app:com.gizclaw.opensource") {
		t.Fatalf("recovery commands = %v", commands)
	}
	token, err := os.ReadFile(filepath.Join(podDir, "workspace", RegistrationTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "replacement-token" {
		t.Fatalf("replacement token = %q", token)
	}
}

func TestBootstrapperFailedRegistrationTokenPutPreservesHandoffFile(t *testing.T) {
	podDir := t.TempDir()
	contextDir := filepath.Join(podDir, "admin_context", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), []byte("context"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspaceDir := filepath.Join(podDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(workspaceDir, RegistrationTokenFile)
	if err := os.WriteFile(tokenFile, []byte("existing-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	bootstrapper := &Bootstrapper{
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(context.Context, string, []string, []string) error {
			return errors.New("INVALID_RESOURCE")
		},
		NewRegistrationToken: func() (string, error) { return "replacement-token", nil },
	}
	if err := bootstrapper.RecoverRegistrationToken(context.Background(), podDir, nil); err == nil {
		t.Fatal("RecoverRegistrationToken() error = nil")
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "existing-token" {
		t.Fatalf("handoff token = %q, want existing-token", token)
	}
}

type catalogResolverFunc func(context.Context) (*Catalog, error)

func (resolve catalogResolverFunc) Resolve(ctx context.Context) (*Catalog, error) {
	return resolve(ctx)
}

func TestBootstrapperMigratesDependencyClosureBeforeDefaultRuntimeProfile(t *testing.T) {
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
			"resources/04-workflows/00-referenced.yaml":     {Data: []byte("kind: Workflow\nmetadata: {name: referenced}\n")},
			"resources/07-runtime-profiles/00-default.yaml": {Data: []byte("kind: RuntimeProfile\nmetadata: {name: default}\nspec:\n  workflows:\n    collections:\n      assistants:\n        demo: {resource_id: referenced}\n")},
			"assets/pets/a.pixa":                            {Data: []byte("pet")},
		},
		Resources: []ResourceEntry{
			{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"},
			{Path: "resources/04-workflows/00-referenced.yaml", Kind: "Workflow", Name: "referenced"},
			{Path: "resources/07-runtime-profiles/00-default.yaml", Kind: "RuntimeProfile", Name: "default"},
		},
		Requirements: []EnvironmentRequirement{{Name: "RAIDS_TOKEN"}},
		PetDefPIXAs:  []PetDefPIXA{{PetDef: "pet-a", PIXA: "assets/pets/a.pixa"}},
	}
	var commands []string
	var applied []string
	bootstrapper := &Bootstrapper{
		Catalog:    catalog,
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(_ context.Context, _ string, args, environment []string) error {
			if !slices.Contains(environment, "input=${input}") {
				t.Fatalf("migration environment does not preserve input placeholder: %v", environment)
			}
			if !slices.Contains(environment, "RAIDS_TOKEN=saved-token") {
				t.Fatalf("migration environment does not contain saved value: %v", environment)
			}
			commands = append(commands, strings.Join(args, " "))
			if args[1] == "apply" {
				data, err := os.ReadFile(args[len(args)-1])
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case strings.Contains(string(data), "name: a"):
					applied = append(applied, "Credential/a")
				case strings.Contains(string(data), "name: referenced"):
					applied = append(applied, "Workflow/referenced")
				case strings.Contains(string(data), "name: default"):
					applied = append(applied, "RuntimeProfile/default")
				default:
					t.Fatalf("unexpected migration apply = %s", data)
				}
			}
			return nil
		},
		NewRegistrationToken: func() (string, error) { return "migrated-token", nil },
	}
	if err := bootstrapper.MigrateRuntimeContract(context.Background(), podDir, map[string]string{"RAIDS_TOKEN": "saved-token"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(applied, ","); got != "Credential/a,Workflow/referenced,RuntimeProfile/default" {
		t.Fatalf("migration applied = %s", got)
	}
	if len(commands) != 7 || !strings.Contains(commands[0], "admin apply") || !strings.Contains(commands[1], "admin apply") || !strings.Contains(commands[2], "admin pet-defs upload-pixa pet-a") || !strings.Contains(commands[3], "runtime-profiles delete default") || !strings.Contains(commands[4], "admin apply") || !strings.Contains(commands[5], "registration-tokens put app:com.gizclaw.opensource") || !strings.Contains(commands[6], "registration-tokens delete desktop-local") {
		t.Fatalf("migration commands = %v", commands)
	}
	token, err := os.ReadFile(filepath.Join(podDir, "workspace", RegistrationTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(token) != "migrated-token" {
		t.Fatalf("migration token = %q", token)
	}
}

func TestRuntimeContractEntriesUseBundledProfileReferences(t *testing.T) {
	source, err := desktopresources.LocalServer()
	if err != nil {
		t.Fatal(err)
	}
	profile := ResourceEntry{
		Path: "resources/07-runtime-profiles/00-default.yaml",
		Kind: "RuntimeProfile",
		Name: defaultRuntimeProfileName,
	}
	catalog := &Catalog{FS: source, Resources: []ResourceEntry{profile}}
	entries, err := (&Bootstrapper{Catalog: catalog}).runtimeContractEntries(profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[len(entries)-1] != profile {
		t.Fatalf("runtime contract entries = %#v", entries)
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
