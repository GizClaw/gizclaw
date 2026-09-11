package giztunnel

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type memTunnelPair struct {
	edge, server   *Conn
	edgeRouter     *Router
	serverRouter   *Router
	edgePhysical   *memChannelConn
	serverPhysical *memChannelConn
	declaration    SessionDeclaration
}

func newMemDeclaration(t *testing.T) SessionDeclaration {
	t.Helper()
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	return SessionDeclaration{SessionID: id, ClientPublicKey: clientKey.Public, RemoteAddr: "198.51.100.8:4242"}
}

func newMemRouters(t *testing.T, edgeConfig, serverConfig Config) memTunnelPair {
	t.Helper()
	edgePhysical, serverPhysical := newMemChannelConnPair()
	serverConfig.AcceptSessions = true
	serverRouter, err := NewRouter(serverPhysical, serverConfig)
	if err != nil {
		t.Fatal(err)
	}
	edgeRouter, err := NewRouter(edgePhysical, edgeConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = edgeRouter.Close()
		_ = serverRouter.Close()
		_ = edgePhysical.Close()
		_ = serverPhysical.Close()
	})
	return memTunnelPair{
		edgeRouter: edgeRouter, serverRouter: serverRouter,
		edgePhysical: edgePhysical, serverPhysical: serverPhysical,
	}
}

func (p *memTunnelPair) dial(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	declaration := newMemDeclaration(t)
	type acceptResult struct {
		conn *Conn
		decl SessionDeclaration
		err  error
	}
	acceptCh := make(chan acceptResult, 1)
	go func() {
		conn, decl, err := p.serverRouter.Accept(ctx)
		acceptCh <- acceptResult{conn: conn, decl: decl, err: err}
	}()
	edge, err := p.edgeRouter.Dial(ctx, declaration)
	if err != nil {
		t.Fatal(err)
	}
	result := <-acceptCh
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.decl != declaration {
		t.Fatalf("declaration = %+v, want %+v", result.decl, declaration)
	}
	p.edge, p.server, p.declaration = edge, result.conn, declaration
}

