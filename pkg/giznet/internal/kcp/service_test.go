package kcp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func serviceMuxPair() (client, server *ServiceMux) {
	var clientMux, serverMux *ServiceMux

	clientMux = NewServiceMux(ServiceMuxConfig{
		IsClient: true,
		Output: func(service uint64, data []byte) error {
			return serverMux.Input(service, data)
		},
	})
	serverMux = NewServiceMux(ServiceMuxConfig{
		IsClient: false,
		Output: func(service uint64, data []byte) error {
			return clientMux.Input(service, data)
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

func TestServiceMux_OutputErrorsReportedOnOpen(t *testing.T) {
	writeErr := errors.New("boom")
	reported := make(chan error, 1)

	mux := NewServiceMux(ServiceMuxConfig{
		Output: func(service uint64, data []byte) error {
			return writeErr
		},
		OnOutputError: func(service uint64, err error) {
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
	mux := NewServiceMux(ServiceMuxConfig{})
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
