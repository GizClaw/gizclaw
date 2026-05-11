package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/kcp"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/noise"
)

func routeServiceMuxInput(dst *ServiceMux, service uint64, protocol byte, data []byte) error {
	if protocol == ProtocolKCP {
		return dst.InputKCP(service, data)
	}
	return dst.InputPacket(protocol, data)
}

func serviceMuxPair() (client, server *ServiceMux) {
	var clientMux, serverMux *ServiceMux

	clientMux = NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		IsClient: true,
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return routeServiceMuxInput(serverMux, service, protocol, data)
		},
	})
	serverMux = NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		IsClient: false,
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return routeServiceMuxInput(clientMux, service, protocol, data)
		},
	})

	return clientMux, serverMux
}

func readServiceMuxExactWithTimeout(t *testing.T, r io.Reader, n int, timeout time.Duration) []byte {
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

func TestServiceMux_OpenCreatesDistinctStreams(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	const serviceID uint64 = 7
	acceptedCh := make(chan net.Conn, 2)
	for range 2 {
		go func() {
			conn, err := server.AcceptStream(serviceID)
			if err != nil {
				t.Errorf("AcceptStream failed: %v", err)
				return
			}
			acceptedCh <- conn
		}()
	}

	time.Sleep(50 * time.Millisecond)

	stream1, err := client.OpenStream(serviceID)
	if err != nil {
		t.Fatalf("OpenStream(1) failed: %v", err)
	}
	defer stream1.Close()

	stream2, err := client.OpenStream(serviceID)
	if err != nil {
		t.Fatalf("OpenStream(2) failed: %v", err)
	}
	defer stream2.Close()

	if stream1 == stream2 {
		t.Fatal("expected distinct streams for the same service")
	}

	accepted1 := <-acceptedCh
	defer accepted1.Close()

	accepted2 := <-acceptedCh
	defer accepted2.Close()

	if accepted1 == accepted2 {
		t.Fatal("expected distinct accepted streams")
	}
}

func TestServiceMux_AcceptStreamRoutesSpecificService(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	const serviceA uint64 = 1
	const serviceB uint64 = 3

	gotA := make(chan net.Conn, 1)
	go func() {
		conn, err := server.AcceptStream(serviceA)
		if err != nil {
			t.Errorf("AcceptStream(%d) failed: %v", serviceA, err)
			return
		}
		gotA <- conn
	}()

	time.Sleep(50 * time.Millisecond)

	streamB, err := client.OpenStream(serviceB)
	if err != nil {
		t.Fatalf("OpenStream(serviceB) failed: %v", err)
	}
	defer streamB.Close()

	select {
	case <-gotA:
		t.Fatalf("AcceptStream(%d) should not receive service %d", serviceA, serviceB)
	case <-time.After(150 * time.Millisecond):
	}

	streamA, err := client.OpenStream(serviceA)
	if err != nil {
		t.Fatalf("OpenStream(serviceA) failed: %v", err)
	}
	defer streamA.Close()

	select {
	case conn := <-gotA:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatalf("AcceptStream(%d) did not unblock", serviceA)
	}
}

func TestServiceMux_BidirectionalDataPath(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	acceptedCh := make(chan net.Conn, 1)
	go func() {
		conn, err := server.AcceptStream(0)
		if err != nil {
			t.Errorf("AcceptStream failed: %v", err)
			return
		}
		acceptedCh <- conn
	}()

	time.Sleep(50 * time.Millisecond)

	clientStream, err := client.OpenStream(0)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer clientStream.Close()

	serverStream := <-acceptedCh
	defer serverStream.Close()

	request := []byte("req")
	if _, err := clientStream.Write(request); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
	if got := readServiceMuxExactWithTimeout(t, serverStream, len(request), 5*time.Second); !bytes.Equal(got, request) {
		t.Fatalf("server recv req mismatch: got=%q want=%q", got, request)
	}

	response := []byte("resp")
	if _, err := serverStream.Write(response); err != nil {
		t.Fatalf("server write failed: %v", err)
	}
	if got := readServiceMuxExactWithTimeout(t, clientStream, len(response), 5*time.Second); !bytes.Equal(got, response) {
		t.Fatalf("client recv resp mismatch: got=%q want=%q", got, response)
	}
}

func TestServiceMux_ReadRoutesDirectPackets(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	want := []byte("read-direct-event")
	done := make(chan struct {
		proto byte
		n     int
		err   error
	}, 1)

	go func() {
		buf := make([]byte, 64)
		proto, n, err := server.Read(buf)
		if err == nil && !bytes.Equal(buf[:n], want) {
			err = errors.New("read payload mismatch")
		}
		done <- struct {
			proto byte
			n     int
			err   error
		}{proto: proto, n: n, err: err}
	}()

	if _, err := client.Write(testDirectProtoA, want); err != nil {
		t.Fatalf("Write(direct) failed: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("Read() failed: %v", result.err)
		}
		if result.proto != testDirectProtoA {
			t.Fatalf("Read() proto=%d, want %d", result.proto, testDirectProtoA)
		}
		if result.n != len(want) {
			t.Fatalf("Read() n=%d, want %d", result.n, len(want))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read() timed out")
	}
}

func TestServiceMux_StopAcceptingServiceBeforeInitIsSticky(t *testing.T) {
	_, server := serviceMuxPair()
	defer server.Close()

	const serviceID uint64 = 9
	if err := server.StopAcceptingService(serviceID); err != nil {
		t.Fatalf("StopAcceptingService error: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := server.AcceptStream(serviceID)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrAcceptQueueClosed) {
			t.Fatalf("AcceptStream err = %v, want %v", err, ErrAcceptQueueClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AcceptStream should fail immediately after StopAcceptingService")
	}
}

func TestServiceMux_CloseServiceBeforeInitIsSticky(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	const serviceID uint64 = 10
	if err := server.CloseService(serviceID); err != nil {
		t.Fatalf("CloseService error: %v", err)
	}

	if _, err := server.OpenStream(serviceID); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("OpenStream err = %v, want %v", err, ErrServiceMuxClosed)
	}
	if _, err := server.AcceptStream(serviceID); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("AcceptStream err = %v, want %v", err, ErrServiceMuxClosed)
	}
}

func TestServiceMux_ReadDirectPacketsDoesNotCreateKcpMux(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	defer mux.Close()

	if err := mux.InputPacket(testDirectProtoA, []byte("event")); err != nil {
		t.Fatalf("InputPacket failed: %v", err)
	}

	if _, ok := mux.getService(0); ok {
		t.Fatal("direct packet path should not create service state")
	}

	buf := make([]byte, 16)
	proto, n, err := mux.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if proto != testDirectProtoA || string(buf[:n]) != "event" {
		t.Fatalf("Read got proto=%d payload=%q", proto, string(buf[:n]))
	}
	if _, ok := mux.getService(0); ok {
		t.Fatal("Read should not create service state for direct packets")
	}
}

func TestServiceMux_OpenStreamLazilyCreatesKcpMux(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		DefaultKcpMuxConfig: kcp.KcpMuxConfig{CloseAckTimeout: 10 * time.Millisecond},
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return nil
		},
	})
	defer mux.Close()

	state, err := mux.getOrCreateService(5)
	if err != nil {
		t.Fatalf("getOrCreateService failed: %v", err)
	}
	if state.mux != nil {
		t.Fatal("service should start without KcpMux")
	}

	stream, err := mux.OpenStream(5)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer stream.Close()

	if state.mux == nil {
		t.Fatal("OpenStream should lazily create KcpMux")
	}
}

