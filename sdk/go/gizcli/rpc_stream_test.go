package gizcli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCStreamReadWriteFrames(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverStream, err := newRPCStream(context.Background(), serverSide)
	if err != nil {
		t.Fatalf("newRPCStream(server) error = %v", err)
	}
	defer serverStream.Close()
	clientStream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream(client) error = %v", err)
	}
	defer clientStream.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- clientStream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2, 3}})
	}()

	frame, err := serverStream.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if frame.Type != rpcapi.FrameTypeBinary || !bytes.Equal(frame.Payload, []byte{1, 2, 3}) {
		t.Fatalf("ReadFrame() = %+v", frame)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
}

func TestRPCStreamResponses(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverStream, err := newRPCStream(context.Background(), serverSide)
	if err != nil {
		t.Fatalf("newRPCStream(server) error = %v", err)
	}
	defer serverStream.Close()
	clientStream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream(client) error = %v", err)
	}
	defer clientStream.Close()

	errCh := make(chan error, 1)
	go func() {
		if err := serverStream.WriteResponse(&rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: "one"}); err != nil {
			errCh <- err
			return
		}
		if err := serverStream.WriteResponse(&rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: "two"}); err != nil {
			errCh <- err
			return
		}
		if err := serverStream.WriteEOS(); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	var got []string
	for resp, err := range clientStream.Responses() {
		if err != nil {
			t.Fatalf("Responses() error = %v", err)
		}
		got = append(got, resp.Id)
	}
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("Responses() ids = %v", got)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server write error = %v", err)
	}
}

func TestRPCStreamWriteRequestEnvelopeSplitsOversizedProtobufEnvelope(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverStream, err := newRPCStream(context.Background(), serverSide)
	if err != nil {
		t.Fatalf("newRPCStream(server) error = %v", err)
	}
	defer serverStream.Close()
	clientStream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream(client) error = %v", err)
	}
	defer clientStream.Close()

	largeID := string(bytes.Repeat([]byte("r"), rpcapi.MaxFrameSize+1024))
	req := &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     largeID,
		Method: rpcapi.RPCMethodAllPing,
	}
	errCh := make(chan error, 1)
	go func() {
		if err := clientStream.WriteRequestEnvelope(req); err != nil {
			errCh <- err
			return
		}
		errCh <- clientStream.WriteEOS()
	}()

	got, consumedEOS, err := serverStream.ReadRequestEnvelope()
	if err != nil {
		t.Fatalf("ReadRequestEnvelope() error = %v", err)
	}
	if !consumedEOS {
		t.Fatal("ReadRequestEnvelope() consumedEOS = false, want true")
	}
	if got.Id != largeID || got.Method != rpcapi.RPCMethodAllPing {
		t.Fatalf("ReadRequestEnvelope() = %+v", got)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteRequestEnvelope() error = %v", err)
	}
}

func TestRPCStreamWriteRequestKeepsSingleFrameLimit(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	clientStream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream(client) error = %v", err)
	}
	defer clientStream.Close()

	largeID := string(bytes.Repeat([]byte("r"), rpcapi.MaxFrameSize+1024))
	err = clientStream.WriteRequest(&rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     largeID,
		Method: rpcapi.RPCMethodAllPing,
	})
	if err == nil {
		t.Fatal("WriteRequest() should reject a protobuf frame larger than MaxFrameSize")
	}
}

func TestRPCStreamReadHonorsContextCancel(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := newRPCStream(ctx, clientSide)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	defer stream.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := stream.ReadFrame()
		errCh <- err
	}()

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadFrame() err = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("ReadFrame() did not unblock after context cancel")
	}
}

func TestRPCStreamFramesStopsOnEOS(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	defer stream.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- rpcapi.WriteEOS(serverSide)
	}()
	for _, err := range stream.Frames() {
		if err != nil {
			t.Fatalf("Frames() error = %v", err)
		}
		t.Fatal("Frames() should not yield EOS")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
}
