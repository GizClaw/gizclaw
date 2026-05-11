package giznet_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestNilListenerGuard(t *testing.T) {
	var l *giznet.Listener

	if _, err := l.Accept(); !errors.Is(err, giznet.ErrNilListener) {
		t.Fatalf("Accept(nil listener) err=%v, want %v", err, giznet.ErrNilListener)
	}

	if _, ok := l.Peer(giznet.PublicKey{}); ok {
		t.Fatal("Peer(nil listener) should not find a Conn")
	}

	if err := l.Close(); !errors.Is(err, giznet.ErrNilListener) {
		t.Fatalf("Close(nil listener) err=%v, want %v", err, giznet.ErrNilListener)
	}
}

func TestListenAndCloseOwnedListener(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	l, err := (&giznet.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: testSecurityPolicy{},
	}).Listen(key)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := l.Accept(); !errors.Is(err, giznet.ErrClosed) {
		t.Fatalf("Accept after Close err=%v, want %v", err, giznet.ErrClosed)
	}
}

func TestListenerPeerMissingReturnsFalse(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	unknown, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate unknown key failed: %v", err)
	}

	l, err := (&giznet.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: testSecurityPolicy{},
	}).Listen(key)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer l.Close()

	if _, ok := l.Peer(unknown.Public); ok {
		t.Fatal("Peer(unknown) should not find a Conn")
	}
}

