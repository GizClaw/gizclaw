package gizcli

import (
	"context"
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
	"slices"
	"strings"
)

func (c *rpcClient) handleToolV0(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handlers := c.peer.deviceControlHandlers()
	switch req.Method {
	case rpcapi.RPCMethodClientRPCMethodsList:
		if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientRpcMethodsListRequest); err != nil {
			return rpcInvalidParams(req.Id), nil
		}
		methods := []rpcpb.RpcMethod{rpcpb.RpcMethod_RPC_METHOD_ALL_PING, rpcpb.RpcMethod_RPC_METHOD_ALL_SPEED_TEST_RUN, rpcpb.RpcMethod_RPC_METHOD_CLIENT_TOOL_V0_INVOKE, rpcpb.RpcMethod_RPC_METHOD_CLIENT_TOOL_V0_LIST, rpcpb.RpcMethod_RPC_METHOD_CLIENT_RPC_METHODS_LIST}
		if handlers != nil {
			if handlers.ReadMhsStates != nil {
				methods = append(methods, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ)
			}
			if handlers.WriteMhsStates != nil {
				methods = append(methods, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_WRITE)
			}
		}
		slices.Sort(methods)
		c.peer.observeClientRPC(req.Method)
		return newRPCResultResponse(req.Id, &rpcpb.ClientRpcMethodsListResponse{Methods: methods}, (*rpcapi.RPCPayload).FromClientRpcMethodsListResponse)
	case rpcapi.RPCMethodClientToolV0List:
		if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientToolV0ListRequest); err != nil {
			return rpcInvalidParams(req.Id), nil
		}
		tools := []rpcpb.ClientTool{rpcpb.ClientTool_CLIENT_TOOL_INFO_GET, rpcpb.ClientTool_CLIENT_TOOL_IDENTIFIERS_GET}
		if c.peer.socialPingHandler() != nil {
			tools = append(tools, rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING)
		}
		tools = append(tools, handlers.supportedTools()...)
		c.peer.deviceMu.RLock()
		for tool := range c.peer.toolHandlers {
			tools = append(tools, tool)
		}
		c.peer.deviceMu.RUnlock()
		slices.Sort(tools)
		tools = slices.Compact(tools)
		c.peer.observeClientRPC(req.Method)
		return newRPCResultResponse(req.Id, &rpcpb.ClientToolV0ListResponse{Tools: tools}, (*rpcapi.RPCPayload).FromClientToolV0ListResponse)
	}
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	invoke, err := req.Params.AsClientToolV0InvokeRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := rpcapi.DecodeClientToolRequest(invoke)
	if err != nil {
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
	message, err := rpcapi.ClientToolRequestFromBytes(invoke.Tool, invoke.Payload)
	if err != nil || rpcapi.ValidateClientToolRequest(message) != nil {
		return rpcInvalidParams(req.Id), nil
	}
	c.peer.observeClientTool(invoke.Tool)
	if handler := c.peer.clientToolHandler(invoke.Tool); handler != nil {
		c.peer.observeClientRPC(req.Method)
		result, err := handler(ctx, message)
		if err != nil {
			return deviceControlError(req.Id, err), nil
		}
		meta, _ := rpcapi.ClientToolMetadata(invoke.Tool)
		if result == nil || string(result.ProtoReflect().Descriptor().Name()) != meta.Response {
			return deviceControlError(req.Id, fmt.Errorf("invalid tool response type")), nil
		}
		if err := validateClientToolResponse(result); err != nil {
			return deviceControlError(req.Id, err), nil
		}
		payload, err := proto.Marshal(result)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, &rpcpb.ClientToolV0InvokeResponse{Payload: payload}, (*rpcapi.RPCPayload).FromClientToolV0InvokeResponse)
	}
	inner := *req
	inner.Params = params
	response, err := c.dispatchClientTool(ctx, invoke.Tool, &inner)
	if err != nil || response == nil || response.Error != nil {
		return response, err
	}
	response.Result, err = rpcapi.EncodeClientToolResponse(invoke.Tool, response.Result)
	return response, err
}

func (c *rpcClient) dispatchClientTool(ctx context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	switch tool {
	case rpcpb.ClientTool_CLIENT_TOOL_INFO_GET:
		return c.handleGetClientInfo(ctx, req)
	case rpcpb.ClientTool_CLIENT_TOOL_IDENTIFIERS_GET:
		return c.handleGetClientIdentifiers(ctx, req)
	case rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING:
		return c.handleSocialPing(ctx, req)
	case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_GET, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_STOP, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_MODE_SET, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND:
		return c.handleAudioPlayer(ctx, tool, req)
	default:
		return c.handleDeviceTool(ctx, tool, req)
	}
}

func validateClientToolResponse(message proto.Message) error {
	if strings.HasPrefix(string(message.ProtoReflect().Descriptor().Name()), "ClientDeviceAudioPlayer") {
		return rpcapi.ValidateAudioPlayerResponse(message)
	}
	return nil
}
