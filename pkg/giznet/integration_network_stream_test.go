package giznet_test

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

// TestIntegration_ConnectionPoolCapacity verifies the server can handle
// 64 concurrent peer connections simultaneously.
func TestIntegration_ConnectionPoolCapacity(t *testing.T) {
	const peerCount = 64

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	server := NewUDPNode(t, serverKey)
	defer server.Close()

	clients := make([]*giznet.UDP, 0, peerCount)
	clientKeys := make([]*giznet.KeyPair, 0, peerCount)
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	for i := range peerCount {
		clientKey, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("Generate client key[%d] failed: %v", i, err)
		}
		client := NewUDPNode(t, clientKey)
		clients = append(clients, client)
		clientKeys = append(clientKeys, clientKey)

		ConnectNodes(t, client, clientKey, server, serverKey)
	}

	if got := server.HostInfo().PeerCount; got < peerCount {
		t.Fatalf("server peer count=%d, want >= %d", got, peerCount)
	}

	for i := range peerCount {
		msg := []byte(fmt.Sprintf("peer-%02d", i))
		n, err := MustPeerMux(t, clients[i], serverKey.Public).Write(testProtocolEvent, msg)
		if err != nil {
			t.Fatalf("client[%d] Write failed: %v", i, err)
		}
		if n != len(msg) {
			t.Fatalf("client[%d] Write bytes=%d, want %d", i, n, len(msg))
		}

		proto, got := ReadFromPeerWithTimeout(t, server, clientKeys[i].Public, 3*time.Second)
		if proto != testProtocolEvent {
			t.Fatalf("server received proto=%d from client[%d], want %d", proto, i, testProtocolEvent)
		}
		if !bytes.Equal(got, msg) {
			t.Fatalf("server payload mismatch from client[%d]: got=%q want=%q", i, string(got), string(msg))
		}
	}
}

// TestIntegration_NetworkInterruptionReconnect verifies that a peer with the
// same key can reconnect from a new endpoint and the server updates accordingly.
func TestIntegration_NetworkInterruptionReconnect(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()

	clientV1 := NewUDPNode(t, clientKey)
	ConnectNodes(t, clientV1, clientKey, server, serverKey)

	oldEndpoint := clientV1.HostInfo().Addr.String()
	_ = clientV1.Close()

	clientV2 := NewUDPNode(t, clientKey)
	defer clientV2.Close()
	ConnectNodes(t, clientV2, clientKey, server, serverKey)

	newEndpoint := clientV2.HostInfo().Addr.String()
	if oldEndpoint == newEndpoint {
		t.Fatalf("expected reconnect with a new local endpoint, got same=%s", newEndpoint)
	}

	msg := []byte("after-reconnect")
	if _, err := MustPeerMux(t, clientV2, serverKey.Public).Write(testProtocolEvent, msg); err != nil {
		t.Fatalf("clientV2 Write failed: %v", err)
	}

	proto, got := ReadFromPeerWithTimeout(t, server, clientKey.Public, 3*time.Second)
	if proto != testProtocolEvent {
		t.Fatalf("server proto after reconnect=%d, want %d", proto, testProtocolEvent)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("server payload after reconnect mismatch: got=%q want=%q", string(got), string(msg))
	}

	info := server.PeerInfo(clientKey.Public)
	if info == nil || info.Endpoint == nil {
		t.Fatalf("server peer info missing after reconnect")
	}
	if info.Endpoint.String() != newEndpoint {
		t.Fatalf("server endpoint after reconnect=%s, want %s", info.Endpoint.String(), newEndpoint)
	}
}

// TestIntegration_KCPService0Stream verifies that a KCP stream on service=0
// can be established and supports bidirectional communication.
func TestIntegration_KCPService0Stream(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		stream, err := MustPeerMux(t, server, clientKey.Public).AcceptStream(0)
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- stream
	}()

	clientStream, err := MustPeerMux(t, client, serverKey.Public).OpenStream(0)
	if err != nil {
		t.Fatalf("client OpenStream(service=0) failed: %v", err)
	}
	defer clientStream.Close()

	request := []byte("kcp-stream-request")
	if _, err := clientStream.Write(request); err != nil {
		t.Fatalf("client stream write failed: %v", err)
	}

	var serverStream net.Conn
	select {
	case serverStream = <-acceptCh:
		defer serverStream.Close()
	case err := <-errCh:
		t.Fatalf("server AcceptStream failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server AcceptStream timeout")
	}

	if got := ReadExactWithTimeout(t, serverStream, len(request), 5*time.Second); !bytes.Equal(got, request) {
		t.Fatalf("server stream payload mismatch: got=%q want=%q", string(got), string(request))
	}

	reply := []byte("kcp-stream-reply")
	if _, err := serverStream.Write(reply); err != nil {
		t.Fatalf("server stream write failed: %v", err)
	}
	if got := ReadExactWithTimeout(t, clientStream, len(reply), 5*time.Second); !bytes.Equal(got, reply) {
		t.Fatalf("client stream payload mismatch: got=%q want=%q", string(got), string(reply))
	}
}

