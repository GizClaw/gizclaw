package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// AssetDownloadResult contains verified metadata and the number of bytes written.
type AssetDownloadResult struct {
	Metadata *rpcpb.AssetMetadata
	Bytes    int64
}

func (c *rpcClient) DownloadAsset(ctx context.Context, conn net.Conn, id string, request rpcpb.AssetDownloadRequest, out io.Writer) (AssetDownloadResult, error) {
	if out == nil {
		return AssetDownloadResult{}, fmt.Errorf("asset download output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromAssetDownloadRequest)
	if err != nil {
		return AssetDownloadResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return AssetDownloadResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodServerAssetDownload, params)); err != nil {
		return AssetDownloadResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return AssetDownloadResult{}, err
	}
	response, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerAssetDownload)
	if err != nil {
		return AssetDownloadResult{}, err
	}
	if response.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return AssetDownloadResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: response.Id, Code: response.Error.Code, Message: response.Error.Message})
	}
	if response.Result == nil {
		return AssetDownloadResult{}, errRPCMissingResult
	}
	result, err := response.Result.AsAssetDownloadResponse()
	if err != nil {
		return AssetDownloadResult{}, wrapRPCResultError("asset download", err)
	}
	metadata := result.GetMetadata()
	if metadata == nil {
		return AssetDownloadResult{}, fmt.Errorf("asset download: missing metadata")
	}
	written, err := copyBinaryFrames(out, stream)
	if err != nil {
		return AssetDownloadResult{}, err
	}
	return AssetDownloadResult{Metadata: metadata, Bytes: written}, nil
}
