package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestLoadConfigJSONAndDefaultClientConfig(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	dir := t.TempDir()
	configDir := filepath.Join(dir, "workspace", "config")
	contextDir := filepath.Join(dir, ".testbench", "context", "gizclaw", "e2e-client")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("create context dir: %v", err)
	}
	configPath := filepath.Join(configDir, "doubao-realtime.json")
	configData := []byte(`{
  "agent": "doubao-realtime",
  "workflow": {
    "name": "doubao-realtime-workflow",
    "realtime_model": "setup-realtime"
  },
  "models": {
    "llm": "setup-chat",
    "tts": "setup-tts",
    "asr": "setup-asr",
    "realtime": "setup-realtime"
  },
  "voice": "setup-voice",
  "rounds": 2,
  "timeout": "5s",
  "persona": "short"
}`)
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	writeSetupContextConfig(t, filepath.Join(contextDir, "config.yaml"), serverKey, clientKey, "aes-gcm")

	cfg, err := loadConfig(configPath, "")
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Server.CipherMode != string(giznet.CipherModeAES256GCM) {
		t.Fatalf("cipher mode = %q", cfg.Server.CipherMode)
	}
	if cfg.Workspace != "doubao-realtime-workflow" || cfg.Agent != "doubao-realtime" {
		t.Fatalf("workspace/agent = %q/%q", cfg.Workspace, cfg.Agent)
	}
	if cfg.Workflow.Name != "doubao-realtime-workflow" || cfg.Workflow.RealtimeModel != "setup-realtime" {
		t.Fatalf("workflow = %+v", cfg.Workflow)
	}
	if cfg.Models != (modelConfig{LLM: "setup-chat", TTS: "setup-tts", ASR: "setup-asr", Realtime: "setup-realtime"}) {
		t.Fatalf("models = %+v", cfg.Models)
	}
	if cfg.Voice != "setup-voice" {
		t.Fatalf("voice = %q", cfg.Voice)
	}
	if cfg.Rounds != 2 || cfg.timeout != 5*time.Second {
		t.Fatalf("rounds/timeout = %d/%s", cfg.Rounds, cfg.timeout)
	}
	if cfg.ClientPrivateKey != clientKey.Private.String() {
		t.Fatalf("client private key was not loaded from setup context identity")
	}
}

func TestLoadConfigJSONWithExplicitClientConfig(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	contextConfigPath := filepath.Join(dir, "config.yaml")
	configData := `{
  "agent": "doubao-realtime",
  "workflow": {
    "name": "demo"
  },
  "models": {
    "llm": "chat",
    "tts": "tts",
    "asr": "asr",
    "realtime": "realtime"
  },
  "voice": "voice",
  "rounds": 1,
  "persona": "persona"
}`
	if err := os.WriteFile(configPath, []byte(configData), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	writeSetupContextConfig(t, contextConfigPath, serverKey, clientKey, "")
	cfg, err := loadConfig(configPath, contextConfigPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.Timeout != "120s" || cfg.timeout != 120*time.Second {
		t.Fatalf("default timeout = %q/%s", cfg.Timeout, cfg.timeout)
	}
	if cfg.Server.CipherMode != string(giznet.CipherModeChaChaPoly) {
		t.Fatalf("default cipher mode = %q", cfg.Server.CipherMode)
	}
}

func TestLoadFlowcraftConfigs(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	contextConfigPath := filepath.Join(t.TempDir(), "config.yaml")
	writeSetupContextConfig(t, contextConfigPath, serverKey, clientKey, "")

	paths, err := flowcraftConfigPaths(filepath.Join("config"))
	if err != nil {
		t.Fatalf("flowcraftConfigPaths() error = %v", err)
	}
	if len(paths) != 9 {
		t.Fatalf("flowcraft config count = %d, want 9: %v", len(paths), paths)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg, err := loadConfig(path, contextConfigPath)
			if err != nil {
				t.Fatalf("loadConfig(%s) error = %v", path, err)
			}
			if cfg.Agent != "flowcraft" || cfg.Workspace != cfg.Workflow.Name || !strings.HasPrefix(cfg.Workflow.Name, "e2e-flowcraft-") {
				t.Fatalf("loaded cfg = %+v", cfg)
			}
			if cfg.Workflow.VoiceAdapter.DefaultVoice == "" || len(cfg.Workflow.VoiceAdapter.NodeVoices) == 0 {
				t.Fatalf("voice adapter = %+v", cfg.Workflow.VoiceAdapter)
			}
		})
	}
}

func TestReadSetupContextConfigErrors(t *testing.T) {
	if _, err := readSetupContextConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing context config succeeded")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("["), 0o600); err != nil {
		t.Fatalf("write bad context config: %v", err)
	}
	if _, err := readSetupContextConfig(path); err == nil || !strings.Contains(err.Error(), "decode context config") {
		t.Fatalf("malformed context config error = %v", err)
	}
}

