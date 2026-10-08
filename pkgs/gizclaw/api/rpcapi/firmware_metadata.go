package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// RPCMethodServerFirmwareMetadataGet reads one metadata key from the bound Firmware.
const RPCMethodServerFirmwareMetadataGet RPCMethod = "server.firmware.metadata.get"

// AsFirmwareMetadataGetRequest decodes one bound-firmware metadata key lookup.
func (t RPCPayload) AsFirmwareMetadataGetRequest() (*rpcpb.FirmwareMetadataGetRequest, error) {
	body := new(rpcpb.FirmwareMetadataGetRequest)
	err := t.decode("FirmwareMetadataGetRequest", body)
	return body, err
}

// FromFirmwareMetadataGetRequest encodes one bound-firmware metadata key lookup.
func (t *RPCPayload) FromFirmwareMetadataGetRequest(v *rpcpb.FirmwareMetadataGetRequest) error {
	return t.encode("FirmwareMetadataGetRequest", v)
}

// AsFirmwareMetadataGetResponse decodes one metadata value as JSON text.
func (t RPCPayload) AsFirmwareMetadataGetResponse() (*rpcpb.FirmwareMetadataGetResponse, error) {
	body := new(rpcpb.FirmwareMetadataGetResponse)
	err := t.decode("FirmwareMetadataGetResponse", body)
	return body, err
}

// FromFirmwareMetadataGetResponse encodes one metadata value as JSON text.
func (t *RPCPayload) FromFirmwareMetadataGetResponse(v *rpcpb.FirmwareMetadataGetResponse) error {
	return t.encode("FirmwareMetadataGetResponse", v)
}
