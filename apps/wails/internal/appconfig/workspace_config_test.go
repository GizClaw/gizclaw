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
	assertServiceStoreBindings(t, first)

	pod.LocalServer.Port = 19821
	if err := materializeLocalServerWorkspace(pod, path); err != nil {
		t.Fatalf("second materialization: %v", err)
	}
	second := readRenderedWorkspace(t, path)
	if second.Identity.PrivateKey != first.Identity.PrivateKey || second.Listen != "0.0.0.0:19821" {
		t.Fatalf("second workspace = %+v", second)
	}
	assertLocalRuntimeStore(t, second)
	assertServiceStoreBindings(t, second)
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
		"memory_objects_store:", "memory_store:", "history-db:",
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
	Listen   string                     `yaml:"listen"`
	Storage  map[string]renderedStorage `yaml:"storage"`
	Stores   map[string]renderedStore   `yaml:"stores"`
	Services map[string]any             `yaml:"services"`
}

type renderedStorage struct {
	Kind string `yaml:"kind"`
	Dir  string `yaml:"dir"`
}

type renderedStore struct {
	Kind    string `yaml:"kind"`
	Storage string `yaml:"storage"`
	Prefix  string `yaml:"prefix"`
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
	agentHost, ok := config.Services["agent_host"].(map[string]any)
	if !ok {
		t.Fatalf("services.agent_host = %#v", config.Services["agent_host"])
	}
	runtimeStore, _ := agentHost["runtime_store"].(string)
	if runtimeStore != "agenthost" {
		t.Fatalf("services.agent_host.runtime_store = %q", runtimeStore)
	}
	store, ok := config.Stores[runtimeStore]
	if !ok {
		t.Fatalf("stores.%s is missing", runtimeStore)
	}
	if store.Kind != "objectstore" || store.Storage != "local-files" {
		t.Fatalf("stores.%s = %+v", runtimeStore, store)
	}
	storage, ok := config.Storage[store.Storage]
	if !ok {
		t.Fatalf("storage.%s is missing", store.Storage)
	}
	if storage.Kind != "filesystem.dir" || storage.Dir != "data/objects" {
		t.Fatalf("storage.%s = %+v", store.Storage, storage)
	}
	flowcraft, ok := agentHost["flowcraft"].(map[string]any)
	if !ok || flowcraft["state_store"] != "flowcraft-state" {
		t.Fatalf("services.agent_host.flowcraft = %#v", agentHost["flowcraft"])
	}
	state := config.Stores["flowcraft-state"]
	if state.Kind != "keyvalue" || state.Storage != "local-kv" || state.Prefix != "flowcraft-state" {
		t.Fatalf("stores.flowcraft-state = %+v", state)
	}
}

func assertServiceStoreBindings(t *testing.T, config renderedWorkspace) {
	t.Helper()
	expected := map[string]map[string]string{
		"peer":             {"store": "keyvalue"},
		"api_key":          {"store": "keyvalue"},
		"credential":       {"store": "keyvalue"},
		"firmware":         {"store": "keyvalue"},
		"runtime_profile":  {"store": "keyvalue"},
		"model":            {"store": "keyvalue"},
		"voice":            {"store": "keyvalue"},
		"memory_layout":    {"store": "keyvalue"},
		"provider_tenants": {"store": "keyvalue"},
		"workflow":         {"store": "keyvalue"},
		"workspace":        {"store": "keyvalue", "assets_store": "objectstore"},
		"toolkit":          {"store": "keyvalue"},
		"contact":          {"store": "keyvalue"},
		"friend":           {"store": "keyvalue"},
		"friend_group":     {"store": "keyvalue"},
		"gameplay":         {"store": "keyvalue", "assets_store": "objectstore", "database_store": "sql"},
		"metrics":          {"store": "metrics"},
	}
	for service, fields := range expected {
		block, ok := config.Services[service].(map[string]any)
		if !ok {
			t.Fatalf("services.%s = %#v", service, config.Services[service])
		}
		for field, kind := range fields {
			name, ok := block[field].(string)
			if !ok || name == "" {
				t.Fatalf("services.%s.%s = %#v", service, field, block[field])
			}
			store, ok := config.Stores[name]
			if !ok || store.Kind != kind {
				t.Fatalf("services.%s.%s references stores.%s = %+v, want kind %s", service, field, name, store, kind)
			}
		}
	}
}
