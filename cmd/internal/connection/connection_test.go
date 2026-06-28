package connection

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizwebrtc"
)

type allowAllSecurityPolicy struct{}

func (allowAllSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (allowAllSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

func resetConnectHooks(t *testing.T) {
	t.Helper()
	origDialFromContext := dialFromContext
	origDialClient := dialClient
	origServeClient := serveClient
	origProbeReady := probeReady
	origTimeout := connectReadyTimeout
	origPoll := connectPollInterval
	t.Cleanup(func() {
		dialFromContext = origDialFromContext
		dialClient = origDialClient
		serveClient = origServeClient
		probeReady = origProbeReady
		connectReadyTimeout = origTimeout
		connectPollInterval = origPoll
	})
}

func testServerPublicKeyText(fill byte) string {
	kp, err := giznet.NewKeyPair(testServerPrivateKey(fill))
	if err != nil {
		panic(err)
	}
	return kp.Public.String()
}

func testServerPrivateKey(fill byte) giznet.Key {
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	return key
}

func TestDialFromContextNoActiveContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, _, _, err := DialFromContext("")
	if err == nil {
		t.Fatal("DialFromContext should fail without an active context")
	}
	if !strings.Contains(err.Error(), "no active context") {
		t.Fatalf("DialFromContext error = %v", err)
	}
}

func TestDialFromContextInvalidServerPublicKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", "127.0.0.1:9820", testServerPublicKeyText(0xab)); err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "local", "config.yaml"), []byte(`
server:
  address: 127.0.0.1:9820
  public-key: not-a-key
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	_, _, _, err = DialFromContext("local")
	if err == nil {
		t.Fatal("DialFromContext should fail on invalid server public key")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("DialFromContext error = %v", err)
	}
}

func TestDialFromContextUsesCipherMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.CreateWithOptions("local", "127.0.0.1:9820", clicontext.CreateOptions{
		ServerPublicKey: testServerPublicKeyText(0xab),
		CipherMode:      giznoise.CipherModeAES256GCM,
	}); err != nil {
		t.Fatalf("CreateWithOptions error = %v", err)
	}

	client, _, _, err := DialFromContext("local")
	if err != nil {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if client.DialTransport == nil {
		t.Fatal("DialTransport is nil")
	}
}

func TestDialFromContextUsesCurrentContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", "127.0.0.1:9820", testServerPublicKeyText(0xab)); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	client, serverPK, serverAddr, err := DialFromContext("")
	if err != nil {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if client == nil || client.KeyPair == nil {
		t.Fatalf("client = %#v, want generated key pair", client)
	}
	if serverPK.String() != testServerPublicKeyText(0xab) {
		t.Fatalf("server public key = %s", serverPK)
	}
	if serverAddr != "127.0.0.1:9820" {
		t.Fatalf("server address = %q", serverAddr)
	}
}

