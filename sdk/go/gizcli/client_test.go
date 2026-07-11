package gizcli

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestClientDialValidation(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *Client
		if err := client.Dial(giznet.PublicKey{}, "127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "nil client") {
			t.Fatalf("Dial(nil) err = %v", err)
		}
	})

	t.Run("nil key pair", func(t *testing.T) {
		client := &Client{}
		if err := client.Dial(giznet.PublicKey{}, "127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "nil key pair") {
			t.Fatalf("Dial(nil key pair) err = %v", err)
		}
	})

	t.Run("empty server addr", func(t *testing.T) {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair error = %v", err)
		}
		client := &Client{KeyPair: keyPair}
		if err := client.Dial(giznet.PublicKey{}, ""); err == nil || !strings.Contains(err.Error(), "empty server addr") {
			t.Fatalf("Dial(empty addr) err = %v", err)
		}
	})

	t.Run("already started", func(t *testing.T) {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair error = %v", err)
		}
		client := &Client{KeyPair: keyPair, listener: testClientListener{}}
		if err := client.Dial(giznet.PublicKey{}, "127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "already started") {
			t.Fatalf("Dial(already started) err = %v", err)
		}
	})

	t.Run("nil dial transport", func(t *testing.T) {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair error = %v", err)
		}
		client := &Client{KeyPair: keyPair}
		if err := client.Dial(giznet.PublicKey{1}, "127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "nil dial transport") {
			t.Fatalf("Dial(nil dial transport) err = %v", err)
		}
	})
}

func TestClientAccessorsAndConversions(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	client := &Client{KeyPair: keyPair, serverPK: keyPair.Public}

	if got := client.ServerPublicKey(); got != keyPair.Public {
		t.Fatalf("ServerPublicKey() = %v, want %v", got, keyPair.Public)
	}
	if got := client.HTTPClient(ServicePeerRPC).Timeout; got != defaultHTTPClientTimeout {
		t.Fatalf("HTTPClient().Timeout = %v, want %v", got, defaultHTTPClientTimeout)
	}
	if got := client.HTTPClientWithTimeout(ServiceAdminHTTP, 3*time.Minute).Timeout; got != 3*time.Minute {
		t.Fatalf("HTTPClientWithTimeout().Timeout = %v, want %v", got, 3*time.Minute)
	}

	if rpcClient := client.rpcClient(); rpcClient == nil || rpcClient.peer != client {
		t.Fatalf("rpcClient() = %+v, want peer client bound", rpcClient)
	}

	ctx := t.Context()
	if _, err := client.Ping(ctx, "ping"); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("Ping() err = %v", err)
	}
	if _, err := client.ServerRunSay(ctx, "say", rpcapi.ServerRunSayRequest{Text: "hello"}); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("ServerRunSay() err = %v", err)
	}

	name := "main"
	device := apitypes.DeviceInfo{
		Sn: func() *string {
			v := "sn-1"
			return &v
		}(),
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: func() *string { v := "Acme"; return &v }(),
			Model:        func() *string { v := "M1"; return &v }(),
			Imeis: &[]apitypes.PeerIMEI{{
				Name:   &name,
				Tac:    "12345678",
				Serial: "0000001",
			}},
			Labels: &[]apitypes.PeerLabel{{
				Key:   "batch",
				Value: "cn-east",
			}},
		},
	}

	info := peerDeviceToPeerRefreshInfo(device)
	if info.Manufacturer == nil || *info.Manufacturer != "Acme" {
		t.Fatalf("peerDeviceToPeerRefreshInfo() = %+v", info)
	}

	identifiers := peerDeviceToPeerRefreshIdentifiers(device)
	if identifiers.Sn == nil || *identifiers.Sn != "sn-1" {
		t.Fatalf("peerDeviceToPeerRefreshIdentifiers().Sn = %+v", identifiers.Sn)
	}
	if identifiers.Imeis == nil || len(*identifiers.Imeis) != 1 || (*identifiers.Imeis)[0].Tac != "12345678" {
		t.Fatalf("peerDeviceToPeerRefreshIdentifiers().Imeis = %+v", identifiers.Imeis)
	}
	if identifiers.Labels == nil || len(*identifiers.Labels) != 1 || (*identifiers.Labels)[0].Value != "cn-east" {
		t.Fatalf("peerDeviceToPeerRefreshIdentifiers().Labels = %+v", identifiers.Labels)
	}

	imei := peerToPeerPeerIMEI(apitypes.PeerIMEI{Name: &name, Tac: "87654321", Serial: "0000009"})
	if imei.Name == nil || *imei.Name != "main" || imei.Tac != "87654321" || imei.Serial != "0000009" {
		t.Fatalf("peerToPeerPeerIMEI() = %+v", imei)
	}

	label := peerToPeerPeerLabel(apitypes.PeerLabel{Key: "batch", Value: "cn-west"})
	if label.Key != "batch" || label.Value != "cn-west" {
		t.Fatalf("peerToPeerPeerLabel() = %+v", label)
	}
}