func TestListenConfigReceivesPeerOnlineOfflineEvents(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	events := make(chan giznet.PeerEvent, 2)
	serverListener := NewTestListenerConfig(t, giznet.ListenConfig{
		PeerEventHandler: giznet.PeerEventHandleFunc(func(ev giznet.PeerEvent) {
			events <- ev
		}),
	}, serverKey)
	defer serverListener.Close()
	clientListener := NewTestListener(t, clientKey)
	defer clientListener.Close()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("client Dial failed: %v", err)
	}

	serverConn, err := AcceptConnWithTimeout(serverListener, 3*time.Second)
	if err != nil {
		t.Fatalf("server Accept failed: %v", err)
	}
	if serverConn.PublicKey() != clientKey.Public {
		t.Fatalf("server accepted key=%v, want %v", serverConn.PublicKey(), clientKey.Public)
	}
	select {
	case ev := <-events:
		if ev.PublicKey != clientKey.Public || ev.State != giznet.PeerStateEstablished {
			t.Fatalf("online event=%+v, want established for client", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("online handler event timeout")
	}

	if err := serverConn.Close(); err != nil {
		t.Fatalf("server Conn.Close failed: %v", err)
	}
	select {
	case ev := <-events:
		if ev.PublicKey != clientKey.Public || ev.State != giznet.PeerStateOffline {
			t.Fatalf("offline event=%+v, want offline for client", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("offline handler event timeout")
	}
	_ = clientConn
}

func TestListenerDoesNotAcceptSamePeerAgainOnReconnect(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := pair.ServerListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	if err := pair.ClientListener.Connect(pair.ServerKey.Public); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	select {
	case conn := <-acceptCh:
		t.Fatalf("Listener.Accept unexpectedly returned duplicate peer %v after reconnect", conn.PublicKey())
	case err := <-errCh:
		t.Fatalf("Listener.Accept failed during reconnect: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
}

// Closing the upper-level Conn is the signal that the Listener may surface a
// new Conn for the same peer. The lower Noise peer is preserved, so new service
// data can create the next service mux/Conn generation.
func TestListenerAcceptsSamePeerAgainAfterConnCloseAndNewData(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	if err := pair.ServerConn.Close(); err != nil {
		t.Fatalf("server Conn.Close failed: %v", err)
	}
	if err := pair.ClientListener.Close(); err != nil {
		t.Fatalf("client listener Close failed: %v", err)
	}
	waitForPeerOffline(t, pair.ServerListener.UDP(), pair.ClientKey.Public)

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := pair.ServerListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	reconnectedClient := NewTestListener(t, pair.ClientKey)
	defer reconnectedClient.Close()
	reconnectedConn, err := reconnectedClient.Dial(pair.ServerKey.Public, pair.ServerListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("reconnected client Dial failed: %v", err)
	}

	var newServerConn *giznet.Conn
	select {
	case newServerConn = <-acceptCh:
		if got := newServerConn.PublicKey(); got != pair.ClientKey.Public {
			t.Fatalf("accepted public key=%v, want %v", got, pair.ClientKey.Public)
		}
	case err := <-errCh:
		t.Fatalf("Listener.Accept failed after Conn.Close: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Listener.Accept timeout after Conn.Close")
	}
	rpcListener := newServerConn.ListenService(testServiceRPC)
	defer rpcListener.Close()
	rpcAcceptCh := make(chan net.Conn, 1)
	rpcErrCh := make(chan error, 1)
	go func() {
		stream, err := rpcListener.Accept()
		if err != nil {
			rpcErrCh <- err
			return
		}
		rpcAcceptCh <- stream
	}()

	clientStream, err := reconnectedConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("reconnected Dial(rpc) failed: %v", err)
	}
	defer clientStream.Close()
	req := []byte(`{"method":"after-conn-close"}`)
	if _, err := clientStream.Write(req); err != nil {
		t.Fatalf("reconnected stream write failed: %v", err)
	}

	select {
	case serverStream := <-rpcAcceptCh:
		defer serverStream.Close()
		if got := ReadExactWithTimeout(t, serverStream, len(req), 5*time.Second); !bytes.Equal(got, req) {
			t.Fatalf("server stream request mismatch: got=%q want=%q", got, req)
		}
	case err := <-rpcErrCh:
		t.Fatalf("new Conn service listener failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("new Conn service listener did not receive reconnected stream")
	}
}

func TestPeerMultipleConcurrentConnections(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}

	serverListener := NewTestListener(t, serverKey)
	defer serverListener.Close()

	const peers = 3
	type clientNode struct {
		key      *giznet.KeyPair
		listener *giznet.Listener
	}
	clients := make([]clientNode, 0, peers)
	for i := range peers {
		k, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("Generate client key %d failed: %v", i, err)
		}
		cl := NewTestListener(t, k)
		clients = append(clients, clientNode{key: k, listener: cl})
	}
	defer func() {
		for _, c := range clients {
			_ = c.listener.Close()
		}
	}()

	var connectWG sync.WaitGroup
	for _, c := range clients {
		connectWG.Add(1)
		client := c
		go func() {
			defer connectWG.Done()
			client.listener.SetPeerEndpoint(serverKey.Public, serverListener.HostInfo().Addr)
			serverListener.SetPeerEndpoint(client.key.Public, client.listener.HostInfo().Addr)
			if err := client.listener.Connect(serverKey.Public); err != nil {
				t.Errorf("Connect failed: %v", err)
			}
		}()
	}
	connectWG.Wait()

	accepted := make(map[giznet.PublicKey]struct{})
	for i := range peers {
		conn, err := AcceptConnWithTimeout(serverListener, 5*time.Second)
		if err != nil {
			t.Fatalf("Accept(%d) failed: %v", i, err)
		}
		accepted[conn.PublicKey()] = struct{}{}
	}

	if len(accepted) != peers {
		t.Fatalf("accepted peers=%d, want %d", len(accepted), peers)
	}
}

func TestNilServiceListenerGuard(t *testing.T) {
	var conn *giznet.Conn

	if _, err := conn.Dial(1); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("Dial(1, nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}

	listener := conn.ListenService(1)
	if _, err := listener.Accept(); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("ListenService(1).Accept(nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
	if err := listener.Close(); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("ListenService(1).Close(nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
}

func TestServiceListenerAcceptAndClose(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	listener := pair.ServerConn.ListenService(testServiceRPC)
	if listener.Service() != testServiceRPC {
		t.Fatalf("listener.Service()=%d, want %d", listener.Service(), testServiceRPC)
	}
	if listener.Addr().Network() != "kcp-service" {
		t.Fatalf("listener.Addr().Network()=%q", listener.Addr().Network())
	}

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		stream, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- stream
	}()

	clientStream, err := pair.ClientConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("Dial(rpc) failed: %v", err)
	}
	defer clientStream.Close()

	select {
	case stream := <-acceptCh:
		_ = stream.Close()
	case err := <-errCh:
		t.Fatalf("listener.Accept failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("listener.Accept timeout")
	}

	done := make(chan error, 1)
	go func() {
		_, err := listener.Accept()
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)

	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close error: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Accept after Close err=%v, want %v", err, net.ErrClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock after listener.Close")
	}
}

func TestNilConnGuard(t *testing.T) {
	var c *giznet.Conn

	if _, err := c.Dial(testServiceRPC); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("Dial(rpc, nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
	if _, err := c.ListenService(testServiceRPC).Accept(); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("ListenService(rpc).Accept(nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
	if _, err := c.Write(testProtocolEvent, []byte("x")); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("Write(nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
	if _, _, err := c.Read(make([]byte, 1)); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("Read(nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
	if err := c.Close(); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("Close(nil conn) err=%v, want %v", err, giznet.ErrNilConn)
	}
	if got := c.PublicKey(); got != (giznet.PublicKey{}) {
		t.Fatalf("PublicKey(nil conn) = %v, want zero key", got)
	}
	if info := c.PeerInfo(); info != nil {
		t.Fatalf("PeerInfo(nil conn) = %+v, want nil", info)
	}
}

func TestConnPeerInfoReturnsTransportSnapshot(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	info := pair.ServerConn.PeerInfo()
	if info == nil {
		t.Fatal("PeerInfo() = nil")
	}
	if info.PublicKey != pair.ClientKey.Public {
		t.Fatalf("PeerInfo().PublicKey = %v, want %v", info.PublicKey, pair.ClientKey.Public)
	}
	if info.State != giznet.PeerStateEstablished {
		t.Fatalf("PeerInfo().State = %v, want %v", info.State, giznet.PeerStateEstablished)
	}
	if info.Endpoint == nil {
		t.Fatal("PeerInfo().Endpoint = nil")
	}
	if info.LastSeen.IsZero() {
		t.Fatal("PeerInfo().LastSeen is zero")
	}
}

func TestListenerAcceptAndConnEventOpus(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	serverListener := NewTestListener(t, serverKey)
	defer serverListener.Close()
	clientListener := NewTestListener(t, clientKey)
	defer clientListener.Close()

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := serverListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- c
	}()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("clientListener.Dial failed: %v", err)
	}

	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("Listener.Accept failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Listener.Accept timeout")
	}

	evt := testEvent{V: testEventVersion, Name: "hello"}
	if err := writeTestEvent(clientConn, evt); err != nil {
		t.Fatalf("writeTestEvent failed: %v", err)
	}

	gotEvent, err := readTestEvent(serverConn)
	if err != nil {
		t.Fatalf("readTestEvent failed: %v", err)
	}
	if gotEvent.Name != evt.Name || gotEvent.V != testEventVersion {
		t.Fatalf("event mismatch: got=%+v want=%+v", gotEvent, evt)
	}
	if gotPK := serverConn.PublicKey(); gotPK != clientKey.Public {
		t.Fatalf("serverConn.PublicKey() mismatch")
	}

	wantStamp := uint64(1234567890123)
	wantRawFrame := []byte("opus-frame")
	frame := stampedopus.Pack(wantStamp, wantRawFrame)
	if err := writeTestOpusFrame(clientConn, frame); err != nil {
		t.Fatalf("writeTestOpusFrame failed: %v", err)
	}

	gotStamp, gotFrame, err := readTestOpusFrame(serverConn)
	if err != nil {
		t.Fatalf("readTestOpusFrame failed: %v", err)
	}
	if gotStamp != wantStamp {
		t.Fatalf("opus frame stamp=%d, want %d", gotStamp, wantStamp)
	}
	if !bytes.Equal(gotFrame, wantRawFrame) {
		t.Fatalf("opus frame payload mismatch: got=%q want=%q", gotFrame, wantRawFrame)
	}

	if err := clientConn.Close(); err != nil {
		t.Fatalf("clientConn.Close failed: %v", err)
	}
}

// Remote close-control tears down the current service mux, but it is not the
// same as closing the accepted Conn. Listener ownership stays with that Conn, so
// a reconnect by the same peer must not create a duplicate Conn. If Accept
// observes the reconnect event, it returns the existing Conn.
func TestListenerDoesNotAcceptSamePeerAgainAfterRemoteClose(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	serverListener := NewTestListener(t, serverKey)
	defer serverListener.Close()
	clientListener := NewTestListener(t, clientKey)

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := serverListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- c
	}()

	if _, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr); err != nil {
		t.Fatalf("clientListener.Dial failed: %v", err)
	}
	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("first Accept failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("first Accept timeout")
	}
	rpcListener := serverConn.ListenService(testServiceRPC)
	defer rpcListener.Close()

	if err := clientListener.Close(); err != nil {
		t.Fatalf("clientListener.Close failed: %v", err)
	}
	waitForPeerOffline(t, serverListener.UDP(), clientKey.Public)

	nextAcceptCh := make(chan *giznet.Conn, 1)
	nextErrCh := make(chan error, 1)
	go func() {
		c, err := serverListener.Accept()
		if err != nil {
			nextErrCh <- err
			return
		}
		nextAcceptCh <- c
	}()

	reconnectedClient := NewTestListener(t, clientKey)
	defer reconnectedClient.Close()
	reconnectedConn, err := reconnectedClient.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("reconnected client Dial failed: %v", err)
	}
	rpcErrCh := make(chan error, 1)
	go func() {
		_, err := rpcListener.Accept()
		rpcErrCh <- err
	}()

	select {
	case err := <-rpcErrCh:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("old Conn service listener err=%v, want %v", err, net.ErrClosed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old Conn service listener did not close after remote close")
	}

	select {
	case conn := <-nextAcceptCh:
		if conn != serverConn {
			t.Fatalf("Listener.Accept returned duplicate Conn %p, want existing %p", conn, serverConn)
		}
	case err := <-nextErrCh:
		t.Fatalf("Listener.Accept failed after remote close: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	_ = reconnectedConn
}

func TestConnOpenAcceptRPC(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	serverListener := NewTestListener(t, serverKey)
	defer serverListener.Close()
	clientListener := NewTestListener(t, clientKey)
	defer clientListener.Close()

	acceptConnCh := make(chan *giznet.Conn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		c, err := serverListener.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		acceptConnCh <- c
	}()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("clientListener.Dial failed: %v", err)
	}

	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptConnCh:
	case err := <-acceptErrCh:
		t.Fatalf("Listener.Accept failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Listener.Accept timeout")
	}

	rpcAcceptCh := make(chan net.Conn, 1)
	rpcErrCh := make(chan error, 1)
	rpcListener := serverConn.ListenService(testServiceRPC)
	defer rpcListener.Close()
	go func() {
		s, err := rpcListener.Accept()
		if err != nil {
			rpcErrCh <- err
			return
		}
		rpcAcceptCh <- s
	}()

	clientStream, err := clientConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("Dial(rpc) failed: %v", err)
	}
	defer clientStream.Close()

	req := []byte(`{"method":"ping"}`)
	if _, err := clientStream.Write(req); err != nil {
		t.Fatalf("client stream write req failed: %v", err)
	}

	var serverStream net.Conn
	select {
	case serverStream = <-rpcAcceptCh:
		defer serverStream.Close()
	case err := <-rpcErrCh:
		t.Fatalf("ListenService(rpc).Accept failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ListenService(rpc).Accept timeout")
	}

	if got := ReadExactWithTimeout(t, serverStream, len(req), 5*time.Second); !bytes.Equal(got, req) {
		t.Fatalf("server stream request mismatch: got=%q want=%q", got, req)
	}

	resp := []byte(`{"ok":true}`)
	if _, err := serverStream.Write(resp); err != nil {
		t.Fatalf("server stream write resp failed: %v", err)
	}
	if got := ReadExactWithTimeout(t, clientStream, len(resp), 5*time.Second); !bytes.Equal(got, resp) {
		t.Fatalf("client stream response mismatch: got=%q want=%q", got, resp)
	}

	clientRPCAcceptCh := make(chan net.Conn, 1)
	clientRPCErrCh := make(chan error, 1)
	clientRPCListener := clientConn.ListenService(testServiceRPC)
	defer clientRPCListener.Close()
	go func() {
		s, err := clientRPCListener.Accept()
		if err != nil {
			clientRPCErrCh <- err
			return
		}
		clientRPCAcceptCh <- s
	}()

	serverStream2, err := serverConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("server Dial(rpc) failed: %v", err)
	}
	defer serverStream2.Close()

	revReq := []byte(`{"method":"pong"}`)
	if _, err := serverStream2.Write(revReq); err != nil {
		t.Fatalf("server stream write reverse req failed: %v", err)
	}

	var clientStream2 net.Conn
	select {
	case clientStream2 = <-clientRPCAcceptCh:
		defer clientStream2.Close()
	case err := <-clientRPCErrCh:
		t.Fatalf("client ListenService(rpc).Accept failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("client ListenService(rpc).Accept timeout")
	}

	if got := ReadExactWithTimeout(t, clientStream2, len(revReq), 5*time.Second); !bytes.Equal(got, revReq) {
		t.Fatalf("client stream reverse request mismatch: got=%q want=%q", got, revReq)
	}

	revResp := []byte(`{"ok":false}`)
	if _, err := clientStream2.Write(revResp); err != nil {
		t.Fatalf("client stream write reverse resp failed: %v", err)
	}
	if got := ReadExactWithTimeout(t, serverStream2, len(revResp), 5*time.Second); !bytes.Equal(got, revResp) {
		t.Fatalf("server stream reverse response mismatch: got=%q want=%q", got, revResp)
	}
}