func TestDialFromContextUsesWebRTCTransport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey := testServerPrivateKey(0xac)
	clientKeyPair, err := giznet.NewKeyPair(clientKey)
	if err != nil {
		t.Fatalf("NewKeyPair(client) error = %v", err)
	}
	serverListener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllSecurityPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("gizwebrtc Listen error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()
	serverURL := strings.TrimPrefix(httpServer.URL, "http://")

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.CreateWithOptions("webrtc", serverURL, clicontext.CreateOptions{
		ServerPublicKey: serverKey.Public.String(),
		CipherMode:      giznoise.CipherModePlaintext,
		Transport:       "webrtc",
	}); err != nil {
		t.Fatalf("CreateWithOptions error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "webrtc", "identity.key"), clientKey[:], 0o600); err != nil {
		t.Fatalf("write identity: %v", err)
	}

	client, serverPK, serverAddr, err := DialFromContext("webrtc")
	if err != nil {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if serverPK != serverKey.Public {
		t.Fatalf("serverPK mismatch")
	}
	if serverAddr == "" {
		t.Fatal("serverAddr is empty")
	}
	if err := client.Dial(serverPK, serverAddr); err != nil {
		t.Fatalf("client Dial error = %v", err)
	}
	defer client.Close()

	accepted := make(chan giznet.Conn, 1)
	go func() {
		conn, _ := serverListener.Accept()
		accepted <- conn
	}()
	select {
	case conn := <-accepted:
		if conn == nil {
			t.Fatal("accepted nil conn")
		}
		defer conn.Close()
		if conn.PublicKey() != clientKeyPair.Public {
			t.Fatalf("accepted public key = %s want %s", conn.PublicKey(), clientKeyPair.Public)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server Accept timeout")
	}
}

func TestDialFromContextMissingNamedContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, _, _, err := DialFromContext("missing")
	if err == nil {
		t.Fatal("DialFromContext should fail for a missing named context")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("DialFromContext error = %v", err)
	}
}

func TestConnectFromContextReturnsReadyClient(t *testing.T) {
	resetConnectHooks(t)
	want := &gizcli.Client{}
	dialFromContext = func(name string) (*gizcli.Client, giznet.PublicKey, string, error) {
		if name != "local" {
			t.Fatalf("name = %q", name)
		}
		return want, giznet.PublicKey{}, "127.0.0.1:9820", nil
	}
	dialClient = func(c *gizcli.Client, _ giznet.PublicKey, addr string) error {
		if c != want {
			t.Fatal("dial received wrong client")
		}
		if addr != "127.0.0.1:9820" {
			t.Fatalf("addr = %q", addr)
		}
		return nil
	}
	serveBlock := make(chan struct{})
	t.Cleanup(func() { close(serveBlock) })
	serveClient = func(*gizcli.Client) error {
		<-serveBlock
		return nil
	}
	probeReady = func(c *gizcli.Client) error {
		if c != want {
			t.Fatal("probe received wrong client")
		}
		return nil
	}
	got, err := ConnectFromContext("local")
	if err != nil {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
	if got != want {
		t.Fatal("ConnectFromContext returned wrong client")
	}
}

func TestConnectFromContextPropagatesDialFromContextError(t *testing.T) {
	resetConnectHooks(t)
	dialFromContext = func(string) (*gizcli.Client, giznet.PublicKey, string, error) {
		return nil, giznet.PublicKey{}, "", errors.New("missing")
	}
	_, err := ConnectFromContext("local")
	if err == nil || err.Error() != "missing" {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
}

func TestConnectFromContextPropagatesDialError(t *testing.T) {
	resetConnectHooks(t)
	dialFromContext = func(string) (*gizcli.Client, giznet.PublicKey, string, error) {
		return &gizcli.Client{}, giznet.PublicKey{}, "127.0.0.1:9820", nil
	}
	dialClient = func(*gizcli.Client, giznet.PublicKey, string) error {
		return errors.New("dial failed")
	}
	_, err := ConnectFromContext("local")
	if err == nil || err.Error() != "dial failed" {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
}

func TestConnectFromContextReportsEarlyServeStop(t *testing.T) {
	resetConnectHooks(t)
	dialFromContext = func(string) (*gizcli.Client, giznet.PublicKey, string, error) {
		return &gizcli.Client{}, giznet.PublicKey{}, "127.0.0.1:9820", nil
	}
	dialClient = func(*gizcli.Client, giznet.PublicKey, string) error { return nil }
	serveClient = func(*gizcli.Client) error { return nil }
	probeReady = func(*gizcli.Client) error { return errors.New("not ready") }
	_, err := ConnectFromContext("local")
	if err == nil || !strings.Contains(err.Error(), "client stopped before ready") {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
}

func TestConnectFromContextPropagatesEarlyServeError(t *testing.T) {
	resetConnectHooks(t)
	dialFromContext = func(string) (*gizcli.Client, giznet.PublicKey, string, error) {
		return &gizcli.Client{}, giznet.PublicKey{}, "127.0.0.1:9820", nil
	}
	dialClient = func(*gizcli.Client, giznet.PublicKey, string) error { return nil }
	serveClient = func(*gizcli.Client) error { return errors.New("serve failed") }
	probeReady = func(*gizcli.Client) error { return errors.New("not ready") }
	_, err := ConnectFromContext("local")
	if err == nil || err.Error() != "serve failed" {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
}

func TestConnectFromContextTimesOut(t *testing.T) {
	resetConnectHooks(t)
	connectReadyTimeout = time.Millisecond
	connectPollInterval = time.Millisecond
	serveBlock := make(chan struct{})
	t.Cleanup(func() { close(serveBlock) })
	dialFromContext = func(string) (*gizcli.Client, giznet.PublicKey, string, error) {
		return &gizcli.Client{}, giznet.PublicKey{}, "127.0.0.1:9820", nil
	}
	dialClient = func(*gizcli.Client, giznet.PublicKey, string) error { return nil }
	serveClient = func(*gizcli.Client) error {
		<-serveBlock
		return nil
	}
	probeReady = func(*gizcli.Client) error { return errors.New("not ready") }
	_, err := ConnectFromContext("local")
	if err == nil || !strings.Contains(err.Error(), "timeout waiting for client readiness") {
		t.Fatalf("ConnectFromContext error = %v", err)
	}
}

func TestProbeServerPublicReadyNilClient(t *testing.T) {
	err := probeServerPublicReady(nil)
	if err == nil {
		t.Fatal("probeServerPublicReady should fail for nil client")
	}
	if !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("probeServerPublicReady error = %v", err)
	}
}

func TestProbeServerPublicReadyRequiresConnection(t *testing.T) {
	err := probeServerPublicReady(&gizcli.Client{})
	if err == nil {
		t.Fatal("probeServerPublicReady should fail without connection")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("probeServerPublicReady error = %v", err)
	}
}

func TestProbeServerPublicReadyConnectedClient(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&giznoise.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: allowAllSecurityPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen(server) error = %v", err)
	}
	defer serverListener.Close()

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

	client := &gizcli.Client{KeyPair: clientKey, DialTransport: func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		listener, err := (&giznoise.ListenConfig{
			Addr:           ":0",
			SecurityPolicy: securityPolicy,
		}).Listen(key)
		if err != nil {
			return nil, nil, err
		}
		udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			_ = listener.Close()
			return nil, nil, err
		}
		conn, err := listener.Dial(serverPK, udpAddr)
		if err != nil {
			_ = listener.Close()
			return nil, nil, err
		}
		return listener, conn, nil
	}}
	if err := client.Dial(serverKey.Public, serverListener.HostInfo().Addr.String()); err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer client.Close()

	var serverConn giznet.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Accept timeout")
	}
	defer serverConn.Close()

	server := gizhttp.NewServer(serverConn, gizcli.ServiceServerPublic, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server-info" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"build_commit":"test","public_key":"server","server_time":1}`))
	}))
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve()
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
		select {
		case err := <-serveErr:
			if err != nil {
				t.Fatalf("server.Serve error = %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("server.Serve did not stop")
		}
	}()

	if err := probeServerPublicReady(client); err != nil {
		t.Fatalf("probeServerPublicReady error = %v", err)
	}
}
