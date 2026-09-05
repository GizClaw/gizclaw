package gizedge

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestGatewayClientSecurityPolicyRejectsRouteRPC(t *testing.T) {
	if (gatewayClientSecurityPolicy{}).AllowService(giznet.PublicKey{}, gizclaw.ServiceEdgeRPC) {
		t.Fatal("gateway client policy exposes the Edge-only route RPC service")
	}
}

func TestConfiguredPoolForAssignmentFailsClosed(t *testing.T) {
	configured := testKeyPair(t, 0x91).Public
	unknown := testKeyPair(t, 0x92).Public
	want := &gatewayPool{}
	gateway := &Gateway{pools: map[giznet.PublicKey]*gatewayPool{configured: want}}
	got, err := gateway.configuredPoolForAssignment(&rpcpb.PeerAssignment{ServerPublicKey: configured.String()})
	if err != nil || got != want {
		t.Fatalf("configured assignment = %p, %v; want %p", got, err, want)
	}
	for _, test := range []struct {
		name       string
		assignment *rpcpb.PeerAssignment
		want       string
	}{
		{name: "missing", want: "no assignment"},
		{name: "invalid", assignment: &rpcpb.PeerAssignment{ServerPublicKey: "bad"}, want: "invalid server identity"},
		{name: "unconfigured", assignment: &rpcpb.PeerAssignment{ServerPublicKey: unknown.String()}, want: "unconfigured server identity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := gateway.configuredPoolForAssignment(test.assignment); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("configuredPoolForAssignment() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAcquirePeerUpstreamFreshLookupMovesFirstClaimLoserToOwner(t *testing.T) {
	serverA := testKeyPair(t, 0x93).Public
	serverB := testKeyPair(t, 0x94).Public
	peer := testKeyPair(t, 0x95).Public
	routeConn := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
	}
	poolA, entryA := routeTestPool(t, &failingGiznetConn{state: giznet.PeerStateEstablished})
	poolB, entryB := routeTestPool(t, &failingGiznetConn{state: giznet.PeerStateEstablished})
	gateway := &Gateway{
		cfg: Config{Upstreams: []UpstreamConfig{{PublicKey: serverA}, {PublicKey: serverB}}},
		pools: map[giznet.PublicKey]*gatewayPool{
			serverA: poolA,
			serverB: poolB,
		},
		poolOrder: []*gatewayPool{poolA, poolB},
		resolvePeerRoute: func(ctx context.Context, peerKey giznet.PublicKey) (*rpcpb.PeerAssignment, error) {
			return resolvePeerAssignment(ctx, routeConn, peerKey)
		},
	}

	first, releaseFirst, err := gateway.acquirePeerUpstream(t.Context(), peer)
	if err != nil {
		t.Fatalf("first unassigned route: %v", err)
	}
	if first != entryA {
		t.Fatalf("first unassigned route = %p, want ordered bootstrap %p", first, entryA)
	}
	releaseFirst()

	routeConn.assignment = &rpcpb.PeerAssignment{ServerPublicKey: serverB.String()}
	owner, releaseOwner, err := gateway.acquirePeerUpstream(t.Context(), peer)
	if err != nil {
		t.Fatalf("fresh fixed-owner route: %v", err)
	}
	defer releaseOwner()
	if owner != entryB {
		t.Fatalf("fresh fixed-owner route = %p, want Server B entry %p", owner, entryB)
	}
	if calls := routeConn.calls.Load(); calls != 2 {
		t.Fatalf("route lookup calls = %d, want initial lookup plus one fresh lookup", calls)
	}
	if entryA.active != 0 || entryB.active != 1 {
		t.Fatalf("active sessions A=%d B=%d, want released loser and pinned owner", entryA.active, entryB.active)
	}
}

func TestOrderedRouteLookupBootstrapsOnReachableResponder(t *testing.T) {
	serverA := testKeyPair(t, 0x96).Public
	serverB := testKeyPair(t, 0x97).Public
	peer := testKeyPair(t, 0x98).Public
	stopped, stop := context.WithCancel(context.Background())
	stop()
	resolver := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{
			cfg:       Config{selectedUpstream: UpstreamConfig{PublicKey: serverA}},
			ctx:       stopped,
			conn:      &failingGiznetConn{dialErr: errors.New("Server A unavailable"), state: giznet.PeerStateOffline},
			connEpoch: 1,
		},
		{
			cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: serverB}},
			conn: &routeRPCGiznetConn{
				failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
			},
			connEpoch: 1,
		},
	}}
	poolA, _ := routeTestPool(t, &failingGiznetConn{state: giznet.PeerStateEstablished})
	poolB, entryB := routeTestPool(t, &failingGiznetConn{state: giznet.PeerStateEstablished})
	gateway := &Gateway{
		cfg: Config{Upstreams: []UpstreamConfig{{PublicKey: serverA}, {PublicKey: serverB}}},
		pools: map[giznet.PublicKey]*gatewayPool{
			serverA: poolA,
			serverB: poolB,
		},
		poolOrder:        []*gatewayPool{poolA, poolB},
		resolvePeerRoute: resolver.resolvePeerAssignment,
	}

	got, release, err := gateway.acquirePeerUpstream(t.Context(), peer)
	if err != nil {
		t.Fatalf("acquirePeerUpstream error = %v", err)
	}
	defer release()
	if got != entryB {
		t.Fatalf("unassigned route = %p, want reachable Server B entry %p", got, entryB)
	}
}

