//go:build gizclaw_e2e

package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/goccy/go-yaml"
)

const (
	contextConfigDefaultHome = "test/gizclaw-e2e/testdata/gizclaw-config-home"
	contextConfigDefaultName = "e2e-client"
	contextConfigDefaultPath = contextConfigDefaultHome + "/gizclaw/" + contextConfigDefaultName + "/config.yaml"
)

type config struct {
	Server    serverConfig    `json:"server"`
	Workspace string          `json:"workspace"`
	Agent     string          `json:"agent"`
	Ensure    *bool           `json:"ensure_workspace,omitempty"`
	Models    modelConfig     `json:"models"`
	Workflow  workflowConfig  `json:"workflow"`
	Voice     string          `json:"voice"`
	Interrupt interruptConfig `json:"interrupt,omitempty"`
	Rounds    int             `json:"rounds"`
	Timeout   string          `json:"timeout"`
	Persona   string          `json:"persona"`
	OutputDir string          `json:"output_dir,omitempty"`

	ClientPrivateKey string        `json:"-"`
	timeout          time.Duration `json:"-"`
}

type interruptConfig struct {
	FirstUtterance  string `json:"first_utterance,omitempty"`
	SecondUtterance string `json:"second_utterance,omitempty"`
}

type serverConfig struct {
	Addr       string `json:"addr"`
	PublicKey  string `json:"public_key"`
	CipherMode string `json:"cipher_mode"`
}

type modelConfig struct {
	LLM         string `json:"llm" yaml:"llm"`
	TTS         string `json:"tts" yaml:"tts"`
	ASR         string `json:"asr" yaml:"asr"`
	Realtime    string `json:"realtime" yaml:"realtime"`
	Translation string `json:"translation" yaml:"translation"`
}

type workflowConfig struct {
	Name         string                               `json:"name"`
	Description  string                               `json:"description,omitempty"`
	Model        string                               `json:"model"`
	Instructions string                               `json:"instructions,omitempty"`
	Audio        *rpcapi.DoubaoRealtimeAudio          `json:"audio,omitempty"`
	Tools        *[]rpcapi.DoubaoRealtimeFunctionTool `json:"tools,omitempty"`
	Extension    *rpcapi.DoubaoRealtimeExtension      `json:"extension,omitempty"`
	Translation  string                               `json:"translation_model,omitempty"`
	Parameters   workspaceParameterConfig             `json:"parameters,omitempty"`
	Flowcraft    map[string]interface{}               `json:"flowcraft,omitempty"`
	VoiceAdapter voiceAdapterConfig                   `json:"voice_adapter,omitempty"`
	ASTTranslate astTranslateConfig                   `json:"ast_translate,omitempty"`
}

type workspaceParameterConfig struct {
	Input                      string                               `json:"input,omitempty"`
	GenerateModel              string                               `json:"generate_model,omitempty"`
	ExtractModel               string                               `json:"extract_model,omitempty"`
	EmbeddingModel             string                               `json:"embedding_model,omitempty"`
	TranslationModel           string                               `json:"translation_model,omitempty"`
	LangPair                   string                               `json:"lang_pair,omitempty"`
	Mode                       string                               `json:"mode,omitempty"`
	Model                      string                               `json:"model,omitempty"`
	Instructions               string                               `json:"instructions,omitempty"`
	Audio                      *rpcapi.DoubaoRealtimeAudio          `json:"audio,omitempty"`
	Tools                      *[]rpcapi.DoubaoRealtimeFunctionTool `json:"tools,omitempty"`
	Extension                  *rpcapi.DoubaoRealtimeExtension      `json:"extension,omitempty"`
	Voice                      workspaceVoiceConfig                 `json:"voice,omitempty"`
	SpeakerID                  string                               `json:"speaker_id,omitempty"`
	IsCustomSpeaker            *bool                                `json:"is_custom_speaker,omitempty"`
	TTSResourceID              string                               `json:"tts_resource_id,omitempty"`
	SpeechRate                 *int                                 `json:"speech_rate,omitempty"`
	EnableSourceLanguageDetect *bool                                `json:"enable_source_language_detect,omitempty"`
	Denoise                    *bool                                `json:"denoise,omitempty"`
}

type voiceAdapterConfig struct {
	ASRModel     string            `json:"asr_model,omitempty"`
	DefaultVoice string            `json:"default_voice,omitempty"`
	NodeVoices   map[string]string `json:"node_voices,omitempty"`
}

