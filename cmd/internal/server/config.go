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
	KeyPair        *giznet.KeyPair
	Listen         string
	Endpoint       string
	ServeToClients bool
	EdgeNodes      []giznet.PublicKey
	ICEServers     []gizwebrtc.ICEServer
	AdminPublicKey giznet.PublicKey
	Storage        map[string]storage.Config
	Stores         map[string]stores.Config
	AgentHost      *AgentHostConfig
	SystemLog      logging.Config
	Friends        FriendsConfig
	FriendGroups   FriendGroupsConfig
	Speech         SpeechConfig
}

// AgentHostConfig binds AgentHost persistence capabilities to logical Stores.
// A non-nil value selects explicit mode, including when every field is omitted.
type AgentHostConfig struct {
	RuntimeStore string                    `yaml:"runtime_store"`
	Flowcraft    *AgentHostFlowcraftConfig `yaml:"flowcraft"`
}

// AgentHostFlowcraftConfig binds Flowcraft persistence capabilities to logical
// Stores owned by the command-layer Store Registry.
type AgentHostFlowcraftConfig struct {
	StateStore         string `yaml:"state_store"`
	HistoryStore       string `yaml:"history_store"`
	MemoryObjectsStore string `yaml:"memory_objects_store"`
}

type FriendsConfig struct{}

type FriendGroupsConfig struct {
	MessageDefaultTTL      string `yaml:"message_default_ttl"`
	MessageMaxTTL          string `yaml:"message_max_ttl"`
	MessageCleanupInterval string `yaml:"message_cleanup_interval"`
	MessageMaxAudioBytes   int64  `yaml:"message_max_audio_bytes"`
}

type SpeechConfig struct {
	Transcription SpeechTranscriptionConfig `yaml:"transcription"`
	Synthesis     SpeechSynthesisConfig     `yaml:"synthesis"`
}

type SpeechTranscriptionConfig struct {
	MaxAudioBytes    int64  `yaml:"max_audio_bytes"`
	MaxAudioDuration string `yaml:"max_audio_duration"`
	RequestTimeout   string `yaml:"request_timeout"`
}

type SpeechSynthesisConfig struct {
	MaxTextBytes   int64  `yaml:"max_text_bytes"`
	MaxOutputBytes int64  `yaml:"max_output_bytes"`
	RequestTimeout string `yaml:"request_timeout"`
}

type speechFileConfig struct {
	Transcription struct {
		MaxAudioBytes    *int64  `yaml:"max_audio_bytes"`
		MaxAudioDuration *string `yaml:"max_audio_duration"`
		RequestTimeout   *string `yaml:"request_timeout"`
	} `yaml:"transcription"`
	Synthesis struct {
		MaxTextBytes   *int64  `yaml:"max_text_bytes"`
		MaxOutputBytes *int64  `yaml:"max_output_bytes"`
		RequestTimeout *string `yaml:"request_timeout"`
	} `yaml:"synthesis"`
}

type IdentityConfig struct {
	PrivateKey giznet.Key `yaml:"private-key"`
}

type ConfigFile struct {
	Identity       IdentityConfig            `yaml:"identity"`
	Listen         string                    `yaml:"listen"`
	Endpoint       string                    `yaml:"endpoint"`
	ServeToClients bool                      `yaml:"serve-to-clients"`
	EdgeNodes      []giznet.PublicKey        `yaml:"edge-nodes"`
	ICEServers     []gizwebrtc.ICEServer     `yaml:"ice-servers"`
	AdminPublicKey giznet.PublicKey          `yaml:"admin-public-key"`
	Storage        map[string]storage.Config `yaml:"storage"`
	Stores         map[string]stores.Config  `yaml:"stores"`
	AgentHost      *AgentHostConfig          `yaml:"agent_host"`
	SystemLog      logging.Config            `yaml:"system_log"`
	Friends        FriendsConfig             `yaml:"friends"`
	FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
	Speech         SpeechConfig              `yaml:"speech"`
}

