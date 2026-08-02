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

type WorkspaceHistoryAudioGetResult struct {
	Metadata rpcapi.WorkspaceHistoryAudioGetResponse
	Bytes    int64
}

type FriendGroupMessageAudioGetResult struct {
	Metadata rpcapi.FriendGroupMessageAudioGetResponse
	Bytes    int64
}

func (c *rpcClient) GetWorkspaceHistoryAudio(ctx context.Context, conn net.Conn, id string, request rpcapi.WorkspaceHistoryAudioGetRequest, out io.Writer) (WorkspaceHistoryAudioGetResult, error) {
	metadata, n, err := getHistoryAudio(ctx, conn, id, rpcapi.RPCMethodServerWorkspaceHistoryAudioGet, request, (*rpcapi.RPCPayload).FromWorkspaceHistoryAudioGetRequest, rpcapi.RPCPayload.AsWorkspaceHistoryAudioGetResponse, out, func(metadata rpcapi.WorkspaceHistoryAudioGetResponse) error {
		if metadata.WorkspaceName != request.WorkspaceName || metadata.HistoryId != request.HistoryId {
			return fmt.Errorf("workspace history audio metadata identity mismatch")
		}
		return validateHistoryAudioMetadata(metadata.MimeType, metadata.SizeBytes)
	})
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	return WorkspaceHistoryAudioGetResult{Metadata: metadata, Bytes: n}, nil
}

func (c *rpcClient) GetFriendGroupMessageAudio(ctx context.Context, conn net.Conn, id string, request rpcapi.FriendGroupMessageAudioGetRequest, out io.Writer) (FriendGroupMessageAudioGetResult, error) {
	metadata, n, err := getHistoryAudio(ctx, conn, id, rpcapi.RPCMethodServerFriendGroupMessagesAudioGet, request, (*rpcapi.RPCPayload).FromFriendGroupMessageAudioGetRequest, rpcapi.RPCPayload.AsFriendGroupMessageAudioGetResponse, out, func(metadata rpcapi.FriendGroupMessageAudioGetResponse) error {
		if metadata.FriendGroupName != request.FriendGroupName || metadata.HistoryId != request.HistoryId {
			return fmt.Errorf("friend group message audio metadata identity mismatch")
		}
		return validateHistoryAudioMetadata(metadata.MimeType, metadata.SizeBytes)
	})
	if err != nil {
		return FriendGroupMessageAudioGetResult{}, err
	}
	return FriendGroupMessageAudioGetResult{Metadata: metadata, Bytes: n}, nil
}

func getHistoryAudio[Request, Metadata any](ctx context.Context, conn net.Conn, id string, method rpcapi.RPCMethod, request Request, encode func(*rpcapi.RPCPayload, Request) error, decode func(rpcapi.RPCPayload) (Metadata, error), out io.Writer, validate func(Metadata) error) (Metadata, int64, error) {
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
	case rpcapi.WorkspaceHistoryAudioGetResponse:
		return value.SizeBytes
	case rpcapi.FriendGroupMessageAudioGetResponse:
		return value.SizeBytes
	default:
		panic("unsupported history audio metadata")
	}
}
