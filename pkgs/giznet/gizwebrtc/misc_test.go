package gizwebrtc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

func TestAddrNetworkAndString(t *testing.T) {
	a := addr("gizwebrtc:test")
	if a.Network() != "webrtc" {
		t.Fatalf("Network = %q, want webrtc", a.Network())
	}
	if a.String() != "gizwebrtc:test" {
		t.Fatalf("String = %q, want gizwebrtc:test", a.String())
	}
}

func TestListenRejectsNilKeyAndTopLevelListenWorks(t *testing.T) {
	if _, err := Listen(nil); err == nil {
		t.Fatal("Listen(nil) error = nil")
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	l, err := Listen(key)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

func TestListenerAcceptCloseAndPeerEvent(t *testing.T) {
	if _, err := (*Listener)(nil).Accept(); !errors.Is(err, giznet.ErrNilListener) {
		t.Fatalf("nil Accept error = %v, want %v", err, giznet.ErrNilListener)
	}
	handler := &recordPeerEventHandler{}
	l := &Listener{
		cfg:      ListenConfig{PeerEventHandler: handler},
		acceptCh: make(chan giznet.Conn, 1),
		closeCh:  make(chan struct{}),
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	conn := &Conn{pk: key.Public}
	l.enqueueConn(conn)
	accepted, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept error = %v", err)
	}
	if accepted != conn {
		t.Fatalf("accepted conn = %p, want %p", accepted, conn)
	}
	if !handler.called || handler.event.PublicKey != key.Public || handler.event.State != giznet.PeerStateEstablished {
		t.Fatalf("peer event = %#v called=%t", handler.event, handler.called)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if _, err := l.Accept(); !errors.Is(err, giznet.ErrClosed) {
		t.Fatalf("Accept after Close error = %v, want %v", err, giznet.ErrClosed)
	}
}

func TestConnPacketReadWriteAndPeerInfoEdges(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	conn := &Conn{
		pk:      key.Public,
		pc:      &webrtc.PeerConnection{},
		readCh:  make(chan directPacket, 1),
		closeCh: make(chan struct{}),
	}
	if got := conn.PublicKey(); got != key.Public {
		t.Fatalf("PublicKey = %s, want %s", got, key.Public)
	}
	info := conn.PeerInfo()
	if info == nil || info.PublicKey != key.Public || info.State != giznet.PeerStateEstablished {
		t.Fatalf("PeerInfo = %#v", info)
	}
	if _, err := conn.Write(0x44, []byte("x")); !errors.Is(err, ErrPacketChannel) {
		t.Fatalf("Write without packet channel error = %v, want %v", err, ErrPacketChannel)
	}
	conn.readCh <- directPacket{protocol: 0x44, payload: []byte("toolarge")}
	if _, _, err := conn.Read(make([]byte, 2)); !errors.Is(err, giznet.ErrPacketBuffer) {
		t.Fatalf("Read small buffer error = %v, want %v", err, giznet.ErrPacketBuffer)
	}
	close(conn.closeCh)
	if _, _, err := conn.Read(make([]byte, 16)); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("Read closed error = %v, want %v", err, giznet.ErrConnClosed)
	}
}

func TestConnListenServiceReuseAndWriteRoutes(t *testing.T) {
	if (*Conn)(nil).ListenService(1) != nil {
		t.Fatal("nil ListenService returned non-nil listener")
	}
	writer := &fakeSampleWriter{}
	raw := &fakeStreamRaw{}
	conn := &Conn{
		pc:         &webrtc.PeerConnection{},
		packetRaw:  raw,
		audioTrack: writer,
		services:   make(map[uint64]*ServiceListener),
		closeCh:    make(chan struct{}),
	}
	first := conn.ListenService(7)
	second := conn.ListenService(7)
	if first != second {
		t.Fatalf("ListenService did not reuse listener: %p != %p", first, second)
	}
	if n, err := conn.Write(0x42, []byte("packet")); err != nil || n != len("packet") {
		t.Fatalf("packet Write n=%d err=%v", n, err)
	}
	if n, err := conn.Write(giznet.ProtocolOpusPacket, opusPacketForTest()); err != nil || n != len(opusPacketForTest()) {
		t.Fatalf("opus Write n=%d err=%v", n, err)
	}
	if len(writer.samples) != 1 {
		t.Fatalf("opus samples = %d, want 1", len(writer.samples))
	}
}

func TestConnNilAndClosedEdges(t *testing.T) {
	if (*Conn)(nil).PublicKey() != (giznet.PublicKey{}) {
		t.Fatal("nil PublicKey returned non-zero key")
	}
	if (*Conn)(nil).PeerInfo() != nil {
		t.Fatal("nil PeerInfo returned non-nil info")
	}
	if err := (*Conn)(nil).validate(); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("nil validate error = %v, want %v", err, giznet.ErrNilConn)
	}
	conn := &Conn{pc: &webrtc.PeerConnection{}}
	conn.closed.Store(true)
	if err := conn.validate(); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("closed validate error = %v, want %v", err, giznet.ErrConnClosed)
	}
	info := conn.PeerInfo()
	if info == nil || info.State != giznet.PeerStateOffline {
		t.Fatalf("closed PeerInfo = %#v", info)
	}
}