func TestServiceMux_ReadRoutesSecondDirectProtocol(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	defer mux.Close()

	done := make(chan struct {
		proto byte
		n     int
		err   error
	}, 1)
	want := []byte("second-direct-payload")

	go func() {
		buf := make([]byte, 64)
		proto, n, err := mux.Read(buf)
		if err == nil && !bytes.Equal(buf[:n], want) {
			err = errors.New("direct payload mismatch")
		}
		done <- struct {
			proto byte
			n     int
			err   error
		}{proto: proto, n: n, err: err}
	}()

	if err := mux.InputPacket(testDirectProtoB, want); err != nil {
		t.Fatalf("InputPacket(proto B) failed: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("Read() failed: %v", result.err)
		}
		if result.proto != testDirectProtoB {
			t.Fatalf("Read() proto=%d, want %d", result.proto, testDirectProtoB)
		}
		if result.n != len(want) {
			t.Fatalf("Read() n=%d, want %d", result.n, len(want))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read() timed out")
	}
}

func TestServiceMux_NumStreamsCountsActiveStreams(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	if got := client.NumStreams(); got != 0 {
		t.Fatalf("client.NumStreams()=%d, want 0", got)
	}
	if got := server.NumStreams(); got != 0 {
		t.Fatalf("server.NumStreams()=%d, want 0", got)
	}

	accepted := make(chan net.Conn, 2)
	for _, service := range []uint64{0, 7} {
		svc := service
		go func() {
			conn, err := server.AcceptStream(svc)
			if err != nil {
				t.Errorf("AcceptStream(%d) failed: %v", svc, err)
				return
			}
			accepted <- conn
		}()
	}

	time.Sleep(50 * time.Millisecond)

	stream0, err := client.OpenStream(0)
	if err != nil {
		t.Fatalf("OpenStream(0) failed: %v", err)
	}
	defer stream0.Close()

	stream7, err := client.OpenStream(7)
	if err != nil {
		t.Fatalf("OpenStream(7) failed: %v", err)
	}
	defer stream7.Close()

	serverStream0 := <-accepted
	defer serverStream0.Close()
	serverStream7 := <-accepted
	defer serverStream7.Close()

	if got := client.NumStreams(); got != 2 {
		t.Fatalf("client.NumStreams()=%d, want 2", got)
	}
	if got := server.NumStreams(); got != 2 {
		t.Fatalf("server.NumStreams()=%d, want 2", got)
	}
}

func TestServiceMux_UnregisteredServiceOpenGetsAbort(t *testing.T) {
	client, server := serviceMuxPair()
	defer client.Close()
	defer server.Close()

	stream, err := client.OpenStream(9)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer stream.Close()

	buf := make([]byte, 1)
	_ = stream.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = stream.Read(buf)
	if err == nil {
		t.Fatal("expected stream to be aborted")
	}
	if errors.Is(err, io.EOF) {
		t.Fatalf("expected abort-like error, got EOF")
	}
}

func TestServiceMux_RejectedServiceOpenGetsAbort(t *testing.T) {
	var clientMux, serverMux *ServiceMux

	clientMux = NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		IsClient: true,
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return routeServiceMuxInput(serverMux, service, protocol, data)
		},
	})
	serverMux = NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		IsClient: false,
		OnNewService: func(peer noise.PublicKey, service uint64) bool {
			return false
		},
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return routeServiceMuxInput(clientMux, service, protocol, data)
		},
	})
	defer clientMux.Close()
	defer serverMux.Close()

	stream, err := clientMux.OpenStream(9)
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	defer stream.Close()

	buf := make([]byte, 1)
	_ = stream.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = stream.Read(buf)
	if err == nil {
		t.Fatal("expected stream to be aborted")
	}
	if errors.Is(err, io.EOF) {
		t.Fatalf("expected abort-like error, got EOF")
	}
}

