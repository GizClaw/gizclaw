package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

type APIKey struct {
	Name          string
	DisplayName   string
	Prefix        string
	APIKey        string
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

type APIKeyListRequest struct {
	Cursor string
	Limit  int64
}

type APIKeyListResponse struct {
	Items      []APIKey
	NextCursor string
}

type APIKeyRevokeRequest struct {
	Name string
}

type APIKeyRevokeResponse struct{}

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
		converted := apiKeyFromProto(key)
		response.Value = &converted
	}
	return response, nil
}

func (t *RPCPayload) FromAPIKeyCreateResponse(value APIKeyCreateResponse) error {
	response := rpcpb.APIKeyCreateResponse{ApiKey: value.APIKey}
	if key := value.Value; key != nil {
		response.Value = apiKeyToProto(*key)
	}
	return t.encode("APIKeyCreateResponse", &response)
}

func (t RPCPayload) AsAPIKeyListRequest() (APIKeyListRequest, error) {
	var value rpcpb.APIKeyListRequest
	if err := t.decode("APIKeyListRequest", &value); err != nil {
		return APIKeyListRequest{}, err
	}
	return APIKeyListRequest{Cursor: value.GetCursor(), Limit: value.GetLimit()}, nil
}

func (t *RPCPayload) FromAPIKeyListRequest(value APIKeyListRequest) error {
	request := rpcpb.APIKeyListRequest{}
	if value.Cursor != "" {
		request.Cursor = &value.Cursor
	}
	if value.Limit != 0 {
		request.Limit = &value.Limit
	}
	return t.encode("APIKeyListRequest", &request)
}

func (t RPCPayload) AsAPIKeyListResponse() (APIKeyListResponse, error) {
	var value rpcpb.APIKeyListResponse
	if err := t.decode("APIKeyListResponse", &value); err != nil {
		return APIKeyListResponse{}, err
	}
	response := APIKeyListResponse{NextCursor: value.GetNextCursor(), Items: make([]APIKey, 0, len(value.GetItems()))}
	for _, item := range value.GetItems() {
		if item != nil {
			response.Items = append(response.Items, apiKeyFromProto(item))
		}
	}
	return response, nil
}

func (t *RPCPayload) FromAPIKeyListResponse(value APIKeyListResponse) error {
	response := rpcpb.APIKeyListResponse{Items: make([]*rpcpb.APIKey, 0, len(value.Items))}
	for _, item := range value.Items {
		response.Items = append(response.Items, apiKeyToProto(item))
	}
	if value.NextCursor != "" {
		response.NextCursor = &value.NextCursor
	}
	return t.encode("APIKeyListResponse", &response)
}

func (t RPCPayload) AsAPIKeyRevokeRequest() (APIKeyRevokeRequest, error) {
	var value rpcpb.APIKeyRevokeRequest
	if err := t.decode("APIKeyRevokeRequest", &value); err != nil {
		return APIKeyRevokeRequest{}, err
	}
	return APIKeyRevokeRequest{Name: value.GetName()}, nil
}

func (t *RPCPayload) FromAPIKeyRevokeRequest(value APIKeyRevokeRequest) error {
	return t.encode("APIKeyRevokeRequest", &rpcpb.APIKeyRevokeRequest{Name: value.Name})
}

func (t RPCPayload) AsAPIKeyRevokeResponse() (APIKeyRevokeResponse, error) {
	var value rpcpb.APIKeyRevokeResponse
	if err := t.decode("APIKeyRevokeResponse", &value); err != nil {
		return APIKeyRevokeResponse{}, err
	}
	return APIKeyRevokeResponse{}, nil
}

func (t *RPCPayload) FromAPIKeyRevokeResponse(APIKeyRevokeResponse) error {
	return t.encode("APIKeyRevokeResponse", &rpcpb.APIKeyRevokeResponse{})
}

func apiKeyFromProto(key *rpcpb.APIKey) APIKey {
	return APIKey{
		Name:          key.GetName(),
		DisplayName:   key.GetDisplayName(),
		Prefix:        key.GetPrefix(),
		APIKey:        key.GetApiKey(),
		ManageAPIKeys: key.GetManageApiKeys(),
		CreatedAt:     key.GetCreatedAt(),
	}
}

func apiKeyToProto(key APIKey) *rpcpb.APIKey {
	return &rpcpb.APIKey{
		Name:          key.Name,
		DisplayName:   key.DisplayName,
		Prefix:        key.Prefix,
		ApiKey:        key.APIKey,
		ManageApiKeys: key.ManageAPIKeys,
		CreatedAt:     key.CreatedAt,
	}
}
