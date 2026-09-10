package rpcapi

// MaxProfileGetKeys bounds one server.profile.get batch. It matches the
// nanopb max_count in api/proto/rpc/nanopb.options.
const MaxProfileGetKeys = 16

// ProfileGetRequest looks up the public profile of up to MaxProfileGetKeys
// Peers by public key.
type ProfileGetRequest struct {
	PeerPublicKeys []string `json:"peer_public_keys"`
}

// ProfileGetResponse holds one item per distinct requested key, in request
// order.
type ProfileGetResponse struct {
	Items []PublicProfile `json:"items"`
}

// PublicProfile is the public projection of a Peer's DeviceInfo: its
// self-chosen display name and emoji, and nothing else.
type PublicProfile struct {
	PeerPublicKey string  `json:"peer_public_key"`
	DisplayName   *string `json:"display_name,omitempty"`
	Emoji         *string `json:"emoji,omitempty"`
}

// AsProfileGetRequest decodes the RPCPayload as a ProfileGetRequest.
func (t RPCPayload) AsProfileGetRequest() (ProfileGetRequest, error) {
	var body ProfileGetRequest
	err := t.decode("ProfileGetRequest", &body)
	return body, err
}

// FromProfileGetRequest encodes the ProfileGetRequest into the RPCPayload.
func (t *RPCPayload) FromProfileGetRequest(v ProfileGetRequest) error {
	return t.encode("ProfileGetRequest", v)
}

// AsProfileGetResponse decodes the RPCPayload as a ProfileGetResponse.
func (t RPCPayload) AsProfileGetResponse() (ProfileGetResponse, error) {
	var body ProfileGetResponse
	err := t.decode("ProfileGetResponse", &body)
	return body, err
}

// FromProfileGetResponse encodes the ProfileGetResponse into the RPCPayload.
func (t *RPCPayload) FromProfileGetResponse(v ProfileGetResponse) error {
	return t.encode("ProfileGetResponse", v)
}
