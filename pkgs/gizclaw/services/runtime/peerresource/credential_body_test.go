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
		ArkApiKey:          testStringPtr("volc-ark"),
		OpenapiAccessKeyId: testStringPtr("ak-id"),
		OpenapiAccessKey:   testStringPtr("ak-secret"),
		SearchApiKey:       testStringPtr("search-key"),
		SpeechApiKey:       testStringPtr("speech-key"),
		SpeechAppId:        testStringPtr("volc-app"),
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
	if volc.SpeechAppId == nil || *volc.SpeechAppId != "volc-app" ||
		volc.SpeechApiKey == nil || *volc.SpeechApiKey != "speech-key" ||
		volc.ArkApiKey == nil || *volc.ArkApiKey != "volc-ark" ||
		volc.SearchApiKey == nil || *volc.SearchApiKey != "search-key" ||
		volc.OpenapiAccessKeyId == nil || *volc.OpenapiAccessKeyId != "ak-id" ||
		volc.OpenapiAccessKey == nil || *volc.OpenapiAccessKey != "ak-secret" {
		t.Fatalf("volc credential body = %#v", volc)
	}
	if _, err := got.Body.AsOpenAICredentialBody(); err == nil {
		t.Fatal("apiCredentialToRPC() encoded volc credential as OpenAI body")
	}

	roundTrip, err := rpcCredentialBodyToAPI(got.Body)
	if err != nil {
		t.Fatalf("rpcCredentialBodyToAPI() error = %v", err)
	}
	roundTripVolc, err := roundTrip.AsVolcCredentialBody()
	if err != nil {
		t.Fatalf("AsVolcCredentialBody(round trip) error = %v", err)
	}
	if roundTripVolc.SpeechAppId == nil || *roundTripVolc.SpeechAppId != "volc-app" ||
		roundTripVolc.SpeechApiKey == nil || *roundTripVolc.SpeechApiKey != "speech-key" ||
		roundTripVolc.ArkApiKey == nil || *roundTripVolc.ArkApiKey != "volc-ark" ||
		roundTripVolc.SearchApiKey == nil || *roundTripVolc.SearchApiKey != "search-key" ||
		roundTripVolc.OpenapiAccessKeyId == nil || *roundTripVolc.OpenapiAccessKeyId != "ak-id" ||
		roundTripVolc.OpenapiAccessKey == nil || *roundTripVolc.OpenapiAccessKey != "ak-secret" {
		t.Fatalf("round-trip volc credential body = %#v", roundTripVolc)
	}
}
