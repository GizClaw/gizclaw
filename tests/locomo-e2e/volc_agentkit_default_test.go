//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"os"
	"strings"
	"testing"

	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
	memoryvolc "github.com/GizClaw/gizclaw-go/pkgs/store/memory/volc"
)

func TestLoCoMoVolcAgentKitDefault(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{})
	config, identity := requireVolcConfig(t)
	store, err := memoryvolc.Open(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	profile := "volc_agentkit_default"
	fingerprint := configFingerprint(profile, config.Mem0.Endpoint, identity, config.APIKeyID, config.MemoryProjectID)
	runLiveProfile(t, settings, profile, fingerprint, reportModels{}, store, nil)
}

func requireVolcConfig(t *testing.T) (memoryvolc.Config, string) {
	t.Helper()
	endpoint := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_MEM0_ENDPOINT")
	apiKey := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_MEM0_API_KEY")
	apiKeyID := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_API_KEY_ID")
	projectID := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_MEMORY_PROJECT_ID")
	accessKeyID := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_SECRET")
	identity := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_DEFAULT_FINGERPRINT")
	if err := validateRequired(map[string]string{
		"GIZCLAW_LOCOMO_E2E_VOLC_MEM0_ENDPOINT":       endpoint,
		"GIZCLAW_LOCOMO_E2E_VOLC_DEFAULT_FINGERPRINT": identity,
	}, "GIZCLAW_LOCOMO_E2E_VOLC_MEM0_ENDPOINT", "GIZCLAW_LOCOMO_E2E_VOLC_DEFAULT_FINGERPRINT"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(apiKey) == "" {
		if err := validateRequired(map[string]string{
			"GIZCLAW_LOCOMO_E2E_VOLC_API_KEY_ID":        apiKeyID,
			"GIZCLAW_LOCOMO_E2E_VOLC_MEMORY_PROJECT_ID": projectID,
			"GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_ID":     accessKeyID,
			"GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_SECRET": accessKeySecret,
		}, "GIZCLAW_LOCOMO_E2E_VOLC_API_KEY_ID", "GIZCLAW_LOCOMO_E2E_VOLC_MEMORY_PROJECT_ID", "GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_ID", "GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_SECRET"); err != nil {
			t.Fatal(err)
		}
	}
	return memoryvolc.Config{
		Mem0:     memorymem0.Config{Endpoint: endpoint, APIKey: apiKey},
		APIKeyID: apiKeyID, MemoryProjectID: projectID,
		ControlEndpoint: os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_CONTROL_ENDPOINT"),
		Region:          envOr("GIZCLAW_LOCOMO_E2E_VOLC_REGION", "cn-beijing"),
		AccessKeyID:     accessKeyID, AccessKeySecret: accessKeySecret,
	}, identity
}
