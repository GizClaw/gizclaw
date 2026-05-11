package core

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/noise"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/timer"
)

func TestConnStates(t *testing.T) {
	tests := []struct {
		state ConnState
		str   string
	}{
		{ConnStateNew, "new"},
		{ConnStateHandshaking, "handshaking"},
		{ConnStateEstablished, "established"},
		{ConnStateClosed, "closed"},
		{ConnState(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.str {
			t.Errorf("ConnState(%d).String() = %s, want %s", tt.state, tt.state.String(), tt.str)
		}
	}
}

func TestConnClose(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Close should work
	if err := conn.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if conn.State() != ConnStateClosed {
		t.Errorf("State() = %v, want ConnStateClosed", conn.State())
	}

	// Double close should be ok
	if err := conn.Close(); err != nil {
		t.Errorf("Double Close() error = %v", err)
	}
}

func TestSetSession(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	remoteKey, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, err := newConn(localKey, transport, NewMockAddr("remote"), remoteKey.Public)
	if err != nil {
		t.Fatalf("newConn() error = %v", err)
	}
	defer conn.Close()

	if conn.Session() != nil {
		t.Error("Session() should be nil initially")
	}

	idx, _ := noise.GenerateIndex()
	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  idx,
		RemoteIndex: idx + 1,
		SendKey:     [32]byte{1, 2, 3},
		RecvKey:     [32]byte{4, 5, 6},
		RemotePK:    remoteKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	conn.SetSession(session)
	if conn.Session() == nil {
		t.Error("Session() should not be nil after SetSession")
	}
	if conn.State() != ConnStateEstablished {
		t.Errorf("State() = %v, want %v", conn.State(), ConnStateEstablished)
	}
	if conn.LocalIndex() != idx {
		t.Errorf("LocalIndex() = %d, want %d", conn.LocalIndex(), idx)
	}

	conn.SetSession(nil)
	if conn.Session() != nil {
		t.Error("Session() should be nil after SetSession(nil)")
	}
}

func TestConnCloseStopsTickTimer(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	conn.mu.RLock()
	tm := conn.tickTimer
	conn.mu.RUnlock()
	if tm == nil {
		t.Fatal("newConn() did not start tick timer")
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	conn.mu.RLock()
	tm = conn.tickTimer
	conn.mu.RUnlock()
	if tm == nil {
		t.Fatal("Close() removed tick timer")
	}
	if tm.Reset(time.Now()) {
		t.Fatal("Close() did not stop tick timer")
	}
}

func TestConnCloseClearsHandshakeState(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	hs, err := noise.NewHandshakeState(noise.Config{
		Pattern:     noise.PatternIK,
		Initiator:   false,
		LocalStatic: key,
	})
	if err != nil {
		t.Fatalf("NewHandshakeState() error = %v", err)
	}

	conn.mu.Lock()
	conn.hsState = hs
	conn.handshakeStarted = time.Now()
	conn.handshakeAttemptStart = time.Now()
	conn.lastHandshakeSent = time.Now()
	conn.rekeyTriggered = true
	conn.mu.Unlock()

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	conn.mu.RLock()
	hsState := conn.hsState
	handshakeStarted := conn.handshakeStarted
	handshakeAttemptStart := conn.handshakeAttemptStart
	lastHandshakeSent := conn.lastHandshakeSent
	rekeyTriggered := conn.rekeyTriggered
	conn.mu.RUnlock()

	if hsState != nil || !handshakeStarted.IsZero() || !handshakeAttemptStart.IsZero() || !lastHandshakeSent.IsZero() || rekeyTriggered {
		t.Fatal("Close() did not clear handshake state")
	}
}

func TestConnRekeyReturnsClosedAfterClose(t *testing.T) {
	localKey, _ := noise.GenerateKeyPair()
	remoteKey, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(localKey, transport, NewMockAddr("server"), remoteKey.Public)
	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := conn.initiateRekey(); err != ErrConnClosed {
		t.Fatalf("initiateRekey() error = %v, want %v", err, ErrConnClosed)
	}
	if err := conn.retransmitHandshake(); err != ErrConnClosed {
		t.Fatalf("retransmitHandshake() error = %v, want %v", err, ErrConnClosed)
	}
}

func TestConnTickHandshakingZeroTimestampsDisarmsTimer(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	conn.tickTimer.Close()
	var calls atomic.Int64
	conn.tickTimer = timer.New(func() {
		calls.Add(1)
		_ = conn.tick(false)
	})
	defer conn.Close()

	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.mu.Unlock()

	if !conn.tickTimer.Reset(time.Now()) {
		t.Fatal("Reset() returned false before Close")
	}

	deadline := time.After(time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timer callback did not run")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("timer callback count = %d, want 1", got)
	}
}

func TestConnTickCleansUpSessionsAfterCleanupTime(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     noise.Key{1, 2, 3},
		RecvKey:     noise.Key{4, 5, 6},
		RemotePK:    noise.PublicKey{},
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	previous, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  3,
		RemoteIndex: 4,
		SendKey:     noise.Key{7, 8, 9},
		RecvKey:     noise.Key{10, 11, 12},
		RemotePK:    noise.PublicKey{},
	})
	if err != nil {
		t.Fatalf("NewSession() previous error = %v", err)
	}

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	hs, err := noise.NewHandshakeState(noise.Config{
		Pattern:     noise.PatternIK,
		Initiator:   false,
		LocalStatic: key,
	})
	if err != nil {
		t.Fatalf("NewHandshakeState() error = %v", err)
	}
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.previous = previous
	conn.hsState = hs
	conn.handshakeStarted = time.Now()
	conn.handshakeAttemptStart = time.Now()
	conn.lastHandshakeSent = time.Now()
	conn.rekeyTriggered = true
	conn.lastReceived = time.Now().Add(-SessionCleanupTime - time.Second)
	conn.pendingPackets = [][]byte{[]byte("queued")}
	conn.mu.Unlock()

	if err := conn.Tick(); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	conn.mu.RLock()
	state := conn.state
	current := conn.current
	prev := conn.previous
	hsState := conn.hsState
	rekeyTriggered := conn.rekeyTriggered
	handshakeStarted := conn.handshakeStarted
	handshakeAttemptStart := conn.handshakeAttemptStart
	lastHandshakeSent := conn.lastHandshakeSent
	pendingCount := len(conn.pendingPackets)
	conn.mu.RUnlock()

	if state != ConnStateNew {
		t.Fatalf("state = %v, want %v", state, ConnStateNew)
	}
	if current != nil || prev != nil {
		t.Fatal("sessions were not cleaned up")
	}
	if pendingCount != 0 {
		t.Fatalf("pendingPackets count = %d, want 0", pendingCount)
	}
	if hsState != nil || rekeyTriggered || !handshakeStarted.IsZero() || !handshakeAttemptStart.IsZero() || !lastHandshakeSent.IsZero() {
		t.Fatal("handshake state was not cleaned up")
	}
	if session.State() != noise.SessionStateExpired || previous.State() != noise.SessionStateExpired {
		t.Fatal("sessions were not expired")
	}
}

func TestConnTimerCleansUpSessionsAfterCleanupTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		session, err := noise.NewSession(noise.SessionConfig{
			LocalIndex:  1,
			RemoteIndex: 2,
			SendKey:     noise.Key{1, 2, 3},
			RecvKey:     noise.Key{4, 5, 6},
			RemotePK:    noise.PublicKey{},
		})
		if err != nil {
			t.Fatalf("NewSession() error = %v", err)
		}

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		defer conn.Close()

		startTime := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.current = session
		conn.lastReceived = startTime
		conn.lastSent = startTime
		conn.sessionCreated = startTime
		conn.mu.Unlock()

		if err := conn.Tick(); err != nil {
			t.Fatalf("Tick() error = %v", err)
		}

		time.Sleep(SessionCleanupTime + time.Second)
		synctest.Wait()

		conn.mu.RLock()
		state := conn.state
		current := conn.current
		conn.mu.RUnlock()

		if state != ConnStateNew {
			t.Fatalf("state = %v, want %v", state, ConnStateNew)
		}
		if current != nil {
			t.Fatal("session was not cleaned up after timeout")
		}
	})
}

func TestConnSendNotEstablished(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// With the new queuing logic, Send() on a new connection queues the packet
	// and returns ErrNotEstablished since no handshake is in progress
	err := conn.Send(testDirectProtoA, []byte("hello"))
	if err != ErrNotEstablished {
		t.Errorf("Send() error = %v, want ErrNotEstablished", err)
	}

	// Verify the packet was queued
	conn.mu.RLock()
	pendingCount := len(conn.pendingPackets)
	conn.mu.RUnlock()

	if pendingCount != 1 {
		t.Errorf("pendingPackets count = %d, want 1", pendingCount)
	}
}

func TestConnSendNonKCPQueuesWhenNotEstablished(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	err := conn.Send(0x55, []byte("hello"))
	if err != ErrNotEstablished {
		t.Fatalf("Send(0x55) err=%v, want %v", err, ErrNotEstablished)
	}
}

func TestConnRecvNotEstablished(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	_, _, err := conn.Recv()
	if err != ErrNotEstablished {
		t.Errorf("Recv() error = %v, want ErrNotEstablished", err)
	}
}

func TestConnSetRemoteAddr(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Initially nil
	if conn.RemoteAddr() != nil {
		t.Error("RemoteAddr() should be nil initially")
	}

	// Set address
	addr := NewMockAddr("new-addr")
	conn.SetRemoteAddr(addr)

	if conn.RemoteAddr().String() != "new-addr" {
		t.Errorf("RemoteAddr() = %s, want new-addr", conn.RemoteAddr().String())
	}
}

