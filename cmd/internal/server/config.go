package server

import (
	"fmt"
	"net"
	"os"
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
	WorkspaceRoot  string `yaml:"-"`
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
	StateStore   string `yaml:"state_store"`
	HistoryStore string `yaml:"history_store"`
}

type FriendsConfig struct{}

type FriendGroupsConfig struct{}

type SpeechConfig struct {
	Transcription SpeechTranscriptionConfig `yaml:"transcription"`
	Extraction    SpeechExtractionConfig    `yaml:"extraction"`
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

type SpeechExtractionConfig struct {
	MaxSchemaBytes      int64  `yaml:"max_schema_bytes"`
	MaxSchemaDepth      int64  `yaml:"max_schema_depth"`
	MaxSchemaProperties int64  `yaml:"max_schema_properties"`
	MaxInstructionBytes int64  `yaml:"max_instruction_bytes"`
	MaxResultBytes      int64  `yaml:"max_result_bytes"`
	RequestTimeout      string `yaml:"request_timeout"`
}

type speechFileConfig struct {
	Transcription struct {
		MaxAudioBytes    *int64  `yaml:"max_audio_bytes"`
		MaxAudioDuration *string `yaml:"max_audio_duration"`
		RequestTimeout   *string `yaml:"request_timeout"`
	} `yaml:"transcription"`
	Extraction struct {
		MaxSchemaBytes      *int64  `yaml:"max_schema_bytes"`
		MaxSchemaDepth      *int64  `yaml:"max_schema_depth"`
		MaxSchemaProperties *int64  `yaml:"max_schema_properties"`
		MaxInstructionBytes *int64  `yaml:"max_instruction_bytes"`
		MaxResultBytes      *int64  `yaml:"max_result_bytes"`
		RequestTimeout      *string `yaml:"request_timeout"`
	} `yaml:"extraction"`
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
	defaultPeersStore                   = "peers"
	defaultCredentialsStore             = "credentials"
	defaultFirmwaresStore               = "firmwares"
	defaultFirmwareAssetsStore          = "firmware-assets"
	defaultRuntimeProfilesStore         = "runtime-profiles"
	defaultMemoryLayoutsStore           = "memory-layouts"
	defaultMiniMaxTenantsStore          = "minimax-tenants"
	defaultDeepSeekTenantsStore         = "deepseek-tenants"
	defaultVoicesStore                  = "voices"
	defaultWorkspacesStore              = "workspaces"
	defaultWorkflowsStore               = "workflows"
	defaultContactsStore                = "contacts"
	defaultFriendInviteTokensStore      = "friend-invite-tokens"
	defaultFriendsStore                 = "friends"
	defaultFriendGroupsStore            = "friend-groups"
	defaultFriendGroupInviteTokensStore = "friend-group-invite-tokens"
	defaultFriendGroupMembersStore      = "friend-group-members"
	defaultFriendGroupBelongsStore      = "friend-group-belongs"
	defaultPetDefsStore                 = "pet-defs"
	defaultBadgeDefsStore               = "badge-defs"
	defaultGameDefsStore                = "game-defs"
	defaultGameplayAssetsStore          = "gameplay-assets"
	defaultWorkspaceAssetsStore         = "workspace-assets"
	defaultGameplayDBStore              = "gameplay-db"
	defaultMetricsStore                 = "metrics"
	maxSpeechExtractionRequestTimeout   = 120 * time.Second
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
	if friendGroups, ok := topLevel["friend_groups"].(map[string]any); ok {
		for _, retired := range []string{"message_default_ttl", "message_max_ttl", "message_cleanup_interval", "message_max_audio_bytes"} {
			if _, exists := friendGroups[retired]; exists {
				return ConfigFile{}, fmt.Errorf("server: friend_groups.%s is not supported; Friend Group messages use Workspace History retention", retired)
			}
		}
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
	if value := cfg.Extraction.MaxSchemaBytes; value != nil {
		if *value <= 0 || *value > 16384 {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.max_schema_bytes must be between 1 and 16384")
		}
		out.Extraction.MaxSchemaBytes = *value
	}
	if value := cfg.Extraction.MaxSchemaDepth; value != nil {
		if *value <= 0 || *value > 16 {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.max_schema_depth must be between 1 and 16")
		}
		out.Extraction.MaxSchemaDepth = *value
	}
	if value := cfg.Extraction.MaxSchemaProperties; value != nil {
		if *value <= 0 || *value > 128 {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.max_schema_properties must be between 1 and 128")
		}
		out.Extraction.MaxSchemaProperties = *value
	}
	if value := cfg.Extraction.MaxInstructionBytes; value != nil {
		if *value <= 0 || *value > 4096 {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.max_instruction_bytes must be between 1 and 4096")
		}
		out.Extraction.MaxInstructionBytes = *value
	}
	if value := cfg.Extraction.MaxResultBytes; value != nil {
		if *value <= 0 || *value > 16384 {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.max_result_bytes must be between 1 and 16384")
		}
		out.Extraction.MaxResultBytes = *value
	}
	if value := cfg.Extraction.RequestTimeout; value != nil {
		timeout, err := parsePositiveConfigDuration(*value)
		if err != nil {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.request_timeout: %w", err)
		}
		if timeout > maxSpeechExtractionRequestTimeout {
			return SpeechConfig{}, fmt.Errorf("server: speech.extraction.request_timeout must be at most 2m0s")
		}
		out.Extraction.RequestTimeout = *value
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
			Extraction: SpeechExtractionConfig{
				MaxSchemaBytes: 16384, MaxSchemaDepth: 16, MaxSchemaProperties: 128,
				MaxInstructionBytes: 4096, MaxResultBytes: 16384, RequestTimeout: "120s",
			},
			Synthesis: SpeechSynthesisConfig{MaxTextBytes: 4096, MaxOutputBytes: 4194304, RequestTimeout: "120s"},
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
	_ = file
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
	if runtime.Extraction.MaxSchemaBytes == 0 {
		runtime.Extraction.MaxSchemaBytes = file.Extraction.MaxSchemaBytes
	}
	if runtime.Extraction.MaxSchemaDepth == 0 {
		runtime.Extraction.MaxSchemaDepth = file.Extraction.MaxSchemaDepth
	}
	if runtime.Extraction.MaxSchemaProperties == 0 {
		runtime.Extraction.MaxSchemaProperties = file.Extraction.MaxSchemaProperties
	}
	if runtime.Extraction.MaxInstructionBytes == 0 {
		runtime.Extraction.MaxInstructionBytes = file.Extraction.MaxInstructionBytes
	}
	if runtime.Extraction.MaxResultBytes == 0 {
		runtime.Extraction.MaxResultBytes = file.Extraction.MaxResultBytes
	}
	if runtime.Extraction.RequestTimeout == "" {
		runtime.Extraction.RequestTimeout = file.Extraction.RequestTimeout
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
	if cfg.Speech.Transcription.MaxAudioBytes <= 0 {
		return fmt.Errorf("server: speech.transcription.max_audio_bytes must be > 0")
	}
	if _, err := parsePositiveConfigDuration(cfg.Speech.Transcription.MaxAudioDuration); err != nil {
		return fmt.Errorf("server: speech.transcription.max_audio_duration: %w", err)
	}
	if _, err := parsePositiveConfigDuration(cfg.Speech.Transcription.RequestTimeout); err != nil {
		return fmt.Errorf("server: speech.transcription.request_timeout: %w", err)
	}
	if cfg.Speech.Extraction.MaxSchemaBytes <= 0 || cfg.Speech.Extraction.MaxSchemaBytes > 16384 {
		return fmt.Errorf("server: speech.extraction.max_schema_bytes must be between 1 and 16384")
	}
	if cfg.Speech.Extraction.MaxSchemaDepth <= 0 || cfg.Speech.Extraction.MaxSchemaDepth > 16 {
		return fmt.Errorf("server: speech.extraction.max_schema_depth must be between 1 and 16")
	}
	if cfg.Speech.Extraction.MaxSchemaProperties <= 0 || cfg.Speech.Extraction.MaxSchemaProperties > 128 {
		return fmt.Errorf("server: speech.extraction.max_schema_properties must be between 1 and 128")
	}
	if cfg.Speech.Extraction.MaxInstructionBytes <= 0 || cfg.Speech.Extraction.MaxInstructionBytes > 4096 {
		return fmt.Errorf("server: speech.extraction.max_instruction_bytes must be between 1 and 4096")
	}
	if cfg.Speech.Extraction.MaxResultBytes <= 0 || cfg.Speech.Extraction.MaxResultBytes > 16384 {
		return fmt.Errorf("server: speech.extraction.max_result_bytes must be between 1 and 16384")
	}
	extractionTimeout, err := parsePositiveConfigDuration(cfg.Speech.Extraction.RequestTimeout)
	if err != nil {
		return fmt.Errorf("server: speech.extraction.request_timeout: %w", err)
	}
	if extractionTimeout > maxSpeechExtractionRequestTimeout {
		return fmt.Errorf("server: speech.extraction.request_timeout must be at most 2m0s")
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
	if cfg.Flowcraft != nil {
		if err := validateStoreReference("agent_host.flowcraft.state_store", cfg.Flowcraft.StateStore); err != nil {
			return err
		}
		if err := validateStoreReference("agent_host.flowcraft.history_store", cfg.Flowcraft.HistoryStore); err != nil {
			return err
		}
	}
	return nil
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
		kind := fmt.Sprint(mapping["kind"])
		if kind == "memory" {
			return fmt.Errorf("server: stores.%s kind %q is no longer supported; configure MemoryLayout resources and RuntimeProfile memory bindings instead", name, kind)
		}
		if kind != stores.KindLog {
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
	if flowcraftValue, exists := agentHost["flowcraft"]; exists {
		flowcraft, ok := flowcraftValue.(map[string]any)
		if !ok {
			return fmt.Errorf("server: agent_host.flowcraft must be a mapping")
		}
		for field := range flowcraft {
			switch field {
			case "state_store", "history_store":
			default:
				return fmt.Errorf("server: agent_host.flowcraft has unknown field %q", field)
			}
		}
		for _, field := range []string{"state_store", "history_store"} {
			if reference, exists := flowcraft[field]; exists {
				if err := validateFileStoreReference("agent_host.flowcraft."+field, reference); err != nil {
					return err
				}
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

func (cfg Config) PublicAPIListenAddr() string {
	return cfg.Listen
}

func (cfg Config) ICEListenAddr() string {
	return cfg.Listen
}

func parseConfigDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if before, ok := strings.CutSuffix(value, "d"); ok {
		days, err := time.ParseDuration(before + "h")
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
