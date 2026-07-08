package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type FirmwareDownloadResult struct {
	Metadata rpcapi.FirmwareFilesDownloadResponse
	Bytes    int64
}

func (c *rpcClient) ListFirmwares(ctx context.Context, conn net.Conn, id string, request rpcapi.FirmwareListRequest) (*rpcapi.FirmwareListResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFirmwareList, request, (*rpcapi.RPCRequest_Params).FromFirmwareListRequest, rpcapi.RPCResponse_Result.AsFirmwareListResponse, "firmware list")
}

func (c *rpcClient) GetFirmware(ctx context.Context, conn net.Conn, id string, request rpcapi.FirmwareGetRequest) (*rpcapi.FirmwareGetResponse, error) {
	return callResourceRPC(ctx, conn, id, rpcapi.RPCMethodServerFirmwareGet, request, (*rpcapi.RPCRequest_Params).FromFirmwareGetRequest, rpcapi.RPCResponse_Result.AsFirmwareGetResponse, "firmware get")
}

func (c *rpcClient) DownloadFirmware(ctx context.Context, conn net.Conn, id string, request rpcapi.FirmwareFilesDownloadRequest, out io.Writer) (FirmwareDownloadResult, error) {
	if out == nil {
		return FirmwareDownloadResult{}, fmt.Errorf("firmware download output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromFirmwareFilesDownloadRequest)
	if err != nil {
		return FirmwareDownloadResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return FirmwareDownloadResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodServerFirmwareFilesDownload, params)); err != nil {
		return FirmwareDownloadResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return FirmwareDownloadResult{}, err
	}
	resp, err := stream.ReadResponse()
	if err != nil {
		return FirmwareDownloadResult{}, err
	}
	if resp.Error != nil {
		_ = stream.ReadEOS()
		return FirmwareDownloadResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return FirmwareDownloadResult{}, errRPCMissingResult
	}
	metadata, err := resp.Result.AsFirmwareFilesDownloadResponse()
	if err != nil {
		return FirmwareDownloadResult{}, wrapRPCResultError("firmware download", err)
	}
	n, err := copyBinaryFrames(out, stream)
	if err != nil {
		return FirmwareDownloadResult{}, err
	}
	return FirmwareDownloadResult{Metadata: metadata, Bytes: n}, nil
}

func copyBinaryFrames(out io.Writer, stream *rpcStream) (int64, error) {
	var written int64
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			return written, err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			return written, nil
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			return written, fmt.Errorf("rpc: expected binary frame, got type %d", frame.Type)
		}
		n, err := out.Write(frame.Payload)
		written += int64(n)
		if err != nil {
			return written, err
		}
		if n != len(frame.Payload) {
			return written, io.ErrShortWrite
		}
	}
}
