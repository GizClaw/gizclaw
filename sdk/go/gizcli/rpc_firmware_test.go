package gizcli

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestClientGetFirmwareRequiresConnection(t *testing.T) {
	client := &Client{}
	_, err := client.GetFirmware(context.Background(), "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameStable})
	if err == nil || !strings.Contains(err.Error(), "client is not connected") {
		t.Fatalf("GetFirmware disconnected err = %v", err)
	}
}

func TestClientGetFirmwareUsesRPCConnection(t *testing.T) {
	client, serverConn, cleanup := connectedFirmwareTestClient(t)
	defer cleanup()
	listener := serverConn.ListenService(ServicePeerRPC)
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		stream, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		request, err := readRPCRequestWithEOS(stream)
		if err != nil {
			serverErr <- err
			return
		}
		params, err := request.Params.AsFirmwareGetRequest()
		if err != nil {
			serverErr <- err
			return
		}
		if request.Method != rpcapi.RPCMethodServerFirmwareGet || params.Channel != rpcapi.FirmwareChannelNameBeta {
			serverErr <- &unexpectedRPCMethodError{got: request.Method, want: rpcapi.RPCMethodServerFirmwareGet}
			return
		}
		response := resourceResponse(request.Id, rpcapi.FirmwareGetResponse{
			Channel:     rpcapi.FirmwareChannelNameBeta,
			Description: new("beta package"),
			Url:         "https://firmware.example/beta.tar.zlib",
			Sha256:      strings.Repeat("a", 64),
			Size:        123,
		}, (*rpcapi.RPCPayload).FromFirmwareGetResponse)
		serverErr <- writeRPCResponseWithEOS(stream, request.Method, response)
	}()

	got, err := client.GetFirmware(context.Background(), "firmware-get", rpcapi.FirmwareGetRequest{Channel: rpcapi.FirmwareChannelNameBeta})
	if err != nil {
		t.Fatalf("GetFirmware: %v", err)
	}
	if got.Channel != rpcapi.FirmwareChannelNameBeta || got.Description == nil || *got.Description != "beta package" || got.Url != "https://firmware.example/beta.tar.zlib" || got.Sha256 != strings.Repeat("a", 64) || got.Size != 123 {
		t.Fatalf("GetFirmware = %#v", got)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func connectedFirmwareTestClient(t *testing.T) (*Client, giznet.Conn, func()) {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server): %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client): %v", err)
	}
	serverListener, signalingURL := newTestWebRTCServer(t, serverKey, clientSecurityPolicy{})
	accepted := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client := &Client{KeyPair: clientKey, DialTransport: testWebRTCDialTransport()}
	if err := client.Dial(serverKey.Public, signalingURL); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	var serverConn giznet.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		_ = client.Close()
		t.Fatalf("Accept: %v", err)
	case <-time.After(3 * time.Second):
		_ = client.Close()
		t.Fatal("Accept timeout")
	}
	return client, serverConn, func() {
		_ = client.Close()
		_ = serverConn.Close()
	}
}

func TestCopyBinaryFramesRejectsUnexpectedFrame(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	errCh := make(chan error, 1)
	go func() {
		errCh <- rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: []byte(`{}`)})
	}()
	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var out bytes.Buffer
	_, err = copyBinaryFrames(&out, stream)
	if err == nil || !strings.Contains(err.Error(), "expected binary frame") {
		t.Fatalf("copyBinaryFrames err = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("writer: %v", err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestCopyBinaryFramesDetectsShortWrite(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	errCh := make(chan error, 1)
	go func() {
		errCh <- rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte("payload")})
	}()
	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = copyBinaryFrames(shortWriter{}, stream)
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("copyBinaryFrames err = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("writer: %v", err)
	}
}
