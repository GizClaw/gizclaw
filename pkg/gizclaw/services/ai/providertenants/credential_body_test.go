package providertenants

import (
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func testStringPtr(value string) *string { return &value }

func testOpenAICredentialBody(apiKey string) apitypes.CredentialBody {
	var body apitypes.CredentialBody
	if err := body.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: testStringPtr(apiKey)}); err != nil {
		panic(err)
	}
	return body
}

func testMiniMaxCredentialBody(apiKey string) apitypes.CredentialBody {
	return testMiniMaxCredentialBodyFromStrings(map[string]string{"api_key": apiKey})
}

func testMiniMaxCredentialBodyFromStrings(values map[string]string) apitypes.CredentialBody {
	typed := apitypes.MiniMaxCredentialBody{}
	for key, value := range values {
		value := value
		switch key {
		case "api_key":
			typed.ApiKey = &value
		case "token":
			typed.Token = &value
		case "base_url":
			typed.BaseUrl = &value
		case "voice_base_url":
			typed.VoiceBaseUrl = &value
		case "minimax_voice_base_url":
			typed.MinimaxVoiceBaseUrl = &value
		default:
			panic("unsupported minimax credential field: " + key)
		}
	}
	var body apitypes.CredentialBody
	if err := body.FromMiniMaxCredentialBody(typed); err != nil {
		panic(err)
	}
	return body
}

func testVolcCredentialBodyFromStrings(values map[string]string) apitypes.CredentialBody {
	typed := apitypes.VolcCredentialBody{}
	for key, value := range values {
		value := value
		switch key {
		case "app_id":
			typed.AppId = &value
		case "ark_api_key":
			typed.ArkApiKey = &value
		case "openapi_access_key_id":
			typed.OpenapiAccessKeyId = &value
		case "secret_access_key":
			typed.SecretAccessKey = &value
		case "session_token":
			typed.SessionToken = &value
		case "speech_token":
			typed.SpeechToken = &value
		case "websearch_api_key":
			typed.WebsearchApiKey = &value
		default:
			panic("unsupported volc credential field: " + key)
		}
	}
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(typed); err != nil {
		panic(err)
	}
	return body
}

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