// Noise rekey replaces the transport session but must not close the service
// mux or KCP streams above it.
func TestKCPStreamSurvivesNoiseRekey(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	rpcListener := pair.ServerConn.ListenService(testServiceRPC)
	defer rpcListener.Close()

	rpcAcceptCh := make(chan net.Conn, 1)
	rpcErrCh := make(chan error, 1)
	go func() {
		stream, err := rpcListener.Accept()
		if err != nil {
			rpcErrCh <- err
			return
		}
		rpcAcceptCh <- stream
	}()

	clientStream, err := pair.ClientConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("Dial(rpc) failed: %v", err)
	}
	defer clientStream.Close()

	beforeRekey := []byte(`{"method":"before-rekey"}`)
	if _, err := clientStream.Write(beforeRekey); err != nil {
		t.Fatalf("client stream write before rekey failed: %v", err)
	}

	var serverStream net.Conn
	select {
	case serverStream = <-rpcAcceptCh:
		defer serverStream.Close()
	case err := <-rpcErrCh:
		t.Fatalf("ListenService(rpc).Accept failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ListenService(rpc).Accept timeout")
	}
	if got := ReadExactWithTimeout(t, serverStream, len(beforeRekey), 5*time.Second); !bytes.Equal(got, beforeRekey) {
		t.Fatalf("server stream request before rekey mismatch: got=%q want=%q", got, beforeRekey)
	}

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := pair.ServerListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	if err := pair.ClientListener.Connect(pair.ServerKey.Public); err != nil {
		t.Fatalf("Noise rekey Connect failed: %v", err)
	}

	afterRekey := []byte(`{"method":"after-rekey"}`)
	if _, err := clientStream.Write(afterRekey); err != nil {
		t.Fatalf("client stream write after rekey failed: %v", err)
	}
	if got := ReadExactWithTimeout(t, serverStream, len(afterRekey), 5*time.Second); !bytes.Equal(got, afterRekey) {
		t.Fatalf("server stream request after rekey mismatch: got=%q want=%q", got, afterRekey)
	}

	resp := []byte(`{"ok":"after-rekey"}`)
	if _, err := serverStream.Write(resp); err != nil {
		t.Fatalf("server stream write after rekey failed: %v", err)
	}
	if got := ReadExactWithTimeout(t, clientStream, len(resp), 5*time.Second); !bytes.Equal(got, resp) {
		t.Fatalf("client stream response after rekey mismatch: got=%q want=%q", got, resp)
	}

	select {
	case conn := <-acceptCh:
		t.Fatalf("Listener.Accept unexpectedly returned duplicate peer %v after rekey", conn.PublicKey())
	case err := <-errCh:
		t.Fatalf("Listener.Accept failed during rekey: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestConnValidationAndPerProtocolReads(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	if err := (testEvent{V: testEventVersion, Name: "   "}).Validate(); !errors.Is(err, errTestEventMissingName) {
		t.Fatalf("Event.Validate(blank name) err=%v, want %v", err, errTestEventMissingName)
	}

	if err := writeTestEvent(pair.ClientConn, testEvent{V: testEventVersion, Name: "event-before-opus"}); err != nil {
		t.Fatalf("writeTestEvent failed: %v", err)
	}
	if err := writeTestOpusFrame(pair.ClientConn, stampedopus.Pack(100, []byte{0xF8})); err != nil {
		t.Fatalf("writeTestOpusFrame(valid) failed: %v", err)
	}

	firstProto, firstPayload, err := readPacketWithTimeout(pair.ServerConn, 5*time.Second)
	if err != nil {
		t.Fatalf("read first packet err=%v", err)
	}

	secondProto, secondPayload, err := readPacketWithTimeout(pair.ServerConn, 5*time.Second)
	if err != nil {
		t.Fatalf("read second packet err=%v", err)
	}
	seenEvent := false
	seenOpus := false
	for _, pkt := range []struct {
		proto   byte
		payload []byte
	}{
		{proto: firstProto, payload: firstPayload},
		{proto: secondProto, payload: secondPayload},
	} {
		switch pkt.proto {
		case testProtocolEvent:
			gotEvent, err := decodeTestEvent(pkt.payload)
			if err != nil {
				t.Fatalf("decode event err=%v", err)
			}
			if gotEvent.Name != "event-before-opus" {
				t.Fatalf("event name=%q, want %q", gotEvent.Name, "event-before-opus")
			}
			seenEvent = true
		case testProtocolOpus:
			gotStamp, gotFrame, ok := stampedopus.Unpack(pkt.payload)
			if !ok {
				t.Fatal("stampedopus.Unpack failed")
			}
			if gotStamp != 100 {
				t.Fatalf("opus frame stamp=%d, want 100", gotStamp)
			}
			if !bytes.Equal(gotFrame, []byte{0xF8}) {
				t.Fatalf("opus frame payload=%v, want %v", gotFrame, []byte{0xF8})
			}
			seenOpus = true
		default:
			t.Fatalf("unexpected packet protocol=%d", pkt.proto)
		}
	}
	if !seenEvent || !seenOpus {
		t.Fatalf("seenEvent=%v seenOpus=%v, want both true", seenEvent, seenOpus)
	}

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	serviceListener := pair.ServerConn.ListenService(1)
	defer serviceListener.Close()
	go func() {
		svcStream, err := serviceListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- svcStream
	}()

	time.Sleep(50 * time.Millisecond)

	clientStream, err := pair.ClientConn.Dial(1)
	if err != nil {
		t.Fatalf("Dial(1) failed: %v", err)
	}
	defer clientStream.Close()

	var svcStream net.Conn
	select {
	case svcStream = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("ListenService(1).Accept err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ListenService(1).Accept timeout")
	}
	_ = svcStream.Close()
}

