package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcClient struct {
	peer *Client
}

func (c *rpcClient) Handle(conn net.Conn) error {
	return handleRPCWithStream(conn, c.dispatch, c.dispatchStream)
}

func (c *rpcClient) dispatchStream(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) (bool, error) {
	if req == nil || req.Method != rpcapi.RPCMethodAllSpeedTestRun {
		return false, nil
	}
	return true, handleRPCSpeedTest(ctx, stream, req)
}

func (c *rpcClient) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.StatusCodeInvalidArgument, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {

	case rpcapi.RPCMethodClientMhsV0Read, rpcapi.RPCMethodClientMhsV0Write:
		if c.peer == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		return c.handleDeviceControl(ctx, req)
	case rpcapi.RPCMethodClientToolV0Invoke, rpcapi.RPCMethodClientToolV0List, rpcapi.RPCMethodClientRPCMethodsList:
		if c.peer == nil {
			return deviceControlUnsupported(req.Id, req.Method), nil
		}
		return c.handleToolV0(ctx, req)
	case rpcapi.RPCMethodAllPing:
		return handleRPCPing(ctx, req)
	default:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unsupported method: " + string(req.Method)}.RPCResponse(), nil
	}
}
