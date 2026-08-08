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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/goccy/go-yaml"
)

type Config struct {
	WorkspaceRoot   string `yaml:"-"`
	KeyPair         *giznet.KeyPair
	Listen          string
	Endpoint        string
	ServeToClients  bool
	EdgeNodes       []giznet.PublicKey
	ICEServers      []gizwebrtc.ICEServer
	AdminPublicKey  giznet.PublicKey
	Storage         map[string]storage.Config
	Stores          map[string]stores.Config
	Services        *ServicesConfig
	Friends         FriendsConfig
	FriendGroups    FriendGroupsConfig
	Speech          SpeechConfig
	PendingDeletion PendingDeletionConfig
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

// ServicesConfig is the fixed service-to-Store binding schema. Unlike the
// storage and stores registries, service names are not operator-defined.
type ServicesConfig struct {
	Peer            *PeerStoresConfig           `yaml:"peer"`
	PublicLogin     *SingleStoreConfig          `yaml:"public_login"`
	Credential      *SingleStoreConfig          `yaml:"credential"`
	Firmware        *SingleStoreConfig          `yaml:"firmware"`
	RuntimeProfile  *SingleStoreConfig          `yaml:"runtime_profile"`
	Model           *SingleStoreConfig          `yaml:"model"`
	Voice           *SingleStoreConfig          `yaml:"voice"`
	MemoryLayout    *SingleStoreConfig          `yaml:"memory_layout"`
	ProviderTenants *ProviderTenantStoresConfig `yaml:"provider_tenants"`
	Workflow        *SingleStoreConfig          `yaml:"workflow"`
	Workspace       *WorkspaceStoresConfig      `yaml:"workspace"`
	Toolkit         *SingleStoreConfig          `yaml:"toolkit"`
	Contact         *SingleStoreConfig          `yaml:"contact"`
	Friend          *FriendStoresConfig         `yaml:"friend"`
	FriendGroup     *FriendGroupStoresConfig    `yaml:"friend_group"`
	Gameplay        *GameplayStoresConfig       `yaml:"gameplay"`
	AgentHost       *AgentHostConfig            `yaml:"agent_host"`
	Metrics         *SingleStoreConfig          `yaml:"metrics"`
	SystemLog       *logging.Config             `yaml:"system_log"`
}

type SingleStoreConfig struct {
	Store string `yaml:"store"`
}

type PeerStoresConfig struct {
	Store      string `yaml:"store"`
	RouteStore string `yaml:"route_store"`
	RunStore   string `yaml:"run_store"`
}

type ProviderTenantStoresConfig struct {
	GenericStore        string `yaml:"generic_store"`
	MiniMaxTenantStore  string `yaml:"minimax_tenant_store"`
	DeepSeekTenantStore string `yaml:"deepseek_tenant_store"`
	VolcTenantStore     string `yaml:"volc_tenant_store"`
	CredentialStore     string `yaml:"credential_store"`
	ModelStore          string `yaml:"model_store"`
	VoiceStore          string `yaml:"voice_store"`
}

type WorkspaceStoresConfig struct {
	Store         string `yaml:"store"`
	WorkflowStore string `yaml:"workflow_store"`
	AssetsStore   string `yaml:"assets_store"`
}

type FriendStoresConfig struct {
	Store            string `yaml:"store"`
	InviteTokenStore string `yaml:"invite_token_store"`
}

type FriendGroupStoresConfig struct {
	Store            string `yaml:"store"`
	InviteTokenStore string `yaml:"invite_token_store"`
	MemberStore      string `yaml:"member_store"`
	BelongStore      string `yaml:"belong_store"`
}

type GameplayStoresConfig struct {
	PetDefStore   string `yaml:"pet_def_store"`
	BadgeDefStore string `yaml:"badge_def_store"`
	GameDefStore  string `yaml:"game_def_store"`
	AssetsStore   string `yaml:"assets_store"`
	DatabaseStore string `yaml:"database_store"`
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

type PendingDeletionConfig struct {
	ScanInterval     string `yaml:"scan_interval"`
	PageSize         int    `yaml:"page_size"`
	DispatchCapacity int    `yaml:"dispatch_capacity"`
	Workers          int    `yaml:"workers"`
	LeaseDuration    string `yaml:"lease_duration"`
	AttemptTimeout   string `yaml:"attempt_timeout"`
	RetryInitial     string `yaml:"retry_initial"`
	RetryMax         string `yaml:"retry_max"`
	MaxAttempts      int    `yaml:"max_attempts"`
}

type pendingDeletionFileConfig struct {
	ScanInterval     *string `yaml:"scan_interval"`
	PageSize         *int    `yaml:"page_size"`
	DispatchCapacity *int    `yaml:"dispatch_capacity"`
	Workers          *int    `yaml:"workers"`
	LeaseDuration    *string `yaml:"lease_duration"`
	AttemptTimeout   *string `yaml:"attempt_timeout"`
	RetryInitial     *string `yaml:"retry_initial"`
	RetryMax         *string `yaml:"retry_max"`
	MaxAttempts      *int    `yaml:"max_attempts"`
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
	Identity        IdentityConfig            `yaml:"identity"`
	Listen          string                    `yaml:"listen"`
	Endpoint        string                    `yaml:"endpoint"`
	ServeToClients  bool                      `yaml:"serve-to-clients"`
	EdgeNodes       []giznet.PublicKey        `yaml:"edge-nodes"`
	ICEServers      []gizwebrtc.ICEServer     `yaml:"ice-servers"`
	AdminPublicKey  giznet.PublicKey          `yaml:"admin-public-key"`
	Storage         map[string]storage.Config `yaml:"storage"`
	Stores          map[string]stores.Config  `yaml:"stores"`
	Services        *ServicesConfig           `yaml:"services"`
	Friends         FriendsConfig             `yaml:"friends"`
	FriendGroups    FriendGroupsConfig        `yaml:"friend_groups"`
	Speech          SpeechConfig              `yaml:"speech"`
	PendingDeletion PendingDeletionConfig     `yaml:"pending_deletion"`
}

const maxSpeechExtractionRequestTimeout = 120 * time.Second

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
		Identity        *IdentityConfig           `yaml:"identity"`
		Listen          string                    `yaml:"listen"`
		Endpoint        string                    `yaml:"endpoint"`
		ServeToClients  *bool                     `yaml:"serve-to-clients"`
		EdgeNodes       []giznet.PublicKey        `yaml:"edge-nodes"`
		ICEServers      []gizwebrtc.ICEServer     `yaml:"ice-servers"`
		AdminPublicKey  *giznet.PublicKey         `yaml:"admin-public-key"`
		Storage         map[string]storage.Config `yaml:"storage"`
		Stores          map[string]stores.Config  `yaml:"stores"`
		Services        *ServicesConfig           `yaml:"services"`
		Friends         FriendsConfig             `yaml:"friends"`
		FriendGroups    FriendGroupsConfig        `yaml:"friend_groups"`
		Speech          speechFileConfig          `yaml:"speech"`
		PendingDeletion pendingDeletionFileConfig `yaml:"pending_deletion"`
	}
	if err := yaml.UnmarshalWithOptions(data, &raw, yaml.DisallowUnknownField()); err != nil {
		return ConfigFile{}, err
	}
	adminPublicKey, err := resolveAdminPublicKey(raw.AdminPublicKey)
	if err != nil {
		return ConfigFile{}, err
	}
	if raw.Services != nil && raw.Services.SystemLog != nil {
		logCfg, err := logging.PrepareConfig(*raw.Services.SystemLog)
		if err != nil {
			return ConfigFile{}, fmt.Errorf("server: %w", err)
		}
		raw.Services.SystemLog = &logCfg
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
	pendingDeletion, err := raw.PendingDeletion.runtimeConfig()
	if err != nil {
		return ConfigFile{}, err
	}
	cfg := ConfigFile{
		Identity:        identity,
		Listen:          raw.Listen,
		Endpoint:        raw.Endpoint,
		ServeToClients:  serveToClients,
		EdgeNodes:       raw.EdgeNodes,
		ICEServers:      raw.ICEServers,
		AdminPublicKey:  adminPublicKey,
		Storage:         raw.Storage,
		Stores:          raw.Stores,
		Services:        raw.Services,
		Friends:         raw.Friends,
		FriendGroups:    raw.FriendGroups,
		Speech:          speech,
		PendingDeletion: pendingDeletion,
	}
	return cfg, nil
}

func (cfg pendingDeletionFileConfig) runtimeConfig() (PendingDeletionConfig, error) {
	var out PendingDeletionConfig
	if cfg.ScanInterval != nil {
		out.ScanInterval = *cfg.ScanInterval
	}
	if cfg.PageSize != nil {
		if *cfg.PageSize <= 0 || *cfg.PageSize > 1000 {
			return PendingDeletionConfig{}, fmt.Errorf("server: pending_deletion.page_size must be between 1 and 1000")
		}
		out.PageSize = *cfg.PageSize
	}
	if cfg.DispatchCapacity != nil {
		if *cfg.DispatchCapacity <= 0 || *cfg.DispatchCapacity > 10000 {
			return PendingDeletionConfig{}, fmt.Errorf("server: pending_deletion.dispatch_capacity must be between 1 and 10000")
		}
		out.DispatchCapacity = *cfg.DispatchCapacity
	}
	if cfg.Workers != nil {
		if *cfg.Workers <= 0 || *cfg.Workers > 256 {
			return PendingDeletionConfig{}, fmt.Errorf("server: pending_deletion.workers must be between 1 and 256")
		}
		out.Workers = *cfg.Workers
	}
	if cfg.LeaseDuration != nil {
		out.LeaseDuration = *cfg.LeaseDuration
	}
	if cfg.AttemptTimeout != nil {
		out.AttemptTimeout = *cfg.AttemptTimeout
	}
	if cfg.RetryInitial != nil {
		out.RetryInitial = *cfg.RetryInitial
	}
	if cfg.RetryMax != nil {
		out.RetryMax = *cfg.RetryMax
	}
	if cfg.MaxAttempts != nil {
		if *cfg.MaxAttempts <= 0 || *cfg.MaxAttempts > 1000 {
			return PendingDeletionConfig{}, fmt.Errorf("server: pending_deletion.max_attempts must be between 1 and 1000")
		}
		out.MaxAttempts = *cfg.MaxAttempts
	}
	if _, err := out.processorConfigAllowZero(); err != nil {
		return PendingDeletionConfig{}, fmt.Errorf("server: pending_deletion: %w", err)
	}
	return out, nil
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
		Listen:   "0.0.0.0:9820",
		Endpoint: "0.0.0.0:9820",
		Speech: SpeechConfig{
			Transcription: SpeechTranscriptionConfig{MaxAudioBytes: 2097152, MaxAudioDuration: "60s", RequestTimeout: "75s"},
			Extraction: SpeechExtractionConfig{
				MaxSchemaBytes: 16384, MaxSchemaDepth: 16, MaxSchemaProperties: 128,
				MaxInstructionBytes: 4096, MaxResultBytes: 16384, RequestTimeout: "120s",
			},
			Synthesis: SpeechSynthesisConfig{MaxTextBytes: 4096, MaxOutputBytes: 4194304, RequestTimeout: "120s"},
		},
		PendingDeletion: PendingDeletionConfig{
			ScanInterval: "30s", PageSize: 100, DispatchCapacity: 256, Workers: 4,
			LeaseDuration: "2m", AttemptTimeout: "90s", RetryInitial: "5s",
			RetryMax: "30m", MaxAttempts: 10,
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
	if cfg.Services == nil {
		cfg.Services = fileCfg.Services
	}
	cfg.Friends = mergeFriendsConfig(cfg.Friends, fileCfg.Friends)
	cfg.FriendGroups = mergeFriendGroupsConfig(cfg.FriendGroups, fileCfg.FriendGroups)
	cfg.Speech = mergeSpeechConfig(cfg.Speech, fileCfg.Speech)
	cfg.PendingDeletion = mergePendingDeletionConfig(cfg.PendingDeletion, fileCfg.PendingDeletion)
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

func mergePendingDeletionConfig(runtime, fallback PendingDeletionConfig) PendingDeletionConfig {
	if runtime.ScanInterval == "" {
		runtime.ScanInterval = fallback.ScanInterval
	}
	if runtime.PageSize == 0 {
		runtime.PageSize = fallback.PageSize
	}
	if runtime.DispatchCapacity == 0 {
		runtime.DispatchCapacity = fallback.DispatchCapacity
	}
	if runtime.Workers == 0 {
		runtime.Workers = fallback.Workers
	}
	if runtime.LeaseDuration == "" {
		runtime.LeaseDuration = fallback.LeaseDuration
	}
	if runtime.AttemptTimeout == "" {
		runtime.AttemptTimeout = fallback.AttemptTimeout
	}
	if runtime.RetryInitial == "" {
		runtime.RetryInitial = fallback.RetryInitial
	}
	if runtime.RetryMax == "" {
		runtime.RetryMax = fallback.RetryMax
	}
	if runtime.MaxAttempts == 0 {
		runtime.MaxAttempts = fallback.MaxAttempts
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
	cfg.PendingDeletion = mergePendingDeletionConfig(cfg.PendingDeletion, defaults.PendingDeletion)
	if cfg.Services != nil && cfg.Services.SystemLog != nil {
		logCfg, err := logging.PrepareConfig(*cfg.Services.SystemLog)
		if err != nil {
			return Config{}, fmt.Errorf("server: %w", err)
		}
		cfg.Services.SystemLog = &logCfg
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
	if err := validateServicesConfig(cfg.Services); err != nil {
		return err
	}
	processorConfig, err := cfg.PendingDeletion.processorConfig()
	if err != nil {
		return fmt.Errorf("server: pending_deletion: %w", err)
	}
	if err := processorConfig.Validate(); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func (cfg PendingDeletionConfig) processorConfig() (pendingdeletion.Config, error) {
	scanInterval, err := parsePositiveConfigDuration(cfg.ScanInterval)
	if err != nil {
		return pendingdeletion.Config{}, fmt.Errorf("scan_interval: %w", err)
	}
	leaseDuration, err := parsePositiveConfigDuration(cfg.LeaseDuration)
	if err != nil {
		return pendingdeletion.Config{}, fmt.Errorf("lease_duration: %w", err)
	}
	attemptTimeout, err := parsePositiveConfigDuration(cfg.AttemptTimeout)
	if err != nil {
		return pendingdeletion.Config{}, fmt.Errorf("attempt_timeout: %w", err)
	}
	retryInitial, err := parsePositiveConfigDuration(cfg.RetryInitial)
	if err != nil {
		return pendingdeletion.Config{}, fmt.Errorf("retry_initial: %w", err)
	}
	retryMax, err := parsePositiveConfigDuration(cfg.RetryMax)
	if err != nil {
		return pendingdeletion.Config{}, fmt.Errorf("retry_max: %w", err)
	}
	return pendingdeletion.Config{
		ScanInterval: scanInterval, PageSize: cfg.PageSize, DispatchCapacity: cfg.DispatchCapacity,
		Workers: cfg.Workers, LeaseDuration: leaseDuration, AttemptTimeout: attemptTimeout,
		RetryInitial: retryInitial, RetryMax: retryMax, MaxAttempts: cfg.MaxAttempts,
	}, nil
}

func (cfg PendingDeletionConfig) processorConfigAllowZero() (pendingdeletion.Config, error) {
	parse := func(name, value string) (time.Duration, error) {
		if value == "" {
			return 0, nil
		}
		duration, err := parsePositiveConfigDuration(value)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
		return duration, nil
	}
	scan, err := parse("scan_interval", cfg.ScanInterval)
	if err != nil {
		return pendingdeletion.Config{}, err
	}
	lease, err := parse("lease_duration", cfg.LeaseDuration)
	if err != nil {
		return pendingdeletion.Config{}, err
	}
	attempt, err := parse("attempt_timeout", cfg.AttemptTimeout)
	if err != nil {
		return pendingdeletion.Config{}, err
	}
	initial, err := parse("retry_initial", cfg.RetryInitial)
	if err != nil {
		return pendingdeletion.Config{}, err
	}
	maximum, err := parse("retry_max", cfg.RetryMax)
	if err != nil {
		return pendingdeletion.Config{}, err
	}
	for name, value := range map[string]int{"page_size": cfg.PageSize, "dispatch_capacity": cfg.DispatchCapacity, "workers": cfg.Workers, "max_attempts": cfg.MaxAttempts} {
		if value < 0 {
			return pendingdeletion.Config{}, fmt.Errorf("%s must be positive", name)
		}
	}
	return pendingdeletion.Config{ScanInterval: scan, PageSize: cfg.PageSize, DispatchCapacity: cfg.DispatchCapacity, Workers: cfg.Workers, LeaseDuration: lease, AttemptTimeout: attempt, RetryInitial: initial, RetryMax: maximum, MaxAttempts: cfg.MaxAttempts}, nil
}

func validateServicesConfig(cfg *ServicesConfig) error {
	if cfg == nil {
		return fmt.Errorf("server: services is required")
	}
	type reference struct{ path, value string }
	references := make([]reference, 0, 40)
	requireBlock := func(path string, present bool) error {
		if !present {
			return fmt.Errorf("server: %s is required", path)
		}
		return nil
	}
	for _, block := range []struct {
		path    string
		present bool
	}{
		{"services.peer", cfg.Peer != nil},
		{"services.public_login", cfg.PublicLogin != nil},
		{"services.credential", cfg.Credential != nil},
		{"services.firmware", cfg.Firmware != nil},
		{"services.runtime_profile", cfg.RuntimeProfile != nil},
		{"services.model", cfg.Model != nil},
		{"services.voice", cfg.Voice != nil},
		{"services.memory_layout", cfg.MemoryLayout != nil},
		{"services.provider_tenants", cfg.ProviderTenants != nil},
		{"services.workflow", cfg.Workflow != nil},
		{"services.workspace", cfg.Workspace != nil},
		{"services.toolkit", cfg.Toolkit != nil},
		{"services.contact", cfg.Contact != nil},
		{"services.friend", cfg.Friend != nil},
		{"services.friend_group", cfg.FriendGroup != nil},
		{"services.gameplay", cfg.Gameplay != nil},
	} {
		if err := requireBlock(block.path, block.present); err != nil {
			return err
		}
	}
	references = append(references,
		reference{"services.peer.store", cfg.Peer.Store},
		reference{"services.peer.route_store", cfg.Peer.RouteStore},
		reference{"services.peer.run_store", cfg.Peer.RunStore},
		reference{"services.public_login.store", cfg.PublicLogin.Store},
		reference{"services.credential.store", cfg.Credential.Store},
		reference{"services.firmware.store", cfg.Firmware.Store},
		reference{"services.runtime_profile.store", cfg.RuntimeProfile.Store},
		reference{"services.model.store", cfg.Model.Store},
		reference{"services.voice.store", cfg.Voice.Store},
		reference{"services.memory_layout.store", cfg.MemoryLayout.Store},
		reference{"services.provider_tenants.generic_store", cfg.ProviderTenants.GenericStore},
		reference{"services.provider_tenants.minimax_tenant_store", cfg.ProviderTenants.MiniMaxTenantStore},
		reference{"services.provider_tenants.deepseek_tenant_store", cfg.ProviderTenants.DeepSeekTenantStore},
		reference{"services.provider_tenants.volc_tenant_store", cfg.ProviderTenants.VolcTenantStore},
		reference{"services.provider_tenants.credential_store", cfg.ProviderTenants.CredentialStore},
		reference{"services.provider_tenants.model_store", cfg.ProviderTenants.ModelStore},
		reference{"services.provider_tenants.voice_store", cfg.ProviderTenants.VoiceStore},
		reference{"services.workflow.store", cfg.Workflow.Store},
		reference{"services.workspace.store", cfg.Workspace.Store},
		reference{"services.workspace.workflow_store", cfg.Workspace.WorkflowStore},
		reference{"services.workspace.assets_store", cfg.Workspace.AssetsStore},
		reference{"services.toolkit.store", cfg.Toolkit.Store},
		reference{"services.contact.store", cfg.Contact.Store},
		reference{"services.friend.store", cfg.Friend.Store},
		reference{"services.friend.invite_token_store", cfg.Friend.InviteTokenStore},
		reference{"services.friend_group.store", cfg.FriendGroup.Store},
		reference{"services.friend_group.invite_token_store", cfg.FriendGroup.InviteTokenStore},
		reference{"services.friend_group.member_store", cfg.FriendGroup.MemberStore},
		reference{"services.friend_group.belong_store", cfg.FriendGroup.BelongStore},
		reference{"services.gameplay.pet_def_store", cfg.Gameplay.PetDefStore},
		reference{"services.gameplay.badge_def_store", cfg.Gameplay.BadgeDefStore},
		reference{"services.gameplay.game_def_store", cfg.Gameplay.GameDefStore},
		reference{"services.gameplay.assets_store", cfg.Gameplay.AssetsStore},
		reference{"services.gameplay.database_store", cfg.Gameplay.DatabaseStore},
	)
	for _, ref := range references {
		if strings.TrimSpace(ref.value) == "" {
			return fmt.Errorf("server: %s is required and must not be whitespace-only", ref.path)
		}
	}
	if cfg.AgentHost != nil {
		if err := validateStoreReference("services.agent_host.runtime_store", cfg.AgentHost.RuntimeStore); err != nil {
			return err
		}
		if cfg.AgentHost.Flowcraft != nil {
			if err := validateStoreReference("services.agent_host.flowcraft.state_store", cfg.AgentHost.Flowcraft.StateStore); err != nil {
				return err
			}
			if err := validateStoreReference("services.agent_host.flowcraft.history_store", cfg.AgentHost.Flowcraft.HistoryStore); err != nil {
				return err
			}
		}
	}
	if cfg.Metrics != nil && strings.TrimSpace(cfg.Metrics.Store) == "" {
		return fmt.Errorf("server: services.metrics.store is required and must not be whitespace-only")
	}
	if cfg.SystemLog != nil {
		if _, err := logging.PrepareConfig(*cfg.SystemLog); err != nil {
			return fmt.Errorf("server: %w", err)
		}
	}
	return nil
}

func (cfg Config) systemLogConfig() logging.Config {
	if cfg.Services == nil || cfg.Services.SystemLog == nil {
		return logging.DefaultConfig()
	}
	return *cfg.Services.SystemLog
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
		return fmt.Errorf("server: top-level log configuration was removed; configure stores and services.system_log instead")
	}
	if agentHostValue, exists := document["agent_host"]; exists {
		if err := validateAgentHostConfigShape("agent_host", agentHostValue); err != nil {
			return err
		}
	}
	if servicesValue, exists := document["services"]; exists {
		services, ok := servicesValue.(map[string]any)
		if !ok {
			return fmt.Errorf("server: services must be a mapping")
		}
		if err := validateServicesConfigShape(services); err != nil {
			return err
		}
	}
	if value, exists := document["pending_deletion"]; exists {
		mapping, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("server: pending_deletion must be a mapping")
		}
		for field := range mapping {
			switch field {
			case "scan_interval", "page_size", "dispatch_capacity", "workers", "lease_duration", "attempt_timeout", "retry_initial", "retry_max", "max_attempts":
			default:
				return fmt.Errorf("server: pending_deletion has unknown field %q", field)
			}
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
		if kind == "log" {
			return fmt.Errorf("server: stores.%s kind %q is not supported; use %s or %s", name, kind, stores.KindLogImmutable, stores.KindLogMutable)
		}
		allowedFields := map[string]map[string]struct{}{
			stores.KindKeyValue:     {"kind": {}, "storage": {}, "prefix": {}},
			stores.KindVecStore:     {"kind": {}, "storage": {}},
			stores.KindObjectStore:  {"kind": {}, "storage": {}, "prefix": {}},
			stores.KindSQL:          {"kind": {}, "storage": {}},
			stores.KindGraph:        {"kind": {}, "backend": {}, "store": {}, "prefix": {}},
			stores.KindMetrics:      {"kind": {}, "storage": {}, "memory": {}, "clickhouse": {}},
			stores.KindLogImmutable: {"kind": {}, "storage": {}, "volc": {}, "clickhouse": {}},
			stores.KindLogMutable:   {"kind": {}, "storage": {}, "volc": {}, "clickhouse": {}},
		}
		allowed, knownKind := allowedFields[kind]
		if !knownKind {
			continue
		}
		for field := range mapping {
			if _, ok := allowed[field]; !ok {
				return fmt.Errorf("server: stores.%s field %q is invalid for kind %s", name, field, kind)
			}
		}
		if volcValue, exists := mapping["volc"]; exists {
			volcMapping, ok := volcValue.(map[string]any)
			if !ok {
				return fmt.Errorf("server: stores.%s.volc must be a mapping", name)
			}
			for field := range volcMapping {
				switch field {
				case "topic_id":
				default:
					return fmt.Errorf("server: stores.%s.volc has unknown field %q", name, field)
				}
			}
		}
		if kind != stores.KindLogImmutable && kind != stores.KindLogMutable && kind != stores.KindMetrics {
			continue
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
			case "database", "table":
			default:
				return fmt.Errorf("server: stores.%s.clickhouse has unknown field %q", name, field)
			}
		}
	}
	return nil
}

func validateServicesConfigShape(services map[string]any) error {
	stringFields := map[string][]string{
		"peer":             {"store", "route_store", "run_store"},
		"public_login":     {"store"},
		"credential":       {"store"},
		"firmware":         {"store"},
		"runtime_profile":  {"store"},
		"model":            {"store"},
		"voice":            {"store"},
		"memory_layout":    {"store"},
		"provider_tenants": {"generic_store", "minimax_tenant_store", "deepseek_tenant_store", "volc_tenant_store", "credential_store", "model_store", "voice_store"},
		"workflow":         {"store"},
		"workspace":        {"store", "workflow_store", "assets_store"},
		"toolkit":          {"store"},
		"contact":          {"store"},
		"friend":           {"store", "invite_token_store"},
		"friend_group":     {"store", "invite_token_store", "member_store", "belong_store"},
		"gameplay":         {"pet_def_store", "badge_def_store", "game_def_store", "assets_store", "database_store"},
		"metrics":          {"store"},
	}
	for service, fields := range stringFields {
		value, exists := services[service]
		if !exists || value == nil {
			continue
		}
		mapping, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("server: services.%s must be a mapping", service)
		}
		for _, field := range fields {
			if reference, exists := mapping[field]; exists {
				if err := validateFileStoreReference("services."+service+"."+field, reference); err != nil {
					return err
				}
			}
		}
	}
	if value, exists := services["agent_host"]; exists {
		if err := validateAgentHostConfigShape("services.agent_host", value); err != nil {
			return err
		}
	}
	if value, exists := services["system_log"]; exists {
		if err := validateSystemLogConfigShape("services.system_log", value); err != nil {
			return err
		}
	}
	return nil
}

func validateSystemLogConfigShape(path string, value any) error {
	mapping, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("server: %s must be a mapping", path)
	}
	for _, field := range []string{"level", "query_store"} {
		if reference, exists := mapping[field]; exists {
			if err := validateFileStoreReference(path+"."+field, reference); err != nil {
				return err
			}
		}
	}
	sinksValue, exists := mapping["sinks"]
	if !exists || sinksValue == nil {
		return nil
	}
	sinks, ok := sinksValue.([]any)
	if !ok {
		return fmt.Errorf("server: %s.sinks must be a sequence", path)
	}
	for index, sinkValue := range sinks {
		sink, ok := sinkValue.(map[string]any)
		if !ok {
			return fmt.Errorf("server: %s.sinks[%d] must be a mapping", path, index)
		}
		for _, field := range []string{"kind", "store", "level"} {
			if reference, exists := sink[field]; exists {
				if err := validateFileStoreReference(fmt.Sprintf("%s.sinks[%d].%s", path, index, field), reference); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateAgentHostConfigShape(path string, value any) error {
	agentHost, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("server: %s must be a mapping", path)
	}
	for field := range agentHost {
		switch field {
		case "runtime_store", "flowcraft":
		default:
			return fmt.Errorf("server: %s has unknown field %q", path, field)
		}
	}
	if runtimeStore, exists := agentHost["runtime_store"]; exists {
		if err := validateFileStoreReference(path+".runtime_store", runtimeStore); err != nil {
			return err
		}
	}
	if flowcraftValue, exists := agentHost["flowcraft"]; exists {
		if flowcraftValue == nil {
			return nil
		}
		flowcraft, ok := flowcraftValue.(map[string]any)
		if !ok {
			return fmt.Errorf("server: %s.flowcraft must be a mapping", path)
		}
		for field := range flowcraft {
			switch field {
			case "state_store", "history_store":
			default:
				return fmt.Errorf("server: %s.flowcraft has unknown field %q", path, field)
			}
		}
		for _, field := range []string{"state_store", "history_store"} {
			if reference, exists := flowcraft[field]; exists {
				if err := validateFileStoreReference(path+".flowcraft."+field, reference); err != nil {
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
