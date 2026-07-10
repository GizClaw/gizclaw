package serverrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type allowRPCPolicy struct{}

func (allowRPCPolicy) AllowPeer(giznet.PublicKey) bool            { return true }
func (allowRPCPolicy) AllowService(giznet.PublicKey, uint64) bool { return true }

// Server is a self-contained WebRTC endpoint for server-initiated SDK RPC
// interoperability tests.
type Server struct {
	Endpoint  string
	PublicKey giznet.PublicKey

	close    sync.Once
	http     *httptest.Server
	listener *gizwebrtc.Listener
}

func New() (*Server, error) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	listener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModeChaChaPoly,
		SecurityPolicy: allowRPCPolicy{},
	}).Listen(key)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle(gizwebrtc.SignalingPath, listener.SignalingHandler())
	mux.HandleFunc("/server-info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"protocol":       "gizclaw-webrtc",
			"public_key":     key.Public.String(),
			"signaling_path": gizwebrtc.SignalingPath,
		})
	})
	httpServer := httptest.NewServer(mux)
	return &Server{
		Endpoint:  strings.TrimPrefix(httpServer.URL, "http://"),
		PublicKey: key.Public,
		http:      httpServer,
		listener:  listener,
	}, nil
}

func (s *Server) Accept(ctx context.Context) (giznet.Conn, error) {
	if s == nil || s.listener == nil {
		return nil, fmt.Errorf("serverrpc: server is closed")
	}
	connCh := make(chan giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := s.listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()
	select {
	case conn := <-connCh:
		return conn, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	s.close.Do(func() {
		if s.listener != nil {
			_ = s.listener.Close()
		}
		if s.http != nil {
			s.http.Close()
		}
	})
}

func Ping(conn giznet.Conn, id string) (rpcapi.PingResponse, error) {
	stream, err := rpcStream(conn)
	if err != nil {
		return rpcapi.PingResponse{}, err
	}
	defer stream.Close()
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()}); err != nil {
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

func SpeedTest(conn giznet.Conn, id string, up, down int64) (int64, int64, error) {
	stream, err := rpcStream(conn)
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
		return 0, 0, fmt.Errorf("write request: %w", err)
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
			if err := rpcapi.WriteFrame(stream, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: chunk[:n]}); err != nil {
				uploadDone <- err
				return
			}
			written += n
		}
		uploadDone <- rpcapi.WriteEOS(stream)
	}()

	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodAllSpeedTestRun)
	if err != nil {
		return 0, 0, fmt.Errorf("read response: %w", err)
	}
	if response.Id != id || response.Error != nil || response.Result == nil {
		return 0, 0, fmt.Errorf("unexpected speed response: %+v", response)
	}
	ack, err := response.Result.AsSpeedTestResponse()
	if err != nil {
		return 0, 0, fmt.Errorf("decode response: %w", err)
	}
	if ack.UpContentLength != up || ack.DownContentLength != down {
		return 0, 0, fmt.Errorf("speed ack = %+v, want up=%d down=%d", ack, up, down)
	}
	var downloaded int64
	for {
		frame, err := rpcapi.ReadFrame(stream)
		if err != nil {
			return 0, downloaded, fmt.Errorf("read download frame: %w", err)
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
		return 0, downloaded, fmt.Errorf("write upload frames: %w", err)
	}
	return up, downloaded, nil
}

func rpcStream(conn giznet.Conn) (net.Conn, error) {
	if conn == nil {
		return nil, fmt.Errorf("serverrpc: nil connection")
	}
	stream, err := conn.Dial(0)
	if err != nil {
		return nil, err
	}
	if err := stream.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = stream.Close()
		return nil, err
	}
	// @roamhq/wrtc can report the channel open locally just before it dispatches
	// the remote datachannel event. Give the remote SDK one event-loop turn to
	// attach its message listener before the probe writes the first frame.
	time.Sleep(100 * time.Millisecond)
	return stream, nil
}
