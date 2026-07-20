package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

func TestMaterializeLocalServerWorkspaceUsesEmbeddedTemplateAndPreservesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	pod := Pod{LocalServer: &LocalServer{Port: 19820}}
	if err := materializeLocalServerWorkspace(pod, path); err != nil {
		t.Fatalf("first materialization: %v", err)
	}
	first := readRenderedWorkspace(t, path)
	if first.Identity.PrivateKey.IsZero() || first.Listen != "0.0.0.0:19820" {
		t.Fatalf("first workspace = %+v", first)
	}
	if first.SystemTasks.PetFlowcraftWorkflow.GenerateModel != "minimax" ||
		first.SystemTasks.PetFlowcraftWorkflow.ExtractModel != "extract-model" ||
		first.SystemTasks.PetFlowcraftWorkflow.ASRModel != "asr-model" {
		t.Fatalf("pet Flowcraft system task = %+v", first.SystemTasks.PetFlowcraftWorkflow)
	}

	pod.LocalServer.Port = 19821
	if err := materializeLocalServerWorkspace(pod, path); err != nil {
		t.Fatalf("second materialization: %v", err)
	}
	second := readRenderedWorkspace(t, path)
	if second.Identity.PrivateKey != first.Identity.PrivateKey || second.Listen != "0.0.0.0:19821" {
		t.Fatalf("second workspace = %+v", second)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"default-peer-view:", "query_store:", "kind: log", "volc:"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("rendered workspace contains %q", forbidden)
		}
	}
}

func readRenderedWorkspace(t *testing.T, path string) struct {
	Identity struct {
		PrivateKey giznet.Key `yaml:"private-key"`
	} `yaml:"identity"`
	Listen      string `yaml:"listen"`
	SystemTasks struct {
		PetFlowcraftWorkflow struct {
			GenerateModel string `yaml:"generate_model"`
			ExtractModel  string `yaml:"extract_model"`
			ASRModel      string `yaml:"asr_model"`
		} `yaml:"pet_flowcraft_workflow"`
	} `yaml:"system_tasks"`
} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Identity struct {
			PrivateKey giznet.Key `yaml:"private-key"`
		} `yaml:"identity"`
		Listen      string `yaml:"listen"`
		SystemTasks struct {
			PetFlowcraftWorkflow struct {
				GenerateModel string `yaml:"generate_model"`
				ExtractModel  string `yaml:"extract_model"`
				ASRModel      string `yaml:"asr_model"`
			} `yaml:"pet_flowcraft_workflow"`
		} `yaml:"system_tasks"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}
