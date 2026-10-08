package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func (c *rpcClient) GetFirmware(ctx context.Context, conn net.Conn, id string, request rpcapi.FirmwareGetRequest) (*rpcapi.FirmwareGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFirmwareGet, request, (*rpcapi.RPCPayload).FromFirmwareGetRequest, rpcapi.RPCPayload.AsFirmwareGetResponse, "firmware get")
}

func (c *rpcClient) GetFirmwareMetadata(ctx context.Context, conn net.Conn, id string, request *rpcpb.FirmwareMetadataGetRequest) (*rpcpb.FirmwareMetadataGetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromFirmwareMetadataGetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerFirmwareMetadataGet, params), rpcapi.RPCPayload.AsFirmwareMetadataGetResponse)
	if err != nil {
		return nil, wrapRPCResultError("firmware metadata get", err)
	}
	return *result, nil
}

// GetFirmwareMetadata reads one channel-independent key from the bound Firmware.
func (c *Client) GetFirmwareMetadata(ctx context.Context, id string, request *rpcpb.FirmwareMetadataGetRequest) (*rpcpb.FirmwareMetadataGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcpb.FirmwareMetadataGetResponse, error) {
		return client.GetFirmwareMetadata(ctx, conn, id, request)
	})
}
