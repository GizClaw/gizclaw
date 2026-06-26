package modelloader

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/genx/generators"
	"github.com/GizClaw/gizclaw-go/pkg/genx/labelers"
	"github.com/GizClaw/gizclaw-go/pkg/genx/transformers"
)

func TestExpandEnv(t *testing.T) {
	// Set test environment variable
	os.Setenv("TEST_API_KEY", "test-key-123")
	defer os.Unsetenv("TEST_API_KEY")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"plain value", "plain-api-key", "plain-api-key"},
		{"env var with $", "$TEST_API_KEY", "test-key-123"},
		{"env var with ${}", "${TEST_API_KEY}", "test-key-123"},
		{"unset env var", "$UNSET_VAR", ""},
		{"mixed content", "prefix-$TEST_API_KEY-suffix", "prefix-$TEST_API_KEY-suffix"}, // Only expands if starts with $
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandEnv(tt.input)
			if result != tt.expected {
				t.Errorf("expandEnv(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseConfig_JSON(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Write test JSON config
	jsonContent := `{
		"schema": "openai/chat/v1",
		"type": "generator",
		"api_key": "test-key",
		"base_url": "https://api.example.com",
		"models": [
			{
				"name": "test/model",
				"model": "gpt-4"
			}
		]
	}`
	jsonPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig(jsonPath)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.Schema != "openai/chat/v1" {
		t.Errorf("Schema = %q, want %q", cfg.Schema, "openai/chat/v1")
	}
	if cfg.Type != "generator" {
		t.Errorf("Type = %q, want %q", cfg.Type, "generator")
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key")
	}
	if len(cfg.Models) != 1 {
		t.Errorf("len(Models) = %d, want 1", len(cfg.Models))
	}
	if cfg.Models[0].Name != "test/model" {
		t.Errorf("Models[0].Name = %q, want %q", cfg.Models[0].Name, "test/model")
	}
}

func TestParseConfig_YAML(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `
schema: doubao/seed_tts/v2
type: tts
app_id: test-app-id
api_key: test-api-key
voices:
  - name: doubao/voice1
    voice_id: zh_female_test
    desc: Test Voice
`
	yamlPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig(yamlPath)
	if err != nil {
		t.Fatalf("parseConfig failed: %v", err)
	}

	if cfg.Schema != "doubao/seed_tts/v2" {
		t.Errorf("Schema = %q, want %q", cfg.Schema, "doubao/seed_tts/v2")
	}
	if cfg.Type != "tts" {
		t.Errorf("Type = %q, want %q", cfg.Type, "tts")
	}
	if cfg.APIKey != "test-api-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-api-key")
	}
	if cfg.AppID != "test-app-id" {
		t.Errorf("AppID = %q, want %q", cfg.AppID, "test-app-id")
	}
	if len(cfg.Voices) != 1 {
		t.Errorf("len(Voices) = %d, want 1", len(cfg.Voices))
	}
}

