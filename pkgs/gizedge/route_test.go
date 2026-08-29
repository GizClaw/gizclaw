package gizedge

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestGatewayClientSecurityPolicyAllowsRouteRPC(t *testing.T) {
	if !((gatewayClientSecurityPolicy{}).AllowService(giznet.PublicKey{}, gizclaw.ServiceEdgeRPC)) {
		t.Fatal("gateway upstream policy rejects the route RPC service")
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
			cfg:       Config{Upstream: UpstreamConfig{PublicKey: serverA}},
			ctx:       stopped,
			conn:      &failingGiznetConn{dialErr: errors.New("Server A unavailable"), state: giznet.PeerStateOffline},
			connEpoch: 1,
		},
		{
			cfg: Config{Upstream: UpstreamConfig{PublicKey: serverB}},
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
	assignment *rpcpb.PeerAssignment
	calls      atomic.Int32
}

func (c *routeRPCGiznetConn) Dial(service uint64) (net.Conn, error) {
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
		if c.assignment == nil {
			_ = rpcapi.WriteResponseForMethod(server, rpcapi.RPCMethodServerRouteResolve, (rpcapi.Error{
				RequestID: request.Id,
				Code:      rpcapi.RPCErrorCodeNotFound,
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
