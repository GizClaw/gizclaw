package kcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	gokcp "github.com/xtaci/kcp-go/v5"
)

func kcpMuxPair(t *testing.T, cfg KcpMuxConfig) (*KcpMux, *KcpMux) {
	t.Helper()

	var clientMux, serverMux *KcpMux
	clientMux = NewKcpMux(0, true, cfg, func(service uint64, data []byte) error {
		return serverMux.Input(data)
	}, nil)
	serverMux = NewKcpMux(0, false, cfg, func(service uint64, data []byte) error {
		return clientMux.Input(data)
	}, nil)
	return clientMux, serverMux
}

func acceptWithTimeout(t *testing.T, mux *KcpMux, timeout time.Duration) net.Conn {
	t.Helper()
	result := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := mux.Accept()
		result <- struct {
			conn net.Conn
			err  error
		}{conn: conn, err: err}
	}()

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("Accept failed: %v", got.err)
		}
		return got.conn
	case <-time.After(timeout):
		t.Fatalf("Accept timed out after %s", timeout)
		return nil
	}
}

func muxControlFrame(frameType byte, streamID uint64, payload ...byte) []byte {
	frame := []byte{frameType}
	frame = binary.LittleEndian.AppendUint32(frame, uint32(streamID))
	frame = append(frame, payload...)
	return frame
}

func muxDataFrame(conv uint32) []byte {
	segment := make([]byte, gokcp.IKCP_OVERHEAD)
	binary.LittleEndian.PutUint32(segment, conv)
	return append([]byte{kcpMuxFrameData}, segment...)
}

func TestDefaultKcpMuxConfigUsesKCPIdleTimeout(t *testing.T) {
	cfg := defaultKcpMuxConfig(KcpMuxConfig{})
	if cfg.IdleStreamTimeout != kcpDefaultIdleTimeout {
		t.Fatalf("IdleStreamTimeout=%s, want %s", cfg.IdleStreamTimeout, kcpDefaultIdleTimeout)
	}
}

func TestKcpMux_OpenCreatesDistinctStreams(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	stream1, err := client.Open()
	if err != nil {
		t.Fatalf("Open(1) failed: %v", err)
	}
	defer stream1.Close()

	stream2, err := client.Open()
	if err != nil {
		t.Fatalf("Open(2) failed: %v", err)
	}
	defer stream2.Close()

	if stream1 == stream2 {
		t.Fatal("expected distinct streams")
	}
	if got := client.NumStreams(); got != 2 {
		t.Fatalf("NumStreams=%d, want 2", got)
	}

	serverStream1 := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream1.Close()
	serverStream2 := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream2.Close()
	if serverStream1 == serverStream2 {
		t.Fatal("expected distinct accepted streams")
	}
}

func TestKcpMux_StopAcceptingClosesQueuedStreams(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	stream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer stream.Close()

	deadline := time.Now().Add(2 * time.Second)
	for server.NumStreams() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := server.NumStreams(); got != 1 {
		t.Fatalf("server NumStreams=%d, want queued stream", got)
	}
	if err := server.StopAccepting(); err != nil {
		t.Fatalf("StopAccepting() error = %v", err)
	}
	if got := server.NumStreams(); got != 0 {
		t.Fatalf("server NumStreams after StopAccepting=%d, want 0", got)
	}
	if _, err := server.Accept(); !errors.Is(err, ErrAcceptQueueClosed) {
		t.Fatalf("Accept() after StopAccepting error = %v, want %v", err, ErrAcceptQueueClosed)
	}
}

func TestKcpMux_OpenThenDataBidirectional(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	clientStream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer clientStream.Close()
	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()

	request := []byte("hello")
	if _, err := clientStream.Write(request); err != nil {
		t.Fatalf("Write(request) failed: %v", err)
	}
	if got := readExactWithTimeout(t, serverStream, len(request), 5*time.Second); !bytes.Equal(got, request) {
		t.Fatalf("request mismatch: got=%q want=%q", got, request)
	}

	response := []byte("world")
	if _, err := serverStream.Write(response); err != nil {
		t.Fatalf("Write(response) failed: %v", err)
	}
	if got := readExactWithTimeout(t, clientStream, len(response), 5*time.Second); !bytes.Equal(got, response) {
		t.Fatalf("response mismatch: got=%q want=%q", got, response)
	}
}

