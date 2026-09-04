package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/goccy/go-yaml"
)

type Config struct {
	WorkspaceRoot   string `yaml:"-"`
	KeyPair         *giznet.KeyPair
	WebRTC          WebRTCConfig
	HTTP            HTTPConfig
	EdgeNodes       []giznet.PublicKey
	ICEServers      []gizwebrtc.ICEServer
	AdminPublicKey  giznet.PublicKey
	Storage         map[string]storage.Config
	Stores          map[string]store.Config
	Services        *ServicesConfig
	Friends         FriendsConfig
	FriendGroups    FriendGroupsConfig
	Speech          SpeechConfig
	PendingDeletion PendingDeletionConfig
	Profiling       ProfilingConfig
}

// WebRTCConfig defines the Server WebRTC transport bind and published tuples.
type WebRTCConfig struct {
	Listen   string `yaml:"listen"`
	Endpoint string `yaml:"endpoint"`
}

// HTTPConfig defines the ordered Server HTTP and HTTPS listeners.
type HTTPConfig struct {
	Listeners []HTTPListenerConfig `yaml:"listeners"`
}

// HTTPListenerConfig defines one Server HTTP bind address and optional TLS files.
type HTTPListenerConfig struct {
	Listen string                `yaml:"listen"`
	TLS    HTTPListenerTLSConfig `yaml:"tls"`
}

// HTTPListenerTLSConfig enables TLS when both certificate files are set.
type HTTPListenerTLSConfig struct {
	CertFile string `yaml:"cert-file"`
	KeyFile  string `yaml:"key-file"`
}

func (cfg HTTPListenerTLSConfig) enabled() bool {
	return cfg.CertFile != "" || cfg.KeyFile != ""
}

func (cfg HTTPListenerTLSConfig) tlsConfig(path string) (*tls.Config, error) {
	if !cfg.enabled() {
		return nil, nil
	}
	if cfg.CertFile == "" || cfg.KeyFile == "" {
		return nil, fmt.Errorf("server: %s requires cert-file and key-file", path)
	}
	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("server: %s load certificate: %w", path, err)
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}, nil
}

// ProfilingConfig controls persistent Go runtime snapshots for the Server process.
type ProfilingConfig struct {
	Enabled bool   `yaml:"enabled"`
	Store   string `yaml:"store"`
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
	Peer            *SingleStoreConfig     `yaml:"peer"`
	PeerRun         *SingleStoreConfig     `yaml:"peer_run"`
	APIKey          *SingleStoreConfig     `yaml:"api_key"`
	Credential      *SingleStoreConfig     `yaml:"credential"`
	Firmware        *SingleStoreConfig     `yaml:"firmware"`
	RuntimeProfile  *SingleStoreConfig     `yaml:"runtime_profile"`
	Model           *SingleStoreConfig     `yaml:"model"`
	Voice           *SingleStoreConfig     `yaml:"voice"`
	MemoryLayout    *SingleStoreConfig     `yaml:"memory_layout"`
	ProviderTenants *SingleStoreConfig     `yaml:"provider_tenants"`
	Workflow        *SingleStoreConfig     `yaml:"workflow"`
	Workspace       *WorkspaceStoresConfig `yaml:"workspace"`
	SFU             *SFUConfig             `yaml:"sfu"`
	Toolkit         *SingleStoreConfig     `yaml:"toolkit"`
	Contact         *SingleStoreConfig     `yaml:"contact"`
	Friend          *SingleStoreConfig     `yaml:"friend"`
	FriendGroup     *SingleStoreConfig     `yaml:"friend_group"`
	Gameplay        *GameplayStoresConfig  `yaml:"gameplay"`
	AgentHost       *AgentHostConfig       `yaml:"agent_host"`
	Metrics         *SingleStoreConfig     `yaml:"metrics"`
	SystemLog       *gizlog.Config         `yaml:"system_log"`
}

type SingleStoreConfig struct {
	Store string `yaml:"store"`
}

type WorkspaceStoresConfig struct {
	Store              string `yaml:"store"`
	HistoryStore       string `yaml:"history_store"`
	HistoryAssetsStore string `yaml:"history_assets_store"`
	AssetsStore        string `yaml:"assets_store"`
}

