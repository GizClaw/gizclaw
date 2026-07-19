package server

import (
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging"
	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/goccy/go-yaml"
)

type Config struct {
	KeyPair         *giznet.KeyPair
	Listen          string
	Endpoint        string
	ServeToClients  bool
	EdgeNodes       []giznet.PublicKey
	ICEServers      []gizwebrtc.ICEServer
	AdminPublicKey  giznet.PublicKey
	DefaultPeerView string
	Storage         map[string]storage.Config
	Stores          map[string]stores.Config
	SystemLog       logging.Config
	Friends         FriendsConfig
	FriendGroups    FriendGroupsConfig
	SystemTasks     SystemTasksConfig
}

type FriendsConfig struct{}

type FriendGroupsConfig struct {
	MessageDefaultTTL      string `yaml:"message_default_ttl"`
	MessageMaxTTL          string `yaml:"message_max_ttl"`
	MessageCleanupInterval string `yaml:"message_cleanup_interval"`
	MessageMaxAudioBytes   int64  `yaml:"message_max_audio_bytes"`
}

type SystemTasksConfig struct {
	PetFlowcraftWorkflow PetFlowcraftWorkflowTaskConfig `yaml:"pet_flowcraft_workflow"`
}

type PetFlowcraftWorkflowTaskConfig struct {
	GenerateModel  string `yaml:"generate_model"`
	ExtractModel   string `yaml:"extract_model"`
	EmbeddingModel string `yaml:"embedding_model"`
	ASRModel       string `yaml:"asr_model"`
}

type IdentityConfig struct {
	PrivateKey giznet.Key `yaml:"private-key"`
}

type ConfigFile struct {
	Identity        IdentityConfig            `yaml:"identity"`
	Listen          string                    `yaml:"listen"`
	Endpoint        string                    `yaml:"endpoint"`
	ServeToClients  bool                      `yaml:"serve-to-clients"`
	EdgeNodes       []giznet.PublicKey        `yaml:"edge-nodes"`
	ICEServers      []gizwebrtc.ICEServer     `yaml:"ice-servers"`
	AdminPublicKey  giznet.PublicKey          `yaml:"admin-public-key"`
	DefaultPeerView string                    `yaml:"default-peer-view"`
	Storage         map[string]storage.Config `yaml:"storage"`
	Stores          map[string]stores.Config  `yaml:"stores"`
	SystemLog       logging.Config            `yaml:"system_log"`
	Friends         FriendsConfig             `yaml:"friends"`
	FriendGroups    FriendGroupsConfig        `yaml:"friend_groups"`
	SystemTasks     SystemTasksConfig         `yaml:"system_tasks"`
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
	defaultWorkspaceAssetsStore          = "workspace-assets"
	defaultWorkflowAssetsStore           = "workflow-assets"
	defaultGameplayDBStore               = "gameplay-db"
	defaultMetricsStore                  = "metrics"
	defaultFlowcraftHistoryStore         = "flowcraft-history"
)

func LoadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	return parseConfigData(data)
}