func TestClientLifecycleWithoutConnection(t *testing.T) {
	var nilClient *Client
	if err := nilClient.Serve(); err == nil || !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("nil Serve() error = %v", err)
	}
	if err := (&Client{}).Serve(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("disconnected Serve() error = %v", err)
	}

	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	client := &Client{}
	client.init(nil, nil, keyPair.Public)
	if client.ServerPublicKey() != keyPair.Public {
		t.Fatalf("ServerPublicKey() = %v, want %v", client.ServerPublicKey(), keyPair.Public)
	}
	if client.rpcClient() == nil {
		t.Fatal("rpcClient() = nil")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if client.PeerConn() != nil {
		t.Fatal("PeerConn() after Close() != nil")
	}
	if client.ServerPublicKey() != (giznet.PublicKey{}) {
		t.Fatalf("ServerPublicKey() after Close() = %v, want zero", client.ServerPublicKey())
	}

	if httpClient := client.HTTPClient(ServicePeerRPC); httpClient == nil {
		t.Fatal("HTTPClient() = nil")
	}
	if adminClient, err := client.ServerAdminClient(); err != nil || adminClient == nil {
		t.Fatalf("ServerAdminClient() = %v, %v", adminClient, err)
	}
	if publicClient, err := client.PeerHTTPClient(); err != nil || publicClient == nil {
		t.Fatalf("PeerHTTPClient() = %v, %v", publicClient, err)
	}
}

