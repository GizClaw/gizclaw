package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

type APIKey struct {
	Name          string
	DisplayName   string
	Prefix        string
	ManageAPIKeys bool
	CreatedAt     string
}

type APIKeyCreateRequest struct {
	DisplayName   string
	ManageAPIKeys bool
}

type APIKeyCreateResponse struct {
	Value  *APIKey
	APIKey string
}

func (t RPCPayload) AsAPIKeyCreateRequest() (APIKeyCreateRequest, error) {
	var value rpcpb.APIKeyCreateRequest
	if err := t.decode("APIKeyCreateRequest", &value); err != nil {
		return APIKeyCreateRequest{}, err
	}
	return APIKeyCreateRequest{
		DisplayName:   value.GetDisplayName(),
		ManageAPIKeys: value.GetManageApiKeys(),
	}, nil
}

func (t *RPCPayload) FromAPIKeyCreateRequest(value APIKeyCreateRequest) error {
	return t.encode("APIKeyCreateRequest", &rpcpb.APIKeyCreateRequest{
		DisplayName:   value.DisplayName,
		ManageApiKeys: value.ManageAPIKeys,
	})
}

func (t RPCPayload) AsAPIKeyCreateResponse() (APIKeyCreateResponse, error) {
	var value rpcpb.APIKeyCreateResponse
	if err := t.decode("APIKeyCreateResponse", &value); err != nil {
		return APIKeyCreateResponse{}, err
	}
	response := APIKeyCreateResponse{APIKey: value.GetApiKey()}
	if key := value.GetValue(); key != nil {
		response.Value = &APIKey{
			Name:          key.GetName(),
			DisplayName:   key.GetDisplayName(),
			Prefix:        key.GetPrefix(),
			ManageAPIKeys: key.GetManageApiKeys(),
			CreatedAt:     key.GetCreatedAt(),
		}
	}
	return response, nil
}

func (t *RPCPayload) FromAPIKeyCreateResponse(value APIKeyCreateResponse) error {
	response := rpcpb.APIKeyCreateResponse{ApiKey: value.APIKey}
	if key := value.Value; key != nil {
		response.Value = &rpcpb.APIKey{
			Name:          key.Name,
			DisplayName:   key.DisplayName,
			Prefix:        key.Prefix,
			ManageApiKeys: key.ManageAPIKeys,
			CreatedAt:     key.CreatedAt,
		}
	}
	return t.encode("APIKeyCreateResponse", &response)
}