func parseConfigData(data []byte) (ConfigFile, error) {
	if err := validateConfigShape(data); err != nil {
		return ConfigFile{}, err
	}
	var topLevel map[string]any
	if err := yaml.Unmarshal(data, &topLevel); err != nil {
		return ConfigFile{}, err
	}
	if _, exists := topLevel["serving-public"]; exists {
		return ConfigFile{}, fmt.Errorf("server: serving-public is not supported; use serve-to-clients")
	}
	var raw struct {
		Identity        *IdentityConfig           `yaml:"identity"`
		Listen          string                    `yaml:"listen"`
		Endpoint        string                    `yaml:"endpoint"`
		ServeToClients  *bool                     `yaml:"serve-to-clients"`
		EdgeNodes       []giznet.PublicKey        `yaml:"edge-nodes"`
		ICEServers      []gizwebrtc.ICEServer     `yaml:"ice-servers"`
		AdminPublicKey  *giznet.PublicKey         `yaml:"admin-public-key"`
		DefaultPeerView string                    `yaml:"default-peer-view"`
		Storage         map[string]storage.Config `yaml:"storage"`
		Stores          map[string]stores.Config  `yaml:"stores"`
		SystemLog       logging.Config            `yaml:"system_log"`
		Friends         FriendsConfig             `yaml:"friends"`
		FriendGroups    FriendGroupsConfig        `yaml:"friend_groups"`
		SystemTasks     SystemTasksConfig         `yaml:"system_tasks"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigFile{}, err
	}
	adminPublicKey, err := resolveAdminPublicKey(raw.AdminPublicKey)
	if err != nil {
		return ConfigFile{}, err
	}
	logCfg, err := logging.PrepareConfig(raw.SystemLog)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("server: %w", err)
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
	serveToClients := raw.ServeToClients != nil && *raw.ServeToClients
	cfg := ConfigFile{
		Identity:        identity,
		Listen:          raw.Listen,
		Endpoint:        raw.Endpoint,
		ServeToClients:  serveToClients,
		EdgeNodes:       raw.EdgeNodes,
		ICEServers:      raw.ICEServers,
		AdminPublicKey:  adminPublicKey,
		DefaultPeerView: raw.DefaultPeerView,
		Storage:         raw.Storage,
		Stores:          raw.Stores,
		SystemLog:       logCfg,
		Friends:         raw.Friends,
		FriendGroups:    raw.FriendGroups,
		SystemTasks:     raw.SystemTasks,
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
		Listen:    "0.0.0.0:9820",
		Endpoint:  "0.0.0.0:9820",
		SystemLog: logging.DefaultConfig(),
	}
}

func mergeFileConfig(cfg Config, fileCfg ConfigFile) (Config, error) {
	if cfg.Listen == "" {
		cfg.Listen = fileCfg.Listen
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = fileCfg.Endpoint
	}
	if !cfg.ServeToClients {
		cfg.ServeToClients = fileCfg.ServeToClients
	}
	if len(cfg.EdgeNodes) == 0 {
		cfg.EdgeNodes = fileCfg.EdgeNodes
	}
	if cfg.AdminPublicKey.IsZero() {
		cfg.AdminPublicKey = fileCfg.AdminPublicKey
	}
	if cfg.DefaultPeerView == "" {
		cfg.DefaultPeerView = fileCfg.DefaultPeerView
	}
	if len(cfg.ICEServers) == 0 {
		cfg.ICEServers = fileCfg.ICEServers
	}
	if len(cfg.EdgeNodes) == 0 {
		cfg.EdgeNodes = fileCfg.EdgeNodes
	}
	if len(cfg.Stores) == 0 {
		cfg.Stores = fileCfg.Stores
	}
	if len(cfg.Storage) == 0 {
		cfg.Storage = fileCfg.Storage
	}
	if cfg.SystemLog.IsZero() {
		cfg.SystemLog = fileCfg.SystemLog
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
	runtime.PetFlowcraftWorkflow = mergePetFlowcraftWorkflowTaskConfig(runtime.PetFlowcraftWorkflow, file.PetFlowcraftWorkflow)
	return runtime
}

func mergePetFlowcraftWorkflowTaskConfig(runtime PetFlowcraftWorkflowTaskConfig, file PetFlowcraftWorkflowTaskConfig) PetFlowcraftWorkflowTaskConfig {
	if runtime.GenerateModel == "" {
		runtime.GenerateModel = file.GenerateModel
	}
	if runtime.ExtractModel == "" {
		runtime.ExtractModel = file.ExtractModel
	}
	if runtime.EmbeddingModel == "" {
		runtime.EmbeddingModel = file.EmbeddingModel
	}
	if runtime.ASRModel == "" {
		runtime.ASRModel = file.ASRModel
	}
	return runtime
}

func prepareConfig(cfg Config) (Config, error) {
	defaults := DefaultConfig()
	cfg.DefaultPeerView = strings.TrimSpace(cfg.DefaultPeerView)
	if cfg.Listen == "" {
		cfg.Listen = defaults.Listen
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = cfg.Listen
	}
	logCfg, err := logging.PrepareConfig(cfg.SystemLog)
	if err != nil {
		return Config{}, fmt.Errorf("server: %w", err)
	}
	cfg.SystemLog = logCfg
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
	if _, err := logging.PrepareConfig(cfg.SystemLog); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	for i, publicKey := range cfg.EdgeNodes {
		if publicKey.IsZero() {
			return fmt.Errorf("server: edge-nodes[%d] is zero", i)
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

func validateConfigShape(data []byte) error {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if _, legacy := document["log"]; legacy {
		return fmt.Errorf("server: top-level log configuration was removed; configure stores and system_log instead")
	}
	if systemLogValue, exists := document["system_log"]; exists {
		mapping, ok := systemLogValue.(map[string]any)
		if !ok {
			return fmt.Errorf("server: system_log must be a mapping")
		}
		for field := range mapping {
			switch field {
			case "level", "query_store", "sinks":
			default:
				return fmt.Errorf("server: system_log has unknown field %q", field)
			}
		}
		if sinksValue, exists := mapping["sinks"]; exists {
			sinks, ok := sinksValue.([]any)
			if !ok {
				return fmt.Errorf("server: system_log.sinks must be a sequence")
			}
			for index, sinkValue := range sinks {
				sink, ok := sinkValue.(map[string]any)
				if !ok {
					return fmt.Errorf("server: system_log.sinks[%d] must be a mapping", index)
				}
				for field := range sink {
					switch field {
					case "kind", "store", "level":
					default:
						return fmt.Errorf("server: system_log.sinks[%d] has unknown field %q", index, field)
					}
				}
			}
		}
	}
	storesValue, exists := document["stores"]
	if !exists || storesValue == nil {
		return nil
	}
	storeMappings, ok := storesValue.(map[string]any)
	if !ok {
		return nil
	}
	for name, value := range storeMappings {
		mapping, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if fmt.Sprint(mapping["kind"]) == stores.KindMemoryStore {
			if err := validateMemoryStoreConfigShape(name, mapping); err != nil {
				return err
			}
			continue
		}
		if fmt.Sprint(mapping["kind"]) != stores.KindLog {
			continue
		}
		for field := range mapping {
			if field != "kind" && field != "volc" && field != "clickhouse" {
				return fmt.Errorf("server: stores.%s field %q is invalid for kind log", name, field)
			}
		}
		if volcValue, exists := mapping["volc"]; exists {
			volcMapping, ok := volcValue.(map[string]any)
			if !ok {
				return fmt.Errorf("server: stores.%s.volc must be a mapping", name)
			}
			for field := range volcMapping {
				switch field {
				case "endpoint", "region", "topic_id", "access_key_id", "access_key_secret":
				default:
					return fmt.Errorf("server: stores.%s.volc has unknown field %q", name, field)
				}
			}
		}
		clickhouseValue, exists := mapping["clickhouse"]
		if !exists {
			continue
		}
		clickhouseMapping, ok := clickhouseValue.(map[string]any)
		if !ok {
			return fmt.Errorf("server: stores.%s.clickhouse must be a mapping", name)
		}
		for field := range clickhouseMapping {
			switch field {
			case "dsn", "database", "table":
			default:
				return fmt.Errorf("server: stores.%s.clickhouse has unknown field %q", name, field)
			}
		}
	}
	return nil
}

func validateMemoryStoreConfigShape(name string, mapping map[string]any) error {
	for field := range mapping {
		switch field {
		case "kind", "flowcraft", "mem0", "volc_memory":
		default:
			return fmt.Errorf("server: stores.%s field %q is invalid for kind memory", name, field)
		}
	}
	if value, exists := mapping["flowcraft"]; exists {
		path := "server: stores." + name + ".flowcraft"
		if err := validateConfigMappingFields(path, value, []string{
			"dir", "runtime_id", "agent_id", "user_id", "extraction_model", "embedding_model", "rerank_model",
			"extraction_mode", "system_prompt", "schema_name", "temperature", "stage_timeout", "async",
		}); err != nil {
			return err
		}
		flowcraft := value.(map[string]any)
		if async, exists := flowcraft["async"]; exists {
			if err := validateConfigMappingFields(path+".async", async, []string{"enabled", "worker_id"}); err != nil {
				return err
			}
		}
	}
	if value, exists := mapping["mem0"]; exists {
		if err := validateConfigMappingFields("server: stores."+name+".mem0", value, []string{
			"endpoint", "api_key", "flavor", "app_id", "user_id", "agent_id", "run_id", "poll_interval",
		}); err != nil {
			return err
		}
	}
	if value, exists := mapping["volc_memory"]; exists {
		path := "server: stores." + name + ".volc_memory"
		if err := validateConfigMappingFields(path, value, []string{
			"mem0", "api_key_id", "memory_project_id", "control_endpoint", "region", "access_key_id", "access_key_secret",
		}); err != nil {
			return err
		}
		volc := value.(map[string]any)
		if mem0, exists := volc["mem0"]; exists {
			if err := validateConfigMappingFields(path+".mem0", mem0, []string{
				"endpoint", "api_key", "flavor", "app_id", "user_id", "agent_id", "run_id", "poll_interval",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateConfigMappingFields(path string, value any, allowed []string) error {
	mapping, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be a mapping", path)
	}
	for field := range mapping {
		if !slices.Contains(allowed, field) {
			return fmt.Errorf("%s has unknown field %q", path, field)
		}
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