// TestIntegration_KCPStreamActiveClose verifies that after one end closes,
// the other end's blocking stream Read fails promptly.
func TestIntegration_KCPStreamActiveClose(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		stream, err := MustPeerMux(t, server, clientKey.Public).AcceptStream(0)
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- stream
	}()

	clientStream, err := MustPeerMux(t, client, serverKey.Public).OpenStream(0)
	if err != nil {
		t.Fatalf("client OpenStream failed: %v", err)
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
		t.Fatalf("server AcceptStream failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server AcceptStream timeout")
	}

	if got := ReadExactWithTimeout(t, serverStream, 1, 5*time.Second); !bytes.Equal(got, []byte("x")) {
		t.Fatalf("server stream priming payload mismatch: got=%q want=%q", string(got), "x")
	}

	readErrCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, readErr := serverStream.Read(buf)
		readErrCh <- readErr
	}()

	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	if err := client.Close(); err != nil {
		t.Fatalf("client Close failed: %v", err)
	}

	select {
	case readErr := <-readErrCh:
		if readErr == nil {
			t.Fatal("server stream Read should fail after peer close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server stream Read did not fail within 2s after peer close")
	}

	if took := time.Since(start); took >= 380*time.Millisecond {
		t.Fatalf("active close should return before ACK-timeout path: took=%v", took)
	}
}

// TestIntegration_KCPMultiStreamSoak runs a short end-to-end stream churn test
// to catch regressions in open/accept/close under concurrent load.
func TestIntegration_KCPMultiStreamSoak(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	const rounds = 4
	const perService = 3
	services := []uint64{0, 7}

	for round := range rounds {
		var wg sync.WaitGroup
		errCh := make(chan error, len(services)*perService*2)
		recvCh := make(chan string, len(services)*perService)
		expected := make(map[string]struct{}, len(services)*perService)

		for _, serviceID := range services {
			svc := serviceID
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range perService {
					stream, err := MustPeerMux(t, server, clientKey.Public).AcceptStream(svc)
					if err != nil {
						errCh <- err
						return
					}
					buf := make([]byte, 64)
					n, err := stream.Read(buf)
					_ = stream.Close()
					if err != nil {
						errCh <- err
						return
					}
					recvCh <- fmt.Sprintf("%d:%s", svc, string(buf[:n]))
				}
			}()
		}

		time.Sleep(20 * time.Millisecond)

		for _, serviceID := range services {
			for i := range perService {
				payload := []byte(fmt.Sprintf("round-%d-service-%d-stream-%d", round, serviceID, i))
				expected[fmt.Sprintf("%d:%s", serviceID, string(payload))] = struct{}{}

				wg.Add(1)
				go func(service uint64, msg []byte) {
					defer wg.Done()
					clientStream, err := MustPeerMux(t, client, serverKey.Public).OpenStream(service)
					if err != nil {
						errCh <- err
						return
					}
					if _, err := clientStream.Write(msg); err != nil {
						errCh <- err
					}
				}(serviceID, payload)
			}
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				t.Fatal(err)
			}
		}
		close(recvCh)
		for got := range recvCh {
			delete(expected, got)
		}
		if len(expected) != 0 {
			t.Fatalf("missing payloads after round %d: %v", round, expected)
		}
	}
}

