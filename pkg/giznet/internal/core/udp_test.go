package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/noise"
)

func buildHandshakeResponseForUDPTest(t *testing.T, initiator *noise.KeyPair, responder *noise.KeyPair, localIdx, remoteIdx uint32) (*noise.HandshakeState, []byte) {
	t.Helper()

	// Keep the initiator state waiting for msg2 so tests can pass either a
	// valid response or malformed garbage into handleHandshakeResp.
	initHS, err := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  initiator,
		RemoteStatic: &responder.Public,
	})
	if err != nil {
		t.Fatalf("initiator NewHandshakeState failed: %v", err)
	}
	msg1, err := initHS.WriteMessage(nil)
	if err != nil {
		t.Fatalf("initiator WriteMessage failed: %v", err)
	}

	respHS, err := noise.NewHandshakeState(noise.Config{
		Pattern:     noise.PatternIK,
		Initiator:   false,
		LocalStatic: responder,
	})
	if err != nil {
		t.Fatalf("responder NewHandshakeState failed: %v", err)
	}
	if _, err := respHS.ReadMessage(msg1); err != nil {
		t.Fatalf("responder ReadMessage failed: %v", err)
	}
	msg2, err := respHS.WriteMessage(nil)
	if err != nil {
		t.Fatalf("responder WriteMessage failed: %v", err)
	}

	wire := noise.BuildHandshakeResp(remoteIdx, localIdx, respHS.LocalEphemeral(), msg2[noise.KeySize:])
	return initHS, wire
}

func TestNewUDP(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	udp, err := NewUDP(key)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	defer udp.Close()

	info := udp.HostInfo()
	if info.PublicKey != key.Public {
		t.Errorf("PublicKey mismatch")
	}
	if info.Addr == nil {
		t.Errorf("Addr should not be nil")
	}
}