func TestKcpMux_CloseOneStreamDoesNotAffectOthers(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	streamA, err := client.Open()
	if err != nil {
		t.Fatalf("Open(A) failed: %v", err)
	}
	defer streamA.Close()
	streamB, err := client.Open()
	if err != nil {
		t.Fatalf("Open(B) failed: %v", err)
	}
	defer streamB.Close()

	serverA := acceptWithTimeout(t, server, 2*time.Second)
	defer serverA.Close()
	serverB := acceptWithTimeout(t, server, 2*time.Second)
	defer serverB.Close()

	if err := streamA.Close(); err != nil {
		t.Fatalf("streamA.Close failed: %v", err)
	}

	payload := []byte("still-alive")
	if _, err := streamB.Write(payload); err != nil {
		t.Fatalf("streamB.Write failed: %v", err)
	}

	got := readExactWithTimeout(t, serverB, len(payload), 5*time.Second)
	if !bytes.Equal(got, payload) {
		t.Fatalf("streamB payload mismatch: got=%q want=%q", got, payload)
	}
}

func TestKcpMux_OpenFramesAreIdempotent(t *testing.T) {
	server := NewKcpMux(0, false, KcpMuxConfig{}, nil, nil)
	defer server.Close()

	frame := muxControlFrame(kcpMuxFrameOpen, 1)

	if err := server.Input(frame); err != nil {
		t.Fatalf("first Input failed: %v", err)
	}
	if err := server.Input(frame); err != nil {
		t.Fatalf("second Input failed: %v", err)
	}
	if got := server.NumStreams(); got != 1 {
		t.Fatalf("NumStreams=%d, want 1", got)
	}
}

func TestKcpMux_DataBeforeOpenIsTolerated(t *testing.T) {
	var (
		mu     sync.Mutex
		queued [][]byte
	)

	server := NewKcpMux(0, false, KcpMuxConfig{}, nil, nil)
	defer server.Close()
	client := NewKcpMux(0, true, KcpMuxConfig{}, func(service uint64, data []byte) error {
		_ = service
		mu.Lock()
		queued = append(queued, append([]byte(nil), data...))
		mu.Unlock()
		return nil
	}, nil)
	defer client.Close()

	clientStream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer clientStream.Close()

	payload := []byte("hello-reordered")
	if _, err := clientStream.Write(payload); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var openFrame, dataFrame []byte
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		snapshot := append([][]byte(nil), queued...)
		mu.Unlock()
		for _, frame := range snapshot {
			_, frameType, _, err := decodeMuxFrame(frame)
			if err != nil {
				continue
			}
			switch frameType {
			case kcpMuxFrameOpen:
				openFrame = frame
			case kcpMuxFrameData:
				dataFrame = frame
			}
		}
		if openFrame != nil && dataFrame != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if openFrame == nil || dataFrame == nil {
		t.Fatalf("expected both open and data frames, got open=%v data=%v", openFrame != nil, dataFrame != nil)
	}

	if err := server.Input(dataFrame); err != nil {
		t.Fatalf("Input(data before open) failed: %v", err)
	}
	if err := server.Input(openFrame); err != nil {
		t.Fatalf("Input(open after data) failed: %v", err)
	}

	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()
	if got := readExactWithTimeout(t, serverStream, len(payload), 5*time.Second); !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got=%q want=%q", got, payload)
	}
}

func TestKcpMux_CloseAckTimeoutForcesCleanup(t *testing.T) {
	client := NewKcpMux(0, true, KcpMuxConfig{CloseAckTimeout: 50 * time.Millisecond}, func(service uint64, data []byte) error {
		return nil
	}, nil)
	defer client.Close()

	stream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for client.NumStreams() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := client.NumStreams(); got != 0 {
		t.Fatalf("NumStreams=%d, want 0", got)
	}
}

func TestKcpMux_IdleTimeoutCleansInactiveStream(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{IdleStreamTimeout: 50 * time.Millisecond})
	defer client.Close()
	defer server.Close()

	stream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer stream.Close()
	accepted := acceptWithTimeout(t, server, 2*time.Second)
	defer accepted.Close()

	deadline := time.Now().Add(500 * time.Millisecond)
	for client.NumStreams() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := client.NumStreams(); got != 0 {
		t.Fatalf("client NumStreams=%d, want 0", got)
	}
	if got := server.NumStreams(); got != 0 {
		t.Fatalf("server NumStreams=%d, want 0", got)
	}
}

