//go:build gizclaw_e2e

package internal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func PreparePushToTalkWorkspace(ctx context.Context, _ string, contextConfigPath, workflow, token string) (string, error) {
	return preparePushToTalkWorkspace(ctx, contextConfigPath, workflow, token, "", true)
}

func PreparePushToTalkWorkspaceNamed(ctx context.Context, _ string, contextConfigPath, workflow, token, name string) (string, error) {
	return preparePushToTalkWorkspace(ctx, contextConfigPath, workflow, token, name, false)
}

func preparePushToTalkWorkspace(ctx context.Context, contextConfigPath, workflow, token, name string, selectWorkspace bool) (string, error) {
	workflow = strings.TrimSpace(workflow)
	if workflow == "" {
		return "", fmt.Errorf("runtime workflow alias is required")
	}
	name = isolatedWorkspaceName(workflow, name)
	if err := context.Cause(ctx); err != nil {
		return "", err
	}
	client, err := NewClient(filepath.Dir(contextConfigPath))
	if err != nil {
		return "", err
	}
	defer client.Close()
	if _, err := client.Register(token); err != nil {
		return "", fmt.Errorf("register C SDK workspace client: %w", err)
	}
	input := rpcpb.WorkspaceInputMode_WORKSPACE_INPUT_MODE_PUSH_TO_TALK
	var response rpcpb.WorkspaceCreateResponse
	if err := client.CallRPC(
		rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_CREATE,
		&rpcpb.WorkspaceCreateRequest{Value: &rpcpb.WorkspaceCreateBody{
			Name: name, Collection: "assistants", WorkflowName: workflow,
			Parameters: &rpcpb.WorkspaceParameters{Value: &rpcpb.WorkspaceParameters_DoubaoRealtimeWorkspaceParameters{
				DoubaoRealtimeWorkspaceParameters: &rpcpb.DoubaoRealtimeWorkspaceParameters{
					AgentType: rpcpb.DoubaoRealtimeWorkspaceParametersAgentType_DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME,
					Input:     &input,
				},
			}},
		}},
		&response,
	); err != nil {
		return "", fmt.Errorf("create C SDK workspace %q: %w", name, err)
	}
	if response.GetValue().GetName() != name {
		return "", fmt.Errorf("created C SDK workspace name %q, want %q", response.GetValue().GetName(), name)
	}
	if selectWorkspace {
		if err := setCSDKRunWorkspace(client, name); err != nil {
			return "", fmt.Errorf("select C SDK workspace %q: %w", name, err)
		}
	}
	return name, nil
}

func CleanupPushToTalkWorkspaces(ctx context.Context, _ string, contextConfigPath, token string, workspaces ...string) (resultErr error) {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	client, err := NewClient(filepath.Dir(contextConfigPath))
	if err != nil {
		return err
	}
	defer client.Close()
	if _, err := client.Register(token); err != nil {
		return fmt.Errorf("register C SDK cleanup client: %w", err)
	}
	var stopResponse rpcpb.ServerStopRunResponse
	if err := client.CallRPC(rpcpb.RpcMethod_RPC_METHOD_SERVER_RUN_STOP, &rpcpb.ServerStopRunRequest{}, &stopResponse); err != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("stop C SDK workspace runtime: %w", err))
	}
	for _, workspace := range workspaces {
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			continue
		}
		var response rpcpb.WorkspaceDeleteResponse
		err := client.CallRPC(rpcpb.RpcMethod_RPC_METHOD_SERVER_WORKSPACE_DELETE, &rpcpb.WorkspaceDeleteRequest{Name: workspace}, &response)
		if err != nil && !isCSDKNotFound(err) {
			resultErr = errors.Join(resultErr, fmt.Errorf("delete C SDK workspace %q: %w", workspace, err))
		}
	}
	return resultErr
}

func isolatedWorkspaceName(workflow, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return fmt.Sprintf("%s-ptt-%x", strings.TrimSpace(workflow), time.Now().UnixNano())
}

func isCSDKNotFound(err error) bool {
	var rpcErr *RPCError
	return errors.As(err, &rpcErr) && rpcErr.Code == rpcpb.RpcErrorCode_RPC_ERROR_CODE_NOT_FOUND
}
