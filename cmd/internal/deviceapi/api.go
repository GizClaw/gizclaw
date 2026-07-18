package deviceapi

import (
	"context"
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

var getServerInfoRPC = func(ctx context.Context, c *gizcli.Client, id string) (*rpcapi.ServerGetInfoResponse, error) {
	return c.GetServerInfo(ctx, id)
}

var putServerInfoRPC = func(ctx context.Context, c *gizcli.Client, id string, request rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
	return c.PutServerInfo(ctx, id, request)
}

var getServerRuntimeRPC = func(ctx context.Context, c *gizcli.Client, id string) (*rpcapi.ServerGetRuntimeResponse, error) {
	return c.GetServerRuntime(ctx, id)
}

func GetInfo(ctx context.Context, c *gizcli.Client) (apitypes.DeviceInfo, error) {
	resp, err := getServerInfoRPC(ctx, c, "server.info.get")
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertClientAPIType[apitypes.DeviceInfo](*resp)
}

func PutInfo(ctx context.Context, c *gizcli.Client, info apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	rpcReq, err := convertClientAPIType[rpcapi.ServerPutInfoRequest](info)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	resp, err := putServerInfoRPC(ctx, c, "server.info.put", rpcReq)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return convertClientAPIType[apitypes.DeviceInfo](*resp)
}

func SetName(ctx context.Context, c *gizcli.Client, name string) (apitypes.DeviceInfo, error) {
	info, err := GetInfo(ctx, c)
	if err != nil {
		return apitypes.DeviceInfo{}, err
	}
	return PutInfo(ctx, c, apitypes.DeviceInfo{
		Name:  &name,
		Emoji: info.Emoji,
	})
}

func GetRuntime(ctx context.Context, c *gizcli.Client) (apitypes.Runtime, error) {
	resp, err := getServerRuntimeRPC(ctx, c, "server.runtime.get")
	if err != nil {
		return apitypes.Runtime{}, err
	}
	return convertClientAPIType[apitypes.Runtime](*resp)
}

func convertClientAPIType[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
