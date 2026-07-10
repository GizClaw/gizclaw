package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging"
	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func testPublicKey(fill byte) giznet.PublicKey {
	var key giznet.PublicKey
	for i := range key {
		key[i] = fill
	}
	return key
}

func testPublicKeyText(fill byte) string {
	return testPublicKey(fill).String()
}

func testPrivateKey(fill byte) giznet.Key {
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	return key
}

func testKeyPair(t *testing.T, fill byte) *giznet.KeyPair {
	t.Helper()
	kp, err := giznet.NewKeyPair(testPrivateKey(fill))
	if err != nil {
		t.Fatalf("NewKeyPair error = %v", err)
	}
	return kp
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Listen != "0.0.0.0:9820" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.Endpoint != "0.0.0.0:9820" {
		t.Fatalf("Endpoint = %q", cfg.Endpoint)
	}
	if cfg.ServeToClients {
		t.Fatal("ServeToClients should default to false")
	}
	if cfg.Log.Level != "info" {
		t.Fatalf("Log.Level = %q, want info", cfg.Log.Level)
	}
}

func TestParseConfigServeToClients(t *testing.T) {
	cfg, err := parseConfigData([]byte(`
serve-to-clients: true
listen: 127.0.0.1:9820
endpoint: 127.0.0.1:9820
`))
	if err != nil {
		t.Fatalf("parseConfigData error = %v", err)
	}
	if !cfg.ServeToClients {
		t.Fatal("ServeToClients = false, want true")
	}
}

func TestParseConfigServingPublicAlias(t *testing.T) {
	cfg, err := parseConfigData([]byte(`
serving-public: true
listen: 127.0.0.1:9820
endpoint: 127.0.0.1:9820
`))
	if err != nil {
		t.Fatalf("parseConfigData error = %v", err)
	}
	if !cfg.ServeToClients {
		t.Fatal("ServeToClients = false, want true from serving-public alias")
	}

	cfg, err = parseConfigData([]byte(`
serve-to-clients: false
serving-public: true
listen: 127.0.0.1:9820
endpoint: 127.0.0.1:9820
`))
	if err != nil {
		t.Fatalf("parseConfigData override error = %v", err)
	}
	if cfg.ServeToClients {
		t.Fatal("ServeToClients = true, want serve-to-clients to override serving-public alias")
	}
}

func TestAdminPublicKeySecurityPolicy(t *testing.T) {
	allowed := testPublicKey(1)
	other := testPublicKey(2)
	policy := adminPublicKeySecurityPolicy{PublicKey: allowed}

	if !policy.AllowPeer(other) {
		t.Fatal("AllowPeer should allow peer transport before service selection")
	}
	if !policy.AllowService(allowed, gizclaw.ServiceAdminHTTP) {
		t.Fatal("AllowService should allow configured admin public key for admin service")
	}
	if policy.AllowService(other, gizclaw.ServiceAdminHTTP) {
		t.Fatal("AllowService allowed a different public key")
	}
	if policy.AllowService(allowed, gizclaw.ServicePeerHTTP) {
		t.Fatal("AllowService allowed a non-admin service")
	}
}