// TestDialAndCommunication tests the full handshake and communication flow using Dial
func TestDialAndCommunication(t *testing.T) {
	// Create key pairs
	initiatorKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() initiator error = %v", err)
	}
	responderKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() responder error = %v", err)
	}

	// Create transports
	initiatorTransport := NewMockTransport("initiator")
	responderTransport := NewMockTransport("responder")
	initiatorTransport.Connect(responderTransport)
	defer initiatorTransport.Close()
	defer responderTransport.Close()

	var wg sync.WaitGroup
	var initiatorConn, responderConn *Conn
	var initiatorErr, responderErr error

	// Start responder listening
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Create responder connection
		responderConn, responderErr = newConn(responderKey, responderTransport, initiatorTransport.LocalAddr(), noise.PublicKey{})
		if responderErr != nil {
			return
		}

		// Receive handshake init
		buf := make([]byte, noise.MaxPacketSize)
		n, _, err := responderTransport.RecvFrom(buf)
		if err != nil {
			responderErr = err
			return
		}

		// Parse and process
		initMsg, err := noise.ParseHandshakeInit(buf[:n])
		if err != nil {
			responderErr = err
			return
		}

		resp, err := responderConn.accept(initMsg)
		if err != nil {
			responderErr = err
			return
		}

		// Send response
		if err := responderTransport.SendTo(resp, initiatorTransport.LocalAddr()); err != nil {
			responderErr = err
			return
		}
	}()

	// Give responder time to start
	time.Sleep(10 * time.Millisecond)

	// Start initiator dial
	wg.Add(1)
	go func() {
		defer wg.Done()
		initiatorConn, initiatorErr = Dial(context.Background(), initiatorTransport, responderTransport.LocalAddr(), responderKey.Public, initiatorKey)
	}()

	wg.Wait()

	if initiatorErr != nil {
		t.Fatalf("Initiator Dial() error = %v", initiatorErr)
	}
	if responderErr != nil {
		t.Fatalf("Responder error = %v", responderErr)
	}

	// Verify connections are established
	if initiatorConn.State() != ConnStateEstablished {
		t.Errorf("Initiator state = %v, want ConnStateEstablished", initiatorConn.State())
	}
	if responderConn.State() != ConnStateEstablished {
		t.Errorf("Responder state = %v, want ConnStateEstablished", responderConn.State())
	}

	// Verify sessions
	if initiatorConn.Session() == nil {
		t.Error("Initiator session is nil")
	}
	if responderConn.Session() == nil {
		t.Error("Responder session is nil")
	}

	// Verify remote public keys
	if initiatorConn.RemotePublicKey() != responderKey.Public {
		t.Error("Initiator remote public key mismatch")
	}
	if responderConn.RemotePublicKey() != initiatorKey.Public {
		t.Error("Responder remote public key mismatch")
	}

	// Test bidirectional communication
	t.Run("Initiator to Responder", func(t *testing.T) {
		testData := []byte("Hello from initiator!")

		// Send from initiator
		if err := initiatorConn.Send(testDirectProtoA, testData); err != nil {
			t.Fatalf("Send() error = %v", err)
		}

		// Receive on responder
		proto, payload, err := responderConn.Recv()
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}

		if proto != testDirectProtoA {
			t.Errorf("protocol = %d, want %d", proto, testDirectProtoA)
		}
		if !bytes.Equal(payload, testData) {
			t.Errorf("payload = %s, want %s", string(payload), string(testData))
		}
	})

	t.Run("Responder to Initiator", func(t *testing.T) {
		testData := []byte("Hello from responder!")

		// Send from responder
		if err := responderConn.Send(testDirectProtoA, testData); err != nil {
			t.Fatalf("Send() error = %v", err)
		}

		// Receive on initiator
		proto, payload, err := initiatorConn.Recv()
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}

		if proto != testDirectProtoA {
			t.Errorf("protocol = %d, want %d", proto, testDirectProtoA)
		}
		if !bytes.Equal(payload, testData) {
			t.Errorf("payload = %s, want %s", string(payload), string(testData))
		}
	})
}

func TestConnMultipleMessages(t *testing.T) {
	// Setup two connected peers
	initiatorKey, _ := noise.GenerateKeyPair()
	responderKey, _ := noise.GenerateKeyPair()

	initiatorTransport := NewMockTransport("initiator")
	responderTransport := NewMockTransport("responder")
	initiatorTransport.Connect(responderTransport)
	defer initiatorTransport.Close()
	defer responderTransport.Close()

	var wg sync.WaitGroup
	var initiatorConn, responderConn *Conn
	var responderErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		responderConn, _ = newConn(responderKey, responderTransport, initiatorTransport.LocalAddr(), noise.PublicKey{})
		buf := make([]byte, noise.MaxPacketSize)
		n, _, err := responderTransport.RecvFrom(buf)
		if err != nil {
			responderErr = err
			return
		}
		initMsg, err := noise.ParseHandshakeInit(buf[:n])
		if err != nil {
			responderErr = err
			return
		}
		resp, err := responderConn.accept(initMsg)
		if err != nil {
			responderErr = err
			return
		}
		if err := responderTransport.SendTo(resp, initiatorTransport.LocalAddr()); err != nil {
			responderErr = err
			return
		}
	}()

	time.Sleep(10 * time.Millisecond)

	var err error
	initiatorConn, err = Dial(context.Background(), initiatorTransport, responderTransport.LocalAddr(), responderKey.Public, initiatorKey)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	wg.Wait()

	if responderErr != nil {
		t.Fatalf("Responder error = %v", responderErr)
	}

	// Send multiple messages
	for i := range 100 {
		// Initiator -> Responder
		msg := []byte("message")
		initiatorConn.Send(testDirectProtoA, msg)

		proto, payload, err := responderConn.Recv()
		if err != nil {
			t.Fatalf("Message %d: Recv() error = %v", i, err)
		}
		if proto != testDirectProtoA || !bytes.Equal(payload, msg) {
			t.Fatalf("Message %d: mismatch", i)
		}

		// Responder -> Initiator
		responderConn.Send(testDirectProtoB, msg)

		proto, payload, err = initiatorConn.Recv()
		if err != nil {
			t.Fatalf("Message %d: Recv() error = %v", i, err)
		}
		if proto != testDirectProtoB || !bytes.Equal(payload, msg) {
			t.Fatalf("Message %d: mismatch", i)
		}
	}
}

// TestConnSendClosed tests Send on a closed connection
func TestConnSendClosed(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	conn.Close()

	err := conn.Send(testDirectProtoA, []byte("hello"))
	if err != ErrConnClosed {
		t.Errorf("Send() error = %v, want ErrConnClosed", err)
	}
}

// TestConnRecvClosed tests Recv on a closed connection
func TestConnRecvClosed(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	conn.Close()

	_, _, err := conn.Recv()
	if err != ErrConnClosed {
		t.Errorf("Recv() error = %v, want ErrConnClosed", err)
	}
}

// TestConnSendKeepaliveNotEstablished tests SendKeepalive when not established
func TestConnSendKeepaliveNotEstablished(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Keepalive should not be queued, should return error immediately
	err := conn.SendKeepalive()
	if err != ErrNotEstablished {
		t.Errorf("SendKeepalive() error = %v, want ErrNotEstablished", err)
	}

	// Verify nothing was queued
	conn.mu.RLock()
	pendingCount := len(conn.pendingPackets)
	conn.mu.RUnlock()

	if pendingCount != 0 {
		t.Errorf("pendingPackets count = %d, want 0 (keepalive should not queue)", pendingCount)
	}
}

// TestConnPendingPacketsFlushed tests that queued packets are sent after handshake
func TestConnPendingPacketsFlushed(t *testing.T) {
	initiatorKey, _ := noise.GenerateKeyPair()
	responderKey, _ := noise.GenerateKeyPair()

	initiatorTransport := NewMockTransport("initiator")
	responderTransport := NewMockTransport("responder")
	initiatorTransport.Connect(responderTransport)
	defer initiatorTransport.Close()
	defer responderTransport.Close()

	// Create initiator conn and queue a packet before establishing
	initiatorConn, _ := newConn(initiatorKey, initiatorTransport, responderTransport.LocalAddr(), responderKey.Public)

	// Queue a packet (will return ErrNotEstablished but packet is queued)
	testData := []byte("queued message")
	err := initiatorConn.Send(testDirectProtoA, testData)
	if err != ErrNotEstablished {
		t.Fatalf("Send() error = %v, want ErrNotEstablished", err)
	}

	// Verify packet is queued
	initiatorConn.mu.RLock()
	pendingCount := len(initiatorConn.pendingPackets)
	initiatorConn.mu.RUnlock()
	if pendingCount != 1 {
		t.Fatalf("pendingPackets = %d, want 1", pendingCount)
	}

	// Now establish connection manually
	var wg sync.WaitGroup
	var responderConn *Conn
	var responderErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		responderConn, _ = newConn(responderKey, responderTransport, initiatorTransport.LocalAddr(), noise.PublicKey{})
		buf := make([]byte, noise.MaxPacketSize)
		n, _, err := responderTransport.RecvFrom(buf)
		if err != nil {
			responderErr = err
			return
		}
		initMsg, _ := noise.ParseHandshakeInit(buf[:n])
		resp, _ := responderConn.accept(initMsg)
		responderTransport.SendTo(resp, initiatorTransport.LocalAddr())
	}()

	// Perform handshake on initiator side
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  initiatorKey,
		RemoteStatic: &responderKey.Public,
	})
	msg1, _ := hs.WriteMessage(nil)
	wireMsg := noise.BuildHandshakeInit(initiatorConn.localIdx, hs.LocalEphemeral(), msg1[noise.KeySize:])
	initiatorTransport.SendTo(wireMsg, responderTransport.LocalAddr())

	// Wait for responder
	wg.Wait()
	if responderErr != nil {
		t.Fatalf("Responder error = %v", responderErr)
	}

	// Receive handshake response
	buf := make([]byte, noise.MaxPacketSize)
	n, _, _ := initiatorTransport.RecvFrom(buf)
	respMsg, _ := noise.ParseHandshakeResp(buf[:n])

	// Complete initiator handshake
	noiseResp := make([]byte, noise.KeySize+16)
	copy(noiseResp[:noise.KeySize], respMsg.Ephemeral[:])
	copy(noiseResp[noise.KeySize:], respMsg.Empty)
	hs.ReadMessage(noiseResp)

	initiatorConn.mu.Lock()
	initiatorConn.hsState = hs
	initiatorConn.mu.Unlock()

	// Complete handshake (this should flush pending packets)
	initiatorConn.completeHandshake(respMsg.SenderIndex, nil)

	// Give time for flush
	time.Sleep(50 * time.Millisecond)

	// Verify pending packets were flushed
	initiatorConn.mu.RLock()
	pendingAfter := len(initiatorConn.pendingPackets)
	initiatorConn.mu.RUnlock()
	if pendingAfter != 0 {
		t.Errorf("pendingPackets after flush = %d, want 0", pendingAfter)
	}

	// Responder should receive the queued message
	proto, payload, err := responderConn.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if proto != testDirectProtoA || !bytes.Equal(payload, testData) {
		t.Errorf("Received wrong data: proto=%d, payload=%s", proto, string(payload))
	}
}

