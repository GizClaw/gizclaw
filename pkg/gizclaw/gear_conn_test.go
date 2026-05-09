package gizclaw

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestGearConnHelpersAndRPCHandle(t *testing.T) {
	t.Run("audio mixer lifecycle", func(t *testing.T) {
		var nilPeer *GearConn
		if _, err := nilPeer.audioMixer(); err != ErrNilGearConn {
			t.Fatalf("audioMixer(nil) err = %v, want %v", err, ErrNilGearConn)
		}

		peer := &GearConn{}
		if _, err := peer.audioMixer(); err != ErrNilGearConnMixer {
			t.Fatalf("audioMixer() err = %v, want %v", err, ErrNilGearConnMixer)
		}

		peer.init()
		if _, err := peer.audioMixer(); err != nil {
			t.Fatalf("audioMixer() after init error = %v", err)
		}

		track, ctrl, err := peer.CreateAudioTrack()
		if err != nil {
			t.Fatalf("CreateAudioTrack() error = %v", err)
		}
		if track == nil || ctrl == nil {
			t.Fatalf("CreateAudioTrack() = (%v, %v)", track, ctrl)
		}
		if err := peer.close(); err != nil {
			t.Fatalf("close() error = %v", err)
		}
		if !peer.isClosed() {
			t.Fatal("peer should be closed")
		}
	})

	t.Run("dispatch missing params", func(t *testing.T) {
		server := &rpcServer{}
		resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "missing",
			Method: rpcapi.RPCMethodPeerPing,
		})
		if err != nil {
			t.Fatalf("dispatch() error = %v", err)
		}
		if resp == nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
			t.Fatalf("dispatch() response = %+v", resp)
		}
	})

	t.Run("dispatch ping and unknown method", func(t *testing.T) {
		server := &rpcServer{}
		params, err := newRPCPingRequestParams(rpcapi.PingRequest{})
		if err != nil {
			t.Fatalf("newRPCPingRequestParams() error = %v", err)
		}
		resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "ping",
			Method: rpcapi.RPCMethodPeerPing,
			Params: params,
		})
		if err != nil {
			t.Fatalf("dispatch(ping) error = %v", err)
		}
		if resp == nil || resp.Result == nil {
			t.Fatalf("dispatch(ping) response = %+v", resp)
		}
		result, err := resp.Result.AsPingResponse()
		if err != nil {
			t.Fatalf("dispatch(ping) result decode error = %v", err)
		}
		if result.ServerTime <= 0 {
			t.Fatalf("dispatch(ping) response = %+v", result)
		}

		resp, err = server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "unknown",
			Method: "rpc.unknown",
		})
		if err != nil {
			t.Fatalf("dispatch(unknown) error = %v", err)
		}
		if resp == nil || resp.Error == nil || !strings.Contains(resp.Error.Message, "unknown method") {
			t.Fatalf("dispatch(unknown) response = %+v", resp)
		}
	})
}

func TestGearConnCloseClosesConn(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&giznet.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen(server) error = %v", err)
	}
	defer serverListener.Close()
	go drainUDP(serverListener.UDP())
	clientListener, err := (&giznet.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{},
	}).Listen(clientKey)
	if err != nil {
		t.Fatalf("Listen(client) error = %v", err)
	}
	defer clientListener.Close()
	go drainUDP(clientListener.UDP())

	acceptCh := make(chan *giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientConn.Close()

	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-errCh:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timeout")
	}

	peer := &GearConn{Conn: serverConn}
	if err := peer.close(); err != nil {
		t.Fatalf("GearConn.close() error = %v", err)
	}
	if err := serverConn.Close(); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("server Conn.Close() after GearConn.close err=%v, want %v", err, giznet.ErrConnClosed)
	}
}

func TestGearConnPCMChunkToInt16(t *testing.T) {
	chunk := &pcm.DataChunk{Data: []byte{0x34, 0x12, 0x78, 0x56}}
	got := gearConnPCMChunkToInt16(chunk)
	if len(got) != 2 {
		t.Fatalf("len(gearConnPCMChunkToInt16()) = %d", len(got))
	}
	if got[0] != 0x1234 || got[1] != 0x5678 {
		t.Fatalf("gearConnPCMChunkToInt16() = %#v", got)
	}
	if out := gearConnPCMChunkToInt16(nil); out != nil {
		t.Fatalf("gearConnPCMChunkToInt16(nil) = %#v", out)
	}
}