func TestConnEventConcurrentDelivery(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	const total = 32

	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			e := testEvent{V: testEventVersion, Name: "evt-concurrent"}
			raw := json.RawMessage(fmt.Sprintf(`{"i":%d}`, idx))
			e.Data = &raw
			if err := writeTestEvent(pair.ClientConn, e); err != nil {
				t.Errorf("writeTestEvent(%d) failed: %v", idx, err)
			}
		}()
	}
	wg.Wait()

	for i := range total {
		evt, err := readEventWithTimeout(pair.ServerConn, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadEvent(%d) failed: %v", i, err)
		}
		if evt.Name != "evt-concurrent" {
			t.Fatalf("event name mismatch: got=%q", evt.Name)
		}
	}
}

func TestConnUnderlyingErrorPropagation(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	_ = pair.ClientListener.UDP().Close()

	if _, err := pair.ClientConn.Dial(testServiceRPC); !errors.Is(err, giznet.ErrUDPClosed) {
		t.Fatalf("Dial(rpc, after close) err=%v, want %v", err, giznet.ErrUDPClosed)
	}
	if _, err := pair.ClientConn.Write(testProtocolEvent, []byte("x")); !errors.Is(err, giznet.ErrUDPClosed) {
		t.Fatalf("Write(after close) err=%v, want %v", err, giznet.ErrUDPClosed)
	}
}