// TestConnLastSentLastReceived tests the timestamp getters
func TestConnLastSentLastReceived(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Timestamps should be set at creation
	lastSent := conn.LastSent()
	lastReceived := conn.LastReceived()

	if time.Since(lastSent) > time.Second {
		t.Error("LastSent() not initialized correctly")
	}
	if time.Since(lastReceived) > time.Second {
		t.Error("LastReceived() not initialized correctly")
	}
}

// TestConnPreviousSessionDecrypt tests decryption with previous session
func TestConnPreviousSessionDecrypt(t *testing.T) {
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	// Create two sessions with different indices
	sendKey1 := [32]byte{1, 2, 3}
	recvKey1 := [32]byte{4, 5, 6}
	sendKey2 := [32]byte{7, 8, 9}
	recvKey2 := [32]byte{10, 11, 12}

	prevSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey1,
		RecvKey:     recvKey1,
		RemotePK:    serverKey.Public,
	})

	currSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  101,
		RemoteIndex: 201,
		SendKey:     sendKey2,
		RecvKey:     recvKey2,
		RemotePK:    serverKey.Public,
	})

	// Create conn with both sessions
	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = currSession
	conn.previous = prevSession
	conn.mu.Unlock()

	// Create a message encrypted with the "previous" session's send key
	// We need to create a peer session that sends to our prevSession
	peerSendKey := recvKey1 // Peer's send = our recv
	peerRecvKey := sendKey1
	peerSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  200,
		RemoteIndex: 100, // Sends to our prevSession's local index
		SendKey:     peerSendKey,
		RecvKey:     peerRecvKey,
		RemotePK:    clientKey.Public,
	})

	// Encrypt a message
	testData := []byte("message from previous session")
	plaintext := EncodePayload(testDirectProtoA, testData)
	ciphertext, counter, _ := peerSession.Encrypt(plaintext)
	wireMsg := noise.BuildTransportMessage(100, counter, ciphertext) // ReceiverIndex = our prev session

	// Inject the message into CLIENT transport (that's where conn reads from)
	clientTransport.InjectPacket(wireMsg, serverTransport.LocalAddr())

	// Receive should work with previous session
	proto, payload, err := conn.Recv()
	if err != nil {
		t.Fatalf("Recv() with previous session error = %v", err)
	}
	if proto != testDirectProtoA || !bytes.Equal(payload, testData) {
		t.Errorf("Received wrong data from previous session")
	}
}

// TestConnInvalidReceiverIndex tests receiving message with wrong index
func TestConnInvalidReceiverIndex(t *testing.T) {
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.mu.Unlock()

	// Create a properly encrypted message but with wrong receiver index
	// We need a fake "peer" session to encrypt
	fakePeerSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  999,     // Their local
		RemoteIndex: 888,     // Wrong index - doesn't match our session
		SendKey:     recvKey, // Reversed keys
		RecvKey:     sendKey,
		RemotePK:    clientKey.Public,
	})

	ciphertext, counter, _ := fakePeerSession.Encrypt([]byte("test"))
	wireMsg := noise.BuildTransportMessage(888, counter, ciphertext) // Wrong receiver index

	// Inject into CLIENT transport (where conn reads from)
	clientTransport.InjectPacket(wireMsg, serverTransport.LocalAddr())

	_, _, err := conn.Recv()
	if err != ErrInvalidReceiverIndex {
		t.Errorf("Recv() error = %v, want ErrInvalidReceiverIndex", err)
	}
}

// TestConnDeliverPacket tests the deliverPacket method for listener-managed connections
func TestConnDeliverPacket(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(key, transport, nil, serverKey.Public)

	// Create inbound channel
	inbound := make(chan inboundPacket, 10)
	conn.setInbound(inbound)

	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.mu.Unlock()

	// Create a mock transport message
	msg := &noise.TransportMessage{
		ReceiverIndex: 1,
		Counter:       0,
		Ciphertext:    []byte("test"),
	}

	// Deliver should succeed
	addr := NewMockAddr("test")
	ok := conn.deliverPacket(msg, addr)
	if !ok {
		t.Error("deliverPacket() returned false, want true")
	}

	// Verify message was delivered
	select {
	case pkt := <-inbound:
		if pkt.msg != msg {
			t.Error("Delivered message mismatch")
		}
		if pkt.addr != addr {
			t.Error("Delivered address mismatch")
		}
	default:
		t.Error("No message in inbound channel")
	}
}

// TestConnDeliverPacketClosed tests deliverPacket on closed connection
func TestConnDeliverPacketClosed(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	inbound := make(chan inboundPacket, 10)
	conn.setInbound(inbound)

	// Close connection
	conn.Close()

	msg := &noise.TransportMessage{}
	ok := conn.deliverPacket(msg, nil)
	if ok {
		t.Error("deliverPacket() on closed conn returned true, want false")
	}
}

// TestConnDeliverPacketNoInbound tests deliverPacket without inbound channel
func TestConnDeliverPacketNoInbound(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	// No inbound channel set

	msg := &noise.TransportMessage{}
	ok := conn.deliverPacket(msg, nil)
	if ok {
		t.Error("deliverPacket() without inbound returned true, want false")
	}
}

// TestConnDeliverPacketChannelFull tests deliverPacket when channel is full
func TestConnDeliverPacketChannelFull(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Create small channel and fill it
	inbound := make(chan inboundPacket, 1)
	conn.setInbound(inbound)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.mu.Unlock()

	// Fill the channel
	inbound <- inboundPacket{}

	// Next delivery should fail (channel full)
	msg := &noise.TransportMessage{}
	ok := conn.deliverPacket(msg, nil)
	if ok {
		t.Error("deliverPacket() on full channel returned true, want false")
	}
}

// TestConnRecvInvalidMessageType tests Recv with unknown message type
func TestConnRecvInvalidMessageType(t *testing.T) {
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.mu.Unlock()

	// Inject packet with unknown message type
	invalidMsg := make([]byte, 20)
	invalidMsg[0] = 99 // Unknown type
	clientTransport.InjectPacket(invalidMsg, serverTransport.LocalAddr())

	_, _, err := conn.Recv()
	if err != noise.ErrInvalidMessageType {
		t.Errorf("Recv() error = %v, want ErrInvalidMessageType", err)
	}
}

// TestConnRecvKeepalive tests receiving empty keepalive message
func TestConnRecvKeepalive(t *testing.T) {
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	clientSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = clientSession
	conn.mu.Unlock()

	// Create peer session to send keepalive
	peerSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  200,
		RemoteIndex: 100,
		SendKey:     recvKey, // Reversed
		RecvKey:     sendKey,
		RemotePK:    clientKey.Public,
	})

	// Send empty keepalive
	ciphertext, counter, _ := peerSession.Encrypt(nil) // Empty payload
	wireMsg := noise.BuildTransportMessage(100, counter, ciphertext)
	clientTransport.InjectPacket(wireMsg, serverTransport.LocalAddr())

	proto, payload, err := conn.Recv()
	if err != nil {
		t.Fatalf("Recv() keepalive error = %v", err)
	}

	// Keepalive returns 0, nil, nil
	if proto != 0 || payload != nil {
		t.Errorf("Keepalive: proto=%d, payload=%v, want 0, nil", proto, payload)
	}
}

// TestConnFailHandshake tests the failHandshake helper
func TestConnFailHandshake(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Set handshaking state
	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.mu.Unlock()

	// Call failHandshake
	testErr := ErrHandshakeTimeout
	err := conn.failHandshake(testErr)

	if err != testErr {
		t.Errorf("failHandshake() returned %v, want %v", err, testErr)
	}

	// Verify state was reset
	conn.mu.RLock()
	state := conn.state
	hsState := conn.hsState
	conn.mu.RUnlock()

	if state != ConnStateNew {
		t.Errorf("State after failHandshake = %v, want ConnStateNew", state)
	}
	if hsState != nil {
		t.Error("hsState should be nil after failHandshake")
	}
}

// TestNewConnErrors tests error conditions in newConn
func TestNewConnErrors(t *testing.T) {
	t.Run("NilLocalKey", func(t *testing.T) {
		transport := NewMockTransport("test")
		defer transport.Close()

		_, err := newConn(nil, transport, nil, noise.PublicKey{})
		if err != ErrMissingLocalKey {
			t.Errorf("newConn() error = %v, want ErrMissingLocalKey", err)
		}
	})

	t.Run("NilTransport", func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()

		_, err := newConn(key, nil, nil, noise.PublicKey{})
		if err != ErrMissingTransport {
			t.Errorf("newConn() error = %v, want ErrMissingTransport", err)
		}
	})
}

// TestConnAcceptErrors tests error conditions in accept
func TestConnAcceptErrors(t *testing.T) {
	t.Run("NotNewState", func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		conn.mu.Lock()
		conn.state = ConnStateEstablished // Not new
		conn.mu.Unlock()

		msg := &noise.HandshakeInitMessage{}
		_, err := conn.accept(msg)
		if err != ErrInvalidConnState {
			t.Errorf("accept() error = %v, want ErrInvalidConnState", err)
		}
	})
}

