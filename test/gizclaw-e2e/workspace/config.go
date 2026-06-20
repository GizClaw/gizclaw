package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/goccy/go-yaml"
)

const contextConfigDefaultPath = "test/gizclaw-e2e/.testbench/context/gizclaw/e2e-client/config.yaml"

type config struct {
	Server    serverConfig   `json:"server"`
	Workspace string         `json:"workspace"`
	Agent     string         `json:"agent"`
	Models    modelConfig    `json:"models"`
	Workflow  workflowConfig `json:"workflow"`
	Voice     string         `json:"voice"`
	Rounds    int            `json:"rounds"`
	Timeout   string         `json:"timeout"`
	Persona   string         `json:"persona"`
	OutputDir string         `json:"output_dir,omitempty"`

	ClientPrivateKey string        `json:"-"`
	timeout          time.Duration `json:"-"`
}

type serverConfig struct {
	Addr       string `json:"addr"`
	PublicKey  string `json:"public_key"`
	CipherMode string `json:"cipher_mode"`
}

type modelConfig struct {
	LLM      string `json:"llm" yaml:"llm"`
	TTS      string `json:"tts" yaml:"tts"`
	ASR      string `json:"asr" yaml:"asr"`
	Realtime string `json:"realtime" yaml:"realtime"`
}

type workflowConfig struct {
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	RealtimeModel string                   `json:"realtime_model"`
	Parameters    workspaceParameterConfig `json:"parameters,omitempty"`
	Session       realtimeSessionConfig    `json:"session"`
	Output        realtimeOutputConfig     `json:"output"`
	Flowcraft     map[string]interface{}   `json:"flowcraft,omitempty"`
	VoiceAdapter  voiceAdapterConfig       `json:"voice_adapter,omitempty"`
}

type workspaceParameterConfig struct {
	E2E            *bool  `json:"e2e,omitempty"`
	GenerateModel  string `json:"generate_model,omitempty"`
	ExtractModel   string `json:"extract_model,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

type realtimeSessionConfig struct {
	AuthMode    string `json:"auth_mode,omitempty"`
	BotName     string `json:"bot_name,omitempty"`
	Model       string `json:"model,omitempty"`
	ResourceID  string `json:"resource_id,omitempty"`
	SystemRole  string `json:"system_role,omitempty"`
	VADWindowMS int    `json:"vad_window_ms,omitempty"`
}

type realtimeOutputConfig struct {
	Speaker string `json:"speaker,omitempty"`
}

type voiceAdapterConfig struct {
	ASRModel     string            `json:"asr_model,omitempty"`
	DefaultVoice string            `json:"default_voice,omitempty"`
	NodeVoices   map[string]string `json:"node_voices,omitempty"`
}

type setupContextConfig struct {
	Server           serverConfig `yaml:"-"`
	ClientPrivateKey string       `yaml:"-"`
}

func loadConfig(path, contextConfigPath string) (config, error) {
	if strings.TrimSpace(path) == "" {
		return config{}, fmt.Errorf("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg config
	if strings.ToLower(filepath.Ext(path)) != ".json" {
		return config{}, fmt.Errorf("config %s must be a .json file", path)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("decode config %s: %w", path, err)
	}

	if contextConfigPath == "" {
		contextConfigPath = defaultContextConfigPath(path)
	}
	contextCfg, err := readSetupContextConfig(contextConfigPath)
	if err != nil {
		return config{}, err
	}
	cfg.applySetupContextConfig(contextCfg)
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func defaultContextConfigPath(configPath string) string {
	configDir := filepath.Dir(configPath)
	candidates := []string{
		filepath.Clean(filepath.Join(configDir, "..", "..", ".testbench", "context", "gizclaw", "e2e-client", "config.yaml")),
		filepath.Clean(contextConfigDefaultPath),
		filepath.Clean(filepath.Join("..", ".testbench", "context", "gizclaw", "e2e-client", "config.yaml")),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func readSetupContextConfig(path string) (setupContextConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return setupContextConfig{}, fmt.Errorf("read context config %s: %w", path, err)
	}
	contextDir := filepath.Dir(path)
	var raw struct {
		Server struct {
			Address    string           `yaml:"address"`
			PublicKey  giznet.PublicKey `yaml:"public-key"`
			CipherMode string           `yaml:"cipher-mode"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return setupContextConfig{}, fmt.Errorf("decode context config %s: %w", path, err)
	}
	identityData, err := os.ReadFile(filepath.Join(contextDir, "identity.key"))
	if err != nil {
		return setupContextConfig{}, fmt.Errorf("read context identity %s: %w", filepath.Join(contextDir, "identity.key"), err)
	}
	if len(identityData) != giznet.KeySize {
		return setupContextConfig{}, fmt.Errorf("context identity length: got %d, want %d", len(identityData), giznet.KeySize)
	}
	var privateKey giznet.Key
	copy(privateKey[:], identityData)
	cfg := setupContextConfig{
		Server: serverConfig{
			Addr:       raw.Server.Address,
			PublicKey:  raw.Server.PublicKey.String(),
			CipherMode: raw.Server.CipherMode,
		},
		ClientPrivateKey: privateKey.String(),
	}
	return cfg, nil
}