// TestIntegration_KCPService0AndNonZeroConcurrentActivity verifies that
// service 0 and a non-zero service can be active concurrently on the same peer
// without cross-talk or blocking each other.
func TestIntegration_KCPService0AndNonZeroConcurrentActivity(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	serverSvc0, clientSvc0 := openAcceptedStreamPair(t, server, client, clientKey.Public, serverKey.Public, 0)
	defer serverSvc0.Close()
	defer clientSvc0.Close()
	serverSvc7, clientSvc7 := openAcceptedStreamPair(t, server, client, clientKey.Public, serverKey.Public, 7)
	defer serverSvc7.Close()
	defer clientSvc7.Close()

	errCh := make(chan error, 4)
	done := make(chan string, 2)
	svc0RespRead := make(chan struct{})
	svc7RespRead := make(chan struct{})
	var clientWG sync.WaitGroup

	go func() {
		req := []byte(`{"method":"ping"}`)
		if got := ReadExactWithTimeout(t, serverSvc0, len(req), 5*time.Second); !bytes.Equal(got, req) {
			errCh <- fmt.Errorf("service 0 req mismatch: got=%q want=%q", got, req)
			return
		}
		resp := []byte(`{"ok":true}`)
		if _, err := serverSvc0.Write(resp); err != nil {
			errCh <- err
			return
		}
		<-svc0RespRead
		done <- "svc0"
	}()

	go func() {
		req := []byte("nonzero-request")
		if got := ReadExactWithTimeout(t, serverSvc7, len(req), 5*time.Second); !bytes.Equal(got, req) {
			errCh <- fmt.Errorf("service 7 req mismatch: got=%q want=%q", got, req)
			return
		}
		resp := []byte("nonzero-response")
		if _, err := serverSvc7.Write(resp); err != nil {
			errCh <- err
			return
		}
		<-svc7RespRead
		done <- "svc7"
	}()

	clientWG.Add(1)
	go func() {
		defer clientWG.Done()
		req := []byte(`{"method":"ping"}`)
		if _, err := clientSvc0.Write(req); err != nil {
			errCh <- err
			return
		}
		resp := []byte(`{"ok":true}`)
		if got := ReadExactWithTimeout(t, clientSvc0, len(resp), 5*time.Second); !bytes.Equal(got, resp) {
			errCh <- fmt.Errorf("service 0 resp mismatch: got=%q want=%q", got, resp)
			return
		}
		close(svc0RespRead)
	}()

	clientWG.Add(1)
	go func() {
		defer clientWG.Done()
		req := []byte("nonzero-request")
		if _, err := clientSvc7.Write(req); err != nil {
			errCh <- err
			return
		}
		resp := []byte("nonzero-response")
		if got := ReadExactWithTimeout(t, clientSvc7, len(resp), 5*time.Second); !bytes.Equal(got, resp) {
			errCh <- fmt.Errorf("service 7 resp mismatch: got=%q want=%q", got, resp)
			return
		}
		close(svc7RespRead)
	}()

	for range 2 {
		select {
		case <-done:
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent service activity timeout")
		}
	}

	clientWG.Wait()

	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
}

