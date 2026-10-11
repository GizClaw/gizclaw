package gizclaw

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func notificationRequest() *rpcpb.SocialPingDeliveryRequest {
	return &rpcpb.SocialPingDeliveryRequest{TargetPeerPublicKey: giznet.PublicKey{1}.String(),
		Ping:           &rpcpb.ClientSocialPingRequest{FromPeerPublicKey: giznet.PublicKey{2}.String()},
		DeadlineUnixMs: time.Now().Add(time.Second).UnixMilli()}
}

func TestSocialPingDeliveryValidatesAndPreservesDeadline(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*rpcpb.SocialPingDeliveryRequest)
	}{
		{"missing-ping", func(req *rpcpb.SocialPingDeliveryRequest) { req.Ping = nil }},
		{"bad-target", func(req *rpcpb.SocialPingDeliveryRequest) { req.TargetPeerPublicKey = "bad" }},
		{"bad-sender", func(req *rpcpb.SocialPingDeliveryRequest) { req.Ping.FromPeerPublicKey = "bad" }},
		{"zero-deadline", func(req *rpcpb.SocialPingDeliveryRequest) { req.DeadlineUnixMs = 0 }},
		{"expired", func(req *rpcpb.SocialPingDeliveryRequest) {
			req.DeadlineUnixMs = time.Now().Add(-time.Second).UnixMilli()
		}},
		{"long-name", func(req *rpcpb.SocialPingDeliveryRequest) { req.Ping.FromDisplayName = new(strings.Repeat("x", 257)) }},
		{"empty-group", func(req *rpcpb.SocialPingDeliveryRequest) { req.Ping.FriendGroupName = new("") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := notificationRequest()
			test.mutate(request)
			if _, cancel, err := SocialPingDeliveryContext(t.Context(), request); err == nil {
				cancel()
				t.Fatal("invalid notification accepted")
			}
		})
	}
	request := notificationRequest()
	ctx, cancel, err := SocialPingDeliveryContext(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	deadline, _ := ctx.Deadline()
	if deadline.UnixMilli() != request.DeadlineUnixMs {
		t.Fatal("relay reset end-to-end deadline")
	}
	request.DeadlineUnixMs = time.Now().Add(time.Hour).UnixMilli()
	ctx, cancel, err = SocialPingDeliveryContext(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	deadline, _ = ctx.Deadline()
	if time.Until(deadline) > SocialPingDeliveryTimeout {
		t.Fatal("receiver did not cap excessive deadline")
	}
}

type socialPingRPCConn struct {
	giznet.Conn
	dial func(context.Context, uint64) (net.Conn, error)
}

func (c socialPingRPCConn) DialContext(ctx context.Context, service uint64) (net.Conn, error) {
	return c.dial(ctx, service)
}

func TestSocialPingDeliveryCancellationClosesOnlyItsStream(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	started, closed := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(closed)
		if _, err := rpcapi.ReadRequest(server); err != nil {
			return
		}
		if err := rpcapi.ReadEOS(server); err != nil {
			return
		}
		close(started)
		var buf [1]byte
		_, _ = server.Read(buf[:])
	}()
	var calls atomic.Int32
	conn := socialPingRPCConn{dial: func(_ context.Context, service uint64) (net.Conn, error) {
		calls.Add(1)
		if service != ServiceEdgeRPC {
			t.Errorf("notification opened service %d", service)
		}
		return client, nil
	}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- CallSocialPingDelivery(ctx, conn, rpcapi.RPCMethodEdgeSocialPingRelay, notificationRequest())
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled request counted as ACK")
		}
	case <-time.After(time.Second):
		t.Fatal("notification ignored cancellation")
	}
	<-closed
	if calls.Load() != 1 {
		t.Fatal("notification retried after sending")
	}
}

func TestInternalSocialPingRejectsUntrustedAndNeverRelaysAgain(t *testing.T) {
	manager := NewManager(nil)
	var params rpcapi.RPCPayload
	if err := params.FromSocialPingDeliveryRequest(notificationRequest()); err != nil {
		t.Fatal(err)
	}
	req := &rpcapi.RPCRequest{Id: "deliver", Method: rpcapi.RPCMethodServerSocialPingDeliver, Params: &params}
	handler := &edgeRPCServer{notifications: manager, authorizeNotification: func(context.Context) bool { return false }}
	response, err := handler.dispatch(t.Context(), req)
	if err != nil || response.Error == nil || response.Error.Code != rpcapi.StatusCodePermissionDenied {
		t.Fatalf("untrusted notification = %+v, %v", response, err)
	}
	handler.authorizeNotification = func(context.Context) bool { return true }
	response, err = handler.dispatch(t.Context(), req)
	if err != nil || response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("offline terminal delivery = %+v, %v", response, err)
	}
}
