//go:build gizclaw_e2e

package rpc_test

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/internal/serverrpc"
)

func TestGoSDKServerInitiatedAllRPC(t *testing.T) {
	server, err := serverrpc.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client := &gizcli.Client{
		KeyPair: clientKey,
		DialTransport: func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
				SignalingURL:   "http://" + serverAddr + gizwebrtc.SignalingPath,
				SecurityPolicy: policy,
			})
		},
	}
	acceptCtx, cancelAccept := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelAccept()
	accepted := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := server.Accept(acceptCtx)
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()
	if err := client.Dial(server.PublicKey, server.Endpoint); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	serveDone := make(chan error, 1)
	go func() { serveDone <- client.Serve() }()

	var serverConn giznet.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		t.Fatal(err)
	case <-acceptCtx.Done():
		t.Fatal(acceptCtx.Err())
	}
	t.Cleanup(func() { _ = serverConn.Close() })

	ping, err := serverrpc.Ping(serverConn, "go-server-ping")
	if err != nil {
		t.Fatal(err)
	}
	if ping.ServerTime <= 0 {
		t.Fatalf("server_time = %d", ping.ServerTime)
	}

	tests := []struct {
		name string
		up   int64
		down int64
	}{
		{name: "zero"},
		{name: "upload-only", up: 32*1024 + 7},
		{name: "download-only", down: 32*1024 + 11},
		{name: "full-duplex", up: 64*1024 + 3, down: 64*1024 + 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uploaded, downloaded, err := serverrpc.SpeedTest(serverConn, "go-server-speed-"+tc.name, tc.up, tc.down)
			if err != nil {
				t.Fatal(err)
			}
			if uploaded != tc.up || downloaded != tc.down {
				t.Fatalf("transferred up=%d down=%d, want up=%d down=%d", uploaded, downloaded, tc.up, tc.down)
			}
		})
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client Serve did not stop")
	}
}
