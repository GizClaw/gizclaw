package gizhttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func TestRoundTrip(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	serverListener := newListenerNode(t, serverKey, gizwebrtc.ListenConfig{
		SecurityPolicy: testSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == 7
			},
		},
	})
	defer serverListener.Close()
	clientListener := newListenerNode(t, clientKey)
	defer clientListener.Close()
	clientConn, serverConn := connectListenerNodes(t, clientListener, clientKey, serverListener, serverKey)

	srv := NewServer(serverConn, 7, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		w.Header().Set("X-Test", "ok")
		_, _ = w.Write([]byte("echo:" + string(payload)))
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	go func() {
		_ = srv.Serve()
	}()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://gizclaw/echo", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := NewClient(clientConn, 7).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "echo:hello" {
		t.Fatalf("body = %q", body)
	}
	if resp.Header.Get("X-Test") != "ok" {
		t.Fatalf("X-Test header = %q", resp.Header.Get("X-Test"))
	}
}

func TestRoundTripKeepsRequestBodyOpenAfterResponseHeaders(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	serverListener := newListenerNode(t, serverKey, gizwebrtc.ListenConfig{
		SecurityPolicy: testSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				return service == 7
			},
		},
	})
	defer serverListener.Close()
	clientListener := newListenerNode(t, clientKey)
	defer clientListener.Close()
	clientConn, serverConn := connectListenerNodes(t, clientListener, clientKey, serverListener, serverKey)

	firstRead := make(chan struct{})
	srv := NewServer(serverConn, 7, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).EnableFullDuplex(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		first := make([]byte, len("first"))
		if _, err := io.ReadFull(r.Body, first); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		close(firstRead)
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		rest, err := io.ReadAll(r.Body)
		if err != nil {
			_, _ = w.Write([]byte("read rest: " + err.Error()))
			return
		}
		_, _ = w.Write([]byte("rest:" + string(rest)))
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	go func() {
		_ = srv.Serve()
	}()

	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://gizclaw/upload", pr)
	if err != nil {
		t.Fatal(err)
	}
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := NewClient(clientConn, 7).Do(req)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()
	if _, err := pw.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstRead:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not read first request chunk")
	}

	var resp *http.Response
	select {
	case resp = <-respCh:
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("client did not receive response headers")
	}
	defer resp.Body.Close()
	if _, err := pw.Write([]byte("second")); err != nil {
		t.Fatalf("write second request chunk after response headers: %v", err)
	}
	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "rest:second" {
		t.Fatalf("body = %q, want rest:second", body)
	}
}

func TestRoundTripNilConnReturnsError(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://gizclaw/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRoundTripper(nil, 7).RoundTrip(req); err == nil || !strings.Contains(err.Error(), "nil conn") {
		t.Fatalf("RoundTrip(nil conn) error = %v", err)
	}
}

func TestListenerCloseUnblocksAccept(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	serverListener := newListenerNode(t, serverKey)
	defer serverListener.Close()
	clientListener := newListenerNode(t, clientKey)
	defer clientListener.Close()
	_, serverConn := connectListenerNodes(t, clientListener, clientKey, serverListener, serverKey)

	l := NewListener(serverConn, 9)
	if l.Addr().Network() != "gizhttp" {
		t.Fatalf("Addr().Network() = %q", l.Addr().Network())
	}
	done := make(chan error, 1)
	go func() {
		_, err := l.Accept()
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if err := l.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	select {
	case err := <-done:
		if !IsClosed(err) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock after Close")
	}
}

func TestPeerCloseUnblocksAccept(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	serverListener := newListenerNode(t, serverKey)
	defer serverListener.Close()
	clientListener := newListenerNode(t, clientKey)
	defer clientListener.Close()
	clientConn, serverConn := connectListenerNodes(t, clientListener, clientKey, serverListener, serverKey)
	defer serverConn.Close()

	l := NewListener(serverConn, 11)
	done := make(chan error, 1)
	go func() {
		_, err := l.Accept()
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)

	if err := clientConn.Close(); err != nil {
		t.Fatalf("client conn close error: %v", err)
	}

	select {
	case err := <-done:
		if !IsClosed(err) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock after peer close")
	}

	if _, err := l.Accept(); !IsClosed(err) && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after peer close err = %v", err)
	}
}
