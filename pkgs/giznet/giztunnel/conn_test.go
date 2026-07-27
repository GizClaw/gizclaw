package giztunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type packetLink struct {
	target *PacketMux
}

func (l *packetLink) Write(protocol byte, payload []byte) (int, error) {
	if protocol != giznet.ProtocolTunnelPacket {
		return 0, giznet.ErrPacketProtocol
	}
	if l.target == nil {
		return 0, giznet.ErrConnClosed
	}
	if err := l.target.HandlePacket(append([]byte(nil), payload...)); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func tunnelTestKeys(t *testing.T) (*giznet.KeyPair, *giznet.KeyPair, *giznet.KeyPair) {
	t.Helper()
	client, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edge, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return client, edge, server
}

func tunnelTestPair(t *testing.T, serverConfig Config) (*Conn, *Conn, OpenRequest) {
	t.Helper()
	clientKey, edgeKey, serverKey := tunnelTestKeys(t)
	sessionID, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	open := OpenRequest{
		SessionID:       sessionID,
		ClientPublicKey: clientKey.Public,
		EdgePublicKey:   edgeKey.Public,
		ServerPublicKey: serverKey.Public,
		IssuedAtUnix:    time.Now().Unix(),
		ExpiresAtUnix:   time.Now().Add(30 * time.Second).Unix(),
		RemoteAddr:      "198.51.100.10:4242",
	}
	leftStream, rightStream := net.Pipe()
	leftLink := &packetLink{}
	rightLink := &packetLink{}
	leftMux := NewPacketMux(leftLink)
	rightMux := NewPacketMux(rightLink)
	leftLink.target = rightMux
	rightLink.target = leftMux

	type acceptResult struct {
		conn *Conn
		open OpenRequest
		err  error
	}
	accepted := make(chan acceptResult, 1)
	serverConfig.PeerPublicKey = clientKey.Public
	go func() {
		conn, gotOpen, acceptErr := Accept(
			context.Background(),
			rightStream,
			rightMux,
			func(OpenRequest) error { return nil },
			serverConfig,
		)
		accepted <- acceptResult{conn: conn, open: gotOpen, err: acceptErr}
	}()
	clientConn, err := Dial(context.Background(), leftStream, leftMux, open, Config{
		PeerPublicKey: serverKey.Public,
	})
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	result := <-accepted
	if result.err != nil {
		t.Fatalf("Accept error = %v", result.err)
	}
	if result.open != open {
		t.Fatalf("accepted open = %+v, want %+v", result.open, open)
	}
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = result.conn.Close()
	})
	return clientConn, result.conn, open
}

func TestConnMultiplexesServicesAndPackets(t *testing.T) {
	clientConn, serverConn, open := tunnelTestPair(t, Config{})
	if !serverConn.PublicKey().Equal(open.ClientPublicKey) {
		t.Fatalf("server logical public key = %s, want %s", serverConn.PublicKey(), open.ClientPublicKey)
	}

	listener := serverConn.ListenService(7)
	clientStream, err := clientConn.Dial(7)
	if err != nil {
		t.Fatalf("Dial service error = %v", err)
	}
	serverStream, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept service error = %v", err)
	}
	defer clientStream.Close()
	defer serverStream.Close()
	if _, err := clientStream.Write([]byte("reliable")); err != nil {
		t.Fatalf("service Write error = %v", err)
	}
	reliable := make([]byte, 16)
	n, err := io.ReadFull(serverStream, reliable[:8])
	if err != nil || n != 8 || string(reliable[:n]) != "reliable" {
		t.Fatalf("service Read = %q, %d, %v", reliable[:n], n, err)
	}

	if _, err := clientConn.Write(0x42, []byte("packet")); err != nil {
		t.Fatalf("packet Write error = %v", err)
	}
	packet := make([]byte, 32)
	protocol, n, err := serverConn.Read(packet)
	if err != nil || protocol != 0x42 || string(packet[:n]) != "packet" {
		t.Fatalf("packet Read = %x %q %v", protocol, packet[:n], err)
	}
	if _, err := serverConn.Write(giznet.ProtocolOpusPacket, []byte{1, 2, 3}); err != nil {
		t.Fatalf("opus Write error = %v", err)
	}
	protocol, n, err = clientConn.Read(packet)
	if err != nil || protocol != giznet.ProtocolOpusPacket || string(packet[:n]) != string([]byte{1, 2, 3}) {
		t.Fatalf("opus Read = %x %v %v", protocol, packet[:n], err)
	}
}

func TestVirtualStreamReadDeadlineInterruptsRead(t *testing.T) {
	clientConn, serverConn, _ := tunnelTestPair(t, Config{})
	listener := serverConn.ListenService(7)
	clientStream, err := clientConn.Dial(7)
	if err != nil {
		t.Fatal(err)
	}
	serverStream, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer clientStream.Close()
	defer serverStream.Close()

	readErr := make(chan error, 1)
	go func() {
		var payload [1]byte
		_, readErrValue := serverStream.Read(payload[:])
		readErr <- readErrValue
	}()
	if err := serverStream.SetReadDeadline(time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readErr:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("Read error = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SetReadDeadline did not interrupt Read")
	}

	if err := serverStream.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := clientStream.Write([]byte{0x42}); err != nil {
		t.Fatal(err)
	}
	var payload [1]byte
	if _, err := io.ReadFull(serverStream, payload[:]); err != nil || payload[0] != 0x42 {
		t.Fatalf("Read after clearing deadline = %x, %v", payload, err)
	}
}

