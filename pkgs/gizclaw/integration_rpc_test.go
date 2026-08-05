package gizclaw_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestIntegrationRPCDialAndPing(t *testing.T) {
	const requests = 32
	ts := startTestServer(t)
	client := newTestClient(t, ts)

	var pingErr error
	if err := waitUntil(testReadyTimeout, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var previous time.Time
		for index := range requests {
			id := fmt.Sprintf("ping-%d", index)
			started := time.Now()
			ping, err := client.Ping(ctx, id)
			if err != nil {
				pingErr = fmt.Errorf("request %d: %w", index+1, err)
				return pingErr
			}
			serverTime := time.UnixMilli(ping.ServerTime)
			if serverTime.IsZero() {
				return fmt.Errorf("request %d ServerTime is zero", index+1)
			}
			if !previous.IsZero() && serverTime.Before(previous) {
				return fmt.Errorf("request %d ServerTime %v is before %v", index+1, serverTime, previous)
			}
			rtt := time.Since(started)
			clientMid := started.Add(rtt / 2)
			clockDiff := serverTime.Sub(clientMid)
			if rtt <= 0 {
				return fmt.Errorf("request %d RTT=%v", index+1, rtt)
			}
			if clockDiff > time.Second || clockDiff < -time.Second {
				return fmt.Errorf("request %d ClockDiff=%v", index+1, clockDiff)
			}
			previous = serverTime
		}
		pingErr = nil
		return nil
	}); err != nil {
		t.Fatalf("Ping err=%v", pingErr)
	}
}

func TestIntegrationRPCDialWithConfiguredCipherMode(t *testing.T) {
	ts := startTestServerWithCipherMode(t, gizwebrtc.CipherModePlaintext)
	client := newTestClient(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := getServerInfo(ctx, client); err != nil {
		t.Fatalf("GetServerInfo with configured cipher mode error = %v", err)
	}
}

func TestIntegrationSameClientKeyReconnectsAfterClose(t *testing.T) {
	ts := startTestServer(t)
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error: %v", err)
	}

	first := &gizcli.Client{KeyPair: keyPair, DialTransport: testWebRTCDialTransport(ts.cipherMode)}
	startTestClient(t, first, ts.server.PublicKey(), ts.addr)
	if err := first.Close(); err != nil {
		t.Fatalf("first Close error = %v", err)
	}

	second := &gizcli.Client{KeyPair: keyPair, DialTransport: testWebRTCDialTransport(ts.cipherMode)}
	startTestClient(t, second, ts.server.PublicKey(), ts.addr)
	t.Cleanup(func() { _ = second.Close() })
}

func TestIntegrationRPCReversePingClient(t *testing.T) {
	ts := startTestServer(t)
	client := newTestClient(t, ts)

	var clientTime time.Time
	var secondClientTime time.Time
	var pingErr error
	if err := waitUntil(testReadyTimeout, func() error {
		manager := ts.server.Manager()
		if manager == nil {
			return fmt.Errorf("server manager not ready")
		}
		conn, ok := manager.Peer(client.KeyPair.Public)
		if !ok {
			return fmt.Errorf("active peer not ready")
		}
		host := &gizclaw.PeerConn{
			Conn:    conn,
			Service: ts.server.PeerService(),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ping, err := host.Ping(ctx, "reverse-ping")
		if err != nil {
			cancel()
			pingErr = err
			return err
		}
		ping2, err := host.Ping(ctx, "reverse-ping-2")
		cancel()
		if err != nil {
			pingErr = err
			return err
		}
		clientTime = time.UnixMilli(ping.ServerTime)
		secondClientTime = time.UnixMilli(ping2.ServerTime)
		pingErr = nil
		return nil
	}); err != nil {
		t.Fatalf("reverse Ping err=%v", pingErr)
	}
	if clientTime.IsZero() {
		t.Fatal("client ServerTime is zero")
	}
	if secondClientTime.IsZero() {
		t.Fatal("second client ServerTime is zero")
	}
	if secondClientTime.Before(clientTime) {
		t.Fatalf("second client ServerTime %v is before first %v", secondClientTime, clientTime)
	}
}

func TestIntegrationRPCPeerClientMethods(t *testing.T) {
	ts := startTestServer(t)
	client := newTestClient(t, ts)

	var errLast error
	if err := waitUntil(testReadyTimeout, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := client.PutServerInfo(ctx, "rpc-put-info-initial", rpcapi.ServerPutInfoRequest{
			Name:  strPtr("rpc-peer"),
			Emoji: strPtr("🤖"),
		}); err != nil {
			errLast = err
			return err
		}
		info, err := client.GetServerInfo(ctx, "rpc-info")
		if err != nil {
			errLast = err
			return err
		}
		if info.Name == nil || *info.Name != "rpc-peer" {
			errLast = fmt.Errorf("peer info = %+v", info)
			return errLast
		}
		if _, err := client.PutServerInfo(ctx, "rpc-put-info", rpcapi.ServerPutInfoRequest{Name: strPtr("rpc-peer-2")}); err != nil {
			errLast = err
			return err
		}
		peer, err := ts.server.Manager().Peers.LoadPeer(ctx, client.KeyPair.Public)
		if err != nil {
			errLast = err
			return err
		}
		if peer.PublicKey == "" || peer.Role != apitypes.PeerRoleClient {
			errLast = fmt.Errorf("peer = %+v", peer)
			return errLast
		}
		runtime, err := client.GetServerRuntime(ctx, "rpc-runtime")
		if err != nil {
			errLast = err
			return err
		}
		if !runtime.Online {
			errLast = fmt.Errorf("peer runtime = %+v", runtime)
			return errLast
		}
		errLast = nil
		return nil
	}); err != nil {
		t.Fatalf("peer RPC client methods err=%v", errLast)
	}
}