// TestConnSendWithRekeyTrigger tests that Send triggers rekey when session is old
func TestConnSendWithRekeyTrigger(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second) // Old session
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.mu.Unlock()

	// Send should succeed and trigger rekey
	err := conn.Send(testDirectProtoA, []byte("test"))
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Give time for rekey goroutine
	time.Sleep(50 * time.Millisecond)

	conn.mu.RLock()
	rekeyTriggered := conn.rekeyTriggered
	conn.mu.RUnlock()

	if !rekeyTriggered {
		t.Error("Send() should have triggered rekey for old session")
	}
}

func TestConnConcurrentTickDoesNotDuplicateHandshakeInit(t *testing.T) {
	const goroutines = 16

	transport := newTickProofTransport(2, 200*time.Millisecond)
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     noise.Key{1, 2, 3},
		RecvKey:     noise.Key{4, 5, 6},
		RemotePK:    serverKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	conn, err := newConn(clientKey, transport, NewMockAddr("server"), serverKey.Public)
	if err != nil {
		t.Fatalf("newConn() error = %v", err)
	}
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.mu.Unlock()

	start := make(chan struct{})
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			<-start
			errs <- conn.tick(true)
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("tick(true) error = %v", err)
		}
	}

	if got := transport.handshakeInitWrites.Load(); got != 1 {
		t.Fatalf("concurrent tick(true) sent %d handshake init packets, want 1", got)
	}
}

type tickProofTransport struct {
	localAddr *MockAddr

	releaseAfter int64
	wait         time.Duration
	releaseOnce  sync.Once
	release      chan struct{}

	handshakeInitWrites atomic.Int64
}

func newTickProofTransport(releaseAfter int64, wait time.Duration) *tickProofTransport {
	return &tickProofTransport{
		localAddr:    NewMockAddr("tick-proof"),
		releaseAfter: releaseAfter,
		wait:         wait,
		release:      make(chan struct{}),
	}
}

func (t *tickProofTransport) ReadFrom(_ []byte) (int, net.Addr, error) {
	return 0, nil, errors.New("ReadFrom not implemented")
}

func (t *tickProofTransport) WriteTo(data []byte, _ net.Addr) (int, error) {
	msgType, err := noise.GetMessageType(data)
	if err == nil && msgType == noise.MessageTypeHandshakeInit {
		if t.handshakeInitWrites.Add(1) >= t.releaseAfter {
			t.releaseOnce.Do(func() {
				close(t.release)
			})
		}

		select {
		case <-t.release:
		case <-time.After(t.wait):
			t.releaseOnce.Do(func() {
				close(t.release)
			})
		}
	}

	return len(data), nil
}

func (t *tickProofTransport) Close() error {
	t.releaseOnce.Do(func() {
		close(t.release)
	})
	return nil
}

func (t *tickProofTransport) LocalAddr() net.Addr {
	return t.localAddr
}

func (t *tickProofTransport) SetDeadline(_ time.Time) error {
	return nil
}

func (t *tickProofTransport) SetReadDeadline(_ time.Time) error {
	return nil
}

func (t *tickProofTransport) SetWriteDeadline(_ time.Time) error {
	return nil
}

// TestConnSendQueuedWhenHandshaking tests Send queues when handshaking
func TestConnSendQueuedWhenHandshaking(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(key, transport, nil, serverKey.Public)

	// Create handshake state
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  key,
		RemoteStatic: &serverKey.Public,
	})

	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = hs
	conn.mu.Unlock()

	// Send should queue the packet (handshaking but no session)
	err := conn.Send(testDirectProtoA, []byte("queued"))
	// Should not error, packet is queued
	if err != nil && err != ErrNotEstablished {
		t.Errorf("Send() error = %v", err)
	}

	conn.mu.RLock()
	pendingCount := len(conn.pendingPackets)
	conn.mu.RUnlock()

	if pendingCount != 1 {
		t.Errorf("pendingPackets = %d, want 1", pendingCount)
	}
}

// TestConnCompleteHandshakeErrors tests error conditions in completeHandshake
func TestConnCompleteHandshakeErrors(t *testing.T) {
	t.Run("NoHsState", func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		conn.mu.Lock()
		conn.hsState = nil
		conn.mu.Unlock()

		err := conn.completeHandshake(1, nil)
		if err != ErrHandshakeIncomplete {
			t.Errorf("completeHandshake() error = %v, want ErrHandshakeIncomplete", err)
		}
	})

	t.Run("HandshakeNotFinished", func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		serverKey, _ := noise.GenerateKeyPair()

		conn, _ := newConn(key, transport, nil, serverKey.Public)

		// Create handshake state but don't complete it
		hs, _ := noise.NewHandshakeState(noise.Config{
			Pattern:      noise.PatternIK,
			Initiator:    true,
			LocalStatic:  key,
			RemoteStatic: &serverKey.Public,
		})

		conn.mu.Lock()
		conn.hsState = hs // Not finished
		conn.mu.Unlock()

		err := conn.completeHandshake(1, nil)
		if err != ErrHandshakeIncomplete {
			t.Errorf("completeHandshake() error = %v, want ErrHandshakeIncomplete", err)
		}
	})
}

// TestConnInitiateRekeyErrors tests error conditions in initiateRekey
func TestConnInitiateRekeyErrors(t *testing.T) {
	t.Run("AlreadyHasHsState", func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		serverKey, _ := noise.GenerateKeyPair()

		conn, _ := newConn(key, transport, nil, serverKey.Public)

		// Create existing handshake state
		hs, _ := noise.NewHandshakeState(noise.Config{
			Pattern:      noise.PatternIK,
			Initiator:    true,
			LocalStatic:  key,
			RemoteStatic: &serverKey.Public,
		})

		conn.mu.Lock()
		conn.hsState = hs // Already has one
		conn.mu.Unlock()

		// Should return nil (no-op)
		err := conn.initiateRekey()
		if err != nil {
			t.Errorf("initiateRekey() error = %v, want nil", err)
		}
	})
}

// TestConnRecvWithHandshakeState tests Recv dispatching to different handlers
func TestConnRecvWithHandshakeState(t *testing.T) {
	// Test that Recv properly handles various message types
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	// Create established connection with pending rekey
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Create pending handshake state for rekey
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})
	hs.WriteMessage(nil) // Move to waiting for response

	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.hsState = hs
	conn.localIdx = 100
	conn.mu.Unlock()

	// Test receiving transport message (should work with existing session)
	peerSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  200,
		RemoteIndex: 100,
		SendKey:     recvKey, // Reversed
		RecvKey:     sendKey,
		RemotePK:    clientKey.Public,
	})

	testData := []byte("test message")
	plaintext := EncodePayload(testDirectProtoA, testData)
	ciphertext, counter, _ := peerSession.Encrypt(plaintext)
	wireMsg := noise.BuildTransportMessage(100, counter, ciphertext)
	clientTransport.InjectPacket(wireMsg, serverTransport.LocalAddr())

	proto, payload, err := conn.Recv()
	if err != nil {
		t.Fatalf("Recv() transport message error = %v", err)
	}
	if proto != testDirectProtoA || !bytes.Equal(payload, testData) {
		t.Error("Transport message mismatch")
	}
}

// TestConnHandleTransportMessageRekeyTrigger tests rekey trigger on old session receive
func TestConnHandleTransportMessageRekeyTrigger(t *testing.T) {
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	// Session older than RekeyOnRecvThreshold (165s)
	conn.sessionCreated = time.Now().Add(-RekeyOnRecvThreshold - time.Second)
	conn.lastReceived = time.Now()
	conn.mu.Unlock()

	// Receive a message
	peerSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  200,
		RemoteIndex: 100,
		SendKey:     recvKey,
		RecvKey:     sendKey,
		RemotePK:    clientKey.Public,
	})

	testData := []byte("test")
	plaintext := EncodePayload(testDirectProtoA, testData)
	ciphertext, counter, _ := peerSession.Encrypt(plaintext)
	wireMsg := noise.BuildTransportMessage(100, counter, ciphertext)
	clientTransport.InjectPacket(wireMsg, serverTransport.LocalAddr())

	_, _, err := conn.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	// Give time for rekey goroutine
	time.Sleep(50 * time.Millisecond)

	// Should have triggered rekey
	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	conn.mu.RUnlock()

	if !hasHsState {
		t.Error("Should have triggered rekey on receive with old session")
	}
}

// TestConnAcceptInvalidHandshake tests accept with invalid handshake data
func TestConnAcceptInvalidHandshake(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Create invalid handshake init message with garbage data
	invalidMsg := &noise.HandshakeInitMessage{
		SenderIndex: 1,
		Ephemeral:   noise.Key{}, // Empty ephemeral
		Static:      make([]byte, 48),
	}

	// Accept should fail due to invalid handshake data
	_, err := conn.accept(invalidMsg)
	if err == nil {
		t.Error("accept() with invalid data should error")
	}

	// State should be reset to new
	conn.mu.RLock()
	state := conn.state
	conn.mu.RUnlock()

	if state != ConnStateNew {
		t.Errorf("State after failed accept = %v, want ConnStateNew", state)
	}
}

// TestConnRetransmitHandshake tests the retransmitHandshake function
func TestConnRetransmitHandshake(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Create initial handshake state
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})

	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = hs
	conn.localIdx = 123
	conn.mu.Unlock()

	// Call retransmitHandshake
	err := conn.retransmitHandshake()
	if err != nil {
		t.Fatalf("retransmitHandshake() error = %v", err)
	}

	// Verify lastHandshakeSent was updated
	conn.mu.RLock()
	lastSent := conn.lastHandshakeSent
	newHsState := conn.hsState
	conn.mu.RUnlock()

	if time.Since(lastSent) > time.Second {
		t.Error("lastHandshakeSent not updated")
	}

	// hsState should be new (fresh ephemeral keys)
	if newHsState == nil {
		t.Error("hsState should not be nil after retransmit")
	}
}