func TestParseConfig_UnsupportedExtension(t *testing.T) {
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "config.txt")
	if err := os.WriteFile(txtPath, []byte("some content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseConfig(txtPath)
	if err == nil {
		t.Error("expected error for unsupported extension")
	}
}

func TestRegisterConfig_LegacyKind(t *testing.T) {
	// Test that legacy format with missing API key returns error
	cfg := ConfigFile{
		Kind: "openai",
		// No APIKey set
	}

	_, err := registerConfig(cfg)
	if err == nil {
		t.Error("expected error for missing api_key")
	}
}

func TestRegisterConfig_SchemaType(t *testing.T) {
	// Test unknown type returns error
	cfg := ConfigFile{
		Schema: "test/schema/v1",
		Type:   "unknown_type",
	}

	_, err := registerConfig(cfg)
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestRegisterConfig_InvalidSchema(t *testing.T) {
	cfg := ConfigFile{
		Schema: "invalid", // Missing parts
		Type:   "generator",
	}

	_, err := registerConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid schema")
	}
}

func TestRegisterOpenAIAndGemini(t *testing.T) {
	oldMux := generators.DefaultMux
	generators.DefaultMux = generators.NewMux()
	t.Cleanup(func() {
		generators.DefaultMux = oldMux
	})

	openAINames, err := registerOpenAI(ConfigFile{
		APIKey:  "sk-test",
		BaseURL: "https://example.invalid/v1",
		Models: []Entry{{
			Name:              "openai/test",
			Model:             "gpt-test",
			SupportJSONOutput: true,
			SupportToolCalls:  true,
			TextOnly:          true,
			PromptRole:        genx.PromptRoleSystem,
			ExtraFields:       map[string]any{"x": "y"},
		}},
	})
	if err != nil {
		t.Fatalf("registerOpenAI() error = %v", err)
	}
	if len(openAINames) != 1 || openAINames[0] != "openai/test" {
		t.Fatalf("openAI names = %v, want [openai/test]", openAINames)
	}
	if _, err := generators.DefaultMux.GenerateStream(testingContext(t), "openai/missing", nil); err == nil {
		t.Fatal("expected missing generator error")
	}

	geminiNames, err := registerGemini(ConfigFile{
		APIKey: "gemini-key",
		Models: []Entry{{
			Name:  "gemini/test",
			Model: "gemini-test",
		}},
	})
	if err != nil {
		t.Fatalf("registerGemini() error = %v", err)
	}
	if len(geminiNames) != 1 || geminiNames[0] != "gemini/test" {
		t.Fatalf("gemini names = %v, want [gemini/test]", geminiNames)
	}
}

func TestRegisterOpenAIAndGeminiValidation(t *testing.T) {
	if _, err := registerOpenAI(ConfigFile{}); err == nil {
		t.Fatal("expected openai missing api_key error")
	}
	if _, err := registerOpenAI(ConfigFile{APIKey: "sk-test", Models: []Entry{{Name: "bad"}}}); err == nil {
		t.Fatal("expected openai missing model error")
	}
	if _, err := registerGemini(ConfigFile{}); err == nil {
		t.Fatal("expected gemini missing api_key error")
	}
	if _, err := registerGemini(ConfigFile{APIKey: "gemini-key", Models: []Entry{{Model: "bad"}}}); err == nil {
		t.Fatal("expected gemini missing name error")
	}
}

func TestRegisterSpeechSchemaDispatch(t *testing.T) {
	oldDefaultMux := transformers.DefaultMux
	oldASRMux := transformers.ASRMux
	oldTTSMux := transformers.TTSMux
	transformers.DefaultMux = transformers.NewMux()
	transformers.ASRMux = transformers.NewASRMux()
	transformers.TTSMux = transformers.NewTTSMux()
	t.Cleanup(func() {
		transformers.DefaultMux = oldDefaultMux
		transformers.ASRMux = oldASRMux
		transformers.TTSMux = oldTTSMux
	})

	tests := []struct {
		name string
		cfg  ConfigFile
		want string
		call func(ConfigFile) ([]string, error)
	}{
		{
			name: "asr",
			cfg: ConfigFile{
				Schema: "doubao/asr/v1",
				AppID:  "app-id",
				APIKey: "api-key",
				Models: []Entry{{Name: "asr/schema-test"}},
			},
			want: "asr/schema-test",
			call: registerASRBySchema,
		},
		{
			name: "realtime",
			cfg: ConfigFile{
				Schema: "doubao/realtime/v1",
				AppID:  "app-id",
				APIKey: "api-key",
				Models: []Entry{{Name: "realtime/schema-test"}},
			},
			want: "realtime/schema-test",
			call: registerRealtimeBySchema,
		},
		{
			name: "doubao tts",
			cfg: ConfigFile{
				Schema: "doubao/seed_tts/v2",
				AppID:  "app-id",
				APIKey: "api-key",
				DefaultParams: map[string]any{
					"format":      "mp3",
					"sample_rate": float64(24000),
				},
				Voices: []VoiceEntry{{Name: "tts/doubao", VoiceID: "voice-doubao"}},
			},
			want: "tts/doubao",
			call: registerTTSBySchema,
		},
		{
			name: "minimax tts",
			cfg: ConfigFile{
				Schema:  "minimax/speech/v1",
				APIKey:  "mini-key",
				BaseURL: "https://example.invalid",
				Model:   "speech-02-hd",
				Voices:  []VoiceEntry{{Name: "tts/minimax", VoiceID: "voice-minimax"}},
			},
			want: "tts/minimax",
			call: registerTTSBySchema,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, err := tt.call(tt.cfg)
			if err != nil {
				t.Fatalf("%s register error = %v", tt.name, err)
			}
			if len(names) != 1 || names[0] != tt.want {
				t.Fatalf("names = %v, want [%s]", names, tt.want)
			}
		})
	}
}

func TestRegisterSpeechSchemaValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  ConfigFile
		call func(ConfigFile) ([]string, error)
	}{
		{name: "asr invalid schema", cfg: ConfigFile{Schema: "bad"}, call: registerASRBySchema},
		{name: "asr unknown provider", cfg: ConfigFile{Schema: "unknown/asr/v1"}, call: registerASRBySchema},
		{name: "asr missing api key", cfg: ConfigFile{Schema: "doubao/asr/v1", AppID: "app-id"}, call: registerASRBySchema},
		{name: "asr missing model name", cfg: ConfigFile{Schema: "doubao/asr/v1", AppID: "app-id", APIKey: "api-key", Models: []Entry{{}}}, call: registerASRBySchema},
		{name: "realtime invalid schema", cfg: ConfigFile{Schema: "bad"}, call: registerRealtimeBySchema},
		{name: "realtime unknown provider", cfg: ConfigFile{Schema: "unknown/realtime/v1"}, call: registerRealtimeBySchema},
		{name: "realtime missing api key", cfg: ConfigFile{Schema: "doubao/realtime/v1", AppID: "app-id"}, call: registerRealtimeBySchema},
		{name: "realtime missing model name", cfg: ConfigFile{Schema: "doubao/realtime/v1", AppID: "app-id", APIKey: "api-key", Models: []Entry{{}}}, call: registerRealtimeBySchema},
		{name: "tts invalid schema", cfg: ConfigFile{Schema: "bad"}, call: registerTTSBySchema},
		{name: "tts unknown provider", cfg: ConfigFile{Schema: "unknown/tts/v1"}, call: registerTTSBySchema},
		{name: "doubao tts missing api key", cfg: ConfigFile{Schema: "doubao/tts/v1", AppID: "app-id"}, call: registerTTSBySchema},
		{name: "doubao tts missing voice", cfg: ConfigFile{Schema: "doubao/tts/v1", AppID: "app-id", APIKey: "api-key", Voices: []VoiceEntry{{Name: "tts/bad"}}}, call: registerTTSBySchema},
		{name: "minimax tts missing key", cfg: ConfigFile{Schema: "minimax/tts/v1"}, call: registerTTSBySchema},
		{name: "minimax tts missing voice", cfg: ConfigFile{Schema: "minimax/tts/v1", APIKey: "mini-key", Voices: []VoiceEntry{{VoiceID: "voice"}}}, call: registerTTSBySchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.call(tc.cfg); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRegisterDoubaoASRUsesFixedTransportAudioDefaults(t *testing.T) {
	oldDefaultMux := transformers.DefaultMux
	oldASRMux := transformers.ASRMux
	transformers.DefaultMux = transformers.NewMux()
	transformers.ASRMux = transformers.NewASRMux()
	t.Cleanup(func() {
		transformers.DefaultMux = oldDefaultMux
		transformers.ASRMux = oldASRMux
	})

	names, err := registerDoubaoASR(ConfigFile{
		AppID:  "app-id",
		APIKey: "api-key",
		DefaultParams: map[string]any{
			"format":      "ogg_opus",
			"sample_rate": float64(24000),
			"bits":        float64(24),
			"channel":     float64(2),
			"language":    "en-US",
		},
		Models: []Entry{{Name: "asr/test", ResourceID: "resource/test"}},
	})
	if err != nil {
		t.Fatalf("registerDoubaoASR() error = %v", err)
	}
	if len(names) != 1 || names[0] != "asr/test" {
		t.Fatalf("names = %v, want [asr/test]", names)
	}

	registered := mustRegisteredDefaultTransformer(t, "asr/test")
	if got := stringField(t, registered, "format"); got != "pcm" {
		t.Fatalf("format = %q, want fixed default pcm", got)
	}
	if got := intField(t, registered, "sampleRate"); got != 16000 {
		t.Fatalf("sampleRate = %d, want fixed default 16000", got)
	}
	if got := intField(t, registered, "bits"); got != 16 {
		t.Fatalf("bits = %d, want fixed default 16", got)
	}
	if got := intField(t, registered, "channels"); got != 1 {
		t.Fatalf("channels = %d, want fixed default 1", got)
	}
	if got := stringField(t, registered, "language"); got != "en-US" {
		t.Fatalf("language = %q, want configured language", got)
	}

	_, err = registerDoubaoASR(ConfigFile{
		AppID:  "app-id",
		APIKey: "api-key",
		Models: []Entry{{Name: "asr/test"}},
	})
	if err == nil {
		t.Fatal("expected duplicate ASR registration error")
	}
}

func TestRegisterDoubaoRealtimeIgnoresOutputAudioDefaults(t *testing.T) {
	oldDefaultMux := transformers.DefaultMux
	transformers.DefaultMux = transformers.NewMux()
	t.Cleanup(func() {
		transformers.DefaultMux = oldDefaultMux
	})

	names, err := registerDoubaoRealtime(ConfigFile{
		AppID:  "app-id",
		APIKey: "api-key",
		DefaultParams: map[string]any{
			"format":      "pcm",
			"sample_rate": float64(8000),
			"model":       "SC",
		},
		Models: []Entry{{Name: "realtime/test", Voice: "voice/test"}},
	})
	if err != nil {
		t.Fatalf("registerDoubaoRealtime() error = %v", err)
	}
	if len(names) != 1 || names[0] != "realtime/test" {
		t.Fatalf("names = %v, want [realtime/test]", names)
	}

	registered := mustRegisteredDefaultTransformer(t, "realtime/test")
	if got := stringField(t, registered, "outputFormat"); got != "ogg_opus" {
		t.Fatalf("outputFormat = %q, want fixed default ogg_opus", got)
	}
	if got := intField(t, registered, "outputSampleRate"); got != 24000 {
		t.Fatalf("outputSampleRate = %d, want fixed default 24000", got)
	}
	if got := stringField(t, registered, "model"); got != "SC" {
		t.Fatalf("model = %q, want configured model", got)
	}
	if got := stringField(t, registered, "outputVoice"); got != "voice/test" {
		t.Fatalf("outputVoice = %q, want configured voice", got)
	}

	_, err = registerDoubaoRealtime(ConfigFile{
		APIKey: "api-key",
		Models: []Entry{{Name: "realtime/test"}},
	})
	if err == nil {
		t.Fatal("expected duplicate realtime registration error")
	}
}

func TestLoadFromDir_SkipsMissingCredentials(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a config with env var that's not set
	jsonContent := `{
		"schema": "openai/chat/v1",
		"type": "generator",
		"api_key": "$NONEXISTENT_API_KEY",
		"models": [{"name": "test/model", "model": "gpt-4"}]
	}`
	jsonPath := filepath.Join(tmpDir, "test.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not error, but should return empty names (config skipped)
	names, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names (skipped), got %d", len(names))
	}
}

func TestLoadFromDir_IgnoresNonConfigFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create non-config files
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("# README"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "script.sh"), []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not error
	names, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestLoadFromDir_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	names, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestVoiceEntry(t *testing.T) {
	v := VoiceEntry{
		Name:    "test/voice",
		VoiceID: "zh_female_test",
		Desc:    "Test description",
		Cluster: "test-cluster",
	}

	if v.Name != "test/voice" {
		t.Errorf("Name = %q, want %q", v.Name, "test/voice")
	}
	if v.VoiceID != "zh_female_test" {
		t.Errorf("VoiceID = %q, want %q", v.VoiceID, "zh_female_test")
	}
}

func TestEntry(t *testing.T) {
	e := Entry{
		Name:              "test/model",
		Model:             "gpt-4",
		SupportJSONOutput: true,
		SupportToolCalls:  true,
		TextOnly:          false,
		PromptRole:        genx.PromptRoleSystem,
	}

	if e.Name != "test/model" {
		t.Errorf("Name = %q, want %q", e.Name, "test/model")
	}
	if !e.SupportJSONOutput {
		t.Error("SupportJSONOutput should be true")
	}
	if !e.SupportToolCalls {
		t.Error("SupportToolCalls should be true")
	}
}

func TestRegisterSegmentorBySchema(t *testing.T) {
	cfg := ConfigFile{
		Schema: "genx/segmentor/v1",
		Type:   "segmentor",
		Models: []Entry{
			{Name: "seg/test-model", Model: "test/gen"},
		},
	}

	names, err := registerSegmentorBySchema(cfg)
	if err != nil {
		t.Fatalf("registerSegmentorBySchema() error = %v", err)
	}
	if len(names) != 1 || names[0] != "seg/test-model" {
		t.Errorf("names = %v, want [seg/test-model]", names)
	}
}

func TestRegisterProfilerBySchema(t *testing.T) {
	cfg := ConfigFile{
		Schema: "genx/profiler/v1",
		Type:   "profiler",
		Models: []Entry{
			{Name: "prof/test-model", Model: "test/gen"},
		},
	}

	names, err := registerProfilerBySchema(cfg)
	if err != nil {
		t.Fatalf("registerProfilerBySchema() error = %v", err)
	}
	if len(names) != 1 || names[0] != "prof/test-model" {
		t.Errorf("names = %v, want [prof/test-model]", names)
	}
}

func TestRegisterSegmentorBySchema_MissingModel(t *testing.T) {
	cfg := ConfigFile{
		Schema: "genx/segmentor/v1",
		Type:   "segmentor",
		Models: []Entry{
			{Name: "seg/bad"},
		},
	}

	_, err := registerSegmentorBySchema(cfg)
	if err == nil {
		t.Error("expected error for missing model (generator pattern)")
	}
}

func TestRegisterProfilerBySchema_MissingName(t *testing.T) {
	cfg := ConfigFile{
		Schema: "genx/profiler/v1",
		Type:   "profiler",
		Models: []Entry{
			{Model: "test/gen"},
		},
	}

	_, err := registerProfilerBySchema(cfg)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParseConfig_SegmentorYAML(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `
schema: "genx/segmentor/v1"
type: "segmentor"
models:
  - name: "seg/qwen-turbo"
    model: "qwen/turbo"
`
	yamlPath := filepath.Join(tmpDir, "segmentor.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig(yamlPath)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Type != "segmentor" {
		t.Errorf("Type = %q, want %q", cfg.Type, "segmentor")
	}
	if len(cfg.Models) != 1 {
		t.Fatalf("len(Models) = %d, want 1", len(cfg.Models))
	}
	if cfg.Models[0].Name != "seg/qwen-turbo" {
		t.Errorf("Models[0].Name = %q, want %q", cfg.Models[0].Name, "seg/qwen-turbo")
	}
	if cfg.Models[0].Model != "qwen/turbo" {
		t.Errorf("Models[0].Model = %q, want %q", cfg.Models[0].Model, "qwen/turbo")
	}
}

func TestLoadFromDir_SegmentorAndProfiler(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a segmentor config
	segYAML := `
schema: "genx/segmentor/v1"
type: "segmentor"
models:
  - name: "seg/loader-test"
    model: "some/gen"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "segmentor.yaml"), []byte(segYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a profiler config
	profYAML := `
schema: "genx/profiler/v1"
type: "profiler"
models:
  - name: "prof/loader-test"
    model: "some/gen"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "profiler.yaml"), []byte(profYAML), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if len(names) != 2 {
		t.Errorf("len(names) = %d, want 2; names = %v", len(names), names)
	}

	// Verify both names are registered.
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["seg/loader-test"] {
		t.Error("seg/loader-test not registered")
	}
	if !found["prof/loader-test"] {
		t.Error("prof/loader-test not registered")
	}
}

func TestRegisterLabelerBySchema(t *testing.T) {
	labelers.DefaultMux = labelers.NewMux()
	cfg := ConfigFile{
		Schema: "genx/labeler/v1",
		Type:   "labeler",
		Models: []Entry{{Name: "labeler/qwen-flash", Model: "qwen/flash"}},
	}
	names, err := registerLabelerBySchema(cfg)
	if err != nil {
		t.Fatalf("registerLabelerBySchema() error = %v", err)
	}
	if len(names) != 1 || names[0] != "labeler/qwen-flash" {
		t.Fatalf("names = %v, want [labeler/qwen-flash]", names)
	}

	got, err := labelers.Get("labeler/qwen-flash")
	if err != nil {
		t.Fatalf("labelers.Get() error = %v", err)
	}
	if got.Model() != "qwen/flash" {
		t.Fatalf("Model() = %q, want %q", got.Model(), "qwen/flash")
	}
}

func TestRegisterLabelerBySchemaMissingModel(t *testing.T) {
	labelers.DefaultMux = labelers.NewMux()
	_, err := registerLabelerBySchema(ConfigFile{
		Schema: "genx/labeler/v1",
		Type:   "labeler",
		Models: []Entry{{Name: "labeler/qwen-flash"}},
	})
	if err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestLabelerConfigMissingName(t *testing.T) {
	labelers.DefaultMux = labelers.NewMux()
	_, err := registerLabelerBySchema(ConfigFile{
		Schema: "genx/labeler/v1",
		Type:   "labeler",
		Models: []Entry{{Model: "qwen/flash"}},
	})
	if err == nil {
		t.Fatal("expected missing name error")
	}
}

func TestRegisterBySchemaLabelerType(t *testing.T) {
	labelers.DefaultMux = labelers.NewMux()
	names, err := registerBySchema(ConfigFile{
		Schema: "genx/labeler/v1",
		Type:   "labeler",
		Models: []Entry{{Name: "labeler/demo", Model: "qwen/demo"}},
	})
	if err != nil {
		t.Fatalf("registerBySchema() error = %v", err)
	}
	if len(names) != 1 || names[0] != "labeler/demo" {
		t.Fatalf("names = %v, want [labeler/demo]", names)
	}
}

func mustRegisteredDefaultTransformer(t *testing.T, pattern string) genx.Transformer {
	t.Helper()
	muxValue := reflect.ValueOf(transformers.DefaultMux).Elem().FieldByName("mux")
	muxValue = reflect.NewAt(muxValue.Type(), unsafe.Pointer(muxValue.UnsafeAddr())).Elem()
	results := muxValue.MethodByName("Get").Call([]reflect.Value{reflect.ValueOf(pattern)})
	if len(results) != 2 || !results[1].Bool() {
		t.Fatalf("transformer %q was not registered", pattern)
	}
	ptr := results[0].Interface().(*genx.Transformer)
	if ptr == nil || *ptr == nil {
		t.Fatalf("transformer %q is nil", pattern)
	}
	return *ptr
}

func stringField(t *testing.T, value any, name string) string {
	t.Helper()
	field := reflect.ValueOf(value).Elem().FieldByName(name)
	field = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	return field.String()
}

func intField(t *testing.T, value any, name string) int {
	t.Helper()
	field := reflect.ValueOf(value).Elem().FieldByName(name)
	field = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	return int(field.Int())
}

func testingContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