func TestNewWithLayeredStorageConfig(t *testing.T) {
	dir := t.TempDir()
	srv, err := New(validLayeredConfig(dir))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if srv.PeerStore == nil || srv.CredentialStore == nil || srv.FirmwareStore == nil || srv.MiniMaxTenantStore == nil || srv.VoiceStore == nil || srv.WorkspaceStore == nil || srv.WorkflowStore == nil {
		t.Fatalf("module stores not wired: %+v", srv)
	}
	if srv.FirmwareAssets == nil {
		t.Fatalf("firmware assets store not wired: %+v", srv.Server)
	}
	if srv.AgentHostStore == nil {
		t.Fatalf("agenthost store not wired: %+v", srv.Server)
	}
	if srv.ContactStore == nil || srv.FriendInviteTokenStore == nil || srv.FriendStore == nil || srv.FriendGroupStore == nil || srv.FriendGroupInviteTokenStore == nil || srv.FriendGroupMemberStore == nil || srv.FriendGroupMessageStore == nil || srv.FriendGroupMessageAssets == nil {
		t.Fatalf("social stores not wired: %+v", srv.Server)
	}
	if srv.GameRulesetStore == nil || srv.PetDefStore == nil || srv.BadgeDefStore == nil || srv.GameDefStore == nil || srv.GameplayAssets == nil || srv.GameplayDB == nil {
		t.Fatalf("gameplay stores not wired: %+v", srv.Server)
	}
	if srv.FriendGroupMessageDefaultTTL != 24*time.Hour || srv.FriendGroupMessageMaxTTL != 7*24*time.Hour || srv.FriendGroupMessageCleanup != 5*time.Minute || srv.FriendGroupMessageMaxBytes != 2097152 {
		t.Fatalf("social timing config not wired: default=%v max=%v cleanup=%v bytes=%d", srv.FriendGroupMessageDefaultTTL, srv.FriendGroupMessageMaxTTL, srv.FriendGroupMessageCleanup, srv.FriendGroupMessageMaxBytes)
	}
	if srv.ACLDB == nil {
		t.Fatalf("acl store not wired: %v", srv.ACLDB)
	}
}

func TestNewWithLayeredStorageReportsStoreErrors(t *testing.T) {
	dir := t.TempDir()

	storageErrCfg := validLayeredConfig(dir)
	storageErrCfg.Storage["memory"] = storage.Config{Kind: storage.KindKeyValue, Backend: "redis"}
	if _, err := New(storageErrCfg); err == nil || !strings.Contains(err.Error(), "server: stores:") {
		t.Fatalf("New(storage error) = %v", err)
	}

	logicalErrCfg := validLayeredConfig(dir)
	logicalErrCfg.Stores["credentials"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "bad:prefix"}
	if _, err := New(logicalErrCfg); err == nil || !strings.Contains(err.Error(), "server: stores:") {
		t.Fatalf("New(logical store error) = %v", err)
	}

	missingCredentialCfg := validLayeredConfig(dir)
	delete(missingCredentialCfg.Stores, "credentials")
	if _, err := New(missingCredentialCfg); err == nil || !strings.Contains(err.Error(), "server: credentials store:") {
		t.Fatalf("New(missing credentials store) = %v", err)
	}

	missingFirmwareCfg := validLayeredConfig(dir)
	delete(missingFirmwareCfg.Stores, "firmwares")
	if _, err := New(missingFirmwareCfg); err == nil || !strings.Contains(err.Error(), "server: firmwares store:") {
		t.Fatalf("New(missing firmwares store) = %v", err)
	}

	badFirmwareAssetsCfg := validLayeredConfig(dir)
	badFirmwareAssetsCfg.Stores["firmware-assets"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "firmware-assets"}
	if _, err := New(badFirmwareAssetsCfg); err == nil || !strings.Contains(err.Error(), "server: firmwares assets store:") {
		t.Fatalf("New(bad firmware assets store) = %v", err)
	}

	badAgentHostCfg := validLayeredConfig(dir)
	badAgentHostCfg.Stores["agenthost"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "agenthost"}
	if _, err := New(badAgentHostCfg); err == nil || !strings.Contains(err.Error(), "server: agenthost store:") {
		t.Fatalf("New(bad agenthost store) = %v", err)
	}

	missingTenantCfg := validLayeredConfig(dir)
	delete(missingTenantCfg.Stores, "minimax-tenants")
	if _, err := New(missingTenantCfg); err == nil || !strings.Contains(err.Error(), "server: minimax tenants store:") {
		t.Fatalf("New(missing tenant store) = %v", err)
	}

	missingVoicesCfg := validLayeredConfig(dir)
	delete(missingVoicesCfg.Stores, "voices")
	if _, err := New(missingVoicesCfg); err == nil || !strings.Contains(err.Error(), "server: voices store:") {
		t.Fatalf("New(missing voices store) = %v", err)
	}

	missingWorkspacesCfg := validLayeredConfig(dir)
	delete(missingWorkspacesCfg.Stores, "workspaces")
	if _, err := New(missingWorkspacesCfg); err == nil || !strings.Contains(err.Error(), "server: workspaces store:") {
		t.Fatalf("New(missing workspaces store) = %v", err)
	}

	missingWorkflowsCfg := validLayeredConfig(dir)
	delete(missingWorkflowsCfg.Stores, "workflows")
	if _, err := New(missingWorkflowsCfg); err == nil || !strings.Contains(err.Error(), "server: workflows store:") {
		t.Fatalf("New(missing workflows store) = %v", err)
	}

	missingACLCfg := validLayeredConfig(dir)
	delete(missingACLCfg.Stores, "acl")
	if _, err := New(missingACLCfg); err == nil || !strings.Contains(err.Error(), "server: acl store:") {
		t.Fatalf("New(missing acl store) = %v", err)
	}
}

