package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/webrtc/v4"
)

func TestPublicTCPMuxRoutesHTTPAndPreservesPrefix(t *testing.T) {
	mux := newTestPublicTCPMux(t)
	defer mux.Close()

	conn, err := net.Dial("tcp", mux.parent.Addr().String())
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer conn.Close()
	payload := "GET /server-info HTTP/1.1\r\nHost: test\r\n\r\n"
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("client Write error = %v", err)
	}

	accepted := acceptFromListener(t, mux.HTTPListener())
	defer accepted.Close()
	got := readExact(t, accepted, len(payload))
	if got != payload {
		t.Fatalf("HTTP payload = %q, want %q", got, payload)
	}
	assertNoConn(t, mux.ICETCPListener())
}

func TestPublicTCPMuxRoutesHTTP2CleartextPreface(t *testing.T) {
	mux := newTestPublicTCPMux(t)
	defer mux.Close()

	conn, err := net.Dial("tcp", mux.parent.Addr().String())
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(http2CleartextPreface)); err != nil {
		t.Fatalf("client Write error = %v", err)
	}

	accepted := acceptFromListener(t, mux.HTTPListener())
	defer accepted.Close()
	if got := readExact(t, accepted, len(http2CleartextPreface)); got != http2CleartextPreface {
		t.Fatalf("HTTP/2 preface = %q, want %q", got, http2CleartextPreface)
	}
}

func TestPublicTCPMuxRoutesNonHTTPToICEAndPreservesPrefix(t *testing.T) {
	mux := newTestPublicTCPMux(t)
	defer mux.Close()

	conn, err := net.Dial("tcp", mux.parent.Addr().String())
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer conn.Close()
	payload := []byte{0x00, 0x14, 0x00, 0x01, 0x00}
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("client Write error = %v", err)
	}

	accepted := acceptFromListener(t, mux.ICETCPListener())
	defer accepted.Close()
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(accepted, got); err != nil {
		t.Fatalf("ICE ReadFull error = %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("ICE payload = %v, want %v", got, payload)
	}
	assertNoConn(t, mux.HTTPListener())
}

func TestClassifyPublicTCPConnTimesOutWhileAmbiguous(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	errCh := make(chan error, 1)
	go func() {
		_, _, err := classifyPublicTCPConn(server, time.Now().Add(50*time.Millisecond))
		errCh <- err
	}()
	if _, err := client.Write([]byte("G")); err != nil {
		t.Fatalf("client Write error = %v", err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("classifyPublicTCPConn error = nil")
		}
	case <-time.After(time.Second):
		t.Fatal("classifyPublicTCPConn did not time out")
	}
}

func TestPublicTCPMuxCloseUnblocksChildAccept(t *testing.T) {
	mux := newTestPublicTCPMux(t)
	accepted := make(chan error, 1)
	go func() {
		_, err := mux.HTTPListener().Accept()
		accepted <- err
	}()
	if err := mux.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Close error = %v", err)
	}
	select {
	case err := <-accepted:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept error = %v, want net.ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Accept did not unblock")
	}
}

func TestPublicTCPMuxServesSignalingAndTCPICEOnSameEndpoint(t *testing.T) {
	mux := newTestPublicTCPMux(t)
	defer mux.Close()

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&gizwebrtc.ListenConfig{
		ICETCPListener:   mux.ICETCPListener(),
		PublicICETCPAddr: mux.parent.Addr().String(),
		CipherMode:       gizwebrtc.CipherModePlaintext,
		SecurityPolicy:   testSecurityPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("gizwebrtc Listen error = %v", err)
	}
	defer serverListener.Close()

	httpServer := &http.Server{Handler: serverListener.SignalingHandler()}
	httpErr := make(chan error, 1)
	go func() {
		err := httpServer.Serve(mux.HTTPListener())
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		httpErr <- err
	}()
	defer func() {
		_ = httpServer.Shutdown(context.Background())
		if err := <-httpErr; err != nil {
			t.Fatalf("HTTP Serve error = %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clientListener, clientConn, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, gizwebrtc.DialConfig{
		API:            newMuxTCPOnlyClientAPI(),
		SignalingURL:   "http://" + mux.parent.Addr().String() + gizwebrtc.SignalingPath,
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: testSecurityPolicy{},
	})
	if err != nil {
		t.Fatalf("gizwebrtc Dial error = %v", err)
	}
	defer clientListener.Close()
	defer clientConn.Close()

	serverConn := acceptGiznetConn(t, serverListener)
	defer serverConn.Close()
	if _, err := clientConn.Write(0x42, []byte("shared tcp endpoint")); err != nil {
		t.Fatalf("client Write error = %v", err)
	}
	buf := make([]byte, 64)
	proto, n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("server Read error = %v", err)
	}
	if proto != 0x42 || string(buf[:n]) != "shared tcp endpoint" {
		t.Fatalf("server packet proto=%d payload=%q", proto, string(buf[:n]))
	}
}

func newTestPublicTCPMux(t *testing.T) *publicTCPMux {
	t.Helper()
	parent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	return newPublicTCPMux(parent)
}

func acceptGiznetConn(t *testing.T, l giznet.Listener) giznet.Conn {
	t.Helper()
	ch := make(chan giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			errCh <- err
			return
		}
		ch <- conn
	}()
	select {
	case conn := <-ch:
		return conn
	case err := <-errCh:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timeout")
	}
	return nil
}

func newMuxTCPOnlyClientAPI() *webrtc.API {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	settingEngine.SetIncludeLoopbackCandidate(true)
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	return webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
}

func acceptFromListener(t *testing.T, l net.Listener) net.Conn {
	t.Helper()
	ch := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			errCh <- err
			return
		}
		ch <- conn
	}()
	select {
	case conn := <-ch:
		return conn
	case err := <-errCh:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("Accept timeout")
	}
	return nil
}

func readExact(t *testing.T, conn net.Conn, size int) string {
	t.Helper()
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("ReadFull error = %v", err)
	}
	return string(buf)
}

func assertNoConn(t *testing.T, l net.Listener) {
	t.Helper()
	ch := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			errCh <- err
			return
		}
		ch <- conn
	}()
	select {
	case conn := <-ch:
		_ = conn.Close()
		t.Fatal("unexpected accepted connection")
	case err := <-errCh:
		t.Fatalf("unexpected Accept error = %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}
