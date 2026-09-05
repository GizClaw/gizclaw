package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// Audio player methods are provided by the device's single player.
const (
	RPCMethodClientDeviceAudioPlayerGet            RPCMethod = "client.device.audioplayer.get"
	RPCMethodClientDeviceAudioPlayerPlaylistGet    RPCMethod = "client.device.audioplayer.playlist.get"
	RPCMethodClientDeviceAudioPlayerPlaylistSet    RPCMethod = "client.device.audioplayer.playlist.set"
	RPCMethodClientDeviceAudioPlayerPlaylistAppend RPCMethod = "client.device.audioplayer.playlist.append"
	RPCMethodClientDeviceAudioPlayerPlay           RPCMethod = "client.device.audioplayer.play"
	RPCMethodClientDeviceAudioPlayerStop           RPCMethod = "client.device.audioplayer.stop"
	RPCMethodClientDeviceAudioPlayerModeSet        RPCMethod = "client.device.audioplayer.mode.set"
)

// AsClientDeviceAudioPlayerGetRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerGetRequest() (*rpcpb.ClientDeviceAudioPlayerGetRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerGetRequest)
	err := p.decode("ClientDeviceAudioPlayerGetRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerGetRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerGetRequest(value *rpcpb.ClientDeviceAudioPlayerGetRequest) error {
	return p.encode("ClientDeviceAudioPlayerGetRequest", value)
}

// AsClientDeviceAudioPlayerGetResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerGetResponse() (*rpcpb.ClientDeviceAudioPlayerGetResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerGetResponse)
	err := p.decode("ClientDeviceAudioPlayerGetResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerGetResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerGetResponse(value *rpcpb.ClientDeviceAudioPlayerGetResponse) error {
	return p.encode("ClientDeviceAudioPlayerGetResponse", value)
}

// AsClientDeviceAudioPlayerPlaylistGetRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlaylistGetRequest() (*rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest)
	err := p.decode("ClientDeviceAudioPlayerPlaylistGetRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlaylistGetRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlaylistGetRequest(value *rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest) error {
	return p.encode("ClientDeviceAudioPlayerPlaylistGetRequest", value)
}

// AsClientDeviceAudioPlayerPlaylistGetResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlaylistGetResponse() (*rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse)
	err := p.decode("ClientDeviceAudioPlayerPlaylistGetResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlaylistGetResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlaylistGetResponse(value *rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse) error {
	return p.encode("ClientDeviceAudioPlayerPlaylistGetResponse", value)
}

// AsClientDeviceAudioPlayerPlaylistSetRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlaylistSetRequest() (*rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest)
	err := p.decode("ClientDeviceAudioPlayerPlaylistSetRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlaylistSetRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlaylistSetRequest(value *rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest) error {
	return p.encode("ClientDeviceAudioPlayerPlaylistSetRequest", value)
}

// AsClientDeviceAudioPlayerPlaylistSetResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlaylistSetResponse() (*rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse)
	err := p.decode("ClientDeviceAudioPlayerPlaylistSetResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlaylistSetResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlaylistSetResponse(value *rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse) error {
	return p.encode("ClientDeviceAudioPlayerPlaylistSetResponse", value)
}

// AsClientDeviceAudioPlayerPlaylistAppendRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlaylistAppendRequest() (*rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest)
	err := p.decode("ClientDeviceAudioPlayerPlaylistAppendRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlaylistAppendRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlaylistAppendRequest(value *rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest) error {
	return p.encode("ClientDeviceAudioPlayerPlaylistAppendRequest", value)
}

// AsClientDeviceAudioPlayerPlaylistAppendResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlaylistAppendResponse() (*rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse)
	err := p.decode("ClientDeviceAudioPlayerPlaylistAppendResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlaylistAppendResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlaylistAppendResponse(value *rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse) error {
	return p.encode("ClientDeviceAudioPlayerPlaylistAppendResponse", value)
}

// AsClientDeviceAudioPlayerPlayRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlayRequest() (*rpcpb.ClientDeviceAudioPlayerPlayRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlayRequest)
	err := p.decode("ClientDeviceAudioPlayerPlayRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlayRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlayRequest(value *rpcpb.ClientDeviceAudioPlayerPlayRequest) error {
	return p.encode("ClientDeviceAudioPlayerPlayRequest", value)
}

// AsClientDeviceAudioPlayerPlayResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerPlayResponse() (*rpcpb.ClientDeviceAudioPlayerPlayResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerPlayResponse)
	err := p.decode("ClientDeviceAudioPlayerPlayResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerPlayResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerPlayResponse(value *rpcpb.ClientDeviceAudioPlayerPlayResponse) error {
	return p.encode("ClientDeviceAudioPlayerPlayResponse", value)
}

// AsClientDeviceAudioPlayerStopRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerStopRequest() (*rpcpb.ClientDeviceAudioPlayerStopRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerStopRequest)
	err := p.decode("ClientDeviceAudioPlayerStopRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerStopRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerStopRequest(value *rpcpb.ClientDeviceAudioPlayerStopRequest) error {
	return p.encode("ClientDeviceAudioPlayerStopRequest", value)
}

// AsClientDeviceAudioPlayerStopResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerStopResponse() (*rpcpb.ClientDeviceAudioPlayerStopResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerStopResponse)
	err := p.decode("ClientDeviceAudioPlayerStopResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerStopResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerStopResponse(value *rpcpb.ClientDeviceAudioPlayerStopResponse) error {
	return p.encode("ClientDeviceAudioPlayerStopResponse", value)
}

// AsClientDeviceAudioPlayerModeSetRequest decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerModeSetRequest() (*rpcpb.ClientDeviceAudioPlayerModeSetRequest, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerModeSetRequest)
	err := p.decode("ClientDeviceAudioPlayerModeSetRequest", value)
	return value, err
}

// FromClientDeviceAudioPlayerModeSetRequest encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerModeSetRequest(value *rpcpb.ClientDeviceAudioPlayerModeSetRequest) error {
	return p.encode("ClientDeviceAudioPlayerModeSetRequest", value)
}

// AsClientDeviceAudioPlayerModeSetResponse decodes the device audio player payload.
func (p RPCPayload) AsClientDeviceAudioPlayerModeSetResponse() (*rpcpb.ClientDeviceAudioPlayerModeSetResponse, error) {
	value := new(rpcpb.ClientDeviceAudioPlayerModeSetResponse)
	err := p.decode("ClientDeviceAudioPlayerModeSetResponse", value)
	return value, err
}

// FromClientDeviceAudioPlayerModeSetResponse encodes the device audio player payload.
func (p *RPCPayload) FromClientDeviceAudioPlayerModeSetResponse(value *rpcpb.ClientDeviceAudioPlayerModeSetResponse) error {
	return p.encode("ClientDeviceAudioPlayerModeSetResponse", value)
}
