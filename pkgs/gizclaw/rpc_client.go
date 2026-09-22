package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

type rpcClient struct{}

func (c *rpcClient) GetClientInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientGetInfoRequest{}, (*rpcapi.RPCPayload).FromClientGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_INFO_GET, params, rpcapi.RPCPayload.AsClientGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("device info", err)
	}
	return result, nil
}

func (c *rpcClient) GetClientIdentifiers(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientGetIdentifiersResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientGetIdentifiersRequest{}, (*rpcapi.RPCPayload).FromClientGetIdentifiersRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_IDENTIFIERS_GET, params, rpcapi.RPCPayload.AsClientGetIdentifiersResponse)
	if err != nil {
		return nil, wrapRPCResultError("device identifiers", err)
	}
	return result, nil
}

func (c *rpcClient) GetDeviceStatus(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientDeviceStatusGetResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientDeviceStatusGetRequest{}, (*rpcapi.RPCPayload).FromClientDeviceStatusGetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_STATUS_GET, params, rpcapi.RPCPayload.AsClientDeviceStatusGetResponse)
	if err != nil {
		return nil, wrapRPCResultError("device status", err)
	}
	return result, nil
}

func (c *rpcClient) PlayDeviceSound(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceSoundPlayRequest) (*rpcapi.ClientDeviceSoundPlayResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceSoundPlayRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, params, rpcapi.RPCPayload.AsClientDeviceSoundPlayResponse)
	if err != nil {
		return nil, wrapRPCResultError("device sound", err)
	}
	return result, nil
}

func (c *rpcClient) FindDevice(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceFindRequest) (*rpcapi.ClientDeviceFindResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceFindRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND, params, rpcapi.RPCPayload.AsClientDeviceFindResponse)
	if err != nil {
		return nil, wrapRPCResultError("device find", err)
	}
	return result, nil
}

func (c *rpcClient) PingSocial(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientSocialPingRequest) (*rpcapi.ClientSocialPingResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientSocialPingRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING, params, rpcapi.RPCPayload.AsClientSocialPingResponse)
	if err != nil {
		return nil, wrapRPCResultError("social ping", err)
	}
	return result, nil
}

func (c *rpcClient) RebootDevice(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceRebootRequest) (*rpcapi.ClientDeviceRebootResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceRebootRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, params, rpcapi.RPCPayload.AsClientDeviceRebootResponse)
	if err != nil {
		return nil, wrapRPCResultError("device reboot", err)
	}
	return result, nil
}

func (c *rpcClient) UpdateDeviceFirmware(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientFirmwareUpdateRequest) (*rpcapi.ClientFirmwareUpdateResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientFirmwareUpdateRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, params, rpcapi.RPCPayload.AsClientFirmwareUpdateResponse)
	if err != nil {
		return nil, wrapRPCResultError("device firmware update", err)
	}
	return result, nil
}

func (c *rpcClient) ListSavedWifi(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientWifiSavedListResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientWifiSavedListRequest{}, (*rpcapi.RPCPayload).FromClientWifiSavedListRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST, params, rpcapi.RPCPayload.AsClientWifiSavedListResponse)
	if err != nil {
		return nil, wrapRPCResultError("wifi saved list", err)
	}
	return result, nil
}

func (c *rpcClient) ForgetSavedWifi(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientWifiSavedForgetRequest) (*rpcapi.ClientWifiSavedForgetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientWifiSavedForgetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, params, rpcapi.RPCPayload.AsClientWifiSavedForgetResponse)
	if err != nil {
		return nil, wrapRPCResultError("wifi saved forget", err)
	}
	return result, nil
}

func (c *rpcClient) ScanWifi(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientWifiScanRequest) (*rpcapi.ClientWifiScanResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientWifiScanRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SCAN, params, rpcapi.RPCPayload.AsClientWifiScanResponse)
	if err != nil {
		return nil, wrapRPCResultError("wifi scan", err)
	}
	return result, nil
}

func (c *rpcClient) ConnectWifi(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientWifiConnectRequest) (*rpcapi.ClientWifiConnectResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientWifiConnectRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_WIFI_CONNECT, params, rpcapi.RPCPayload.AsClientWifiConnectResponse)
	if err != nil {
		return nil, wrapRPCResultError("wifi connect", err)
	}
	return result, nil
}

func (c *rpcClient) FactoryResetDevice(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceFactoryResetRequest) (*rpcapi.ClientDeviceFactoryResetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceFactoryResetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FACTORY_RESET, params, rpcapi.RPCPayload.AsClientDeviceFactoryResetResponse)
	if err != nil {
		return nil, wrapRPCResultError("device factory reset", err)
	}
	return result, nil
}

func (c *rpcClient) SetRunWorkspace(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientRunWorkspaceSetRequest) (*rpcapi.ClientRunWorkspaceSetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientRunWorkspaceSetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callClientToolResult(ctx, conn, id, rpcpb.ClientTool_CLIENT_TOOL_RUN_WORKSPACE_SET, params, rpcapi.RPCPayload.AsClientRunWorkspaceSetResponse)
	if err != nil {
		return nil, wrapRPCResultError("run workspace set", err)
	}
	return result, nil
}

func (c *rpcClient) ReadMhsStates(ctx context.Context, conn net.Conn, id string, request *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientMhsV0ReadRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientMhsV0Read, params), rpcapi.RPCPayload.AsClientMhsV0ReadResponse)
	if err != nil {
		return nil, wrapRPCResultError("MHS states", err)
	}
	return *result, nil
}

func (c *rpcClient) WriteMhsStates(ctx context.Context, conn net.Conn, id string, request *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientMhsV0WriteRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientMhsV0Write, params), rpcapi.RPCPayload.AsClientMhsV0WriteResponse)
	if err != nil {
		return nil, wrapRPCResultError("MHS states", err)
	}
	return *result, nil
}
