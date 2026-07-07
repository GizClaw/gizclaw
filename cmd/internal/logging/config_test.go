package logging

import (
	"reflect"
	"testing"
)

func TestPrepareConfigDefaultsAndDisabledVolc(t *testing.T) {
	got, err := PrepareConfig(Config{})
	if err != nil {
		t.Fatalf("PrepareConfig() error = %v", err)
	}
	if got.Level != "info" {
		t.Fatalf("default level = %q, want info", got.Level)
	}
	if got.Volc.Enabled {
		t.Fatal("Volc should be disabled by default")
	}
}

func TestPrepareConfigRejectsInvalidLevel(t *testing.T) {
	if _, err := PrepareConfig(Config{Level: "verbose"}); err == nil {
		t.Fatal("PrepareConfig should reject invalid level")
	}
}

func TestPrepareConfigAllowsDisabledVolcPlaceholders(t *testing.T) {
	_, err := PrepareConfig(Config{
		Level: "debug",
		Volc:  VolcConfig{Endpoint: "https://tls-cn-beijing.volces.com"},
	})
	if err != nil {
		t.Fatalf("disabled Volc config should not require credentials: %v", err)
	}
}

func TestPrepareConfigRejectsEnabledVolcMissingFields(t *testing.T) {
	base := VolcConfig{
		Enabled:         true,
		Endpoint:        "https://tls-cn-beijing.volces.com",
		Region:          "cn-beijing",
		TopicID:         "topic",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	}
	tests := []struct {
		name string
		edit func(*VolcConfig)
	}{
		{name: "endpoint", edit: func(c *VolcConfig) { c.Endpoint = "" }},
		{name: "region", edit: func(c *VolcConfig) { c.Region = "" }},
		{name: "topic", edit: func(c *VolcConfig) { c.TopicID = "" }},
		{name: "access key id", edit: func(c *VolcConfig) { c.AccessKeyID = "" }},
		{name: "access key secret", edit: func(c *VolcConfig) { c.AccessKeySecret = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.edit(&cfg)
			if _, err := PrepareConfig(Config{Volc: cfg}); err == nil {
				t.Fatal("PrepareConfig should reject missing required field")
			}
		})
	}
}

func TestPrepareConfigExpandsEnvironment(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_LOG_LEVEL", "debug")
	t.Setenv("GIZCLAW_TEST_VOLC_ENDPOINT", "https://tls-cn-beijing.volces.com")
	t.Setenv("GIZCLAW_TEST_VOLC_REGION", "cn-beijing")
	t.Setenv("GIZCLAW_TEST_VOLC_TOPIC", "topic")
	t.Setenv("GIZCLAW_TEST_VOLC_AK", "ak")
	t.Setenv("GIZCLAW_TEST_VOLC_SK", "sk")
	got, err := PrepareConfig(Config{
		Level: "$GIZCLAW_TEST_LOG_LEVEL",
		Volc: VolcConfig{
			Enabled:         true,
			Endpoint:        "$GIZCLAW_TEST_VOLC_ENDPOINT",
			Region:          "$GIZCLAW_TEST_VOLC_REGION",
			TopicID:         "$GIZCLAW_TEST_VOLC_TOPIC",
			AccessKeyID:     "$GIZCLAW_TEST_VOLC_AK",
			AccessKeySecret: "$GIZCLAW_TEST_VOLC_SK",
		},
	})
	if err != nil {
		t.Fatalf("PrepareConfig() error = %v", err)
	}
	if got.Level != "debug" {
		t.Fatalf("expanded level = %q", got.Level)
	}
	if got.Volc.Endpoint != "https://tls-cn-beijing.volces.com" || got.Volc.AccessKeySecret != "sk" {
		t.Fatalf("expanded config = %+v", got.Volc)
	}
}

func TestConfigSurfaceStaysMinimal(t *testing.T) {
	got := yamlFields(reflect.TypeOf(Config{}), "")
	want := map[string]bool{
		"level":                  true,
		"volc":                   true,
		"volc.enabled":           true,
		"volc.endpoint":          true,
		"volc.region":            true,
		"volc.topic_id":          true,
		"volc.access_key_id":     true,
		"volc.access_key_secret": true,
	}
	for field := range got {
		if !want[field] {
			t.Fatalf("unexpected public log config field %q", field)
		}
	}
	for field := range want {
		if !got[field] {
			t.Fatalf("missing public log config field %q", field)
		}
	}
}

func yamlFields(typ reflect.Type, prefix string) map[string]bool {
	fields := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := field.Tag.Get("yaml")
		if comma := len(name); comma > 0 {
			for j, r := range name {
				if r == ',' {
					comma = j
					break
				}
			}
			name = name[:comma]
		}
		if name == "" || name == "-" {
			continue
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		fields[key] = true
		if field.Type.Kind() == reflect.Struct {
			for nested := range yamlFields(field.Type, key) {
				fields[nested] = true
			}
		}
	}
	return fields
}
