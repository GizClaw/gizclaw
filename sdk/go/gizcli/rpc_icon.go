package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type WorkflowIconDownloadResult struct {
	Metadata rpcapi.WorkflowIconDownloadResponse
	Bytes    int64
}

type WorkspaceIconDownloadResult struct {
	Metadata rpcapi.WorkspaceIconDownloadResponse
	Bytes    int64
}

func (c *rpcClient) DownloadWorkflowIcon(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkflowIconDownloadRequest, out io.Writer) (WorkflowIconDownloadResult, error) {
	metadata, n, err := downloadIcon(ctx, conn, id, rpcapi.RPCMethodServerWorkflowIconDownload, request, (*rpcapi.RPCPayload).FromWorkflowIconDownloadRequest, rpcapi.RPCPayload.AsWorkflowIconDownloadResponse, out)
	return WorkflowIconDownloadResult{Metadata: metadata, Bytes: n}, err
}

func (c *rpcClient) DownloadWorkspaceIcon(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceIconDownloadRequest, out io.Writer) (WorkspaceIconDownloadResult, error) {
	metadata, n, err := downloadIcon(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceIconDownload, request, (*rpcapi.RPCPayload).FromWorkspaceIconDownloadRequest, rpcapi.RPCPayload.AsWorkspaceIconDownloadResponse, out)
	return WorkspaceIconDownloadResult{Metadata: metadata, Bytes: n}, err
}

func downloadIcon[Request, Response any](ctx context.Context, conn net.Conn, id string, method rpcapi.RPCMethod, request Request, encode func(*rpcapi.RPCPayload, Request) error, decode func(rpcapi.RPCPayload) (Response, error), out io.Writer) (Response, int64, error) {
	var zero Response
	if out == nil {
		return zero, 0, fmt.Errorf("icon download output is required")
	}
	params, err := newRPCRequestParams(request, encode)
	if err != nil {
		return zero, 0, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return zero, 0, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, method, params)); err != nil {
		return zero, 0, err
	}
	if err := stream.WriteEOS(); err != nil {
		return zero, 0, err
	}
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(method)
	if err != nil {
		return zero, 0, err
	}
	if resp.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return zero, 0, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return zero, 0, errRPCMissingResult
	}
	metadata, err := decode(*resp.Result)
	if err != nil {
		return zero, 0, wrapRPCResultError("icon download", err)
	}
	n, err := copyBinaryFrames(out, stream)
	return metadata, n, err
}
