package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
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

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenAddr != ":9820" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.CipherMode != "" {
		t.Fatalf("CipherMode = %q, want empty default", cfg.CipherMode)
	}
}

func TestAdminPublicKeySecurityPolicy(t *testing.T) {
	allowed := testPublicKey(1)
	other := testPublicKey(2)
	policy := adminPublicKeySecurityPolicy{PublicKey: allowed}

	if !policy.AllowPeer(other) {
		t.Fatal("AllowPeer should allow peer transport before service selection")
	}
	if !policy.AllowService(allowed, gizclaw.ServiceAdmin) {
		t.Fatal("AllowService should allow configured admin public key for admin service")
	}
	if policy.AllowService(other, gizclaw.ServiceAdmin) {
		t.Fatal("AllowService allowed a different public key")
	}
	if policy.AllowService(allowed, gizclaw.ServiceServerPublic) {
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
	if srv.ContactStore == nil || srv.FriendRequestStore == nil || srv.FriendStore == nil || srv.FriendGroupStore == nil || srv.FriendGroupMemberStore == nil || srv.FriendGroupMessageStore == nil || srv.FriendGroupMessageAssets == nil {
		t.Fatalf("social stores not wired: %+v", srv.Server)
	}
	if srv.FriendOTPTTL != 10*time.Minute || srv.FriendGroupMessageDefaultTTL != 24*time.Hour || srv.FriendGroupMessageMaxTTL != 7*24*time.Hour || srv.FriendGroupMessageCleanup != 5*time.Minute || srv.FriendGroupMessageMaxBytes != 2097152 {
		t.Fatalf("social timing config not wired: friend=%v default=%v max=%v cleanup=%v bytes=%d", srv.FriendOTPTTL, srv.FriendGroupMessageDefaultTTL, srv.FriendGroupMessageMaxTTL, srv.FriendGroupMessageCleanup, srv.FriendGroupMessageMaxBytes)
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
	missingFirmwareCfg.Firmwares.Store = "missing"
	if _, err := New(missingFirmwareCfg); err == nil || !strings.Contains(err.Error(), "server: firmwares store:") {
		t.Fatalf("New(missing firmwares store) = %v", err)
	}

	missingMiniMaxCredentialCfg := validLayeredConfig(dir)
	missingMiniMaxCredentialCfg.MiniMax.CredentialsStore = "missing"
	if _, err := New(missingMiniMaxCredentialCfg); err == nil || !strings.Contains(err.Error(), "server: minimax credentials store:") {
		t.Fatalf("New(missing minimax credentials store) = %v", err)
	}

	missingTenantCfg := validLayeredConfig(dir)
	missingTenantCfg.MiniMax.TenantsStore = "missing"
	if _, err := New(missingTenantCfg); err == nil || !strings.Contains(err.Error(), "server: minimax tenants store:") {
		t.Fatalf("New(missing tenant store) = %v", err)
	}

	missingVoicesCfg := validLayeredConfig(dir)
	missingVoicesCfg.MiniMax.VoicesStore = "missing"
	if _, err := New(missingVoicesCfg); err == nil || !strings.Contains(err.Error(), "server: voices store:") {
		t.Fatalf("New(missing voices store) = %v", err)
	}

	missingWorkspacesCfg := validLayeredConfig(dir)
	missingWorkspacesCfg.Workspaces.Store = "missing"
	if _, err := New(missingWorkspacesCfg); err == nil || !strings.Contains(err.Error(), "server: workspaces store:") {
		t.Fatalf("New(missing workspaces store) = %v", err)
	}

	missingWorkflowsCfg := validLayeredConfig(dir)
	missingWorkflowsCfg.Workflows.Store = "missing"
	if _, err := New(missingWorkflowsCfg); err == nil || !strings.Contains(err.Error(), "server: workflows store:") {
		t.Fatalf("New(missing workflows store) = %v", err)
	}

	missingACLCfg := validLayeredConfig(dir)
	missingACLCfg.ACL.Store = "missing"
	if _, err := New(missingACLCfg); err == nil || !strings.Contains(err.Error(), "server: acl store:") {
		t.Fatalf("New(missing acl store) = %v", err)
	}

	missingPetSpeciesCfg := validLayeredConfig(dir)
	missingPetSpeciesCfg.PetSpecies.Store = "missing"
	if _, err := New(missingPetSpeciesCfg); err == nil || !strings.Contains(err.Error(), "server: pet_species store:") {
		t.Fatalf("New(missing pet species store) = %v", err)
	}

	missingPetSpeciesAssetsCfg := validLayeredConfig(dir)
	missingPetSpeciesAssetsCfg.PetSpecies.AssetsStore = "missing"
	if _, err := New(missingPetSpeciesAssetsCfg); err == nil || !strings.Contains(err.Error(), "server: pet_species assets store:") {
		t.Fatalf("New(missing pet species assets store) = %v", err)
	}

	missingBadgesCfg := validLayeredConfig(dir)
	missingBadgesCfg.Badges.Store = "missing"
	if _, err := New(missingBadgesCfg); err == nil || !strings.Contains(err.Error(), "server: badges store:") {
		t.Fatalf("New(missing badges store) = %v", err)
	}

	missingBadgeAssetsCfg := validLayeredConfig(dir)
	missingBadgeAssetsCfg.Badges.AssetsStore = "missing"
	if _, err := New(missingBadgeAssetsCfg); err == nil || !strings.Contains(err.Error(), "server: badges assets store:") {
		t.Fatalf("New(missing badge assets store) = %v", err)
	}

	missingPetsCfg := validLayeredConfig(dir)
	missingPetsCfg.Pets.Store = "missing"
	if _, err := New(missingPetsCfg); err == nil || !strings.Contains(err.Error(), "server: pets store:") {
		t.Fatalf("New(missing pets store) = %v", err)
	}

	missingRewardsCfg := validLayeredConfig(dir)
	missingRewardsCfg.Rewards.Store = "missing"
	if _, err := New(missingRewardsCfg); err == nil || !strings.Contains(err.Error(), "server: rewards store:") {
		t.Fatalf("New(missing rewards store) = %v", err)
	}

	missingWalletsCfg := validLayeredConfig(dir)
	missingWalletsCfg.Wallets.Store = "missing"
	if _, err := New(missingWalletsCfg); err == nil || !strings.Contains(err.Error(), "server: wallets store:") {
		t.Fatalf("New(missing wallets store) = %v", err)
	}

	missingContactsCfg := validLayeredConfig(dir)
	missingContactsCfg.Contacts.Store = "missing"
	if _, err := New(missingContactsCfg); err == nil || !strings.Contains(err.Error(), "server: contacts store:") {
		t.Fatalf("New(missing contacts store) = %v", err)
	}

	missingFriendRequestsCfg := validLayeredConfig(dir)
	missingFriendRequestsCfg.Friends.RequestsStore = "missing"
	if _, err := New(missingFriendRequestsCfg); err == nil || !strings.Contains(err.Error(), "server: friend requests store:") {
		t.Fatalf("New(missing friend requests store) = %v", err)
	}

	missingFriendsCfg := validLayeredConfig(dir)
	missingFriendsCfg.Friends.Store = "missing"
	if _, err := New(missingFriendsCfg); err == nil || !strings.Contains(err.Error(), "server: friends store:") {
		t.Fatalf("New(missing friends store) = %v", err)
	}

	missingFriendGroupsCfg := validLayeredConfig(dir)
	missingFriendGroupsCfg.FriendGroups.Store = "missing"
	if _, err := New(missingFriendGroupsCfg); err == nil || !strings.Contains(err.Error(), "server: friend_groups store:") {
		t.Fatalf("New(missing friend_groups store) = %v", err)
	}

	missingFriendGroupMembersCfg := validLayeredConfig(dir)
	missingFriendGroupMembersCfg.FriendGroups.MembersStore = "missing"
	if _, err := New(missingFriendGroupMembersCfg); err == nil || !strings.Contains(err.Error(), "server: friend group members store:") {
		t.Fatalf("New(missing group members store) = %v", err)
	}

	missingFriendGroupMessagesCfg := validLayeredConfig(dir)
	missingFriendGroupMessagesCfg.FriendGroups.MessagesStore = "missing"
	if _, err := New(missingFriendGroupMessagesCfg); err == nil || !strings.Contains(err.Error(), "server: friend group messages store:") {
		t.Fatalf("New(missing friend group messages store) = %v", err)
	}

	missingFriendGroupMessageAssetsCfg := validLayeredConfig(dir)
	missingFriendGroupMessageAssetsCfg.FriendGroups.MessageAssetsStore = "missing"
	if _, err := New(missingFriendGroupMessageAssetsCfg); err == nil || !strings.Contains(err.Error(), "server: friend group message assets store:") {
		t.Fatalf("New(missing friend group message assets store) = %v", err)
	}

}

func TestNewWithPreparedConfig(t *testing.T) {
	adminPublicKey := strings.Repeat("ab", giznet.KeySize)
	adminKey, err := giznet.KeyFromHex(adminPublicKey)
	if err != nil {
		t.Fatalf("KeyFromHex error = %v", err)
	}
	srv, err := New(Config{
		ListenAddr:     ":1234",
		AdminPublicKey: adminKey,
		Stores: map[string]stores.Config{
			"mem": {Kind: stores.KindKeyValue, Backend: "memory"},
		},
		Peers: PeersConfig{
			Store: "mem",
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

func TestNewWiresCipherMode(t *testing.T) {
	srv, err := New(Config{
		ListenAddr: ":1234",
		CipherMode: giznet.CipherModeAES256GCM,
		Stores:     map[string]stores.Config{"mem": {Kind: stores.KindKeyValue, Backend: "memory"}},
		Peers:      PeersConfig{Store: "mem"},
	})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if srv.Server.CipherMode != giznet.CipherModeAES256GCM {
		t.Fatalf("CipherMode = %q, want %q", srv.Server.CipherMode, giznet.CipherModeAES256GCM)
	}
}

func TestConfigValidateRequiresStores(t *testing.T) {
	cfg := Config{}
	if err := cfg.validate(); err == nil {
		t.Fatal("validate should fail without required stores")
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

func TestLoadConfigAcceptsTextEncodedAdminPublicKey(t *testing.T) {
	adminKey, err := giznet.KeyFromHex(strings.Repeat("ab", giznet.KeySize))
	if err != nil {
		t.Fatalf("KeyFromHex error = %v", err)
	}
	adminKeyText, err := adminKey.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("admin-public-key: "+string(adminKeyText)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if cfg.AdminPublicKey != adminKey {
		t.Fatalf("AdminPublicKey = %v, want %v", cfg.AdminPublicKey, adminKey)
	}
}

func TestLoadConfigCipherMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("cipher-mode: aes_256_gcm\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if cfg.CipherMode != giznet.CipherModeAES256GCM {
		t.Fatalf("CipherMode = %q, want %q", cfg.CipherMode, giznet.CipherModeAES256GCM)
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
		ListenAddr:     ":9999",
		CipherMode:     giznet.CipherModePlaintext,
		AdminPublicKey: adminKey,
		Storage: map[string]storage.Config{
			"runtime-storage": {Kind: "keyvalue", Backend: "memory"},
		},
		Stores: map[string]stores.Config{
			"runtime": {Kind: "keyvalue", Backend: "memory"},
		},
		Peers: PeersConfig{
			Store: "runtime-peers",
		},
		Credentials: CredentialsConfig{Store: "runtime-credentials"},
		Firmwares:   FirmwaresConfig{Store: "runtime-firmwares"},
		MiniMax: MiniMaxConfig{
			TenantsStore:     "runtime-tenants",
			VoicesStore:      "runtime-voices",
			CredentialsStore: "runtime-credentials",
		},
		Workspaces: WorkspacesConfig{Store: "runtime-workspaces"},
		Workflows:  WorkflowsConfig{Store: "runtime-workflows"},
		ACL:        ACLConfig{Store: "runtime-acl"},
		PetSpecies: AssetResourceConfig{Store: "runtime-pet-species", AssetsStore: "runtime-pet-species-assets"},
		Badges:     AssetResourceConfig{Store: "runtime-badges", AssetsStore: "runtime-badge-assets"},
		Pets:       StoreConfig{Store: "runtime-pets"},
		Rewards:    StoreConfig{Store: "runtime-rewards"},
		Wallets:    StoreConfig{Store: "runtime-wallets"},
		Contacts:   StoreConfig{Store: "runtime-contacts"},
		Friends:    FriendsConfig{RequestsStore: "runtime-friend-requests", Store: "runtime-friends", FriendOTPTTL: "2m"},
		FriendGroups: FriendGroupsConfig{
			Store:                  "runtime-friend-groups",
			MembersStore:           "runtime-friend-group-members",
			MessagesStore:          "runtime-friend-group-messages",
			MessageAssetsStore:     "runtime-friend-group-message-assets",
			MessageDefaultTTL:      "2h",
			MessageMaxTTL:          "3d",
			MessageCleanupInterval: "30s",
			MessageMaxAudioBytes:   1024,
		},
		SystemTasks: SystemTasksConfig{
			RewardClaim: RewardClaimTaskConfig{Generator: "model/runtime-reward", Cooldown: "5m"},
			PetAction:   GeneratorTaskConfig{Generator: "model/runtime-pet"},
		},
	}
	fileCfg := ConfigFile{
		ListenAddr:     ":1234",
		CipherMode:     giznet.CipherModeAES256GCM,
		AdminPublicKey: fileAdminKey,
		Storage: map[string]storage.Config{
			"file-storage": {Kind: "keyvalue", Backend: "memory"},
		},
		Stores: map[string]stores.Config{
			"file": {Kind: "keyvalue", Backend: "memory"},
		},
		Peers: PeersConfig{
			Store: "file-peers",
		},
		Credentials: CredentialsConfig{Store: "file-credentials"},
		Firmwares:   FirmwaresConfig{Store: "file-firmwares"},
		MiniMax: MiniMaxConfig{
			TenantsStore:     "file-tenants",
			VoicesStore:      "file-voices",
			CredentialsStore: "file-credentials",
		},
		Workspaces: WorkspacesConfig{Store: "file-workspaces"},
		Workflows:  WorkflowsConfig{Store: "file-workflows"},
		ACL:        ACLConfig{Store: "file-acl"},
		PetSpecies: AssetResourceConfig{Store: "file-pet-species", AssetsStore: "file-pet-species-assets"},
		Badges:     AssetResourceConfig{Store: "file-badges", AssetsStore: "file-badge-assets"},
		Pets:       StoreConfig{Store: "file-pets"},
		Rewards:    StoreConfig{Store: "file-rewards"},
		Wallets:    StoreConfig{Store: "file-wallets"},
		Contacts:   StoreConfig{Store: "file-contacts"},
		Friends:    FriendsConfig{RequestsStore: "file-friend-requests", Store: "file-friends", FriendOTPTTL: "10m"},
		FriendGroups: FriendGroupsConfig{
			Store:                  "file-friend-groups",
			MembersStore:           "file-friend-group-members",
			MessagesStore:          "file-friend-group-messages",
			MessageAssetsStore:     "file-friend-group-message-assets",
			MessageDefaultTTL:      "24h",
			MessageMaxTTL:          "7d",
			MessageCleanupInterval: "5m",
			MessageMaxAudioBytes:   2048,
		},
		SystemTasks: SystemTasksConfig{
			RewardClaim: RewardClaimTaskConfig{Generator: "model/file-reward", Cooldown: "30m"},
			PetAction:   GeneratorTaskConfig{Generator: "model/file-pet"},
		},
	}

	merged, err := mergeFileConfig(runtimeCfg, fileCfg)
	if err != nil {
		t.Fatalf("mergeFileConfig error = %v", err)
	}
	if merged.ListenAddr != ":9999" {
		t.Fatalf("ListenAddr = %q", merged.ListenAddr)
	}
	if merged.CipherMode != giznet.CipherModePlaintext {
		t.Fatalf("CipherMode = %q, want %q", merged.CipherMode, giznet.CipherModePlaintext)
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
	if merged.Peers.Store != "runtime-peers" {
		t.Fatalf("Peers.Store = %q", merged.Peers.Store)
	}
	if merged.Credentials.Store != "runtime-credentials" {
		t.Fatalf("Credentials.Store = %q", merged.Credentials.Store)
	}
	if merged.Firmwares.Store != "runtime-firmwares" {
		t.Fatalf("Firmwares.Store = %q", merged.Firmwares.Store)
	}
	if merged.MiniMax.TenantsStore != "runtime-tenants" || merged.MiniMax.VoicesStore != "runtime-voices" || merged.MiniMax.CredentialsStore != "runtime-credentials" {
		t.Fatalf("MiniMax = %+v", merged.MiniMax)
	}
	if merged.Workspaces.Store != "runtime-workspaces" {
		t.Fatalf("Workspaces = %+v", merged.Workspaces)
	}
	if merged.Workflows.Store != "runtime-workflows" {
		t.Fatalf("Workflows.Store = %q", merged.Workflows.Store)
	}
	if merged.ACL.Store != "runtime-acl" {
		t.Fatalf("ACL.Store = %q", merged.ACL.Store)
	}
	if merged.PetSpecies.Store != "runtime-pet-species" || merged.PetSpecies.AssetsStore != "runtime-pet-species-assets" {
		t.Fatalf("PetSpecies = %+v", merged.PetSpecies)
	}
	if merged.Badges.Store != "runtime-badges" || merged.Badges.AssetsStore != "runtime-badge-assets" {
		t.Fatalf("Badges = %+v", merged.Badges)
	}
	if merged.Pets.Store != "runtime-pets" || merged.Rewards.Store != "runtime-rewards" || merged.Wallets.Store != "runtime-wallets" {
		t.Fatalf("business stores = pets:%+v rewards:%+v wallets:%+v", merged.Pets, merged.Rewards, merged.Wallets)
	}
	if merged.Contacts.Store != "runtime-contacts" || merged.Friends.Store != "runtime-friends" || merged.Friends.RequestsStore != "runtime-friend-requests" || merged.Friends.FriendOTPTTL != "2m" {
		t.Fatalf("social friend/contact config = contacts:%+v friends:%+v", merged.Contacts, merged.Friends)
	}
	if merged.FriendGroups.Store != "runtime-friend-groups" || merged.FriendGroups.MembersStore != "runtime-friend-group-members" || merged.FriendGroups.MessagesStore != "runtime-friend-group-messages" || merged.FriendGroups.MessageAssetsStore != "runtime-friend-group-message-assets" || merged.FriendGroups.MessageDefaultTTL != "2h" || merged.FriendGroups.MessageMaxTTL != "3d" || merged.FriendGroups.MessageCleanupInterval != "30s" || merged.FriendGroups.MessageMaxAudioBytes != 1024 {
		t.Fatalf("social friend group config = friend_groups:%+v", merged.FriendGroups)
	}
	if merged.SystemTasks.RewardClaim.Generator != "model/runtime-reward" || merged.SystemTasks.RewardClaim.Cooldown != "5m" || merged.SystemTasks.PetAction.Generator != "model/runtime-pet" {
		t.Fatalf("SystemTasks = %+v", merged.SystemTasks)
	}
}

func TestMergeFileConfigUsesFileCipherModeWhenRuntimeEmpty(t *testing.T) {
	merged, err := mergeFileConfig(Config{}, ConfigFile{CipherMode: giznet.CipherModeAES256GCM})
	if err != nil {
		t.Fatalf("mergeFileConfig error = %v", err)
	}
	if merged.CipherMode != giznet.CipherModeAES256GCM {
		t.Fatalf("CipherMode = %q, want %q", merged.CipherMode, giznet.CipherModeAES256GCM)
	}
}

func TestValidateReportsSpecificMissingFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "missing peers store",
			cfg:  Config{},
			want: "server: peers.store is required",
		},
		{
			name: "invalid cipher mode",
			cfg:  Config{CipherMode: giznet.CipherMode("bad")},
			want: "server: unsupported cipher-mode \"bad\"",
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
		Storage:     map[string]storage.Config{"memory": {Kind: storage.KindKeyValue, Memory: &storage.MemoryConfig{}}},
		Peers:       PeersConfig{Store: "peers"},
		Credentials: CredentialsConfig{Store: "credentials"},
		Firmwares:   FirmwaresConfig{Store: "firmwares"},
		MiniMax:     MiniMaxConfig{TenantsStore: "minimax-tenants", VoicesStore: "voices", CredentialsStore: "credentials"},
		Workspaces:  WorkspacesConfig{Store: "workspaces"},
		Workflows:   WorkflowsConfig{Store: "workflows"},
		ACL:         ACLConfig{Store: "acl"},
		PetSpecies:  AssetResourceConfig{Store: "pet-species", AssetsStore: "pet-species-assets"},
		Badges:      AssetResourceConfig{Store: "badges", AssetsStore: "badge-assets"},
		Pets:        StoreConfig{Store: "pets"},
		Rewards:     StoreConfig{Store: "rewards"},
		Wallets:     StoreConfig{Store: "wallets"},
	}
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"missing credentials", func(c *Config) { c.Credentials.Store = "" }, "server: credentials.store is required"},
		{"missing firmwares", func(c *Config) { c.Firmwares.Store = "" }, "server: firmwares.store is required"},
		{"missing minimax tenants", func(c *Config) { c.MiniMax.TenantsStore = "" }, "server: minimax.tenants-store is required"},
		{"missing minimax voices", func(c *Config) { c.MiniMax.VoicesStore = "" }, "server: minimax.voices-store is required"},
		{"missing minimax credentials", func(c *Config) { c.MiniMax.CredentialsStore = "" }, "server: minimax.credentials-store is required"},
		{"missing workspaces", func(c *Config) { c.Workspaces.Store = "" }, "server: workspaces.store is required"},
		{"missing workflows", func(c *Config) { c.Workflows.Store = "" }, "server: workflows.store is required"},
		{"missing acl", func(c *Config) { c.ACL.Store = "" }, "server: acl.store is required"},
		{"bad reward generator", func(c *Config) { c.SystemTasks.RewardClaim.Generator = "voice/main" }, "server: system_tasks.reward_claim.generator must match model/<id>"},
		{"bad pet generator", func(c *Config) { c.SystemTasks.PetAction.Generator = "voice/main" }, "server: system_tasks.pet_action.generator must match model/<id>"},
		{"bad cooldown", func(c *Config) { c.SystemTasks.RewardClaim.Cooldown = "soon" }, "server: system_tasks.reward_claim.cooldown: time: invalid duration \"soon\""},
		{"bad friend otp ttl", func(c *Config) { c.Friends.FriendOTPTTL = "later" }, "server: friends.friend_otp_ttl: time: invalid duration \"later\""},
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

func TestPrepareConfigGeneratesKeyPairAndDefaultListenAddr(t *testing.T) {
	cfg, err := prepareConfig(Config{
		Peers: PeersConfig{Store: "g"},
	})
	if err != nil {
		t.Fatalf("prepareConfig error = %v", err)
	}
	if cfg.KeyPair == nil {
		t.Fatal("KeyPair should be generated")
	}
	if cfg.ListenAddr != DefaultConfig().ListenAddr {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, DefaultConfig().ListenAddr)
	}
}

func TestNewRejectsUnknownStores(t *testing.T) {
	_, err := New(Config{
		Stores: map[string]stores.Config{
			"bad": {Kind: "keyvalue", Backend: "unknown"},
		},
		Peers: PeersConfig{Store: "bad"},
	})
	if err == nil || !strings.Contains(err.Error(), "server: stores:") {
		t.Fatalf("New error = %v", err)
	}
}

func TestNewRejectsMissingNamedStores(t *testing.T) {
	_, err := New(Config{
		Stores: map[string]stores.Config{
			"mem": {Kind: "keyvalue", Backend: "memory"},
		},
		Peers: PeersConfig{Store: "missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "server: peers store:") {
		t.Fatalf("New error = %v", err)
	}

}

func validLayeredConfig(dir string) Config {
	return Config{
		ListenAddr: ":1234",
		Storage: map[string]storage.Config{
			"memory":      {Kind: storage.KindKeyValue, Memory: &storage.MemoryConfig{}},
			"local-files": {Kind: storage.KindObjectStore, FS: &storage.FSConfig{Dir: dir}},
			"acl-db":      {Kind: storage.KindSQL, SQLite: &storage.SQLConfig{Dir: filepath.Join(dir, "acl.sqlite")}},
			"wallet-db":   {Kind: storage.KindSQL, SQLite: &storage.SQLConfig{Dir: filepath.Join(dir, "wallet.sqlite")}},
		},
		Stores: map[string]stores.Config{
			"peers":                       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "peers"},
			"credentials":                 {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "credentials"},
			"firmwares":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "firmwares"},
			"minimax-tenants":             {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "minimax-tenants"},
			"voices":                      {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "voices"},
			"workspaces":                  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "workspaces"},
			"workflows":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "workflows"},
			"pet-species":                 {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "pet-species"},
			"badges":                      {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "badges"},
			"pets":                        {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "pets"},
			"rewards":                     {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "rewards"},
			"contacts":                    {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "contacts"},
			"friend-requests":             {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-requests"},
			"friends":                     {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friends"},
			"friend-groups":               {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-groups"},
			"friend-group-members":        {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-members"},
			"friend-group-messages":       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-messages"},
			"friend-group-message-assets": {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "friend-group-messages"},
			"pet-species-assets":          {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "pet-species"},
			"badge-assets":                {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "badges"},
			"wallets":                     {Kind: stores.KindSQL, Storage: "wallet-db"},
			"acl":                         {Kind: stores.KindSQL, Storage: "acl-db"},
		},
		Peers:       PeersConfig{Store: "peers"},
		Credentials: CredentialsConfig{Store: "credentials"},
		Firmwares:   FirmwaresConfig{Store: "firmwares"},
		MiniMax: MiniMaxConfig{
			TenantsStore:     "minimax-tenants",
			VoicesStore:      "voices",
			CredentialsStore: "credentials",
		},
		Workspaces: WorkspacesConfig{Store: "workspaces"},
		Workflows:  WorkflowsConfig{Store: "workflows"},
		ACL:        ACLConfig{Store: "acl"},
		PetSpecies: AssetResourceConfig{Store: "pet-species", AssetsStore: "pet-species-assets"},
		Badges:     AssetResourceConfig{Store: "badges", AssetsStore: "badge-assets"},
		Pets:       StoreConfig{Store: "pets"},
		Rewards:    StoreConfig{Store: "rewards"},
		Wallets:    StoreConfig{Store: "wallets"},
		Contacts:   StoreConfig{Store: "contacts"},
		Friends:    FriendsConfig{RequestsStore: "friend-requests", Store: "friends", FriendOTPTTL: "10m"},
		FriendGroups: FriendGroupsConfig{
			Store:                  "friend-groups",
			MembersStore:           "friend-group-members",
			MessagesStore:          "friend-group-messages",
			MessageAssetsStore:     "friend-group-message-assets",
			MessageDefaultTTL:      "24h",
			MessageMaxTTL:          "7d",
			MessageCleanupInterval: "5m",
			MessageMaxAudioBytes:   2097152,
		},
		SystemTasks: SystemTasksConfig{
			RewardClaim: RewardClaimTaskConfig{Generator: "model/reward-claim", Cooldown: "30m"},
			PetAction:   GeneratorTaskConfig{Generator: "model/pet-action"},
		},
	}
}