func TestNewWithPreparedConfig(t *testing.T) {
	adminPublicKey := strings.Repeat("ab", giznet.KeySize)
	adminKey, err := giznet.KeyFromHex(adminPublicKey)
	if err != nil {
		t.Fatalf("KeyFromHex error = %v", err)
	}
	srv, err := New(Config{
		Listen:         "127.0.0.1:1234",
		Endpoint:       "127.0.0.1:1234",
		AdminPublicKey: adminKey,
		Stores: map[string]stores.Config{
			"peers": {Kind: stores.KindKeyValue, Backend: "memory"},
		},
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if srv.PeerStore == nil {
		t.Fatal("PeerStore is nil")
	}
	if srv.PublicKey().String() == "" {
		t.Fatal("PublicKey should not be empty")
	}
	if srv.AdminPublicKey != adminKey {
		t.Fatalf("AdminPublicKey = %v, want %v", srv.AdminPublicKey, adminKey)
	}
}

func TestNewWiresLogQueryBackendFromVolcLogConfig(t *testing.T) {
	disabledCfg := Config{
		Listen:   "127.0.0.1:1234",
		Endpoint: "127.0.0.1:1234",
		Stores:   map[string]stores.Config{"peers": {Kind: stores.KindKeyValue, Backend: "memory"}},
	}
	disabled, err := New(disabledCfg)
	if err != nil {
		t.Fatalf("New(disabled) error = %v", err)
	}
	t.Cleanup(func() { _ = disabled.Close() })
	if disabled.ServerLogQuery != nil {
		t.Fatal("disabled Volc logging should not install log query backend")
	}

	enabledCfg := disabledCfg
	enabledCfg.Log = logging.Config{
		Volc: logging.VolcConfig{
			Enabled:         true,
			Endpoint:        "https://tls-cn-beijing.volces.com",
			Region:          "cn-beijing",
			TopicID:         "topic",
			AccessKeyID:     "ak",
			AccessKeySecret: "sk",
		},
	}
	enabled, err := New(enabledCfg)
	if err != nil {
		t.Fatalf("New(enabled) error = %v", err)
	}
	t.Cleanup(func() { _ = enabled.Close() })
	if enabled.ServerLogQuery == nil {
		t.Fatal("enabled Volc logging should install log query backend")
	}
}

func TestNewWiresPeerListenerFactory(t *testing.T) {
	srv, err := New(Config{
		Listen:   "127.0.0.1:1234",
		Endpoint: "127.0.0.1:1234",
		Stores:   map[string]stores.Config{"peers": {Kind: stores.KindKeyValue, Backend: "memory"}},
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if len(srv.Server.PeerListenerFactories) != 1 {
		t.Fatalf("PeerListenerFactories len = %d, want 1", len(srv.Server.PeerListenerFactories))
	}
}

func TestConfigValidateRequiresStores(t *testing.T) {
	cfg := Config{Listen: "127.0.0.1:9820", Endpoint: "127.0.0.1:9820"}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate should allow default store names without service bindings: %v", err)
	}
}

func TestLoadConfigReadsAdminPublicKey(t *testing.T) {
	adminKP := testKeyPair(t, 0x10)
	path := filepath.Join(t.TempDir(), "config.yaml")
	serverKP := testKeyPair(t, 0x11)
	if err := os.WriteFile(path, []byte("identity:\n  private-key: \""+serverKP.Private.String()+"\"\nadmin-public-key: \""+adminKP.Public.String()+"\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if cfg.AdminPublicKey != adminKP.Public {
		t.Fatalf("AdminPublicKey = %v, want %v", cfg.AdminPublicKey, adminKP.Public)
	}
	if cfg.Identity.PrivateKey != serverKP.Private {
		t.Fatalf("Identity.PrivateKey = %v, want %v", cfg.Identity.PrivateKey, serverKP.Private)
	}
}

func TestLoadConfigReadsEdgeNodes(t *testing.T) {
	edgeOne := testPublicKey(0x20)
	edgeTwo := testPublicKey(0x21)
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "edge-nodes:\n  - \"" + edgeOne.String() + "\"\n  - \"" + edgeTwo.String() + "\"\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if len(cfg.EdgeNodes) != 2 || cfg.EdgeNodes[0] != edgeOne || cfg.EdgeNodes[1] != edgeTwo {
		t.Fatalf("EdgeNodes = %+v", cfg.EdgeNodes)
	}
}

func TestLoadConfigReadsICEServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
ice-servers:
  - urls:
      - turn:edge.example.com:3478?transport=udp
      - stun:edge.example.com:3478
    username: user
    credential: pass
`), 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if len(cfg.ICEServers) != 1 {
		t.Fatalf("ICEServers len = %d, want 1", len(cfg.ICEServers))
	}
	if got := cfg.ICEServers[0].URLs; len(got) != 2 || got[0] != "turn:edge.example.com:3478?transport=udp" || got[1] != "stun:edge.example.com:3478" {
		t.Fatalf("ICEServers[0].URLs = %#v", got)
	}
	if cfg.ICEServers[0].Username != "user" {
		t.Fatalf("ICEServers[0].Username = %q", cfg.ICEServers[0].Username)
	}
	if cfg.ICEServers[0].Credential != "pass" {
		t.Fatalf("ICEServers[0].Credential = %q", cfg.ICEServers[0].Credential)
	}
}

func TestNewBootstrapsConfiguredEdgeNodes(t *testing.T) {
	dir := t.TempDir()
	edgeKey := testKeyPair(t, 0x13)
	cfg := validLayeredConfig(dir)
	cfg.EdgeNodes = []giznet.PublicKey{edgeKey.Public}
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	peerStore := &runtimepeer.Server{Store: srv.Server.PeerStore}
	peer, err := peerStore.LoadPeer(context.Background(), edgeKey.Public)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if peer.Role != apitypes.PeerRoleEdgeNode || peer.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("bootstrapped edge peer = %+v", peer)
	}
}

func TestNewBootstrapsConfiguredEdgeNodesWithLegacySharedStore(t *testing.T) {
	edgeKey := testKeyPair(t, 0x14)
	srv, err := New(Config{
		EdgeNodes: []giznet.PublicKey{edgeKey.Public},
		Stores: map[string]stores.Config{
			"peers": {Kind: stores.KindKeyValue, Backend: "memory"},
		},
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	peerStore := &runtimepeer.Server{Store: kv.Prefixed(srv.Server.PeerStore, kv.Key{"peers"})}
	peer, err := peerStore.LoadPeer(context.Background(), edgeKey.Public)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if peer.Role != apitypes.PeerRoleEdgeNode || peer.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("bootstrapped legacy edge peer = %+v", peer)
	}
}

func TestBootstrapEdgeNodesPreservesExistingPeerMetadata(t *testing.T) {
	edgeKey := testKeyPair(t, 0x15)
	peerStore := &runtimepeer.Server{Store: kv.NewMemory(nil)}
	name := "edge-a"
	view := "dashboard"
	autoRegistered := true
	approvedAt := time.Unix(123, 0).UTC()
	existing, err := peerStore.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:      edgeKey.Public.String(),
		Role:           apitypes.PeerRoleClient,
		Status:         apitypes.PeerRegistrationStatusBlocked,
		ApprovedAt:     &approvedAt,
		AutoRegistered: &autoRegistered,
		Device:         apitypes.DeviceInfo{Name: &name},
		Configuration:  apitypes.Configuration{View: &view},
	})
	if err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	if err := bootstrapEdgeNodes(context.Background(), peerStore, []giznet.PublicKey{edgeKey.Public}); err != nil {
		t.Fatalf("bootstrapEdgeNodes error = %v", err)
	}
	peer, err := peerStore.LoadPeer(context.Background(), edgeKey.Public)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if peer.Role != apitypes.PeerRoleEdgeNode || peer.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("bootstrapped edge peer = %+v", peer)
	}
	if peer.Device.Name == nil || *peer.Device.Name != name {
		t.Fatalf("Device.Name = %v, want %q", peer.Device.Name, name)
	}
	if peer.Configuration.View == nil || *peer.Configuration.View != view {
		t.Fatalf("Configuration.View = %v, want %q", peer.Configuration.View, view)
	}
	if peer.AutoRegistered == nil || !*peer.AutoRegistered {
		t.Fatalf("AutoRegistered = %v, want true", peer.AutoRegistered)
	}
	if peer.ApprovedAt == nil || !peer.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("ApprovedAt = %v, want %v", peer.ApprovedAt, approvedAt)
	}
	if !peer.CreatedAt.Equal(existing.CreatedAt) {
		t.Fatalf("CreatedAt = %v, want %v", peer.CreatedAt, existing.CreatedAt)
	}
}

func TestLoadConfigReadsAndExpandsLogConfig(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_VOLC_AK", "ak")
	t.Setenv("GIZCLAW_TEST_VOLC_SK", "sk")
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `
log:
  level: debug
  volc:
    enabled: true
    endpoint: https://tls-cn-beijing.volces.com
    region: cn-beijing
    topic_id: test-topic
    access_key_id: ${GIZCLAW_TEST_VOLC_AK}
    access_key_secret: ${GIZCLAW_TEST_VOLC_SK}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("Log.Level = %q", cfg.Log.Level)
	}
	if !cfg.Log.Volc.Enabled || cfg.Log.Volc.AccessKeyID != "ak" || cfg.Log.Volc.AccessKeySecret != "sk" {
		t.Fatalf("Log.Volc = %+v", cfg.Log.Volc)
	}
}

func TestLoadConfigRejectsInvalidLogConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("log:\n  level: verbose\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "log.level") {
		t.Fatalf("LoadConfig invalid log level err = %v", err)
	}

	if err := os.WriteFile(path, []byte("log:\n  volc:\n    enabled: true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile enabled error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "log.volc.endpoint") {
		t.Fatalf("LoadConfig invalid volc err = %v", err)
	}
}

func TestE2ELogConfigFixturesUseReadablePlaceholders(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "server-workspace", "config.yaml.template"),
	} {
		t.Run(path, func(t *testing.T) {
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("LoadConfig(%s) error = %v", path, err)
			}
			if cfg.Log.Level != "info" {
				t.Fatalf("fixture log level = %q, want info", cfg.Log.Level)
			}
			if cfg.Log.Volc.Enabled {
				t.Fatal("fixture Volc logging should be disabled")
			}
			for name, value := range map[string]string{
				"endpoint":          cfg.Log.Volc.Endpoint,
				"region":            cfg.Log.Volc.Region,
				"topic_id":          cfg.Log.Volc.TopicID,
				"access_key_id":     cfg.Log.Volc.AccessKeyID,
				"access_key_secret": cfg.Log.Volc.AccessKeySecret,
			} {
				if value == "" || strings.Contains(value, "${") {
					t.Fatalf("fixture log.volc.%s = %q, want readable placeholder", name, value)
				}
			}
		})
	}
}

