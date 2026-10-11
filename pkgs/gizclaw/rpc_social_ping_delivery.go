package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// SocialPingDeliveryTimeout bounds the entire notification path, including
// routing and the final device acknowledgement.
const SocialPingDeliveryTimeout = 3 * time.Second

// SocialPingDeliveryContext validates a supported notification and preserves
// its end-to-end deadline. Each receiver also caps its local work at three seconds.
func SocialPingDeliveryContext(ctx context.Context, request *rpcpb.SocialPingDeliveryRequest) (context.Context, context.CancelFunc, error) {
	if request == nil || request.Ping == nil {
		return nil, nil, errors.New("social ping delivery: ping required")
	}
	for _, value := range []string{request.TargetPeerPublicKey, request.Ping.FromPeerPublicKey} {
		key, err := parseSocialPingPeer(value)
		if err != nil || key.String() != value {
			return nil, nil, errors.New("social ping delivery: invalid peer public key")
		}
	}
	ping := request.Ping
	if !utf8.ValidString(ping.GetFromDisplayName()) || len(ping.GetFromDisplayName()) > 256 {
		return nil, nil, errors.New("social ping delivery: invalid display name")
	}
	if ping.FriendGroupName != nil && (!utf8.ValidString(*ping.FriendGroupName) ||
		strings.TrimSpace(*ping.FriendGroupName) == "" || len(*ping.FriendGroupName) > 255) {
		return nil, nil, errors.New("social ping delivery: invalid group name")
	}
	if request.DeadlineUnixMs <= 0 {
		return nil, nil, errors.New("social ping delivery: deadline required")
	}
	deadline := time.UnixMilli(request.DeadlineUnixMs)
	if !deadline.After(time.Now()) {
		return nil, nil, context.DeadlineExceeded
	}
	if maximum := time.Now().Add(SocialPingDeliveryTimeout); deadline.After(maximum) {
		deadline = maximum
	}
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	return callCtx, cancel, nil
}

// CallSocialPingDelivery performs exactly one internal notification RPC on an
// authenticated connection. Only device acknowledgement is a successful result.
// The caller owns the parent connection; this call owns and closes its stream.
func CallSocialPingDelivery(ctx context.Context, conn giznet.Conn, method rpcapi.RPCMethod, request *rpcpb.SocialPingDeliveryRequest) error {
	if method != rpcapi.RPCMethodEdgeSocialPingRelay && method != rpcapi.RPCMethodServerSocialPingDeliver {
		return errors.New("social ping delivery: unsupported method")
	}
	callCtx, cancel, err := SocialPingDeliveryContext(ctx, request)
	if err != nil {
		return err
	}
	defer cancel()
	stream, err := dialSocialPingStream(callCtx, conn, ServiceEdgeRPC)
	if err != nil {
		return err
	}
	defer stream.Close()
	stop := context.AfterFunc(callCtx, func() { _ = stream.Close() })
	defer stop()
	deadline, _ := callCtx.Deadline()
	if err := stream.SetDeadline(deadline); err != nil {
		return err
	}
	var params rpcapi.RPCPayload
	if err := params.FromSocialPingDeliveryRequest(request); err != nil {
		return err
	}
	id := gizlog.RequestID(ctx)
	if id == "" {
		id, err = gizlog.NewID()
		if err != nil {
			return err
		}
	}
	if err := rpcapi.WriteRequest(stream, &rpcapi.RPCRequest{V: rpcapi.RPCVersionV1, Id: id, Method: method, Params: &params}); err != nil {
		return err
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		return err
	}
	response, err := rpcapi.ReadResponseForMethod(stream, method)
	if err != nil {
		return err
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		return err
	}
	if response.Id != id {
		return errors.New("social ping delivery: response id mismatch")
	}
	if response.Error != nil {
		return rpcapi.Error{Code: response.Error.Code, Message: response.Error.Message, Reason: response.Error.Reason}
	}
	if response.Result == nil {
		return errors.New("social ping delivery: missing acknowledgement")
	}
	_, err = response.Result.AsSocialPingDeliveryResponse()
	return err
}

func dialSocialPingStream(ctx context.Context, conn giznet.Conn, service uint64) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dialer, ok := conn.(giznet.ContextDialer); ok {
		return dialer.DialContext(ctx, service)
	}
	if conn == nil {
		return nil, ErrDeviceOffline
	}
	return conn.Dial(service)
}

func socialPingDeliveryError(id string, err error) *rpcapi.RPCResponse {
	code := rpcapi.StatusCodeUnavailable
	var status rpcapi.Error
	switch {
	case errors.As(err, &status):
		code = status.Code
	case errors.Is(err, context.DeadlineExceeded):
		code = rpcapi.StatusCodeDeadlineExceeded
	case errors.Is(err, context.Canceled):
		code = rpcapi.StatusCodeCancelled
	case errors.Is(err, ErrDeviceOffline):
		code = rpcapi.StatusCodeNotFound
	}
	return rpcapi.Error{RequestID: id, Code: code, Message: fmt.Sprintf("social ping delivery: %s", code)}.RPCResponse()
}
