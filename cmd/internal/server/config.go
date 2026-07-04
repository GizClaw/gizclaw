package server

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

type Config struct {
	KeyPair        *giznet.KeyPair
	Listen         string
	Endpoint       string
	AdminPublicKey giznet.PublicKey
	Storage        map[string]storage.Config
	Stores         map[string]stores.Config
	Friends        FriendsConfig
	FriendGroups   FriendGroupsConfig
}

type FriendsConfig struct{}

type FriendGroupsConfig struct {
	MessageDefaultTTL      string `yaml:"message_default_ttl"`
	MessageMaxTTL          string `yaml:"message_max_ttl"`
	MessageCleanupInterval string `yaml:"message_cleanup_interval"`
	MessageMaxAudioBytes   int64  `yaml:"message_max_audio_bytes"`
}

type IdentityConfig struct {
	PrivateKey giznet.Key `yaml:"private-key"`
}

type ConfigFile struct {
	Identity       IdentityConfig            `yaml:"identity"`
	Listen         string                    `yaml:"listen"`
	Endpoint       string                    `yaml:"endpoint"`
	AdminPublicKey giznet.PublicKey          `yaml:"admin-public-key"`
	Storage        map[string]storage.Config `yaml:"storage"`
	Stores         map[string]stores.Config  `yaml:"stores"`
	Friends        FriendsConfig             `yaml:"friends"`
	FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
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
	defaultContactsStore                 = "contacts"
	defaultFriendInviteTokensStore       = "friend-invite-tokens"
	defaultFriendsStore                  = "friends"
	defaultFriendGroupsStore             = "friend-groups"
	defaultFriendGroupInviteTokensStore  = "friend-group-invite-tokens"
	defaultFriendGroupMembersStore       = "friend-group-members"
	defaultFriendGroupBelongsStore       = "friend-group-belongs"
	defaultFriendGroupMessagesStore      = "friend-group-messages"
	defaultFriendGroupMessageAssetsStore = "friend-group-message-assets"
	defaultGameRulesetsStore             = "game-rulesets"
	defaultPetDefsStore                  = "pet-defs"
	defaultBadgeDefsStore                = "badge-defs"
	defaultGameDefsStore                 = "game-defs"
	defaultGameplayAssetsStore           = "gameplay-assets"
	defaultGameplayDBStore               = "gameplay-db"
)

func LoadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	var raw struct {
		Identity       *IdentityConfig           `yaml:"identity"`
		Listen         string                    `yaml:"listen"`
		Endpoint       string                    `yaml:"endpoint"`
		AdminPublicKey *giznet.PublicKey         `yaml:"admin-public-key"`
		Storage        map[string]storage.Config `yaml:"storage"`
		Stores         map[string]stores.Config  `yaml:"stores"`
		Friends        FriendsConfig             `yaml:"friends"`
		FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigFile{}, err
	}
	adminPublicKey, err := resolveAdminPublicKey(raw.AdminPublicKey)
	if err != nil {
		return ConfigFile{}, err
	}
	var identity IdentityConfig
	if raw.Identity != nil {
		if raw.Identity.PrivateKey.IsZero() {
			return ConfigFile{}, fmt.Errorf("server: invalid identity.private-key: zero key")
		}
		keyPair, err := giznet.NewKeyPair(raw.Identity.PrivateKey)
		if err != nil {
			return ConfigFile{}, fmt.Errorf("server: invalid identity.private-key: %w", err)
		}
		identity = *raw.Identity
		identity.PrivateKey = keyPair.Private
	}
	cfg := ConfigFile{
		Identity:       identity,
		Listen:         raw.Listen,
		Endpoint:       raw.Endpoint,
		AdminPublicKey: adminPublicKey,
		Storage:        raw.Storage,
		Stores:         raw.Stores,
		Friends:        raw.Friends,
		FriendGroups:   raw.FriendGroups,
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
		Listen:   "0.0.0.0:9820",
		Endpoint: "0.0.0.0:9820",
	}
}

func mergeFileConfig(cfg Config, fileCfg ConfigFile) (Config, error) {
	if cfg.Listen == "" {
		cfg.Listen = fileCfg.Listen
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = fileCfg.Endpoint
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

func prepareConfig(cfg Config) (Config, error) {
	defaults := DefaultConfig()
	if cfg.Listen == "" {
		cfg.Listen = defaults.Listen
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = cfg.Listen
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
	if err := validateHostPort("listen", cfg.Listen); err != nil {
		return err
	}
	if err := validateHostPort("endpoint", cfg.Endpoint); err != nil {
		return err
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
	return cfg.Listen
}

func (cfg Config) ICEListenAddr() string {
	return cfg.Listen
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

func validateHostPort(field, value string) error {
	if strings.Contains(value, "://") {
		return fmt.Errorf("server: %s must be host:port, got %q", field, value)
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("server: invalid %s: %w", field, err)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("server: %s host is empty", field)
	}
	if strings.TrimSpace(port) == "" {
		return fmt.Errorf("server: %s port is empty", field)
	}
	return nil
}
