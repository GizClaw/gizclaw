package server

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/goccy/go-yaml"
)

type Config struct {
	KeyPair        *giznet.KeyPair
	ListenAddr     string
	CipherMode     giznet.CipherMode
	AdminPublicKey giznet.PublicKey
	Storage        map[string]storage.Config
	Stores         map[string]stores.Config
	Peers          PeersConfig
	Credentials    CredentialsConfig
	Firmwares      FirmwaresConfig
	MiniMax        MiniMaxConfig
	Workspaces     WorkspacesConfig
	Workflows      WorkflowsConfig
	ACL            ACLConfig
	PetSpecies     AssetResourceConfig
	Badges         AssetResourceConfig
	Pets           StoreConfig
	Rewards        StoreConfig
	Wallets        StoreConfig
	Contacts       StoreConfig
	Friends        FriendsConfig
	FriendGroups   FriendGroupsConfig
	SystemTasks    SystemTasksConfig
}

type StoreConfig struct {
	Store string `yaml:"store"`
}

type FriendsConfig struct {
	Store         string `yaml:"store"`
	RequestsStore string `yaml:"requests_store"`
	FriendOTPTTL  string `yaml:"friend_otp_ttl"`
}

type FriendGroupsConfig struct {
	Store                  string `yaml:"store"`
	MembersStore           string `yaml:"members_store"`
	MessagesStore          string `yaml:"messages_store"`
	MessageAssetsStore     string `yaml:"message_assets_store"`
	MessageDefaultTTL      string `yaml:"message_default_ttl"`
	MessageMaxTTL          string `yaml:"message_max_ttl"`
	MessageCleanupInterval string `yaml:"message_cleanup_interval"`
	MessageMaxAudioBytes   int64  `yaml:"message_max_audio_bytes"`
}

type AssetResourceConfig struct {
	Store       string `yaml:"store"`
	AssetsStore string `yaml:"assets_store"`
}

type PeersConfig struct {
	Store string `yaml:"store"`
}

type CredentialsConfig struct {
	Store string `yaml:"store"`
}

type FirmwaresConfig struct {
	Store string `yaml:"store"`
}

type MiniMaxConfig struct {
	TenantsStore     string `yaml:"tenants-store"`
	VoicesStore      string `yaml:"voices-store"`
	CredentialsStore string `yaml:"credentials-store"`
}

type WorkspacesConfig struct {
	Store string `yaml:"store"`
}

type WorkflowsConfig struct {
	Store string `yaml:"store"`
}

type ACLConfig struct {
	Store string `yaml:"store"`
}

type SystemTasksConfig struct {
	RewardClaim RewardClaimTaskConfig `yaml:"reward_claim"`
	PetAction   GeneratorTaskConfig   `yaml:"pet_action"`
}

type RewardClaimTaskConfig struct {
	Generator string `yaml:"generator"`
	Cooldown  string `yaml:"cooldown"`
}

type GeneratorTaskConfig struct {
	Generator string `yaml:"generator"`
}

type ConfigFile struct {
	ListenAddr     string                    `yaml:"listen"`
	CipherMode     giznet.CipherMode         `yaml:"cipher-mode"`
	AdminPublicKey giznet.PublicKey          `yaml:"admin-public-key"`
	Storage        map[string]storage.Config `yaml:"storage"`
	Stores         map[string]stores.Config  `yaml:"stores"`
	Peers          PeersConfig               `yaml:"peers"`
	Credentials    CredentialsConfig         `yaml:"credentials"`
	Firmwares      FirmwaresConfig           `yaml:"firmwares"`
	MiniMax        MiniMaxConfig             `yaml:"minimax"`
	Workspaces     WorkspacesConfig          `yaml:"workspaces"`
	Workflows      WorkflowsConfig           `yaml:"workflows"`
	ACL            ACLConfig                 `yaml:"acl"`
	PetSpecies     AssetResourceConfig       `yaml:"pet_species"`
	Badges         AssetResourceConfig       `yaml:"badges"`
	Pets           StoreConfig               `yaml:"pets"`
	Rewards        StoreConfig               `yaml:"rewards"`
	Wallets        StoreConfig               `yaml:"wallets"`
	Contacts       StoreConfig               `yaml:"contacts"`
	Friends        FriendsConfig             `yaml:"friends"`
	FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
	SystemTasks    SystemTasksConfig         `yaml:"system_tasks"`
}

func LoadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	var raw struct {
		ListenAddr     string                    `yaml:"listen"`
		CipherMode     giznet.CipherMode         `yaml:"cipher-mode"`
		AdminPublicKey *giznet.PublicKey         `yaml:"admin-public-key"`
		Storage        map[string]storage.Config `yaml:"storage"`
		Stores         map[string]stores.Config  `yaml:"stores"`
		Peers          PeersConfig               `yaml:"peers"`
		Credentials    CredentialsConfig         `yaml:"credentials"`
		Firmwares      FirmwaresConfig           `yaml:"firmwares"`
		MiniMax        MiniMaxConfig             `yaml:"minimax"`
		Workspaces     WorkspacesConfig          `yaml:"workspaces"`
		Workflows      WorkflowsConfig           `yaml:"workflows"`
		ACL            ACLConfig                 `yaml:"acl"`
		PetSpecies     AssetResourceConfig       `yaml:"pet_species"`
		Badges         AssetResourceConfig       `yaml:"badges"`
		Pets           StoreConfig               `yaml:"pets"`
		Rewards        StoreConfig               `yaml:"rewards"`
		Wallets        StoreConfig               `yaml:"wallets"`
		Contacts       StoreConfig               `yaml:"contacts"`
		Friends        FriendsConfig             `yaml:"friends"`
		FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
		SystemTasks    SystemTasksConfig         `yaml:"system_tasks"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigFile{}, err
	}
	var adminPublicKey giznet.PublicKey
	if raw.AdminPublicKey != nil {
		if raw.AdminPublicKey.IsZero() {
			return ConfigFile{}, fmt.Errorf("server: invalid admin-public-key: zero key")
		}
		adminPublicKey = *raw.AdminPublicKey
	}
	cfg := ConfigFile{
		ListenAddr:     raw.ListenAddr,
		CipherMode:     raw.CipherMode,
		AdminPublicKey: adminPublicKey,
		Storage:        raw.Storage,
		Stores:         raw.Stores,
		Peers:          raw.Peers,
		Credentials:    raw.Credentials,
		Firmwares:      raw.Firmwares,
		MiniMax:        raw.MiniMax,
		Workspaces:     raw.Workspaces,
		Workflows:      raw.Workflows,
		ACL:            raw.ACL,
		PetSpecies:     raw.PetSpecies,
		Badges:         raw.Badges,
		Pets:           raw.Pets,
		Rewards:        raw.Rewards,
		Wallets:        raw.Wallets,
		Contacts:       raw.Contacts,
		Friends:        raw.Friends,
		FriendGroups:   raw.FriendGroups,
		SystemTasks:    raw.SystemTasks,
	}
	return cfg, nil
}

func DefaultConfig() Config {
	return Config{
		ListenAddr: ":9820",
	}
}

func mergeFileConfig(cfg Config, fileCfg ConfigFile) (Config, error) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = fileCfg.ListenAddr
	}
	if cfg.CipherMode == "" {
		cfg.CipherMode = fileCfg.CipherMode
	}
	if cfg.AdminPublicKey.IsZero() {
		cfg.AdminPublicKey = fileCfg.AdminPublicKey
	}
	if len(cfg.Stores) == 0 {
		cfg.Stores = fileCfg.Stores
	}
	if len(cfg.Storage) == 0 {
		cfg.Storage = fileCfg.Storage
	}
	cfg.Peers = mergePeersConfig(cfg.Peers, fileCfg.Peers)
	cfg.Credentials = mergeCredentialsConfig(cfg.Credentials, fileCfg.Credentials)
	cfg.Firmwares = mergeFirmwaresConfig(cfg.Firmwares, fileCfg.Firmwares)
	cfg.MiniMax = mergeMiniMaxConfig(cfg.MiniMax, fileCfg.MiniMax)
	cfg.Workspaces = mergeWorkspacesConfig(cfg.Workspaces, fileCfg.Workspaces)
	cfg.Workflows = mergeWorkflowsConfig(cfg.Workflows, fileCfg.Workflows)
	cfg.ACL = mergeACLConfig(cfg.ACL, fileCfg.ACL)
	cfg.PetSpecies = mergeAssetResourceConfig(cfg.PetSpecies, fileCfg.PetSpecies)
	cfg.Badges = mergeAssetResourceConfig(cfg.Badges, fileCfg.Badges)
	cfg.Pets = mergeStoreConfig(cfg.Pets, fileCfg.Pets)
	cfg.Rewards = mergeStoreConfig(cfg.Rewards, fileCfg.Rewards)
	cfg.Wallets = mergeStoreConfig(cfg.Wallets, fileCfg.Wallets)
	cfg.Contacts = mergeStoreConfig(cfg.Contacts, fileCfg.Contacts)
	cfg.Friends = mergeFriendsConfig(cfg.Friends, fileCfg.Friends)
	cfg.FriendGroups = mergeFriendGroupsConfig(cfg.FriendGroups, fileCfg.FriendGroups)
	cfg.SystemTasks = mergeSystemTasksConfig(cfg.SystemTasks, fileCfg.SystemTasks)
	return cfg, nil
}

func mergeStoreConfig(runtime StoreConfig, file StoreConfig) StoreConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeAssetResourceConfig(runtime AssetResourceConfig, file AssetResourceConfig) AssetResourceConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	if runtime.AssetsStore == "" {
		runtime.AssetsStore = file.AssetsStore
	}
	return runtime
}

func mergeFriendsConfig(runtime FriendsConfig, file FriendsConfig) FriendsConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	if runtime.RequestsStore == "" {
		runtime.RequestsStore = file.RequestsStore
	}
	if runtime.FriendOTPTTL == "" {
		runtime.FriendOTPTTL = file.FriendOTPTTL
	}
	return runtime
}

func mergeFriendGroupsConfig(runtime FriendGroupsConfig, file FriendGroupsConfig) FriendGroupsConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	if runtime.MembersStore == "" {
		runtime.MembersStore = file.MembersStore
	}
	if runtime.MessagesStore == "" {
		runtime.MessagesStore = file.MessagesStore
	}
	if runtime.MessageAssetsStore == "" {
		runtime.MessageAssetsStore = file.MessageAssetsStore
	}
	if runtime.MessageDefaultTTL == "" {
		runtime.MessageDefaultTTL = file.MessageDefaultTTL
	}
	if runtime.MessageMaxTTL == "" {
		runtime.MessageMaxTTL = file.MessageMaxTTL
	}
	if runtime.MessageCleanupInterval == "" {
		runtime.MessageCleanupInterval = file.MessageCleanupInterval
	}
	if runtime.MessageMaxAudioBytes == 0 {
		runtime.MessageMaxAudioBytes = file.MessageMaxAudioBytes
	}
	return runtime
}

func mergePeersConfig(runtime PeersConfig, file PeersConfig) PeersConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeCredentialsConfig(runtime CredentialsConfig, file CredentialsConfig) CredentialsConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeFirmwaresConfig(runtime FirmwaresConfig, file FirmwaresConfig) FirmwaresConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeMiniMaxConfig(runtime MiniMaxConfig, file MiniMaxConfig) MiniMaxConfig {
	if runtime.TenantsStore == "" {
		runtime.TenantsStore = file.TenantsStore
	}
	if runtime.VoicesStore == "" {
		runtime.VoicesStore = file.VoicesStore
	}
	if runtime.CredentialsStore == "" {
		runtime.CredentialsStore = file.CredentialsStore
	}
	return runtime
}

func mergeWorkspacesConfig(runtime WorkspacesConfig, file WorkspacesConfig) WorkspacesConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeWorkflowsConfig(runtime WorkflowsConfig, file WorkflowsConfig) WorkflowsConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeACLConfig(runtime ACLConfig, file ACLConfig) ACLConfig {
	if runtime.Store == "" {
		runtime.Store = file.Store
	}
	return runtime
}

func mergeSystemTasksConfig(runtime SystemTasksConfig, file SystemTasksConfig) SystemTasksConfig {
	runtime.RewardClaim = mergeRewardClaimTaskConfig(runtime.RewardClaim, file.RewardClaim)
	runtime.PetAction = mergeGeneratorTaskConfig(runtime.PetAction, file.PetAction)
	return runtime
}

func mergeRewardClaimTaskConfig(runtime RewardClaimTaskConfig, file RewardClaimTaskConfig) RewardClaimTaskConfig {
	if runtime.Generator == "" {
		runtime.Generator = file.Generator
	}
	if runtime.Cooldown == "" {
		runtime.Cooldown = file.Cooldown
	}
	return runtime
}

func mergeGeneratorTaskConfig(runtime GeneratorTaskConfig, file GeneratorTaskConfig) GeneratorTaskConfig {
	if runtime.Generator == "" {
		runtime.Generator = file.Generator
	}
	return runtime
}

func prepareConfig(cfg Config) (Config, error) {
	defaults := DefaultConfig()
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaults.ListenAddr
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	if cfg.KeyPair == nil {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			return Config{}, fmt.Errorf("server: generate key pair: %w", err)
		}
		cfg.KeyPair = keyPair
	}
	return cfg, nil
}

func (cfg Config) validate() error {
	if err := validateCipherMode(cfg.CipherMode); err != nil {
		return err
	}
	if cfg.Peers.Store == "" {
		return fmt.Errorf("server: peers.store is required")
	}
	if len(cfg.Storage) == 0 {
		return nil
	}
	if cfg.Credentials.Store == "" {
		return fmt.Errorf("server: credentials.store is required")
	}
	if cfg.Firmwares.Store == "" {
		return fmt.Errorf("server: firmwares.store is required")
	}
	if cfg.MiniMax.TenantsStore == "" {
		return fmt.Errorf("server: minimax.tenants-store is required")
	}
	if cfg.MiniMax.VoicesStore == "" {
		return fmt.Errorf("server: minimax.voices-store is required")
	}
	if cfg.MiniMax.CredentialsStore == "" {
		return fmt.Errorf("server: minimax.credentials-store is required")
	}
	if cfg.Workspaces.Store == "" {
		return fmt.Errorf("server: workspaces.store is required")
	}
	if cfg.Workflows.Store == "" {
		return fmt.Errorf("server: workflows.store is required")
	}
	if cfg.ACL.Store == "" {
		return fmt.Errorf("server: acl.store is required")
	}
	if err := validateOptionalModelPattern("system_tasks.reward_claim.generator", cfg.SystemTasks.RewardClaim.Generator); err != nil {
		return err
	}
	if err := validateOptionalModelPattern("system_tasks.pet_action.generator", cfg.SystemTasks.PetAction.Generator); err != nil {
		return err
	}
	if cfg.SystemTasks.RewardClaim.Cooldown != "" {
		if _, err := time.ParseDuration(cfg.SystemTasks.RewardClaim.Cooldown); err != nil {
			return fmt.Errorf("server: system_tasks.reward_claim.cooldown: %w", err)
		}
	}
	if cfg.Friends.FriendOTPTTL != "" {
		if _, err := parseConfigDuration(cfg.Friends.FriendOTPTTL); err != nil {
			return fmt.Errorf("server: friends.friend_otp_ttl: %w", err)
		}
	}
	if cfg.FriendGroups.MessageDefaultTTL != "" {
		if _, err := parseConfigDuration(cfg.FriendGroups.MessageDefaultTTL); err != nil {
			return fmt.Errorf("server: friend_groups.message_default_ttl: %w", err)
		}
	}
	if cfg.FriendGroups.MessageMaxTTL != "" {
		if _, err := parseConfigDuration(cfg.FriendGroups.MessageMaxTTL); err != nil {
			return fmt.Errorf("server: friend_groups.message_max_ttl: %w", err)
		}
	}
	if cfg.FriendGroups.MessageCleanupInterval != "" {
		if _, err := parseConfigDuration(cfg.FriendGroups.MessageCleanupInterval); err != nil {
			return fmt.Errorf("server: friend_groups.message_cleanup_interval: %w", err)
		}
	}
	if cfg.FriendGroups.MessageMaxAudioBytes < 0 {
		return fmt.Errorf("server: friend_groups.message_max_audio_bytes must be >= 0")
	}
	return nil
}

func parseConfigDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(value, "d") + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil
	}
	return time.ParseDuration(value)
}

func validateCipherMode(mode giznet.CipherMode) error {
	switch mode {
	case "", giznet.CipherModeChaChaPoly, giznet.CipherModeAES256GCM, giznet.CipherModePlaintext:
		return nil
	default:
		return fmt.Errorf("server: unsupported cipher-mode %q", mode)
	}
}

func validateOptionalModelPattern(field, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	if !strings.HasPrefix(pattern, "model/") || strings.TrimSpace(strings.TrimPrefix(pattern, "model/")) == "" {
		return fmt.Errorf("server: %s must match model/<id>", field)
	}
	return nil
}
