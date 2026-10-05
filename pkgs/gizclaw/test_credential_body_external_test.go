package gizclaw_test

import (
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func testCredentialBodyString(body apitypes.CredentialBody, key string) string {
	data, err := body.MarshalJSON()
	if err != nil {
		return ""
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return ""
	}
	return values[key]
}

func testEinoWorkspaceParameters() *apitypes.WorkspaceParameters {
	var params apitypes.WorkspaceParameters
	if err := params.FromEinoWorkspaceParameters(apitypes.EinoWorkspaceParameters{
		AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino,
	}); err != nil {
		panic(err)
	}
	return &params
}

//go:fix inline
func stringPtr(value string) *string { return new(value) }
