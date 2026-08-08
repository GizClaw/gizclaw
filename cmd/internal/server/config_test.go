package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging"
	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
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

type closedPeerListener struct{}

func (closedPeerListener) Accept() (giznet.Conn, error) { return nil, giznet.ErrClosed }
func (closedPeerListener) Close() error                 { return nil }

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
	if cfg.systemLogConfig().Level != "info" {
		t.Fatalf("SystemLog.Level = %q, want info", cfg.systemLogConfig().Level)
	}
	if cfg.Speech.Transcription.MaxAudioBytes != 2*1024*1024 ||
		cfg.Speech.Transcription.MaxAudioDuration != "60s" ||
		cfg.Speech.Transcription.RequestTimeout != "75s" {
		t.Fatalf("Speech.Transcription = %+v", cfg.Speech.Transcription)
	}
	if cfg.Speech.Synthesis.MaxTextBytes != 4096 ||
		cfg.Speech.Synthesis.MaxOutputBytes != 4*1024*1024 ||
		cfg.Speech.Synthesis.RequestTimeout != "120s" {
		t.Fatalf("Speech.Synthesis = %+v", cfg.Speech.Synthesis)
	}
	if cfg.PendingDeletion.ScanInterval != "30s" || cfg.PendingDeletion.PageSize != 100 ||
		cfg.PendingDeletion.DispatchCapacity != 256 || cfg.PendingDeletion.Workers != 4 ||
		cfg.PendingDeletion.LeaseDuration != "2m" || cfg.PendingDeletion.AttemptTimeout != "90s" ||
		cfg.PendingDeletion.RetryInitial != "5s" || cfg.PendingDeletion.RetryMax != "30m" ||
		cfg.PendingDeletion.MaxAttempts != 10 {
		t.Fatalf("PendingDeletion = %+v", cfg.PendingDeletion)
	}
}

func TestParsePendingDeletionConfig(t *testing.T) {
	file, err := parseConfigData([]byte(`pending_deletion:
  scan_interval: 2s
  page_size: 25
  workers: 2
`))
	if err != nil {
		t.Fatalf("parseConfigData() error = %v", err)
	}
	merged, err := mergeFileConfig(Config{}, file)
	if err != nil {
		t.Fatalf("mergeFileConfig() error = %v", err)
	}
	merged.Services = validServicesConfig()
	prepared, err := prepareConfig(merged)
	if err != nil {
		t.Fatalf("prepareConfig() error = %v", err)
	}
	if prepared.PendingDeletion.ScanInterval != "2s" || prepared.PendingDeletion.PageSize != 25 ||
		prepared.PendingDeletion.Workers != 2 || prepared.PendingDeletion.LeaseDuration != "2m" {
		t.Fatalf("PendingDeletion = %+v", prepared.PendingDeletion)
	}
}

func TestPendingDeletionConfigRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name string
		yaml string
		want string
	}{
		{"unknown", "pending_deletion:\n  retention: 1h\n", "unknown field"},
		{"zero workers", "pending_deletion:\n  workers: 0\n", "workers must be between"},
		{"zero duration", "pending_deletion:\n  scan_interval: 0s\n", "scan_interval"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseConfigData([]byte(test.yaml))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseConfigData() error = %v, want %q", err, test.want)
			}
		})
	}
	cfg := DefaultConfig()
	cfg.Services = validServicesConfig()
	cfg.PendingDeletion.AttemptTimeout = "2m"
	if _, err := prepareConfig(cfg); err == nil || !strings.Contains(err.Error(), "attempt timeout must be shorter") {
		t.Fatalf("prepareConfig() error = %v", err)
	}
}

func TestParseConfigAgentHostPresence(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		check func(*testing.T, *AgentHostConfig)
	}{
		{
			name: "absent",
			yaml: "{}",
			check: func(t *testing.T, cfg *AgentHostConfig) {
				t.Helper()
				if cfg != nil {
					t.Fatalf("AgentHost = %+v, want nil", cfg)
				}
			},
		},
		{
			name: "present empty",
			yaml: "services:\n  agent_host: {}",
			check: func(t *testing.T, cfg *AgentHostConfig) {
				t.Helper()
				if cfg == nil {
					t.Fatal("AgentHost = nil, want present block")
				}
				if cfg.RuntimeStore != "" || cfg.Flowcraft != nil {
					t.Fatalf("AgentHost = %+v, want empty block", cfg)
				}
			},
		},
		{
			name: "present partial",
			yaml: "services:\n  agent_host:\n    flowcraft:\n      state_store: state\n",
			check: func(t *testing.T, cfg *AgentHostConfig) {
				t.Helper()
				if cfg == nil || cfg.Flowcraft == nil || cfg.Flowcraft.StateStore != "state" {
					t.Fatalf("AgentHost = %+v", cfg)
				}
				if cfg.RuntimeStore != "" || cfg.Flowcraft.HistoryStore != "" {
					t.Fatalf("AgentHost partial fields = %+v", cfg)
				}
			},
		},
		{
			name: "complete",
			yaml: `
services:
  agent_host:
    runtime_store: runtime
    flowcraft:
      state_store: state
      history_store: history
`,
			check: func(t *testing.T, cfg *AgentHostConfig) {
				t.Helper()
				if cfg == nil || cfg.Flowcraft == nil {
					t.Fatalf("AgentHost = %+v", cfg)
				}
				if cfg.RuntimeStore != "runtime" ||
					cfg.Flowcraft.StateStore != "state" ||
					cfg.Flowcraft.HistoryStore != "history" {
					t.Fatalf("AgentHost = %+v", cfg)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := parseConfigData([]byte(test.yaml))
			if err != nil {
				t.Fatalf("parseConfigData() error = %v", err)
			}
			if cfg.Services == nil {
				test.check(t, nil)
			} else {
				test.check(t, cfg.Services.AgentHost)
			}
		})
	}
}

