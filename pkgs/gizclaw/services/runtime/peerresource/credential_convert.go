package peerresource

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func rpcCredentialUpsertToAdmin(in rpcapi.Credential) (adminhttp.CredentialUpsert, error) {
	body, err := rpcCredentialBodyToAPI(in.Body)
	if err != nil {
		return adminhttp.CredentialUpsert{}, err
	}
	return adminhttp.CredentialUpsert{
		Body:        body,
		Description: in.Description,
		Name:        in.Name,
		Provider:    in.Provider,
	}, nil
}

func rpcCredentialBodyToAPI(in rpcapi.CredentialBody) (apitypes.CredentialBody, error) {
	var out apitypes.CredentialBody
	if typed, err := in.AsOpenAICredentialBody(); err == nil {
		err = out.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{
			ApiKey:       typed.ApiKey,
			BaseUrl:      typed.BaseUrl,
			Organization: typed.Organization,
			Project:      typed.Project,
			Token:        typed.Token,
		})
		return out, err
	}
	if typed, err := in.AsGeminiCredentialBody(); err == nil {
		err = out.FromGeminiCredentialBody(apitypes.GeminiCredentialBody{
			ApiKey:  typed.ApiKey,
			BaseUrl: typed.BaseUrl,
			Token:   typed.Token,
		})
		return out, err
	}
	if typed, err := in.AsDashScopeCredentialBody(); err == nil {
		err = out.FromDashScopeCredentialBody(apitypes.DashScopeCredentialBody{
			ApiKey:  typed.ApiKey,
			BaseUrl: typed.BaseUrl,
			Token:   typed.Token,
		})
		return out, err
	}
	if typed, err := in.AsMiniMaxCredentialBody(); err == nil {
		err = out.FromMiniMaxCredentialBody(apitypes.MiniMaxCredentialBody{
			ApiKey:              typed.ApiKey,
			BaseUrl:             typed.BaseUrl,
			MinimaxVoiceBaseUrl: typed.MinimaxVoiceBaseUrl,
			Token:               typed.Token,
			VoiceBaseUrl:        typed.VoiceBaseUrl,
		})
		return out, err
	}
	if typed, err := in.AsVolcCredentialBody(); err == nil {
		err = out.FromVolcCredentialBody(apitypes.VolcCredentialBody{
			ApiKey:             typed.ApiKey,
			AppId:              typed.AppId,
			OpenapiAccessKey:   typed.OpenapiAccessKey,
			OpenapiAccessKeyId: typed.OpenapiAccessKeyId,
			SearchApiKey:       typed.SearchApiKey,
		})
		return out, err
	}
	return out, fmt.Errorf("credential body is empty or unsupported")
}

func apiCredentialListToRPC(in adminhttp.CredentialList) (rpcapi.CredentialListResponse, error) {
	items := make([]rpcapi.Credential, 0, len(in.Items))
	for _, item := range in.Items {
		converted, err := apiCredentialToRPC(item)
		if err != nil {
			return rpcapi.CredentialListResponse{}, err
		}
		items = append(items, converted)
	}
	return rpcapi.CredentialListResponse{
		HasNext:    in.HasNext,
		Items:      items,
		NextCursor: in.NextCursor,
	}, nil
}

func apiCredentialToRPC(in apitypes.Credential) (rpcapi.Credential, error) {
	body, err := apiCredentialBodyToRPC(in.Body)
	if err != nil {
		return rpcapi.Credential{}, err
	}
	return rpcapi.Credential{
		Body:        body,
		CreatedAt:   in.CreatedAt,
		Description: in.Description,
		Name:        in.Name,
		Provider:    in.Provider,
		UpdatedAt:   in.UpdatedAt,
	}, nil
}

func apiCredentialBodyToRPC(in apitypes.CredentialBody) (rpcapi.CredentialBody, error) {
	var out rpcapi.CredentialBody
	if typed, err := in.AsOpenAICredentialBody(); err == nil {
		err = out.FromOpenAICredentialBody(rpcapi.OpenAICredentialBody{
			ApiKey:       typed.ApiKey,
			BaseUrl:      typed.BaseUrl,
			Organization: typed.Organization,
			Project:      typed.Project,
			Token:        typed.Token,
		})
		return out, err
	}
	if typed, err := in.AsGeminiCredentialBody(); err == nil {
		err = out.FromGeminiCredentialBody(rpcapi.GeminiCredentialBody{
			ApiKey:  typed.ApiKey,
			BaseUrl: typed.BaseUrl,
			Token:   typed.Token,
		})
		return out, err
	}
	if typed, err := in.AsDashScopeCredentialBody(); err == nil {
		err = out.FromDashScopeCredentialBody(rpcapi.DashScopeCredentialBody{
			ApiKey:  typed.ApiKey,
			BaseUrl: typed.BaseUrl,
			Token:   typed.Token,
		})
		return out, err
	}
	if typed, err := in.AsMiniMaxCredentialBody(); err == nil {
		err = out.FromMiniMaxCredentialBody(rpcapi.MiniMaxCredentialBody{
			ApiKey:              typed.ApiKey,
			BaseUrl:             typed.BaseUrl,
			MinimaxVoiceBaseUrl: typed.MinimaxVoiceBaseUrl,
			Token:               typed.Token,
			VoiceBaseUrl:        typed.VoiceBaseUrl,
		})
		return out, err
	}
	if typed, err := in.AsVolcCredentialBody(); err == nil {
		err = out.FromVolcCredentialBody(rpcapi.VolcCredentialBody{
			ApiKey:             typed.ApiKey,
			AppId:              typed.AppId,
			OpenapiAccessKey:   typed.OpenapiAccessKey,
			OpenapiAccessKeyId: typed.OpenapiAccessKeyId,
			SearchApiKey:       typed.SearchApiKey,
		})
		return out, err
	}
	return out, fmt.Errorf("credential body is empty or unsupported")
}
