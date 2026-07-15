package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type PetPixaDownloadResult struct {
	Metadata rpcapi.PetPixaDownloadResponse
	Bytes    int64
}

type BadgeDefPixaDownloadResult struct {
	Metadata rpcapi.BadgeDefPixaDownloadResponse
	Bytes    int64
}

func (c *rpcClient) DownloadPetPixa(ctx context.Context, conn net.Conn, id string, request rpcapi.PetPixaDownloadRequest, out io.Writer) (PetPixaDownloadResult, error) {
	if out == nil {
		return PetPixaDownloadResult{}, fmt.Errorf("pet pixa download output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromServerPetPixaDownloadRequest)
	if err != nil {
		return PetPixaDownloadResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return PetPixaDownloadResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodServerPetPixaDownload, params)); err != nil {
		return PetPixaDownloadResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return PetPixaDownloadResult{}, err
	}
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerPetPixaDownload)
	if err != nil {
		return PetPixaDownloadResult{}, err
	}
	if resp.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return PetPixaDownloadResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return PetPixaDownloadResult{}, errRPCMissingResult
	}
	metadata, err := resp.Result.AsServerPetPixaDownloadResponse()
	if err != nil {
		return PetPixaDownloadResult{}, wrapRPCResultError("pet pixa download", err)
	}
	n, err := copyBinaryFrames(out, stream)
	if err != nil {
		return PetPixaDownloadResult{}, err
	}
	return PetPixaDownloadResult{Metadata: metadata, Bytes: n}, nil
}

func (c *rpcClient) DownloadBadgeDefPixa(ctx context.Context, conn net.Conn, id string, request rpcapi.BadgeDefPixaDownloadRequest, out io.Writer) (BadgeDefPixaDownloadResult, error) {
	if out == nil {
		return BadgeDefPixaDownloadResult{}, fmt.Errorf("badgedef pixa download output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromBadgeDefPixaDownloadRequest)
	if err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodServerBadgeDefPixaDownload, params)); err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerBadgeDefPixaDownload)
	if err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	if resp.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return BadgeDefPixaDownloadResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return BadgeDefPixaDownloadResult{}, errRPCMissingResult
	}
	metadata, err := resp.Result.AsBadgeDefPixaDownloadResponse()
	if err != nil {
		return BadgeDefPixaDownloadResult{}, wrapRPCResultError("badgedef pixa download", err)
	}
	n, err := copyBinaryFrames(out, stream)
	if err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	return BadgeDefPixaDownloadResult{Metadata: metadata, Bytes: n}, nil
}