func TestKcpMux_InvalidFrameClosesStreamWithInvalidReason(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	stream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer stream.Close()
	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()

	frame := muxControlFrame(byte(99), 1)
	if err := server.Input(frame); err != nil {
		t.Fatalf("Input failed: %v", err)
	}

	buf := make([]byte, 1)
	_ = stream.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = stream.Read(buf)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected invalid-close error, got %v", err)
	}
}

func TestKcpMux_ClosedStateOperations(t *testing.T) {
	mux := NewKcpMux(0, true, KcpMuxConfig{}, nil, nil)
	if err := mux.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := mux.Open(); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("Open(after Close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
	if _, err := mux.Accept(); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("Accept(after Close) err=%v, want %v", err, ErrServiceMuxClosed)
	}

	frame := muxControlFrame(kcpMuxFrameOpen, 0)
	if err := mux.Input(frame); !errors.Is(err, ErrServiceMuxClosed) {
		t.Fatalf("Input(after Close) err=%v, want %v", err, ErrServiceMuxClosed)
	}
}

func TestKcpMux_MaxActiveStreamsRejectsExtraRemoteOpen(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{MaxActiveStreams: 1})
	defer client.Close()
	defer server.Close()

	stream1, err := client.Open()
	if err != nil {
		t.Fatalf("Open(first) failed: %v", err)
	}
	defer stream1.Close()
	serverStream1 := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream1.Close()

	stream2, err := client.Open()
	if err != nil {
		t.Fatalf("Open(second) failed: %v", err)
	}
	defer stream2.Close()

	buf := make([]byte, 1)
	_ = stream2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = stream2.Read(buf)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected abort-like error for second stream, got %v", err)
	}
	if got := server.NumStreams(); got != 1 {
		t.Fatalf("server NumStreams=%d, want 1", got)
	}
}

func TestKcpMux_AcceptBacklogFullRejectsExtraRemoteOpen(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{AcceptBacklog: 1, MaxActiveStreams: 4})
	defer client.Close()
	defer server.Close()

	stream1, err := client.Open()
	if err != nil {
		t.Fatalf("Open(first) failed: %v", err)
	}
	defer stream1.Close()

	stream2, err := client.Open()
	if err != nil {
		t.Fatalf("Open(second) failed: %v", err)
	}
	defer stream2.Close()

	buf := make([]byte, 1)
	_ = stream2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = stream2.Read(buf)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected abort-like error for backlog overflow, got %v", err)
	}

	serverStream1 := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream1.Close()
}

func TestDecodeMuxFrameRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "data without segment", data: []byte{kcpMuxFrameData}},
		{name: "control without conv", data: []byte{kcpMuxFrameOpen}},
		{name: "short control conv", data: []byte{kcpMuxFrameOpen, 0x01, 0x02, 0x03}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := decodeMuxFrame(tc.data); !errors.Is(err, ErrInvalidServiceFrame) {
				t.Fatalf("decodeMuxFrame(%s) err=%v, want %v", tc.name, err, ErrInvalidServiceFrame)
			}
		})
	}
}

func TestKcpMux_GracefulRemoteCloseMapsReadAndWriteErrors(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	clientStream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer clientStream.Close()
	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()

	if err := serverStream.Close(); err != nil {
		t.Fatalf("serverStream.Close failed: %v", err)
	}

	buf := make([]byte, 1)
	_ = clientStream.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = clientStream.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("clientStream.Read after remote close err=%v, want %v", err, io.EOF)
	}

	_, err = clientStream.Write([]byte("x"))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("clientStream.Write after remote close err=%v, want %v", err, io.ErrClosedPipe)
	}
}

func TestKcpMux_StreamAccessors(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	clientStream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer clientStream.Close()
	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()

	if clientStream.LocalAddr().Network() != "kcp" || clientStream.RemoteAddr().Network() != "kcp" {
		t.Fatal("stream LocalAddr/RemoteAddr should use kcp network")
	}

	dl := time.Now().Add(500 * time.Millisecond)
	if err := clientStream.SetDeadline(dl); err != nil {
		t.Fatalf("SetDeadline failed: %v", err)
	}
	if err := clientStream.SetWriteDeadline(dl); err != nil {
		t.Fatalf("SetWriteDeadline failed: %v", err)
	}
}

