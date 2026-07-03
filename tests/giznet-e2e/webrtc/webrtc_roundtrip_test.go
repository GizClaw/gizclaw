//go:build giznet_e2e

package webrtc_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

const echoService uint64 = 7001

var errPayloadMismatch = errors.New("payload mismatch")

type allowAllPolicy struct{}

func (allowAllPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (allowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

func TestWebRTCSignalingPacketAndServiceStream(t *testing.T) {
	for _, mode := range []gizwebrtc.CipherMode{
		gizwebrtc.CipherModePlaintext,
		gizwebrtc.CipherModeChaChaPoly,
		gizwebrtc.CipherModeAES256GCM,
	} {
		t.Run(string(mode), func(t *testing.T) {
			serverKey := mustKeyPair(t)
			clientKey := mustKeyPair(t)
			server := startWebRTCServer(t, serverKey, mode)
			defer server.Close()

			clientListener, clientConn := dialWebRTC(t, clientKey, serverKey.Public, server.signalingURL, mode)
			defer clientListener.Close()
			defer clientConn.Close()

			serverConn := acceptConn(t, server.listener)
			defer serverConn.Close()

			roundTripPacket(t, clientConn, serverConn, 0x42, []byte("packet"))
			roundTripStampedOpus(t, clientConn, serverConn)

			done := serveEchoService(t, serverConn)
			payload := bytes.Repeat([]byte("webrtc-stream-"), 8192)
			if got := roundTripStream(t, clientConn, payload); !bytes.Equal(got, payload) {
				t.Fatalf("stream echo len=%d, want %d", len(got), len(payload))
			}
			serverConn.CloseService(echoService)
			waitServerDone(t, done)
		})
	}
}

func TestWebRTCConcurrentServiceStreams(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	server := startWebRTCServer(t, serverKey, gizwebrtc.CipherModePlaintext)
	defer server.Close()

	clientListener, clientConn := dialWebRTC(t, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.CipherModePlaintext)
	defer clientListener.Close()
	defer clientConn.Close()

	serverConn := acceptConn(t, server.listener)
	defer serverConn.Close()

	done := serveEchoService(t, serverConn)
	const streams = 8
	payload := bytes.Repeat([]byte("stream-backpressure-"), 2048)
	var wg sync.WaitGroup
	errCh := make(chan error, streams)
	for i := range streams {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			stream, err := clientConn.Dial(echoService)
			if err != nil {
				errCh <- err
				return
			}
			defer stream.Close()
			if err := stream.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
				errCh <- err
				return
			}
			want := append([]byte{byte(i)}, payload...)
			if _, err := stream.Write(want); err != nil {
				errCh <- err
				return
			}
			got := make([]byte, len(want))
			if _, err := io.ReadFull(stream, got); err != nil {
				errCh <- err
				return
			}
			if !bytes.Equal(got, want) {
				errCh <- errPayloadMismatch
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent stream error = %v", err)
		}
	}
	serverConn.CloseService(echoService)
	waitServerDone(t, done)
}

func TestWebRTCServerConnClosesAfterClientDisconnect(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	server := startWebRTCServer(t, serverKey, gizwebrtc.CipherModePlaintext)
	defer server.Close()

	clientListener, clientConn := dialWebRTC(t, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.CipherModePlaintext)
	serverConn := acceptConn(t, server.listener)
	defer serverConn.Close()

	roundTripPacket(t, clientConn, serverConn, 0x42, []byte("before close"))
	if err := clientConn.Close(); err != nil {
		t.Fatalf("client Conn Close error = %v", err)
	}
	if err := clientListener.Close(); err != nil {
		t.Fatalf("client Listener Close error = %v", err)
	}
	waitConnReadClosed(t, serverConn)
	if info := serverConn.PeerInfo(); info == nil || info.State != giznet.PeerStateOffline {
		t.Fatalf("server peer info after client close = %#v, want offline", info)
	}
}

func TestWebRTCDuplicatePublicKeyFollowsGizClawSingleActivePeer(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	server := startWebRTCServer(t, serverKey, gizwebrtc.CipherModePlaintext)
	defer server.Close()

	oldClientListener, oldClientConn := dialWebRTC(t, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.CipherModePlaintext)
	defer oldClientListener.Close()
	defer oldClientConn.Close()
	oldServerConn := acceptConn(t, server.listener)
	defer oldServerConn.Close()

	newClientListener, newClientConn := dialWebRTC(t, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.CipherModePlaintext)
	defer newClientListener.Close()
	defer newClientConn.Close()
	newServerConn := acceptConn(t, server.listener)
	defer newServerConn.Close()

	if oldServerConn.PublicKey() != clientKey.Public || newServerConn.PublicKey() != clientKey.Public {
		t.Fatalf("server conns public keys = %s/%s, want %s", oldServerConn.PublicKey(), newServerConn.PublicKey(), clientKey.Public)
	}
	if oldServerConn == newServerConn {
		t.Fatal("duplicate WebRTC accept returned the same connection")
	}

	manager := gizclaw.NewManager(nil)
	if oldConn := manager.SetPeerUp(clientKey.Public, oldServerConn); oldConn != nil {
		t.Fatal("first SetPeerUp returned a replaced conn, want nil")
	}
	if oldConn := manager.SetPeerUp(clientKey.Public, newServerConn); oldConn != oldServerConn {
		t.Fatal("replacement did not return first WebRTC conn")
	}
	manager.SetPeerDown(clientKey.Public, oldServerConn)
	if got, ok := manager.Peer(clientKey.Public); !ok || got != newServerConn {
		t.Fatalf("stale WebRTC teardown cleared active peer: ok=%v", ok)
	}
	manager.SetPeerDown(clientKey.Public, newServerConn)
	if _, ok := manager.Peer(clientKey.Public); ok {
		t.Fatal("active WebRTC teardown should remove active peer")
	}
}

type webRTCServer struct {
	listener     *gizwebrtc.Listener
	httpServer   *httptest.Server
	signalingURL string
}

func startWebRTCServer(tb testing.TB, key *giznet.KeyPair, mode gizwebrtc.CipherMode) *webRTCServer {
	tb.Helper()
	listener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     mode,
		SecurityPolicy: allowAllPolicy{},
	}).Listen(key)
	if err != nil {
		tb.Fatalf("gizwebrtc Listen error = %v", err)
	}
	httpServer := httptest.NewServer(listener.SignalingHandler())
	return &webRTCServer{
		listener:     listener,
		httpServer:   httpServer,
		signalingURL: httpServer.URL + gizwebrtc.SignalingPath,
	}
}

