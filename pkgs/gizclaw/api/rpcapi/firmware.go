package rpcapi

// ClientFirmwareUpdateRequest asks the device to run one firmware update.
// Channel names the channel to install and defaults to the channel the device
// already uses. Sha256 declares the package the caller resolved, so the device
// can refuse when its own configuration resolves a different package.
type ClientFirmwareUpdateRequest struct {
	Channel *FirmwareChannelName `json:"channel,omitempty"`
	Sha256  *string              `json:"sha256,omitempty"`
}

// ClientFirmwareUpdateResponse acknowledges a firmware update request.
type ClientFirmwareUpdateResponse struct{}

// AsClientFirmwareUpdateRequest decodes the RPCPayload as a ClientFirmwareUpdateRequest.
func (t RPCPayload) AsClientFirmwareUpdateRequest() (ClientFirmwareUpdateRequest, error) {
	var body ClientFirmwareUpdateRequest
	err := t.decode("ClientFirmwareUpdateRequest", &body)
	return body, err
}

// FromClientFirmwareUpdateRequest encodes the ClientFirmwareUpdateRequest into the RPCPayload.
func (t *RPCPayload) FromClientFirmwareUpdateRequest(v ClientFirmwareUpdateRequest) error {
	return t.encode("ClientFirmwareUpdateRequest", v)
}

// AsClientFirmwareUpdateResponse decodes the RPCPayload as a ClientFirmwareUpdateResponse.
func (t RPCPayload) AsClientFirmwareUpdateResponse() (ClientFirmwareUpdateResponse, error) {
	var body ClientFirmwareUpdateResponse
	err := t.decode("ClientFirmwareUpdateResponse", &body)
	return body, err
}

// FromClientFirmwareUpdateResponse encodes the ClientFirmwareUpdateResponse into the RPCPayload.
func (t *RPCPayload) FromClientFirmwareUpdateResponse(v ClientFirmwareUpdateResponse) error {
	return t.encode("ClientFirmwareUpdateResponse", v)
}