// TestConnRetransmitHandshakeNoHsState tests retransmit with nil hsState
func TestConnRetransmitHandshakeNoHsState(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	conn.mu.Lock()
	conn.hsState = nil
	conn.mu.Unlock()

	// Should return nil (no-op)
	err := conn.retransmitHandshake()
	if err != nil {
		t.Errorf("retransmitHandshake() with nil hsState error = %v", err)
	}
}

// TestConnHandleHandshakeInitSuccess tests successful handleHandshakeInit
func TestConnHandleHandshakeInitSuccess(t *testing.T) {
	serverKey, _ := noise.GenerateKeyPair()
	clientKey, _ := noise.GenerateKeyPair()

	serverTransport := NewMockTransport("server")
	clientTransport := NewMockTransport("client")
	serverTransport.Connect(clientTransport)
	defer serverTransport.Close()
	defer clientTransport.Close()

	// Create server conn that knows about client
	serverConn, _ := newConn(serverKey, serverTransport, clientTransport.LocalAddr(), clientKey.Public)
	serverConn.mu.Lock()
	serverConn.state = ConnStateEstablished
	serverConn.mu.Unlock()

	// Client creates handshake init
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})
	msg1, _ := hs.WriteMessage(nil)
	wireInit := noise.BuildHandshakeInit(999, hs.LocalEphemeral(), msg1[noise.KeySize:])
	initMsg, _ := noise.ParseHandshakeInit(wireInit)

	// Server handles init
	_, _, err := serverConn.handleHandshakeInit(initMsg, clientTransport.LocalAddr())
	if err != nil {
		t.Fatalf("handleHandshakeInit() error = %v", err)
	}

	// Verify server state
	serverConn.mu.RLock()
	state := serverConn.state
	isInitiator := serverConn.isInitiator
	current := serverConn.current
	serverConn.mu.RUnlock()

	if state != ConnStateEstablished {
		t.Errorf("State = %v, want ConnStateEstablished", state)
	}
	if isInitiator {
		t.Error("isInitiator should be false for responder")
	}
	if current == nil {
		t.Error("current session should not be nil")
	}
}

// TestConnSendMessageCountRekey tests rekey triggered by message count
func TestConnSendMessageCountRekey(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.sessionCreated = time.Now() // Recent session (no time-based rekey)
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.mu.Unlock()

	// Manually set session counter near RekeyAfterMessages threshold
	// This would normally be done by many encryptions
	// Just test that the logic path exists

	// Send a message
	err := conn.Send(testDirectProtoA, []byte("test"))
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}
}

// TestConnRecvDecryptError tests Recv with decryption failure
func TestConnRecvDecryptError(t *testing.T) {
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  100,
		RemoteIndex: 200,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.mu.Unlock()

	// Create message with wrong key (will fail decryption)
	wrongKey := [32]byte{9, 9, 9}
	wrongSession, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  200,
		RemoteIndex: 100,
		SendKey:     wrongKey, // Wrong key
		RecvKey:     wrongKey,
		RemotePK:    clientKey.Public,
	})

	ciphertext, counter, _ := wrongSession.Encrypt([]byte("test"))
	wireMsg := noise.BuildTransportMessage(100, counter, ciphertext)
	clientTransport.InjectPacket(wireMsg, serverTransport.LocalAddr())

	_, _, err := conn.Recv()
	if err == nil {
		t.Error("Recv() with wrong key should error")
	}
}

func TestTickNewConn(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}
}

func TestTickClosedConn(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})
	conn.Close()

	err := conn.Tick()
	if err != ErrConnClosed {
		t.Errorf("Tick() error = %v, want ErrConnClosed", err)
	}
}

func TestTickKeepalive(t *testing.T) {
	// Create two connected transports
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	// Create a mock session for the client
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, err := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Manually set connection to established state with an old lastSent time
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.lastSent = time.Now().Add(-KeepaliveTimeout - time.Second)
	conn.lastReceived = time.Now() // Recent receive, so passive keepalive should trigger
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	// Tick should send a keepalive
	err = conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Verify lastSent was updated (keepalive was sent)
	conn.mu.RLock()
	lastSent := conn.lastSent
	conn.mu.RUnlock()

	if time.Since(lastSent) > time.Second {
		t.Error("Tick() did not update lastSent, keepalive may not have been sent")
	}
}

func TestTickNoKeepaliveWhenNoRecentReceive(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	// Create a mock session
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}
	remotePK := noise.PublicKey{}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    remotePK,
	})

	conn, _ := newConn(key, transport, nil, remotePK)

	// Set both lastSent and lastReceived to old times
	// Passive keepalive only triggers if we've received recently but not sent
	oldTime := time.Now().Add(-KeepaliveTimeout - time.Second)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.lastSent = oldTime
	conn.lastReceived = oldTime // No recent receive, so no passive keepalive
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	originalLastSent := oldTime
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// lastSent should NOT have been updated (no keepalive sent)
	conn.mu.RLock()
	lastSent := conn.lastSent
	conn.mu.RUnlock()

	if lastSent != originalLastSent {
		t.Error("Tick() sent keepalive when it shouldn't have (no recent receive)")
	}
}

func TestTickTimeout(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Manually set connection to established state with old lastReceived
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now().Add(-RejectAfterTime - time.Second)
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	err := conn.Tick()
	if err != ErrConnTimeout {
		t.Errorf("Tick() error = %v, want ErrConnTimeout", err)
	}
}

func TestTickHandshakeTimeout(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Manually set connection to handshaking state with expired attempt
	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.handshakeAttemptStart = time.Now().Add(-RekeyAttemptTime - time.Second)
	conn.mu.Unlock()

	err := conn.Tick()
	if err != ErrHandshakeTimeout {
		t.Errorf("Tick() error = %v, want ErrHandshakeTimeout", err)
	}
}

func TestTickNoAction(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	// Create a mock session
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}
	remotePK := noise.PublicKey{}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    remotePK,
	})

	conn, _ := newConn(key, transport, nil, remotePK)

	// Set all timestamps to recent
	now := time.Now()
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.lastSent = now
	conn.lastReceived = now
	conn.sessionCreated = now
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}
}

func TestTickRekeyTrigger(t *testing.T) {
	// Create two connected transports
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	// Create a mock session for the client
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Set as initiator with old session (past RekeyAfterTime)
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
	conn.mu.Unlock()

	// Tick should trigger rekey (but won't error since it initiates in background)
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Give the rekey goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	// Check that a handshake state was created
	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	rekeyTriggered := conn.rekeyTriggered
	conn.mu.RUnlock()

	if !hasHsState {
		t.Error("Tick() did not trigger rekey (hsState is nil)")
	}
	if !rekeyTriggered {
		t.Error("Tick() did not set rekeyTriggered")
	}
}

func TestTickResponderNoRekey(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	// Create a mock session
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}
	remotePK := noise.PublicKey{}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    remotePK,
	})

	conn, _ := newConn(key, transport, nil, remotePK)

	// Set as responder (not initiator) with old session
	// Responders should NOT trigger rekey, only initiators
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = false // Responder
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Responder should not have triggered rekey
	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	conn.mu.RUnlock()

	if hasHsState {
		t.Error("Responder should not trigger rekey")
	}
}

// TestTickHandshakeRetransmit tests that Tick retransmits handshake after RekeyTimeout
func TestTickHandshakeRetransmit(t *testing.T) {
	// Create two connected transports
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Create initial handshake state
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})

	// Set connection in handshaking state with old lastHandshakeSent
	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = hs
	conn.handshakeAttemptStart = time.Now()
	conn.lastHandshakeSent = time.Now().Add(-RekeyTimeout - time.Second) // Past retransmit time
	conn.mu.Unlock()

	// Tick should trigger retransmit
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Verify lastHandshakeSent was updated (retransmit happened)
	conn.mu.RLock()
	lastHandshakeSent := conn.lastHandshakeSent
	conn.mu.RUnlock()

	if time.Since(lastHandshakeSent) > time.Second {
		t.Error("Tick() did not retransmit handshake")
	}
}

// TestTickEstablishedWithPendingRekey tests Tick behavior when established but rekey in progress
func TestTickEstablishedWithPendingRekey(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	// Create a mock session
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Create handshake state for pending rekey
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})

	// Set established state with pending rekey that needs retransmit
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.hsState = hs // Pending rekey
	conn.handshakeAttemptStart = time.Now()
	conn.lastHandshakeSent = time.Now().Add(-RekeyTimeout - time.Second)
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Verify retransmit happened
	conn.mu.RLock()
	lastHandshakeSent := conn.lastHandshakeSent
	conn.mu.RUnlock()

	if time.Since(lastHandshakeSent) > time.Second {
		t.Error("Tick() did not retransmit during established rekey")
	}
}

// TestTickEstablishedRekeyTimeout tests that rekey timeout is detected in established state
func TestTickEstablishedRekeyTimeout(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	serverKey, _ := noise.GenerateKeyPair()

	// Create a mock session
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(key, transport, nil, serverKey.Public)

	// Create handshake state for pending rekey
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  key,
		RemoteStatic: &serverKey.Public,
	})

	// Set established state with expired rekey attempt
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.hsState = hs
	conn.handshakeAttemptStart = time.Now().Add(-RekeyAttemptTime - time.Second) // Expired
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	err := conn.Tick()
	if err != ErrHandshakeTimeout {
		t.Errorf("Tick() error = %v, want ErrHandshakeTimeout", err)
	}
}

// TestTickHandshakingRetransmit tests retransmit in handshaking state
func TestTickHandshakingRetransmit(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Create handshake state
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})

	// Set handshaking state needing retransmit
	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = hs
	conn.handshakeAttemptStart = time.Now()
	conn.lastHandshakeSent = time.Now().Add(-RekeyTimeout - time.Second)
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Verify retransmit
	conn.mu.RLock()
	lastSent := conn.lastHandshakeSent
	conn.mu.RUnlock()

	if time.Since(lastSent) > time.Second {
		t.Error("Tick() did not retransmit in handshaking state")
	}
}