func (s *webRTCServer) Close() {
	if s == nil {
		return
	}
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func dialWebRTC(tb testing.TB, key *giznet.KeyPair, serverPK giznet.PublicKey, signalingURL string, mode gizwebrtc.CipherMode) (giznet.Listener, giznet.Conn) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	listener, conn, err := gizwebrtc.Dial(ctx, key, serverPK, gizwebrtc.DialConfig{
		SignalingURL:   signalingURL,
		CipherMode:     mode,
		SecurityPolicy: allowAllPolicy{},
	})
	if err != nil {
		tb.Fatalf("gizwebrtc Dial error = %v", err)
	}
	return listener, conn
}

func acceptConn(tb testing.TB, listener *gizwebrtc.Listener) giznet.Conn {
	tb.Helper()
	type result struct {
		conn giznet.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := listener.Accept()
		ch <- result{conn: conn, err: err}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			tb.Fatalf("Accept error = %v", res.err)
		}
		return res.conn
	case <-time.After(5 * time.Second):
		tb.Fatal("Accept timeout")
		return nil
	}
}

func roundTripPacket(t *testing.T, client, server giznet.Conn, protocol byte, payload []byte) {
	t.Helper()
	if _, err := client.Write(protocol, payload); err != nil {
		t.Fatalf("client packet Write error = %v", err)
	}
	gotProtocol, gotPayload := readPacket(t, server)
	if gotProtocol != protocol || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("packet proto=%d payload=%q, want proto=%d payload=%q", gotProtocol, gotPayload, protocol, payload)
	}
}

func roundTripStampedOpus(t *testing.T, client, server giznet.Conn) {
	t.Helper()
	frame := []byte{0x00, 0xaa, 0xbb}
	payload := stampedopus.Pack(uint64(time.Now().UnixMilli()), frame)
	if _, err := client.Write(gizwebrtc.ProtocolStampedOpus, payload); err != nil {
		t.Fatalf("client stamped opus Write error = %v", err)
	}
	gotProtocol, gotPayload := readPacket(t, server)
	if gotProtocol != gizwebrtc.ProtocolStampedOpus {
		t.Fatalf("stamped opus proto=%d, want %d", gotProtocol, gizwebrtc.ProtocolStampedOpus)
	}
	if _, gotFrame, ok := stampedopus.Unpack(gotPayload); !ok || !bytes.Equal(gotFrame, frame) {
		t.Fatalf("stamped opus payload=%v frame=%v ok=%t, want frame=%v", gotPayload, gotFrame, ok, frame)
	}
}

func readPacket(t *testing.T, conn giznet.Conn) (byte, []byte) {
	t.Helper()
	type result struct {
		protocol byte
		payload  []byte
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 64*1024)
		protocol, n, err := conn.Read(buf)
		ch <- result{protocol: protocol, payload: append([]byte(nil), buf[:n]...), err: err}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("packet Read error = %v", res.err)
		}
		return res.protocol, res.payload
	case <-time.After(5 * time.Second):
		t.Fatal("packet Read timeout")
		return 0, nil
	}
}

func waitConnReadClosed(t *testing.T, conn giznet.Conn) {
	t.Helper()
	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 1)
		_, _, err := conn.Read(buf)
		ch <- result{err: err}
	}()
	select {
	case res := <-ch:
		if res.err == nil {
			t.Fatal("server Read after client close error = nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server conn did not close after client disconnect")
	}
}

func serveEchoService(t *testing.T, conn giznet.Conn) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	service := conn.ListenService(echoService)
	go func() {
		defer close(done)
		defer service.Close()
		for {
			stream, err := service.Accept()
			if err != nil {
				return
			}
			go func(stream net.Conn) {
				defer stream.Close()
				_, _ = io.Copy(stream, stream)
			}(stream)
		}
	}()
	return done
}

func roundTripStream(t *testing.T, conn giznet.Conn, payload []byte) []byte {
	t.Helper()
	stream, err := conn.Dial(echoService)
	if err != nil {
		t.Fatalf("Dial(echoService) error = %v", err)
	}
	defer stream.Close()
	if err := stream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetDeadline error = %v", err)
	}
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("stream Write error = %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, got); err != nil {
		t.Fatalf("stream ReadFull error = %v", err)
	}
	return got
}

func waitServerDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("echo server error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("echo server did not stop")
	}
}

func mustKeyPair(tb testing.TB) *giznet.KeyPair {
	tb.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		tb.Fatalf("GenerateKeyPair error = %v", err)
	}
	return key
}