type astTranslateConfig struct {
	Mode                       string                  `json:"mode,omitempty"`
	Voice                      astTranslateVoiceConfig `json:"voice,omitempty"`
	SpeakerID                  string                  `json:"speaker_id,omitempty"`
	IsCustomSpeaker            *bool                   `json:"is_custom_speaker,omitempty"`
	TTSResourceID              string                  `json:"tts_resource_id,omitempty"`
	SpeechRate                 *int                    `json:"speech_rate,omitempty"`
	EnableSourceLanguageDetect *bool                   `json:"enable_source_language_detect,omitempty"`
	Denoise                    *bool                   `json:"denoise,omitempty"`
	ResourceID                 string                  `json:"resource_id,omitempty"`
}

type astTranslateVoiceConfig struct {
	SpeakerID       string `json:"speaker_id,omitempty"`
	IsCustomSpeaker *bool  `json:"is_custom_speaker,omitempty"`
	TTSResourceID   string `json:"tts_resource_id,omitempty"`
	SpeechRate      *int   `json:"speech_rate,omitempty"`
	TTSVoice        string `json:"tts_voice,omitempty"`
}

type workspaceVoiceConfig struct {
	RealtimeSpeakerID string `json:"realtime_speaker_id,omitempty"`
	SpeakerID         string `json:"speaker_id,omitempty"`
	IsCustomSpeaker   *bool  `json:"is_custom_speaker,omitempty"`
	TTSResourceID     string `json:"tts_resource_id,omitempty"`
	SpeechRate        *int   `json:"speech_rate,omitempty"`
	TTSVoice          string `json:"tts_voice,omitempty"`
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
		filepath.Clean(filepath.Join(configDir, "..", "gizclaw-config-home", "gizclaw", "e2e-client", "config.yaml")),
		filepath.Clean(filepath.Join(configDir, "..", "..", "testdata", "gizclaw-config-home", "gizclaw", "e2e-client", "config.yaml")),
		filepath.Clean(contextConfigDefaultPath),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func envContextConfigPath(homeEnv, contextEnv, defaultHome, defaultName string) string {
	home := strings.TrimSpace(os.Getenv(homeEnv))
	name := strings.TrimSpace(os.Getenv(contextEnv))
	if home == "" && name == "" {
		return ""
	}
	if home == "" {
		home = defaultHome
	}
	if name == "" {
		name = defaultName
	}
	return filepath.Join(home, "gizclaw", name, "config.yaml")
}

func clientContextConfigPath() string {
	return envContextConfigPath(
		"GIZCLAW_E2E_CLIENT_CONFIG_HOME",
		"GIZCLAW_E2E_CLIENT_CONTEXT",
		contextConfigDefaultHome,
		contextConfigDefaultName,
	)
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
	if raw.Server.PublicKey.IsZero() {
		return setupContextConfig{}, fmt.Errorf("decode context config %s: missing server public-key", path)
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
	c.Models.Translation = strings.TrimSpace(c.Models.Translation)
	c.Workflow.Name = strings.TrimSpace(c.Workflow.Name)
	c.Workflow.Description = strings.TrimSpace(c.Workflow.Description)
	c.Workflow.Model = strings.TrimSpace(c.Workflow.Model)
	c.Workflow.Instructions = strings.TrimSpace(c.Workflow.Instructions)
	c.Workflow.Translation = strings.TrimSpace(c.Workflow.Translation)
	c.Workflow.Parameters.Input = strings.TrimSpace(c.Workflow.Parameters.Input)
	c.Workflow.Parameters.GenerateModel = strings.TrimSpace(c.Workflow.Parameters.GenerateModel)
	c.Workflow.Parameters.ExtractModel = strings.TrimSpace(c.Workflow.Parameters.ExtractModel)
	c.Workflow.Parameters.EmbeddingModel = strings.TrimSpace(c.Workflow.Parameters.EmbeddingModel)
	c.Workflow.Parameters.TranslationModel = strings.TrimSpace(c.Workflow.Parameters.TranslationModel)
	c.Workflow.Parameters.LangPair = strings.TrimSpace(c.Workflow.Parameters.LangPair)
	c.Workflow.Parameters.Mode = strings.TrimSpace(c.Workflow.Parameters.Mode)
	c.Workflow.Parameters.Model = strings.TrimSpace(c.Workflow.Parameters.Model)
	c.Workflow.Parameters.Instructions = strings.TrimSpace(c.Workflow.Parameters.Instructions)
	c.Workflow.Parameters.SpeakerID = strings.TrimSpace(c.Workflow.Parameters.SpeakerID)
	c.Workflow.Parameters.TTSResourceID = strings.TrimSpace(c.Workflow.Parameters.TTSResourceID)
	c.Workflow.Parameters.Voice.trim()
	c.Workflow.VoiceAdapter.ASRModel = strings.TrimSpace(c.Workflow.VoiceAdapter.ASRModel)
	c.Workflow.VoiceAdapter.DefaultVoice = strings.TrimSpace(c.Workflow.VoiceAdapter.DefaultVoice)
	c.Workflow.ASTTranslate.Mode = strings.TrimSpace(c.Workflow.ASTTranslate.Mode)
	c.Workflow.ASTTranslate.SpeakerID = strings.TrimSpace(c.Workflow.ASTTranslate.SpeakerID)
	c.Workflow.ASTTranslate.TTSResourceID = strings.TrimSpace(c.Workflow.ASTTranslate.TTSResourceID)
	c.Workflow.ASTTranslate.ResourceID = strings.TrimSpace(c.Workflow.ASTTranslate.ResourceID)
	c.Workflow.ASTTranslate.Voice.trim()
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
	if c.Workflow.Name == "" && c.Workspace != "" {
		c.Workflow.Name = c.Workspace
	}
	if c.Workspace == "" {
		c.Workspace = c.Workflow.Name
	}
	if c.Workspace == "" {
		return fmt.Errorf("workspace or workflow.name is required")
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
	if c.isDoubaoRealtimeAgent() && c.Models.Realtime == "" {
		return fmt.Errorf("models.realtime is required")
	}
	if c.isASTTranslateAgent() && c.Models.Translation == "" {
		return fmt.Errorf("models.translation is required")
	}
	if c.Workflow.Model == "" {
		c.Workflow.Model = c.Models.Realtime
	}
	if c.Workflow.Translation == "" {
		c.Workflow.Translation = c.Models.Translation
	}
	if c.isDoubaoRealtimeAgent() {
		if c.Workflow.Instructions == "" {
			c.Workflow.Instructions = "你是一个简短、自然的中文语音聊天助手。"
		}
		if c.Workflow.Audio == nil {
			c.Workflow.Audio = defaultDoubaoRealtimeAudio()
		}
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
	if c.isASTTranslateAgent() {
		if c.Workflow.ASTTranslate.Mode == "" {
			c.Workflow.ASTTranslate.Mode = "s2s"
		}
		if c.Workflow.Parameters.TranslationModel == "" {
			c.Workflow.Parameters.TranslationModel = c.Workflow.Translation
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

func (c config) isDoubaoRealtimeAgent() bool {
	return c.Agent == "doubao-realtime"
}

func (c config) isASTTranslateAgent() bool {
	return c.Agent == "ast-translate"
}

func defaultDoubaoRealtimeAudio() *rpcapi.DoubaoRealtimeAudio {
	return &rpcapi.DoubaoRealtimeAudio{
		Input: rpcapi.DoubaoRealtimeAudioInput{
			Format: rpcapi.DoubaoRealtimeAudioFormat{
				Type: rpcapi.DoubaoRealtimeAudioFormatType("speech_opus"),
				Rate: 16000,
			},
		},
		Output: rpcapi.DoubaoRealtimeAudioOutput{
			Format: rpcapi.DoubaoRealtimeAudioFormat{
				Type: rpcapi.DoubaoRealtimeAudioFormatType("ogg_opus"),
				Rate: 24000,
			},
			Voice: optionalString("zh_female_vv_jupiter_bigtts"),
		},
	}
}

func (c config) shouldEnsureWorkspace() bool {
	return c.Ensure == nil || *c.Ensure
}

func (c config) flowcraftStartsSelf() bool {
	if !c.isFlowcraftAgent() {
		return false
	}
	conversation, ok := c.Workflow.Flowcraft["conversation"].(map[string]interface{})
	if !ok {
		return false
	}
	starts, _ := conversation["starts"].(string)
	return strings.EqualFold(strings.TrimSpace(starts), "self")
}

func (c config) workspaceMode() string {
	return normalizeWorkspaceMode(c.Workflow.Parameters.Input)
}

func normalizeWorkspaceMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "realtime", "real_time", "real-time":
		return "realtime"
	default:
		return "push_to_talk"
	}
}

func (v *astTranslateVoiceConfig) trim() {
	v.SpeakerID = strings.TrimSpace(v.SpeakerID)
	v.TTSResourceID = strings.TrimSpace(v.TTSResourceID)
	v.TTSVoice = strings.TrimSpace(v.TTSVoice)
}

func (v *workspaceVoiceConfig) trim() {
	v.RealtimeSpeakerID = strings.TrimSpace(v.RealtimeSpeakerID)
	v.SpeakerID = strings.TrimSpace(v.SpeakerID)
	v.TTSResourceID = strings.TrimSpace(v.TTSResourceID)
	v.TTSVoice = strings.TrimSpace(v.TTSVoice)
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