func (c *config) applySetupContextConfig(contextCfg setupContextConfig) {
	c.Server = contextCfg.Server
	c.ClientPrivateKey = contextCfg.ClientPrivateKey
}

func (c *config) validate() error {
	c.Server.Addr = strings.TrimSpace(c.Server.Addr)
	c.Server.PublicKey = strings.TrimSpace(c.Server.PublicKey)
	c.Server.CipherMode = normalizeCipherMode(strings.TrimSpace(c.Server.CipherMode))
	c.Workspace = strings.TrimSpace(c.Workspace)
	c.Agent = strings.TrimSpace(c.Agent)
	c.Models.LLM = strings.TrimSpace(c.Models.LLM)
	c.Models.TTS = strings.TrimSpace(c.Models.TTS)
	c.Models.ASR = strings.TrimSpace(c.Models.ASR)
	c.Models.Realtime = strings.TrimSpace(c.Models.Realtime)
	c.Workflow.Name = strings.TrimSpace(c.Workflow.Name)
	c.Workflow.Description = strings.TrimSpace(c.Workflow.Description)
	c.Workflow.RealtimeModel = strings.TrimSpace(c.Workflow.RealtimeModel)
	c.Workflow.Session.AuthMode = strings.TrimSpace(c.Workflow.Session.AuthMode)
	c.Workflow.Session.BotName = strings.TrimSpace(c.Workflow.Session.BotName)
	c.Workflow.Session.Model = strings.TrimSpace(c.Workflow.Session.Model)
	c.Workflow.Session.ResourceID = strings.TrimSpace(c.Workflow.Session.ResourceID)
	c.Workflow.Session.SystemRole = strings.TrimSpace(c.Workflow.Session.SystemRole)
	c.Workflow.Output.Speaker = strings.TrimSpace(c.Workflow.Output.Speaker)
	c.Workflow.VoiceAdapter.ASRModel = strings.TrimSpace(c.Workflow.VoiceAdapter.ASRModel)
	c.Workflow.VoiceAdapter.DefaultVoice = strings.TrimSpace(c.Workflow.VoiceAdapter.DefaultVoice)
	for rawNodeID, voice := range c.Workflow.VoiceAdapter.NodeVoices {
		nodeID := strings.TrimSpace(rawNodeID)
		voice = strings.TrimSpace(voice)
		delete(c.Workflow.VoiceAdapter.NodeVoices, rawNodeID)
		if nodeID == "" || voice == "" {
			continue
		}
		c.Workflow.VoiceAdapter.NodeVoices[nodeID] = voice
	}
	c.Voice = strings.TrimSpace(c.Voice)
	c.Timeout = strings.TrimSpace(c.Timeout)
	c.Persona = strings.TrimSpace(c.Persona)
	c.OutputDir = strings.TrimSpace(c.OutputDir)

	if c.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}
	if _, err := parsePublicKey(c.Server.PublicKey); err != nil {
		return fmt.Errorf("server.public_key: %w", err)
	}
	if c.Server.CipherMode == "" {
		c.Server.CipherMode = string(giznet.CipherModeChaChaPoly)
	}
	if c.Workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if c.Workflow.Name == "" {
		c.Workflow.Name = c.Workspace
	}
	if c.Agent == "" {
		return fmt.Errorf("agent is required")
	}
	if c.Models.LLM == "" {
		return fmt.Errorf("models.llm is required")
	}
	if c.Models.TTS == "" {
		return fmt.Errorf("models.tts is required")
	}
	if c.Models.ASR == "" {
		return fmt.Errorf("models.asr is required")
	}
	if !c.isFlowcraftAgent() && c.Models.Realtime == "" {
		return fmt.Errorf("models.realtime is required")
	}
	if c.Workflow.RealtimeModel == "" {
		c.Workflow.RealtimeModel = c.Models.Realtime
	}
	if c.Workflow.Session.AuthMode == "" {
		c.Workflow.Session.AuthMode = "v2"
	}
	if c.Workflow.Session.BotName == "" {
		c.Workflow.Session.BotName = "豆包"
	}
	if c.Workflow.Session.Model == "" {
		c.Workflow.Session.Model = "O"
	}
	if c.Workflow.Session.ResourceID == "" {
		c.Workflow.Session.ResourceID = "volc.speech.dialog"
	}
	if c.Workflow.Session.SystemRole == "" {
		c.Workflow.Session.SystemRole = "你是一个简短、自然的中文语音聊天助手。"
	}
	if c.Workflow.Session.VADWindowMS <= 0 {
		c.Workflow.Session.VADWindowMS = 200
	}
	if c.Workflow.Output.Speaker == "" {
		c.Workflow.Output.Speaker = "zh_female_vv_jupiter_bigtts"
	}
	if c.isFlowcraftAgent() {
		if c.Workflow.VoiceAdapter.ASRModel == "" {
			c.Workflow.VoiceAdapter.ASRModel = c.Models.ASR
		}
		if c.Workflow.VoiceAdapter.DefaultVoice == "" {
			c.Workflow.VoiceAdapter.DefaultVoice = c.Voice
		}
		if c.Workflow.Parameters.GenerateModel == "" {
			c.Workflow.Parameters.GenerateModel = c.Models.LLM
		}
	}
	if c.Voice == "" {
		return fmt.Errorf("voice is required")
	}
	if c.Rounds <= 0 {
		return fmt.Errorf("rounds must be positive")
	}
	if c.Timeout == "" {
		c.Timeout = "120s"
	}
	timeout, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return fmt.Errorf("timeout: %w", err)
	}
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	c.timeout = timeout
	if c.Persona == "" {
		return fmt.Errorf("persona is required")
	}
	if c.ClientPrivateKey == "" {
		return fmt.Errorf("client private key is required in setup context identity")
	}
	if _, err := parsePrivateKey(c.ClientPrivateKey); err != nil {
		return fmt.Errorf("client private key: %w", err)
	}
	return nil
}

func (c config) isFlowcraftAgent() bool {
	return c.Agent == "flowcraft"
}

func normalizeCipherMode(mode string) string {
	switch strings.ToLower(strings.ReplaceAll(mode, "-", "_")) {
	case "", "chacha", "chacha_poly", "chacha20_poly1305":
		return string(giznet.CipherModeChaChaPoly)
	case "aes", "aes_gcm", "aes_256_gcm", "aes256_gcm":
		return string(giznet.CipherModeAES256GCM)
	case "plain", "plaintext":
		return string(giznet.CipherModePlaintext)
	default:
		return mode
	}
}

func parsePublicKey(text string) (giznet.PublicKey, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(text)); err != nil {
		return giznet.PublicKey{}, err
	}
	return key, nil
}

func parsePrivateKey(text string) (*giznet.KeyPair, error) {
	var key giznet.Key
	if err := key.UnmarshalText([]byte(text)); err != nil {
		return nil, err
	}
	return giznet.NewKeyPair(key)
}