func TestResolveAPIKeyAssignmentUsesEdgeRPC(t *testing.T) {
	assignment := &rpcpb.PeerAssignment{PeerPublicKey: testKeyPair(t, 0xa1).Public.String(), ServerPublicKey: testKeyPair(t, 0xa2).Public.String()}
	conn := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		apiAssignment:     assignment,
	}
	got, err := resolveAPIKeyAssignment(t.Context(), conn, "secret-for-routing")
	if err != nil {
		t.Fatal(err)
	}
	forwarded, _ := conn.apiKey.Load().(string)
	if got.GetServerPublicKey() != assignment.GetServerPublicKey() || forwarded != "secret-for-routing" {
		t.Fatalf("assignment = %+v, credential forwarded = %t", got, forwarded == "secret-for-routing")
	}
}

func TestResolveAPIKeyAssignmentMapsAuthorizationAndOwnerErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		code rpcapi.StatusCode
		want error
	}{
		{name: "invalid key", code: rpcapi.StatusCodePermissionDenied, want: errAPIKeyUnauthorized},
		{name: "owner unavailable", code: rpcapi.StatusCodeNotFound, want: errAPIKeyOwnerUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn := &routeRPCGiznetConn{
				failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
				apiErrorCode:      test.code,
			}
			if _, err := resolveAPIKeyAssignment(t.Context(), conn, "secret"); !errors.Is(err, test.want) {
				t.Fatalf("resolveAPIKeyAssignment() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestOrderedTransportRoutesAPIRequestToAssignedServer(t *testing.T) {
	serverA := testKeyPair(t, 0xa3).Public
	serverB := testKeyPair(t, 0xa4).Public
	peer := testKeyPair(t, 0xa5).Public
	resolver := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		apiAssignment:     &rpcpb.PeerAssignment{PeerPublicKey: peer.String(), ServerPublicKey: serverB.String()},
	}
	target := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		httpServerID:      "server-b",
	}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: serverA}}, conn: resolver, connEpoch: 1},
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: serverB}}, conn: target, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw/openai/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer routed-key")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Test-Server") != "server-b" || target.httpCalls.Load() != 1 {
		t.Fatalf("response Server = %q, target calls = %d", response.Header.Get("X-Test-Server"), target.httpCalls.Load())
	}
	if resolver.httpCalls.Load() != 0 {
		t.Fatal("API request was forwarded to the route resolver instead of the assigned Server")
	}
}

func TestOrderedTransportFailsClosedForUnconfiguredAPIKeyServer(t *testing.T) {
	serverA := testKeyPair(t, 0xa6).Public
	serverB := testKeyPair(t, 0xa7).Public
	resolver := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		apiAssignment:     &rpcpb.PeerAssignment{ServerPublicKey: serverB.String()},
	}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: serverA}}, conn: resolver, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer routed-key")
	if _, err := transport.RoundTrip(request); !errors.Is(err, errAPIKeyTargetUnconfigured) {
		t.Fatalf("RoundTrip() error = %v, want unconfigured target", err)
	}
}

func TestOrderedTransportReportsUnavailableAPIKeyResolver(t *testing.T) {
	stopped, stop := context.WithCancel(context.Background())
	stop()
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{{
		ctx:       stopped,
		conn:      &failingGiznetConn{dialErr: errors.New("resolver unavailable"), state: giznet.PeerStateOffline},
		connEpoch: 1,
	}}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw/openai/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer routed-key")
	if _, err := transport.RoundTrip(request); !errors.Is(err, errAPIKeyTargetUnavailable) {
		t.Fatalf("RoundTrip() error = %v, want unavailable API routing Server", err)
	}
}

func routeTestPool(t *testing.T, conn giznet.Conn) (*gatewayPool, *gatewayUpstream) {
	t.Helper()
	pool := &gatewayPool{
		ctx: t.Context(),
		cfg: Config{Gateway: GatewayConfig{
			MaxUpstreams:        1,
			SessionsPerUpstream: 8,
			ChannelsPerUpstream: 64,
		}},
	}
	entry := &gatewayUpstream{pool: pool, conn: conn}
	pool.entries = []*gatewayUpstream{entry}
	return pool, entry
}