const (
	defaultPeersStore                    = "peers"
	defaultCredentialsStore              = "credentials"
	defaultFirmwaresStore                = "firmwares"
	defaultFirmwareAssetsStore           = "firmware-assets"
	defaultRuntimeProfilesStore          = "runtime-profiles"
	defaultAgentHostStore                = "agenthost"
	defaultMiniMaxTenantsStore           = "minimax-tenants"
	defaultDeepSeekTenantsStore          = "deepseek-tenants"
	defaultVoicesStore                   = "voices"
	defaultWorkspacesStore               = "workspaces"
	defaultWorkflowsStore                = "workflows"
	defaultContactsStore                 = "contacts"
	defaultFriendInviteTokensStore       = "friend-invite-tokens"
	defaultFriendsStore                  = "friends"
	defaultFriendGroupsStore             = "friend-groups"
	defaultFriendGroupInviteTokensStore  = "friend-group-invite-tokens"
	defaultFriendGroupMembersStore       = "friend-group-members"
	defaultFriendGroupBelongsStore       = "friend-group-belongs"
	defaultFriendGroupMessagesStore      = "friend-group-messages"
	defaultFriendGroupMessageAssetsStore = "friend-group-message-assets"
	defaultPetDefsStore                  = "pet-defs"
	defaultBadgeDefsStore                = "badge-defs"
	defaultGameDefsStore                 = "game-defs"
	defaultGameplayAssetsStore           = "gameplay-assets"
	defaultWorkspaceAssetsStore          = "workspace-assets"
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
	if _, exists := topLevel["system_tasks"]; exists {
		return ConfigFile{}, fmt.Errorf("server: system_tasks is not supported; configure Pet model aliases in the RuntimeProfile")
	}
	var raw struct {
		Identity       *IdentityConfig           `yaml:"identity"`
		Listen         string                    `yaml:"listen"`
		Endpoint       string                    `yaml:"endpoint"`
		ServeToClients *bool                     `yaml:"serve-to-clients"`
		EdgeNodes      []giznet.PublicKey        `yaml:"edge-nodes"`
		ICEServers     []gizwebrtc.ICEServer     `yaml:"ice-servers"`
		AdminPublicKey *giznet.PublicKey         `yaml:"admin-public-key"`
		Storage        map[string]storage.Config `yaml:"storage"`
		Stores         map[string]stores.Config  `yaml:"stores"`
		AgentHost      *AgentHostConfig          `yaml:"agent_host"`
		SystemLog      logging.Config            `yaml:"system_log"`
		Friends        FriendsConfig             `yaml:"friends"`
		FriendGroups   FriendGroupsConfig        `yaml:"friend_groups"`
		Speech         speechFileConfig          `yaml:"speech"`
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
	speech, err := raw.Speech.runtimeConfig()
	if err != nil {
		return ConfigFile{}, err
	}
	cfg := ConfigFile{
		Identity:       identity,
		Listen:         raw.Listen,
		Endpoint:       raw.Endpoint,
		ServeToClients: serveToClients,
		EdgeNodes:      raw.EdgeNodes,
		ICEServers:     raw.ICEServers,
		AdminPublicKey: adminPublicKey,
		Storage:        raw.Storage,
		Stores:         raw.Stores,
		AgentHost:      raw.AgentHost,
		SystemLog:      logCfg,
		Friends:        raw.Friends,
		FriendGroups:   raw.FriendGroups,
		Speech:         speech,
	}
	return cfg, nil
}

func (cfg speechFileConfig) runtimeConfig() (SpeechConfig, error) {
	var out SpeechConfig
	if value := cfg.Transcription.MaxAudioBytes; value != nil {
		if *value <= 0 {
			return SpeechConfig{}, fmt.Errorf("server: speech.transcription.max_audio_bytes must be > 0")
		}
		out.Transcription.MaxAudioBytes = *value
	}
	if value := cfg.Transcription.MaxAudioDuration; value != nil {
		if _, err := parsePositiveConfigDuration(*value); err != nil {
			return SpeechConfig{}, fmt.Errorf("server: speech.transcription.max_audio_duration: %w", err)
		}
		out.Transcription.MaxAudioDuration = *value
	}
	if value := cfg.Transcription.RequestTimeout; value != nil {
		if _, err := parsePositiveConfigDuration(*value); err != nil {
			return SpeechConfig{}, fmt.Errorf("server: speech.transcription.request_timeout: %w", err)
		}
		out.Transcription.RequestTimeout = *value
	}
	if value := cfg.Synthesis.MaxTextBytes; value != nil {
		if *value <= 0 {
			return SpeechConfig{}, fmt.Errorf("server: speech.synthesis.max_text_bytes must be > 0")
		}
		out.Synthesis.MaxTextBytes = *value
	}
	if value := cfg.Synthesis.MaxOutputBytes; value != nil {
		if *value <= 0 {
			return SpeechConfig{}, fmt.Errorf("server: speech.synthesis.max_output_bytes must be > 0")
		}
		out.Synthesis.MaxOutputBytes = *value
	}
	if value := cfg.Synthesis.RequestTimeout; value != nil {
		if _, err := parsePositiveConfigDuration(*value); err != nil {
			return SpeechConfig{}, fmt.Errorf("server: speech.synthesis.request_timeout: %w", err)
		}
		out.Synthesis.RequestTimeout = *value
	}
	return out, nil
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
		Speech: SpeechConfig{
			Transcription: SpeechTranscriptionConfig{MaxAudioBytes: 2097152, MaxAudioDuration: "60s", RequestTimeout: "75s"},
			Synthesis:     SpeechSynthesisConfig{MaxTextBytes: 4096, MaxOutputBytes: 4194304, RequestTimeout: "120s"},
		},
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
	if cfg.AgentHost == nil {
		cfg.AgentHost = fileCfg.AgentHost
	}
	if cfg.SystemLog.IsZero() {
		cfg.SystemLog = fileCfg.SystemLog
	}
	cfg.Friends = mergeFriendsConfig(cfg.Friends, fileCfg.Friends)
	cfg.FriendGroups = mergeFriendGroupsConfig(cfg.FriendGroups, fileCfg.FriendGroups)
	cfg.Speech = mergeSpeechConfig(cfg.Speech, fileCfg.Speech)
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

func mergeSpeechConfig(runtime SpeechConfig, file SpeechConfig) SpeechConfig {
	if runtime.Transcription.MaxAudioBytes == 0 {
		runtime.Transcription.MaxAudioBytes = file.Transcription.MaxAudioBytes
	}
	if runtime.Transcription.MaxAudioDuration == "" {
		runtime.Transcription.MaxAudioDuration = file.Transcription.MaxAudioDuration
	}
	if runtime.Transcription.RequestTimeout == "" {
		runtime.Transcription.RequestTimeout = file.Transcription.RequestTimeout
	}
	if runtime.Synthesis.MaxTextBytes == 0 {
		runtime.Synthesis.MaxTextBytes = file.Synthesis.MaxTextBytes
	}
	if runtime.Synthesis.MaxOutputBytes == 0 {
		runtime.Synthesis.MaxOutputBytes = file.Synthesis.MaxOutputBytes
	}
	if runtime.Synthesis.RequestTimeout == "" {
		runtime.Synthesis.RequestTimeout = file.Synthesis.RequestTimeout
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
	cfg.Speech = mergeSpeechConfig(cfg.Speech, defaults.Speech)
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
	if cfg.Speech.Transcription.MaxAudioBytes <= 0 {
		return fmt.Errorf("server: speech.transcription.max_audio_bytes must be > 0")
	}
	if _, err := parsePositiveConfigDuration(cfg.Speech.Transcription.MaxAudioDuration); err != nil {
		return fmt.Errorf("server: speech.transcription.max_audio_duration: %w", err)
	}
	if _, err := parsePositiveConfigDuration(cfg.Speech.Transcription.RequestTimeout); err != nil {
		return fmt.Errorf("server: speech.transcription.request_timeout: %w", err)
	}
	if cfg.Speech.Synthesis.MaxTextBytes <= 0 {
		return fmt.Errorf("server: speech.synthesis.max_text_bytes must be > 0")
	}
	if cfg.Speech.Synthesis.MaxOutputBytes <= 0 {
		return fmt.Errorf("server: speech.synthesis.max_output_bytes must be > 0")
	}
	if _, err := parsePositiveConfigDuration(cfg.Speech.Synthesis.RequestTimeout); err != nil {
		return fmt.Errorf("server: speech.synthesis.request_timeout: %w", err)
	}
	if err := validateAgentHostConfig(cfg.AgentHost); err != nil {
		return err
	}
	return nil
}

func validateAgentHostConfig(cfg *AgentHostConfig) error {
	if cfg == nil {
		return nil
	}
	if err := validateStoreReference("agent_host.runtime_store", cfg.RuntimeStore); err != nil {
		return err
	}
	if cfg.Flowcraft == nil {
		return nil
	}
	if err := validateStoreReference("agent_host.flowcraft.state_store", cfg.Flowcraft.StateStore); err != nil {
		return err
	}
	if err := validateStoreReference("agent_host.flowcraft.history_store", cfg.Flowcraft.HistoryStore); err != nil {
		return err
	}
	return validateStoreReference("agent_host.flowcraft.memory_objects_store", cfg.Flowcraft.MemoryObjectsStore)
}

func validateStoreReference(path, value string) error {
	if value != "" && strings.TrimSpace(value) == "" {
		return fmt.Errorf("server: %s must not be whitespace-only", path)
	}
	return nil
}

func parsePositiveConfigDuration(value string) (time.Duration, error) {
	duration, err := parseConfigDuration(value)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("must be > 0")
	}
	return duration, nil
}

func validateConfigShape(data []byte) error {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if _, legacy := document["log"]; legacy {
		return fmt.Errorf("server: top-level log configuration was removed; configure stores and system_log instead")
	}
	if agentHostValue, exists := document["agent_host"]; exists {
		if err := validateAgentHostConfigShape(agentHostValue); err != nil {
			return err
		}
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

func validateAgentHostConfigShape(value any) error {
	agentHost, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("server: agent_host must be a mapping")
	}
	for field := range agentHost {
		switch field {
		case "runtime_store", "flowcraft":
		default:
			return fmt.Errorf("server: agent_host has unknown field %q", field)
		}
	}
	if runtimeStore, exists := agentHost["runtime_store"]; exists {
		if err := validateFileStoreReference("agent_host.runtime_store", runtimeStore); err != nil {
			return err
		}
	}
	flowcraftValue, exists := agentHost["flowcraft"]
	if !exists {
		return nil
	}
	flowcraft, ok := flowcraftValue.(map[string]any)
	if !ok {
		return fmt.Errorf("server: agent_host.flowcraft must be a mapping")
	}
	for field := range flowcraft {
		switch field {
		case "state_store", "history_store", "memory_objects_store":
		default:
			return fmt.Errorf("server: agent_host.flowcraft has unknown field %q", field)
		}
	}
	for _, field := range []string{"state_store", "history_store", "memory_objects_store"} {
		if reference, exists := flowcraft[field]; exists {
			if err := validateFileStoreReference("agent_host.flowcraft."+field, reference); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFileStoreReference(path string, value any) error {
	reference, ok := value.(string)
	if !ok {
		return fmt.Errorf("server: %s must be a string", path)
	}
	if strings.TrimSpace(reference) == "" {
		return fmt.Errorf("server: %s must not be empty", path)
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
			"dir", "extraction_model", "embedding_model", "rerank_model", "extraction_mode", "system_prompt",
			"schema_name", "temperature", "stage_timeout", "graph_enabled", "async", "bbh",
		}); err != nil {
			return err
		}
		flowcraft := value.(map[string]any)
		if async, exists := flowcraft["async"]; exists {
			if err := validateConfigMappingFields(path+".async", async, []string{"enabled"}); err != nil {
				return err
			}
		}
		if bbh, exists := flowcraft["bbh"]; exists {
			if err := validateBBHConfigShape(path+".bbh", bbh); err != nil {
				return err
			}
		}
	}
	if value, exists := mapping["mem0"]; exists {
		if err := validateConfigMappingFields("server: stores."+name+".mem0", value, []string{
			"endpoint", "api_key", "flavor", "poll_interval",
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
				"endpoint", "api_key", "flavor", "poll_interval",
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBBHConfigShape(path string, value any) error {
	if err := validateConfigMappingFields(path, value, []string{"search_overfetch", "bleve", "hnsw"}); err != nil {
		return err
	}
	mapping := value.(map[string]any)
	if bleve, exists := mapping["bleve"]; exists {
		if err := validateConfigMappingFields(path+".bleve", bleve, []string{"analyzer", "gojieba"}); err != nil {
			return err
		}
		bleveMapping := bleve.(map[string]any)
		if gojieba, exists := bleveMapping["gojieba"]; exists {
			if err := validateConfigMappingFields(path+".bleve.gojieba", gojieba, []string{
				"mode", "hmm", "dict_path", "hmm_path", "user_dict_path", "idf_path", "stop_words_path",
			}); err != nil {
				return err
			}
		}
	}
	if hnsw, exists := mapping["hnsw"]; exists {
		return validateConfigMappingFields(path+".hnsw", hnsw, []string{"flush_interval"})
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