// TestTickHandshakingNoRetransmitYet tests no retransmit before RekeyTimeout
func TestTickHandshakingNoRetransmitYet(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(key, transport, nil, serverKey.Public)

	// Create handshake state
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  key,
		RemoteStatic: &serverKey.Public,
	})

	originalTime := time.Now().Add(-time.Second) // Recent, not needing retransmit

	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = hs
	conn.handshakeAttemptStart = time.Now()
	conn.lastHandshakeSent = originalTime
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Should not have retransmitted
	conn.mu.RLock()
	lastSent := conn.lastHandshakeSent
	conn.mu.RUnlock()

	if lastSent != originalTime {
		t.Error("Tick() retransmitted too early")
	}
}

// TestTickInvalidState tests Tick with an invalid state
func TestTickInvalidState(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Set invalid state
	conn.mu.Lock()
	conn.state = ConnState(99) // Invalid state
	conn.mu.Unlock()

	err := conn.Tick()
	if err != ErrInvalidConnState {
		t.Errorf("Tick() error = %v, want ErrInvalidConnState", err)
	}
}

// TestTickHandshakingWithoutHsState tests handshaking state but no hsState set
func TestTickHandshakingWithoutHsState(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	// Set handshaking state but no hsState
	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = nil // No handshake state
	conn.handshakeAttemptStart = time.Now()
	conn.mu.Unlock()

	// Should not error, just nothing to retransmit
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}
}

// TestTickHandshakingWithZeroTimestamps tests handshaking with zero timestamps
func TestTickHandshakingWithZeroTimestamps(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	serverKey, _ := noise.GenerateKeyPair()

	conn, _ := newConn(key, transport, nil, serverKey.Public)

	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  key,
		RemoteStatic: &serverKey.Public,
	})

	conn.mu.Lock()
	conn.state = ConnStateHandshaking
	conn.hsState = hs
	// Zero timestamps - should not trigger retransmit or timeout
	conn.handshakeAttemptStart = time.Time{}
	conn.lastHandshakeSent = time.Time{}
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() with zero timestamps error = %v", err)
	}
}

// TestTickRekeyNotDuplicate tests that rekey is not triggered twice
func TestTickRekeyNotDuplicate(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
	conn.rekeyTriggered = true // Already triggered
	conn.mu.Unlock()

	// Tick should NOT trigger rekey again
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	conn.mu.RUnlock()

	if hasHsState {
		t.Error("Should not trigger rekey when rekeyTriggered is already true")
	}
}

// TestTickEstablishedNoSessionNoPanic tests Tick doesn't panic with nil current session
func TestTickEstablishedNoSessionNoPanic(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	conn, _ := newConn(key, transport, nil, noise.PublicKey{})

	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = nil // No session
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	// Should not panic
	err := conn.Tick()
	// May or may not error, but should not panic
	_ = err
}

// TestTickMessageBasedRekey tests rekey triggered by message count
func TestTickMessageBasedRekey(t *testing.T) {
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now()
	conn.sessionCreated = time.Now() // Recent session (no time-based rekey)
	conn.mu.Unlock()

	// Tick should check session nonces but not trigger rekey for fresh session
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Verify no rekey was triggered (nonces are low)
	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	conn.mu.RUnlock()

	if hasHsState {
		t.Error("Should not trigger rekey for fresh session with low nonces")
	}
}

// TestTickDisconnectionDetection tests that initiator detects disconnection
// when no packets received for KeepaliveTimeout + RekeyTimeout (15s).
// This should trigger a new handshake to re-establish connection.
func TestTickDisconnectionDetection(t *testing.T) {
	// Create two connected transports
	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	// Create a mock session
	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)

	// Set as initiator with no recent received data (past disconnection threshold)
	disconnectionThreshold := KeepaliveTimeout + RekeyTimeout
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = true
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now().Add(-disconnectionThreshold - time.Second) // Past 15s
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	// Tick should detect disconnection and initiate rekey
	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Give the rekey a moment to start
	time.Sleep(50 * time.Millisecond)

	// Verify that a new handshake was initiated
	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	rekeyTriggered := conn.rekeyTriggered
	conn.mu.RUnlock()

	if !hasHsState {
		t.Error("Tick() should initiate new handshake on disconnection detection")
	}
	if !rekeyTriggered {
		t.Error("Tick() should set rekeyTriggered on disconnection detection")
	}
}

// TestTickDisconnectionResponderNoAction tests that responder does NOT
// initiate handshake on disconnection detection (only initiator does).
func TestTickDisconnectionResponderNoAction(t *testing.T) {
	key, _ := noise.GenerateKeyPair()
	transport := NewMockTransport("test")
	defer transport.Close()

	serverKey, _ := noise.GenerateKeyPair()

	sendKey := [32]byte{1, 2, 3}
	recvKey := [32]byte{4, 5, 6}

	session, _ := noise.NewSession(noise.SessionConfig{
		LocalIndex:  1,
		RemoteIndex: 2,
		SendKey:     sendKey,
		RecvKey:     recvKey,
		RemotePK:    serverKey.Public,
	})

	conn, _ := newConn(key, transport, nil, serverKey.Public)

	// Set as responder with no recent received data
	disconnectionThreshold := KeepaliveTimeout + RekeyTimeout
	conn.mu.Lock()
	conn.state = ConnStateEstablished
	conn.current = session
	conn.isInitiator = false // Responder
	conn.lastSent = time.Now()
	conn.lastReceived = time.Now().Add(-disconnectionThreshold - time.Second) // Past 15s
	conn.sessionCreated = time.Now()
	conn.mu.Unlock()

	err := conn.Tick()
	if err != nil {
		t.Errorf("Tick() error = %v", err)
	}

	// Responder should NOT initiate handshake
	conn.mu.RLock()
	hasHsState := conn.hsState != nil
	conn.mu.RUnlock()

	if hasHsState {
		t.Error("Responder should NOT initiate handshake on disconnection")
	}
}

func TestTickKeepaliveWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clientTransport := NewMockTransport("client")
		serverTransport := NewMockTransport("server")
		clientTransport.Connect(serverTransport)
		defer clientTransport.Close()
		defer serverTransport.Close()

		clientKey, _ := noise.GenerateKeyPair()
		serverKey, _ := noise.GenerateKeyPair()

		session, _ := noise.NewSession(noise.SessionConfig{
			LocalIndex:  1,
			RemoteIndex: 2,
			SendKey:     [32]byte{1, 2, 3},
			RecvKey:     [32]byte{4, 5, 6},
			RemotePK:    serverKey.Public,
		})

		conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
		defer conn.Close()

		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.current = session
		conn.sessionCreated = time.Now()
		conn.lastSent = time.Now()
		conn.lastReceived = time.Now()
		conn.mu.Unlock()

		time.Sleep(KeepaliveTimeout + time.Second)

		conn.mu.Lock()
		conn.lastSent = time.Now().Add(-KeepaliveTimeout - time.Second)
		conn.lastReceived = time.Now()
		conn.mu.Unlock()

		err := conn.Tick()
		if err != nil {
			t.Errorf("Tick() error = %v", err)
		}

		conn.mu.RLock()
		lastSent := conn.lastSent
		conn.mu.RUnlock()

		if time.Since(lastSent) > time.Second {
			t.Error("Keepalive was not sent")
		}
	})
}

func TestConnectionTimeoutWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		defer conn.Close()

		startTime := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.lastSent = startTime
		conn.lastReceived = startTime
		conn.sessionCreated = startTime
		conn.mu.Unlock()

		time.Sleep(RejectAfterTime + time.Second)

		conn.mu.Lock()
		conn.lastReceived = startTime
		conn.mu.Unlock()

		err := conn.Tick()
		if err != ErrConnTimeout {
			t.Errorf("Tick() error = %v, want ErrConnTimeout", err)
		}
	})
}

func TestHandshakeTimeoutWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		defer conn.Close()

		startTime := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateHandshaking
		conn.handshakeAttemptStart = startTime
		conn.mu.Unlock()

		time.Sleep(RekeyAttemptTime + time.Second)

		err := conn.Tick()
		if err != ErrHandshakeTimeout {
			t.Errorf("Tick() error = %v, want ErrHandshakeTimeout", err)
		}
	})
}

func TestHandshakeRetransmitIntervalWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clientTransport := NewMockTransport("client")
		serverTransport := NewMockTransport("server")
		clientTransport.Connect(serverTransport)
		defer clientTransport.Close()
		defer serverTransport.Close()

		clientKey, _ := noise.GenerateKeyPair()
		serverKey, _ := noise.GenerateKeyPair()

		conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
		defer conn.Close()

		hs, _ := noise.NewHandshakeState(noise.Config{
			Pattern:      noise.PatternIK,
			Initiator:    true,
			LocalStatic:  clientKey,
			RemoteStatic: &serverKey.Public,
		})

		startTime := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateHandshaking
		conn.hsState = hs
		conn.handshakeAttemptStart = startTime
		conn.lastHandshakeSent = startTime
		conn.mu.Unlock()
		if err := conn.tick(false); err != nil {
			t.Fatalf("tick() error = %v", err)
		}

		time.Sleep(RekeyTimeout + 100*time.Millisecond)
		synctest.Wait()

		conn.mu.RLock()
		firstRetransmit := conn.lastHandshakeSent
		conn.mu.RUnlock()
		if !firstRetransmit.After(startTime) {
			t.Fatal("expected timer to retransmit handshake after RekeyTimeout")
		}

		time.Sleep(RekeyTimeout + 100*time.Millisecond)
		synctest.Wait()

		conn.mu.RLock()
		secondRetransmit := conn.lastHandshakeSent
		conn.mu.RUnlock()
		if !secondRetransmit.After(firstRetransmit) {
			t.Fatal("expected timer to retransmit handshake again after another RekeyTimeout")
		}
	})
}

func TestSessionExpirationWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		defer conn.Close()

		startTime := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.sessionCreated = startTime
		conn.lastReceived = startTime
		conn.mu.Unlock()

		time.Sleep(RejectAfterTime + time.Second)

		conn.mu.Lock()
		conn.lastReceived = startTime
		conn.mu.Unlock()

		err := conn.Tick()
		if err != ErrConnTimeout {
			t.Errorf("Tick() after session expiration = %v, want ErrConnTimeout", err)
		}
	})
}

func TestNoKeepaliveWhenBothOldWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		remotePK := noise.PublicKey{}
		session, _ := noise.NewSession(noise.SessionConfig{
			LocalIndex:  1,
			RemoteIndex: 2,
			SendKey:     [32]byte{1, 2, 3},
			RecvKey:     [32]byte{4, 5, 6},
			RemotePK:    remotePK,
		})

		conn, _ := newConn(key, transport, nil, remotePK)
		defer conn.Close()

		startTime := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.current = session
		conn.sessionCreated = startTime
		conn.lastSent = startTime
		conn.lastReceived = startTime
		conn.mu.Unlock()

		time.Sleep(KeepaliveTimeout + time.Second)

		conn.mu.Lock()
		conn.lastSent = startTime
		conn.lastReceived = startTime
		originalLastSent := conn.lastSent
		conn.mu.Unlock()

		err := conn.Tick()
		if err != nil {
			t.Errorf("Tick() error = %v", err)
		}

		conn.mu.RLock()
		newLastSent := conn.lastSent
		conn.mu.RUnlock()

		if newLastSent != originalLastSent {
			t.Error("Should not have sent keepalive when lastReceived is also old")
		}
	})
}

// setupConnPair creates a pair of established connections for testing.
// Returns initiator, responder connections and a cleanup function.
func setupConnPair(t *testing.T) (initiator, responder *Conn, cleanup func()) {
	t.Helper()

	initiatorKey, _ := noise.GenerateKeyPair()
	responderKey, _ := noise.GenerateKeyPair()

	initiatorTransport := NewMockTransport("initiator")
	responderTransport := NewMockTransport("responder")
	initiatorTransport.Connect(responderTransport)

	var wg sync.WaitGroup
	var initiatorConn, responderConn *Conn
	var responderErr error

	// Start responder
	wg.Add(1)
	go func() {
		defer wg.Done()
		responderConn, _ = newConn(responderKey, responderTransport, initiatorTransport.LocalAddr(), noise.PublicKey{})
		buf := make([]byte, noise.MaxPacketSize)
		n, _, err := responderTransport.RecvFrom(buf)
		if err != nil {
			responderErr = err
			return
		}
		initMsg, err := noise.ParseHandshakeInit(buf[:n])
		if err != nil {
			responderErr = err
			return
		}
		resp, err := responderConn.accept(initMsg)
		if err != nil {
			responderErr = err
			return
		}
		if err := responderTransport.SendTo(resp, initiatorTransport.LocalAddr()); err != nil {
			responderErr = err
			return
		}
	}()

	time.Sleep(10 * time.Millisecond)

	// Dial from initiator
	initiatorConn, err := Dial(context.Background(), initiatorTransport, responderTransport.LocalAddr(), responderKey.Public, initiatorKey)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	wg.Wait()

	if responderErr != nil {
		t.Fatalf("Responder error = %v", responderErr)
	}

	cleanup = func() {
		initiatorConn.Close()
		responderConn.Close()
		initiatorTransport.Close()
		responderTransport.Close()
	}

	return initiatorConn, responderConn, cleanup
}

// TestRekeyInitiatorFlow tests the complete rekey flow initiated by the initiator.
// 1. Establish initial connection
// 2. Trigger rekey by making session appear expired
// 3. Responder receives handshake init and sends response
// 4. Initiator processes response via Recv()
// 5. Verify session rotation: current -> previous
func TestRekeyInitiatorFlow(t *testing.T) {
	initiator, responder, cleanup := setupConnPair(t)
	defer cleanup()

	// Save old session info
	initiator.mu.RLock()
	oldSession := initiator.current
	oldLocalIdx := oldSession.LocalIndex()
	initiator.mu.RUnlock()

	// Make session appear expired to trigger rekey
	initiator.mu.Lock()
	initiator.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
	initiator.isInitiator = true
	initiator.mu.Unlock()

	// Tick should trigger rekey
	err := initiator.Tick()
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	// Give time for rekey handshake to be sent
	time.Sleep(50 * time.Millisecond)

	// Verify initiator has started rekey (has hsState)
	initiator.mu.RLock()
	hasHsState := initiator.hsState != nil
	newLocalIdx := initiator.localIdx
	initiator.mu.RUnlock()

	if !hasHsState {
		t.Fatal("Initiator did not start rekey")
	}
	if newLocalIdx == oldLocalIdx {
		t.Error("Initiator should have new local index for rekey")
	}

	// Responder receives handshake init via Recv()
	// This should trigger handleHandshakeInit
	proto, payload, err := responder.Recv()
	if err != nil {
		t.Fatalf("Responder Recv() error = %v", err)
	}

	// Empty payload indicates handshake message was processed
	if proto != 0 || payload != nil {
		t.Errorf("Expected empty payload for handshake, got proto=%d, payload=%v", proto, payload)
	}

	// Verify responder has new session
	responder.mu.RLock()
	responderHasNew := responder.current != nil
	responderHasPrev := responder.previous != nil
	responder.mu.RUnlock()

	if !responderHasNew {
		t.Error("Responder should have new current session")
	}
	if !responderHasPrev {
		t.Error("Responder should have previous session after rekey")
	}

	// Initiator receives handshake response via Recv()
	proto, payload, err = initiator.Recv()
	if err != nil {
		t.Fatalf("Initiator Recv() error = %v", err)
	}

	// Empty payload indicates handshake response was processed
	if proto != 0 || payload != nil {
		t.Errorf("Expected empty payload for handshake response, got proto=%d, payload=%v", proto, payload)
	}

	// Verify initiator session rotation
	initiator.mu.RLock()
	initiatorCurrent := initiator.current
	initiatorPrev := initiator.previous
	initiatorHsState := initiator.hsState
	initiator.mu.RUnlock()

	if initiatorCurrent == nil {
		t.Fatal("Initiator current session is nil after rekey")
	}
	if initiatorPrev == nil {
		t.Error("Initiator should have previous session after rekey")
	}
	if initiatorPrev != oldSession {
		t.Error("Initiator previous should be old session")
	}
	if initiatorHsState != nil {
		t.Error("Initiator hsState should be nil after rekey completes")
	}

	// Test that communication still works with new session
	testData := []byte("message after rekey")
	if err := initiator.Send(testDirectProtoA, testData); err != nil {
		t.Fatalf("Send after rekey error = %v", err)
	}

	proto, payload, err = responder.Recv()
	if err != nil {
		t.Fatalf("Recv after rekey error = %v", err)
	}
	if proto != testDirectProtoA || !bytes.Equal(payload, testData) {
		t.Errorf("Message mismatch after rekey")
	}
}

// TestRekeyResponderFlow tests rekey when initiated by the peer.
// The responder should handle handshake init and complete the rekey.
func TestRekeyResponderFlow(t *testing.T) {
	// Setup connection where we're the responder
	clientKey, _ := noise.GenerateKeyPair()
	serverKey, _ := noise.GenerateKeyPair()

	clientTransport := NewMockTransport("client")
	serverTransport := NewMockTransport("server")
	clientTransport.Connect(serverTransport)
	defer clientTransport.Close()
	defer serverTransport.Close()

	// Establish initial connection
	var wg sync.WaitGroup
	var serverConn *Conn
	var serverErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn, _ = newConn(serverKey, serverTransport, clientTransport.LocalAddr(), noise.PublicKey{})
		buf := make([]byte, noise.MaxPacketSize)
		n, _, err := serverTransport.RecvFrom(buf)
		if err != nil {
			serverErr = err
			return
		}
		initMsg, _ := noise.ParseHandshakeInit(buf[:n])
		resp, _ := serverConn.accept(initMsg)
		serverTransport.SendTo(resp, clientTransport.LocalAddr())
	}()

	time.Sleep(10 * time.Millisecond)

	clientConn, err := Dial(context.Background(), clientTransport, serverTransport.LocalAddr(), serverKey.Public, clientKey)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer clientConn.Close()
	wg.Wait()

	if serverErr != nil {
		t.Fatalf("Server error = %v", serverErr)
	}
	defer serverConn.Close()

	// Save server's old session
	serverConn.mu.RLock()
	oldServerSession := serverConn.current
	serverConn.mu.RUnlock()

	// Client initiates rekey by creating and sending new handshake init
	newClientIdx, _ := noise.GenerateIndex()
	hs, _ := noise.NewHandshakeState(noise.Config{
		Pattern:      noise.PatternIK,
		Initiator:    true,
		LocalStatic:  clientKey,
		RemoteStatic: &serverKey.Public,
	})
	msg1, _ := hs.WriteMessage(nil)
	wireInit := noise.BuildHandshakeInit(newClientIdx, hs.LocalEphemeral(), msg1[noise.KeySize:])

	// Inject handshake init to server
	serverTransport.InjectPacket(wireInit, clientTransport.LocalAddr())

	// Server calls Recv() which processes the handshake init
	proto, payload, err := serverConn.Recv()
	if err != nil {
		t.Fatalf("Server Recv() for rekey error = %v", err)
	}

	// Empty payload for handshake
	if proto != 0 || payload != nil {
		t.Errorf("Expected empty for handshake init processing")
	}

	// Verify server has rotated sessions
	serverConn.mu.RLock()
	newServerSession := serverConn.current
	prevServerSession := serverConn.previous
	serverIsInitiator := serverConn.isInitiator
	serverConn.mu.RUnlock()

	if newServerSession == oldServerSession {
		t.Error("Server should have new session after rekey")
	}
	if prevServerSession != oldServerSession {
		t.Error("Server previous should be old session")
	}
	if serverIsInitiator {
		t.Error("Server should be responder (isInitiator=false)")
	}
}