func openAcceptedStreamPair(t *testing.T, server, client *giznet.UDP, clientPK, serverPK giznet.PublicKey, service uint64) (net.Conn, net.Conn) {
	t.Helper()

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	acceptReady := make(chan struct{})
	go func() {
		close(acceptReady)
		stream, err := MustPeerMux(t, server, clientPK).AcceptStream(service)
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- stream
	}()

	<-acceptReady
	time.Sleep(50 * time.Millisecond)

	clientStream, err := MustPeerMux(t, client, serverPK).OpenStream(service)
	if err != nil {
		t.Fatalf("OpenStream(service=%d) err=%v", service, err)
	}

	var serverStream net.Conn
	select {
	case serverStream = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("AcceptStream(service=%d) err=%v", service, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("AcceptStream(service=%d) timeout", service)
	}
	return serverStream, clientStream
}

// TestIntegration_KCPNonZeroServiceStream tests that non-zero service streams
// can be opened and accepted end-to-end through the foundation layer.
func TestIntegration_KCPNonZeroServiceStream(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	acceptCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		accepted, err := MustPeerMux(t, server, clientKey.Public).AcceptStream(7)
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- accepted
	}()

	time.Sleep(50 * time.Millisecond)

	stream, err := MustPeerMux(t, client, serverKey.Public).OpenStream(7)
	if err != nil {
		t.Fatalf("OpenStream(service=7) err=%v", err)
	}
	defer stream.Close()

	msg := []byte("non-zero-service")
	if _, err := stream.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var accepted net.Conn
	select {
	case accepted = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("AcceptStream(7) err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("AcceptStream(7) timeout")
	}
	defer accepted.Close()

	got := ReadExactWithTimeout(t, accepted, len(msg), 5*time.Second)
	if !bytes.Equal(got, msg) {
		t.Fatalf("payload mismatch: got=%q want=%q", got, msg)
	}
}

// TestIntegration_RPCBidirectionalOverKCPStream verifies bidirectional RPC
// request/response: one call from A->B and one from B->A both complete.
func TestIntegration_RPCBidirectionalOverKCPStream(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	assertRPC := func(caller, callee *giznet.UDP, callerPK, calleePK giznet.PublicKey, req, resp []byte) {
		t.Helper()

		acceptCh := make(chan net.Conn, 1)
		errCh := make(chan error, 1)
		go func() {
			stream, err := MustPeerMux(t, callee, callerPK).AcceptStream(0)
			if err != nil {
				errCh <- err
				return
			}
			acceptCh <- stream
		}()

		callerStream, err := MustPeerMux(t, caller, calleePK).OpenStream(0)
		if err != nil {
			t.Fatalf("OpenStream failed: %v", err)
		}
		defer callerStream.Close()

		if _, err := callerStream.Write(req); err != nil {
			t.Fatalf("caller write req failed: %v", err)
		}

		var calleeStream net.Conn
		select {
		case calleeStream = <-acceptCh:
			defer calleeStream.Close()
		case err := <-errCh:
			t.Fatalf("AcceptStream failed: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("AcceptStream timeout")
		}

		if got := ReadExactWithTimeout(t, calleeStream, len(req), 5*time.Second); !bytes.Equal(got, req) {
			t.Fatalf("callee read req mismatch: got=%q want=%q", string(got), string(req))
		}

		if _, err := calleeStream.Write(resp); err != nil {
			t.Fatalf("callee write resp failed: %v", err)
		}
		if got := ReadExactWithTimeout(t, callerStream, len(resp), 5*time.Second); !bytes.Equal(got, resp) {
			t.Fatalf("caller read resp mismatch: got=%q want=%q", string(got), string(resp))
		}
	}

	assertRPC(client, server, clientKey.Public, serverKey.Public, []byte(`{"method":"ping"}`), []byte(`{"ok":true}`))
	assertRPC(server, client, serverKey.Public, clientKey.Public, []byte(`{"method":"echo"}`), []byte(`{"msg":"ok"}`))
}

// TestIntegration_EVENTFireAndForgetBidirectional verifies bidirectional
// fire-and-forget EVENT delivery: 64 events each way without blocking.
func TestIntegration_EVENTFireAndForgetBidirectional(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	const burst = 64

	start := time.Now()
	for i := range burst {
		msg := []byte(fmt.Sprintf("event-c2s-%02d", i))
		if _, err := MustPeerMux(t, client, serverKey.Public).Write(testProtocolEvent, msg); err != nil {
			t.Fatalf("client EVENT write[%d] failed: %v", i, err)
		}
	}
	for i := range burst {
		msg := []byte(fmt.Sprintf("event-s2c-%02d", i))
		if _, err := MustPeerMux(t, server, clientKey.Public).Write(testProtocolEvent, msg); err != nil {
			t.Fatalf("server EVENT write[%d] failed: %v", i, err)
		}
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("EVENT fire-and-forget writes took too long: %s", took)
	}

	serverWant := make(map[string]struct{}, burst)
	for i := range burst {
		serverWant[fmt.Sprintf("event-c2s-%02d", i)] = struct{}{}
	}
	for i := range burst {
		proto, got := ReadFromPeerWithTimeout(t, server, clientKey.Public, 3*time.Second)
		if proto != testProtocolEvent {
			t.Fatalf("server EVENT proto[%d]=%d, want %d", i, proto, testProtocolEvent)
		}
		if _, ok := serverWant[string(got)]; !ok {
			t.Fatalf("server EVENT payload[%d] unexpected: got=%q remaining=%v", i, string(got), serverWant)
		}
		delete(serverWant, string(got))
	}

	clientWant := make(map[string]struct{}, burst)
	for i := range burst {
		clientWant[fmt.Sprintf("event-s2c-%02d", i)] = struct{}{}
	}
	for i := range burst {
		proto, got := ReadFromPeerWithTimeout(t, client, serverKey.Public, 3*time.Second)
		if proto != testProtocolEvent {
			t.Fatalf("client EVENT proto[%d]=%d, want %d", i, proto, testProtocolEvent)
		}
		if _, ok := clientWant[string(got)]; !ok {
			t.Fatalf("client EVENT payload[%d] unexpected: got=%q remaining=%v", i, string(got), clientWant)
		}
		delete(clientWant, string(got))
	}
}

// TestIntegration_OPUSFramesDelivered verifies that 40 consecutive OPUS frames
// are received with correct protocol identification.
func TestIntegration_OPUSFramesDelivered(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	const frames = 40
	for i := range frames {
		frame := []byte(fmt.Sprintf("opus-frame-%03d", i))
		if _, err := MustPeerMux(t, client, serverKey.Public).Write(testProtocolOpus, frame); err != nil {
			t.Fatalf("client OPUS write[%d] failed: %v", i, err)
		}
	}

	wantFrames := make(map[string]struct{}, frames)
	for i := range frames {
		wantFrames[fmt.Sprintf("opus-frame-%03d", i)] = struct{}{}
	}
	for i := range frames {
		proto, got := ReadFromPeerWithTimeout(t, server, clientKey.Public, 3*time.Second)
		if proto != testProtocolOpus {
			t.Fatalf("server OPUS proto[%d]=%d, want %d", i, proto, testProtocolOpus)
		}
		if _, ok := wantFrames[string(got)]; !ok {
			t.Fatalf("server OPUS payload[%d] unexpected: got=%q remaining=%v", i, string(got), wantFrames)
		}
		delete(wantFrames, string(got))
	}
}

// TestIntegration_UnknownPeerOperations verifies that PeerServiceMux returns
// ErrPeerNotFound for an unknown peer.
func TestIntegration_UnknownPeerOperations(t *testing.T) {
	localKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate local key failed: %v", err)
	}
	unknownKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate unknown key failed: %v", err)
	}

	u := NewUDPNode(t, localKey)
	defer u.Close()

	if _, err := u.PeerServiceMux(unknownKey.Public); err != giznet.ErrPeerNotFound {
		t.Fatalf("PeerServiceMux(unknown peer) err=%v, want %v", err, giznet.ErrPeerNotFound)
	}
}