func TestServiceListenerEdges(t *testing.T) {
	if _, err := (*ServiceListener)(nil).Accept(); !errors.Is(err, giznet.ErrNilConn) {
		t.Fatalf("nil Accept error = %v, want %v", err, giznet.ErrNilConn)
	}
	conn := &Conn{localAddr: addr("local"), closeCh: make(chan struct{})}
	l := newServiceListener(conn, 7)
	if l.Addr().String() != "local" {
		t.Fatalf("Addr = %v, want local", l.Addr())
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	raw := &fakeStreamRaw{}
	stream := newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	if err := l.enqueue(stream); !errors.Is(err, giznet.ErrServiceMuxClosed) {
		t.Fatalf("enqueue after close error = %v, want %v", err, giznet.ErrServiceMuxClosed)
	}
	if !raw.closed {
		t.Fatal("enqueue after close did not close stream")
	}
	if (*ServiceListener)(nil).Addr() != nil {
		t.Fatal("nil Addr returned non-nil addr")
	}

	connClosed := &Conn{closeCh: make(chan struct{})}
	close(connClosed.closeCh)
	l = newServiceListener(connClosed, 8)
	if _, err := l.Accept(); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("Accept after conn close error = %v, want %v", err, giznet.ErrConnClosed)
	}
	raw = &fakeStreamRaw{}
	stream = newDataChannelConn(raw, nil, addr("local"), addr("remote"))
	if err := l.enqueue(stream); !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("enqueue after conn close error = %v, want %v", err, giznet.ErrConnClosed)
	}
	if !raw.closed {
		t.Fatal("enqueue after conn close did not close stream")
	}
}

func TestLabelsAndLoopbackHelpers(t *testing.T) {
	label := serviceLabel(123)
	service, ok := parseServiceLabel(label)
	if !ok || service != 123 {
		t.Fatalf("parseServiceLabel(%q) = %d/%t, want 123/true", label, service, ok)
	}
	for _, bad := range []string{"", serviceLabelPrefix, serviceLabelPrefix + "abc", packetLabel} {
		if _, ok := parseServiceLabel(bad); ok {
			t.Fatalf("parseServiceLabel(%q) ok = true, want false", bad)
		}
	}
	for _, loopback := range []string{"127.0.0.1:9821", "localhost:9821", "::1"} {
		if !isLoopbackICEAddr(loopback) {
			t.Fatalf("isLoopbackICEAddr(%q) = false, want true", loopback)
		}
	}
	if isLoopbackICEAddr("192.0.2.10:9821") {
		t.Fatal("isLoopbackICEAddr(non-loopback) = true, want false")
	}
}

func TestICEMuxAddrs(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ListenConfig
		wantUDP string
		wantTCP string
	}{
		{
			name: "nil config",
		},
		{
			name:    "shared address",
			cfg:     &ListenConfig{ICEAddr: "127.0.0.1:9820"},
			wantUDP: "127.0.0.1:9820",
			wantTCP: "127.0.0.1:9820",
		},
		{
			name:    "udp override",
			cfg:     &ListenConfig{ICEAddr: "127.0.0.1:9820", ICEUDPAddr: "127.0.0.1:9821"},
			wantUDP: "127.0.0.1:9821",
			wantTCP: "127.0.0.1:9820",
		},
		{
			name:    "tcp override",
			cfg:     &ListenConfig{ICEAddr: "127.0.0.1:9820", ICETCPAddr: "127.0.0.1:9822"},
			wantUDP: "127.0.0.1:9820",
			wantTCP: "127.0.0.1:9822",
		},
		{
			name:    "split addresses",
			cfg:     &ListenConfig{ICEUDPAddr: "127.0.0.1:9821", ICETCPAddr: "127.0.0.1:9822"},
			wantUDP: "127.0.0.1:9821",
			wantTCP: "127.0.0.1:9822",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUDP, gotTCP := iceMuxAddrs(tt.cfg)
			if gotUDP != tt.wantUDP || gotTCP != tt.wantTCP {
				t.Fatalf("iceMuxAddrs() = %q, %q; want %q, %q", gotUDP, gotTCP, tt.wantUDP, tt.wantTCP)
			}
		})
	}
}

