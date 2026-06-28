package server

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/goccy/go-yaml"
)

type Config struct {
	KeyPair        *giznet.KeyPair
	Host           string
	PublicAPIPort  int
	NoiseUDPPort   int
	ICEPort        int
	ListenAddr     string
	CipherMode     giznoise.CipherMode
	AdminPublicKey giznet.PublicKey
	Storage        map[string]storage.Config
	Stores         map[string]stores.Config
	Friends        FriendsConfig
	FriendGroups   FriendGroupsConfig
	SystemTasks    SystemTasksConfig
}

type FriendsConfig struct{}

type FriendGroupsConfig struct {
	MessageDefaultTTL      string `yaml:"message_default_ttl"`
	MessageMaxTTL          string `yaml:"message_max_ttl"`
	MessageCleanupInterval string `yaml:"message_cleanup_interval"`
	MessageMaxAudioBytes   int64  `yaml:"message_max_audio_bytes"`
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
	Host           string                    `yaml:"host"`
	PublicAPIPort  int                       `yaml:"public-api-port"`
	NoiseUDPPort   int                       `yaml:"noise-udp-port"`
	ICEPort        int                       `yaml:"ice-port"`
	ListenAddr     string                    `yaml:"listen"`
	CipherMode     giznoise.CipherMode       `yaml:"cipher-mode"`
	AdminPublicKey giznet.PublicKey          `yaml:"admin-public-key"`
	Storage        map[string]storage.Config `yaml:"storage"`
	Stores         map[string]stores.Config  `yaml:"stores"`
	Friends        FriendsConfig             `yaml:"friends"`
	FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
	SystemTasks    SystemTasksConfig         `yaml:"system_tasks"`
}

const (
	defaultPeersStore                    = "peers"
	defaultCredentialsStore              = "credentials"
	defaultFirmwaresStore                = "firmwares"
	defaultFirmwareAssetsStore           = "firmware-assets"
	defaultAgentHostStore                = "agenthost"
	defaultMiniMaxTenantsStore           = "minimax-tenants"
	defaultVoicesStore                   = "voices"
	defaultWorkspacesStore               = "workspaces"
	defaultWorkflowsStore                = "workflows"
	defaultACLStore                      = "acl"
	defaultPetSpeciesStore               = "pet-species"
	defaultPetSpeciesAssetsStore         = "pet-species-assets"
	defaultBadgesStore                   = "badges"
	defaultBadgeAssetsStore              = "badge-assets"
	defaultPetsStore                     = "pets"
	defaultRewardsStore                  = "rewards"
	defaultWalletsStore                  = "wallets"
	defaultContactsStore                 = "contacts"
	defaultFriendInviteTokensStore       = "friend-invite-tokens"
	defaultFriendsStore                  = "friends"
	defaultFriendGroupsStore             = "friend-groups"
	defaultFriendGroupInviteTokensStore  = "friend-group-invite-tokens"
	defaultFriendGroupMembersStore       = "friend-group-members"
	defaultFriendGroupBelongsStore       = "friend-group-belongs"
	defaultFriendGroupMessagesStore      = "friend-group-messages"
	defaultFriendGroupMessageAssetsStore = "friend-group-message-assets"
)

func LoadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	var keyCheck map[string]any
	if err := yaml.Unmarshal(data, &keyCheck); err != nil {
		return ConfigFile{}, err
	}
	if _, ok := keyCheck["admin-private-key"]; ok {
		return ConfigFile{}, fmt.Errorf("server: admin-private-key is not supported; use admin-public-key")
	}
	if _, ok := keyCheck["admin-identity-key"]; ok {
		return ConfigFile{}, fmt.Errorf("server: admin-identity-key is not supported; use admin-public-key")
	}
	var raw struct {
		Host           string                    `yaml:"host"`
		PublicAPIPort  int                       `yaml:"public-api-port"`
		NoiseUDPPort   int                       `yaml:"noise-udp-port"`
		ICEPort        int                       `yaml:"ice-port"`
		ListenAddr     string                    `yaml:"listen"`
		CipherMode     giznoise.CipherMode       `yaml:"cipher-mode"`
		AdminPublicKey *giznet.PublicKey         `yaml:"admin-public-key"`
		Storage        map[string]storage.Config `yaml:"storage"`
		Stores         map[string]stores.Config  `yaml:"stores"`
		Friends        FriendsConfig             `yaml:"friends"`
		FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
		SystemTasks    SystemTasksConfig         `yaml:"system_tasks"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigFile{}, err
	}
	adminPublicKey, err := resolveAdminPublicKey(raw.AdminPublicKey)
	if err != nil {
		return ConfigFile{}, err
	}
	cfg := ConfigFile{
		Host:           raw.Host,
		PublicAPIPort:  raw.PublicAPIPort,
		NoiseUDPPort:   raw.NoiseUDPPort,
		ICEPort:        raw.ICEPort,
		ListenAddr:     raw.ListenAddr,
		CipherMode:     raw.CipherMode,
		AdminPublicKey: adminPublicKey,
		Storage:        raw.Storage,
		Stores:         raw.Stores,
		Friends:        raw.Friends,
		FriendGroups:   raw.FriendGroups,
		SystemTasks:    raw.SystemTasks,
	}
	return cfg, nil
}

func resolveAdminPublicKey(publicKey *giznet.PublicKey) (giznet.PublicKey, error) {
	if publicKey == nil {
		return giznet.PublicKey{}, nil
	}
	if publicKey.IsZero() {
		return giznet.PublicKey{}, fmt.Errorf("server: invalid admin-public-key: zero key")
	}
	return *publicKey, nil
}

func DefaultConfig() Config {
	return Config{
		Host:          "0.0.0.0",
		PublicAPIPort: 9820,
		NoiseUDPPort:  9820,
		ICEPort:       9821,
		ListenAddr:    ":9820",
	}
}

func mergeFileConfig(cfg Config, fileCfg ConfigFile) (Config, error) {
	if cfg.Host == "" {
		cfg.Host = fileCfg.Host
	}
	if cfg.PublicAPIPort == 0 {
		cfg.PublicAPIPort = fileCfg.PublicAPIPort
	}
	if cfg.NoiseUDPPort == 0 {
		cfg.NoiseUDPPort = fileCfg.NoiseUDPPort
	}
	if cfg.ICEPort == 0 {
		cfg.ICEPort = fileCfg.ICEPort
	}
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
	cfg.Friends = mergeFriendsConfig(cfg.Friends, fileCfg.Friends)
	cfg.FriendGroups = mergeFriendGroupsConfig(cfg.FriendGroups, fileCfg.FriendGroups)
	cfg.SystemTasks = mergeSystemTasksConfig(cfg.SystemTasks, fileCfg.SystemTasks)
	return cfg, nil
}

func mergeFriendsConfig(runtime FriendsConfig, file FriendsConfig) FriendsConfig {
	_ = file
	return runtime
}

func mergeFriendGroupsConfig(runtime FriendGroupsConfig, file FriendGroupsConfig) FriendGroupsConfig {
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
	if cfg.Host == "" {
		cfg.Host = defaults.Host
	}
	if cfg.PublicAPIPort == 0 {
		cfg.PublicAPIPort = defaults.PublicAPIPort
	}
	if cfg.NoiseUDPPort == 0 {
		cfg.NoiseUDPPort = defaults.NoiseUDPPort
	}
	if cfg.ICEPort == 0 {
		cfg.ICEPort = defaults.ICEPort
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
	if err := validatePort("public-api-port", cfg.PublicAPIPort); err != nil {
		return err
	}
	if err := validatePort("noise-udp-port", cfg.NoiseUDPPort); err != nil {
		return err
	}
	if err := validatePort("ice-port", cfg.ICEPort); err != nil {
		return err
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

func (cfg Config) PublicAPIListenAddr() string {
	if cfg.ListenAddr != "" {
		return cfg.ListenAddr
	}
	return net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.PublicAPIPort))
}

func (cfg Config) NoiseUDPListenAddr() string {
	if cfg.ListenAddr != "" {
		return cfg.ListenAddr
	}
	return net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.NoiseUDPPort))
}

func (cfg Config) ICEListenAddr() string {
	return net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.ICEPort))
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

func validateCipherMode(mode giznoise.CipherMode) error {
	switch mode {
	case "", giznoise.CipherModeChaChaPoly, giznoise.CipherModeAES256GCM, giznoise.CipherModePlaintext:
		return nil
	default:
		return fmt.Errorf("server: unsupported cipher-mode %q", mode)
	}
}

func validatePort(field string, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("server: %s must be between 1 and 65535", field)
	}
	return nil
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