// Listener.Peer returns the active Conn owned by the listener, not a new wrapper.
func TestListenerPeerReturnsEstablishedConn(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	got, ok := pair.ClientListener.Peer(pair.ServerKey.Public)
	if !ok {
		t.Fatal("clientListener.Peer did not find established Conn")
	}
	if got != pair.ClientConn {
		t.Fatal("clientListener.Peer should return the established Conn")
	}

	if err := pair.ClientConn.Close(); err != nil {
		t.Fatalf("clientConn.Close failed: %v", err)
	}
	if _, ok := pair.ClientListener.Peer(pair.ServerKey.Public); ok {
		t.Fatal("clientListener.Peer should not return closed Conn")
	}
}

// Listener.Dial actively establishes the listener's single Conn for a peer.
// Repeated Dial calls and concurrent Accept observations return that same Conn.
func TestListenerDialReturnsExistingConnAndAcceptCanObserveIt(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	serverListener := NewTestListener(t, serverKey)
	defer serverListener.Close()
	clientListener := NewTestListener(t, clientKey)
	defer clientListener.Close()

	clientAcceptCh := make(chan *giznet.Conn, 1)
	clientAcceptErrCh := make(chan error, 1)
	go func() {
		conn, err := clientListener.Accept()
		if err != nil {
			clientAcceptErrCh <- err
			return
		}
		clientAcceptCh <- conn
	}()

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	firstConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("first Dial failed: %v", err)
	}

	var clientAcceptedConn *giznet.Conn
	select {
	case clientAcceptedConn = <-clientAcceptCh:
		if clientAcceptedConn != firstConn {
			t.Fatal("client Accept should return the same Conn established by Dial")
		}
	case err := <-clientAcceptErrCh:
		t.Fatalf("client Accept failed during first Dial: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("client Accept timeout after first Dial")
	}

	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("server Accept failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("server Accept timeout")
	}

	first := testEvent{V: testEventVersion, Name: "first-generation"}
	if err := writeTestEvent(firstConn, first); err != nil {
		t.Fatalf("first generation write failed: %v", err)
	}
	if got, err := readTestEvent(serverConn); err != nil {
		t.Fatalf("server read first generation failed: %v", err)
	} else if got.Name != first.Name {
		t.Fatalf("first generation event=%q, want %q", got.Name, first.Name)
	}

	secondConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("second Dial failed: %v", err)
	}
	if secondConn != firstConn {
		t.Fatal("second Dial should return the existing Conn")
	}

	second := testEvent{V: testEventVersion, Name: "second-dial"}
	if err := writeTestEvent(secondConn, second); err != nil {
		t.Fatalf("second Dial write failed: %v", err)
	}
	if got, err := readTestEvent(serverConn); err != nil {
		t.Fatalf("server read second Dial event failed: %v", err)
	} else if got.Name != second.Name {
		t.Fatalf("second Dial event=%q, want %q", got.Name, second.Name)
	}
}

