package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func seedSlowTTS(ctx context.Context, api *adminhttp.ClientWithResponses, profile, tokenID, token string) error {
	return seedVoiceFixture(ctx, api, profile, tokenID, token, "slow-tts")
}

func seedVoiceFixture(ctx context.Context, api *adminhttp.ClientWithResponses, profile, tokenID, token, fixture string) error {
	// Deliberately unusable outside the provider-only test build overlay.
	if err := seedProvider(ctx, api, volcCredentials{appID: "local-fixture", apiKey: "local-fixture", searchKey: "local-fixture", arkKey: "local-fixture", accessKeyID: "local-fixture", accessKey: "local-fixture"}); err != nil {
		return err
	}
	spec := runtimeProfileSpec(true)
	if fixture == "speaker-segments" {
		for _, name := range []string{"default", "fox", "bird"} {
			id := "speaker-" + name
			var voiceData apitypes.VoiceProviderData
			if err := voiceData.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{ResourceId: new(ttsResource), VoiceId: new(id)}); err != nil {
				return err
			}
			if err := upsertVoice(ctx, api, adminhttp.VoiceUpsert{Id: id, Source: apitypes.VoiceSourceManual, ProviderData: &voiceData, Provider: apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindVolcTenant, Id: "volc-main"}}); err != nil {
				return err
			}
			(*spec.Resources.Voices)["story."+name] = binding(id, id, id)
		}
	}
	workflows := map[string]apitypes.RuntimeProfileBinding{}
	for _, driver := range []string{"eino", "flowcraft"} {
		data, err := os.ReadFile("tests/gizclaw-e2e/testdata/" + fixture + "/" + driver + ".json")
		if err != nil {
			return err
		}
		var workflow apitypes.WorkflowSpec
		if err := json.Unmarshal(data, &workflow); err != nil {
			return err
		}
		id := fixture + "-" + driver
		if err := upsertWorkflow(ctx, api, adminhttp.WorkflowUpsert{Id: id, Spec: workflow}); err != nil {
			return err
		}
		workflows[id] = binding(id, id, id)
	}
	for alias, workflow := range workflows {
		spec.Workflows[alias] = workflow
	}
	if err := upsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{Id: profile, Spec: spec}); err != nil {
		return err
	}
	_, err := upsertRegistrationToken(ctx, api, adminhttp.RegistrationTokenUpsert{Id: tokenID, Token: token, RuntimeProfileId: profile})
	if err == nil {
		fmt.Println(fixture + " fixture seeded")
	}
	return err
}
