package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type PetDefPixaDownloadResult struct {
	Metadata rpcapi.PetDefPixaDownloadResponse
	Bytes    int64
}

type BadgeDefPixaDownloadResult struct {
	Metadata rpcapi.BadgeDefPixaDownloadResponse
	Bytes    int64
}

func (c *rpcClient) DownloadPetDefPixa(ctx context.Context, conn net.Conn, id string, request rpcapi.PetDefPixaDownloadRequest, out io.Writer) (PetDefPixaDownloadResult, error) {
	if out == nil {
		return PetDefPixaDownloadResult{}, fmt.Errorf("petdef pixa download output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromPetDefPixaDownloadRequest)
	if err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodServerPetDefPixaDownload, params)); err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerPetDefPixaDownload)
	if err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	if resp.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return PetDefPixaDownloadResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return PetDefPixaDownloadResult{}, errRPCMissingResult
	}
	metadata, err := resp.Result.AsPetDefPixaDownloadResponse()
	if err != nil {
		return PetDefPixaDownloadResult{}, wrapRPCResultError("petdef pixa download", err)
	}
	n, err := copyBinaryFrames(out, stream)
	if err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	return PetDefPixaDownloadResult{Metadata: metadata, Bytes: n}, nil
}

func (c *rpcClient) DownloadBadgeDefPixa(ctx context.Context, conn net.Conn, id string, request rpcapi.BadgeDefPixaDownloadRequest, out io.Writer) (BadgeDefPixaDownloadResult, error) {
	if out == nil {
		return BadgeDefPixaDownloadResult{}, fmt.Errorf("badgedef pixa download output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromBadgeDefPixaDownloadRequest)
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
