package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

type ServerRegisterRequest struct {
	Token string
}

type ServerRegisterResponse struct {
	RuntimeProfileName string
	FirmwareID         *string
}

func (t RPCPayload) AsServerRegisterRequest() (ServerRegisterRequest, error) {
	var value rpcpb.ServerRegisterRequest
	if err := t.decode("ServerRegisterRequest", &value); err != nil {
		return ServerRegisterRequest{}, err
	}
	return ServerRegisterRequest{Token: value.GetToken()}, nil
}

func (t *RPCPayload) FromServerRegisterRequest(value ServerRegisterRequest) error {
	return t.encode("ServerRegisterRequest", &rpcpb.ServerRegisterRequest{Token: value.Token})
}

func (t RPCPayload) AsServerRegisterResponse() (ServerRegisterResponse, error) {
	var value rpcpb.ServerRegisterResponse
	if err := t.decode("ServerRegisterResponse", &value); err != nil {
		return ServerRegisterResponse{}, err
	}
	response := ServerRegisterResponse{
		RuntimeProfileName: value.GetRuntimeProfileName(),
	}
	if value.FirmwareId != nil {
		firmwareID := value.GetFirmwareId()
		response.FirmwareID = &firmwareID
	}
	return response, nil
}

func (t *RPCPayload) FromServerRegisterResponse(value ServerRegisterResponse) error {
	return t.encode("ServerRegisterResponse", &rpcpb.ServerRegisterResponse{
		RuntimeProfileName: value.RuntimeProfileName,
		FirmwareId:         value.FirmwareID,
	})
}
