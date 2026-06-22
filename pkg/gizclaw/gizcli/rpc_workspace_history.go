package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

type WorkspaceHistoryAudioGetResult struct {
	Metadata rpcapi.WorkspaceHistoryAudioGetResponse
	Bytes    int64
}

func (c *rpcClient) GetWorkspaceHistoryAudio(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryAudioGetRequest, out io.Writer) (WorkspaceHistoryAudioGetResult, error) {
	if out == nil {
		return WorkspaceHistoryAudioGetResult{}, fmt.Errorf("workspace history audio output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromWorkspaceHistoryAudioGetRequest)
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodServerWorkspaceHistoryAudioGet, params)); err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	resp, err := stream.ReadResponse()
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	if resp.Error != nil {
		_ = stream.ReadEOS()
		return WorkspaceHistoryAudioGetResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return WorkspaceHistoryAudioGetResult{}, errRPCMissingResult
	}
	metadata, err := resp.Result.AsWorkspaceHistoryAudioGetResponse()
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, wrapRPCResultError("workspace history audio", err)
	}
	n, err := copyBinaryFrames(out, stream)
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	return WorkspaceHistoryAudioGetResult{Metadata: metadata, Bytes: n}, nil
}
