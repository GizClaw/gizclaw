package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// AsSocialPingDeliveryRequest decodes an internal notification request.
func (t RPCPayload) AsSocialPingDeliveryRequest() (*rpcpb.SocialPingDeliveryRequest, error) {
	body := new(rpcpb.SocialPingDeliveryRequest)
	err := t.decode("SocialPingDeliveryRequest", body)
	return body, err
}

// FromSocialPingDeliveryRequest encodes an internal notification request.
func (t *RPCPayload) FromSocialPingDeliveryRequest(v *rpcpb.SocialPingDeliveryRequest) error {
	return t.encode("SocialPingDeliveryRequest", v)
}

// AsSocialPingDeliveryResponse decodes a device acknowledgement relayed by Edge.
func (t RPCPayload) AsSocialPingDeliveryResponse() (*rpcpb.SocialPingDeliveryResponse, error) {
	body := new(rpcpb.SocialPingDeliveryResponse)
	err := t.decode("SocialPingDeliveryResponse", body)
	return body, err
}

// FromSocialPingDeliveryResponse encodes a relayed device acknowledgement.
func (t *RPCPayload) FromSocialPingDeliveryResponse(v *rpcpb.SocialPingDeliveryResponse) error {
	return t.encode("SocialPingDeliveryResponse", v)
}
