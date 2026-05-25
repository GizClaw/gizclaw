package gizclaw

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
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
		client := &Client{KeyPair: keyPair, listener: &giznet.Listener{}}
		if err := client.Dial(giznet.PublicKey{}, "127.0.0.1:1"); err == nil || !strings.Contains(err.Error(), "already started") {
			t.Fatalf("Dial(already started) err = %v", err)
		}
	})
}

func TestClientProxyHandlerValidation(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *Client
		server := httptest.NewServer(client.ProxyHandler())
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/admin/peers")
		if err != nil {
			t.Fatalf("GET /api/admin/peers error = %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("GET /api/admin/peers status = %d body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("disconnected client", func(t *testing.T) {
		client := &Client{}
		server := httptest.NewServer(client.ProxyHandler())
		defer server.Close()

		resp, err := http.Get(server.URL + "/api/public/server-info")
		if err != nil {
			t.Fatalf("GET /api/public/server-info error = %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("GET /api/public/server-info status = %d body=%s", resp.StatusCode, string(body))
		}
	})
}

func TestClientProxyMuxRoutesRemoteServices(t *testing.T) {
	client, serverConn, cleanup := newProxyTestPair(t)
	defer cleanup()

	gearServer := &peer.Server{
		Store:           mustBadgerInMemory(t, nil),
		BuildCommit:     "test-build",
		ServerPublicKey: giznet.PublicKey{1},
	}
	manager := NewManager(gearServer)
	service := &PeerService{
		manager: manager,
		admin: &adminService{
			PeerAdminService: gearServer,
		},
		gear: gearServer,
		public: &serverPublic{
			ServerPublicService: gearServer,
		},
	}

	go func() { _ = service.serveAdmin(serverConn) }()
	go func() { _ = service.serveGear(serverConn) }()
	go func() { _ = service.servePublic(serverConn) }()

	proxy := httptest.NewServer(client.ProxyHandler())
	defer proxy.Close()

	resp, body := mustProxyGET(t, proxy.URL+"/api/admin/peers")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/admin/peers status = %d body=%s", resp.StatusCode, string(body))
	}

	resp, body = mustProxyGET(t, proxy.URL+"/api/public/server-info")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/public/server-info status = %d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), gearServer.ServerPublicKey.String()) {
		t.Fatalf("GET /api/public/server-info body = %s", string(body))
	}

	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var err error
	resp, err = noRedirect.Get(proxy.URL + "/api/admin")
	if err != nil {
		t.Fatalf("GET /api/admin error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("GET /api/admin status = %d", resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); location != "/api/admin/" {
		t.Fatalf("GET /api/admin location = %q", location)
	}

	resp, err = noRedirect.Get(proxy.URL + "/api")
	if err != nil {
		t.Fatalf("GET /api error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("GET /api status = %d", resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); location != "/api/" {
		t.Fatalf("GET /api location = %q", location)
	}
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

	if rpcClient := client.rpcClient(); rpcClient == nil || rpcClient.peer != client {
		t.Fatalf("rpcClient() = %+v, want peer client bound", rpcClient)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := client.Ping(ctx, "ping"); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("Ping() err = %v", err)
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
			Imeis: &[]apitypes.GearIMEI{{
				Name:   &name,
				Tac:    "12345678",
				Serial: "0000001",
			}},
			Labels: &[]apitypes.GearLabel{{
				Key:   "batch",
				Value: "cn-east",
			}},
		},
	}

	info := gearDeviceToPeerRefreshInfo(device)
	if info.Manufacturer == nil || *info.Manufacturer != "Acme" {
		t.Fatalf("gearDeviceToPeerRefreshInfo() = %+v", info)
	}

	identifiers := gearDeviceToPeerRefreshIdentifiers(device)
	if identifiers.Sn == nil || *identifiers.Sn != "sn-1" {
		t.Fatalf("gearDeviceToPeerRefreshIdentifiers().Sn = %+v", identifiers.Sn)
	}
	if identifiers.Imeis == nil || len(*identifiers.Imeis) != 1 || (*identifiers.Imeis)[0].Tac != "12345678" {
		t.Fatalf("gearDeviceToPeerRefreshIdentifiers().Imeis = %+v", identifiers.Imeis)
	}
	if identifiers.Labels == nil || len(*identifiers.Labels) != 1 || (*identifiers.Labels)[0].Value != "cn-east" {
		t.Fatalf("gearDeviceToPeerRefreshIdentifiers().Labels = %+v", identifiers.Labels)
	}

	imei := gearToPeerGearIMEI(apitypes.GearIMEI{Name: &name, Tac: "87654321", Serial: "0000009"})
	if imei.Name == nil || *imei.Name != "main" || imei.Tac != "87654321" || imei.Serial != "0000009" {
		t.Fatalf("gearToPeerGearIMEI() = %+v", imei)
	}

	label := gearToPeerGearLabel(apitypes.GearLabel{Key: "batch", Value: "cn-west"})
	if label.Key != "batch" || label.Value != "cn-west" {
		t.Fatalf("gearToPeerGearLabel() = %+v", label)
	}
}

func TestClientRPCHandle(t *testing.T) {
	t.Run("dispatch missing params", func(t *testing.T) {
		client := &rpcClient{}
		resp, err := client.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "missing",
			Method: rpcapi.RPCMethodPeerPing,
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
			Method: rpcapi.RPCMethodPeerPing,
			Params: params,
		}); err != nil {
			t.Fatalf("WriteRequest() error = %v", err)
		}

		resp, err := rpcapi.ReadResponse(clientSide)
		if err != nil {
			t.Fatalf("ReadResponse() error = %v", err)
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
}

func newProxyTestPair(t *testing.T) (*Client, *giznet.Conn, func()) {
	t.Helper()

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	serverListener, err := (&giznet.ListenConfig{
		Addr: "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				switch service {
				case ServiceAdmin, ServiceGear, ServiceServerPublic:
					return true
				default:
					return false
				}
			},
		},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("giznet.Listen(server) error = %v", err)
	}
	go drainUDP(serverListener.UDP())

	clientListener, err := (&giznet.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{},
	}).Listen(clientKey)
	if err != nil {
		_ = serverListener.Close()
		t.Fatalf("giznet.Listen(client) error = %v", err)
	}
	go drainUDP(clientListener.UDP())

	connCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, acceptErr := serverListener.Accept()
		if acceptErr != nil {
			errCh <- acceptErr
			return
		}
		connCh <- conn
	}()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		_ = clientListener.Close()
		_ = serverListener.Close()
		t.Fatalf("Dial error = %v", err)
	}

	var serverConn *giznet.Conn
	select {
	case serverConn = <-connCh:
	case acceptErr := <-errCh:
		_ = clientConn.Close()
		_ = clientListener.Close()
		_ = serverListener.Close()
		t.Fatalf("Accept error = %v", acceptErr)
	}

	client := &Client{conn: clientConn}
	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = clientListener.Close()
		_ = serverListener.Close()
	}
	return client, serverConn, cleanup
}

func mustProxyGET(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()

	var lastErr error
	var lastStatus int
	var lastBody []byte
	for range 50 {
		resp, err := http.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			resp.Body = io.NopCloser(strings.NewReader(string(body)))
			return resp, body
		}
		lastStatus = resp.StatusCode
		lastBody = body
		time.Sleep(20 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("GET %s error = %v", url, lastErr)
	}
	t.Fatalf("GET %s status = %d body=%s", url, lastStatus, string(lastBody))
	return nil, nil
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