func TestServiceMux_OutputErrorsReportedOnOpen(t *testing.T) {
	writeErr := errors.New("boom")
	reported := make(chan error, 1)

	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return writeErr
		},
		OnOutputError: func(peer noise.PublicKey, service uint64, err error) {
			reported <- err
		},
	})
	defer mux.Close()

	if _, err := mux.OpenStream(0); !errors.Is(err, writeErr) {
		t.Fatalf("OpenStream err=%v, want %v", err, writeErr)
	}

	select {
	case err := <-reported:
		if !errors.Is(err, writeErr) {
			t.Fatalf("reported err=%v, want %v", err, writeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected output error callback")
	}

	if got := mux.OutputErrorCount(); got == 0 {
		t.Fatal("expected output error count to increase")
	}
}

func TestServiceMux_CloseUnblocksAcceptOn(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	done := make(chan error, 1)

	go func() {
		_, err := mux.AcceptStream(12)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if err := mux.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrServiceMuxClosed) {
			t.Fatalf("AcceptStream err=%v, want %v", err, ErrServiceMuxClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AcceptStream did not unblock on Close")
	}
}

func TestServiceMux_InputPacketAndKCPRejectedService(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		OnNewService: func(peer noise.PublicKey, service uint64) bool {
			return false
		},
	})
	defer mux.Close()

	if err := mux.InputPacket(0xFF, []byte("x")); err != nil {
		t.Fatalf("InputPacket(0xFF) err=%v, want nil", err)
	}
	openFrame := binary.AppendUvarint(nil, 0)
	openFrame = append(openFrame, serviceMuxFrameOpen)
	if err := mux.InputKCP(1, openFrame); err != nil {
		t.Fatalf("InputKCP(rejected service) err=%v, want nil", err)
	}
}

func TestServiceMux_ReadUnblocksOnClose(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})

	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, _, err := mux.Read(buf)
		readDone <- err
	}()

	readDone2 := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, _, err := mux.Read(buf)
		readDone2 <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if err := mux.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case err := <-readDone:
		if !errors.Is(err, ErrServiceMuxClosed) {
			t.Fatalf("Read unblock err=%v, want %v", err, ErrServiceMuxClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not unblock on Close")
	}

	select {
	case err := <-readDone2:
		if !errors.Is(err, ErrServiceMuxClosed) {
			t.Fatalf("second Read unblock err=%v, want %v", err, ErrServiceMuxClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second Read did not unblock on Close")
	}
}

func TestServiceMux_ReadDropsPacketWhenCloseWinsAfterReceive(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		OnNewService: func(peer noise.PublicKey, service uint64) bool {
			return true
		},
	})
	defer func() {
		afterServiceMuxDirectReadHook = nil
		_ = mux.Close()
	}()

	if err := mux.InputPacket(testDirectProtoA, []byte("queued")); err != nil {
		t.Fatalf("InputPacket failed: %v", err)
	}

	afterServiceMuxDirectReadHook = func() {
		if err := mux.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}

	buf := make([]byte, 16)
	_, n, err := mux.Read(buf)
	if !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("Read after close race err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if n != 0 {
		t.Fatalf("Read returned n=%d, want 0 when close wins", n)
	}
}