// Conn.Close closes the service mux but keeps the UDP/Noise peer. Without a
// rekey, the next client RPC stream should recreate the service mux and allow
// Listener.Accept to surface a new Conn for the same peer.
func TestServerConnCloseAllowsNextRPCStreamOnNewConn(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	rpcListener := pair.ServerConn.ListenService(testServiceRPC)
	defer rpcListener.Close()
	go func() {
		stream, err := rpcListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- stream
	}()

	clientStream, err := pair.ClientConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("client Dial(rpc) failed: %v", err)
	}
	defer clientStream.Close()

	if _, err := clientStream.Write([]byte("x")); err != nil {
		t.Fatalf("client stream priming write failed: %v", err)
	}

	var serverStream net.Conn
	select {
	case serverStream = <-acceptCh:
		defer serverStream.Close()
	case err := <-errCh:
		t.Fatalf("server ListenService(rpc).Accept failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server ListenService(rpc).Accept timeout")
	}

	if got := ReadExactWithTimeout(t, serverStream, 1, 5*time.Second); !bytes.Equal(got, []byte("x")) {
		t.Fatalf("server stream priming payload mismatch: got=%q want=%q", string(got), "x")
	}

	if err := pair.ServerConn.Close(); err != nil {
		t.Fatalf("serverConn.Close failed: %v", err)
	}

	nextConnCh := make(chan *giznet.Conn, 1)
	nextConnErrCh := make(chan error, 1)
	go func() {
		conn, err := pair.ServerListener.Accept()
		if err != nil {
			nextConnErrCh <- err
			return
		}
		nextConnCh <- conn
	}()

	if err := writeTestEvent(pair.ClientConn, testEvent{V: testEventVersion, Name: "wake-next-conn"}); err != nil {
		t.Fatalf("client wake write after server Conn.Close failed: %v", err)
	}

	var nextServerConn *giznet.Conn
	select {
	case nextServerConn = <-nextConnCh:
		if got := nextServerConn.PublicKey(); got != pair.ClientKey.Public {
			t.Fatalf("next Conn public key=%v, want %v", got, pair.ClientKey.Public)
		}
	case err := <-nextConnErrCh:
		t.Fatalf("Listener.Accept after server Conn.Close failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Listener.Accept after server Conn.Close timeout")
	}
	if got, err := readTestEvent(nextServerConn); err != nil {
		t.Fatalf("next Conn read wake event failed: %v", err)
	} else if got.Name != "wake-next-conn" {
		t.Fatalf("next Conn wake event=%q, want %q", got.Name, "wake-next-conn")
	}

	nextRPCListener := nextServerConn.ListenService(testServiceRPC)
	defer nextRPCListener.Close()
	nextAcceptCh := make(chan net.Conn, 1)
	nextErrCh := make(chan error, 1)
	go func() {
		stream, err := nextRPCListener.Accept()
		if err != nil {
			nextErrCh <- err
			return
		}
		nextAcceptCh <- stream
	}()

	nextClientStream, err := pair.ClientConn.Dial(testServiceRPC)
	if err != nil {
		t.Fatalf("client Dial(rpc) after server Conn.Close failed: %v", err)
	}
	defer nextClientStream.Close()

	req := []byte("z")
	if _, err := nextClientStream.Write(req); err != nil {
		t.Fatalf("client stream write on next RPC stream failed: %v", err)
	}

	select {
	case nextServerStream := <-nextAcceptCh:
		defer nextServerStream.Close()
		if got := ReadExactWithTimeout(t, nextServerStream, len(req), 5*time.Second); !bytes.Equal(got, req) {
			t.Fatalf("next server stream payload mismatch: got=%q want=%q", string(got), string(req))
		}
	case err := <-nextErrCh:
		t.Fatalf("next Conn service listener failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("next Conn service listener timeout")
	}
}