func TestClientServeClearsPeerConnWhenUnderlyingConnCloses(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, signalingURL := newTestWebRTCServer(t, serverKey, clientSecurityPolicy{})

	accepted := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client := &Client{KeyPair: clientKey, DialTransport: testWebRTCDialTransport()}
	if err := client.Dial(serverKey.Public, signalingURL); err != nil {
		t.Fatalf("Dial() error = %v", err)
	}

	select {
	case conn := <-accepted:
		defer conn.Close()
	case err := <-acceptErr:
		t.Fatalf("Accept() error = %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Accept() timeout")
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- client.Serve()
	}()
	deadline := time.After(3 * time.Second)
	for client.PeerConn() == nil {
		select {
		case <-deadline:
			t.Fatal("client PeerConn() was never set")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := client.PeerConn().Close(); err != nil {
		t.Fatalf("underlying Conn.Close() error = %v", err)
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve() did not exit after underlying Conn.Close()")
	}
	if client.PeerConn() != nil {
		t.Fatal("PeerConn() after Serve exit != nil")
	}
}

func TestClientRPCWithoutContextDeadlineUsesDefaultStreamTimeout(t *testing.T) {
	oldTimeout := defaultRPCStreamTimeout
	defaultRPCStreamTimeout = 20 * time.Millisecond
	t.Cleanup(func() { defaultRPCStreamTimeout = oldTimeout })

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, signalingURL := newTestWebRTCServer(t, serverKey, clientSecurityPolicy{})

	accepted := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client := &Client{KeyPair: clientKey, DialTransport: testWebRTCDialTransport()}
	if err := client.Dial(serverKey.Public, signalingURL); err != nil {
		t.Fatalf("Dial() error = %v", err)
	}

	select {
	case conn := <-accepted:
		defer conn.Close()
	case err := <-acceptErr:
		t.Fatalf("Accept() error = %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Accept() timeout")
	}

	start := time.Now()
	_, err = client.Ping(context.Background(), "no-deadline")
	if err == nil {
		t.Fatal("Ping() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("Ping() error = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Ping() elapsed = %s, want bounded by default RPC stream timeout", elapsed)
	}
}

func TestClientSecurityPolicyAllowsExpectedPeerAndService(t *testing.T) {
	if !(clientSecurityPolicy{}).AllowPeer(giznet.PublicKey{}) {
		t.Fatal("AllowPeer() = false, want true")
	}
	if !(clientSecurityPolicy{}).AllowService(giznet.PublicKey{}, ServicePeerRPC) {
		t.Fatal("AllowService(ServicePeerRPC) = false, want true")
	}
	if !(clientSecurityPolicy{}).AllowService(giznet.PublicKey{}, ServiceEdgeRPC) {
		t.Fatal("AllowService(ServiceEdgeRPC) = false, want true")
	}
}

func TestClientRPCHandle(t *testing.T) {
	t.Run("dispatch missing params", func(t *testing.T) {
		client := &rpcClient{}
		resp, err := client.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "missing",
			Method: rpcapi.RPCMethodAllPing,
		})
		if err != nil {
			t.Fatalf("dispatch() error = %v", err)
		}
		if resp == nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
			t.Fatalf("dispatch() response = %+v", resp)
		}
	})

	t.Run("dispatch unsupported method", func(t *testing.T) {
		client := &rpcClient{}
		resp, err := client.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "unknown",
			Method: "rpc.unknown",
		})
		if err != nil {
			t.Fatalf("dispatch() error = %v", err)
		}
		if resp == nil || resp.Error == nil || !strings.Contains(resp.Error.Message, "unsupported method") {
			t.Fatalf("dispatch() response = %+v", resp)
		}
	})

	t.Run("serve rpc stream ping", func(t *testing.T) {
		serverSide, clientSide := net.Pipe()
		defer serverSide.Close()
		defer clientSide.Close()

		errCh := make(chan error, 1)
		go func() {
			errCh <- (&rpcClient{}).Handle(serverSide)
		}()

		params, err := newRPCPingRequestParams(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()})
		if err != nil {
			t.Fatalf("newRPCPingRequestParams() error = %v", err)
		}
		if err := rpcapi.WriteRequest(clientSide, &rpcapi.RPCRequest{
			V:      rpcapi.RPCVersionV1,
			Id:     "ping",
			Method: rpcapi.RPCMethodAllPing,
			Params: params,
		}); err != nil {
			t.Fatalf("WriteRequest() error = %v", err)
		}
		if err := rpcapi.WriteEOS(clientSide); err != nil {
			t.Fatalf("WriteEOS() error = %v", err)
		}

		resp, err := rpcapi.ReadResponseForMethod(clientSide, rpcapi.RPCMethodAllPing)
		if err != nil {
			t.Fatalf("ReadResponse() error = %v", err)
		}
		if err := rpcapi.ReadEOS(clientSide); err != nil {
			t.Fatalf("ReadEOS() error = %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("Handle() response error = %+v", resp.Error)
		}
		if resp.Result == nil {
			t.Fatalf("Handle() response result = %+v", resp.Result)
		}
		result, err := resp.Result.AsPingResponse()
		if err != nil {
			t.Fatalf("Handle() response result decode error = %v", err)
		}
		if result.ServerTime <= 0 {
			t.Fatalf("Handle() response result = %+v", result)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
	})

	t.Run("serve rpc stream speed test", func(t *testing.T) {
		serverSide, clientSide := net.Pipe()
		defer serverSide.Close()
		defer clientSide.Close()

		errCh := make(chan error, 1)
		go func() {
			errCh <- (&rpcClient{}).Handle(serverSide)
		}()

		result, err := callRPCSpeedTest(context.Background(), clientSide, "server-speed", rpcapi.SpeedTestRequest{
			UpContentLength:   rpcSpeedTestFrameSize + 5,
			DownContentLength: rpcSpeedTestFrameSize + 7,
		})
		if err != nil {
			t.Fatalf("callRPCSpeedTest() error = %v", err)
		}
		if result.UpBytes != rpcSpeedTestFrameSize+5 || result.DownBytes != rpcSpeedTestFrameSize+7 {
			t.Fatalf("callRPCSpeedTest() result = %+v", result)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("Handle() error = %v", err)
		}
	})
}
