//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"os"
	"testing"

	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
)

func TestLoCoMoMem0PlatformDefault(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{})
	endpoint := os.Getenv("GIZCLAW_LOCOMO_E2E_MEM0_DEFAULT_ENDPOINT")
	apiKey := os.Getenv("GIZCLAW_LOCOMO_E2E_MEM0_DEFAULT_API_KEY")
	identity := os.Getenv("GIZCLAW_LOCOMO_E2E_MEM0_DEFAULT_FINGERPRINT")
	if err := validateRequired(map[string]string{
		"endpoint": endpoint, "api_key": apiKey, "fingerprint": identity,
	}, "endpoint", "api_key", "fingerprint"); err != nil {
		t.Fatal(err)
	}
	store, err := memorymem0.New(memorymem0.Config{
		Endpoint: endpoint, APIKey: apiKey, Flavor: memorymem0.Platform,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := "mem0_platform_default"
	fingerprint := configFingerprint(profile, endpoint, identity)
	runLiveProfile(t, settings, profile, fingerprint, store, nil)
}
