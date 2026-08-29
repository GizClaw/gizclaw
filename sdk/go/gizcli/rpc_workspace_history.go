package gizcli

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type WorkspaceHistoryAudioDownloadResult struct {
	Metadata rpcapi.WorkspaceHistoryAudioDownloadResponse
	Bytes    int64
}

type FriendGroupMessageAudioDownloadResult struct {
	Metadata rpcapi.FriendGroupMessageAudioDownloadResponse
	Bytes    int64
}

func (c *rpcClient) DownloadWorkspaceHistoryAudio(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryAudioDownloadRequest, out io.Writer) (WorkspaceHistoryAudioDownloadResult, error) {
	metadata, n, err := downloadHistoryAudio(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceHistoryAudioDownload, request, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioDownloadRequest, rpcapi.RPCPayload.AsWorkspaceHistoryAudioDownloadResponse, out, func(metadata rpcapi.WorkspaceHistoryAudioDownloadResponse) error {
		if metadata.WorkspaceName != request.WorkspaceName || metadata.HistoryName != request.HistoryName {
			return fmt.Errorf("workspace history audio metadata identity mismatch")
		}
		return validateHistoryAudioMetadata(metadata.MimeType, metadata.SizeBytes)
	})
	if err != nil {
		return WorkspaceHistoryAudioDownloadResult{}, err
	}
	return WorkspaceHistoryAudioDownloadResult{Metadata: metadata, Bytes: n}, nil
}

func (c *rpcClient) DownloadFriendGroupMessageAudio(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageAudioDownloadRequest, out io.Writer) (FriendGroupMessageAudioDownloadResult, error) {
	metadata, n, err := downloadHistoryAudio(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesAudioDownload, request, (*rpcapi.RPCPayload).FromFriendGroupMessageAudioDownloadRequest, rpcapi.RPCPayload.AsFriendGroupMessageAudioDownloadResponse, out, func(metadata rpcapi.FriendGroupMessageAudioDownloadResponse) error {
		if metadata.FriendGroupName != request.FriendGroupName || metadata.HistoryName != request.HistoryName {
			return fmt.Errorf("friend group message audio metadata identity mismatch")
		}
		return validateHistoryAudioMetadata(metadata.MimeType, metadata.SizeBytes)
	})
	if err != nil {
		return FriendGroupMessageAudioDownloadResult{}, err
	}
	return FriendGroupMessageAudioDownloadResult{Metadata: metadata, Bytes: n}, nil
}

func downloadHistoryAudio[Request, Metadata any](ctx context.Context, conn net.Conn, id string, method rpcapi.RPCMethod, request Request, encode func(*rpcapi.RPCPayload, Request) error, decode func(rpcapi.RPCPayload) (Metadata, error), out io.Writer, validate func(Metadata) error) (Metadata, int64, error) {
	var zero Metadata
	if out == nil {
		return zero, 0, fmt.Errorf("history audio output is required")
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
		return zero, 0, wrapRPCResultError("history audio", err)
	}
	if err := validate(metadata); err != nil {
		return zero, 0, err
	}
	n, err := copyBinaryFrames(out, stream)
	if err != nil {
		return zero, 0, err
	}
	expectedSize := historyAudioSize(metadata)
	if n != expectedSize {
		return zero, 0, fmt.Errorf("history audio size mismatch: metadata=%d stream=%d", expectedSize, n)
	}
	return metadata, n, nil
}

func validateHistoryAudioMetadata(mimeType string, sizeBytes int64) error {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(mimeType))
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "audio/") {
		return fmt.Errorf("invalid history audio MIME type %q", mimeType)
	}
	if sizeBytes < 0 {
		return fmt.Errorf("invalid history audio size %d", sizeBytes)
	}
	return nil
}

func historyAudioSize(metadata any) int64 {
	switch value := metadata.(type) {
	case rpcapi.WorkspaceHistoryAudioDownloadResponse:
		return value.SizeBytes
	case rpcapi.FriendGroupMessageAudioDownloadResponse:
		return value.SizeBytes
	default:
		panic("unsupported history audio metadata")
	}
}
