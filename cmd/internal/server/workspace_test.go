package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
)

func TestPrepareWorkspaceConfigLoadsWorkspaceConfig(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(fmt.Sprintf(`
endpoint: "127.0.0.1:39001"
admin-public-key: %q
storage:
  memory:
    kind: keyvalue
    memory: {}
  local-files:
    kind: objectstore
    fs:
      dir: .
  acl-db:
    kind: sql
    sqlite:
      dir: data/acl.sqlite
  wallet-db:
    kind: sql
    sqlite:
      dir: data/wallet.sqlite
stores:
  peers:
    kind: keyvalue
    storage: memory
    prefix: peers
  credentials:
    kind: keyvalue
    storage: memory
    prefix: credentials
  firmwares:
    kind: keyvalue
    storage: memory
    prefix: firmwares
  minimax-tenants:
    kind: keyvalue
    storage: memory
    prefix: minimax-tenants
  voices:
    kind: keyvalue
    storage: memory
    prefix: voices
  workspaces:
    kind: keyvalue
    storage: memory
    prefix: workspaces
  workflows:
    kind: keyvalue
    storage: memory
    prefix: workflows
  pet-species:
    kind: keyvalue
    storage: memory
    prefix: pet-species
  badges:
    kind: keyvalue
    storage: memory
    prefix: badges
  pets:
    kind: keyvalue
    storage: memory
    prefix: pets
  rewards:
    kind: keyvalue
    storage: memory
    prefix: rewards
  pet-species-assets:
    kind: objectstore
    storage: local-files
    prefix: pet-species
  badge-assets:
    kind: objectstore
    storage: local-files
    prefix: badges
  wallets:
    kind: sql
    storage: wallet-db
  acl:
    kind: sql
    storage: acl-db
system_tasks:
  reward_claim:
    generator: model/reward-claim
    cooldown: 30m
  pet_action:
    generator: model/pet-action
gameplay:
  pet_adopt_point_cost: -1
`, testKeyPair(t, 0xab).Public.String())), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		t.Fatalf("prepareWorkspaceConfig error = %v", err)
	}
	if cfg.KeyPair == nil {
		t.Fatal("KeyPair should not be nil")
	}
	if cfg.Endpoint != "127.0.0.1:39001" {
		t.Fatalf("Endpoint = %q", cfg.Endpoint)
	}
	adminKey := testKeyPair(t, 0xab).Public
	if cfg.AdminPublicKey != adminKey {
		t.Fatalf("AdminPublicKey = %v", cfg.AdminPublicKey)
	}
	if got := cfg.Storage["memory"].Dir; got != "" {
		t.Fatalf("memory store dir = %q", got)
	}
	if got := cfg.Storage["local-files"].FS.Dir; got != workspace {
		t.Fatalf("local-files dir = %q", got)
	}
	if got := cfg.Storage["acl-db"].SQLite.Dir; got != filepath.Join(workspace, "data", "acl.sqlite") {
		t.Fatalf("acl db dir = %q", got)
	}
	if cfg.Gameplay.PetAdoptPointCost != -1 {
		t.Fatalf("Gameplay = %+v", cfg.Gameplay)
	}
}

