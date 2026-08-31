//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"os"
	"testing"

	memorymem0 "github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
)

func TestLoCoMoMem0SelfHosted(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{embedding: true})
	endpoint := os.Getenv("GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL")
	if err := validateRequired(map[string]string{
		"GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL": endpoint,
	}, "GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL"); err != nil {
		t.Fatal(err)
	}
	store, err := memorymem0.New(memorymem0.Config{
		Endpoint: endpoint,
		Flavor:   memorymem0.SelfHosted,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := "mem0_self_hosted"
	fingerprint := configFingerprint(profile, endpoint, "mem0ai-2.0.3", settings.extractionModel, settings.embeddingModel)
	runLiveProfile(t, settings, profile, fingerprint, reportModels{
		Extraction: settings.extractionModel,
		Embedding:  settings.embeddingModel,
	}, store, nil)
}
