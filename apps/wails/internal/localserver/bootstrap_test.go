package localserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goccy/go-yaml"
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
			"resources/00-credentials/a.yaml":               {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: a\nspec: {}\n")},
			"resources/00-credentials/b.yaml":               {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: b\nspec: {}\n")},
			"resources/05-pet-defs/pet-a.yaml":              {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: PetDef\nmetadata:\n  name: pet-a\nspec: {}\n")},
			"resources/07-runtime-profiles/00-default.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\nspec: {}\n")},
			"resources/08-registration-tokens/default.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RegistrationToken\nmetadata:\n  name: default-runtime\nspec:\n  token: public-token\n  runtime_profile_name: default\n")},
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
					if strings.Contains(string(data), "kind: "+candidate.kind) && strings.Contains(string(data), "name: "+candidate.name) {
						kind, name = candidate.kind, candidate.name
						break
					}
				}
				if kind == "" {
					t.Fatalf("unexpected apply document = %s", data)
				}
				result = []byte(fmt.Sprintf(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":%q,"name":%q,"id":%q,"action":"created"}`, kind, name, name+"-id"))
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
	if !strings.Contains(commands[3], "admin pet-defs upload-pixa pet-a-id") {
		t.Fatalf("PetDef PIXA upload = %q", commands[3])
	}
	if !strings.Contains(commands[4], "admin apply") {
		t.Fatalf("RuntimeProfile apply = %q", commands[4])
	}
	if !strings.Contains(commands[5], "admin apply") {
		t.Fatalf("RegistrationToken command = %q", commands[5])
	}
}

func TestResolveCatalogResourceIDsUsesCanonicalIDs(t *testing.T) {
	ids := catalogResourceIDs{
		"Credential":     {"default": "credential-id"},
		"MiniMaxTenant":  {"default": "tenant-id"},
		"Workflow":       {"chat": "workflow-id"},
		"Model":          {"chat": "model-id"},
		"Voice":          {"voice": "voice-id"},
		"Tool":           {"clock": "tool-id"},
		"PetDef":         {"codex": "pet-id"},
		"BadgeDef":       {"welcome": "badge-id"},
		"GameDef":        {"quest": "game-id"},
		"MemoryLayout":   {"default": "memory-layout-id"},
		"RuntimeProfile": {"default": "runtime-profile-id"},
		"Firmware":       {"desktop": "firmware-id"},
	}
	tests := []struct {
		name  string
		entry ResourceEntry
		input string
		want  map[string]any
	}{
		{
			name:  "tenant credential",
			entry: ResourceEntry{Kind: "MiniMaxTenant", Name: "default"},
			input: "spec:\n  credential_name: default\n",
			want:  map[string]any{"credential_id": "credential-id"},
		},
		{
			name:  "model provider",
			entry: ResourceEntry{Kind: "Model", Name: "chat"},
			input: "spec:\n  provider:\n    kind: minimax-tenant\n    name: default\n",
			want:  map[string]any{"provider": map[string]any{"kind": "minimax-tenant", "id": "tenant-id"}},
		},
		{
			name:  "runtime profile",
			entry: ResourceEntry{Kind: "RuntimeProfile", Name: "default"},
			input: `spec:
  workflows:
    system:
      friend_chatroom: chat
      group_chatroom: chat
      pet: chat
    collections:
      default:
        chat:
          resource_id: chat
  resources:
    models:
      chat:
        resource_id: chat
    voices:
      voice:
        resource_id: voice
    tools:
      clock:
        resource_id: clock
    pet_defs:
      codex:
        resource_id: codex
    badge_defs:
      welcome:
        resource_id: welcome
    game_defs:
      quest:
        resource_id: quest
    memories:
      default:
        layout_id: default
`,
			want: map[string]any{
				"workflows": map[string]any{
					"system":      map[string]any{"friend_chatroom": "workflow-id", "group_chatroom": "workflow-id", "pet": "workflow-id"},
					"collections": map[string]any{"default": map[string]any{"chat": map[string]any{"resource_id": "workflow-id"}}},
				},
				"resources": map[string]any{
					"models":     map[string]any{"chat": map[string]any{"resource_id": "model-id"}},
					"voices":     map[string]any{"voice": map[string]any{"resource_id": "voice-id"}},
					"tools":      map[string]any{"clock": map[string]any{"resource_id": "tool-id"}},
					"pet_defs":   map[string]any{"codex": map[string]any{"resource_id": "pet-id"}},
					"badge_defs": map[string]any{"welcome": map[string]any{"resource_id": "badge-id"}},
					"game_defs":  map[string]any{"quest": map[string]any{"resource_id": "game-id"}},
					"memories":   map[string]any{"default": map[string]any{"layout_id": "memory-layout-id"}},
				},
			},
		},
		{
			name:  "registration token",
			entry: ResourceEntry{Kind: "RegistrationToken", Name: "default-runtime"},
			input: "spec:\n  runtime_profile_name: default\n  firmware_name: desktop\n",
			want:  map[string]any{"runtime_profile_id": "runtime-profile-id", "firmware_id": "firmware-id"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := resolveCatalogResourceIDs([]byte(tt.input), tt.entry, ids)
			if err != nil {
				t.Fatal(err)
			}
			jsonData, err := yaml.YAMLToJSON(output)
			if err != nil {
				t.Fatal(err)
			}
			var document struct {
				Spec map[string]any `json:"spec"`
			}
			if err := json.Unmarshal(jsonData, &document); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(document.Spec, tt.want) {
				t.Fatalf("spec = %v, want %v", document.Spec, tt.want)
			}
		})
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
		FS:        fstest.MapFS{"resources/00-credentials/a.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  name: a\nspec:\n  provider: openai\n  body:\n    api_key: secret\n")}},
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
		catalogResourceIDs{"PetDef": {"petdef-codex": "petdef-id"}},
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