func TestNAT1To1IPsFromConfig(t *testing.T) {
	if got := nat1To1IPs(&ListenConfig{NAT1To1IPs: []string{"198.51.100.10"}}); len(got) != 1 || got[0] != "198.51.100.10" {
		t.Fatalf("nat1To1IPs(config) = %#v", got)
	}
}

func TestICELiteFromConfig(t *testing.T) {
	if !iceLite(&ListenConfig{ICELite: true}) {
		t.Fatal("iceLite(config) = false, want true")
	}
}

func TestNewPionAPICleansUDPWhenTCPBindFails(t *testing.T) {
	udpProbe, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket probe error = %v", err)
	}
	udpAddr := udpProbe.LocalAddr().String()
	if err := udpProbe.Close(); err != nil {
		t.Fatalf("Close udp probe error = %v", err)
	}

	tcpBlocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen tcp blocker error = %v", err)
	}
	defer tcpBlocker.Close()

	_, closers, err := newPionAPI(&ListenConfig{
		ICEUDPAddr: udpAddr,
		ICETCPAddr: tcpBlocker.Addr().String(),
	})
	for _, closeFn := range closers {
		_ = closeFn()
	}
	if err == nil {
		t.Fatal("newPionAPI with occupied TCP addr error = nil")
	}

	udpAfter, err := net.ListenPacket("udp", udpAddr)
	if err != nil {
		t.Fatalf("UDP addr was not released after TCP bind failure: %v", err)
	}
	if err := udpAfter.Close(); err != nil {
		t.Fatalf("Close udp after error = %v", err)
	}
}

func TestNewPionAPIRejectsICETCPListenerAndAddrConflict(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  func(net.Listener) *ListenConfig
	}{
		{
			name: "ICETCPAddr",
			cfg: func(listener net.Listener) *ListenConfig {
				return &ListenConfig{ICETCPAddr: "127.0.0.1:0", ICETCPListener: listener}
			},
		},
		{
			name: "ICEAddr",
			cfg: func(listener net.Listener) *ListenConfig {
				return &ListenConfig{ICEAddr: "127.0.0.1:0", ICETCPListener: listener}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Listen tcp error = %v", err)
			}
			defer tcpListener.Close()

			_, closers, err := newPionAPI(tt.cfg(tcpListener))
			for _, closeFn := range closers {
				_ = closeFn()
			}
			if err == nil {
				t.Fatalf("newPionAPI with %s and ICETCPListener error = nil", tt.name)
			}
		})
	}
}

func TestNewPionAPITakesOwnershipOfICETCPListener(t *testing.T) {
	tcpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen tcp error = %v", err)
	}

	_, closers, err := newPionAPI(&ListenConfig{ICETCPListener: tcpListener})
	if err != nil {
		_ = tcpListener.Close()
		t.Fatalf("newPionAPI error = %v", err)
	}
	for _, closeFn := range closers {
		if err := closeFn(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("closeFn error = %v", err)
		}
	}

	conn, err := net.DialTimeout("tcp", tcpListener.Addr().String(), 50*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial to ICETCPListener succeeded after close")
	}
}