func TestConfigValidationRejectsMissingSecret(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	cfg := config{
		Server:   serverConfig{Addr: "127.0.0.1:9820", PublicKey: serverKey.Public.String()},
		Agent:    "doubao-realtime",
		Models:   modelConfig{LLM: "chat", TTS: "tts", ASR: "asr", Realtime: "realtime"},
		Workflow: workflowConfig{Name: "demo"},
		Voice:    "voice",
		Rounds:   1,
		Persona:  "persona",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("validate() succeeded without client private key")
	}
}

func TestConfigValidationErrors(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	valid := func() config {
		return config{
			Server:           serverConfig{Addr: "127.0.0.1:9820", PublicKey: serverKey.Public.String()},
			Agent:            "doubao-realtime",
			Models:           modelConfig{LLM: "chat", TTS: "tts", ASR: "asr", Realtime: "realtime"},
			Workflow:         workflowConfig{Name: "demo"},
			Voice:            "voice",
			Rounds:           1,
			Timeout:          "1s",
			Persona:          "persona",
			ClientPrivateKey: clientKey.Private.String(),
		}
	}
	tests := []struct {
		name string
		edit func(*config)
		want string
	}{
		{"addr", func(c *config) { c.Server.Addr = "" }, "server.addr"},
		{"public key", func(c *config) { c.Server.PublicKey = "bad" }, "server.public_key"},
		{"workflow name", func(c *config) { c.Workflow.Name = "" }, "workflow.name"},
		{"agent", func(c *config) { c.Agent = "" }, "agent"},
		{"llm", func(c *config) { c.Models.LLM = "" }, "models.llm"},
		{"tts", func(c *config) { c.Models.TTS = "" }, "models.tts"},
		{"asr", func(c *config) { c.Models.ASR = "" }, "models.asr"},
		{"realtime", func(c *config) { c.Models.Realtime = "" }, "models.realtime"},
		{"voice", func(c *config) { c.Voice = "" }, "voice"},
		{"rounds", func(c *config) { c.Rounds = 0 }, "rounds"},
		{"timeout parse", func(c *config) { c.Timeout = "bad" }, "timeout"},
		{"timeout positive", func(c *config) { c.Timeout = "-1s" }, "positive"},
		{"persona", func(c *config) { c.Persona = "" }, "persona"},
		{"private key", func(c *config) { c.ClientPrivateKey = "bad" }, "client private key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.edit(&cfg)
			err := cfg.validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate() error = %v, want %q", err, tt.want)
			}
		})
	}
	cfg := valid()
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid config error = %v", err)
	}
	if cfg.timeout != time.Second {
		t.Fatalf("timeout = %s", cfg.timeout)
	}
	if cfg.Workflow.Name != "demo" || cfg.Workflow.RealtimeModel != "realtime" {
		t.Fatalf("workflow defaults = %+v", cfg.Workflow)
	}
}

func TestConfigWorkspaceMode(t *testing.T) {
	cfg := config{}
	if cfg.workspaceMode() != "push_to_talk" {
		t.Fatalf("default workspace mode = %q", cfg.workspaceMode())
	}
	cfg.Workflow.Parameters.Input = "realtime"
	if cfg.workspaceMode() != "realtime" {
		t.Fatalf("input realtime mode = %q", cfg.workspaceMode())
	}
	cfg.Workflow.Parameters.Input = "push"
	if cfg.workspaceMode() != "push_to_talk" {
		t.Fatalf("push workspace mode = %q", cfg.workspaceMode())
	}
}

func writeSetupContextConfig(t *testing.T, path string, serverKey, clientKey *giznet.KeyPair, cipherMode string) {
	t.Helper()
	if cipherMode == "" {
		cipherMode = string(giznet.CipherModeChaChaPoly)
	}
	contextDir := filepath.Dir(path)
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("create context dir: %v", err)
	}
	contextYAML := "server:\n  address: 127.0.0.1:9820\n  public-key: " + serverKey.Public.String() + "\n  cipher-mode: " + cipherMode + "\n"
	if err := os.WriteFile(path, []byte(contextYAML), 0o644); err != nil {
		t.Fatalf("write context config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "identity.key"), clientKey.Private[:], 0o600); err != nil {
		t.Fatalf("write context identity: %v", err)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	if _, err := loadConfig("", ""); err == nil {
		t.Fatal("empty config path succeeded")
	}
	if _, err := loadConfig(filepath.Join(t.TempDir(), "missing.json"), ""); err == nil {
		t.Fatal("missing config succeeded")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(":"), 0o644); err != nil {
		t.Fatalf("write bad config: %v", err)
	}
	if _, err := loadConfig(path, ""); err == nil {
		t.Fatal("bad config succeeded")
	}
}

func TestNormalizeCipherMode(t *testing.T) {
	tests := map[string]string{
		"":             string(giznet.CipherModeChaChaPoly),
		"aes-gcm":      string(giznet.CipherModeAES256GCM),
		"aes_256_gcm":  string(giznet.CipherModeAES256GCM),
		"plaintext":    string(giznet.CipherModePlaintext),
		"chacha-poly":  string(giznet.CipherModeChaChaPoly),
		"custom-value": "custom-value",
	}
	for in, want := range tests {
		if got := normalizeCipherMode(in); got != want {
			t.Fatalf("normalizeCipherMode(%q) = %q, want %q", in, got, want)
		}
	}
}
