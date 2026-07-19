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
	endpoint := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_MEM0_ENDPOINT")
	apiKey := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_MEM0_API_KEY")
	apiKeyID := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_API_KEY_ID")
	projectID := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_MEMORY_PROJECT_ID")
	accessKeyID := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_SECRET")
	identity := os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_DEFAULT_FINGERPRINT")
	if err := validateRequired(map[string]string{"endpoint": endpoint, "fingerprint": identity}, "endpoint", "fingerprint"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(apiKey) == "" {
		if err := validateRequired(map[string]string{
			"api_key_id": apiKeyID, "project_id": projectID,
			"access_key_id": accessKeyID, "access_key_secret": accessKeySecret,
		}, "api_key_id", "project_id", "access_key_id", "access_key_secret"); err != nil {
			t.Fatal(err)
		}
	}
	store, err := memoryvolc.Open(context.Background(), memoryvolc.Config{
		Mem0:     memorymem0.Config{Endpoint: endpoint, APIKey: apiKey, Flavor: memorymem0.Platform},
		APIKeyID: apiKeyID, MemoryProjectID: projectID,
		ControlEndpoint: os.Getenv("GIZCLAW_LOCOMO_E2E_VOLC_CONTROL_ENDPOINT"),
		Region:          envOr("GIZCLAW_LOCOMO_E2E_VOLC_REGION", "cn-beijing"),
		AccessKeyID:     accessKeyID, AccessKeySecret: accessKeySecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := "volc_agentkit_default"
	fingerprint := configFingerprint(profile, endpoint, identity, apiKeyID, projectID)
	runLiveProfile(t, settings, profile, fingerprint, store, nil)
}
