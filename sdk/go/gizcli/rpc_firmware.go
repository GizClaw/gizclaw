package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func (c *rpcClient) GetFirmware(ctx context.Context, conn net.Conn, id string, request rpcapi.FirmwareGetRequest) (*rpcapi.FirmwareGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFirmwareGet, request, (*rpcapi.RPCPayload).FromFirmwareGetRequest, rpcapi.RPCPayload.AsFirmwareGetResponse, "firmware get")
}