// TestRekeyResponseErrors tests error handling in handleHandshakeResponse
func TestRekeyResponseErrors(t *testing.T) {
	t.Run("NoHandshakeState", func(t *testing.T) {
		key, _ := noise.GenerateKeyPair()
		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(key, transport, nil, noise.PublicKey{})
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.hsState = nil // No pending handshake
		conn.mu.Unlock()

		// Create a fake handshake response
		resp := &noise.HandshakeRespMessage{
			SenderIndex:   1,
			ReceiverIndex: 2,
		}

		_, _, err := conn.handleHandshakeResponse(resp, nil)
		if err != ErrInvalidConnState {
			t.Errorf("handleHandshakeResponse() error = %v, want ErrInvalidConnState", err)
		}
	})

	t.Run("WrongReceiverIndex", func(t *testing.T) {
		clientKey, _ := noise.GenerateKeyPair()
		serverKey, _ := noise.GenerateKeyPair()

		transport := NewMockTransport("test")
		defer transport.Close()

		conn, _ := newConn(clientKey, transport, nil, serverKey.Public)

		// Create handshake state
		hs, _ := noise.NewHandshakeState(noise.Config{
			Pattern:      noise.PatternIK,
			Initiator:    true,
			LocalStatic:  clientKey,
			RemoteStatic: &serverKey.Public,
		})
		hs.WriteMessage(nil)

		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.hsState = hs
		conn.localIdx = 100
		conn.mu.Unlock()

		// Create response with wrong receiver index
		resp := &noise.HandshakeRespMessage{
			SenderIndex:   1,
			ReceiverIndex: 999, // Wrong index
		}

		_, _, err := conn.handleHandshakeResponse(resp, nil)
		if err != ErrInvalidReceiverIndex {
			t.Errorf("handleHandshakeResponse() error = %v, want ErrInvalidReceiverIndex", err)
		}
	})
}

// TestRekeyInitErrors tests error handling in handleHandshakeInit
func TestRekeyInitErrors(t *testing.T) {
	t.Run("InvalidRemotePK", func(t *testing.T) {
		serverKey, _ := noise.GenerateKeyPair()
		clientKey, _ := noise.GenerateKeyPair()
		wrongKey, _ := noise.GenerateKeyPair() // Different key

		serverTransport := NewMockTransport("server")
		clientTransport := NewMockTransport("client")
		serverTransport.Connect(clientTransport)
		defer serverTransport.Close()
		defer clientTransport.Close()

		// Create server conn expecting wrongKey, not clientKey
		serverConn, _ := newConn(serverKey, serverTransport, clientTransport.LocalAddr(), wrongKey.Public)
		serverConn.mu.Lock()
		serverConn.state = ConnStateEstablished
		serverConn.mu.Unlock()

		// Client sends handshake init with clientKey
		hs, _ := noise.NewHandshakeState(noise.Config{
			Pattern:      noise.PatternIK,
			Initiator:    true,
			LocalStatic:  clientKey,
			RemoteStatic: &serverKey.Public,
		})
		msg1, _ := hs.WriteMessage(nil)
		wireInit := noise.BuildHandshakeInit(1, hs.LocalEphemeral(), msg1[noise.KeySize:])

		// Parse and handle
		initMsg, _ := noise.ParseHandshakeInit(wireInit)
		_, _, err := serverConn.handleHandshakeInit(initMsg, nil)

		if err != ErrInvalidRemotePK {
			t.Errorf("handleHandshakeInit() error = %v, want ErrInvalidRemotePK", err)
		}
	})
}

// TestRekeyWithDataExchange tests that data can still be exchanged during and after rekey
func TestRekeyWithDataExchange(t *testing.T) {
	initiator, responder, cleanup := setupConnPair(t)
	defer cleanup()

	// Exchange data before rekey
	preRekeyData := []byte("before rekey")
	if err := initiator.Send(testDirectProtoA, preRekeyData); err != nil {
		t.Fatalf("Send before rekey error = %v", err)
	}
	proto, payload, _ := responder.Recv()
	if !bytes.Equal(payload, preRekeyData) {
		t.Error("Pre-rekey data mismatch")
	}
	_ = proto

	// Trigger rekey
	initiator.mu.Lock()
	initiator.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
	initiator.isInitiator = true
	initiator.mu.Unlock()

	initiator.Tick()
	time.Sleep(50 * time.Millisecond)

	// Process rekey on both sides
	responder.Recv() // Handles handshake init
	initiator.Recv() // Handles handshake response

	// Exchange data after rekey
	postRekeyData := []byte("after rekey")
	if err := initiator.Send(testDirectProtoA, postRekeyData); err != nil {
		t.Fatalf("Send after rekey error = %v", err)
	}
	proto, payload, err := responder.Recv()
	if err != nil {
		t.Fatalf("Recv after rekey error = %v", err)
	}
	if !bytes.Equal(payload, postRekeyData) {
		t.Error("Post-rekey data mismatch")
	}

	// Bidirectional
	if err := responder.Send(testDirectProtoA, postRekeyData); err != nil {
		t.Fatalf("Responder send after rekey error = %v", err)
	}
	proto, payload, err = initiator.Recv()
	if err != nil {
		t.Fatalf("Initiator recv after rekey error = %v", err)
	}
	if proto != testDirectProtoA || !bytes.Equal(payload, postRekeyData) {
		t.Error("Bidirectional data mismatch after rekey")
	}
}

// TestMultipleRekeys tests multiple consecutive rekeys
func TestMultipleRekeys(t *testing.T) {
	initiator, responder, cleanup := setupConnPair(t)
	defer cleanup()

	for i := range 3 {
		// Trigger rekey
		initiator.mu.Lock()
		initiator.sessionCreated = time.Now().Add(-RekeyAfterTime - time.Second)
		initiator.isInitiator = true
		initiator.rekeyTriggered = false
		initiator.mu.Unlock()

		initiator.Tick()
		time.Sleep(50 * time.Millisecond)

		// Complete rekey
		responder.Recv()
		initiator.Recv()

		// Verify communication
		testData := []byte("test")
		if err := initiator.Send(testDirectProtoA, testData); err != nil {
			t.Fatalf("Rekey %d: send error = %v", i, err)
		}
		_, payload, err := responder.Recv()
		if err != nil {
			t.Fatalf("Rekey %d: Recv error = %v", i, err)
		}
		if !bytes.Equal(payload, testData) {
			t.Errorf("Rekey %d: data mismatch", i)
		}
	}
}

func TestRekeyTriggerWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clientTransport := NewMockTransport("client")
		serverTransport := NewMockTransport("server")
		clientTransport.Connect(serverTransport)
		defer clientTransport.Close()
		defer serverTransport.Close()

		clientKey, _ := noise.GenerateKeyPair()
		serverKey, _ := noise.GenerateKeyPair()

		session, _ := noise.NewSession(noise.SessionConfig{
			LocalIndex:  1,
			RemoteIndex: 2,
			SendKey:     [32]byte{1, 2, 3},
			RecvKey:     [32]byte{4, 5, 6},
			RemotePK:    serverKey.Public,
		})

		conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
		defer conn.Close()

		sessionStart := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.current = session
		conn.isInitiator = true
		conn.sessionCreated = sessionStart
		conn.lastSent = sessionStart
		conn.lastReceived = sessionStart
		conn.mu.Unlock()

		time.Sleep(RekeyAfterTime - time.Second)

		conn.mu.Lock()
		conn.sessionCreated = sessionStart
		conn.lastReceived = time.Now()
		conn.mu.Unlock()

		err := conn.Tick()
		if err != nil {
			t.Errorf("Tick() before rekey time error = %v", err)
		}

		conn.mu.RLock()
		hasHs := conn.hsState != nil
		conn.mu.RUnlock()

		if hasHs {
			t.Error("Should not have triggered rekey before RekeyAfterTime")
		}

		time.Sleep(2 * time.Second)

		err = conn.Tick()
		if err != nil {
			t.Errorf("Tick() at rekey time error = %v", err)
		}

		synctest.Wait()

		conn.mu.RLock()
		hasHsAfter := conn.hsState != nil
		conn.mu.RUnlock()

		if !hasHsAfter {
			t.Error("Should have triggered rekey after RekeyAfterTime")
		}
	})
}

func TestRekeyOnReceiveThresholdWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clientTransport := NewMockTransport("client")
		serverTransport := NewMockTransport("server")
		clientTransport.Connect(serverTransport)
		defer clientTransport.Close()
		defer serverTransport.Close()

		clientKey, _ := noise.GenerateKeyPair()
		serverKey, _ := noise.GenerateKeyPair()

		session, _ := noise.NewSession(noise.SessionConfig{
			LocalIndex:  1,
			RemoteIndex: 2,
			SendKey:     [32]byte{1, 2, 3},
			RecvKey:     [32]byte{4, 5, 6},
			RemotePK:    serverKey.Public,
		})

		conn, _ := newConn(clientKey, clientTransport, serverTransport.LocalAddr(), serverKey.Public)
		defer conn.Close()

		sessionStart := time.Now()
		conn.mu.Lock()
		conn.state = ConnStateEstablished
		conn.current = session
		conn.isInitiator = true
		conn.sessionCreated = sessionStart
		conn.lastSent = sessionStart
		conn.lastReceived = sessionStart
		conn.mu.Unlock()

		time.Sleep(RekeyOnRecvThreshold + time.Second)

		if RekeyOnRecvThreshold != 165*time.Second {
			t.Errorf("RekeyOnRecvThreshold = %v, want 165s", RekeyOnRecvThreshold)
		}
	})
}
