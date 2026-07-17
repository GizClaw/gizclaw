package localserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBootstrapperAppliesResourcesSyncsVoicesThenACLAndAssets(t *testing.T) {
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
			"resources/00-credentials/a.yaml": {Data: []byte("credential")},
			"resources/90-acl/a.yaml":         {Data: []byte("acl")},
			"assets/workflows/a.png":          {Data: []byte("png")},
			"assets/workflows/a.pixa":         {Data: []byte("pixa")},
			"assets/pets/a.pixa":              {Data: []byte("pet")},
		},
		Resources: []ResourceEntry{
			{Path: "resources/00-credentials/a.yaml", Kind: "Credential", Name: "a"},
			{Path: "resources/90-acl/a.yaml", Kind: "ACLPolicyBinding", Name: "a"},
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
	bootstrapper := &Bootstrapper{
		Catalog:    catalog,
		Executable: func() (string, error) { return "/fake/gizclaw", nil },
		Run: func(_ context.Context, executable string, args, environment []string) error {
			if executable != "/fake/gizclaw" {
				t.Fatalf("executable = %q", executable)
			}
			joinedEnvironment := strings.Join(environment, "\n")
			if !strings.Contains(joinedEnvironment, "BOOTSTRAP_SAVED=desktop") || !strings.Contains(joinedEnvironment, "input=${input}") {
				t.Fatalf("environment does not contain resolved values")
			}
			commands = append(commands, strings.Join(args, " "))
			return nil
		},
	}
	if err := bootstrapper.Apply(context.Background(), podDir, map[string]string{"BOOTSTRAP_SAVED": "desktop"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 6 {
		t.Fatalf("commands = %d: %v", len(commands), commands)
	}
	if !strings.Contains(commands[0], "admin apply") || !strings.Contains(commands[1], "volc-tenants sync-voices volc-main") || !strings.Contains(commands[2], "admin apply") {
		t.Fatalf("resource/sync/ACL order = %v", commands[:3])
	}
	if !strings.Contains(commands[3], "upload-icon workflow-a --format png") || !strings.Contains(commands[4], "upload-icon workflow-a --format pixa") || !strings.Contains(commands[5], "upload-pixa pet-a") {
		t.Fatalf("asset commands = %v", commands[3:])
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
		FS:        fstest.MapFS{"resources/00-credentials/a.yaml": {Data: []byte("secret")}},
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