// This keeps Accept waiting before the Conn is closed, then sends service data
// immediately after Close returns. It covers the close/new-data handoff without
// changing the implementation for tighter concurrent races.
func TestServerConnCloseWithWaitingAcceptAndImmediateData(t *testing.T) {
	pair := NewConnectedPeerPair(t)
	defer pair.Close()

	serverConn := pair.ServerConn
	for i := range 5 {
		nextConnCh := make(chan *giznet.Conn, 1)
		nextErrCh := make(chan error, 1)
		go func() {
			conn, err := pair.ServerListener.Accept()
			if err != nil {
				nextErrCh <- err
				return
			}
			nextConnCh <- conn
		}()

		if err := serverConn.Close(); err != nil {
			t.Fatalf("iteration %d: server Conn.Close failed: %v", i, err)
		}
		eventName := fmt.Sprintf("immediate-after-close-%d", i)
		if err := writeTestEvent(pair.ClientConn, testEvent{V: testEventVersion, Name: eventName}); err != nil {
			t.Fatalf("iteration %d: client immediate write failed: %v", i, err)
		}

		select {
		case serverConn = <-nextConnCh:
			if got, err := readTestEvent(serverConn); err != nil {
				t.Fatalf("iteration %d: read immediate event failed: %v", i, err)
			} else if got.Name != eventName {
				t.Fatalf("iteration %d: event=%q, want %q", i, got.Name, eventName)
			}
		case err := <-nextErrCh:
			t.Fatalf("iteration %d: Accept failed after immediate write: %v", i, err)
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: Accept timeout after immediate write", i)
		}
	}
}