func TestSignalingCryptoModesAndAAD(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	nonce := fixedNonce(9)
	ts := time.Now().Unix()
	for _, mode := range []CipherMode{CipherModeChaChaPoly, CipherModeAES256GCM, CipherModePlaintext, ""} {
		clientReqAEAD, clientReqNonce, clientRespAEAD, clientRespNonce, err := deriveSignaling(clientKey, serverKey.Public, nonce, ts, mode)
		if err != nil {
			t.Fatalf("deriveSignaling client mode %q error = %v", mode, err)
		}
		serverReqAEAD, serverReqNonce, serverRespAEAD, serverRespNonce, err := deriveSignaling(serverKey, clientKey.Public, nonce, ts, mode)
		if err != nil {
			t.Fatalf("deriveSignaling server mode %q error = %v", mode, err)
		}
		if !bytes.Equal(clientReqNonce, serverReqNonce) || !bytes.Equal(clientRespNonce, serverRespNonce) {
			t.Fatalf("nonces differ for mode %q", mode)
		}
		reqAAD := requestAAD(clientKey.Public, ts, nonce)
		reqCiphertext := clientReqAEAD.Seal(nil, clientReqNonce, []byte("offer"), reqAAD)
		reqPlaintext, err := serverReqAEAD.Open(nil, serverReqNonce, reqCiphertext, reqAAD)
		if err != nil {
			t.Fatalf("server request open mode %q error = %v", mode, err)
		}
		if string(reqPlaintext) != "offer" {
			t.Fatalf("request plaintext = %q, want offer", reqPlaintext)
		}
		respAAD := responseAAD(clientKey.Public, ts, nonce)
		respCiphertext := serverRespAEAD.Seal(nil, serverRespNonce, []byte("answer"), respAAD)
		respPlaintext, err := clientRespAEAD.Open(nil, clientRespNonce, respCiphertext, respAAD)
		if err != nil {
			t.Fatalf("client response open mode %q error = %v", mode, err)
		}
		if string(respPlaintext) != "answer" {
			t.Fatalf("response plaintext = %q, want answer", respPlaintext)
		}
	}
	if _, _, _, _, err := deriveSignaling(clientKey, serverKey.Public, "bad", ts, CipherModeChaChaPoly); err == nil {
		t.Fatal("deriveSignaling bad nonce error = nil")
	}
	if _, _, _, _, err := deriveSignaling(clientKey, serverKey.Public, nonce, ts, CipherMode("bad")); err == nil {
		t.Fatal("deriveSignaling bad mode error = nil")
	}
	plain := plaintextAEAD{}
	if plain.NonceSize() != 12 {
		t.Fatalf("plaintext NonceSize = %d, want 12", plain.NonceSize())
	}
	if plain.Overhead() != 0 {
		t.Fatalf("plaintext Overhead = %d, want 0", plain.Overhead())
	}
}

func TestPostOfferRejectsEmptySignalingURLAndDialRejectsNilKey(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	if _, err := postOffer(context.Background(), serverKey, serverKey.Public, "offer", DialConfig{}); err == nil {
		t.Fatal("postOffer empty URL error = nil")
	}
	if _, _, err := Dial(nil, nil, serverKey.Public, DialConfig{}); err == nil {
		t.Fatal("Dial nil key error = nil")
	}
}

func TestWaitForGatheringRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForGathering(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForGathering canceled error = %v, want context.Canceled", err)
	}

	done := make(chan struct{})
	close(done)
	if err := waitForGathering(context.Background(), done); err != nil {
		t.Fatalf("waitForGathering completed error = %v", err)
	}
}

func TestPostOfferAndDialReportSignalingHTTPError(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer httpServer.Close()

	cfg := DialConfig{SignalingURL: httpServer.URL + SignalingPath, CipherMode: CipherModePlaintext}
	if _, err := postOffer(context.Background(), clientKey, serverKey.Public, "offer", cfg); err == nil {
		t.Fatal("postOffer HTTP error = nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := Dial(ctx, clientKey, serverKey.Public, cfg); err == nil {
		t.Fatal("Dial signaling HTTP error = nil")
	}
}

func TestSignalingHandlerClosedListenerAndAcceptOfferInvalidSDP(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	listener, err := (&ListenConfig{CipherMode: CipherModePlaintext}).Listen(key)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	if _, _, err := listener.acceptOffer(context.Background(), clientKey.Public, "not sdp"); err == nil {
		t.Fatal("acceptOffer invalid SDP error = nil")
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, SignalingPath, nil)
	listener.SignalingHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed listener status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func opusPacketForTest() []byte {
	return []byte{0x00, 0xaa}
}

type recordPeerEventHandler struct {
	called bool
	event  giznet.PeerEvent
}

func (h *recordPeerEventHandler) HandlePeerEvent(event giznet.PeerEvent) {
	h.called = true
	h.event = event
}
