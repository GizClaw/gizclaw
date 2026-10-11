package gizedge

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type notificationTargetConn struct {
	*failingGiznetConn
	calls atomic.Int32
	ack   <-chan struct{}
}

func (c *notificationTargetConn) DialContext(ctx context.Context, service uint64) (net.Conn, error) {
	client, server := net.Pipe()
	c.calls.Add(1)
	go func() {
		defer server.Close()
		req, err := rpcapi.ReadRequest(server)
		if err != nil {
			return
		}
		if err := rpcapi.ReadEOS(server); err != nil {
			return
		}
		if req.Method != rpcapi.RPCMethodServerSocialPingDeliver || service != gizclaw.ServiceEdgeRPC {
			return
		}
		select {
		case <-c.ack:
		case <-ctx.Done():
			return
		}
		var result rpcapi.RPCPayload
		_ = result.FromSocialPingDeliveryResponse(&rpcpb.SocialPingDeliveryResponse{})
		_ = rpcapi.WriteResponseForMethod(server, req.Method, &rpcapi.RPCResponse{V: 1, Id: req.Id, Result: &result})
		_ = rpcapi.WriteEOS(server)
	}()
	return client, nil
}

func TestNotificationRelayRoutesToOwnerAndWaitsForAck(t *testing.T) {
	source, target := testKeyPair(t, 0x71).Public, testKeyPair(t, 0x72).Public
	route := &routeRPCGiznetConn{failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		assignment: &rpcpb.PeerAssignment{ServerPublicKey: target.String()}}
	ack := make(chan struct{})
	destination := &notificationTargetConn{failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished}, ack: ack}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: source}}, conn: route},
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: target}}, conn: destination},
	}}
	var params rpcapi.RPCPayload
	_ = params.FromSocialPingDeliveryRequest(&rpcpb.SocialPingDeliveryRequest{
		TargetPeerPublicKey: giznet.PublicKey{7}.String(), Ping: &rpcpb.ClientSocialPingRequest{FromPeerPublicKey: giznet.PublicKey{8}.String()},
		DeadlineUnixMs: time.Now().Add(time.Second).UnixMilli(),
	})
	req := &rpcapi.RPCRequest{V: 1, Id: "relay", Method: rpcapi.RPCMethodEdgeSocialPingRelay, Params: &params}
	denied := transport.dispatchNotification(t.Context(), giznet.PublicKey{9}, req)
	if denied.Error == nil || denied.Error.Code != rpcapi.StatusCodePermissionDenied {
		t.Fatal("unconfigured Server reached relay")
	}
	done := make(chan *rpcapi.RPCResponse, 1)
	go func() { done <- transport.dispatchNotification(t.Context(), source, req) }()
	select {
	case response := <-done:
		t.Fatalf("relay returned before device ACK: %+v", response)
	case <-time.After(100 * time.Millisecond):
	}
	close(ack)
	response := <-done
	if response.Error != nil || response.Result == nil || destination.calls.Load() != 1 || route.calls.Load() != 1 {
		t.Fatalf("relay response = %+v, delivery calls=%d route calls=%d", response, destination.calls.Load(), route.calls.Load())
	}
}

func TestUpstreamConnectWaitAndCloseRemainCancelable(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	endpoint, _ := url.Parse(server.URL)
	transport := &upstreamTransport{ctx: t.Context(), upstreamURL: endpoint,
		cfg: Config{KeyPair: testKeyPair(t, 0x73), selectedUpstream: UpstreamConfig{PublicKey: testKeyPair(t, 0x74).Public}}}
	first := make(chan error, 1)
	go func() { _, _, err := transport.currentConnContext(t.Context()); first <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("dial never reached signaling")
	}
	waitCtx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := transport.currentConnContext(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting dial = %v, want deadline", err)
	}
	closed := make(chan struct{})
	go func() { _ = transport.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close blocked behind network dial")
	}
	select {
	case err := <-first:
		if err == nil {
			t.Fatal("closed transport published a connection")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel in-flight dial")
	}
	select {
	case <-entered:
		t.Fatal("waiter started duplicate connection")
	default:
	}
}
