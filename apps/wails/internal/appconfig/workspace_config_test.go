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
	assertLocalRuntimeStore(t, first)

	pod.LocalServer.Port = 19821
	if err := materializeLocalServerWorkspace(pod, path); err != nil {
		t.Fatalf("second materialization: %v", err)
	}
	second := readRenderedWorkspace(t, path)
	if second.Identity.PrivateKey != first.Identity.PrivateKey || second.Listen != "0.0.0.0:19821" {
		t.Fatalf("second workspace = %+v", second)
	}
	assertLocalRuntimeStore(t, second)
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
	for _, forbidden := range []string{
		"default-peer-view:", "query_store:", "kind: log", "volc:",
		"memory_objects_store:", "memory_store:",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("rendered workspace contains %q", forbidden)
		}
	}
}

type renderedWorkspace struct {
	Identity struct {
		PrivateKey giznet.Key `yaml:"private-key"`
	} `yaml:"identity"`
	Listen    string                     `yaml:"listen"`
	Storage   map[string]renderedStorage `yaml:"storage"`
	Stores    map[string]renderedStore   `yaml:"stores"`
	AgentHost struct {
		RuntimeStore string `yaml:"runtime_store"`
	} `yaml:"agent_host"`
}

type renderedStorage struct {
	Kind string `yaml:"kind"`
	FS   struct {
		Dir string `yaml:"dir"`
	} `yaml:"fs"`
}

type renderedStore struct {
	Kind    string `yaml:"kind"`
	Storage string `yaml:"storage"`
}

func readRenderedWorkspace(t *testing.T, path string) renderedWorkspace {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config renderedWorkspace
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func assertLocalRuntimeStore(t *testing.T, config renderedWorkspace) {
	t.Helper()
	if config.AgentHost.RuntimeStore != "agenthost" {
		t.Fatalf("agent_host.runtime_store = %q", config.AgentHost.RuntimeStore)
	}
	store, ok := config.Stores[config.AgentHost.RuntimeStore]
	if !ok {
		t.Fatalf("stores.%s is missing", config.AgentHost.RuntimeStore)
	}
	if store.Kind != "objectstore" || store.Storage != "agenthost-files" {
		t.Fatalf("stores.%s = %+v", config.AgentHost.RuntimeStore, store)
	}
	storage, ok := config.Storage[store.Storage]
	if !ok {
		t.Fatalf("storage.%s is missing", store.Storage)
	}
	if storage.Kind != "objectstore" || storage.FS.Dir != "data/agenthost" {
		t.Fatalf("storage.%s = %+v", store.Storage, storage)
	}
}
