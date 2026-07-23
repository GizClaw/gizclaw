package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type DeviceRPCClient interface {
	ToolPeerAvailable(peerID string) bool
	InvokePeerTool(context.Context, string, rpcapi.ToolInvokeRequest) (rpcapi.ToolInvokeResponse, error)
}

type DeviceRPCExecutor struct {
	Client DeviceRPCClient
}

func (e *DeviceRPCExecutor) ToolAvailable(_ context.Context, tool Tool) (bool, error) {
	if tool.Executor.Kind != ToolExecutorKindDeviceRPC {
		return false, nil
	}
	peerID, err := deviceToolPeerID(tool)
	if err != nil {
		return false, err
	}
	return e != nil && e.Client != nil && e.Client.ToolPeerAvailable(peerID), nil
}

func (e *DeviceRPCExecutor) Invoke(ctx context.Context, call Call) (Result, error) {
	if e == nil || e.Client == nil {
		return Result{}, ErrNotConfigured
	}
	if call.Tool.Executor.Kind != ToolExecutorKindDeviceRPC {
		return Result{}, fmt.Errorf("%w: %s", ErrExecutorNotFound, call.Tool.ID)
	}
	peerID, err := deviceToolPeerID(call.Tool)
	if err != nil {
		return Result{}, err
	}
	if !e.Client.ToolPeerAvailable(peerID) {
		return Result{}, fmt.Errorf("%w: %s", ErrExecutorUnavailable, peerID)
	}
	args := map[string]any{}
	if len(call.Args) > 0 {
		if err := json.Unmarshal(call.Args, &args); err != nil || args == nil {
			return Result{}, fmt.Errorf("%w: tool arguments must be a JSON object", ErrInvalidTool)
		}
	}
	response, err := e.Client.InvokePeerTool(ctx, peerID, rpcapi.ToolInvokeRequest{
		CallId: call.ID,
		ToolId: call.Tool.ID,
		Method: trimPtr(call.Tool.Executor.Method),
		Args:   args,
	})
	if err != nil {
		if errors.Is(err, ErrExecutorUnavailable) {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("toolkit: device executor %s: %w", call.Tool.ID, err)
	}
	if len(response.DataJson) == 0 {
		response.DataJson = "null"
	}
	if !json.Valid([]byte(response.DataJson)) {
		return Result{}, fmt.Errorf("toolkit: device executor %s returned invalid JSON", call.Tool.ID)
	}
	return Result{Data: cloneRaw(json.RawMessage(response.DataJson))}, nil
}

func deviceToolPeerID(tool Tool) (string, error) {
	owner := strings.TrimSpace(valueOrEmpty(tool.OwnerPeer))
	peerID := strings.TrimSpace(valueOrEmpty(tool.Executor.PeerID))
	if owner != "" && peerID != "" && owner != peerID {
		return "", fmt.Errorf("%w: owner_peer and executor.peer_id conflict", ErrInvalidTool)
	}
	if peerID != "" {
		return peerID, nil
	}
	if owner != "" {
		return owner, nil
	}
	return "", fmt.Errorf("%w: device executor peer is required", ErrInvalidTool)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