func TestPrepareWorkspaceConfigUsesDefaultPorts(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(`
stores:
  mem:
    kind: keyvalue
    backend: memory
peers:
  store: mem
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		t.Fatalf("prepareWorkspaceConfig error = %v", err)
	}
	defaults := DefaultConfig()
	if cfg.Endpoint != defaults.Endpoint {
		t.Fatalf("default endpoint = %q, want %q", cfg.Endpoint, defaults.Endpoint)
	}
}

func TestPrepareWorkspaceConfigLoadError(t *testing.T) {
	_, err := prepareWorkspaceConfig(t.TempDir())
	if err == nil {
		t.Fatal("prepareWorkspaceConfig should fail without config.yaml")
	}
}

func TestPrepareWorkspaceConfigResolvesRelativeStoreDirs(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(`
storage:
  memory:
    kind: keyvalue
    memory: {}
  fw-files:
    kind: objectstore
    fs:
      dir: .
  acl-db:
    kind: sql
    sqlite:
      dir: data/acl.sqlite
  wallet-db:
    kind: sql
    sqlite:
      dir: data/wallet.sqlite
stores:
  fw-meta:
    kind: keyvalue
    storage: memory
    prefix: files-meta
  fw-assets:
    kind: objectstore
    storage: fw-files
    prefix: firmware
  pet-species-assets:
    kind: objectstore
    storage: fw-files
    prefix: pet-species
  badge-assets:
    kind: objectstore
    storage: fw-files
    prefix: badges
  wallets:
    kind: sql
    storage: wallet-db
  acl:
    kind: sql
    storage: acl-db
system_tasks:
  reward_claim:
    generator: model/reward-claim
  pet_action:
    generator: model/pet-action
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := prepareWorkspaceConfig(workspace)
	if err != nil {
		t.Fatalf("prepareWorkspaceConfig error = %v", err)
	}
	if got := cfg.Storage["fw-files"].FS.Dir; got != workspace {
		t.Fatalf("fw dir = %q", got)
	}
	if got := cfg.Stores["fw-assets"].Prefix; got != "firmware" {
		t.Fatalf("fw-assets prefix = %q", got)
	}
}

func TestPrepareWorkspaceConfigIdentityError(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(`
stores:
  mem:
    kind: keyvalue
    backend: memory
peers:
  store: mem
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, workspaceIdentityFile), 0o755); err != nil {
		t.Fatalf("Mkdir error = %v", err)
	}

	_, err := prepareWorkspaceConfig(workspace)
	if err == nil {
		t.Fatal("prepareWorkspaceConfig should fail when identity.key is a directory")
	}
}

func TestResolveWorkspaceStoreConfigsPreservesAbsoluteDirs(t *testing.T) {
	root := t.TempDir()
	absoluteDir := filepath.Join(t.TempDir(), "files")

	gotStorage := resolveWorkspaceStorageConfigs(root, map[string]storage.Config{
		"fw": {
			Kind: storage.KindObjectStore,
			FS:   &storage.FSConfig{Dir: absoluteDir},
		},
	})
	if gotStorage["fw"].FS.Dir != absoluteDir {
		t.Fatalf("fw storage dir = %q, want %q", gotStorage["fw"].FS.Dir, absoluteDir)
	}

	gotStores := resolveWorkspaceStoreConfigs(root, map[string]stores.Config{
		"kv": {
			Kind:    stores.KindKeyValue,
			Backend: "badger",
			Dir:     absoluteDir,
		},
	})
	if gotStores["kv"].Dir != absoluteDir {
		t.Fatalf("kv store dir = %q, want %q", gotStores["kv"].Dir, absoluteDir)
	}
}

func TestServeRejectsDirectRun(t *testing.T) {
	err := Serve(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "direct serve is disabled") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Serve(direct) err = %v", err)
	}
}

func TestForceServeReturnsWorkspaceLoadError(t *testing.T) {
	err := ServeContext(context.Background(), t.TempDir(), ServeOptions{Force: true})
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("force serve err = %v, want workspace load error", err)
	}
}

func TestServeReturnsServerBuildError(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(`
stores:
  bad:
    kind: keyvalue
    backend: unknown
peers:
  store: bad
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	err := ServeContext(context.Background(), workspace, ServeOptions{Force: true})
	if err == nil {
		t.Fatal("service-managed serve should fail when New cannot build stores")
	}
}

func TestServeContextClosesStoresWhenPIDAcquireFails(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, workspaceConfigFile), []byte(`
storage:
  main-kv:
    kind: keyvalue
    badger:
      dir: data/kv
  local-files:
    kind: objectstore
    fs:
      dir: .
  acl-db:
    kind: sql
    sqlite:
      dir: data/acl.sqlite
  wallet-db:
    kind: sql
    sqlite:
      dir: data/wallet.sqlite
stores:
  peers:
    kind: keyvalue
    storage: main-kv
    prefix: peers
  credentials:
    kind: keyvalue
    storage: main-kv
    prefix: credentials
  firmwares:
    kind: keyvalue
    storage: main-kv
    prefix: firmwares
  minimax-tenants:
    kind: keyvalue
    storage: main-kv
    prefix: minimax-tenants
  voices:
    kind: keyvalue
    storage: main-kv
    prefix: voices
  workspaces:
    kind: keyvalue
    storage: main-kv
    prefix: workspaces
  workflows:
    kind: keyvalue
    storage: main-kv
    prefix: workflows
  pet-species:
    kind: keyvalue
    storage: main-kv
    prefix: pet-species
  badges:
    kind: keyvalue
    storage: main-kv
    prefix: badges
  pets:
    kind: keyvalue
    storage: main-kv
    prefix: pets
  rewards:
    kind: keyvalue
    storage: main-kv
    prefix: rewards
  pet-species-assets:
    kind: objectstore
    storage: local-files
    prefix: pet-species
  badge-assets:
    kind: objectstore
    storage: local-files
    prefix: badges
  wallets:
    kind: sql
    storage: wallet-db
  acl:
    kind: sql
    storage: acl-db
system_tasks:
  reward_claim:
    generator: model/reward-claim
    cooldown: 30m
  pet_action:
    generator: model/pet-action
`), 0o644); err != nil {
		t.Fatalf("WriteFile config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, workspacePIDFile), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		t.Fatalf("WriteFile pid error = %v", err)
	}

	err := ServeContext(context.Background(), workspace, ServeOptions{Force: true})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("ServeContext() err = %v", err)
	}

	reopened, err := storage.New(map[string]storage.Config{
		"main-kv": {Kind: storage.KindKeyValue, Badger: &storage.BadgerConfig{Dir: filepath.Join(workspace, "data", "kv")}},
	})
	if err != nil {
		t.Fatalf("storage should be closed after PID error, reopen: %v", err)
	}
	defer reopened.Close()
}