func TestParseConfigRejectsInvalidAgentHost(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"legacy top-level", "agent_host: {}\n", `unknown field "agent_host"`},
		{"unknown field", "services:\n  agent_host:\n    runtime: store\n", `unknown field "runtime"`},
		{"non-string runtime", "services:\n  agent_host:\n    runtime_store: 42\n", "services.agent_host.runtime_store must be a string"},
		{"unknown flowcraft field", "services:\n  agent_host:\n    flowcraft:\n      memories: memory\n", `unknown field "memories"`},
		{"non-string state", "services:\n  agent_host:\n    flowcraft:\n      state_store: {}\n", "services.agent_host.flowcraft.state_store must be a string"},
		{"legacy memory objects", "services:\n  agent_host:\n    flowcraft:\n      memory_objects_store: old\n", `unknown field "memory_objects_store"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseConfigData([]byte(test.yaml))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseConfigData() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseConfigRejectsNonStringServiceStoreReferences(t *testing.T) {
	tests := []struct {
		yaml string
		want string
	}{
		{"services:\n  peer:\n    store: 42\n", "services.peer.store must be a string"},
		{"services:\n  provider_tenants:\n    model_store: []\n", "services.provider_tenants.model_store must be a string"},
		{"services:\n  metrics:\n    store: true\n", "services.metrics.store must be a string"},
		{"services:\n  system_log:\n    query_store: 42\n", "services.system_log.query_store must be a string"},
		{"services:\n  system_log:\n    sinks:\n      - kind: store\n        store: {}\n", "services.system_log.sinks[0].store must be a string"},
		{"services:\n  workspace: named-store\n", "services.workspace must be a mapping"},
	}
	for _, test := range tests {
		_, err := parseConfigData([]byte(test.yaml))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("parseConfigData(%q) error = %v, want %q", test.yaml, err, test.want)
		}
	}
}

func TestMergeFileConfigAgentHostBlock(t *testing.T) {
	fileBlock := &AgentHostConfig{
		RuntimeStore: "file-runtime",
		Flowcraft: &AgentHostFlowcraftConfig{
			StateStore:   "file-state",
			HistoryStore: "file-history",
		},
	}
	fileServices := &ServicesConfig{AgentHost: fileBlock}
	retained, err := mergeFileConfig(Config{}, ConfigFile{Services: fileServices})
	if err != nil {
		t.Fatalf("mergeFileConfig(retain) error = %v", err)
	}
	if retained.Services != fileServices {
		t.Fatalf("mergeFileConfig(retain) Services = %+v", retained.Services)
	}

	runtimeBlock := &AgentHostConfig{Flowcraft: &AgentHostFlowcraftConfig{StateStore: "runtime-state"}}
	runtimeServices := &ServicesConfig{AgentHost: runtimeBlock}
	replaced, err := mergeFileConfig(Config{Services: runtimeServices}, ConfigFile{Services: fileServices})
	if err != nil {
		t.Fatalf("mergeFileConfig(replace) error = %v", err)
	}
	if replaced.Services != runtimeServices {
		t.Fatalf("mergeFileConfig(replace) Services = %+v, want runtime block", replaced.Services)
	}
	if replaced.Services.AgentHost.RuntimeStore != "" || replaced.Services.AgentHost.Flowcraft.HistoryStore != "" {
		t.Fatalf("mergeFileConfig(replace) field-merged blocks: %+v", replaced.Services)
	}
}

func TestValidateAgentHostRejectsProgrammaticWhitespaceReference(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Services = validServicesConfig()
	cfg.Services.AgentHost = &AgentHostConfig{Flowcraft: &AgentHostFlowcraftConfig{StateStore: " "}}
	err := cfg.validate()
	if err == nil || !strings.Contains(err.Error(), "services.agent_host.flowcraft.state_store must not be whitespace-only") {
		t.Fatalf("validate() error = %v", err)
	}
}

func TestParseConfigSpeechLimits(t *testing.T) {
	cfg, err := parseConfigData([]byte(`
speech:
  transcription:
    max_audio_bytes: 1024
    max_audio_duration: 3s
    request_timeout: 4s
  extraction:
    max_schema_bytes: 4096
    max_schema_depth: 8
    max_schema_properties: 32
    max_instruction_bytes: 1024
    max_result_bytes: 4096
    request_timeout: 6s
  synthesis:
    max_text_bytes: 512
    max_output_bytes: 2048
    request_timeout: 5s
`))
	if err != nil {
		t.Fatalf("parseConfigData error = %v", err)
	}
	if cfg.Speech.Transcription.MaxAudioBytes != 1024 ||
		cfg.Speech.Transcription.MaxAudioDuration != "3s" ||
		cfg.Speech.Transcription.RequestTimeout != "4s" ||
		cfg.Speech.Extraction.MaxSchemaBytes != 4096 ||
		cfg.Speech.Extraction.MaxSchemaDepth != 8 ||
		cfg.Speech.Extraction.MaxSchemaProperties != 32 ||
		cfg.Speech.Extraction.MaxInstructionBytes != 1024 ||
		cfg.Speech.Extraction.MaxResultBytes != 4096 ||
		cfg.Speech.Extraction.RequestTimeout != "6s" ||
		cfg.Speech.Synthesis.MaxTextBytes != 512 ||
		cfg.Speech.Synthesis.MaxOutputBytes != 2048 ||
		cfg.Speech.Synthesis.RequestTimeout != "5s" {
		t.Fatalf("Speech = %+v", cfg.Speech)
	}
}

func TestValidateSpeechLimits(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"audio bytes", func(c *Config) { c.Speech.Transcription.MaxAudioBytes = -1 }, "server: speech.transcription.max_audio_bytes must be > 0"},
		{"audio duration", func(c *Config) { c.Speech.Transcription.MaxAudioDuration = "0s" }, "server: speech.transcription.max_audio_duration: must be > 0"},
		{"transcription timeout", func(c *Config) { c.Speech.Transcription.RequestTimeout = "later" }, "server: speech.transcription.request_timeout: time: invalid duration \"later\""},
		{"schema bytes", func(c *Config) { c.Speech.Extraction.MaxSchemaBytes = 16385 }, "server: speech.extraction.max_schema_bytes must be between 1 and 16384"},
		{"schema depth", func(c *Config) { c.Speech.Extraction.MaxSchemaDepth = 17 }, "server: speech.extraction.max_schema_depth must be between 1 and 16"},
		{"schema properties", func(c *Config) { c.Speech.Extraction.MaxSchemaProperties = 129 }, "server: speech.extraction.max_schema_properties must be between 1 and 128"},
		{"instruction bytes", func(c *Config) { c.Speech.Extraction.MaxInstructionBytes = 4097 }, "server: speech.extraction.max_instruction_bytes must be between 1 and 4096"},
		{"result bytes", func(c *Config) { c.Speech.Extraction.MaxResultBytes = 0 }, "server: speech.extraction.max_result_bytes must be between 1 and 16384"},
		{"extraction timeout", func(c *Config) { c.Speech.Extraction.RequestTimeout = "later" }, "server: speech.extraction.request_timeout: time: invalid duration \"later\""},
		{"extraction timeout maximum", func(c *Config) { c.Speech.Extraction.RequestTimeout = "121s" }, "server: speech.extraction.request_timeout must be at most 2m0s"},
		{"text bytes", func(c *Config) { c.Speech.Synthesis.MaxTextBytes = 0 }, "server: speech.synthesis.max_text_bytes must be > 0"},
		{"output bytes", func(c *Config) { c.Speech.Synthesis.MaxOutputBytes = -1 }, "server: speech.synthesis.max_output_bytes must be > 0"},
		{"synthesis timeout", func(c *Config) { c.Speech.Synthesis.RequestTimeout = "0s" }, "server: speech.synthesis.request_timeout: must be > 0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultConfig()
			test.edit(&cfg)
			err := cfg.validate()
			if err == nil || err.Error() != test.want {
				t.Fatalf("validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseConfigRejectsExplicitInvalidSpeechLimits(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"zero audio bytes", "speech:\n  transcription:\n    max_audio_bytes: 0\n", "server: speech.transcription.max_audio_bytes must be > 0"},
		{"zero schema bytes", "speech:\n  extraction:\n    max_schema_bytes: 0\n", "server: speech.extraction.max_schema_bytes must be between 1 and 16384"},
		{"oversized result", "speech:\n  extraction:\n    max_result_bytes: 16385\n", "server: speech.extraction.max_result_bytes must be between 1 and 16384"},
		{"zero extraction timeout", "speech:\n  extraction:\n    request_timeout: 0s\n", "server: speech.extraction.request_timeout: must be > 0"},
		{"oversized extraction timeout", "speech:\n  extraction:\n    request_timeout: 121s\n", "server: speech.extraction.request_timeout must be at most 2m0s"},
		{"zero text bytes", "speech:\n  synthesis:\n    max_text_bytes: 0\n", "server: speech.synthesis.max_text_bytes must be > 0"},
		{"empty transcription timeout", "speech:\n  transcription:\n    request_timeout: \"\"\n", "server: speech.transcription.request_timeout: time: invalid duration \"\""},
		{"zero synthesis timeout", "speech:\n  synthesis:\n    request_timeout: 0s\n", "server: speech.synthesis.request_timeout: must be > 0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseConfigData([]byte(test.yaml))
			if err == nil || err.Error() != test.want {
				t.Fatalf("parseConfigData() error = %v, want %q", err, test.want)
			}
		})
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

func TestParseConfigRejectsServingPublic(t *testing.T) {
	_, err := parseConfigData([]byte(`
serving-public: true
listen: 127.0.0.1:9820
endpoint: 127.0.0.1:9820
`))
	if err == nil || !strings.Contains(err.Error(), "serving-public is not supported") {
		t.Fatalf("parseConfigData error = %v, want unsupported alias error", err)
	}
}

func TestParseConfigRejectsLegacySystemTasks(t *testing.T) {
	_, err := parseConfigData([]byte(`
system_tasks:
  pet_flowcraft_workflow:
    generate_model: legacy
`))
	if err == nil || !strings.Contains(err.Error(), "system_tasks is not supported") {
		t.Fatalf("parseConfigData() error = %v", err)
	}
}

func TestParseConfigRejectsLegacyFriendGroupMessageSettings(t *testing.T) {
	_, err := parseConfigData([]byte("friend_groups:\n  message_default_ttl: 24h\n"))
	if err == nil || !strings.Contains(err.Error(), "message_default_ttl") {
		t.Fatalf("parseConfigData() error = %v, want retired Friend Group message setting rejection", err)
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
	if srv.AgentHostStore == nil {
		t.Fatalf("agenthost store not wired: %+v", srv.Server)
	}
	if srv.ContactStore == nil || srv.FriendInviteTokenStore == nil || srv.FriendStore == nil || srv.FriendGroupStore == nil || srv.FriendGroupInviteTokenStore == nil || srv.FriendGroupMemberStore == nil {
		t.Fatalf("social stores not wired: %+v", srv.Server)
	}
	if srv.PetDefStore == nil || srv.BadgeDefStore == nil || srv.GameDefStore == nil || srv.GameplayAssets == nil || srv.WorkspaceAssets == nil || srv.GameplayDB == nil {
		t.Fatalf("gameplay stores not wired: %+v", srv.Server)
	}
}

func TestNewPreservesPostgresDialectThroughLayeredStorage(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	cfg := validLayeredConfig(t.TempDir())
	cfg.Storage["gameplay-db"] = storage.Config{Kind: storage.KindSQL, Postgres: &storage.SQLConfig{DSN: dsn}}

	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	for name, db := range map[string]interface {
		DriverName() string
		Rebind(string) string
	}{
		"gameplay": srv.GameplayDB,
	} {
		if db == nil {
			t.Fatalf("%s DB = nil", name)
		}
		if got := db.DriverName(); got != "postgres" {
			t.Fatalf("%s DriverName() = %q, want postgres", name, got)
		}
		if got := db.Rebind("SELECT ?"); got != "SELECT $1" {
			t.Fatalf("%s Rebind() = %q, want %q", name, got, "SELECT $1")
		}
	}
}

func TestNewWithLayeredStorageReportsStoreErrors(t *testing.T) {
	dir := t.TempDir()

	storageErrCfg := validLayeredConfig(dir)
	storageErrCfg.Storage["memory"] = storage.Config{Kind: storage.KindKeyValue}
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
	if _, err := New(missingCredentialCfg); err == nil || !strings.Contains(err.Error(), "services.credential.store") {
		t.Fatalf("New(missing credentials store) = %v", err)
	}

	missingFirmwareCfg := validLayeredConfig(dir)
	delete(missingFirmwareCfg.Stores, "firmwares")
	if _, err := New(missingFirmwareCfg); err == nil || !strings.Contains(err.Error(), "services.firmware.store") {
		t.Fatalf("New(missing firmwares store) = %v", err)
	}

	badAgentHostCfg := validLayeredConfig(dir)
	badAgentHostCfg.Stores["agenthost"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "agenthost"}
	if _, err := New(badAgentHostCfg); err == nil || !strings.Contains(err.Error(), `agent_host.runtime_store "agenthost" requires objectstore.ObjectStore`) {
		t.Fatalf("New(bad agenthost store) = %v", err)
	}

	missingTenantCfg := validLayeredConfig(dir)
	delete(missingTenantCfg.Stores, "minimax-tenants")
	if _, err := New(missingTenantCfg); err == nil || !strings.Contains(err.Error(), "services.provider_tenants.minimax_tenant_store") {
		t.Fatalf("New(missing tenant store) = %v", err)
	}

	missingVoicesCfg := validLayeredConfig(dir)
	delete(missingVoicesCfg.Stores, "voices")
	if _, err := New(missingVoicesCfg); err == nil || !strings.Contains(err.Error(), "services.voice.store") {
		t.Fatalf("New(missing voices store) = %v", err)
	}

	missingWorkspacesCfg := validLayeredConfig(dir)
	delete(missingWorkspacesCfg.Stores, "workspaces")
	if _, err := New(missingWorkspacesCfg); err == nil || !strings.Contains(err.Error(), "services.workspace.store") {
		t.Fatalf("New(missing workspaces store) = %v", err)
	}

	missingWorkflowsCfg := validLayeredConfig(dir)
	delete(missingWorkflowsCfg.Stores, "workflows")
	if _, err := New(missingWorkflowsCfg); err == nil || !strings.Contains(err.Error(), "services.workflow.store") {
		t.Fatalf("New(missing workflows store) = %v", err)
	}

	missingLogSinkCfg := validLayeredConfig(dir)
	missingLogSinkCfg.Services.SystemLog = &logging.Config{Sinks: []logging.SinkConfig{{Kind: logging.SinkStore, Store: "missing-logs"}}}
	if _, err := New(missingLogSinkCfg); err == nil || !strings.Contains(err.Error(), "services.system_log.sinks[0].store") {
		t.Fatalf("New(missing system log sink) = %v", err)
	}

	wrongLogSinkCfg := validLayeredConfig(dir)
	wrongLogSinkCfg.Services.SystemLog = &logging.Config{Sinks: []logging.SinkConfig{{Kind: logging.SinkStore, Store: "metrics"}}}
	if _, err := New(wrongLogSinkCfg); err == nil || !strings.Contains(err.Error(), "logstore.ImmutableStore") {
		t.Fatalf("New(wrong system log sink) = %v", err)
	}

}

func TestNewLeavesLogQueryUnconfiguredWithoutQueryStore(t *testing.T) {
	disabledCfg := validLayeredConfig(t.TempDir())
	disabled, err := New(disabledCfg)
	if err != nil {
		t.Fatalf("New(disabled) error = %v", err)
	}
	t.Cleanup(func() { _ = disabled.Close() })
	if disabled.ServerLogQuery != nil {
		t.Fatal("query service should be absent without system_log.query_store")
	}
}

func TestNewWiresPeerListenerFactory(t *testing.T) {
	srv, err := New(validLayeredConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	if len(srv.Server.PeerListenerFactories) != 1 {
		t.Fatalf("PeerListenerFactories len = %d, want 1", len(srv.Server.PeerListenerFactories))
	}
}

func TestConfigValidateRequiresServices(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Listen = "127.0.0.1:9820"
	cfg.Endpoint = "127.0.0.1:9820"
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "services is required") {
		t.Fatalf("validate error = %v, want required services", err)
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
    credential-mode: turn-rest
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
	if cfg.ICEServers[0].CredentialMode != gizwebrtc.ICECredentialModeTURNREST {
		t.Fatalf("ICEServers[0].CredentialMode = %q", cfg.ICEServers[0].CredentialMode)
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

func TestLoadConfigReadsSystemLogConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `
services:
  system_log:
    level: debug
    query_store: logs
    sinks:
      - kind: stderr
      - kind: store
        store: logs
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig error = %v", err)
	}
	if cfg.Services == nil || cfg.Services.SystemLog == nil || cfg.Services.SystemLog.Level != "debug" || cfg.Services.SystemLog.QueryStore != "logs" || len(cfg.Services.SystemLog.Sinks) != 2 {
		t.Fatalf("Services = %+v", cfg.Services)
	}
}

