package gizwebrtc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

type allowAllPolicy struct{}

func (allowAllPolicy) AllowPeer(giznet.PublicKey) bool { return true }
func (allowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

func TestDialSignalingPacketAndServiceStream(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&ListenConfig{
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clientListener, clientConn, err := Dial(ctx, clientKey, serverKey.Public, DialConfig{
		SignalingURL:   httpServer.URL + SignalingPath,
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	})
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientListener.Close()
	defer clientConn.Close()

	serverConn := acceptConn(t, serverListener)
	defer serverConn.Close()

	if _, err := clientConn.Write(0x42, []byte("packet")); err != nil {
		t.Fatalf("client packet Write error = %v", err)
	}
	buf := make([]byte, 64)
	proto, n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("server packet Read error = %v", err)
	}
	if proto != 0x42 || string(buf[:n]) != "packet" {
		t.Fatalf("server packet proto=%d payload=%q", proto, string(buf[:n]))
	}

	opusFrame := []byte{0x00, 0xaa, 0xbb}
	if _, err := clientConn.Write(giznet.ProtocolOpusPacket, opusFrame); err != nil {
		t.Fatalf("client opus Write error = %v", err)
	}
	proto, payload := readDirectPacketWithTimeout(t, serverConn)
	if proto != giznet.ProtocolOpusPacket {
		t.Fatalf("server opus proto=%d, want %d", proto, giznet.ProtocolOpusPacket)
	}
	if string(payload) != string(opusFrame) {
		t.Fatalf("server opus frame = %v, want %v", payload, opusFrame)
	}

	service := serverConn.ListenService(100)
	clientStream, err := clientConn.Dial(100)
	if err != nil {
		t.Fatalf("client Dial(service) error = %v", err)
	}
	defer clientStream.Close()
	serverStreamCh := make(chan interface{}, 1)
	go func() {
		s, err := service.Accept()
		if err != nil {
			serverStreamCh <- err
			return
		}
		serverStreamCh <- s
	}()
	var serverStream interface{}
	select {
	case serverStream = <-serverStreamCh:
	case <-time.After(5 * time.Second):
		t.Fatal("server service Accept timeout")
	}
	if err, ok := serverStream.(error); ok {
		t.Fatalf("server service Accept error = %v", err)
	}
	accepted := serverStream.(interface {
		Read([]byte) (int, error)
		Write([]byte) (int, error)
		Close() error
	})
	defer accepted.Close()
	if _, err := clientStream.Write([]byte("hello stream")); err != nil {
		t.Fatalf("client stream Write error = %v", err)
	}
	n, err = accepted.Read(buf)
	if err != nil {
		t.Fatalf("server stream Read error = %v", err)
	}
	if string(buf[:n]) != "hello stream" {
		t.Fatalf("server stream payload = %q", string(buf[:n]))
	}
}

func TestDialSignalingWithFixedICEPort(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	serverListener, err := (&ListenConfig{
		ICEAddr:        freeLocalICEAddr(t),
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()

	for i := range 2 {
		clientKey, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair(client %d) error = %v", i, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		clientListener, clientConn, err := Dial(ctx, clientKey, serverKey.Public, DialConfig{
			SignalingURL:   httpServer.URL + SignalingPath,
			CipherMode:     CipherModePlaintext,
			SecurityPolicy: allowAllPolicy{},
		})
		cancel()
		if err != nil {
			t.Fatalf("Dial client %d error = %v", i, err)
		}
		defer clientListener.Close()
		defer clientConn.Close()

		serverConn := acceptConn(t, serverListener)
		defer serverConn.Close()

		payload := fmt.Sprintf("fixed ice %d", i)
		if _, err := clientConn.Write(0x42, []byte(payload)); err != nil {
			t.Fatalf("client %d packet Write error = %v", i, err)
		}
		buf := make([]byte, 64)
		proto, n, err := serverConn.Read(buf)
		if err != nil {
			t.Fatalf("server packet Read client %d error = %v", i, err)
		}
		if proto != 0x42 || string(buf[:n]) != payload {
			t.Fatalf("server packet client %d proto=%d payload=%q", i, proto, string(buf[:n]))
		}
	}
}

func TestDialSignalingOverTCPOnlyICE(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	iceTCPListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen ICE TCP error = %v", err)
	}
	serverListener, err := (&ListenConfig{
		ICETCPListener:   iceTCPListener,
		PublicICETCPAddr: iceTCPListener.Addr().String(),
		CipherMode:       CipherModePlaintext,
		SecurityPolicy:   allowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	clientListener, clientConn, err := Dial(ctx, clientKey, serverKey.Public, DialConfig{
		API:            newTCPOnlyClientAPI(),
		SignalingURL:   httpServer.URL + SignalingPath,
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	})
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientListener.Close()
	defer clientConn.Close()

	serverConn := acceptConn(t, serverListener)
	defer serverConn.Close()

	if _, err := clientConn.Write(0x42, []byte("tcp packet")); err != nil {
		t.Fatalf("client packet Write error = %v", err)
	}
	buf := make([]byte, 64)
	proto, n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("server packet Read error = %v", err)
	}
	if proto != 0x42 || string(buf[:n]) != "tcp packet" {
		t.Fatalf("server packet proto=%d payload=%q", proto, string(buf[:n]))
	}
}

func TestPacketWriteRejectsLargePayload(t *testing.T) {
	if _, err := writePacket(noopPacketRaw{}, giznet.ProtocolServiceStream, nil); !errors.Is(err, giznet.ErrPacketProtocol) {
		t.Fatalf("writePacket service-stream protocol err = %v, want %v", err, giznet.ErrPacketProtocol)
	}
	if _, err := writePacket(nil, 0x40, nil); !errors.Is(err, ErrPacketChannel) {
		t.Fatalf("writePacket nil err = %v, want %v", err, ErrPacketChannel)
	}
	payload := make([]byte, maxPacketMessageSize)
	if _, err := writePacket(noopPacketRaw{}, 0x40, payload); !errors.Is(err, giznet.ErrPacketTooLarge) {
		t.Fatalf("writePacket large err = %v, want %v", err, giznet.ErrPacketTooLarge)
	}
	payload = make([]byte, maxPacketMessageSize-1)
	raw := &recordingPacketRaw{}
	n, err := writePacket(raw, 0x40, payload)
	if err != nil {
		t.Fatalf("writePacket max payload error = %v", err)
	}
	if n != len(payload) || len(raw.writes) != maxPacketMessageSize {
		t.Fatalf("writePacket max payload n=%d write_len=%d, want %d", n, len(raw.writes), maxPacketMessageSize)
	}
}

func TestPacketRejectsReservedProtocols(t *testing.T) {
	for _, protocol := range []byte{0x01, 0x0f, 0x11, 0x3f} {
		t.Run(fmt.Sprintf("write_%02x", protocol), func(t *testing.T) {
			if _, err := writePacket(noopPacketRaw{}, protocol, nil); !errors.Is(err, giznet.ErrPacketProtocol) {
				t.Fatalf("writePacket reserved protocol err = %v, want %v", err, giznet.ErrPacketProtocol)
			}
		})
		t.Run(fmt.Sprintf("read_%02x", protocol), func(t *testing.T) {
			raw := &fakeStreamRaw{reads: [][]byte{{protocol, 'x'}}}
			if _, err := readPacket(raw); !errors.Is(err, giznet.ErrPacketProtocol) {
				t.Fatalf("readPacket reserved protocol err = %v, want %v", err, giznet.ErrPacketProtocol)
			}
		})
	}
}

func TestPacketReadRejectsServiceStreamProtocol(t *testing.T) {
	raw := &fakeStreamRaw{reads: [][]byte{{giznet.ProtocolServiceStream, 'x'}}}
	if _, err := readPacket(raw); !errors.Is(err, giznet.ErrPacketProtocol) {
		t.Fatalf("readPacket service-stream protocol err = %v, want %v", err, giznet.ErrPacketProtocol)
	}
}

func newTCPOnlyClientAPI() *webrtc.API {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	settingEngine.SetIncludeLoopbackCandidate(true)
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	return webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
}

func readDirectPacketWithTimeout(t *testing.T, conn *Conn) (byte, []byte) {
	t.Helper()
	type result struct {
		protocol byte
		payload  []byte
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, maxPacketMessageSize)
		protocol, n, err := conn.Read(buf)
		ch <- result{protocol: protocol, payload: append([]byte(nil), buf[:n]...), err: err}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("Read error = %v", res.err)
		}
		return res.protocol, res.payload
	case <-time.After(5 * time.Second):
		t.Fatal("Read timeout")
	}
	return 0, nil
}

