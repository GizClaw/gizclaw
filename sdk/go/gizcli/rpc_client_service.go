package gizcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const (
	maxClientToolArgumentsBytes = 64 << 10
	maxClientToolResultBytes    = 64 << 10
)

var clientToolNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

type ToolHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

// HandleTool mounts one current-Client Tool handler by canonical Resource name.
func (c *Client) HandleTool(name string, handler ToolHandler) error {
	if c == nil {
		return errors.New("gizclaw: nil client")
	}
	if !clientToolNamePattern.MatchString(name) {
		return fmt.Errorf("gizclaw: invalid Tool name %q", name)
	}
	if handler == nil {
		return errors.New("gizclaw: Tool handler is required")
	}
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	if c.toolHandlers == nil {
		c.toolHandlers = make(map[string]ToolHandler)
	}
	c.toolHandlers[name] = handler
	return nil
}

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

func (c *rpcClient) handleGetClientInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientGetInfoRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer client not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ClientGetInfoResponse](peerDeviceToPeerRefreshInfo(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientGetInfoResponse)
}

func (c *rpcClient) handleGetClientIdentifiers(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsClientGetIdentifiersRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer client not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ClientGetIdentifiersResponse](peerDeviceToPeerRefreshIdentifiers(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientGetIdentifiersResponse)
}

func (c *rpcClient) handleInvokeTool(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsToolInvokeRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(params.InvokeName)
	if c.peer == nil || !clientToolNamePattern.MatchString(name) {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid Tool name"}.RPCResponse(), nil
	}
	c.peer.toolMu.RLock()
	handler := c.peer.toolHandlers[name]
	c.peer.toolMu.RUnlock()
	if handler == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "Tool unavailable"}.RPCResponse(), nil
	}
	args, err := json.Marshal(params.Args)
	if err != nil || len(args) > maxClientToolArgumentsBytes {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid Tool arguments"}.RPCResponse(), nil
	}
	result, err := handler(ctx, json.RawMessage(args))
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "Tool handler failed"}.RPCResponse(), nil
	}
	if len(result) == 0 {
		result = json.RawMessage(`null`)
	}
	if len(result) > maxClientToolResultBytes || !json.Valid(result) {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "Tool handler returned invalid JSON"}.RPCResponse(), nil
	}
	return newRPCResultResponse(req.Id, rpcapi.ToolInvokeResponse{DataJson: string(result)}, (*rpcapi.RPCPayload).FromToolInvokeResponse)
}
