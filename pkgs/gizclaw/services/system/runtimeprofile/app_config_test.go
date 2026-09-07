package runtimeprofile

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func appConfigUpsert(config apitypes.RuntimeProfileAppConfig) adminhttp.RuntimeProfileUpsert {
	return adminhttp.RuntimeProfileUpsert{
		Id: "test-profile",
		Spec: apitypes.RuntimeProfileSpec{
			Workflows: apitypes.RuntimeProfileWorkflows{
				System:      apitypes.RuntimeProfileSystemWorkflows{Pet: "pet-care"},
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
			},
			AppConfig: &config,
		},
	}
}

func TestNormalizeProfileKeepsAppConfigValuesVerbatim(t *testing.T) {
	t.Parallel()
	value := "  {\n  \"theme\": \"深色\"\n}  "
	normalized, err := normalizeProfile(appConfigUpsert(apitypes.RuntimeProfileAppConfig{
		" ui.theme ": value,
		"plain-text": "not json at all",
		"empty":      "",
	}), "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	config := *normalized.Spec.AppConfig
	if got, ok := config["ui.theme"]; !ok || got != value {
		t.Fatalf("app_config[ui.theme] = %q, ok = %v; want the value verbatim", got, ok)
	}
	if got := config["plain-text"]; got != "not json at all" {
		t.Fatalf("app_config[plain-text] = %q", got)
	}
	if got, ok := config["empty"]; !ok || got != "" {
		t.Fatalf("app_config[empty] = %q, ok = %v", got, ok)
	}
	if len(config) != 3 {
		t.Fatalf("normalized app_config = %#v", config)
	}
}

func TestNormalizeProfileRejectsInvalidAppConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		config apitypes.RuntimeProfileAppConfig
		want   string
	}{
		{
			name:   "uppercase key",
			config: apitypes.RuntimeProfileAppConfig{"UI.Theme": "dark"},
			want:   "app_config key",
		},
		{
			name:   "underscore key",
			config: apitypes.RuntimeProfileAppConfig{"ui_theme": "dark"},
			want:   "app_config key",
		},
		{
			name:   "empty key",
			config: apitypes.RuntimeProfileAppConfig{"   ": "dark"},
			want:   "app_config key",
		},
		{
			name:   "duplicate after trim",
			config: apitypes.RuntimeProfileAppConfig{"ui.theme": "dark", " ui.theme": "light"},
			want:   "duplicated after normalization",
		},
		{
			name:   "oversized value",
			config: apitypes.RuntimeProfileAppConfig{"ui.theme": strings.Repeat("x", MaxAppConfigValueBytes+1)},
			want:   "must not exceed 4096 bytes",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeProfile(appConfigUpsert(testCase.config), "")
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("normalizeProfile() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestNormalizeProfileRejectsTooManyAppConfigEntries(t *testing.T) {
	t.Parallel()
	config := make(apitypes.RuntimeProfileAppConfig, MaxAppConfigEntries+1)
	for i := range MaxAppConfigEntries + 1 {
		config[appConfigTestKey(i)] = "value"
	}
	_, err := normalizeProfile(appConfigUpsert(config), "")
	if err == nil || !strings.Contains(err.Error(), "must not exceed 64 entries") {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
}

func TestAppConfigParticipatesInProfileRevision(t *testing.T) {
	t.Parallel()
	first, err := normalizeProfile(appConfigUpsert(apitypes.RuntimeProfileAppConfig{"ui.theme": "dark"}), "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	if err := setProfileRevision(&first); err != nil {
		t.Fatalf("setProfileRevision() error = %v", err)
	}
	second, err := normalizeProfile(appConfigUpsert(apitypes.RuntimeProfileAppConfig{"ui.theme": "light"}), "")
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	if err := setProfileRevision(&second); err != nil {
		t.Fatalf("setProfileRevision() error = %v", err)
	}
	if first.Revision == second.Revision {
		t.Fatalf("revision %q did not change with app_config", first.Revision)
	}
}

func appConfigTestKey(index int) string {
	digits := "0123456789"
	return "key-" + string(digits[index/10]) + string(digits[index%10])
}
