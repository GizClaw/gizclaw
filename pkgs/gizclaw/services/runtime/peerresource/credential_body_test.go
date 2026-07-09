package peerresource

import "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"

func testStringPtr(value string) *string { return &value }

func testRPCOpenAICredentialBody(apiKey string) rpcapi.CredentialBody {
	var body rpcapi.CredentialBody
	if err := body.FromOpenAICredentialBody(rpcapi.OpenAICredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testRPCCredentialBodyString(body rpcapi.CredentialBody, key string) string {
	openAI, err := body.AsOpenAICredentialBody()
	if err != nil || key != "api_key" || openAI.ApiKey == nil {
		return ""
	}
	return *openAI.ApiKey
}
