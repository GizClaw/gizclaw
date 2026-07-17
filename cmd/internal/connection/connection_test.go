package connection

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
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
	origFetchServerInfo := fetchServerInfo
	origDialClient := dialClient
	origServeClient := serveClient
	origProbeReady := probeReady
	origTimeout := connectReadyTimeout
	origPoll := connectPollInterval
	origServerInfoTimeout := serverInfoAttemptTimeout
	origServerInfoRetryDelay := serverInfoRetryDelay
	t.Cleanup(func() {
		dialFromContext = origDialFromContext
		fetchServerInfo = origFetchServerInfo
		dialClient = origDialClient
		serveClient = origServeClient
		probeReady = origProbeReady
		connectReadyTimeout = origTimeout
		connectPollInterval = origPoll
		serverInfoAttemptTimeout = origServerInfoTimeout
		serverInfoRetryDelay = origServerInfoRetryDelay
	})
}

func newServerInfoHTTPServer(t *testing.T, body string) (endpoint string, close func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server-info" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	return strings.TrimPrefix(server.URL, "http://"), server.Close
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

func TestDialFromContextInvalidServerInfoPublicKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	endpoint, closeServer := newServerInfoHTTPServer(t, `{"protocol":"gizclaw-webrtc","public_key":"not-a-key","signaling_path":"/webrtc/v1/offer"}`)
	defer closeServer()
	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", endpoint); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	_, _, _, err = DialFromContext("local")
	if err == nil || !strings.Contains(err.Error(), "server-info invalid public_key") {
		t.Fatalf("DialFromContext error = %v", err)
	}
}

func TestDialFromContextMissingServerInfoPublicKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	endpoint, closeServer := newServerInfoHTTPServer(t, `{"protocol":"gizclaw-webrtc","signaling_path":"/webrtc/v1/offer"}`)
	defer closeServer()
	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", endpoint); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	_, _, _, err = DialFromContext("local")
	if err == nil || !strings.Contains(err.Error(), "server-info missing public_key") {
		t.Fatalf("DialFromContext error = %v", err)
	}
}

func TestDialFromContextRetriesTransientServerInfoTimeout(t *testing.T) {
	resetConnectHooks(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	serverInfoAttemptTimeout = 20 * time.Millisecond
	serverInfoRetryDelay = time.Millisecond

	serverKey := testServerPublicKeyText(0xab)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protocol":"gizclaw-webrtc","public_key":"` + serverKey + `"}`))
	}))
	defer server.Close()

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", strings.TrimPrefix(server.URL, "http://")); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	_, serverPK, _, err := DialFromContext("local")
	if err != nil {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if serverPK.String() != serverKey {
		t.Fatalf("server public key = %s, want %s", serverPK, serverKey)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestDialFromContextRetriesServerInfo5xx(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	serverKey := testServerPublicKeyText(0xab)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "stale upstream", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protocol":"gizclaw-webrtc","public_key":"` + serverKey + `"}`))
	}))
	defer server.Close()

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", strings.TrimPrefix(server.URL, "http://")); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	_, _, _, err = DialFromContext("local")
	if err != nil {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestDialFromContextDoesNotRetryInvalidServerInfo(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protocol":"not-gizclaw","public_key":"ignored"}`))
	}))
	defer server.Close()

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("local", strings.TrimPrefix(server.URL, "http://")); err != nil {
		t.Fatalf("Create error = %v", err)
	}

	_, _, _, err = DialFromContext("local")
	if err == nil || !strings.Contains(err.Error(), "server-info protocol") {
		t.Fatalf("DialFromContext error = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestFetchPeerHTTPInfoDefaultsSignalingPath(t *testing.T) {
	endpoint, closeServer := newServerInfoHTTPServer(t, `{"protocol":"gizclaw-webrtc","public_key":"`+testServerPublicKeyText(0xab)+`"}`)
	defer closeServer()

	info, err := fetchPeerHTTPInfo(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("fetchPeerHTTPInfo error = %v", err)
	}
	if info.SignalingURL != "http://"+endpoint+gizwebrtc.SignalingPath {
		t.Fatalf("signaling URL = %q", info.SignalingURL)
	}
}

func TestFetchPeerHTTPInfoReportsFetchFailure(t *testing.T) {
	_, err := fetchPeerHTTPInfo(context.Background(), "127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "server-info fetch") {
		t.Fatalf("fetchPeerHTTPInfo error = %v", err)
	}
}

func TestDialFromContextUsesCurrentContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	endpoint, closeServer := newServerInfoHTTPServer(t, `{"protocol":"gizclaw-webrtc","public_key":"`+testServerPublicKeyText(0xab)+`","signaling_path":"/webrtc/v1/offer"}`)
	defer closeServer()
	if err := store.Create("local", endpoint); err != nil {
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
	if serverAddr != endpoint {
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
		SecurityPolicy: allowAllSecurityPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("gizwebrtc Listen error = %v", err)
	}
	defer serverListener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/server-info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"protocol":"gizclaw-webrtc","public_key":"` + serverKey.Public.String() + `","signaling_path":"/custom/offer"}`))
	})
	mux.Handle("/custom/offer", serverListener.SignalingHandler())
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	serverURL := strings.TrimPrefix(httpServer.URL, "http://")

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	if err := store.Create("webrtc", serverURL); err != nil {
		t.Fatalf("CreateWithOptions error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "webrtc", "config.yaml"), []byte(`
identity:
  private-key: `+clientKey.String()+`
server:
  endpoint: `+serverURL+`
`), 0o600); err != nil {
		t.Fatalf("write context config: %v", err)
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

func TestProbePeerHTTPReadyNilClient(t *testing.T) {
	err := probePeerHTTPReady(nil)
	if err == nil {
		t.Fatal("probePeerHTTPReady should fail for nil client")
	}
	if !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("probePeerHTTPReady error = %v", err)
	}
}

func TestProbePeerHTTPReadyRequiresConnection(t *testing.T) {
	err := probePeerHTTPReady(&gizcli.Client{})
	if err == nil {
		t.Fatal("probePeerHTTPReady should fail without connection")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("probePeerHTTPReady error = %v", err)
	}
}

func TestProbePeerHTTPReadyConnectedClient(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&gizwebrtc.ListenConfig{
		SecurityPolicy: allowAllSecurityPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen(server) error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()
	serverAddr := strings.TrimPrefix(httpServer.URL, "http://")

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
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
			SignalingURL:   "http://" + serverAddr + gizwebrtc.SignalingPath,
			SecurityPolicy: securityPolicy,
		})
	}}
	if err := client.Dial(serverKey.Public, serverAddr); err != nil {
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

	server := gizhttp.NewServer(serverConn, gizcli.ServicePeerHTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	if err := probePeerHTTPReady(client); err != nil {
		t.Fatalf("probePeerHTTPReady error = %v", err)
	}
}