func acceptConn(t *testing.T, l *Listener) *Conn {
	t.Helper()
	ch := make(chan giznet.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			errCh <- err
			return
		}
		ch <- c
	}()
	select {
	case c := <-ch:
		wc, ok := c.(*Conn)
		if !ok {
			t.Fatalf("accepted conn type = %T", c)
		}
		return wc
	case err := <-errCh:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timeout")
	}
	return nil
}

func freeLocalICEAddr(t *testing.T) string {
	t.Helper()
	for range 20 {
		tcp, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen tcp: %v", err)
		}
		port := tcp.Addr().(*net.TCPAddr).Port
		udp, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = udp.Close()
			_ = tcp.Close()
			return fmt.Sprintf("127.0.0.1:%d", port)
		}
		_ = tcp.Close()
	}
	t.Fatal("could not allocate local ICE addr")
	return ""
}

type noopPacketRaw struct{}

func (noopPacketRaw) Read([]byte) (int, error)                  { return 0, nil }
func (noopPacketRaw) Write([]byte) (int, error)                 { return 0, nil }
func (noopPacketRaw) ReadDataChannel([]byte) (int, bool, error) { return 0, false, nil }
func (noopPacketRaw) WriteDataChannel([]byte, bool) (int, error) {
	return 0, nil
}
func (noopPacketRaw) Close() error                     { return nil }
func (noopPacketRaw) SetReadDeadline(time.Time) error  { return nil }
func (noopPacketRaw) SetWriteDeadline(time.Time) error { return nil }

type recordingPacketRaw struct {
	noopPacketRaw
	writes []byte
}

func (r *recordingPacketRaw) WriteDataChannel(data []byte, _ bool) (int, error) {
	r.writes = append(r.writes[:0], data...)
	return len(data), nil
}