func TestHandleExistingWorkspacePIDRejectsStaleWithoutForce(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	err := handleExistingWorkspacePID(pidPath, false)
	if err == nil || !strings.Contains(err.Error(), "stale pid file") {
		t.Fatalf("handleExistingWorkspacePID() err = %v", err)
	}
}

func TestHandleExistingWorkspacePIDForceRemovesStale(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if err := handleExistingWorkspacePID(pidPath, true); err != nil {
		t.Fatalf("handleExistingWorkspacePID(force) error = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed, stat err = %v", err)
	}
}

func TestAcquireWorkspacePIDRejectsRunningPID(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	_, err := acquireWorkspacePID(workspace, false)
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("acquireWorkspacePID() err = %v", err)
	}
}

func TestHandleExistingWorkspacePIDForceRemovesUnreadablePID(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("not-a-pid\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if err := handleExistingWorkspacePID(pidPath, true); err != nil {
		t.Fatalf("handleExistingWorkspacePID(force invalid) error = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed, stat err = %v", err)
	}
}

func TestReadWorkspacePIDRejectsInvalidPID(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), workspacePIDFile)
	if err := os.WriteFile(pidPath, []byte("0\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if _, err := readWorkspacePID(pidPath); err == nil {
		t.Fatal("readWorkspacePID invalid pid error = nil")
	}
}

func TestProcessRunningAndWaitForProcessExitForMissingPID(t *testing.T) {
	if processRunning(0) {
		t.Fatal("processRunning(0) = true")
	}
	if err := waitForProcessExit(999999, time.Millisecond); err != nil {
		t.Fatalf("waitForProcessExit(missing) error = %v", err)
	}
}

func TestAcquireWorkspacePIDWritesAndRemovesCurrentPID(t *testing.T) {
	workspace := t.TempDir()

	release, err := acquireWorkspacePID(workspace, false)
	if err != nil {
		t.Fatalf("acquireWorkspacePID error = %v", err)
	}

	pidPath := filepath.Join(workspace, workspacePIDFile)
	pid, err := readWorkspacePID(pidPath)
	if err != nil {
		t.Fatalf("readWorkspacePID error = %v", err)
	}
	if pid != os.Getpid() {
		t.Fatalf("pid = %d, want %d", pid, os.Getpid())
	}

	release()
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file should be removed, stat err = %v", err)
	}
}
