package gizwebrtc

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// TestPeerInfoLastSeenTracksActivity verifies that LastSeen reports observed
// traffic rather than the moment of the query: it stays put while the
// connection is idle and advances again once a packet or a service stream
// carries bytes.
func TestPeerInfoLastSeenTracksActivity(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&ListenConfig{
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clientListener, clientConn, err := Dial(ctx, clientKey, serverKey.Public, DialConfig{
		SignalingURL:   httpServer.URL + SignalingPath,
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	})
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientListener.Close()
	defer clientConn.Close()
	serverConn := acceptConn(t, serverListener)
	defer serverConn.Close()

	if _, err := clientConn.Write(0x42, []byte("packet")); err != nil {
		t.Fatalf("client packet Write error = %v", err)
	}
	buf := make([]byte, 64)
	if _, _, err := serverConn.Read(buf); err != nil {
		t.Fatalf("server packet Read error = %v", err)
	}
	afterPacket := serverConn.PeerInfo().LastSeen
	if afterPacket.IsZero() {
		t.Fatal("server LastSeen is zero after packet traffic")
	}

	// An idle connection keeps the timestamp of its last traffic; a query time
	// would move on its own.
	time.Sleep(20 * time.Millisecond)
	idleQuery := time.Now()
	if idle := serverConn.PeerInfo().LastSeen; !idle.Equal(afterPacket) {
		t.Fatalf("idle LastSeen = %v, want %v", idle, afterPacket)
	}
	if !afterPacket.Before(idleQuery) {
		t.Fatalf("LastSeen %v is not older than the idle query at %v", afterPacket, idleQuery)
	}

	service := serverConn.ListenService(100)
	clientStream, err := clientConn.Dial(100)
	if err != nil {
		t.Fatalf("client Dial(service) error = %v", err)
	}
	defer clientStream.Close()
	accepted, err := service.Accept()
	if err != nil {
		t.Fatalf("server service Accept error = %v", err)
	}
	defer accepted.Close()
	if _, err := clientStream.Write([]byte("hello stream")); err != nil {
		t.Fatalf("client stream Write error = %v", err)
	}
	if _, err := accepted.Read(buf); err != nil {
		t.Fatalf("server stream Read error = %v", err)
	}
	afterStream := serverConn.PeerInfo().LastSeen
	if !afterStream.After(afterPacket) {
		t.Fatalf("stream LastSeen = %v, want after %v", afterStream, afterPacket)
	}
}