func TestLoadConfigRejectsLegacyAndInvalidSystemLogConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("log:\n  level: info\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "top-level log") {
		t.Fatalf("LoadConfig legacy log err = %v", err)
	}

	if err := os.WriteFile(path, []byte("services:\n  system_log:\n    level: verbose\n"), 0o644); err != nil {
		t.Fatalf("WriteFile enabled error = %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "system_log.level") {
		t.Fatalf("LoadConfig invalid system log err = %v", err)
	}
}

func TestParseConfigRejectsUnknownLoggingFields(t *testing.T) {
	for _, data := range []string{
		"log: null\n",
		"log: disabled\nsystem_log:\n  sinks:\n    - kind: stderr\n",
		"system_log: null\n",
		"services:\n  system_log: stderr\n",
		"services:\n  system_log:\n    unknown: true\n",
		"services:\n  system_log:\n    sinks:\n      - kind: stderr\n        path: file.log\n",
		"stores:\n  logs:\n    kind: log\n    clickhouse:\n      dsn: x\n      unknown: y\n",
		"stores:\n  logs:\n    kind: log\n    volc:\n      endpoint: x\n      unknown: y\n",
		"stores:\n  agent-memory:\n    kind: memory\n    storage: x\n    flowcraft: {}\n",
		"stores:\n  agent-memory:\n    kind: memory\n    mem0:\n      endpoint: https://example.test\n      unknown: y\n",
		"stores:\n  agent-memory:\n    kind: memory\n    volc_memory:\n      api_key_id: x\n      unknown: y\n",
		"stores:\n  agent-memory:\n    kind: memory\n    flowcraft:\n      async:\n        unknown: y\n",
		"stores:\n  agent-memory:\n    kind: memory\n    volc_memory:\n      mem0:\n        unknown: y\n",
		"stores:\n  agent-memory:\n    kind: memory\n    flowcraft:\n      runtime_id: legacy\n",
		"stores:\n  agent-memory:\n    kind: memory\n    flowcraft:\n      async:\n        worker_id: legacy\n",
		"stores:\n  agent-memory:\n    kind: memory\n    mem0:\n      user_id: legacy\n",
		"stores:\n  agent-memory:\n    kind: memory\n    volc_memory:\n      mem0:\n        run_id: legacy\n",
		"stores:\n  agent-memory:\n    kind: memory\n    flowcraft:\n      bbh:\n        unknown: y\n",
	} {
		if _, err := parseConfigData([]byte(data)); err == nil {
			t.Fatalf("parseConfigData(%q) error = nil", data)
		}
	}
}

