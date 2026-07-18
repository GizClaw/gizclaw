package appconfig

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

func TestStoreLocalPodMaterializesPrivateProjection(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	admin := testKey(t, 0x11)
	client := testKey(t, 0x22)
	store := Store{Paths: paths}
	pod := Pod{
		Version: PodVersion,
		ID:      "local-lab",
		Name:    "Local Lab",
		LocalServer: &LocalServer{
			Port:            9820,
			AdminPrivateKey: admin,
		},
		ClientPrivateKey: client,
	}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}

	podDir := filepath.Join(paths.PodsDir, pod.ID)
	assertMode(t, podDir, 0o700)
	assertMode(t, filepath.Join(podDir, PodManifestFile), 0o600)
	adminConfig, err := contextstore.LoadConfig(filepath.Join(podDir, "admin_context", "local"))
	if err != nil {
		t.Fatal(err)
	}
	if adminConfig.Server.Endpoint != "127.0.0.1:9820" {
		t.Fatalf("admin endpoint = %q", adminConfig.Server.Endpoint)
	}
	clientConfig, err := contextstore.LoadConfig(filepath.Join(podDir, "client_context"))
	if err != nil {
		t.Fatal(err)
	}
	if clientConfig.Server.Endpoint != "127.0.0.1:9820" {
		t.Fatalf("client endpoint = %q", clientConfig.Server.Endpoint)
	}
	workspaceData, err := os.ReadFile(filepath.Join(podDir, "workspace", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	type workspaceDir struct {
		Dir string `yaml:"dir"`
	}
	type workspaceStorage struct {
		Kind   string        `yaml:"kind"`
		Badger *workspaceDir `yaml:"badger"`
	}
	type workspaceStore struct {
		Kind    string `yaml:"kind"`
		Storage string `yaml:"storage"`
		Prefix  string `yaml:"prefix"`
	}
	var workspace struct {
		Listen         string                      `yaml:"listen"`
		Endpoint       string                      `yaml:"endpoint"`
		ServeToClients bool                        `yaml:"serve-to-clients"`
		AdminPublicKey string                      `yaml:"admin-public-key"`
		EdgeNodes      []any                       `yaml:"edge-nodes"`
		Storage        map[string]workspaceStorage `yaml:"storage"`
		Stores         map[string]workspaceStore   `yaml:"stores"`
		SystemLog      struct {
			Level string `yaml:"level"`
			Sinks []struct {
				Kind string `yaml:"kind"`
			} `yaml:"sinks"`
		} `yaml:"system_log"`
	}
	if err := yaml.Unmarshal(workspaceData, &workspace); err != nil {
		t.Fatal(err)
	}
	endpointHost, endpointPort, splitErr := net.SplitHostPort(workspace.Endpoint)
	if workspace.Listen != "0.0.0.0:9820" || splitErr != nil || endpointPort != "9820" || endpointHost == "" || endpointHost == "0.0.0.0" {
		t.Fatalf("workspace listen/endpoint = %q/%q", workspace.Listen, workspace.Endpoint)
	}
	if !workspace.ServeToClients || workspace.AdminPublicKey == "" || len(workspace.EdgeNodes) != 0 {
		t.Fatalf("workspace admin key/edge nodes = %q/%v", workspace.AdminPublicKey, workspace.EdgeNodes)
	}
	if storage := workspace.Storage["local-kv"]; storage.Kind != "keyvalue" || storage.Badger == nil || storage.Badger.Dir != "data/kv" {
		t.Fatalf("workspace local-kv storage = %+v", storage)
	}
	if peers := workspace.Stores["peers"]; peers.Kind != "keyvalue" || peers.Storage != "local-kv" || peers.Prefix != "peers" {
		t.Fatalf("workspace peers store = %+v", peers)
	}
	if workspace.SystemLog.Level != "info" || len(workspace.SystemLog.Sinks) != 1 || workspace.SystemLog.Sinks[0].Kind != "stderr" {
		t.Fatalf("workspace system log = %+v", workspace.SystemLog)
	}
	for _, forbidden := range []string{"query_store:", "kind: log", "volc:", "access_key_secret:"} {
		if strings.Contains(string(workspaceData), forbidden) {
			t.Fatalf("workspace contains forbidden default %q", forbidden)
		}
	}
	for _, required := range []string{"credentials", "firmwares", "runtime-profiles", "minimax-tenants", "voices", "workspaces", "workflows"} {
		if _, ok := workspace.Stores[required]; !ok {
			t.Fatalf("workspace required store %q is missing", required)
		}
	}
	for name, prefix := range map[string]string{
		"workspace-assets": "workspaces",
		"workflow-assets":  "workflows",
	} {
		store, ok := workspace.Stores[name]
		if !ok || store.Kind != "objectstore" || store.Storage != "local-files" || store.Prefix != prefix {
			t.Fatalf("workspace %s store = %+v", name, store)
		}
	}
}

func TestStoreRemotePodHasNoWorkspaceAndPerServerAdminContexts(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	pod := Pod{
		Version: PodVersion,
		ID:      "cn-dev",
		Name:    "China Development",
		RemoteServers: []RemoteServer{
			{ID: "beijing-a", Name: "Beijing A", Endpoint: "127.0.0.1:19001", AdminPrivateKey: testKey(t, 0x31)},
			{ID: "beijing-b", Name: "Beijing B", Endpoint: "127.0.0.1:19002"},
		},
		RemoteAccessPoint: "127.0.0.1:19820",
		ClientPrivateKey:  testKey(t, 0x32),
	}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	podDir := filepath.Join(paths.PodsDir, pod.ID)
	if _, err := os.Stat(filepath.Join(podDir, "workspace")); !os.IsNotExist(err) {
		t.Fatalf("workspace should not exist: %v", err)
	}
	if _, err := contextstore.LoadConfig(filepath.Join(podDir, "admin_context", "beijing-a")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(podDir, "admin_context", "beijing-b")); !os.IsNotExist(err) {
		t.Fatalf("admin context without key should not exist: %v", err)
	}
	client, err := contextstore.LoadConfig(filepath.Join(podDir, "client_context"))
	if err != nil {
		t.Fatal(err)
	}
	if client.Server.Endpoint != pod.RemoteAccessPoint {
		t.Fatalf("client endpoint = %q", client.Server.Endpoint)
	}
}

func TestStoreRemotePodAllowsServerInventoryToBeAddedLater(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	pod := Pod{Version: PodVersion, ID: "remote-empty", Name: "Remote", RemoteAccessPoint: "127.0.0.1:19820"}
	store := Store{Paths: paths}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(pod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RemoteAccessPoint != pod.RemoteAccessPoint || len(loaded.RemoteServers) != 0 {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestStoreLocalAdminOnlyKeepsServerInfoPublic(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	pod := Pod{
		Version: PodVersion,
		ID:      "admin-only",
		Name:    "Admin Only",
		LocalServer: &LocalServer{
			Port:            19825,
			AdminPrivateKey: testKey(t, 0x34),
		},
	}
	if err := (Store{Paths: paths}).Save(pod); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(paths.PodsDir, pod.ID, "workspace", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ServeToClients bool `yaml:"serve-to-clients"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if !config.ServeToClients {
		t.Fatal("serve-to-clients = false, Admin browser cannot fetch server-info")
	}
}

func TestLoadRejectsUnknownManifestField(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(paths.PodsDir, "bad-pod")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"version": 1, "id": "bad-pod", "name": "Bad", "local_server": map[string]any{"port": 9820}, "theme": "dark"}
	data, _ := json.Marshal(body)
	if err := os.WriteFile(filepath.Join(dir, PodManifestFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Paths: paths}).Load("bad-pod"); err == nil {
		t.Fatal("Load error = nil, want unknown field rejection")
	}
}

func TestEntriesKeepsMalformedPodRecoverable(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	if err := store.Save(Pod{Version: 1, ID: "healthy", Name: "Healthy", LocalServer: &LocalServer{Port: 19822}}); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(paths.PodsDir, "broken")
	if err := os.MkdirAll(badDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, PodManifestFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("Entries() = %+v", entries)
	}
	var healthy, broken bool
	for _, entry := range entries {
		healthy = healthy || entry.ID == "healthy" && entry.Err == nil
		broken = broken || entry.ID == "broken" && entry.Err != nil
	}
	if !healthy || !broken {
		t.Fatalf("Entries() = %+v", entries)
	}
}

func TestPodValidationEnforcesExclusiveModes(t *testing.T) {
	pod := Pod{Version: 1, ID: "mixed", Name: "Mixed", LocalServer: &LocalServer{Port: 9820}, RemoteServers: []RemoteServer{{ID: "remote", Name: "Remote", Endpoint: "127.0.0.1:9820"}}, RemoteAccessPoint: "127.0.0.1:9820"}
	if err := pod.Validate(); err == nil {
		t.Fatal("Validate error = nil, want mutually exclusive mode error")
	}
}

func TestPodValidationRejectsRemoteServersInLocalMode(t *testing.T) {
	pod := Pod{Version: 1, ID: "mixed", Name: "Mixed", LocalServer: &LocalServer{Port: 9820}, RemoteServers: []RemoteServer{{ID: "remote", Name: "Remote", Endpoint: "127.0.0.1:9820"}}}
	if err := pod.Validate(); err == nil {
		t.Fatal("Validate error = nil, want remote_servers rejection")
	}
}

func TestPodValidationRejectsAmbiguousIDsAndNonNumericPorts(t *testing.T) {
	for _, pod := range []Pod{
		{Version: 1, ID: "double--hyphen", Name: "Bad ID", LocalServer: &LocalServer{Port: 9820}},
		{Version: 1, ID: "remote", Name: "Bad Endpoint", RemoteServers: []RemoteServer{{ID: "server", Name: "Server", Endpoint: "example.com:http"}}, RemoteAccessPoint: "example.com:9820"},
		{Version: 1, ID: "remote-path", Name: "Bad Host", RemoteAccessPoint: "foo/bar:9820"},
	} {
		if err := pod.Validate(); err == nil {
			t.Fatalf("Validate(%q) error = nil", pod.ID)
		}
	}
}

func TestStoreRollsBackManifestWhenProjectionFails(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	pod := Pod{Version: 1, ID: "rollback", Name: "Before", LocalServer: &LocalServer{Port: 19824}}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	clientDir := filepath.Join(paths.PodsDir, pod.ID, "client_context")
	if err := os.Symlink(t.TempDir(), clientDir); err != nil {
		t.Fatal(err)
	}
	pod.Name = "After"
	pod.ClientPrivateKey = testKey(t, 0x63)
	if err := store.Save(pod); err == nil {
		t.Fatal("Save error = nil, want projection failure")
	}
	loaded, err := store.Load(pod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Before" || loaded.ClientPrivateKey != "" {
		t.Fatalf("manifest was not rolled back: %+v", loaded)
	}
}

func TestStoreRollsBackAllProjectionsWhenMaterializationFails(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	pod := Pod{Version: 1, ID: "projection-rollback", Name: "Before", LocalServer: &LocalServer{Port: 19825, AdminPrivateKey: testKey(t, 0x31)}, ClientPrivateKey: testKey(t, 0x32)}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(paths.PodsDir, pod.ID)
	pathsToCheck := []string{
		filepath.Join(dir, PodManifestFile),
		filepath.Join(dir, "workspace", "config.yaml"),
		filepath.Join(dir, "admin_context", "local", contextstore.ConfigFile),
		filepath.Join(dir, "client_context", contextstore.ConfigFile),
	}
	want := map[string][]byte{}
	for _, path := range pathsToCheck {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want[path] = data
	}
	store.materializeHook = func(updated Pod) error {
		if err := store.materialize(updated); err != nil {
			return err
		}
		return errors.New("injected materialization failure")
	}
	pod.Name = "After"
	pod.LocalServer.AdminPrivateKey = testKey(t, 0x41)
	pod.ClientPrivateKey = testKey(t, 0x42)
	if err := store.Save(pod); err == nil {
		t.Fatal("Save error = nil, want injected failure")
	}
	for path, expected := range want {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(expected) {
			t.Fatalf("projection %s was not restored", path)
		}
	}
}

func TestStoreRefusesToReplaceCorruptWorkspaceIdentity(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	pod := Pod{Version: 1, ID: "corrupt-workspace", Name: "Before", LocalServer: &LocalServer{Port: 19826}}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(paths.PodsDir, pod.ID, "workspace", "config.yaml")
	corrupt := []byte("identity: [")
	if err := os.WriteFile(configPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	pod.Name = "After"
	if err := store.Save(pod); err == nil {
		t.Fatal("Save error = nil, want corrupt workspace rejection")
	}
	loaded, err := store.Load(pod.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Before" || string(got) != string(corrupt) {
		t.Fatalf("manifest/workspace changed: name=%q config=%q", loaded.Name, got)
	}
}

func TestStoreNormalizesPrivateKeysBeforePersisting(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	var raw giznet.Key
	for i := range raw {
		raw[i] = 0xff
	}
	input := raw.String()
	want, err := giznet.NewKeyPair(raw)
	if err != nil {
		t.Fatal(err)
	}
	pod := Pod{Version: 1, ID: "normalized", Name: "Normalized", LocalServer: &LocalServer{Port: 19820, AdminPrivateKey: input}}
	store := Store{Paths: paths}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(pod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LocalServer.AdminPrivateKey != want.Private.String() || loaded.LocalServer.AdminPrivateKey == input {
		t.Fatalf("stored key = %q, want normalized %q", loaded.LocalServer.AdminPrivateKey, want.Private.String())
	}
}

func TestStoreRejectsContextSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional Windows privileges")
	}
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	pod := Pod{Version: 1, ID: "safe-pod", Name: "Safe Pod", LocalServer: &LocalServer{Port: 19821}}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	clientDir := filepath.Join(paths.PodsDir, pod.ID, "client_context")
	if err := os.Symlink(external, clientDir); err != nil {
		t.Fatal(err)
	}
	pod.ClientPrivateKey = testKey(t, 0x55)
	if err := store.Save(pod); err == nil {
		t.Fatal("Save error = nil, want symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(external, contextstore.ConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("external config should not be written: %v", err)
	}
}

func TestStoreRejectsConflictingMaterializedContext(t *testing.T) {
	paths := NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store := Store{Paths: paths}
	pod := Pod{Version: 1, ID: "conflict", Name: "Conflict", LocalServer: &LocalServer{Port: 19823, AdminPrivateKey: testKey(t, 0x61)}}
	if err := store.Save(pod); err != nil {
		t.Fatal(err)
	}
	contextDir := filepath.Join(paths.PodsDir, pod.ID, "admin_context", "local")
	if err := writeContext(contextDir, "Conflicting", testKey(t, 0x62), "127.0.0.1:19823"); err != nil {
		t.Fatal(err)
	}
	pod.Name = "Renamed"
	if err := store.Save(pod); err == nil {
		t.Fatal("Save error = nil, want conflicting projection rejection")
	}
	loaded, err := store.Load(pod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Conflict" {
		t.Fatalf("manifest changed after conflict: %+v", loaded)
	}
}

func testKey(t *testing.T, fill byte) string {
	t.Helper()
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	kp, err := giznet.NewKeyPair(key)
	if err != nil {
		t.Fatal(err)
	}
	return kp.Private.String()
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
