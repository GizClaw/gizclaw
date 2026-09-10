package rpcapi

// SocialPingResult is the outcome of one friend ping or Friend Group rally.
type SocialPingResult string

const (
	SocialPingResultDelivered   SocialPingResult = "delivered"
	SocialPingResultNotOnline   SocialPingResult = "not_online"
	SocialPingResultRateLimited SocialPingResult = "rate_limited"
)

// Valid indicates whether the value is a known member of the SocialPingResult enum.
func (e SocialPingResult) Valid() bool {
	switch e {
	case SocialPingResultDelivered, SocialPingResultNotOnline, SocialPingResultRateLimited:
		return true
	default:
		return false
	}
}

// FriendPingRequest pings the caller's Friend named Name.
type FriendPingRequest struct {
	Name string `json:"name"`
}

// FriendPingResponse reports whether the Friend's device took the ping.
type FriendPingResponse struct {
	Result            SocialPingResult `json:"result"`
	DeliveredCount    int32            `json:"delivered_count"`
	RetryAfterSeconds *int32           `json:"retry_after_seconds,omitempty"`
}

// FriendGroupPingRequest rallies the caller's Friend Group named Name.
type FriendGroupPingRequest struct {
	Name string `json:"name"`
}

// FriendGroupPingResponse reports how many member devices took the rally.
type FriendGroupPingResponse struct {
	Result            SocialPingResult `json:"result"`
	DeliveredCount    int32            `json:"delivered_count"`
	RetryAfterSeconds *int32           `json:"retry_after_seconds,omitempty"`
}

// ClientSocialPingRequest tells a device that a Friend pinged it or a Friend
// Group member rallied the group. FriendGroupName is the receiving device's
// own name for the group and is nil for a Friend ping.
type ClientSocialPingRequest struct {
	FromPeerPublicKey string  `json:"from_peer_public_key"`
	FromDisplayName   *string `json:"from_display_name,omitempty"`
	FriendGroupName   *string `json:"friend_group_name,omitempty"`
}

// ClientSocialPingResponse acknowledges a social ping.
type ClientSocialPingResponse struct{}

// AsFriendPingRequest decodes the RPCPayload as a FriendPingRequest.
func (t RPCPayload) AsFriendPingRequest() (FriendPingRequest, error) {
	var body FriendPingRequest
	err := t.decode("FriendPingRequest", &body)
	return body, err
}

// FromFriendPingRequest encodes the FriendPingRequest into the RPCPayload.
func (t *RPCPayload) FromFriendPingRequest(v FriendPingRequest) error {
	return t.encode("FriendPingRequest", v)
}

// AsFriendPingResponse decodes the RPCPayload as a FriendPingResponse.
func (t RPCPayload) AsFriendPingResponse() (FriendPingResponse, error) {
	var body FriendPingResponse
	err := t.decode("FriendPingResponse", &body)
	return body, err
}

// FromFriendPingResponse encodes the FriendPingResponse into the RPCPayload.
func (t *RPCPayload) FromFriendPingResponse(v FriendPingResponse) error {
	return t.encode("FriendPingResponse", v)
}

// AsFriendGroupPingRequest decodes the RPCPayload as a FriendGroupPingRequest.
func (t RPCPayload) AsFriendGroupPingRequest() (FriendGroupPingRequest, error) {
	var body FriendGroupPingRequest
	err := t.decode("FriendGroupPingRequest", &body)
	return body, err
}

// FromFriendGroupPingRequest encodes the FriendGroupPingRequest into the RPCPayload.
func (t *RPCPayload) FromFriendGroupPingRequest(v FriendGroupPingRequest) error {
	return t.encode("FriendGroupPingRequest", v)
}

// AsFriendGroupPingResponse decodes the RPCPayload as a FriendGroupPingResponse.
func (t RPCPayload) AsFriendGroupPingResponse() (FriendGroupPingResponse, error) {
	var body FriendGroupPingResponse
	err := t.decode("FriendGroupPingResponse", &body)
	return body, err
}

// FromFriendGroupPingResponse encodes the FriendGroupPingResponse into the RPCPayload.
func (t *RPCPayload) FromFriendGroupPingResponse(v FriendGroupPingResponse) error {
	return t.encode("FriendGroupPingResponse", v)
}

// AsClientSocialPingRequest decodes the RPCPayload as a ClientSocialPingRequest.
func (t RPCPayload) AsClientSocialPingRequest() (ClientSocialPingRequest, error) {
	var body ClientSocialPingRequest
	err := t.decode("ClientSocialPingRequest", &body)
	return body, err
}

// FromClientSocialPingRequest encodes the ClientSocialPingRequest into the RPCPayload.
func (t *RPCPayload) FromClientSocialPingRequest(v ClientSocialPingRequest) error {
	return t.encode("ClientSocialPingRequest", v)
}

// AsClientSocialPingResponse decodes the RPCPayload as a ClientSocialPingResponse.
func (t RPCPayload) AsClientSocialPingResponse() (ClientSocialPingResponse, error) {
	var body ClientSocialPingResponse
	err := t.decode("ClientSocialPingResponse", &body)
	return body, err
}

// FromClientSocialPingResponse encodes the ClientSocialPingResponse into the RPCPayload.
func (t *RPCPayload) FromClientSocialPingResponse(v ClientSocialPingResponse) error {
	return t.encode("ClientSocialPingResponse", v)
}
