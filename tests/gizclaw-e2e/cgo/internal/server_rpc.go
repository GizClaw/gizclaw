//go:build gizclaw_e2e

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type allowServerRPCPolicy struct{}

func (allowServerRPCPolicy) AllowPeer(giznet.PublicKey) bool { return true }
func (allowServerRPCPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

type ServerRPCFixture struct {
	Client *Client
	Conn   giznet.Conn

	cancel   context.CancelFunc
	pollDone chan struct{}
	close    sync.Once
	listener *gizwebrtc.Listener
	http     *httptest.Server
}

func NewServerRPCFixture(t *testing.T) *ServerRPCFixture {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModeChaChaPoly,
		SecurityPolicy: allowServerRPCPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(gizwebrtc.SignalingPath, listener.SignalingHandler())
	mux.HandleFunc("/server-info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"protocol":       "gizclaw-webrtc",
			"public_key":     serverKey.Public.String(),
			"signaling_path": gizwebrtc.SignalingPath,
		})
	})
	httpServer := httptest.NewServer(mux)
	client, err := NewClientWithCredentials(
		strings.TrimPrefix(httpServer.URL, "http://"),
		clientKey.Private.String(),
	)
	if err != nil {
		httpServer.Close()
		_ = listener.Close()
		t.Fatal(err)
	}

	acceptCh := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		acceptCh <- conn
	}()
	var serverConn giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-acceptErr:
		client.Close()
		httpServer.Close()
		_ = listener.Close()
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		client.Close()
		httpServer.Close()
		_ = listener.Close()
		t.Fatal("accept C WebRTC client timeout")
	}

	ctx, cancel := context.WithCancel(context.Background())
	fixture := &ServerRPCFixture{
		Client: client, Conn: serverConn, cancel: cancel,
		pollDone: make(chan struct{}), listener: listener, http: httpServer,
	}
	go func() {
		defer close(fixture.pollDone)
		for ctx.Err() == nil {
			if err := client.Poll(10 * time.Millisecond); err != nil && ctx.Err() == nil {
				return
			}
		}
	}()
	t.Cleanup(fixture.Close)
	return fixture
}

func (f *ServerRPCFixture) Close() {
	if f == nil {
		return
	}
	f.close.Do(func() {
		f.cancel()
		<-f.pollDone
		f.Client.Close()
		_ = f.Conn.Close()
		f.http.Close()
		_ = f.listener.Close()
	})
}

func (f *ServerRPCFixture) Ping(id string) (rpcapi.PingResponse, error) {
	stream, err := f.rpcStream()
	if err != nil {
		return rpcapi.PingResponse{}, err
	}
	defer stream.Close()
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: 12345}); err != nil {
		return rpcapi.PingResponse{}, err
	}
	request := &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: id,
		Method: rpcapi.RPCMethodAllPing, Params: &params,
	}
	if err := rpcapi.WriteRequest(stream, request); err != nil {
		return rpcapi.PingResponse{}, err
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		return rpcapi.PingResponse{}, err
	}
	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodAllPing)
	if err != nil {
		return rpcapi.PingResponse{}, err
	}
	if response.Id != id || response.Error != nil || response.Result == nil {
		return rpcapi.PingResponse{}, fmt.Errorf("unexpected ping response: %+v", response)
	}
	result, err := response.Result.AsPingResponse()
	if err != nil {
		return rpcapi.PingResponse{}, err
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		return rpcapi.PingResponse{}, err
	}
	return result, nil
}

func (f *ServerRPCFixture) SpeedTest(id string, up, down int64) (int64, int64, error) {
	stream, err := f.rpcStream()
	if err != nil {
		return 0, 0, err
	}
	defer stream.Close()
	var params rpcapi.RPCPayload
	if err := params.FromSpeedTestRequest(rpcapi.SpeedTestRequest{
		UpContentLength: up, DownContentLength: down,
	}); err != nil {
		return 0, 0, err
	}
	request := &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: id,
		Method: rpcapi.RPCMethodAllSpeedTestRun, Params: &params,
	}
	if err := rpcapi.WriteRequest(stream, request); err != nil {
		return 0, 0, err
	}

	uploadDone := make(chan error, 1)
	go func() {
		chunk := make([]byte, 32*1024)
		var written int64
		for written < up {
			n := int64(len(chunk))
			if remaining := up - written; remaining < n {
				n = remaining
			}
			if err := rpcapi.WriteFrame(stream, rpcapi.Frame{
				Type: rpcapi.FrameTypeBinary, Payload: chunk[:n],
			}); err != nil {
				uploadDone <- err
				return
			}
			written += n
		}
		uploadDone <- rpcapi.WriteEOS(stream)
	}()

	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodAllSpeedTestRun)
	if err != nil {
		return 0, 0, err
	}
	if response.Id != id || response.Error != nil || response.Result == nil {
		return 0, 0, fmt.Errorf("unexpected speed response: %+v", response)
	}
	ack, err := response.Result.AsSpeedTestResponse()
	if err != nil {
		return 0, 0, err
	}
	if ack.UpContentLength != up || ack.DownContentLength != down {
		return 0, 0, fmt.Errorf("speed ack = %+v, want up=%d down=%d", ack, up, down)
	}
	var downloaded int64
	for {
		frame, err := rpcapi.ReadFrame(stream)
		if err != nil {
			return 0, downloaded, err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			break
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			return 0, downloaded, fmt.Errorf("unexpected download frame type %d", frame.Type)
		}
		downloaded += int64(len(frame.Payload))
	}
	if err := <-uploadDone; err != nil {
		return 0, downloaded, err
	}
	return up, downloaded, nil
}

func (f *ServerRPCFixture) rpcStream() (net.Conn, error) {
	stream, err := f.Conn.Dial(0)
	if err != nil {
		return nil, err
	}
	if err := stream.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		_ = stream.Close()
		return nil, err
	}
	return stream, nil
}