func TestParseConfigReadsFlowcraftHistoryClickHouse(t *testing.T) {
	cfg, err := parseConfigData([]byte(`
stores:
  flowcraft-history:
    kind: log.mutable
    storage: analytics
    clickhouse:
      database: default
      table: gizclaw_flowcraft_history
`))
	if err != nil {
		t.Fatal(err)
	}
	store := cfg.Stores["flowcraft-history"]
	if store.Kind != stores.KindLogMutable || store.ClickHouse == nil {
		t.Fatalf("flowcraft history store = %+v", store)
	}
	if store.ClickHouse.Database != "default" ||
		store.ClickHouse.Table != "gizclaw_flowcraft_history" {
		t.Fatalf("clickhouse config = %+v", store.ClickHouse)
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
			if cfg.Services == nil || cfg.Services.SystemLog == nil || cfg.Services.SystemLog.Level != "info" || len(cfg.Services.SystemLog.Sinks) != 1 || cfg.Services.SystemLog.Sinks[0].Kind != logging.SinkStderr {
				t.Fatalf("fixture services = %+v, want info stderr", cfg.Services)
			}
		})
	}
}

func TestParseCompleteServerConfigurationExample(t *testing.T) {
	path := filepath.Join("..", "..", "..", "guides", "snippets", "server-storage-stores-services.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	serverKey := testKeyPair(t, 0x71)
	adminKey := testKeyPair(t, 0x72)
	data = []byte(strings.NewReplacer(
		"${GIZCLAW_SERVER_PRIVATE_KEY}", serverKey.Private.String(),
		"${GIZCLAW_ADMIN_PUBLIC_KEY}", adminKey.Public.String(),
	).Replace(string(data)))
	cfg, err := parseConfigData(data)
	if err != nil {
		t.Fatalf("parseConfigData(%s) error = %v", path, err)
	}
	if cfg.Services == nil || cfg.Services.AgentHost == nil || cfg.Services.AgentHost.Flowcraft == nil || cfg.Services.Metrics == nil || cfg.Services.SystemLog == nil {
		t.Fatalf("complete services block = %+v", cfg.Services)
	}
	if len(cfg.Storage) != 5 {
		t.Fatalf("storage count = %d, want 5", len(cfg.Storage))
	}
	for _, name := range []string{
		"logs", "metrics", "flowcraft-history", "flowcraft-state", "peers", "peer-routes", "peer-run",
		"public-login", "credentials", "firmwares", "runtime-profiles", "models", "voices", "memory-layouts",
		"provider-tenants", "minimax-tenants", "deepseek-tenants", "volc-tenants", "workflows", "workspaces",
		"tools", "contacts", "friend-invite-tokens", "friends", "friend-groups", "friend-group-invite-tokens",
		"friend-group-members", "friend-group-belongs", "pet-defs", "badge-defs", "game-defs", "agenthost",
		"workspace-assets", "gameplay-assets", "gameplay-db",
	} {
		if _, exists := cfg.Stores[name]; !exists {
			t.Fatalf("stores.%s is missing", name)
		}
	}
	assertCompleteServerConfigInventory(t, cfg)
}

func assertCompleteServerConfigInventory(t *testing.T, cfg ConfigFile) {
	t.Helper()
	consumed := make(map[string]struct{}, len(cfg.Stores))
	expect := func(path, name string, kinds ...string) {
		t.Helper()
		store, exists := cfg.Stores[name]
		if !exists {
			t.Fatalf("%s references missing stores.%s", path, name)
		}
		if !slices.Contains(kinds, store.Kind) {
			t.Fatalf("%s references stores.%s kind %q, want one of %v", path, name, store.Kind, kinds)
		}
		consumed[name] = struct{}{}
	}
	services := cfg.Services
	expect("services.peer.store", services.Peer.Store, stores.KindKeyValue)
	expect("services.peer.route_store", services.Peer.RouteStore, stores.KindKeyValue)
	expect("services.peer.run_store", services.Peer.RunStore, stores.KindKeyValue)
	for path, name := range map[string]string{
		"services.public_login.store":                     services.PublicLogin.Store,
		"services.credential.store":                       services.Credential.Store,
		"services.firmware.store":                         services.Firmware.Store,
		"services.runtime_profile.store":                  services.RuntimeProfile.Store,
		"services.model.store":                            services.Model.Store,
		"services.voice.store":                            services.Voice.Store,
		"services.memory_layout.store":                    services.MemoryLayout.Store,
		"services.provider_tenants.generic_store":         services.ProviderTenants.GenericStore,
		"services.provider_tenants.minimax_tenant_store":  services.ProviderTenants.MiniMaxTenantStore,
		"services.provider_tenants.deepseek_tenant_store": services.ProviderTenants.DeepSeekTenantStore,
		"services.provider_tenants.volc_tenant_store":     services.ProviderTenants.VolcTenantStore,
		"services.provider_tenants.credential_store":      services.ProviderTenants.CredentialStore,
		"services.provider_tenants.model_store":           services.ProviderTenants.ModelStore,
		"services.provider_tenants.voice_store":           services.ProviderTenants.VoiceStore,
		"services.workflow.store":                         services.Workflow.Store,
		"services.workspace.store":                        services.Workspace.Store,
		"services.workspace.workflow_store":               services.Workspace.WorkflowStore,
		"services.toolkit.store":                          services.Toolkit.Store,
		"services.contact.store":                          services.Contact.Store,
		"services.friend.store":                           services.Friend.Store,
		"services.friend.invite_token_store":              services.Friend.InviteTokenStore,
		"services.friend_group.store":                     services.FriendGroup.Store,
		"services.friend_group.invite_token_store":        services.FriendGroup.InviteTokenStore,
		"services.friend_group.member_store":              services.FriendGroup.MemberStore,
		"services.friend_group.belong_store":              services.FriendGroup.BelongStore,
		"services.gameplay.pet_def_store":                 services.Gameplay.PetDefStore,
		"services.gameplay.badge_def_store":               services.Gameplay.BadgeDefStore,
		"services.gameplay.game_def_store":                services.Gameplay.GameDefStore,
	} {
		expect(path, name, stores.KindKeyValue)
	}
	expect("services.workspace.assets_store", services.Workspace.AssetsStore, stores.KindObjectStore)
	expect("services.gameplay.assets_store", services.Gameplay.AssetsStore, stores.KindObjectStore)
	expect("services.gameplay.database_store", services.Gameplay.DatabaseStore, stores.KindSQL)
	expect("services.agent_host.runtime_store", services.AgentHost.RuntimeStore, stores.KindObjectStore)
	expect("services.agent_host.flowcraft.state_store", services.AgentHost.Flowcraft.StateStore, stores.KindKeyValue)
	expect("services.agent_host.flowcraft.history_store", services.AgentHost.Flowcraft.HistoryStore, stores.KindLogMutable)
	expect("services.metrics.store", services.Metrics.Store, stores.KindMetrics)
	if services.SystemLog.QueryStore != "" {
		expect("services.system_log.query_store", services.SystemLog.QueryStore, stores.KindLogImmutable, stores.KindLogMutable)
	}
	for index, sink := range services.SystemLog.Sinks {
		if sink.Kind == logging.SinkStore {
			expect(fmt.Sprintf("services.system_log.sinks[%d].store", index), sink.Store, stores.KindLogImmutable, stores.KindLogMutable)
		}
	}

	physicalConsumers := make(map[string]struct{}, len(cfg.Storage))
	for name, store := range cfg.Stores {
		if _, exists := consumed[name]; !exists {
			t.Fatalf("stores.%s has no explicit service consumer", name)
		}
		if store.Storage == "" {
			continue
		}
		if _, exists := cfg.Storage[store.Storage]; !exists {
			t.Fatalf("stores.%s references missing storage.%s", name, store.Storage)
		}
		physicalConsumers[store.Storage] = struct{}{}
	}
	for name := range cfg.Storage {
		if _, exists := physicalConsumers[name]; !exists {
			t.Fatalf("storage.%s has no logical Store consumer", name)
		}
	}
}

func TestWailsWorkspaceTemplateUsesStrictServerConfiguration(t *testing.T) {
	path := filepath.Join("..", "..", "..", "apps", "wails", "internal", "appconfig", "templates", "local_server_workspace.yaml.gotmpl")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	tmpl, err := template.New("workspace").Funcs(template.FuncMap{"quote": quote}).Option("missingkey=error").Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	key := testKeyPair(t, 0x73)
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, struct {
		PrivateKey     string
		Listen         string
		Endpoint       string
		ServeToClients bool
		AdminPublicKey string
	}{
		PrivateKey: key.Private.String(), Listen: "0.0.0.0:19820",
		Endpoint: "127.0.0.1:19820", ServeToClients: true,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfigData(rendered.Bytes())
	if err != nil {
		t.Fatalf("rendered Wails template does not satisfy Server parsing: %v", err)
	}
	if err := validateServicesConfig(cfg.Services); err != nil {
		t.Fatalf("rendered Wails services are incomplete: %v", err)
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

func TestPrepareConfigGeneratesKeyPairAndDefaultPorts(t *testing.T) {
	cfg, err := prepareConfig(Config{Services: validServicesConfig()})
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

func TestNewUsesExplicitServiceBindings(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	cfg.Storage["renamed-kv-connector"] = cfg.Storage["memory"]
	delete(cfg.Storage, "memory")
	for name, store := range cfg.Stores {
		if store.Storage == "memory" {
			store.Storage = "renamed-kv-connector"
			cfg.Stores[name] = store
		}
	}
	cfg.Stores["renamed-peers"] = stores.Config{Kind: stores.KindKeyValue, Storage: "renamed-kv-connector", Prefix: "renamed-peers"}
	cfg.Services.Peer.Store = "renamed-peers"
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	if srv.PeerStore == nil || srv.PeerRouteStore == nil || srv.WorkspaceWorkflowStore == nil {
		t.Fatalf("explicit service Stores were not wired: %+v", srv.Server)
	}
}

func TestNewRejectsServiceCapabilityMismatch(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	cfg.Services.Peer.Store = "workspace-assets"
	_, err := New(cfg)
	if err == nil || !strings.Contains(err.Error(), `services.peer.store "workspace-assets" requires kv.Store`) {
		t.Fatalf("New() error = %v", err)
	}
}

func TestNewRejectsMissingRequiredServiceBlock(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	cfg.Services.Peer = nil
	_, err := New(cfg)
	if err == nil || !strings.Contains(err.Error(), "services.peer is required") {
		t.Fatalf("New() error = %v", err)
	}
}

func validLayeredConfig(dir string) Config {
	return Config{
		Listen:   "127.0.0.1:1234",
		Endpoint: "127.0.0.1:1234",
		Storage: map[string]storage.Config{
			"memory":      {Kind: storage.KindKeyValue, Memory: &storage.MemoryConfig{}},
			"local-files": {Kind: storage.KindObjectStore, FS: &storage.FSConfig{Dir: dir}},
			"gameplay-db": {Kind: storage.KindSQL, SQLite: &storage.SQLConfig{Dir: filepath.Join(dir, "gameplay.sqlite")}},
		},
		Stores: map[string]stores.Config{
			"peers":                      {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "peers"},
			"peer-routes":                {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "peer-routes"},
			"peer-run":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "peer-run"},
			"public-login":               {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "public-login"},
			"credentials":                {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "credentials"},
			"firmwares":                  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "firmwares"},
			"runtime-profiles":           {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "runtime-profiles"},
			"memory-layouts":             {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "memory-layouts"},
			"agenthost":                  {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "agenthost"},
			"minimax-tenants":            {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "minimax-tenants"},
			"provider-tenants":           {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "provider-tenants"},
			"deepseek-tenants":           {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "deepseek-tenants"},
			"volc-tenants":               {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "volc-tenants"},
			"models":                     {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "models"},
			"voices":                     {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "voices"},
			"workspaces":                 {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "workspaces"},
			"workflows":                  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "workflows"},
			"tools":                      {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "tools"},
			"contacts":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "contacts"},
			"friend-invite-tokens":       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-invite-tokens"},
			"friends":                    {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friends"},
			"friend-groups":              {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-groups"},
			"friend-group-invite-tokens": {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-invite-tokens"},
			"friend-group-members":       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-members"},
			"friend-group-belongs":       {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "friend-group-belongs"},
			"pet-defs":                   {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "pet-defs"},
			"badge-defs":                 {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "badge-defs"},
			"game-defs":                  {Kind: stores.KindKeyValue, Storage: "memory", Prefix: "game-defs"},
			"gameplay-assets":            {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "gameplay"},
			"workspace-assets":           {Kind: stores.KindObjectStore, Storage: "local-files", Prefix: "workspaces"},
			"gameplay-db":                {Kind: stores.KindSQL, Storage: "gameplay-db"},
		},
		Services: validServicesConfig(),
	}
}

func validServicesConfig() *ServicesConfig {
	return &ServicesConfig{
		Peer:           &PeerStoresConfig{Store: "peers", RouteStore: "peer-routes", RunStore: "peer-run"},
		PublicLogin:    &SingleStoreConfig{Store: "public-login"},
		Credential:     &SingleStoreConfig{Store: "credentials"},
		Firmware:       &SingleStoreConfig{Store: "firmwares"},
		RuntimeProfile: &SingleStoreConfig{Store: "runtime-profiles"},
		Model:          &SingleStoreConfig{Store: "models"},
		Voice:          &SingleStoreConfig{Store: "voices"},
		MemoryLayout:   &SingleStoreConfig{Store: "memory-layouts"},
		ProviderTenants: &ProviderTenantStoresConfig{
			GenericStore: "provider-tenants", MiniMaxTenantStore: "minimax-tenants",
			DeepSeekTenantStore: "deepseek-tenants", VolcTenantStore: "volc-tenants",
			CredentialStore: "credentials", ModelStore: "models", VoiceStore: "voices",
		},
		Workflow:  &SingleStoreConfig{Store: "workflows"},
		Workspace: &WorkspaceStoresConfig{Store: "workspaces", WorkflowStore: "workflows", AssetsStore: "workspace-assets"},
		Toolkit:   &SingleStoreConfig{Store: "tools"},
		Contact:   &SingleStoreConfig{Store: "contacts"},
		Friend:    &FriendStoresConfig{Store: "friends", InviteTokenStore: "friend-invite-tokens"},
		FriendGroup: &FriendGroupStoresConfig{
			Store: "friend-groups", InviteTokenStore: "friend-group-invite-tokens",
			MemberStore: "friend-group-members", BelongStore: "friend-group-belongs",
		},
		Gameplay: &GameplayStoresConfig{
			PetDefStore: "pet-defs", BadgeDefStore: "badge-defs", GameDefStore: "game-defs",
			AssetsStore: "gameplay-assets", DatabaseStore: "gameplay-db",
		},
		AgentHost: &AgentHostConfig{RuntimeStore: "agenthost"},
	}
}
