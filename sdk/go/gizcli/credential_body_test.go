package gizcli

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func testStringPtr(value string) *string { return &value }

func testRPCOpenAICredentialBody(apiKey string) rpcapi.CredentialBody {
	var body rpcapi.CredentialBody
	if err := body.FromOpenAICredentialBody(rpcapi.OpenAICredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}
