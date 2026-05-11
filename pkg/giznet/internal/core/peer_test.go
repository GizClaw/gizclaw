package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/noise"
)

func readExactWithTimeout(t *testing.T, r io.Reader, n int, timeout time.Duration) []byte {
	t.Helper()

	buf := make([]byte, n)
	errCh := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(r, buf)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ReadFull failed: %v", err)
		}
		return buf
	case <-time.After(timeout):
		t.Fatalf("ReadFull timeout after %s", timeout)
		return nil
	}
}

func TestPeerOpenStreamAcceptStream(t *testing.T) {
	// Generate key pairs
	clientKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate client key: %v", err)
	}
	serverKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate server key: %v", err)
	}

	// Create UDP instances
	client, err := NewUDP(clientKey, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("Failed to create client UDP: %v", err)
	}
	defer client.Close()

	server, err := NewUDP(serverKey, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("Failed to create server UDP: %v", err)
	}
	defer server.Close()

	// Set up peer endpoints
	clientAddr := client.HostInfo().Addr
	serverAddr := server.HostInfo().Addr
	client.SetPeerEndpoint(serverKey.Public, serverAddr)
	server.SetPeerEndpoint(clientKey.Public, clientAddr)

	// Start receive loops
	clientRecvDone := make(chan struct{})
	serverRecvDone := make(chan struct{})

	go func() {
		defer close(clientRecvDone)
		buf := make([]byte, 65535)
		for {
			_, _, err := client.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	go func() {
		defer close(serverRecvDone)
		buf := make([]byte, 65535)
		for {
			_, _, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	// Connect client to server
	if err := client.Connect(serverKey.Public); err != nil {
		t.Fatalf("Client handshake failed: %v", err)
	}

	// Wait for handshake to complete on server side
	time.Sleep(100 * time.Millisecond)

	// Open stream from client
	clientStream := mustOpenStream(t, client, serverKey.Public, 0)
	defer clientStream.Close()

	serverStreamChan := make(chan io.ReadWriteCloser, 1)
	serverErrChan := make(chan error, 1)
	go func() {
		serverStreamChan <- mustAcceptStream(t, server, clientKey.Public, 0)
	}()

	// Write data from client
	testData := []byte("Hello from client stream!")
	n, err := clientStream.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write to stream: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned %d, expected %d", n, len(testData))
	}

	// Wait for server to accept the stream
	var serverStream io.ReadWriteCloser
	select {
	case serverStream = <-serverStreamChan:
	case err := <-serverErrChan:
		t.Fatalf("Server AcceptStream failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for server to accept stream")
	}
	defer serverStream.Close()

	// Give time for KCP to process
	time.Sleep(200 * time.Millisecond)

	// Read data on server
	readBuf := make([]byte, 1024)
	n, err = serverStream.Read(readBuf)
	if err != nil {
		t.Fatalf("Failed to read from stream: %v", err)
	}
	if string(readBuf[:n]) != string(testData) {
		t.Errorf("Read data mismatch: got %q, expected %q", string(readBuf[:n]), string(testData))
	}
}

func TestPeerReadWrite(t *testing.T) {
	// Generate key pairs
	clientKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate client key: %v", err)
	}
	serverKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate server key: %v", err)
	}

	// Create UDP instances
	client, err := NewUDP(clientKey, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("Failed to create client UDP: %v", err)
	}
	defer client.Close()

	server, err := NewUDP(serverKey, WithBindAddr("127.0.0.1:0"), WithAllowFunc(func(noise.PublicKey) bool {
		return true
	}))
	if err != nil {
		t.Fatalf("Failed to create server UDP: %v", err)
	}
	defer server.Close()

	// Set up peer endpoints
	clientAddr := client.HostInfo().Addr
	serverAddr := server.HostInfo().Addr
	client.SetPeerEndpoint(serverKey.Public, serverAddr)
	server.SetPeerEndpoint(clientKey.Public, clientAddr)

	// Start receive loops
	go func() {
		buf := make([]byte, 65535)
		for {
			_, _, err := client.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 65535)
		for {
			_, _, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
		}
	}()

	// Connect client to server
	if err := client.Connect(serverKey.Public); err != nil {
		t.Fatalf("Client handshake failed: %v", err)
	}

	// Wait for handshake
	time.Sleep(100 * time.Millisecond)

	// Test Write with custom protocol
	testData := []byte("Hello with custom protocol!")
	testProto := testDirectProtoA

	// Start Read goroutine on server
	readResultChan := make(chan struct {
		proto   byte
		n       int
		err     error
		payload []byte
	}, 1)
	go func() {
		buf := make([]byte, 1024)
		smux := mustServiceMux(t, server, clientKey.Public)
		proto, n, err := smux.Read(buf)
		readResultChan <- struct {
			proto   byte
			n       int
			err     error
			payload []byte
		}{proto, n, err, buf[:n]}
	}()

	// Write from client
	n, err := mustServiceMux(t, client, serverKey.Public).Write(testProto, testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned %d, expected %d", n, len(testData))
	}

	// Wait for Read result
	select {
	case result := <-readResultChan:
		if result.err != nil {
			t.Fatalf("Read failed: %v", result.err)
		}
		if result.proto != testProto {
			t.Errorf("Protocol mismatch: got %d, expected %d", result.proto, testProto)
		}
		if string(result.payload) != string(testData) {
			t.Errorf("Payload mismatch: got %q, expected %q", string(result.payload), string(testData))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for Read")
	}
}

func TestPeerWriteAcceptsNonKCPDirectProtocol(t *testing.T) {
	server, client, serverKey, _ := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	payload := []byte("payload")
	n, err := mustServiceMux(t, client, serverKey.Public).Write(0x55, payload)
	if err != nil {
		t.Fatalf("Write(0x55) err=%v, want nil", err)
	}
	if n != len(payload) {
		t.Fatalf("Write n=%d, want %d", n, len(payload))
	}
}

func TestOpenStreamNonZeroService(t *testing.T) {
	server, client, serverKey, clientKey := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	acceptedCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		acceptedCh <- mustAcceptStream(t, server, clientKey.Public, 7)
	}()

	time.Sleep(50 * time.Millisecond)

	stream := mustOpenStream(t, client, serverKey.Public, 7)
	defer stream.Close()

	if _, err := stream.Write([]byte("hello")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var accepted net.Conn
	select {
	case accepted = <-acceptedCh:
	case err := <-errCh:
		t.Fatalf("AcceptStream(7) err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("AcceptStream(7) timeout")
	}
	defer accepted.Close()

	buf := make([]byte, 64)
	n, err := accepted.Read(buf)
	if err != nil {
		t.Fatalf("Read err=%v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("Read got=%q, want %q", string(buf[:n]), "hello")
	}
}

func TestPeer_MultiServiceConcurrentStreams(t *testing.T) {
	server, client, serverKey, clientKey := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	services := []uint64{0, 7}
	perService := 3
	expected := make(map[string]struct{}, len(services)*perService)
	received := make(chan string, len(services)*perService)
	errCh := make(chan error, len(services)*perService*2)

	var acceptWG sync.WaitGroup
	for _, serviceID := range services {
		svc := serviceID
		acceptWG.Add(1)
		go func() {
			defer acceptWG.Done()
			for range perService {
				stream, err := mustServiceMux(t, server, clientKey.Public).AcceptStream(svc)
				if err != nil {
					errCh <- err
					return
				}
				if err := stream.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
					errCh <- err
					_ = stream.Close()
					return
				}
				buf := make([]byte, 128)
				n, err := stream.Read(buf)
				if err != nil && !(errors.Is(err, io.EOF) && n > 0) {
					errCh <- err
					_ = stream.Close()
					return
				}
				received <- fmt.Sprintf("%d:%s", svc, string(buf[:n]))
				_ = stream.Close()
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)

	var openWG sync.WaitGroup
	for _, serviceID := range services {
		for i := range perService {
			payload := []byte(fmt.Sprintf("service-%d-stream-%d", serviceID, i))
			expected[fmt.Sprintf("%d:%s", serviceID, string(payload))] = struct{}{}
			openWG.Add(1)
			go func(service uint64, msg []byte) {
				defer openWG.Done()
				stream, err := mustServiceMux(t, client, serverKey.Public).OpenStream(service)
				if err != nil {
					errCh <- err
					return
				}
				defer stream.Close()
				if _, err := stream.Write(msg); err != nil {
					errCh <- err
				}
				time.Sleep(20 * time.Millisecond)
			}(serviceID, payload)
		}
	}

	openWG.Wait()
	acceptWG.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	close(received)
	for got := range received {
		delete(expected, got)
	}
	if len(expected) != 0 {
		t.Fatalf("missing payloads: %v", expected)
	}
}

func TestPeer_MultiServiceConcurrentBidirectionalNoCrossTalk(t *testing.T) {
	server, client, serverKey, clientKey := createConnectedPair(t)
	defer server.Close()
	defer client.Close()

	services := []uint64{0, 7}
	errCh := make(chan error, len(services)*4)

	var acceptWG sync.WaitGroup
	for _, serviceID := range services {
		svc := serviceID
		acceptWG.Add(1)
		go func() {
			defer acceptWG.Done()
			stream, err := mustServiceMux(t, server, clientKey.Public).AcceptStream(svc)
			if err != nil {
				errCh <- err
				return
			}

			req := []byte(fmt.Sprintf("req-%d", svc))
			got := readExactWithTimeout(t, stream, len(req), 5*time.Second)
			if !bytes.Equal(got, req) {
				errCh <- fmt.Errorf("service %d request mismatch: got=%q want=%q", svc, got, req)
				return
			}

			resp := []byte(fmt.Sprintf("resp-%d", svc))
			if _, err := stream.Write(resp); err != nil {
				errCh <- err
				return
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)

	type result struct {
		service uint64
		resp    []byte
	}
	resultCh := make(chan result, len(services))
	var openWG sync.WaitGroup
	for _, serviceID := range services {
		svc := serviceID
		openWG.Add(1)
		go func() {
			defer openWG.Done()
			stream, err := mustServiceMux(t, client, serverKey.Public).OpenStream(svc)
			if err != nil {
				errCh <- err
				return
			}
			defer stream.Close()

			req := []byte(fmt.Sprintf("req-%d", svc))
			if _, err := stream.Write(req); err != nil {
				errCh <- err
				return
			}

			resp := readExactWithTimeout(t, stream, len(fmt.Sprintf("resp-%d", svc)), 5*time.Second)
			_ = stream.Close()
			resultCh <- result{service: svc, resp: resp}
		}()
	}

	openWG.Wait()
	acceptWG.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	close(resultCh)
	for got := range resultCh {
		want := []byte(fmt.Sprintf("resp-%d", got.service))
		if !bytes.Equal(got.resp, want) {
			t.Fatalf("service %d response mismatch: got=%q want=%q", got.service, got.resp, want)
		}
	}
}

func TestKCPOutputErrorCounterFromServiceMux(t *testing.T) {
	localKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(local) failed: %v", err)
	}
	remoteKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(remote) failed: %v", err)
	}

	u := &UDP{localKey: localKey}
	peer := &peerState{pk: remoteKey.Public}
	smux := u.createServiceMux(peer)
	defer smux.Close()

	if _, err := smux.OpenStream(0); err == nil {
		t.Fatal("OpenStream(service=0) should fail without a peer endpoint")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := u.kcpOutputErrors.Load(); got > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("kcpOutputErrors did not increase, got=%d", u.kcpOutputErrors.Load())
}

func TestPeerCloseServiceMuxDoesNotCloseIfMatchWhenNoActiveMux(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	events := make(chan PeerEvent, 1)
	u := &UDP{
		onPeerEvent: func(ev PeerEvent) bool {
			events <- ev
			return true
		},
	}
	ifMatch := NewServiceMux(key.Public, ServiceMuxConfig{})
	peer := &Peer{
		udp: u,
		state: &peerState{
			pk:    key.Public,
			state: PeerStateEstablished,
		},
	}

	peer.CloseServiceMux(ifMatch)

	if err := ifMatch.InputPacket(testDirectProtoA, []byte("still-open")); err != nil {
		t.Fatalf("ifMatch token should remain open, InputPacket err=%v", err)
	}
	if got := peer.State(); got != PeerStateEstablished {
		t.Fatalf("peer.State()=%v, want %v", got, PeerStateEstablished)
	}
	select {
	case ev := <-events:
		t.Fatalf("CloseServiceMux emitted event for unowned target: %+v", ev)
	default:
	}
}

func TestPeerCloseServiceMuxDoesNotCloseMismatchedActiveMux(t *testing.T) {
	key, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	active := NewServiceMux(key.Public, ServiceMuxConfig{})
	defer active.Close()
	ifMatch := NewServiceMux(key.Public, ServiceMuxConfig{})
	defer ifMatch.Close()
	peer := &Peer{
		udp: &UDP{},
		state: &peerState{
			pk:         key.Public,
			serviceMux: active,
			state:      PeerStateEstablished,
		},
	}

	peer.CloseServiceMux(ifMatch)
	if err := active.InputPacket(testDirectProtoA, []byte("still-open")); err != nil {
		t.Fatalf("active service mux should remain open, InputPacket err=%v", err)
	}
	if err := ifMatch.InputPacket(testDirectProtoA, []byte("still-open")); err != nil {
		t.Fatalf("ifMatch token should remain open, InputPacket err=%v", err)
	}
	if got := peer.State(); got != PeerStateEstablished {
		t.Fatalf("peer.State()=%v, want %v", got, PeerStateEstablished)
	}
}

func TestCreateServiceMux_UsesInjectedPeerAwareOnNewService(t *testing.T) {
	localKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(local) failed: %v", err)
	}
	remoteKey, err := noise.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(remote) failed: %v", err)
	}

	var gotPeer noise.PublicKey
	var gotService uint64
	u := &UDP{
		localKey: localKey,
		serviceMuxConfig: ServiceMuxConfig{
			OnNewService: func(peer noise.PublicKey, service uint64) bool {
				gotPeer = peer
				gotService = service
				return peer == remoteKey.Public && service == 3
			},
		},
	}
	peer := &peerState{pk: remoteKey.Public}
	smux := u.createServiceMux(peer)
	defer smux.Close()

	streamID := uint64(0)
	if !u.isKCPClient(remoteKey.Public) {
		streamID = 1
	}
	frame := binary.AppendUvarint(nil, streamID)
	frame = append(frame, 0)

	if err := smux.InputKCP(3, frame); err != nil {
		t.Fatalf("InputKCP(service=3) failed: %v", err)
	}
	if gotPeer != remoteKey.Public {
		t.Fatalf("OnNewService peer=%v, want %v", gotPeer, remoteKey.Public)
	}
	if gotService != 3 {
		t.Fatalf("OnNewService service=%d, want 3", gotService)
	}
	if smux.NumServices() != 1 {
		t.Fatalf("NumServices=%d, want 1", smux.NumServices())
	}
}
