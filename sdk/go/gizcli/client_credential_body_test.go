package gizcli

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

//go:fix inline
func testStringPtr(value string) *string { return new(value) }

func testRPCOpenAICredentialBody(apiKey string) rpcapi.CredentialBody {
	var body rpcapi.CredentialBody
	if err := body.FromOpenAICredentialBody(rpcapi.OpenAICredentialBody{ApiKey: new(apiKey)}); err != nil {
		panic(err)
	}
	return body
}