func TestConnRejectsForbiddenServiceWithoutAffectingPacketMux(t *testing.T) {
	clientConn, serverConn, _ := tunnelTestPair(t, Config{
		AllowRemoteService: func(service uint64) bool { return service == 1 },
	})
	if _, err := clientConn.Dial(2); err != nil {
		t.Fatalf("client Dial returns before remote rejection: %v", err)
	}
	select {
	case <-serverConn.closeCh:
	case <-time.After(time.Second):
		t.Fatal("forbidden service did not close the logical session")
	}
	if !errors.Is(serverConn.err(), ErrServiceForbidden) {
		t.Fatalf("server close error = %v, want %v", serverConn.err(), ErrServiceForbidden)
	}
}

func TestConnRejectsRemoteStreamIDsFromLocalParity(t *testing.T) {
	clientConn, serverConn, _ := tunnelTestPair(t, Config{})
	if err := clientConn.acceptRemoteStream(1, 7); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("initiator accepted odd remote stream ID: %v", err)
	}
	if err := serverConn.acceptRemoteStream(2, 7); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("acceptor accepted even remote stream ID: %v", err)
	}
	if len(clientConn.streams) != 0 || len(serverConn.streams) != 0 {
		t.Fatalf("rejected stream IDs mutated stream maps: client=%d server=%d",
			len(clientConn.streams), len(serverConn.streams))
	}

	clientListener := clientConn.ListenService(9)
	serverStream, err := serverConn.Dial(9)
	if err != nil {
		t.Fatalf("acceptor Dial error = %v", err)
	}
	defer serverStream.Close()
	clientStream, err := clientListener.Accept()
	if err != nil {
		t.Fatalf("initiator Accept error = %v", err)
	}
	defer clientStream.Close()
}

func TestConnDialRejectsLocalStreamIDCollision(t *testing.T) {
	clientConn, _, _ := tunnelTestPair(t, Config{})
	collision := newVirtualStream(clientConn, 1, 7)
	clientConn.mu.Lock()
	clientConn.addStreamLocked(collision)
	clientConn.mu.Unlock()
	t.Cleanup(func() {
		clientConn.removeStream(collision.id)
		collision.abort()
	})

	if _, err := clientConn.Dial(8); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Dial collision error = %v, want %v", err, ErrInvalidState)
	}
}

func TestConnBufferLimitClosesOnlySession(t *testing.T) {
	clientConn, serverConn, _ := tunnelTestPair(t, Config{
		MaxBufferedBytes: 4,
		StreamQueueSize:  1,
	})
	listener := serverConn.ListenService(1)
	stream, err := clientConn.Dial(1)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	if _, err := stream.Write([]byte("too large")); err != nil {
		t.Fatalf("local Write error = %v", err)
	}
	select {
	case <-serverConn.closeCh:
	case <-time.After(time.Second):
		t.Fatal("buffer overflow did not close the logical session")
	}
	if !errors.Is(serverConn.err(), ErrBufferLimit) {
		t.Fatalf("server close error = %v, want %v", serverConn.err(), ErrBufferLimit)
	}
}

func TestVirtualStreamDrainsDataBeforeRemoteClose(t *testing.T) {
	clientConn, serverConn, _ := tunnelTestPair(t, Config{})
	listener := serverConn.ListenService(1)
	stream, err := clientConn.Dial(1)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	if _, err := stream.Write([]byte("final")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(accepted)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "final" {
		t.Fatalf("drained payload = %q", payload)
	}
}

func TestVirtualStreamCloseReleasesUnreadRemoteData(t *testing.T) {
	clientConn, serverConn, _ := tunnelTestPair(t, Config{})
	listener := serverConn.ListenService(1)
	stream, err := clientConn.Dial(1)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("unread")); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for serverConn.buffered.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := serverConn.buffered.Load(); got != int64(len("unread")) {
		t.Fatalf("buffered before local close = %d", got)
	}
	if err := accepted.Close(); err != nil {
		t.Fatal(err)
	}
	if got := serverConn.buffered.Load(); got != 0 {
		t.Fatalf("buffered after local close = %d", got)
	}
}

func TestPacketMuxIsolatesUnknownAndOverflowSessions(t *testing.T) {
	link := &packetLink{}
	mux := NewPacketMux(link)
	link.target = mux
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	end, err := mux.register(id)
	if err != nil {
		t.Fatal(err)
	}
	defer end.close()
	frame := make([]byte, packetHeaderSize+1)
	frame[0] = Version
	copy(frame[1:17], id[:])
	frame[17] = 0x44
	frame[18] = 'x'
	if err := mux.HandlePacket(frame); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	protocol, n, err := end.read(buf)
	if err != nil || protocol != 0x44 || n != 1 || buf[0] != 'x' {
		t.Fatalf("read = %x %d %v %v", protocol, n, buf, err)
	}
	frame[1] ^= 0xff
	if err := mux.HandlePacket(frame); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session error = %v", err)
	}
}
