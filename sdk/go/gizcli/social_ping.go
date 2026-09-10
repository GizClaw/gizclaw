package gizcli

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

// SocialPingHandler receives client.social.ping: a Friend pinged this device,
// or a Friend Group member rallied the group when FriendGroupName is set. It
// should alert the user and return promptly; the Server waits a few seconds
// for the acknowledgement and counts a timeout or error as not delivered.
type SocialPingHandler func(context.Context, rpcapi.ClientSocialPingRequest) error

// HandleSocialPing installs the client.social.ping provider. A nil handler
// answers METHOD_NOT_FOUND, which the Server counts as not delivered.
func (c *Client) HandleSocialPing(handler SocialPingHandler) error {
	if c == nil {
		return errors.New("gizclaw: nil client")
	}
	c.socialMu.Lock()
	defer c.socialMu.Unlock()
	c.socialPing = handler
	return nil
}

func (c *Client) socialPingHandler() SocialPingHandler {
	if c == nil {
		return nil
	}
	c.socialMu.RLock()
	defer c.socialMu.RUnlock()
	return c.socialPing
}

func (c *rpcClient) handleSocialPing(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsClientSocialPingRequest()
	if err != nil || params.FromPeerPublicKey == "" {
		return rpcInvalidParams(req.Id), nil
	}
	handler := c.peer.socialPingHandler()
	if handler == nil {
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
	c.peer.observeClientRPC(req.Method)
	if err := handler(ctx, params); err != nil {
		return deviceControlError(req.Id, err), nil
	}
	return newRPCResultResponse(req.Id, rpcapi.ClientSocialPingResponse{}, (*rpcapi.RPCPayload).FromClientSocialPingResponse)
}