// TestIntegration_StreamBeforeSession verifies that PeerServiceMux returns
// ErrNoSession when the peer is registered but the handshake is not complete.
func TestIntegration_StreamBeforeSession(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	// Register endpoints only, without Connect (peer exists but no session).
	client.SetPeerEndpoint(serverKey.Public, server.HostInfo().Addr)
	server.SetPeerEndpoint(clientKey.Public, client.HostInfo().Addr)

	if _, err := client.PeerServiceMux(serverKey.Public); err != giznet.ErrNoSession {
		t.Fatalf("PeerServiceMux(before session) err=%v, want %v", err, giznet.ErrNoSession)
	}
}

// TestIntegration_ClosedNodeOperations verifies that PeerServiceMux returns
// ErrUDPClosed after the node is closed.
func TestIntegration_ClosedNodeOperations(t *testing.T) {
	localKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate local key failed: %v", err)
	}
	peerKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate peer key failed: %v", err)
	}

	u := NewUDPNode(t, localKey)
	if err := u.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := u.PeerServiceMux(peerKey.Public); err != giznet.ErrUDPClosed {
		t.Fatalf("PeerServiceMux(after close) err=%v, want %v", err, giznet.ErrUDPClosed)
	}
}

// TestIntegration_ZeroLengthPayloads verifies that EVENT/OPUS support
// zero-length payloads and the receiver correctly identifies the protocol.
func TestIntegration_ZeroLengthPayloads(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Generate client key failed: %v", err)
	}

	server := NewUDPNode(t, serverKey)
	defer server.Close()
	client := NewUDPNode(t, clientKey)
	defer client.Close()

	ConnectNodes(t, client, clientKey, server, serverKey)

	tests := []struct {
		name  string
		proto byte
	}{
		{name: "EVENT empty payload", proto: testProtocolEvent},
		{name: "OPUS empty payload", proto: testProtocolOpus},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, err := MustPeerMux(t, client, serverKey.Public).Write(tc.proto, nil)
			if err != nil {
				t.Fatalf("Write(empty payload) failed: %v", err)
			}
			if n != 0 {
				t.Fatalf("Write(empty payload) bytes=%d, want 0", n)
			}

			proto, got := ReadFromPeerWithTimeout(t, server, clientKey.Public, 3*time.Second)
			if proto != tc.proto {
				t.Fatalf("Read proto=%d, want %d", proto, tc.proto)
			}
			if len(got) != 0 {
				t.Fatalf("Read payload len=%d, want 0", len(got))
			}
		})
	}
}
