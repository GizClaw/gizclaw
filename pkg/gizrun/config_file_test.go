package gizrun

import (
	"os"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/configfile"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/log/volclog"
	"github.com/goccy/go-yaml"
)

func TestEnvConfigApplyExpandsBeforeSetting(t *testing.T) {
	t.Setenv("BASE_ENV_VALUE", "base")
	os.Unsetenv("GIZRUN_ENV_FIRST")
	os.Unsetenv("GIZRUN_ENV_SECOND")
	t.Cleanup(func() {
		os.Unsetenv("GIZRUN_ENV_FIRST")
		os.Unsetenv("GIZRUN_ENV_SECOND")
	})

	config := envConfig{
		"GIZRUN_ENV_FIRST":  "${BASE_ENV_VALUE}",
		"GIZRUN_ENV_SECOND": "${GIZRUN_ENV_FIRST}",
	}
	if err := config.apply(); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if got := os.Getenv("GIZRUN_ENV_FIRST"); got != "base" {
		t.Fatalf("GIZRUN_ENV_FIRST = %q, want base", got)
	}
	if got := os.Getenv("GIZRUN_ENV_SECOND"); got != "" {
		t.Fatalf("GIZRUN_ENV_SECOND = %q, want empty because env expansion is not sequential", got)
	}
}

func TestEnvConfigApplyRejectsEmptyKey(t *testing.T) {
	err := envConfig{" ": "value"}.apply()
	if err == nil || !strings.Contains(err.Error(), "key is empty") {
		t.Fatalf("apply err = %v, want empty key error", err)
	}
}

func TestEnvConfigParser(t *testing.T) {
	parser := configfile.NewYamlParser()
	parser.Register("env", configfile.ParseFunc(func(data []byte) (any, error) {
		var config envConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, err
		}
		return config, nil
	}))

	value, err := parser.Parse([]byte(`
env:
  OPENAI_API_KEY: test-key
`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	config, ok := value.(configfile.ConfigFile)
	if !ok {
		t.Fatalf("Parse value type = %T, want configfile.ConfigFile", value)
	}
	section, ok := config.Config("env")
	if !ok {
		t.Fatal("env section not found")
	}
	env, ok := section.(envConfig)
	if !ok {
		t.Fatalf("env section type = %T, want envConfig", section)
	}
	if env["OPENAI_API_KEY"] != "test-key" {
		t.Fatalf("OPENAI_API_KEY = %q, want test-key", env["OPENAI_API_KEY"])
	}
}

func TestVolcLogHandlerRequiresConfig(t *testing.T) {
	if _, err := volclog.NewHandler(volclog.Config{}); err == nil {
		t.Fatal("volclog.NewHandler err = nil, want validation error")
	}
}

func TestVolcLogConfigParser(t *testing.T) {
	registry := configfile.NewYamlParser()
	registry.Register("volc_log", configfile.ParseFunc(func(data []byte) (any, error) {
		var config volcLogConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, err
		}
		return config, nil
	}))
	value, err := registry.Parse([]byte(`
volc_log:
  enabled: true
  endpoint: tls-cn.example.com
  topic_id: topic
`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	config, ok := value.(configfile.ConfigFile)
	if !ok {
		t.Fatalf("Parse value type = %T, want configfile.ConfigFile", value)
	}
	section, ok := config.Config("volc_log")
	if !ok {
		t.Fatal("volc_log section not found")
	}
	logSection, ok := section.(volcLogConfig)
	if !ok {
		t.Fatalf("volc_log section type = %T, want volcLogConfig", section)
	}
	if !logSection.Enabled {
		t.Fatal("volc enabled = false, want true")
	}
	if logSection.Endpoint != "tls-cn.example.com" {
		t.Fatalf("volc endpoint = %q, want tls-cn.example.com", logSection.Endpoint)
	}
	if logSection.TopicID != "topic" {
		t.Fatalf("volc topic id = %q, want topic", logSection.TopicID)
	}
}
