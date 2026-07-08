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

func TestDownloadFirmwareRejectsNilOutput(t *testing.T) {
	client := &rpcClient{}
	_, err := client.DownloadFirmware(context.Background(), nil, "firmware-download", rpcapi.FirmwareFilesDownloadRequest{}, nil)
	if err == nil || !strings.Contains(err.Error(), "firmware download output is required") {
		t.Fatalf("DownloadFirmware nil output err = %v", err)
	}
}

func TestClientFirmwareMethodsRequireConnection(t *testing.T) {
	client := &Client{}
	if _, err := client.ListFirmwares(context.Background(), "firmware-list", rpcapi.FirmwareListRequest{}); err == nil || !strings.Contains(err.Error(), "client is not connected") {
		t.Fatalf("ListFirmwares disconnected err = %v", err)
	}
	if _, err := client.GetFirmware(context.Background(), "firmware-get", rpcapi.FirmwareGetRequest{FirmwareId: "devkit"}); err == nil || !strings.Contains(err.Error(), "client is not connected") {
		t.Fatalf("GetFirmware disconnected err = %v", err)
	}
	var out bytes.Buffer
	if _, err := client.DownloadFirmware(context.Background(), "firmware-download", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware.bin",
	}, &out); err == nil || !strings.Contains(err.Error(), "client is not connected") {
		t.Fatalf("DownloadFirmware disconnected err = %v", err)
	}
}

func TestClientFirmwareMethodsUseRPCConnection(t *testing.T) {
	client, serverConn, cleanup := connectedFirmwareTestClient(t)
	defer cleanup()

	listener := serverConn.ListenService(ServicePeerRPC)
	defer listener.Close()

	serverErrCh := make(chan error, 3)
	go func() {
		serveFirmwareRPCResponse(t, listener, rpcapi.RPCMethodServerFirmwareList, rpcapi.FirmwareListResponse{
			Items:   []rpcapi.Firmware{{Name: "devkit"}},
			HasNext: false,
		}, (*rpcapi.RPCResponse_Result).FromFirmwareListResponse, nil, serverErrCh)
		serveFirmwareRPCResponse(t, listener, rpcapi.RPCMethodServerFirmwareGet, rpcapi.FirmwareGetResponse{Name: "devkit"}, (*rpcapi.RPCResponse_Result).FromFirmwareGetResponse, nil, serverErrCh)
		serveFirmwareRPCResponse(t, listener, rpcapi.RPCMethodServerFirmwareFilesDownload, rpcapi.FirmwareFilesDownloadResponse{
			FirmwareId: "devkit",
			Channel:    rpcapi.FirmwareChannelNameStable,
			Path:       "firmware.bin",
			Artifact:   rpcapi.FirmwareArtifact{TarPath: "devkit/stable/artifact/artifact.tar", Size: 1024, ContentType: "application/x-tar"},
			File:       rpcapi.FirmwareArtifactEntry{Path: "firmware.bin", Type: rpcapi.FirmwareArtifactEntryTypeFile, Size: int64(len("firmware-payload"))},
		}, (*rpcapi.RPCResponse_Result).FromFirmwareFilesDownloadResponse, []byte("firmware-payload"), serverErrCh)
	}()
	list, err := client.ListFirmwares(context.Background(), "firmware-list", rpcapi.FirmwareListRequest{})
	if err != nil {
		t.Fatalf("ListFirmwares error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "devkit" {
		t.Fatalf("ListFirmwares = %#v", list)
	}

	got, err := client.GetFirmware(context.Background(), "firmware-get", rpcapi.FirmwareGetRequest{FirmwareId: "devkit"})
	if err != nil {
		t.Fatalf("GetFirmware error = %v", err)
	}
	if got.Name != "devkit" {
		t.Fatalf("GetFirmware = %#v", got)
	}

	payload := []byte("firmware-payload")
	var out bytes.Buffer
	download, err := client.DownloadFirmware(context.Background(), "firmware-download", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware.bin",
	}, &out)
	if err != nil {
		t.Fatalf("DownloadFirmware error = %v", err)
	}
	if download.Bytes != int64(len(payload)) || out.String() != string(payload) {
		t.Fatalf("DownloadFirmware = %#v payload=%q", download, out.String())
	}

	for i := 0; i < 3; i++ {
		if err := <-serverErrCh; err != nil {
			t.Fatalf("server error = %v", err)
		}
	}
}

func TestDownloadFirmwareReturnsRPCError(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		resp := rpcapi.Error{
			RequestID: req.Id,
			Code:      rpcapi.RPCErrorCodeNotFound,
			Message:   "firmware artifact not found",
		}.RPCResponse()
		serverErrCh <- writeRPCResponseWithEOS(serverSide, req.Method, resp)
	}()

	client := &rpcClient{}
	var out bytes.Buffer
	_, err := client.DownloadFirmware(context.Background(), clientSide, "firmware-download", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: "devkit",
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware.bin",
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "firmware artifact not found") {
		t.Fatalf("DownloadFirmware RPC error = %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func connectedFirmwareTestClient(t *testing.T) (*Client, giznet.Conn, func()) {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
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
		t.Fatalf("Dial() error = %v", err)
	}

	var serverConn giznet.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		_ = client.Close()
		t.Fatalf("Accept() error = %v", err)
	case <-time.After(3 * time.Second):
		_ = client.Close()
		t.Fatal("Accept() timeout")
	}

	cleanup := func() {
		_ = client.Close()
		_ = serverConn.Close()
	}
	return client, serverConn, cleanup
}

func serveFirmwareRPCResponse[T any](
	t *testing.T,
	listener giznet.ServiceListener,
	wantMethod rpcapi.RPCMethod,
	response T,
	encode func(*rpcapi.RPCResponse_Result, T) error,
	payload []byte,
	errCh chan<- error,
) {
	t.Helper()
	stream, err := listener.Accept()
	if err != nil {
		errCh <- err
		return
	}
	req, err := readRPCRequestWithEOS(stream)
	if err != nil {
		errCh <- err
		return
	}
	if req.Method != wantMethod {
		errCh <- &unexpectedRPCMethodError{got: req.Method, want: wantMethod}
		return
	}
	resp := resourceResponse(req.Id, response, encode)
	if err := rpcapi.WriteResponseForMethod(stream, wantMethod, resp); err != nil {
		errCh <- err
		return
	}
	if payload != nil {
		if err := rpcapi.WriteFrame(stream, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload}); err != nil {
			errCh <- err
			return
		}
	}
	errCh <- rpcapi.WriteEOS(stream)
}

func TestCopyBinaryFramesRejectsUnexpectedFrame(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeJSON, Payload: []byte(`{}`)})
	}()

	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var out bytes.Buffer
	_, err = copyBinaryFrames(&out, stream)
	if err == nil || !strings.Contains(err.Error(), "expected binary frame") {
		t.Fatalf("copyBinaryFrames unexpected frame err = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("writer error = %v", err)
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
		t.Fatalf("copyBinaryFrames short write err = %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("writer error = %v", err)
	}
}