func TestKcpMux_SendFrameReportsOutputErrors(t *testing.T) {
	wantErr := errors.New("boom")
	reported := make(chan error, 1)
	mux := NewKcpMux(0, true, KcpMuxConfig{}, func(service uint64, data []byte) error {
		return wantErr
	}, func(service uint64, err error) {
		reported <- err
	})
	defer mux.Close()

	if err := mux.sendFrame(1, kcpMuxFrameOpen, nil); !errors.Is(err, wantErr) {
		t.Fatalf("sendFrame err=%v, want %v", err, wantErr)
	}

	select {
	case err := <-reported:
		if !errors.Is(err, wantErr) {
			t.Fatalf("reported err=%v, want %v", err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected reported output error")
	}
}

func TestKcpMux_AcceptIgnoresStaleQueueEntries(t *testing.T) {
	mux := NewKcpMux(0, true, KcpMuxConfig{}, nil, nil)
	defer mux.Close()

	done := make(chan error, 1)
	go func() {
		_, err := mux.Accept()
		done <- err
	}()

	mux.acceptCh <- 99
	time.Sleep(50 * time.Millisecond)
	if err := mux.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrServiceMuxClosed) {
			t.Fatalf("Accept stale queue err=%v, want %v", err, ErrServiceMuxClosed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not exit after Close")
	}
}

func TestKcpMux_AllocateLocalStreamIDWraps(t *testing.T) {
	mux := NewKcpMux(0, true, KcpMuxConfig{}, nil, nil)
	defer mux.Close()

	mux.mu.Lock()
	mux.nextLocalStreamID = math.MaxUint32
	streamID, err := mux.allocateLocalStreamIDLocked()
	next := mux.nextLocalStreamID
	mux.mu.Unlock()

	if err != nil {
		t.Fatalf("allocateLocalStreamIDLocked failed: %v", err)
	}
	if streamID != math.MaxUint32 {
		t.Fatalf("streamID=%d, want %d", streamID, uint64(math.MaxUint32))
	}
	if next != 1 {
		t.Fatalf("nextLocalStreamID=%d, want 1 after wrap", next)
	}
}

func TestKcpMux_ReportOutputIgnoresNil(t *testing.T) {
	mux := NewKcpMux(0, true, KcpMuxConfig{}, nil, func(service uint64, err error) {
		t.Fatalf("reportErr should not be called for nil error")
	})
	defer mux.Close()

	mux.reportOutput(nil)
}

func TestKcpMux_OpenRemovesStreamOnOutputError(t *testing.T) {
	wantErr := errors.New("open failed")
	mux := NewKcpMux(0, true, KcpMuxConfig{}, func(service uint64, data []byte) error {
		return wantErr
	}, nil)
	defer mux.Close()

	if _, err := mux.Open(); !errors.Is(err, wantErr) {
		t.Fatalf("Open err=%v, want %v", err, wantErr)
	}
	if got := mux.NumStreams(); got != 0 {
		t.Fatalf("NumStreams=%d, want 0 after failed Open", got)
	}
}

func TestKcpMux_InputInvalidFramesForExistingAndUnknownStreams(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	clientStream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer clientStream.Close()
	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()

	frame := muxControlFrame(kcpMuxFrameOpen, 1, 0x01)
	if err := server.Input(frame); err != nil {
		t.Fatalf("Input(existing invalid open payload) failed: %v", err)
	}

	buf := make([]byte, 1)
	_ = clientStream.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := clientStream.Read(buf); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected invalid-close error after malformed open, got %v", err)
	}

	unknownData := muxDataFrame(99)
	if err := server.Input(unknownData); err != nil {
		t.Fatalf("Input(unknown data stream) failed: %v", err)
	}
}

func TestKcpMux_CloseStreamTwiceAndMissingStream(t *testing.T) {
	client, server := kcpMuxPair(t, KcpMuxConfig{})
	defer client.Close()
	defer server.Close()

	stream, err := client.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer stream.Close()
	serverStream := acceptWithTimeout(t, server, 2*time.Second)
	defer serverStream.Close()

	if err := client.closeStream(1); err != nil {
		t.Fatalf("closeStream(first) failed: %v", err)
	}
	if err := client.closeStream(1); err != nil {
		t.Fatalf("closeStream(second) failed: %v", err)
	}
	if err := client.closeStream(999); err != nil {
		t.Fatalf("closeStream(missing) failed: %v", err)
	}
}