type routeRPCGiznetConn struct {
	*failingGiznetConn
	assignment    *rpcpb.PeerAssignment
	apiAssignment *rpcpb.PeerAssignment
	apiErrorCode  rpcapi.StatusCode
	apiKey        atomic.Value
	calls         atomic.Int32
	httpServerID  string
	httpCalls     atomic.Int32
}

func (c *routeRPCGiznetConn) DialContext(_ context.Context, service uint64) (net.Conn, error) {
	return c.Dial(service)
}

func (c *routeRPCGiznetConn) Dial(service uint64) (net.Conn, error) {
	if service == gizclaw.ServiceEdgeHTTP {
		client, server := net.Pipe()
		c.httpCalls.Add(1)
		go func() {
			defer server.Close()
			request, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				return
			}
			_ = request.Body.Close()
			response := &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     http.Header{"X-Test-Server": []string{c.httpServerID}},
				Body:       http.NoBody,
			}
			_ = response.Write(server)
		}()
		return client, nil
	}
	if service != gizclaw.ServiceEdgeRPC {
		return nil, c.dialErr
	}
	c.calls.Add(1)
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		request, err := rpcapi.ReadRequest(server)
		if err != nil {
			return
		}
		if err := rpcapi.ReadEOS(server); err != nil {
			return
		}
		if request.Method == rpcapi.RPCMethodServerAPIKeyResolve {
			params, decodeErr := request.Params.AsServerAPIKeyResolveRequest()
			if decodeErr != nil {
				return
			}
			c.apiKey.Store(params.ApiKey)
			if c.apiErrorCode != 0 {
				_ = rpcapi.WriteResponseForMethod(server, request.Method, (rpcapi.Error{RequestID: request.Id, Code: c.apiErrorCode, Message: "rejected"}).RPCResponse())
				_ = rpcapi.WriteEOS(server)
				return
			}
			var result rpcapi.RPCPayload
			if err := result.FromServerAPIKeyResolveResponse(rpcpb.ServerAPIKeyResolveResponse{Assignment: c.apiAssignment}); err != nil {
				return
			}
			_ = rpcapi.WriteResponseForMethod(server, request.Method, &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: request.Id, Result: &result})
			_ = rpcapi.WriteEOS(server)
			return
		}
		if c.assignment == nil {
			_ = rpcapi.WriteResponseForMethod(server, rpcapi.RPCMethodServerRouteResolve, (rpcapi.Error{
				RequestID: request.Id,
				Code:      rpcapi.StatusCodeNotFound,
				Message:   "assignment not found",
			}).RPCResponse())
			_ = rpcapi.WriteEOS(server)
			return
		}
		var result rpcapi.RPCPayload
		if err := result.FromServerRouteResolveResponse(rpcpb.ServerRouteResolveResponse{Assignment: c.assignment}); err != nil {
			return
		}
		_ = rpcapi.WriteResponseForMethod(server, rpcapi.RPCMethodServerRouteResolve, &rpcapi.RPCResponse{
			V: rpcapi.RPCVersionV1, Id: request.Id, Result: &result,
		})
		_ = rpcapi.WriteEOS(server)
	}()
	return client, nil
}

func TestOrderedTransportRoutesDebugRequestToAssignedServer(t *testing.T) {
	serverA := testKeyPair(t, 0xa3).Public
	serverB := testKeyPair(t, 0xa4).Public
	peer := testKeyPair(t, 0xa5).Public
	resolver := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		assignment:        &rpcpb.PeerAssignment{PeerPublicKey: peer.String(), ServerPublicKey: serverB.String()},
	}
	target := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		httpServerID:      "server-b",
	}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: serverA}}, conn: resolver, connEpoch: 1},
		{cfg: Config{selectedUpstream: UpstreamConfig{PublicKey: serverB}}, conn: target, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw/gizclaw/v1/device", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer gizclaw_pk_"+peer.String())
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Test-Server") != "server-b" || target.httpCalls.Load() != 1 {
		t.Fatalf("response Server = %q, target calls = %d", response.Header.Get("X-Test-Server"), target.httpCalls.Load())
	}
	if resolver.httpCalls.Load() != 0 {
		t.Fatal("API request was forwarded to the route resolver instead of the assigned Server")
	}
}

func TestDebugProxyErrorsAreNotCacheable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{errInvalidDebugPublicKey, 400}, {errAPIKeyOwnerUnavailable, 403},
		{errAPIKeyTargetUnavailable, 503}, {errors.New("transport failed"), 502},
	} {
		req := httptest.NewRequest("GET", "/gizclaw/v1/device", nil)
		req.Header.Set("Authorization", "Bearer gizclaw_pk_invalid")
		res := httptest.NewRecorder()
		writeEdgeProxyError(res, req, tc.err)
		if res.Code != tc.want || res.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d headers=%v", res.Code, res.Header())
		}
	}
}
