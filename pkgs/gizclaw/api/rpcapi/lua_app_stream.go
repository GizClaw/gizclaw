package rpcapi

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// UploadLuaApp sends one archive through a request-owned connection. It closes
// conn and body on every outcome, including early device errors and cancellation.
// body.Close must unblock Read. It never retries or buffers the entire archive.
func UploadLuaApp(ctx context.Context, conn net.Conn, metadata *rpcpb.ClientLuaAppInstallStreamRequest, body io.ReadCloser) (*rpcpb.ClientLuaAppInstallResponse, error) {
	if conn == nil || body == nil {
		return nil, errors.New("lua app: connection and body required")
	}
	defer conn.Close()
	defer body.Close()
	if metadata == nil || ValidateLuaAppRequest(metadata) != nil {
		return nil, errors.New("lua app: invalid metadata")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close(); _ = body.Close() })
	defer stop()
	params := new(RPCPayload)
	if err := params.FromClientLuaAppInstallStreamRequest(metadata); err != nil {
		return nil, err
	}
	req := &RPCRequest{V: RPCVersionV1, Id: "lua-app-install", Method: RPCMethodClientLuaAppInstall, Params: params}
	var eosQueued atomic.Bool
	sent := make(chan error, 1)
	go func() {
		err := WriteRequest(conn, req)
		if err == nil {
			err = writeLuaAppBody(conn, body, metadata.ContentLength)
		}
		if err == nil {
			eosQueued.Store(true)
			err = WriteEOS(conn)
		}
		sent <- err
		if err != nil {
			_ = conn.Close()
		}
	}()
	response, readErr := ReadResponseForMethod(conn, req.Method)
	if readErr == nil {
		readErr = ReadEOS(conn)
	}
	if readErr == nil && response.Error == nil && !eosQueued.Load() {
		readErr = errors.New("lua app: success before request EOS")
	}
	// Interrupting a server HTTP request body can cancel its request context.
	// Preserve an already received device error through our own abort cleanup.
	contextErr := ctx.Err()
	// An error may arrive before upload completion. Success must wait for the
	// writer's EOS, otherwise closing here races the last successful write.
	var sendErr error
	if readErr == nil && response.Error == nil {
		sendErr = <-sent
	} else {
		_ = body.Close()
		_ = conn.Close()
		sendErr = <-sent
	}
	if contextErr != nil {
		return nil, contextErr
	}
	if readErr == nil && response.Error != nil {
		return nil, Error{Code: response.Error.Code, Message: response.Error.Message}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if sendErr != nil {
		return nil, sendErr
	}
	if readErr != nil {
		return nil, readErr
	}
	if response.Id != req.Id || response.Result == nil {
		return nil, errors.New("lua app: invalid response")
	}
	result, err := response.Result.AsClientLuaAppInstallResponse()
	if err != nil {
		return nil, err
	}
	if err := ValidateLuaAppResponse(result); err != nil {
		return nil, err
	}
	return result, nil
}

func writeLuaAppBody(conn net.Conn, body io.Reader, length uint32) error {
	buffer := make([]byte, 32*1024)
	remaining := int64(length)
	for remaining > 0 {
		n, err := io.ReadFull(body, buffer[:min(int64(len(buffer)), remaining)])
		if err != nil {
			return Error{Code: StatusCodeInvalidArgument, Message: "truncated Lua app archive"}
		}
		if err = WriteFrame(conn, Frame{Type: FrameTypeBinary, Payload: buffer[:n]}); err != nil {
			return err
		}
		remaining -= int64(n)
	}
	var extra [1]byte
	if n, err := io.ReadFull(body, extra[:]); n != 0 || err != io.EOF {
		return Error{Code: StatusCodeInvalidArgument, Message: "Lua app archive exceeds declared length or is unreadable"}
	}
	return nil
}