func TestLoadConfigRejectsInvalidIdentityPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("identity:\n  private-key: \"not-a-key\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile invalid error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "invalid key text") {
		t.Fatalf("LoadConfig invalid identity private key err = %v", err)
	}

	if err := os.WriteFile(path, []byte("identity:\n  private-key: \""+testPrivateKey(0).String()+"\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile zero error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "zero key") {
		t.Fatalf("LoadConfig zero identity private key err = %v", err)
	}
}

func TestLoadConfigNormalizesIdentityPrivateKey(t *testing.T) {
	var rawPrivate giznet.Key
	for i := range rawPrivate {
		rawPrivate[i] = 0xff
	}
	want, err := giznet.NewKeyPair(rawPrivate)
	if err != nil {
		t.Fatalf("NewKeyPair error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("identity:\n  private-key: \""+rawPrivate.String()+"\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if cfg.Identity.PrivateKey != want.Private {
		t.Fatalf("identity private key = %s, want normalized %s", cfg.Identity.PrivateKey, want.Private)
	}
}

func TestLoadConfigRejectsInvalidAdminPublicKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("admin-public-key: \"not-a-key\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile invalid error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig should fail for invalid admin public key")
	}

	if err := os.WriteFile(path, []byte("admin-public-key: \""+testPublicKey(0).String()+"\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile zero error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "zero key") {
		t.Fatalf("LoadConfig zero admin public key err = %v", err)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("LoadConfig should fail for a missing file")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("listen: ["), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig should fail for invalid yaml")
	}
}