func TestServiceMux_DirectPathRejectsKCPProtocolBytes(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	defer mux.Close()

	if err := mux.InputPacket(ProtocolKCP, []byte("x")); !errors.Is(err, ErrKCPMustUseStream) {
		t.Fatalf("InputPacket(KCP) err=%v, want %v", err, ErrKCPMustUseStream)
	}
}

func TestServiceMux_WriteErrorPaths(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	defer mux.Close()

	if _, err := mux.Write(ProtocolKCP, []byte("x")); !errors.Is(err, ErrKCPMustUseStream) {
		t.Fatalf("Write(KCP) err=%v, want %v", err, ErrKCPMustUseStream)
	}
	if _, err := mux.Write(0xFF, []byte("x")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("Write(without Output) err=%v, want %v", err, ErrNoSession)
	}
	if _, err := mux.Write(testDirectProtoA, []byte("x")); !errors.Is(err, ErrNoSession) {
		t.Fatalf("Write(without Output) err=%v, want %v", err, ErrNoSession)
	}

	wantErr := errors.New("boom")
	reported := make(chan error, 1)
	muxWithOutput := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{
		Output: func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error {
			return wantErr
		},
		OnOutputError: func(peer noise.PublicKey, service uint64, err error) {
			reported <- err
		},
	})
	defer muxWithOutput.Close()

	if _, err := muxWithOutput.Write(testDirectProtoA, []byte("x")); !errors.Is(err, wantErr) {
		t.Fatalf("Write(output error) err=%v, want %v", err, wantErr)
	}
	select {
	case err := <-reported:
		if !errors.Is(err, wantErr) {
			t.Fatalf("reported output err=%v, want %v", err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected output error callback from Write")
	}
	if got := muxWithOutput.OutputErrorCount(); got != 1 {
		t.Fatalf("OutputErrorCount=%d, want 1", got)
	}
}

func TestServiceMux_ClosedStateOperations(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	if err := mux.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := mux.InputPacket(testDirectProtoA, []byte("x")); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("InputPacket(after close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if err := mux.InputKCP(0, []byte{0}); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("InputKCP(after close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if _, _, err := mux.Read(make([]byte, 1)); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("Read(after close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if _, err := mux.Write(testDirectProtoA, []byte("x")); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("Write(after close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if _, err := mux.OpenStream(0); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("OpenStream(after close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if _, err := mux.AcceptStream(0); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("AcceptStream(after close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
}

func TestServiceMux_InputReturnsInboundQueueFull(t *testing.T) {
	mux := NewServiceMux(noise.PublicKey{}, ServiceMuxConfig{})
	defer mux.Close()

	for i := range InboundChanSize {
		if err := mux.InputPacket(testDirectProtoA, []byte("x")); err != nil {
			t.Fatalf("InputPacket fill[%d] failed: %v", i, err)
		}
	}
	if err := mux.InputPacket(testDirectProtoA, []byte("overflow")); !errors.Is(err, ErrInboundQueueFull) {
		t.Fatalf("InputPacket overflow err=%v, want %v", err, ErrInboundQueueFull)
	}
}

// TestClosedChanGoroutineLeak verifies that AcceptStream does not leak
// goroutines via closedChan().
func TestClosedChanGoroutineLeak(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	// Call AcceptStream multiple times across fresh connected pairs. After each
	// AcceptStream returns, the closedChan goroutine should exit.
	const iterations = 10
	for i := range iterations {
		server, client, serverKey, clientKey := createConnectedPair(t)

		accepted := make(chan io.ReadWriteCloser, 1)
		go func() {
			s, err := mustServiceMux(t, server, clientKey.Public).AcceptStream(0)
			if err != nil {
				return
			}
			accepted <- s
		}()

		time.Sleep(20 * time.Millisecond)
		cs, err := mustServiceMux(t, client, serverKey.Public).OpenStream(0)
		if err != nil {
			t.Fatalf("OpenStream %d: %v", i, err)
		}
		if _, err := cs.Write([]byte("x")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}

		select {
		case ss := <-accepted:
			ss.Close()
		case <-time.After(3 * time.Second):
			t.Fatalf("Timeout waiting for AcceptStream %d", i)
		}
		cs.Close()
		_ = server.Close()
		_ = client.Close()
	}

	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()
	leaked := after - baseline

	if leaked > 5 {
		t.Errorf("GOROUTINE LEAK: %d goroutines leaked after %d AcceptStream calls "+
			"(baseline=%d, after=%d). closedChan() likely leaking goroutines.",
			leaked, iterations, baseline, after)
	}
}