func newMemTunnelPair(t *testing.T, edgeConfig, serverConfig Config) memTunnelPair {
	t.Helper()
	pair := newMemRouters(t, edgeConfig, serverConfig)
	pair.dial(t)
	return pair
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestChannelConnTransportAggregatesLogicalConnection(t *testing.T) {
	pair := newMemTunnelPair(t, Config{}, Config{})
	if !pair.server.PublicKey().Equal(pair.declaration.ClientPublicKey) {
		t.Fatalf("logical public key = %s", pair.server.PublicKey())
	}

	listener := pair.server.ListenService(7)
	edgeStream, err := pair.edge.Dial(7)
	if err != nil {
		t.Fatal(err)
	}
	serverStream, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edgeStream.Write([]byte("uplink")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	if _, err := io.ReadFull(serverStream, buf[:6]); err != nil || string(buf[:6]) != "uplink" {
		t.Fatalf("service read = %q, %v", buf[:6], err)
	}
	if _, err := serverStream.Write([]byte("reply")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(edgeStream, buf[:5]); err != nil || string(buf[:5]) != "reply" {
		t.Fatalf("service reply = %q, %v", buf[:5], err)
	}
	if got := pair.edgeRouter.ActiveChannels(); got != 3 {
		t.Fatalf("edge active channels = %d, want 3", got)
	}

	if _, err := pair.edge.Write(0x40, []byte("event")); err != nil {
		t.Fatal(err)
	}
	protocol, n, err := pair.server.Read(buf)
	if err != nil || protocol != 0x40 || string(buf[:n]) != "event" {
		t.Fatalf("packet read protocol=%x payload=%q err=%v", protocol, buf[:n], err)
	}

	if _, err := pair.edge.Write(giznet.ProtocolOpusPacket, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	protocol, n, err = pair.serverPhysical.Read(buf)
	if err != nil || protocol != giznet.ProtocolTunnelPacket {
		t.Fatalf("physical packet protocol=%x err=%v", protocol, err)
	}
	if err := pair.serverRouter.HandlePacket(buf[:n]); err != nil {
		t.Fatal(err)
	}
	protocol, n, err = pair.server.Read(buf)
	if err != nil || protocol != giznet.ProtocolOpusPacket || string(buf[:n]) != string([]byte{1, 2, 3}) {
		t.Fatalf("opus read protocol=%x payload=%v err=%v", protocol, buf[:n], err)
	}

	_ = edgeStream.Close()
	_ = serverStream.Close()
	waitFor(t, "service channel release", func() bool {
		return pair.edgeRouter.ActiveChannels() == 2 && pair.serverRouter.ActiveChannels() == 2
	})
}

func TestDialReassemblesFragmentedSessionResult(t *testing.T) {
	pair := newMemRouters(t, Config{}, Config{})
	pair.edgePhysical.readChunk = 1
	pair.dial(t)
	if pair.edge == nil || pair.server == nil {
		t.Fatal("session was not established")
	}
}

func TestDialReassemblesFragmentedRejection(t *testing.T) {
	edgePhysical, serverPhysical := newMemChannelConnPair()
	edgePhysical.readChunk = 1
	serverRouter, err := NewRouter(serverPhysical, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer serverRouter.Close()
	edgeRouter, err := NewRouter(edgePhysical, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer edgeRouter.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = edgeRouter.Dial(ctx, newMemDeclaration(t))
	if !errors.Is(err, ErrSessionRejected) || !strings.Contains(err.Error(), "remote session creation is disabled") {
		t.Fatalf("Dial error = %v, want fragmented rejection reason", err)
	}
	if got := edgeRouter.ActiveChannels(); got != 0 {
		t.Fatalf("active channels after rejection = %d", got)
	}
}

func TestDialFailsOnTruncatedOrInvalidSessionResult(t *testing.T) {
	valid, err := encodeSessionResult(sessionRejected, "no")
	if err != nil {
		t.Fatal(err)
	}
	invalid := append([]byte(nil), valid...)
	invalid[0] ^= 0xff
	for _, test := range []struct {
		name  string
		frame []byte
		want  error
	}{
		{name: "truncated header", frame: valid[:4], want: io.ErrUnexpectedEOF},
		{name: "truncated reason", frame: valid[:len(valid)-1], want: io.ErrUnexpectedEOF},
		{name: "empty", frame: nil, want: io.EOF},
		{name: "bad magic", frame: invalid, want: ErrInvalidFrame},
	} {
		t.Run(test.name, func(t *testing.T) {
			edgePhysical, serverPhysical := newMemChannelConnPair()
			defer edgePhysical.Close()
			defer serverPhysical.Close()
			if _, err := serverPhysical.HandleChannels(LabelPrefix, func(channel giznet.Channel) {
				if strings.HasSuffix(channel.Label(), "/packet") {
					return
				}
				_, _ = channel.Write(test.frame)
				_ = channel.Close()
			}); err != nil {
				t.Fatal(err)
			}
			edgeRouter, err := NewRouter(edgePhysical, Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer edgeRouter.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			declaration := newMemDeclaration(t)
			if _, err := edgeRouter.Dial(ctx, declaration); !errors.Is(err, test.want) {
				t.Fatalf("Dial error = %v, want %v", err, test.want)
			}
			edgeRouter.mu.Lock()
			pending := len(edgeRouter.pending)
			edgeRouter.mu.Unlock()
			if pending != 0 || edgeRouter.ActiveChannels() != 0 {
				t.Fatalf("pending=%d active=%d after failed dial", pending, edgeRouter.ActiveChannels())
			}
		})
	}
}

func TestChannelReliabilityMismatchIsRejected(t *testing.T) {
	pair := newMemTunnelPair(t, Config{}, Config{})
	ctx := t.Context()
	service, err := serviceLabel(pair.declaration.SessionID, 7, 101)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := packetLabel(pair.declaration.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		label  string
		remote giznet.ChannelReliability
	}{
		{label: service, remote: giznet.ChannelUnreliable},
		{label: service, remote: giznet.ChannelReliabilityUnknown},
		{label: packet, remote: giznet.ChannelReliable},
	} {
		channel, err := pair.edgePhysical.openChannel(ctx, test.label, giznet.ChannelReliable, test.remote)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := channel.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Fatalf("%s as %s: read = %v, want EOF from rejected channel", test.label, test.remote, err)
		}
		_ = channel.Close()
	}
	if got := pair.serverRouter.ActiveChannels(); got != 2 {
		t.Fatalf("server active channels = %d, want session channels only", got)
	}
}

func TestUnacceptedSessionTimesOutOverChannelConn(t *testing.T) {
	pair := newMemRouters(t, Config{HandshakeTimeout: 200 * time.Millisecond}, Config{HandshakeTimeout: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := pair.edgeRouter.Dial(ctx, newMemDeclaration(t)); err == nil {
		t.Fatal("Dial succeeded without server application acceptance")
	}
	waitFor(t, "admission release", func() bool {
		return pair.edgeRouter.ActiveChannels() == 0 && pair.serverRouter.ActiveChannels() == 0
	})
}

func TestRouterCloseUnregistersChannelHandler(t *testing.T) {
	pair := newMemTunnelPair(t, Config{}, Config{})
	if !pair.serverPhysical.handlerRegistered() || pair.serverPhysical.deliveredChannels() != 2 {
		t.Fatalf("registered=%t delivered=%d", pair.serverPhysical.handlerRegistered(), pair.serverPhysical.deliveredChannels())
	}
	if err := pair.serverRouter.Close(); err != nil {
		t.Fatal(err)
	}
	if pair.serverPhysical.handlerRegistered() {
		t.Fatal("router close left the channel handler registered")
	}
	waitFor(t, "delivered channel close", func() bool { return pair.serverPhysical.deliveredChannels() == 0 })
	unregister, err := pair.serverPhysical.HandleChannels(LabelPrefix, func(giznet.Channel) {})
	if err != nil {
		t.Fatalf("prefix was not released: %v", err)
	}
	unregister()
}

func TestAssociationWriteBudgetFollowsConfig(t *testing.T) {
	physical, peer := newMemChannelConnPair()
	defer physical.Close()
	defer peer.Close()
	for _, test := range []struct {
		name string
		cfg  Config
		want uint64
		err  bool
	}{
		{name: "default", want: defaultMaxAssociationBuffer},
		{name: "explicit", cfg: Config{MaxAssociationBufferedBytes: 8 << 20}, want: 8 << 20},
		{name: "below session budget", cfg: Config{MaxBufferedBytes: 2 << 20, MaxAssociationBufferedBytes: 1 << 20}, err: true},
		{name: "above bound", cfg: Config{MaxAssociationBufferedBytes: maxAssociationBuffer + 1}, err: true},
		{name: "negative", cfg: Config{MaxAssociationBufferedBytes: -1}, err: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, err := NewRouter(physical, test.cfg)
			if test.err {
				if err == nil {
					_ = router.Close()
					t.Fatal("NewRouter accepted an invalid association budget")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if got := router.writeBudget.Limit(); got != test.want {
				t.Fatalf("association budget = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNewRouterRejectsNilTransports(t *testing.T) {
	var typedNil *memChannelConn
	for _, transport := range []giznet.ChannelConn{nil, typedNil} {
		if router, err := NewRouter(transport, Config{}); !errors.Is(err, giznet.ErrNilConn) {
			if router != nil {
				_ = router.Close()
			}
			t.Fatalf("NewRouter(%T) error = %v, want ErrNilConn", transport, err)
		}
	}
}
