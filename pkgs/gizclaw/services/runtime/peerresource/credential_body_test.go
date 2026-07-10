package peerresource

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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

func testRPCCredentialBodyString(body rpcapi.CredentialBody, key string) string {
	openAI, err := body.AsOpenAICredentialBody()
	if err != nil || key != "api_key" || openAI.ApiKey == nil {
		return ""
	}
	return *openAI.ApiKey
}

func TestAPICredentialToRPCUsesProviderForBodyUnion(t *testing.T) {
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		AppId:              testStringPtr("volc-app"),
		OpenapiAccessKeyId: testStringPtr("ak-id"),
		OpenapiAccessKey:   testStringPtr("ak-secret"),
	}); err != nil {
		t.Fatalf("FromVolcCredentialBody() error = %v", err)
	}

	got, err := apiCredentialToRPC(apitypes.Credential{
		Name:     "volc-credential",
		Provider: "volc",
		Body:     body,
	})
	if err != nil {
		t.Fatalf("apiCredentialToRPC() error = %v", err)
	}
	volc, err := got.Body.AsVolcCredentialBody()
	if err != nil {
		t.Fatalf("AsVolcCredentialBody() error = %v", err)
	}
	if volc.AppId == nil || *volc.AppId != "volc-app" || volc.OpenapiAccessKeyId == nil || *volc.OpenapiAccessKeyId != "ak-id" {
		t.Fatalf("volc credential body = %#v", volc)
	}
	if _, err := got.Body.AsOpenAICredentialBody(); err == nil {
		t.Fatal("apiCredentialToRPC() encoded volc credential as OpenAI body")
	}
}
