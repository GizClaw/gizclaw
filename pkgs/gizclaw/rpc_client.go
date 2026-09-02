package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type rpcClient struct{}

func (c *rpcClient) GetClientInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientGetInfoRequest{}, (*rpcapi.RPCPayload).FromClientGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientInfoGet, params), rpcapi.RPCPayload.AsClientGetInfoResponse)
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
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientIdentifiersGet, params), rpcapi.RPCPayload.AsClientGetIdentifiersResponse)
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
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientDeviceStatusGet, params), rpcapi.RPCPayload.AsClientDeviceStatusGetResponse)
	if err != nil {
		return nil, wrapRPCResultError("device status", err)
	}
	return result, nil
}

func (c *rpcClient) SetDeviceVolume(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceVolumeSetRequest) (*rpcapi.ClientDeviceVolumeSetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceVolumeSetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientDeviceVolumeSet, params), rpcapi.RPCPayload.AsClientDeviceVolumeSetResponse)
	if err != nil {
		return nil, wrapRPCResultError("device volume", err)
	}
	return result, nil
}

func (c *rpcClient) PlayDeviceSound(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceSoundPlayRequest) (*rpcapi.ClientDeviceSoundPlayResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceSoundPlayRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientDeviceSoundPlay, params), rpcapi.RPCPayload.AsClientDeviceSoundPlayResponse)
	if err != nil {
		return nil, wrapRPCResultError("device sound", err)
	}
	return result, nil
}

func (c *rpcClient) RebootDevice(ctx context.Context, conn net.Conn, id string, request rpcapi.ClientDeviceRebootRequest) (*rpcapi.ClientDeviceRebootResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceRebootRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientDeviceReboot, params), rpcapi.RPCPayload.AsClientDeviceRebootResponse)
	if err != nil {
		return nil, wrapRPCResultError("device reboot", err)
	}
	return result, nil
}

func (c *rpcClient) GetWifiStatus(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientWifiStatusGetResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientWifiStatusGetRequest{}, (*rpcapi.RPCPayload).FromClientWifiStatusGetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientWifiStatusGet, params), rpcapi.RPCPayload.AsClientWifiStatusGetResponse)
	if err != nil {
		return nil, wrapRPCResultError("wifi status", err)
	}
	return result, nil
}

func (c *rpcClient) ListSavedWifi(ctx context.Context, conn net.Conn, id string) (*rpcapi.ClientWifiSavedListResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ClientWifiSavedListRequest{}, (*rpcapi.RPCPayload).FromClientWifiSavedListRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientWifiSavedList, params), rpcapi.RPCPayload.AsClientWifiSavedListResponse)
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
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodClientWifiSavedForget, params), rpcapi.RPCPayload.AsClientWifiSavedForgetResponse)
	if err != nil {
		return nil, wrapRPCResultError("wifi saved forget", err)
	}
	return result, nil
}