// SFUConfig binds the Server to one SFU signaling endpoint. Credentials are
// only ever loaded from files at startup; a missing section disables SFU
// Workspaces on this Server.
type SFUConfig struct {
	URL              string `yaml:"url"`
	APIKeyFile       string `yaml:"api_key_file"`
	APISecretFile    string `yaml:"api_secret_file"`
	RecheckInterval  string `yaml:"recheck_interval"`
	ReconnectTimeout string `yaml:"reconnect_timeout"`
	// TalkHangover closes a Peer's talk utterance after that long without a
	// voiced Opus frame; FloorIdle releases the downlink floor after that
	// long without a voiced packet from its holder.
	TalkHangover string `yaml:"talk_hangover"`
	FloorIdle    string `yaml:"floor_idle"`
}

// sfuDurations holds the parsed optional durations of services.sfu.
type sfuDurations struct {
	recheck      time.Duration
	reconnect    time.Duration
	talkHangover time.Duration
	floorIdle    time.Duration
}

func (cfg *SFUConfig) validate() error {
	if cfg == nil {
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil {
		return fmt.Errorf("server: services.sfu.url: %w", err)
	}
	if (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" {
		return fmt.Errorf("server: services.sfu.url must be a ws:// or wss:// URL")
	}
	if strings.TrimSpace(cfg.APIKeyFile) == "" {
		return fmt.Errorf("server: services.sfu.api_key_file is required")
	}
	if strings.TrimSpace(cfg.APISecretFile) == "" {
		return fmt.Errorf("server: services.sfu.api_secret_file is required")
	}
	if _, err := cfg.durations(); err != nil {
		return err
	}
	return nil
}

func (cfg *SFUConfig) durations() (sfuDurations, error) {
	parsed := sfuDurations{
		recheck:      sfu.DefaultRecheckInterval,
		reconnect:    sfu.DefaultReconnectTimeout,
		talkHangover: sfu.DefaultTalkHangover,
		floorIdle:    sfu.DefaultFloorIdle,
	}
	for _, field := range []struct {
		name  string
		value string
		into  *time.Duration
	}{
		{"recheck_interval", cfg.RecheckInterval, &parsed.recheck},
		{"reconnect_timeout", cfg.ReconnectTimeout, &parsed.reconnect},
		{"talk_hangover", cfg.TalkHangover, &parsed.talkHangover},
		{"floor_idle", cfg.FloorIdle, &parsed.floorIdle},
	} {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		duration, err := parsePositiveConfigDuration(field.value)
		if err != nil {
			return sfuDurations{}, fmt.Errorf("server: services.sfu.%s: %w", field.name, err)
		}
		*field.into = duration
	}
	return parsed, nil
}

// connectorConfig loads the credential files and returns the runtime
// connector configuration. A nil receiver yields the zero, disabled config.
func (cfg *SFUConfig) connectorConfig() (sfu.Config, error) {
	if cfg == nil {
		return sfu.Config{}, nil
	}
	if err := cfg.validate(); err != nil {
		return sfu.Config{}, err
	}
	durations, err := cfg.durations()
	if err != nil {
		return sfu.Config{}, err
	}
	apiKey, err := readSecretFile("services.sfu.api_key_file", cfg.APIKeyFile)
	if err != nil {
		return sfu.Config{}, err
	}
	apiSecret, err := readSecretFile("services.sfu.api_secret_file", cfg.APISecretFile)
	if err != nil {
		return sfu.Config{}, err
	}
	return sfu.Config{
		URL:              strings.TrimSpace(cfg.URL),
		APIKey:           apiKey,
		APISecret:        apiSecret,
		RecheckInterval:  durations.recheck,
		ReconnectTimeout: durations.reconnect,
		TalkHangover:     durations.talkHangover,
		FloorIdle:        durations.floorIdle,
	}, nil
}

func readSecretFile(path, file string) (string, error) {
	data, err := os.ReadFile(os.ExpandEnv(strings.TrimSpace(file)))
	if err != nil {
		return "", fmt.Errorf("server: %s: %w", path, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("server: %s is empty", path)
	}
	return value, nil
}

type GameplayStoresConfig struct {
	Store         string `yaml:"store"`
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

type storageFileConfig struct {
	Kind            string `yaml:"kind"`
	Dir             string `yaml:"dir"`
	DSN             string `yaml:"dsn"`
	URL             string `yaml:"url"`
	TLSCAFile       string `yaml:"tls_ca_file"`
	RemoteWriteURL  string `yaml:"remote_write_url"`
	QueryURL        string `yaml:"query_url"`
	BearerToken     string `yaml:"bearer_token"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	Bucket          string `yaml:"bucket"`
	SessionToken    string `yaml:"session_token"`
	SecurityToken   string `yaml:"security_token"`
	CredentialsFile string `yaml:"credentials_file"`
	AccountURL      string `yaml:"account_url"`
	Container       string `yaml:"container"`
}

func (cfg storageFileConfig) runtimeConfig() (storage.Config, error) {
	switch cfg.Kind {
	case storage.KindBadger:
		return storage.BadgerConfig{Dir: os.ExpandEnv(cfg.Dir)}, nil
	case storage.KindMemory:
		return storage.MemoryConfig{}, nil
	case storage.KindFilesystemDir:
		return storage.FilesystemDirConfig{Dir: os.ExpandEnv(cfg.Dir)}, nil
	case storage.KindSQLite:
		return storage.SQLiteConfig{Dir: os.ExpandEnv(cfg.Dir), DSN: os.ExpandEnv(cfg.DSN)}, nil
	case storage.KindPostgreSQL:
		return storage.PostgreSQLConfig{DSN: os.ExpandEnv(cfg.DSN)}, nil
	case storage.KindClickHouse:
		return storage.ClickHouseConfig{DSN: os.ExpandEnv(cfg.DSN)}, nil
	case storage.KindRedis:
		return storage.RedisConfig{URL: os.ExpandEnv(cfg.URL), TLSCAFile: os.ExpandEnv(cfg.TLSCAFile)}, nil
	case storage.KindPrometheus:
		return storage.PrometheusConfig{
			RemoteWriteURL: os.ExpandEnv(cfg.RemoteWriteURL),
			QueryURL:       os.ExpandEnv(cfg.QueryURL),
			BearerToken:    os.ExpandEnv(cfg.BearerToken),
		}, nil
	case storage.KindVolcTLS:
		return storage.VolcTLSConfig{
			Endpoint:        os.ExpandEnv(cfg.Endpoint),
			Region:          os.ExpandEnv(cfg.Region),
			AccessKeyID:     os.ExpandEnv(cfg.AccessKeyID),
			AccessKeySecret: os.ExpandEnv(cfg.AccessKeySecret),
		}, nil
	case storage.KindVolcTOS:
		return storage.VolcTOSConfig{
			Endpoint: os.ExpandEnv(cfg.Endpoint), Region: os.ExpandEnv(cfg.Region), Bucket: os.ExpandEnv(cfg.Bucket),
			AccessKeyID: os.ExpandEnv(cfg.AccessKeyID), AccessKeySecret: os.ExpandEnv(cfg.AccessKeySecret),
			SessionToken: os.ExpandEnv(cfg.SessionToken),
		}, nil
	case storage.KindAliyunOSS:
		return storage.AliyunOSSConfig{
			Endpoint: os.ExpandEnv(cfg.Endpoint), Bucket: os.ExpandEnv(cfg.Bucket),
			AccessKeyID: os.ExpandEnv(cfg.AccessKeyID), AccessKeySecret: os.ExpandEnv(cfg.AccessKeySecret),
			SecurityToken: os.ExpandEnv(cfg.SecurityToken),
		}, nil
	case storage.KindGCS:
		return storage.GCSConfig{Bucket: os.ExpandEnv(cfg.Bucket), CredentialsFile: os.ExpandEnv(cfg.CredentialsFile)}, nil
	case storage.KindAzureBlob:
		return storage.AzureBlobConfig{AccountURL: os.ExpandEnv(cfg.AccountURL), Container: os.ExpandEnv(cfg.Container)}, nil
	default:
		return nil, fmt.Errorf("server: unknown storage kind %q", cfg.Kind)
	}
}

type storeFileConfig struct {
	Kind     string `yaml:"kind"`
	Storage  string `yaml:"storage"`
	Prefix   string `yaml:"prefix"`
	Database string `yaml:"database"`
	Table    string `yaml:"table"`
	TopicID  string `yaml:"topic_id"`
	TTL      string `yaml:"ttl"`
}

func (cfg storeFileConfig) runtimeConfig() (store.Config, error) {
	var ttl time.Duration
	if strings.TrimSpace(cfg.TTL) != "" {
		parsed, err := time.ParseDuration(cfg.TTL)
		if err != nil || parsed <= 0 {
			return store.Config{}, fmt.Errorf("ttl must be a positive duration")
		}
		ttl = parsed
	}
	return store.Config{
		Kind: cfg.Kind, Storage: cfg.Storage, Prefix: cfg.Prefix,
		Database: os.ExpandEnv(cfg.Database), Table: os.ExpandEnv(cfg.Table), TopicID: os.ExpandEnv(cfg.TopicID),
		TTL: ttl,
	}, nil
}

type ConfigFile struct {
	Identity        IdentityConfig               `yaml:"identity"`
	WebRTC          *WebRTCConfig                `yaml:"webrtc"`
	HTTP            *HTTPConfig                  `yaml:"http"`
	EdgeNodes       []giznet.PublicKey           `yaml:"edge-nodes"`
	ICEServers      []gizwebrtc.ICEServer        `yaml:"ice-servers"`
	AdminPublicKey  giznet.PublicKey             `yaml:"admin-public-key"`
	Storage         map[string]storageFileConfig `yaml:"storage"`
	Stores          map[string]storeFileConfig   `yaml:"stores"`
	Services        *ServicesConfig              `yaml:"services"`
	Friends         FriendsConfig                `yaml:"friends"`
	FriendGroups    FriendGroupsConfig           `yaml:"friend_groups"`
	Speech          SpeechConfig                 `yaml:"speech"`
	PendingDeletion PendingDeletionConfig        `yaml:"pending_deletion"`
	Profiling       ProfilingConfig              `yaml:"profiling"`
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
	for _, retired := range []string{"serving-public", "serve-to-clients"} {
		if _, exists := topLevel[retired]; exists {
			return ConfigFile{}, fmt.Errorf("server: %s was removed; public client APIs are Edge-only", retired)
		}
	}
	for _, retired := range []string{"listen", "endpoint"} {
		if _, exists := topLevel[retired]; exists {
			return ConfigFile{}, fmt.Errorf("server: top-level %s was removed; use webrtc.%s", retired, retired)
		}
	}
	if _, exists := topLevel["system_tasks"]; exists {
		return ConfigFile{}, fmt.Errorf("server: system_tasks is not supported; configure Pet model aliases in the RuntimeProfile")
	}
	var raw struct {
		Identity        *IdentityConfig              `yaml:"identity"`
		WebRTC          *WebRTCConfig                `yaml:"webrtc"`
		HTTP            *HTTPConfig                  `yaml:"http"`
		EdgeNodes       []giznet.PublicKey           `yaml:"edge-nodes"`
		ICEServers      []gizwebrtc.ICEServer        `yaml:"ice-servers"`
		AdminPublicKey  *giznet.PublicKey            `yaml:"admin-public-key"`
		Storage         map[string]storageFileConfig `yaml:"storage"`
		Stores          map[string]storeFileConfig   `yaml:"stores"`
		Services        *ServicesConfig              `yaml:"services"`
		Friends         FriendsConfig                `yaml:"friends"`
		FriendGroups    FriendGroupsConfig           `yaml:"friend_groups"`
		Speech          speechFileConfig             `yaml:"speech"`
		PendingDeletion pendingDeletionFileConfig    `yaml:"pending_deletion"`
		Profiling       ProfilingConfig              `yaml:"profiling"`
	}
	if err := yaml.UnmarshalWithOptions(data, &raw, yaml.DisallowUnknownField()); err != nil {
		return ConfigFile{}, err
	}
	adminPublicKey, err := resolveAdminPublicKey(raw.AdminPublicKey)
	if err != nil {
		return ConfigFile{}, err
	}
	if raw.Services != nil && raw.Services.SystemLog != nil {
		logCfg, err := gizlog.PrepareConfig(*raw.Services.SystemLog)
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
		WebRTC:          raw.WebRTC,
		HTTP:            raw.HTTP,
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
		Profiling:       raw.Profiling,
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
		WebRTC: WebRTCConfig{Listen: "0.0.0.0:9820", Endpoint: "0.0.0.0:9820"},
		HTTP:   HTTPConfig{Listeners: []HTTPListenerConfig{{Listen: "0.0.0.0:9820"}}},
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
	if cfg.WebRTC == (WebRTCConfig{}) && fileCfg.WebRTC != nil {
		cfg.WebRTC = *fileCfg.WebRTC
	}
	if fileCfg.HTTP != nil && len(fileCfg.HTTP.Listeners) == 0 {
		return Config{}, fmt.Errorf("server: http.listeners must not be empty")
	}
	if len(cfg.HTTP.Listeners) == 0 && fileCfg.HTTP != nil {
		cfg.HTTP.Listeners = append([]HTTPListenerConfig(nil), fileCfg.HTTP.Listeners...)
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
		var err error
		cfg.Stores, err = runtimeStoreConfigs(fileCfg.Stores)
		if err != nil {
			return Config{}, err
		}
	}
	if len(cfg.Storage) == 0 {
		var err error
		cfg.Storage, err = runtimeStorageConfigs(fileCfg.Storage)
		if err != nil {
			return Config{}, err
		}
	}
	if cfg.Services == nil {
		cfg.Services = fileCfg.Services
	}
	cfg.Friends = mergeFriendsConfig(cfg.Friends, fileCfg.Friends)
	cfg.FriendGroups = mergeFriendGroupsConfig(cfg.FriendGroups, fileCfg.FriendGroups)
	cfg.Speech = mergeSpeechConfig(cfg.Speech, fileCfg.Speech)
	cfg.PendingDeletion = mergePendingDeletionConfig(cfg.PendingDeletion, fileCfg.PendingDeletion)
	if !cfg.Profiling.Enabled && cfg.Profiling.Store == "" {
		cfg.Profiling = fileCfg.Profiling
	}
	return cfg, nil
}

func runtimeStorageConfigs(configs map[string]storageFileConfig) (map[string]storage.Config, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	out := make(map[string]storage.Config, len(configs))
	for name, cfg := range configs {
		runtime, err := cfg.runtimeConfig()
		if err != nil {
			return nil, fmt.Errorf("server: storage.%s: %w", name, err)
		}
		out[name] = runtime
	}
	return out, nil
}

func runtimeStoreConfigs(configs map[string]storeFileConfig) (map[string]store.Config, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	out := make(map[string]store.Config, len(configs))
	for name, cfg := range configs {
		runtime, err := cfg.runtimeConfig()
		if err != nil {
			return nil, fmt.Errorf("server: stores.%s: %w", name, err)
		}
		out[name] = runtime
	}
	return out, nil
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
	if cfg.WebRTC.Listen == "" {
		return Config{}, fmt.Errorf("server: webrtc.listen is required")
	}
	if cfg.WebRTC.Endpoint == "" {
		return Config{}, fmt.Errorf("server: webrtc.endpoint is required")
	}
	if len(cfg.HTTP.Listeners) == 0 {
		return Config{}, fmt.Errorf("server: http.listeners is required")
	}
	for index := range cfg.HTTP.Listeners {
		cfg.HTTP.Listeners[index].TLS.CertFile = os.ExpandEnv(cfg.HTTP.Listeners[index].TLS.CertFile)
		cfg.HTTP.Listeners[index].TLS.KeyFile = os.ExpandEnv(cfg.HTTP.Listeners[index].TLS.KeyFile)
	}
	cfg.Speech = mergeSpeechConfig(cfg.Speech, defaults.Speech)
	cfg.PendingDeletion = mergePendingDeletionConfig(cfg.PendingDeletion, defaults.PendingDeletion)
	if cfg.Services != nil && cfg.Services.SystemLog != nil {
		logCfg, err := gizlog.PrepareConfig(*cfg.Services.SystemLog)
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
	if err := validateHostPort("webrtc.listen", cfg.WebRTC.Listen); err != nil {
		return err
	}
	if err := validateHostPort("webrtc.endpoint", cfg.WebRTC.Endpoint); err != nil {
		return err
	}
	httpListeners := cfg.HTTP.Listeners
	if len(httpListeners) == 0 {
		return fmt.Errorf("server: http.listeners is required")
	}
	if httpListeners[0].Listen != cfg.WebRTC.Listen {
		return fmt.Errorf("server: http.listeners[0].listen must match webrtc.listen")
	}
	seenHTTPListeners := make(map[string]int, len(httpListeners))
	for index, listener := range httpListeners {
		path := fmt.Sprintf("http.listeners[%d]", index)
		if err := validateHostPort(path+".listen", listener.Listen); err != nil {
			return err
		}
		if previous, ok := seenHTTPListeners[listener.Listen]; ok {
			return fmt.Errorf("server: %s.listen duplicates http.listeners[%d]", path, previous)
		}
		seenHTTPListeners[listener.Listen] = index
		if _, err := listener.TLS.tlsConfig(path + ".tls"); err != nil {
			return err
		}
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
	if cfg.Services.Peer.Store == cfg.Services.PeerRun.Store {
		return fmt.Errorf("server: services.peer_run.store must be separate from services.peer.store")
	}
	if err := validateProfilingConfig(cfg.Profiling, cfg.Services); err != nil {
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

func validateProfilingConfig(cfg ProfilingConfig, services *ServicesConfig) error {
	storeName := strings.TrimSpace(cfg.Store)
	if cfg.Store != storeName {
		return fmt.Errorf("server: profiling.store must not have leading or trailing whitespace")
	}
	if cfg.Enabled && storeName == "" {
		return fmt.Errorf("server: profiling.store is required when profiling.enabled is true")
	}
	if storeName == "" || services == nil {
		return nil
	}
	for path, businessStore := range map[string]string{
		"services.workspace.assets_store": services.Workspace.AssetsStore,
		"services.gameplay.assets_store":  services.Gameplay.AssetsStore,
		"services.agent_host.runtime_store": func() string {
			if services.AgentHost == nil {
				return ""
			}
			return services.AgentHost.RuntimeStore
		}(),
	} {
		if storeName == businessStore {
			return fmt.Errorf("server: profiling.store %q must be dedicated and cannot reuse %s", storeName, path)
		}
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
		{"services.peer_run", cfg.PeerRun != nil},
		{"services.api_key", cfg.APIKey != nil},
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
		reference{"services.peer_run.store", cfg.PeerRun.Store},
		reference{"services.api_key.store", cfg.APIKey.Store},
		reference{"services.credential.store", cfg.Credential.Store},
		reference{"services.firmware.store", cfg.Firmware.Store},
		reference{"services.runtime_profile.store", cfg.RuntimeProfile.Store},
		reference{"services.model.store", cfg.Model.Store},
		reference{"services.voice.store", cfg.Voice.Store},
		reference{"services.memory_layout.store", cfg.MemoryLayout.Store},
		reference{"services.provider_tenants.store", cfg.ProviderTenants.Store},
		reference{"services.workflow.store", cfg.Workflow.Store},
		reference{"services.workspace.store", cfg.Workspace.Store},
		reference{"services.workspace.history_store", cfg.Workspace.HistoryStore},
		reference{"services.workspace.history_assets_store", cfg.Workspace.HistoryAssetsStore},
		reference{"services.workspace.assets_store", cfg.Workspace.AssetsStore},
		reference{"services.toolkit.store", cfg.Toolkit.Store},
		reference{"services.contact.store", cfg.Contact.Store},
		reference{"services.friend.store", cfg.Friend.Store},
		reference{"services.friend_group.store", cfg.FriendGroup.Store},
		reference{"services.gameplay.store", cfg.Gameplay.Store},
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
	if err := cfg.SFU.validate(); err != nil {
		return err
	}
	if cfg.SystemLog != nil {
		if _, err := gizlog.PrepareConfig(*cfg.SystemLog); err != nil {
			return fmt.Errorf("server: %w", err)
		}
	}
	return nil
}

func (cfg Config) systemLogConfig() gizlog.Config {
	if cfg.Services == nil || cfg.Services.SystemLog == nil {
		return gizlog.DefaultConfig()
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
	if value, exists := document["profiling"]; exists {
		mapping, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("server: profiling must be a mapping")
		}
		for field := range mapping {
			switch field {
			case "enabled", "store":
			default:
				return fmt.Errorf("server: profiling has unknown field %q", field)
			}
		}
	}
	if value, exists := document["storage"]; exists {
		if err := validateStorageConfigShape(value); err != nil {
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
		if kind == "log" {
			return fmt.Errorf("server: stores.%s kind %q is not supported; use %s or %s", name, kind, store.KindLogImmutable, store.KindLogMutable)
		}
		allowedFields := map[string]map[string]struct{}{
			store.KindKeyValue:     {"kind": {}, "storage": {}, "prefix": {}},
			store.KindObjectStore:  {"kind": {}, "storage": {}, "prefix": {}, "ttl": {}},
			store.KindSQL:          {"kind": {}, "storage": {}},
			store.KindMetrics:      {"kind": {}, "storage": {}, "table": {}, "database": {}},
			store.KindLogImmutable: {"kind": {}, "storage": {}, "topic_id": {}, "database": {}, "table": {}, "ttl": {}},
			store.KindLogMutable:   {"kind": {}, "storage": {}, "database": {}, "table": {}, "ttl": {}},
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
	}
	return nil
}

func validateStorageConfigShape(value any) error {
	registry, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("server: storage must be a mapping")
	}
	allowedByKind := map[string]map[string]struct{}{
		storage.KindBadger:        {"kind": {}, "dir": {}},
		storage.KindMemory:        {"kind": {}},
		storage.KindFilesystemDir: {"kind": {}, "dir": {}},
		storage.KindSQLite:        {"kind": {}, "dir": {}, "dsn": {}},
		storage.KindPostgreSQL:    {"kind": {}, "dsn": {}},
		storage.KindClickHouse:    {"kind": {}, "dsn": {}},
		storage.KindRedis:         {"kind": {}, "url": {}, "tls_ca_file": {}},
		storage.KindPrometheus:    {"kind": {}, "remote_write_url": {}, "query_url": {}, "bearer_token": {}},
		storage.KindVolcTLS:       {"kind": {}, "endpoint": {}, "region": {}, "access_key_id": {}, "access_key_secret": {}},
		storage.KindVolcTOS:       {"kind": {}, "endpoint": {}, "region": {}, "bucket": {}, "access_key_id": {}, "access_key_secret": {}, "session_token": {}},
		storage.KindAliyunOSS:     {"kind": {}, "endpoint": {}, "bucket": {}, "access_key_id": {}, "access_key_secret": {}, "security_token": {}},
		storage.KindGCS:           {"kind": {}, "bucket": {}, "credentials_file": {}},
		storage.KindAzureBlob:     {"kind": {}, "account_url": {}, "container": {}},
	}
	for name, entry := range registry {
		mapping, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("server: storage.%s must be a mapping", name)
		}
		kind := fmt.Sprint(mapping["kind"])
		allowed, known := allowedByKind[kind]
		if !known {
			continue
		}
		for field := range mapping {
			if _, ok := allowed[field]; !ok {
				return fmt.Errorf("server: storage.%s field %q is invalid for kind %s", name, field, kind)
			}
		}
	}
	return nil
}

func validateServicesConfigShape(services map[string]any) error {
	stringFields := map[string][]string{
		"peer":             {"store"},
		"peer_run":         {"store"},
		"api_key":          {"store"},
		"credential":       {"store"},
		"firmware":         {"store"},
		"runtime_profile":  {"store"},
		"model":            {"store"},
		"voice":            {"store"},
		"memory_layout":    {"store"},
		"provider_tenants": {"store"},
		"workflow":         {"store"},
		"workspace":        {"store", "history_store", "history_assets_store", "assets_store"},
		"toolkit":          {"store"},
		"contact":          {"store"},
		"friend":           {"store"},
		"friend_group":     {"store"},
		"gameplay":         {"store", "assets_store", "database_store"},
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
	if value, exists := services["sfu"]; exists && value != nil {
		if err := validateSFUConfigShape("services.sfu", value); err != nil {
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
	for _, field := range []string{"level", "node_id", "query_store"} {
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

func validateSFUConfigShape(path string, value any) error {
	mapping, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("server: %s must be a mapping", path)
	}
	for field := range mapping {
		switch field {
		case "url", "api_key_file", "api_secret_file", "recheck_interval", "reconnect_timeout", "talk_hangover", "floor_idle":
		default:
			return fmt.Errorf("server: %s has unknown field %q; credentials are loaded from api_key_file and api_secret_file", path, field)
		}
	}
	for _, field := range []string{"url", "api_key_file", "api_secret_file"} {
		if reference, exists := mapping[field]; exists {
			if err := validateFileStoreReference(path+"."+field, reference); err != nil {
				return err
			}
		}
	}
	for _, field := range []string{"recheck_interval", "reconnect_timeout", "talk_hangover", "floor_idle"} {
		if reference, exists := mapping[field]; exists {
			if _, ok := reference.(string); !ok {
				return fmt.Errorf("server: %s.%s must be a string", path, field)
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
	return cfg.WebRTC.Listen
}

func (cfg Config) ICEListenAddr() string {
	return cfg.WebRTC.Listen
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
