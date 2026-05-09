package gizclaw_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestIntegrationRPCDialAndPing(t *testing.T) {
	ts := startTestServer(t)
	client := newTestClient(t, ts)

	var serverTime time.Time
	var rtt time.Duration
	var clockDiff time.Duration
	var secondServerTime time.Time
	var pingErr error
	if err := waitUntil(testReadyTimeout, func() error {
		t1 := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ping, err := client.Ping(ctx, "ping")
		if err != nil {
			cancel()
			pingErr = err
			return err
		}
		ping2, err := client.Ping(ctx, "ping-2")
		cancel()
		if err != nil {
			pingErr = err
			return err
		}
		t4 := time.Now()
		rtt = t4.Sub(t1)
		serverTime = time.UnixMilli(ping.ServerTime)
		secondServerTime = time.UnixMilli(ping2.ServerTime)
		clientMid := t1.Add(rtt / 2)
		clockDiff = serverTime.Sub(clientMid)
		pingErr = nil
		return nil
	}); err != nil {
		t.Fatalf("Ping err=%v", pingErr)
	}
	if serverTime.IsZero() {
		t.Fatal("ServerTime is zero")
	}
	if secondServerTime.IsZero() {
		t.Fatal("second ServerTime is zero")
	}
	if secondServerTime.Before(serverTime) {
		t.Fatalf("second ServerTime %v is before first %v", secondServerTime, serverTime)
	}
	if rtt <= 0 {
		t.Fatalf("RTT=%v", rtt)
	}
	if clockDiff > time.Second || clockDiff < -time.Second {
		t.Fatalf("ClockDiff=%v (too large for localhost)", clockDiff)
	}
}

func TestIntegrationSameClientKeyReconnectsAfterClose(t *testing.T) {
	ts := startTestServer(t)
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error: %v", err)
	}

	first := &gizclaw.Client{KeyPair: keyPair}
	startTestClient(t, first, ts.server.PublicKey(), ts.addr)
	if err := first.Close(); err != nil {
		t.Fatalf("first Close error = %v", err)
	}

	second := &gizclaw.Client{KeyPair: keyPair}
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
		host := &gizclaw.GearConn{
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

func TestIntegrationRPCGearClientMethods(t *testing.T) {
	ts := startTestServer(t)
	client := newTestClient(t, ts)

	var errLast error
	if err := waitUntil(testReadyTimeout, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := client.RegisterGear(ctx, "rpc-register", rpcapi.GearRegisterRequest{
			Device: rpcapi.DeviceInfo{
				Name: strPtr("rpc-gear"),
				Sn:   strPtr("rpc-sn"),
			},
		})
		if err != nil {
			errLast = err
			return err
		}
		info, err := client.GetGearInfo(ctx, "rpc-info")
		if err != nil {
			errLast = err
			return err
		}
		if info.Name == nil || *info.Name != "rpc-gear" {
			errLast = fmt.Errorf("gear info = %+v", info)
			return errLast
		}
		if _, err := client.PutGearInfo(ctx, "rpc-put-info", rpcapi.GearPutInfoRequest{Name: strPtr("rpc-gear-2")}); err != nil {
			errLast = err
			return err
		}
		if _, err := client.GetGearConfig(ctx, "rpc-config"); err != nil {
			errLast = err
			return err
		}
		registration, err := client.GetGearRegistration(ctx, "rpc-registration")
		if err != nil {
			errLast = err
			return err
		}
		if registration.PublicKey == "" || registration.Role != rpcapi.GearRoleUnspecified {
			errLast = fmt.Errorf("gear registration = %+v", registration)
			return errLast
		}
		runtime, err := client.GetGearRuntime(ctx, "rpc-runtime")
		if err != nil {
			errLast = err
			return err
		}
		if !runtime.Online {
			errLast = fmt.Errorf("gear runtime = %+v", runtime)
			return errLast
		}
		errLast = nil
		return nil
	}); err != nil {
		t.Fatalf("gear RPC client methods err=%v", errLast)
	}
}
