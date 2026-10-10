package gizcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// LuaAppInstallSession owns a device installation. Write must consume its
// borrowed bytes before returning; synchronous consumption supplies backpressure.
// Finish runs only after exact length, compressed SHA-256 and request EOS are
// verified. It validates the manifest/files/tar/zlib and atomically publishes.
// Close runs exactly once: nil after successful Finish, an error on abort.
// Every operation must honor ctx; no callback may retain the borrowed bytes.
type LuaAppInstallSession interface {
	Write(ctx context.Context, chunk []byte) error
	Finish(ctx context.Context) (*rpcpb.ClientLuaAppInstallResponse, error)
	Close(err error)
}

func (c *rpcClient) handleLuaAppInstall(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = stream.conn.SetDeadline(time.Now()) })
	defer stop()
	reply := func(response *rpcapi.RPCResponse) error {
		if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, response); err != nil {
			return err
		}
		return stream.WriteEOS()
	}
	if req.Params == nil {
		return reply(rpcInvalidParams(req.Id))
	}
	metadata, err := req.Params.AsClientLuaAppInstallStreamRequest()
	if err != nil || rpcapi.ValidateLuaAppRequest(metadata) != nil {
		return reply(rpcInvalidParams(req.Id))
	}
	if c.peer == nil {
		return reply(deviceControlUnsupported(req.Id, req.Method))
	}
	handlers := c.peer.deviceControlHandlers()
	if handlers == nil || handlers.InstallLuaApp == nil {
		return reply(deviceControlUnsupported(req.Id, req.Method))
	}
	c.peer.observeClientRPC(req.Method)
	session, err := handlers.InstallLuaApp(ctx, metadata)
	if err != nil {
		return reply(deviceControlError(req.Id, err))
	}
	if session == nil {
		return reply(deviceControlError(req.Id, errors.New("nil installer")))
	}
	outcome := error(ErrDeviceRejected)
	defer func() { session.Close(outcome) }()
	digest := sha256.New()
	var received uint32
	buffer := make([]byte, rpcapi.MaxFrameSize)
	for {
		frame, err := stream.ReadFrameInto(buffer)
		if err != nil {
			outcome = err
			return err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			break
		}
		if frame.Type != rpcapi.FrameTypeBinary || len(frame.Payload) == 0 || uint32(len(frame.Payload)) > metadata.ContentLength-received {
			return reply(rpcInvalidParams(req.Id))
		}
		received += uint32(len(frame.Payload))
		_, _ = digest.Write(frame.Payload)
		if err := session.Write(ctx, frame.Payload); err != nil {
			outcome = err
			return reply(deviceControlError(req.Id, err))
		}
	}
	if received != metadata.ContentLength || !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), metadata.Sha256) {
		return reply(rpcInvalidParams(req.Id))
	}
	// After request EOS only the terminal response is legal. Observe cancellation
	// and disconnect while the device validates and commits its staged files.
	finishCtx, stopRead := rpcConnContext(stream.conn)
	defer stopRead()
	finishCtx, cancelFinish := context.WithCancel(finishCtx)
	defer cancelFinish()
	stopFinish := context.AfterFunc(ctx, cancelFinish)
	defer stopFinish()
	result, err := session.Finish(finishCtx)
	if err == nil {
		err = rpcapi.ValidateLuaAppResponse(result)
	}
	if err != nil {
		outcome = err
		return reply(deviceControlError(req.Id, err))
	}
	response, err := newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromClientLuaAppInstallResponse)
	if err != nil {
		outcome = err
		return err
	}
	outcome = nil
	return reply(response)
}