func TestMergeFileConfigKeepsRuntimeOverrides(t *testing.T) {
	adminKey, err := giznet.KeyFromHex(strings.Repeat("01", giznet.KeySize))
	if err != nil {
		t.Fatalf("KeyFromHex error = %v", err)
	}
	fileAdminKey, err := giznet.KeyFromHex(strings.Repeat("02", giznet.KeySize))
	if err != nil {
		t.Fatalf("KeyFromHex file error = %v", err)
	}
	runtimeCfg := Config{
		Listen:         "0.0.0.0:9999",
		Endpoint:       "127.0.0.1:9999",
		AdminPublicKey: adminKey,
		Storage: map[string]storage.Config{
			"runtime-storage": {Kind: "keyvalue", Backend: "memory"},
		},
		Stores: map[string]stores.Config{
			"runtime": {Kind: "keyvalue", Backend: "memory"},
		},
		FriendGroups: FriendGroupsConfig{
			MessageDefaultTTL:      "2h",
			MessageMaxTTL:          "3d",
			MessageCleanupInterval: "30s",
			MessageMaxAudioBytes:   1024,
		},
		Log: logging.Config{Level: "error"},
	}
	fileCfg := ConfigFile{
		Listen:         "0.0.0.0:1234",
		Endpoint:       "127.0.0.1:1234",
		AdminPublicKey: fileAdminKey,
		Storage: map[string]storage.Config{
			"file-storage": {Kind: "keyvalue", Backend: "memory"},
		},
		Stores: map[string]stores.Config{
			"file": {Kind: "keyvalue", Backend: "memory"},
		},
		FriendGroups: FriendGroupsConfig{
			MessageDefaultTTL:      "24h",
			MessageMaxTTL:          "7d",
			MessageCleanupInterval: "5m",
			MessageMaxAudioBytes:   2048,
		},
		Log: logging.Config{Level: "warn"},
	}

	merged, err := mergeFileConfig(runtimeCfg, fileCfg)
	if err != nil {
		t.Fatalf("mergeFileConfig error = %v", err)
	}
	if merged.Endpoint != "127.0.0.1:9999" {
		t.Fatalf("Endpoint = %q", merged.Endpoint)
	}
	if merged.Listen != "0.0.0.0:9999" {
		t.Fatalf("Listen = %q", merged.Listen)
	}
	if merged.AdminPublicKey != runtimeCfg.AdminPublicKey {
		t.Fatalf("AdminPublicKey = %v, want %v", merged.AdminPublicKey, runtimeCfg.AdminPublicKey)
	}
	if len(merged.Stores) != 1 || merged.Stores["runtime"].Backend != "memory" {
		t.Fatalf("Stores = %+v", merged.Stores)
	}
	if len(merged.Storage) != 1 || merged.Storage["runtime-storage"].Backend != "memory" {
		t.Fatalf("Storage = %+v", merged.Storage)
	}
	if merged.FriendGroups.MessageDefaultTTL != "2h" || merged.FriendGroups.MessageMaxTTL != "3d" || merged.FriendGroups.MessageCleanupInterval != "30s" || merged.FriendGroups.MessageMaxAudioBytes != 1024 {
		t.Fatalf("FriendGroups = %+v", merged.FriendGroups)
	}
	if merged.Log.Level != "error" {
		t.Fatalf("runtime Log should win, got %+v", merged.Log)
	}
	merged, err = mergeFileConfig(Config{}, fileCfg)
	if err != nil {
		t.Fatalf("mergeFileConfig file-only error = %v", err)
	}
	if merged.Log.Level != "warn" {
		t.Fatalf("file Log should be used when runtime is empty, got %+v", merged.Log)
	}
}

func TestValidateReportsSpecificMissingFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "invalid listen",
			cfg:  Config{Listen: "http://127.0.0.1:9820", Endpoint: "127.0.0.1:9820"},
			want: "server: listen must be host:port, got \"http://127.0.0.1:9820\"",
		},
		{
			name: "invalid endpoint",
			cfg:  Config{Listen: "127.0.0.1:9820", Endpoint: "http://127.0.0.1:9820"},
			want: "server: endpoint must be host:port, got \"http://127.0.0.1:9820\"",
		},
		{
			name: "empty endpoint host",
			cfg:  Config{Listen: "127.0.0.1:9820", Endpoint: ":9820"},
			want: "server: endpoint host is empty",
		},
		{
			name: "empty endpoint port",
			cfg:  Config{Listen: "127.0.0.1:9820", Endpoint: "127.0.0.1:"},
			want: "server: endpoint port is empty",
		},
		{
			name: "zero edge node",
			cfg:  Config{Listen: "127.0.0.1:9820", Endpoint: "127.0.0.1:9820", EdgeNodes: []giznet.PublicKey{{}}},
			want: "server: edge-nodes[0] is zero",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if err == nil || err.Error() != tc.want {
				t.Fatalf("validate error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateReportsLayeredStorageMissingFields(t *testing.T) {
	base := Config{
		Listen:   "127.0.0.1:9820",
		Endpoint: "127.0.0.1:9820",
		Storage:  map[string]storage.Config{"memory": {Kind: storage.KindKeyValue, Memory: &storage.MemoryConfig{}}},
	}
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"bad friend group default ttl", func(c *Config) { c.FriendGroups.MessageDefaultTTL = "later" }, "server: friend_groups.message_default_ttl: time: invalid duration \"later\""},
		{"bad friend group max ttl", func(c *Config) { c.FriendGroups.MessageMaxTTL = "later" }, "server: friend_groups.message_max_ttl: time: invalid duration \"later\""},
		{"bad friend group cleanup interval", func(c *Config) { c.FriendGroups.MessageCleanupInterval = "later" }, "server: friend_groups.message_cleanup_interval: time: invalid duration \"later\""},
		{"bad friend group message size", func(c *Config) { c.FriendGroups.MessageMaxAudioBytes = -1 }, "server: friend_groups.message_max_audio_bytes must be >= 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			err := cfg.validate()
			if err == nil || err.Error() != tc.want {
				t.Fatalf("validate error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPrepareConfigGeneratesKeyPairAndDefaultPorts(t *testing.T) {
	cfg, err := prepareConfig(Config{})
	if err != nil {
		t.Fatalf("prepareConfig error = %v", err)
	}
	if cfg.KeyPair == nil {
		t.Fatal("KeyPair should be generated")
	}
	defaults := DefaultConfig()
	if cfg.Listen != defaults.Listen {
		t.Fatalf("Listen = %q, want %q", cfg.Listen, defaults.Listen)
	}
	if cfg.Endpoint != defaults.Endpoint {
		t.Fatalf("Endpoint = %q, want %q", cfg.Endpoint, defaults.Endpoint)
	}
}

func TestNewRejectsUnknownStores(t *testing.T) {
	_, err := New(Config{
		Stores: map[string]stores.Config{
			"peers": {Kind: "keyvalue", Backend: "unknown"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "server: stores:") {
		t.Fatalf("New error = %v", err)
	}
}

func TestNewRejectsMissingDefaultPeerStore(t *testing.T) {
	_, err := New(Config{
		Stores: map[string]stores.Config{
			"mem": {Kind: "keyvalue", Backend: "memory"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "server: peers store:") {
		t.Fatalf("New error = %v", err)
	}

}

func validLayeredConfig(dir string) Config {
	return Config{
		Listen:   "127.0.0.1:1234",
		Endpoint: "127.0.0.1:1234",
		Storage: map[string]storage.Config{
			"memory":      {Kind: storage.KindKeyValue, Memory: &storage.MemoryConfig{}},
			"local-files": {Kind: storage.KindObjectStore, FS: &storage.FSConfig{Dir: dir}},
			"acl-db":      {Kind: storage.KindSQL, SQLite: &storage.SQLConfig{Dir: filepath.Join(dir, "acl.sqlite")}},
			"gameplay-db": {Kind: storage.KindSQL, SQLite: &storage.SQLConfig{Dir: filepath.Join(dir, "gameplay.sqlite")}},
		},
		Stores: map[string]stores.Config{
			"peers":                       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "peers"},
			"credentials":                 {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "credentials"},
			"firmwares":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "firmwares"},
			"firmware-assets":             {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "firmwares"},
			"agenthost":                   {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "agenthost"},
			"minimax-tenants":             {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "minimax-tenants"},
			"voices":                      {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "voices"},
			"workspaces":                  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "workspaces"},
			"workflows":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "workflows"},
			"contacts":                    {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "contacts"},
			"friend-invite-tokens":        {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-invite-tokens"},
			"friends":                     {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friends"},
			"friend-groups":               {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-groups"},
			"friend-group-invite-tokens":  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-invite-tokens"},
			"friend-group-members":        {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-members"},
			"friend-group-messages":       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-messages"},
			"friend-group-message-assets": {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "friend-group-messages"},
			"acl":                         {Kind: stores.KindSQL, Storage: "acl-db"},
			"game-rulesets":               {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "game-rulesets"},
			"pet-defs":                    {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "pet-defs"},
			"badge-defs":                  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "badge-defs"},
			"game-defs":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "game-defs"},
			"gameplay-assets":             {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "gameplay"},
			"gameplay-db":                 {Kind: stores.KindSQL, Storage: "gameplay-db"},
		},
		FriendGroups: FriendGroupsConfig{
			MessageDefaultTTL:      "24h",
			MessageMaxTTL:          "7d",
			MessageCleanupInterval: "5m",
			MessageMaxAudioBytes:   2097152,
		},
	}
}
