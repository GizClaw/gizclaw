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
	// Deliberately unusable outside the provider-only test build overlay.
	if err := seedProvider(ctx, api, volcCredentials{appID: "local-fixture", apiKey: "local-fixture", searchKey: "local-fixture", arkKey: "local-fixture", accessKeyID: "local-fixture", accessKey: "local-fixture"}); err != nil {
		return err
	}
	spec := runtimeProfileSpec(true)
	workflows := map[string]apitypes.RuntimeProfileBinding{}
	for _, driver := range []string{"eino", "flowcraft"} {
		data, err := os.ReadFile("tests/gizclaw-e2e/testdata/slow-tts/" + driver + ".json")
		if err != nil {
			return err
		}
		var workflow apitypes.WorkflowSpec
		if err := json.Unmarshal(data, &workflow); err != nil {
			return err
		}
		id := "slow-tts-" + driver
		if err := upsertWorkflow(ctx, api, adminhttp.WorkflowUpsert{Id: id, Spec: workflow}); err != nil {
			return err
		}
		workflows[id] = binding(id, id, id)
	}
	spec.Workflows.Collections["assistants"] = workflows
	if err := upsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{Id: profile, Spec: spec}); err != nil {
		return err
	}
	_, err := upsertRegistrationToken(ctx, api, adminhttp.RegistrationTokenUpsert{Id: tokenID, Token: token, RuntimeProfileId: profile})
	if err == nil {
		fmt.Println("slow-tts fixture seeded")
	}
	return err
}
