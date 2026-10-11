package gizedge

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// listenNotifications runs only on outbound, identity-verified Server
// connections (control and gateway pool transports). Device ingress never
// registers this service. Closing the parent closes the listener and streams.
func (t *orderedUpstreamTransport) listenNotifications(conn giznet.Conn) {
	listener := conn.ListenService(gizclaw.ServiceEdgeRPC)
	go func() {
		defer listener.Close()
		for {
			stream, err := listener.Accept()
			if err != nil {
				return
			}
			go t.handleNotification(conn.PublicKey(), stream)
		}
	}()
}

func (t *orderedUpstreamTransport) handleNotification(server giznet.PublicKey, stream net.Conn) {
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(gizclaw.SocialPingDeliveryTimeout))
	req, err := rpcapi.ReadRequest(stream)
	if err != nil {
		return
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gizclaw.SocialPingDeliveryTimeout)
	stop := context.AfterFunc(ctx, func() { _ = stream.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		var extra [1]byte
		_, _ = stream.Read(extra[:])
		cancel()
	}()
	defer func() {
		stop()
		cancel()
		_ = stream.Close()
		<-done
	}()
	response := t.dispatchNotification(ctx, server, req)
	if err := rpcapi.WriteResponseForMethod(stream, req.Method, response); err != nil {
		return
	}
	_ = rpcapi.WriteEOS(stream)
}

func (t *orderedUpstreamTransport) dispatchNotification(parent context.Context, server giznet.PublicKey, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	status := func(code rpcapi.StatusCode) *rpcapi.RPCResponse {
		return rpcapi.Error{RequestID: req.Id, Code: code, Message: "notification relay: " + code.String()}.RPCResponse()
	}
	trusted := false
	for _, entry := range t.entries {
		trusted = trusted || entry.cfg.selectedUpstream.PublicKey == server
	}
	if !trusted {
		return status(rpcapi.StatusCodePermissionDenied)
	}
	if req.Method != rpcapi.RPCMethodEdgeSocialPingRelay {
		return status(rpcapi.StatusCodeUnimplemented)
	}
	if req.Params == nil {
		return status(rpcapi.StatusCodeInvalidArgument)
	}
	request, err := req.Params.AsSocialPingDeliveryRequest()
	if err != nil {
		return status(rpcapi.StatusCodeInvalidArgument)
	}
	ctx, cancel, err := gizclaw.SocialPingDeliveryContext(parent, request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return status(rpcapi.StatusCodeDeadlineExceeded)
		}
		return status(rpcapi.StatusCodeInvalidArgument)
	}
	defer cancel()
	ctx = gizlog.WithRequestID(ctx, req.Id)
	if err := t.relaySocialPing(ctx, server, request); err != nil {
		if rpcErr, ok := errors.AsType[rpcapi.Error](err); ok {
			return status(rpcErr.Code)
		}
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return status(rpcapi.StatusCodeDeadlineExceeded)
		}
		return status(rpcapi.StatusCodeUnavailable)
	}
	var result rpcapi.RPCPayload
	if err := result.FromSocialPingDeliveryResponse(&rpcpb.SocialPingDeliveryResponse{}); err != nil {
		return status(rpcapi.StatusCodeInternal)
	}
	return &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: req.Id, Result: &result}
}

func (t *orderedUpstreamTransport) relaySocialPing(ctx context.Context, source giznet.PublicKey, request *rpcpb.SocialPingDeliveryRequest) error {
	var sourceEntry *upstreamTransport
	for _, entry := range t.entries {
		if entry.cfg.selectedUpstream.PublicKey == source {
			sourceEntry = entry
			break
		}
	}
	if sourceEntry == nil {
		return rpcapi.Error{Code: rpcapi.StatusCodePermissionDenied}
	}
	conn, _, err := sourceEntry.currentConnContext(ctx)
	if err != nil {
		return err
	}
	release, err := sourceEntry.acquireSlot(ctx)
	if err != nil {
		return err
	}
	var target giznet.PublicKey
	_ = target.UnmarshalText([]byte(request.TargetPeerPublicKey))
	assignment, err := resolvePeerAssignment(ctx, conn, target)
	release()
	if err != nil {
		if errors.Is(err, errRouteAssignmentNotFound) {
			return rpcapi.Error{Code: rpcapi.StatusCodeNotFound}
		}
		return err
	}
	entry, err := t.entryForAssignment(assignment)
	if err != nil {
		return err
	}
	conn, _, err = entry.currentConnContext(ctx)
	if err != nil {
		return err
	}
	release, err = entry.acquireSlot(ctx)
	if err != nil {
		return err
	}
	defer release()
	err = gizclaw.CallSocialPingDelivery(ctx, conn, rpcapi.RPCMethodServerSocialPingDeliver, request)
	slog.DebugContext(ctx, "social ping routed", "target_peer_public_key", request.TargetPeerPublicKey,
		"source_server_public_key", source.String(), "target_server_public_key", assignment.ServerPublicKey,
		"acknowledged", err == nil)
	return err
}