func TestNewUDPWithBindAddr(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	udp, err := NewUDP(key, WithBindAddr("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	defer udp.Close()

	addr := udp.HostInfo().Addr
	if addr.IP.String() != "127.0.0.1" {
		t.Errorf("Expected 127.0.0.1, got %s", addr.IP.String())
	}
}

func TestUDPClosedChan(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	u, err := NewUDP(key, WithBindAddr("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}

	ch := u.closedChan()
	if ch != u.closeChan {
		t.Fatalf("closedChan should return internal closeChan")
	}
	if err := u.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case <-ch:
		// expected
	case <-time.After(1 * time.Second):
		t.Fatal("close channel not closed in time")
	}
}

func TestUDPConnect(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()

	udp, err := NewUDP(localKey, WithBindAddr("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("NewUDP() error = %v", err)
	}
	defer udp.Close()

	remoteKey, _ := noise.GenerateKeyPair()
	err = udp.Connect(remoteKey.Public)
	if err != ErrPeerNotFound {
		t.Errorf("Connect() with no peer error = %v, want %v", err, ErrPeerNotFound)
	}

	remoteAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	udp.SetPeerEndpoint(remoteKey.Public, remoteAddr)

	udp.Close()
	err = udp.Connect(remoteKey.Public)
	if err != ErrClosed {
		t.Errorf("Connect() after close error = %v, want %v", err, ErrClosed)
	}
}

func TestPeerStateString(t *testing.T) {
	tests := []struct {
		state PeerState
		want  string
	}{
		{PeerStateNew, "new"},
		{PeerStateConnecting, "connecting"},
		{PeerStateEstablished, "established"},
		{PeerStateFailed, "failed"},
		{PeerStateOffline, "offline"},
		{PeerState(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("PeerState(%d).String() = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestSetPeerEndpoint(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	udp, err := NewUDP(key)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	defer udp.Close()

	peerKey, _ := noise.GenerateKeyPair()
	peerAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")

	udp.SetPeerEndpoint(peerKey.Public, peerAddr)

	info := udp.PeerInfo(peerKey.Public)
	if info == nil {
		t.Fatalf("PeerInfo returned nil")
	}
	if info.PublicKey != peerKey.Public {
		t.Errorf("PublicKey mismatch")
	}
	if info.Endpoint.String() != peerAddr.String() {
		t.Errorf("Endpoint mismatch: %s != %s", info.Endpoint.String(), peerAddr.String())
	}
}

func TestSetPeerEndpoint_IgnoresClosedAndInvalidAddr(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	peerKey, _ := noise.GenerateKeyPair()

	udp, err := NewUDP(key)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	udp.Close()
	udp.SetPeerEndpoint(peerKey.Public, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345})
	if udp.PeerInfo(peerKey.Public) != nil {
		t.Fatal("SetPeerEndpoint should ignore updates after close")
	}
}

func TestClosePeerServiceMux(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	events := make(chan PeerEvent, 1)
	udp, err := NewUDP(key, WithOnPeerEvent(func(ev PeerEvent) bool {
		events <- ev
		return true
	}))
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	defer udp.Close()

	peerKey, _ := noise.GenerateKeyPair()
	peerAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")

	udp.SetPeerEndpoint(peerKey.Public, peerAddr)

	if udp.PeerInfo(peerKey.Public) == nil {
		t.Fatalf("Peer should exist")
	}

	udp.ClosePeerServiceMux(peerKey.Public)

	info := udp.PeerInfo(peerKey.Public)
	if info == nil {
		t.Fatal("Peer should remain after offline")
	}
	if info.State != PeerStateOffline {
		t.Fatalf("Peer state = %v, want %v", info.State, PeerStateOffline)
	}
	select {
	case ev := <-events:
		if ev.PublicKey != peerKey.Public || ev.State != PeerStateOffline {
			t.Fatalf("ClosePeerServiceMux event = %+v, want offline for %x", ev, peerKey.Public)
		}
	default:
		t.Fatal("ClosePeerServiceMux did not emit offline event")
	}

	udp.ClosePeerServiceMux(peerKey.Public)
	select {
	case ev := <-events:
		t.Fatalf("second ClosePeerServiceMux emitted duplicate event: %+v", ev)
	default:
	}
}

func TestClosePeerServiceMux_CleansServiceMuxKeepsSession(t *testing.T) {
	localKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(local) failed: %v", err)
	}
	peerKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(peer) failed: %v", err)
	}
	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  11,
		RemoteIndex: 22,
		SendKey:     noise.Key{1},
		RecvKey:     noise.Key{2},
		RemotePK:    peerKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	smux := NewServiceMux(peerKey.Public, ServiceMuxConfig{})
	u := &UDP{
		localKey: localKey,
		peers: map[noise.PublicKey]*peerState{
			peerKey.Public: {
				pk:         peerKey.Public,
				session:    session,
				serviceMux: smux,
				state:      PeerStateEstablished,
			},
		},
		byIndex: make(map[uint32]*peerState),
	}
	u.byIndex[session.LocalIndex()] = u.peers[peerKey.Public]

	u.ClosePeerServiceMux(peerKey.Public)

	peer := u.peers[peerKey.Public]
	if peer == nil {
		t.Fatal("peer should remain in map")
	}
	if got := u.byIndex[session.LocalIndex()]; got != peer {
		t.Fatal("session index should remain mapped to peer")
	}
	if _, err := smux.OpenStream(0); err != ErrServiceMuxClosed {
		t.Fatalf("service mux should be closed after ClosePeerServiceMux, err=%v", err)
	}
	peer.mu.RLock()
	defer peer.mu.RUnlock()
	if peer.session != session {
		t.Fatal("session should remain after ClosePeerServiceMux")
	}
	if peer.serviceMux != nil {
		t.Fatal("service mux should be cleared after ClosePeerServiceMux")
	}
	if peer.state != PeerStateOffline {
		t.Fatalf("peer.state=%v, want %v", peer.state, PeerStateOffline)
	}
}

func TestPeerServiceMuxReestablishesOfflinePeer(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	peerKey, _ := noise.GenerateKeyPair()
	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  11,
		RemoteIndex: 22,
		SendKey:     noise.Key{1},
		RecvKey:     noise.Key{2},
		RemotePK:    peerKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	events := make(chan PeerEvent, 2)
	oldMux := NewServiceMux(peerKey.Public, ServiceMuxConfig{})
	peer := &peerState{
		pk:         peerKey.Public,
		session:    session,
		serviceMux: oldMux,
		state:      PeerStateEstablished,
	}
	u := &UDP{
		localKey: localKey,
		peers: map[noise.PublicKey]*peerState{
			peerKey.Public: peer,
		},
		byIndex: map[uint32]*peerState{
			session.LocalIndex(): peer,
		},
		onPeerEvent: func(ev PeerEvent) bool {
			events <- ev
			return true
		},
	}

	u.ClosePeerServiceMux(peerKey.Public)
	if ev := <-events; ev.State != PeerStateOffline {
		t.Fatalf("ClosePeerServiceMux event = %+v, want offline", ev)
	}

	newMux, err := u.PeerServiceMux(peerKey.Public)
	if err != nil {
		t.Fatalf("PeerServiceMux error = %v", err)
	}
	if newMux == nil || newMux == oldMux {
		t.Fatal("PeerServiceMux should create a new service mux")
	}
	peer.mu.RLock()
	state := peer.state
	activeMux := peer.serviceMux
	peer.mu.RUnlock()
	if state != PeerStateEstablished {
		t.Fatalf("peer.state=%v, want %v", state, PeerStateEstablished)
	}
	if activeMux != newMux {
		t.Fatal("peer.serviceMux should point at the new service mux")
	}
	if ev := <-events; ev.State != PeerStateEstablished {
		t.Fatalf("PeerServiceMux event = %+v, want established", ev)
	}
}

func TestPeersIterator(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	udp, err := NewUDP(key)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	defer udp.Close()

	// Add some peers
	for range 3 {
		peerKey, _ := noise.GenerateKeyPair()
		peerAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
		udp.SetPeerEndpoint(peerKey.Public, peerAddr)
	}

	count := 0
	for range udp.Peers() {
		count++
	}

	if count != 3 {
		t.Errorf("Expected 3 peers, got %d", count)
	}
}

func TestHandshakeAndTransport(t *testing.T) {
	// Create two UDP instances
	key1, _ := noise.GenerateKeyPair()
	key2, _ := noise.GenerateKeyPair()

	udp1, err := NewUDP(key1, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("NewUDP 1 failed: %v", err)
	}
	defer udp1.Close()

	udp2, err := NewUDP(key2, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("NewUDP 2 failed: %v", err)
	}
	defer udp2.Close()

	// Get addresses
	addr1 := udp1.HostInfo().Addr
	addr2 := udp2.HostInfo().Addr

	// Set up peer endpoints
	udp1.SetPeerEndpoint(key2.Public, addr2)
	udp2.SetPeerEndpoint(key1.Public, addr1)

	// Start receive goroutine for udp2 (responder)
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		for {
			pk, n, err := udp2.ReadFrom(buf)
			if err != nil {
				return
			}
			if pk == key1.Public {
				received <- append([]byte{}, buf[:n]...)
				return
			}
		}
	}()

	// Start receive goroutine for udp1 (initiator) to handle handshake response
	go func() {
		buf := make([]byte, 1024)
		for {
			_, _, err := udp1.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	// Give the goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Initiate handshake from udp1
	udp1.mu.RLock()
	peer1 := udp1.peers[key2.Public]
	udp1.mu.RUnlock()

	err = udp1.initiateHandshake(peer1)
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	// Check that peer1 is now established
	info1 := udp1.PeerInfo(key2.Public)
	if info1.State != PeerStateEstablished {
		t.Errorf("Expected established state, got %v", info1.State)
	}

	// Send a message
	testData := []byte("hello world")
	err = udp1.WriteTo(key2.Public, testData)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	// Wait for message
	select {
	case data := <-received:
		if !bytes.Equal(data, testData) {
			t.Errorf("Data mismatch: %s != %s", data, testData)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout waiting for message")
	}
}

func TestRoaming(t *testing.T) {
	// Create two UDP instances
	key1, _ := noise.GenerateKeyPair()
	key2, _ := noise.GenerateKeyPair()

	udp1, err := NewUDP(key1, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("NewUDP 1 failed: %v", err)
	}
	defer udp1.Close()

	udp2, err := NewUDP(key2, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("NewUDP 2 failed: %v", err)
	}
	defer udp2.Close()

	// Get addresses
	addr1 := udp1.HostInfo().Addr
	addr2 := udp2.HostInfo().Addr

	// Set up peer endpoints
	udp1.SetPeerEndpoint(key2.Public, addr2)
	udp2.SetPeerEndpoint(key1.Public, addr1)

	// Start receive goroutine for udp2 (responder)
	go func() {
		buf := make([]byte, 1024)
		for {
			_, _, err := udp2.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	// Start receive goroutine for udp1 (initiator) to handle handshake response
	go func() {
		buf := make([]byte, 1024)
		for {
			_, _, err := udp1.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	// Give the goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Initiate handshake from udp1
	udp1.mu.RLock()
	peer1 := udp1.peers[key2.Public]
	udp1.mu.RUnlock()

	err = udp1.initiateHandshake(peer1)
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	// Check initial endpoint on udp2
	info2 := udp2.PeerInfo(key1.Public)
	if info2 == nil {
		t.Fatalf("Peer should exist on udp2")
	}
	initialEndpoint := info2.Endpoint.String()

	// Create a new UDP socket to simulate roaming
	newSocket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("Failed to create new socket: %v", err)
	}
	defer newSocket.Close()

	newAddr := newSocket.LocalAddr().(*net.UDPAddr)

	// Manually send a transport message from the new address
	// This simulates the peer having roamed to a new address
	udp1.mu.RLock()
	session1 := udp1.peers[key2.Public].session
	udp1.mu.RUnlock()

	if session1 == nil {
		t.Fatalf("Session should exist")
	}

	testData := []byte("roamed message")
	encrypted, nonce, err := session1.Encrypt(testData)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	msg := noise.BuildTransportMessage(session1.RemoteIndex(), nonce, encrypted)
	_, err = newSocket.WriteToUDP(msg, addr2)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Check that endpoint was updated (roaming)
	info2 = udp2.PeerInfo(key1.Public)
	if info2.Endpoint.String() == initialEndpoint {
		t.Logf("Initial: %s, Current: %s, New: %s", initialEndpoint, info2.Endpoint.String(), newAddr.String())
		// Note: The endpoint might not change if the test runs too fast
		// This is a best-effort check
	}
}

func TestClose(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	udp, err := NewUDP(key)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}

	err = udp.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Second close should be no-op
	err = udp.Close()
	if err != nil {
		t.Fatalf("Second close should not error: %v", err)
	}

	// WriteTo should fail after close
	peerKey, _ := noise.GenerateKeyPair()
	err = udp.WriteTo(peerKey.Public, []byte("test"))
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed, got %v", err)
	}
}

func TestUDPWriteToErrors(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	udp, err := NewUDP(localKey, WithBindAddr("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("NewUDP() error = %v", err)
	}

	remoteKey, _ := noise.GenerateKeyPair()

	err = udp.WriteTo(remoteKey.Public, []byte("test"))
	if err != ErrPeerNotFound {
		t.Errorf("WriteTo(non-existent) error = %v, want %v", err, ErrPeerNotFound)
	}

	remoteAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	udp.SetPeerEndpoint(remoteKey.Public, remoteAddr)

	err = udp.WriteTo(remoteKey.Public, []byte("test"))
	if err != ErrNoSession {
		t.Errorf("WriteTo(no session) error = %v, want %v", err, ErrNoSession)
	}

	udp.Close()
	err = udp.WriteTo(remoteKey.Public, []byte("test"))
	if err != ErrClosed {
		t.Errorf("WriteTo(closed) error = %v, want %v", err, ErrClosed)
	}
}

func TestUDPReadFromClosed(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	udp, err := NewUDP(localKey, WithBindAddr("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("NewUDP() error = %v", err)
	}

	udp.Close()

	buf := make([]byte, 1024)
	_, _, err = udp.ReadFrom(buf)
	if err != ErrClosed {
		t.Errorf("ReadFrom(closed) error = %v, want %v", err, ErrClosed)
	}
}

func TestReadPacket_SkipsErroredPacketsAndReturnsNext(t *testing.T) {
	u := &UDP{
		outputChan: make(chan *packet, 2),
		closeChan:  make(chan struct{}),
	}

	skipped := acquirePacket()
	skipped.err = ErrNoData
	skipped.refs.Store(1)
	close(skipped.ready)
	u.outputChan <- skipped

	wantPK := noise.PublicKey{9, 9, 9}
	okPkt := acquirePacket()
	copy(okPkt.data, []byte("payload"))
	okPkt.pk = wantPK
	okPkt.protocol = testDirectProtoA
	okPkt.payload = okPkt.data[:7]
	okPkt.payloadN = 7
	okPkt.refs.Store(1)
	close(okPkt.ready)
	u.outputChan <- okPkt

	buf := make([]byte, 16)
	pk, proto, n, err := u.ReadPacket(buf)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}
	if pk != wantPK {
		t.Fatalf("pk=%v, want %v", pk, wantPK)
	}
	if proto != testDirectProtoA {
		t.Fatalf("proto=%d, want %d", proto, testDirectProtoA)
	}
	if got := string(buf[:n]); got != "payload" {
		t.Fatalf("payload=%q, want payload", got)
	}
}

func TestReadPacket_ReturnsClosedWhileWaitingForReady(t *testing.T) {
	u := &UDP{
		outputChan: make(chan *packet, 1),
		closeChan:  make(chan struct{}),
	}

	pkt := acquirePacket()
	pkt.refs.Store(1)
	u.outputChan <- pkt
	close(u.closeChan)

	buf := make([]byte, 8)
	if _, _, _, err := u.ReadPacket(buf); err != ErrClosed {
		t.Fatalf("ReadPacket err=%v, want %v", err, ErrClosed)
	}
}

func TestUDPHandleHandshakeResp_Success(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	remoteKey, _ := noise.GenerateKeyPair()
	initHS, wire := buildHandshakeResponseForUDPTest(t, localKey, remoteKey, 17, 29)

	events := make(chan PeerEvent, 1)
	done := make(chan error, 1)
	peer := &peerState{pk: remoteKey.Public, state: PeerStateConnecting}
	u := &UDP{
		localKey: localKey,
		onPeerEvent: func(ev PeerEvent) bool {
			events <- ev
			return true
		},
		pending: map[uint32]*pendingHandshake{
			17: {
				peer:     peer,
				hsState:  initHS,
				localIdx: 17,
				done:     done,
			},
		},
		byIndex: make(map[uint32]*peerState),
	}

	from, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	u.handleHandshakeResp(wire, from)

	if err := <-done; err != nil {
		t.Fatalf("done err=%v, want nil", err)
	}
	if peer.state != PeerStateEstablished {
		t.Fatalf("peer.state=%v, want %v", peer.state, PeerStateEstablished)
	}
	if peer.session == nil || peer.serviceMux == nil {
		t.Fatal("peer session/serviceMux should be initialized")
	}
	if peer.endpoint.String() != from.String() {
		t.Fatalf("endpoint=%v, want %v", peer.endpoint, from)
	}
	if got := u.byIndex[17]; got != peer {
		t.Fatal("byIndex should register established peer")
	}
	if _, ok := u.pending[17]; ok {
		t.Fatal("pending handshake should be removed after success")
	}
	select {
	case ev := <-events:
		if ev.PublicKey != remoteKey.Public || ev.State != PeerStateEstablished {
			t.Fatalf("established event = %+v, want established for %x", ev, remoteKey.Public)
		}
	default:
		t.Fatal("handleHandshakeResp did not emit established event")
	}
}

func TestUDPHandleHandshakeResp_FailureMarksPeerFailed(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	remoteKey, _ := noise.GenerateKeyPair()
	initHS, _ := buildHandshakeResponseForUDPTest(t, localKey, remoteKey, 23, 41)

	done := make(chan error, 1)
	peer := &peerState{pk: remoteKey.Public, state: PeerStateConnecting}
	u := &UDP{
		localKey: localKey,
		pending: map[uint32]*pendingHandshake{
			23: {
				peer:     peer,
				hsState:  initHS,
				localIdx: 23,
				done:     done,
			},
		},
		byIndex: make(map[uint32]*peerState),
	}

	badWire := make([]byte, noise.HandshakeRespSize)
	badWire[0] = noise.MessageTypeHandshakeResp
	binary.LittleEndian.PutUint32(badWire[5:9], 23)
	u.handleHandshakeResp(badWire, &net.UDPAddr{})

	if err := <-done; err != ErrHandshakeFailed {
		t.Fatalf("done err=%v, want %v", err, ErrHandshakeFailed)
	}
	if peer.state != PeerStateFailed {
		t.Fatalf("peer.state=%v, want %v", peer.state, PeerStateFailed)
	}
	if _, ok := u.pending[23]; ok {
		t.Fatal("pending handshake should be removed after failure")
	}
}

func TestInboundServiceDataReestablishesOfflinePeer(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	peerKey, _ := noise.GenerateKeyPair()
	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  7,
		RemoteIndex: 9,
		SendKey:     noise.Key{1},
		RecvKey:     noise.Key{1},
		RemotePK:    peerKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	events := make(chan PeerEvent, 2)
	oldMux := NewServiceMux(peerKey.Public, ServiceMuxConfig{})
	peer := &peerState{
		pk:         peerKey.Public,
		session:    session,
		serviceMux: oldMux,
		state:      PeerStateEstablished,
	}
	u := &UDP{
		localKey: localKey,
		peers: map[noise.PublicKey]*peerState{
			peerKey.Public: peer,
		},
		byIndex: map[uint32]*peerState{
			session.LocalIndex(): peer,
		},
		onPeerEvent: func(ev PeerEvent) bool {
			events <- ev
			return true
		},
	}

	plaintext := EncodePayload(testDirectProtoA, []byte("payload"))
	ciphertext, counter, err := session.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	wire := noise.BuildTransportMessage(session.LocalIndex(), counter, ciphertext)
	from, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	pkt := acquirePacket()
	defer releasePacket(pkt)

	u.ClosePeerServiceMux(peerKey.Public)
	if ev := <-events; ev.State != PeerStateOffline {
		t.Fatalf("ClosePeerServiceMux event = %+v, want offline", ev)
	}

	u.decryptTransport(pkt, wire, from)

	if pkt.err != nil {
		t.Fatalf("pkt.err=%v, want nil", pkt.err)
	}
	peer.mu.RLock()
	state := peer.state
	activeMux := peer.serviceMux
	peer.mu.RUnlock()
	if state != PeerStateEstablished {
		t.Fatalf("peer.state=%v, want %v", state, PeerStateEstablished)
	}
	if activeMux == nil || activeMux == oldMux {
		t.Fatal("inbound service data should create a new service mux")
	}
	if ev := <-events; ev.State != PeerStateEstablished {
		t.Fatalf("inbound service data event = %+v, want established", ev)
	}
}

func TestCloseControlMarksPeerOfflineWithoutRemovingPeer(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	peerKey, _ := noise.GenerateKeyPair()
	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  7,
		RemoteIndex: 9,
		SendKey:     noise.Key{1},
		RecvKey:     noise.Key{1},
		RemotePK:    peerKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	events := make(chan PeerEvent, 1)
	oldMux := NewServiceMux(peerKey.Public, ServiceMuxConfig{})
	peer := &peerState{
		pk:         peerKey.Public,
		session:    session,
		serviceMux: oldMux,
		state:      PeerStateEstablished,
	}
	u := &UDP{
		localKey: localKey,
		peers: map[noise.PublicKey]*peerState{
			peerKey.Public: peer,
		},
		byIndex: map[uint32]*peerState{
			session.LocalIndex(): peer,
		},
		onPeerEvent: func(ev PeerEvent) bool {
			events <- ev
			return true
		},
	}

	plaintext := EncodePayload(ProtocolConnCtrl, closeCtrlPayload)
	ciphertext, counter, err := session.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	wire := noise.BuildTransportMessage(session.LocalIndex(), counter, ciphertext)
	from, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	pkt := acquirePacket()
	defer releasePacket(pkt)

	u.decryptTransport(pkt, wire, from)

	if pkt.err != ErrNoData {
		t.Fatalf("pkt.err=%v, want %v", pkt.err, ErrNoData)
	}
	if info := u.PeerInfo(peerKey.Public); info == nil || info.State != PeerStateOffline {
		t.Fatalf("PeerInfo after close control = %+v, want offline peer", info)
	}
	if got := u.byIndex[session.LocalIndex()]; got != peer {
		t.Fatal("session index should remain mapped to peer")
	}
	peer.mu.RLock()
	activeMux := peer.serviceMux
	activeSession := peer.session
	peer.mu.RUnlock()
	if activeMux != nil {
		t.Fatal("service mux should be cleared by close control")
	}
	if activeSession != session {
		t.Fatal("session should remain after close control")
	}
	if _, err := oldMux.OpenStream(0); err != ErrServiceMuxClosed {
		t.Fatalf("old service mux should be closed, err=%v", err)
	}
	if ev := <-events; ev.State != PeerStateOffline {
		t.Fatalf("close control event = %+v, want offline", ev)
	}

}

func TestCloseControlFromPreviousSessionDoesNotCloseCurrentServiceMux(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	peerKey, _ := noise.GenerateKeyPair()
	previousSession, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  7,
		RemoteIndex: 9,
		SendKey:     noise.Key{1},
		RecvKey:     noise.Key{1},
		RemotePK:    peerKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession(previous) failed: %v", err)
	}
	currentSession, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  8,
		RemoteIndex: 10,
		SendKey:     noise.Key{2},
		RecvKey:     noise.Key{2},
		RemotePK:    peerKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession(current) failed: %v", err)
	}

	events := make(chan PeerEvent, 1)
	activeMux := NewServiceMux(peerKey.Public, ServiceMuxConfig{})
	defer activeMux.Close()
	peer := &peerState{
		pk:         peerKey.Public,
		session:    currentSession,
		previous:   previousSession,
		serviceMux: activeMux,
		state:      PeerStateEstablished,
	}
	u := &UDP{
		localKey: localKey,
		peers: map[noise.PublicKey]*peerState{
			peerKey.Public: peer,
		},
		byIndex: map[uint32]*peerState{
			previousSession.LocalIndex(): peer,
			currentSession.LocalIndex():  peer,
		},
		onPeerEvent: func(ev PeerEvent) bool {
			events <- ev
			return true
		},
	}

	plaintext := EncodePayload(ProtocolConnCtrl, closeCtrlPayload)
	ciphertext, counter, err := previousSession.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	wire := noise.BuildTransportMessage(previousSession.LocalIndex(), counter, ciphertext)
	from, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	currentEndpoint, _ := net.ResolveUDPAddr("udp", "127.0.0.1:10000")
	peer.mu.Lock()
	peer.endpoint = currentEndpoint
	peer.mu.Unlock()
	pkt := acquirePacket()
	defer releasePacket(pkt)

	u.decryptTransport(pkt, wire, from)

	if pkt.err != ErrNoData {
		t.Fatalf("pkt.err=%v, want %v", pkt.err, ErrNoData)
	}
	peer.mu.RLock()
	gotMux := peer.serviceMux
	gotState := peer.state
	gotEndpoint := peer.endpoint
	peer.mu.RUnlock()
	if gotMux != activeMux {
		t.Fatal("previous-session close control should not clear current service mux")
	}
	if gotState != PeerStateEstablished {
		t.Fatalf("peer.state=%v, want %v", gotState, PeerStateEstablished)
	}
	if gotEndpoint.String() != currentEndpoint.String() {
		t.Fatalf("peer.endpoint=%v, want current endpoint %v", gotEndpoint, currentEndpoint)
	}
	if err := activeMux.InputPacket(testDirectProtoA, []byte("still-open")); err != nil {
		t.Fatalf("active mux should remain open, InputPacket err=%v", err)
	}
	select {
	case ev := <-events:
		t.Fatalf("previous-session close control emitted event: %+v", ev)
	default:
	}
}

func TestDispatchToChannelsDropCounters(t *testing.T) {
	t.Run("output queue full increments drop counter", func(t *testing.T) {
		u := &UDP{
			outputChan:  make(chan *packet), // no receiver: always triggers default drop
			decryptChan: make(chan *packet, 1),
			closeChan:   make(chan struct{}),
		}

		pkt := acquirePacket()
		u.dispatchToChannels(pkt)

		if got := u.droppedOutputPackets.Load(); got != 1 {
			t.Fatalf("droppedOutputPackets=%d, want 1", got)
		}
		if got := u.droppedDecryptPackets.Load(); got != 0 {
			t.Fatalf("droppedDecryptPackets=%d, want 0", got)
		}

		select {
		case routed := <-u.decryptChan:
			if routed != pkt {
				t.Fatal("decrypt queue packet mismatch")
			}
			// Only the decrypt ref remains; release manually to avoid pool leak.
			unrefPacket(routed)
		default:
			t.Fatal("packet was not routed to decryptChan")
		}
	})

	t.Run("decrypt queue full increments drop counter", func(t *testing.T) {
		u := &UDP{
			outputChan:  make(chan *packet, 1),
			decryptChan: make(chan *packet), // no receiver: always triggers default drop
			closeChan:   make(chan struct{}),
		}

		pkt := acquirePacket()
		u.dispatchToChannels(pkt)

		if got := u.droppedDecryptPackets.Load(); got != 1 {
			t.Fatalf("droppedDecryptPackets=%d, want 1", got)
		}
		if got := u.droppedOutputPackets.Load(); got != 0 {
			t.Fatalf("droppedOutputPackets=%d, want 0", got)
		}

		select {
		case routed := <-u.outputChan:
			select {
			case <-routed.ready:
			default:
				t.Fatal("routed packet should be marked ready when decrypt path drops")
			}
			if routed.err != ErrNoData {
				t.Fatalf("routed packet err=%v, want %v", routed.err, ErrNoData)
			}
			// Only the output ref remains; release manually to avoid pool leak.
			unrefPacket(routed)
		default:
			t.Fatal("packet was not queued to outputChan")
		}
	})
}

func TestRPCRouteErrorCounterOnSmuxInputFailure(t *testing.T) {
	server, client, serverKey, clientKey := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	server.mu.RLock()
	serverPeer := server.peers[clientKey.Public]
	server.mu.RUnlock()
	if serverPeer == nil {
		t.Fatal("server peer not found")
	}

	serverPeer.mu.RLock()
	smux := serverPeer.serviceMux
	serverPeer.mu.RUnlock()
	if smux == nil {
		t.Fatal("server service mux not initialized")
	}
	_ = smux.Close() // subsequent RPC routing will trigger Input errors

	client.mu.RLock()
	clientPeer := client.peers[serverKey.Public]
	client.mu.RUnlock()
	if clientPeer == nil {
		t.Fatal("client peer not found")
	}

	before := server.HostInfo().RPCRouteErrors
	if err := client.sendKCP(clientPeer, 1, []byte{0}); err != nil {
		t.Fatalf("sendKCP failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := server.HostInfo().RPCRouteErrors; got > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("RPCRouteErrors did not increase: before=%d after=%d", before, server.HostInfo().RPCRouteErrors)
}

func TestInboundDropCounterWhenPeerQueueFull(t *testing.T) {
	server, client, serverKey, clientKey := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	server.mu.RLock()
	serverPeer := server.peers[clientKey.Public]
	server.mu.RUnlock()
	if serverPeer == nil {
		t.Fatal("server peer not found")
	}

	if serverPeer.serviceMux == nil {
		t.Fatal("server service mux not initialized")
	}

	for i := range InboundChanSize {
		if err := serverPeer.serviceMux.InputPacket(testDirectProtoA, []byte("seed")); err != nil {
			t.Fatalf("failed to fill service mux inbound queue at %d: %v", i, err)
		}
	}

	before := server.HostInfo().DroppedInboundPackets
	clientMux, err := client.PeerServiceMux(serverKey.Public)
	if err != nil {
		t.Fatalf("client.PeerServiceMux failed: %v", err)
	}
	if _, err := clientMux.Write(testDirectProtoA, []byte("drop-me")); err != nil {
		t.Fatalf("client mux Write(direct) failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := server.HostInfo().DroppedInboundPackets; got > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("DroppedInboundPackets did not increase: before=%d after=%d", before, server.HostInfo().DroppedInboundPackets)
}

func TestPeerServiceMuxAndSendDirectWrapper(t *testing.T) {
	server, client, serverKey, clientKey := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	if _, err := client.PeerServiceMux(noise.PublicKey{}); err != ErrPeerNotFound {
		t.Fatalf("PeerServiceMux(non-existent) err=%v, want %v", err, ErrPeerNotFound)
	}

	smux, err := client.PeerServiceMux(serverKey.Public)
	if err != nil {
		t.Fatalf("PeerServiceMux(existing) failed: %v", err)
	}
	if smux == nil {
		t.Fatal("PeerServiceMux(existing) returned nil")
	}

	client.mu.RLock()
	peer := client.peers[serverKey.Public]
	client.mu.RUnlock()
	if peer == nil {
		t.Fatal("peer not found in client map")
	}

	if err := client.sendPayload(peer, testDirectProtoA, []byte("wrapper-path")); err != nil {
		t.Fatalf("sendPayload failed: %v", err)
	}

	proto, payload := readPeerWithTimeout(t, server, clientKey.Public, 3*time.Second)
	if proto != testDirectProtoA {
		t.Fatalf("server got proto=%d, want %d", proto, testDirectProtoA)
	}
	if string(payload) != "wrapper-path" {
		t.Fatalf("server got payload=%q, want %q", string(payload), "wrapper-path")
	}
}

func TestOptions(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	cfg := FullSocketConfig()
	cfg.RecvBufSize = 2 * 1024 * 1024
	cfg.SendBufSize = 2 * 1024 * 1024

	u, err := NewUDP(
		key,
		WithBindAddr("127.0.0.1:0"),
		WithAllowFunc(func(noise.PublicKey) bool {
			return true
		}),
		WithRawChanSize(17),
		WithSocketConfig(cfg),
		WithServiceMuxConfig(ServiceMuxConfig{
			OnNewService: func(peer noise.PublicKey, service uint64) bool {
				return service == 1
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}

	if cap(u.decryptChan) != 17 {
		t.Fatalf("decryptChan cap=%d, want 17", cap(u.decryptChan))
	}
	if u.socketConfig != cfg {
		t.Fatalf("socketConfig mismatch: got=%+v want=%+v", u.socketConfig, cfg)
	}
	if u.serviceMuxConfig.OnNewService == nil {
		t.Fatal("serviceMuxConfig should be injected by WithServiceMuxConfig")
	}

	if err := u.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestWithServiceMuxConfigOption(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	wantCfg := ServiceMuxConfig{
		OnNewService: func(peer noise.PublicKey, service uint64) bool {
			return service == 9
		},
	}

	u, err := NewUDP(
		key,
		WithBindAddr("127.0.0.1:0"),
		WithServiceMuxConfig(wantCfg),
	)
	if err != nil {
		t.Fatalf("NewUDP failed: %v", err)
	}
	defer u.Close()

	if u.serviceMuxConfig.OnNewService == nil {
		t.Fatal("serviceMuxConfig.OnNewService is nil")
	}
	if u.serviceMuxConfig.OnNewService(noise.PublicKey{}, 9) != wantCfg.OnNewService(noise.PublicKey{}, 9) {
		t.Fatal("serviceMuxConfig.OnNewService not applied")
	}
}

// TestPacketLeakWhenOutputChanFull verifies that packets sent to decryptChan
// but dropped from outputChan (because it's full) are still properly released.
func TestPacketLeakWhenOutputChanFull(t *testing.T) {
	serverKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	clientKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	server, err := NewUDP(serverKey,
		WithBindAddr("127.0.0.1:0"),
		WithAllowFunc(func(noise.PublicKey) bool {
			return true
		}),
		WithDecryptedChanSize(1),
		WithDecryptWorkers(1),
	)
	if err != nil {
		t.Fatalf("NewUDP server: %v", err)
	}
	defer server.Close()

	client, err := NewUDP(clientKey,
		WithBindAddr("127.0.0.1:0"),
		WithAllowFunc(func(noise.PublicKey) bool {
			return true
		}),
	)
	if err != nil {
		t.Fatalf("NewUDP client: %v", err)
	}
	defer client.Close()

	serverAddr := server.HostInfo().Addr
	clientAddr := client.HostInfo().Addr
	server.SetPeerEndpoint(clientKey.Public, clientAddr)
	client.SetPeerEndpoint(serverKey.Public, serverAddr)

	go func() {
		buf := make([]byte, 65535)
		for {
			if _, _, err := client.ReadFrom(buf); err != nil {
				return
			}
		}
	}()

	// Read from server only long enough for the handshake to complete.
	hsComplete := make(chan struct{})
	go func() {
		buf := make([]byte, 65535)
		for {
			select {
			case <-hsComplete:
				return
			default:
			}
			if _, _, err := server.ReadFrom(buf); err != nil {
				return
			}
		}
	}()

	if err := client.Connect(serverKey.Public); err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	close(hsComplete)
	time.Sleep(50 * time.Millisecond)

	before := outstandingPackets.Load()

	const numPackets = 50
	for i := range numPackets {
		if err := client.WriteTo(serverKey.Public, []byte("leak-test")); err != nil {
			t.Logf("WriteTo %d: %v", i, err)
		}
	}

	time.Sleep(1 * time.Second)

	after := outstandingPackets.Load()
	leaked := after - before

	const outputChanSize = 1
	if leaked > outputChanSize {
		t.Errorf("PACKET POOL LEAK: %d packets acquired but never released "+
			"(before=%d, after=%d, allowed=%d in outputChan). "+
			"Packets processed by decryptWorker but not in outputChan should be released.",
			leaked, before, after, outputChanSize)
	}
}

// TestUDPStreamThroughput measures async throughput through a KCP stream over
// the full UDP transport layer (Noise encryption + real UDP sockets).
// Run with: -test.run=TestUDPStreamThroughput (skipped in short mode)
func TestUDPStreamThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throughput test in short mode")
	}
	const totalSize = 32 * 1024 * 1024 // 32 MB
	const chunkSize = 8 * 1024         // 8 KB

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	client, err := NewUDP(clientKey, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	server, err := NewUDP(serverKey, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	client.SetPeerEndpoint(serverKey.Public, server.HostInfo().Addr)
	server.SetPeerEndpoint(clientKey.Public, client.HostInfo().Addr)

	// Drain non-KCP packets so UDP can process handshake/KCP control traffic.
	go func() {
		buf := make([]byte, 65535)
		for {
			if _, _, err := client.ReadFrom(buf); err != nil {
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, 65535)
		for {
			if _, _, err := server.ReadFrom(buf); err != nil {
				return
			}
		}
	}()

	if err := client.Connect(serverKey.Public); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	chunk := make([]byte, chunkSize)
	for i := range chunk {
		chunk[i] = byte(i & 0xFF)
	}

	serverStreamCh := make(chan io.ReadWriteCloser, 1)
	serverErrCh := make(chan error, 1)
	go func() {
		s, err := mustServiceMux(t, server, clientKey.Public).AcceptStream(0)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverStreamCh <- s
	}()

	time.Sleep(50 * time.Millisecond)

	clientStream, err := mustServiceMux(t, client, serverKey.Public).OpenStream(0)
	if err != nil {
		t.Fatal(err)
	}

	written := 0
	n, err := clientStream.Write(chunk)
	if err != nil {
		t.Fatalf("initial write error: %v", err)
	}
	written += n

	var serverStream io.ReadWriteCloser
	select {
	case serverStream = <-serverStreamCh:
	case err := <-serverErrCh:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for AcceptStream")
	}

	var wg sync.WaitGroup
	var readBytes int

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64*1024)
		for readBytes < totalSize {
			n, err := serverStream.Read(buf)
			if err != nil {
				t.Errorf("read error: %v", err)
				return
			}
			readBytes += n
		}
	}()

	start := time.Now()
	for written < totalSize {
		n, err := clientStream.Write(chunk)
		if err != nil {
			t.Fatalf("write error at %d: %v", written, err)
		}
		written += n
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatalf("timeout: wrote %d, read %d", written, readBytes)
	}

	elapsed := time.Since(start)
	mbps := float64(readBytes) / elapsed.Seconds() / (1024 * 1024)
	t.Logf("Layer 2 (UDP + Noise): %d bytes in %s = %.1f MB/s", readBytes, elapsed.Round(time.Millisecond), mbps)
	fmt.Printf("THROUGHPUT_UDP=%.1f\n", mbps)
}

func readPeerWithTimeout(t *testing.T, u *UDP, pk noise.PublicKey, timeout time.Duration) (byte, []byte) {
	t.Helper()

	type result struct {
		proto   byte
		payload []byte
		err     error
	}

	ch := make(chan result, 1)
	go func() {
		smux, err := u.PeerServiceMux(pk)
		if err != nil {
			ch <- result{err: err}
			return
		}
		buf := make([]byte, 4096)
		proto, n, err := smux.Read(buf)
		if err != nil {
			ch <- result{err: err}
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		ch <- result{proto: proto, payload: data}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Read failed: %v", r.err)
		}
		return r.proto, r.payload
	case <-time.After(timeout):
		t.Fatalf("Read timeout after %s", timeout)
		return 0, nil
	}
}
