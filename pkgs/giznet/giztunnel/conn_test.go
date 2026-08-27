package giztunnel

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type allowAllWebRTCPolicy struct{}

func (allowAllWebRTCPolicy) AllowPeer(giznet.PublicKey) bool            { return true }
func (allowAllWebRTCPolicy) AllowService(giznet.PublicKey, uint64) bool { return true }

type tunnelPair struct {
	edge, server   *Conn
	edgeRouter     *Router
	serverRouter   *Router
	edgePhysical   *gizwebrtc.Conn
	serverPhysical *gizwebrtc.Conn
	declaration    SessionDeclaration
}

func newTunnelPair(t *testing.T, edgeConfig, serverConfig Config) tunnelPair {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllWebRTCPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(listener.SignalingHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	clientListener, physical, err := gizwebrtc.Dial(ctx, edgeKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL:   httpServer.URL + gizwebrtc.SignalingPath,
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllWebRTCPolicy{},
	})
	if err != nil {
		cancel()
		httpServer.Close()
		_ = listener.Close()
		t.Fatal(err)
	}
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	edgePhysical := physical
	serverPhysical := accepted.(*gizwebrtc.Conn)
	serverConfig.AcceptSessions = true
	serverRouter, err := NewRouter(serverPhysical, serverConfig)
	if err != nil {
		t.Fatal(err)
	}
	edgeRouter, err := NewRouter(edgePhysical, edgeConfig)
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	declaration := SessionDeclaration{
		SessionID:       id,
		ClientPublicKey: clientKey.Public,
		RemoteAddr:      "198.51.100.8:4242",
	}
	type acceptResult struct {
		conn *Conn
		decl SessionDeclaration
		err  error
	}
	acceptCh := make(chan acceptResult, 1)
	go func() {
		conn, decl, acceptErr := serverRouter.Accept(ctx)
		acceptCh <- acceptResult{conn: conn, decl: decl, err: acceptErr}
	}()
	edgeConn, err := edgeRouter.Dial(ctx, declaration)
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
	t.Cleanup(func() {
		_ = edgeRouter.Close()
		_ = serverRouter.Close()
		_ = edgePhysical.Close()
		_ = serverPhysical.Close()
		_ = clientListener.Close()
		httpServer.Close()
		_ = listener.Close()
		cancel()
	})
	return tunnelPair{
		edge: edgeConn, server: result.conn,
		edgeRouter: edgeRouter, serverRouter: serverRouter,
		edgePhysical: edgePhysical, serverPhysical: serverPhysical,
		declaration: declaration,
	}
}

func TestNativeChannelsAggregateLogicalConnection(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{
		AllowRemoteService: func(client giznet.PublicKey, service uint64) bool {
			return client.Equal(pairKeyPlaceholder()) || service == 7
		},
	})
	if !pair.server.PublicKey().Equal(pair.declaration.ClientPublicKey) {
		t.Fatalf("logical public key = %s", pair.server.PublicKey())
	}
	info := pair.server.PeerInfo()
	if info == nil || info.Endpoint == nil || info.Endpoint.String() != pair.declaration.RemoteAddr {
		t.Fatalf("logical peer info = %+v", info)
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
	defer edgeStream.Close()
	defer serverStream.Close()
	if _, err := edgeStream.Write([]byte("native service")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := serverStream.Read(buf)
	if err != nil || string(buf[:n]) != "native service" {
		t.Fatalf("service read = %q, %v", buf[:n], err)
	}

	if _, err := pair.edge.Write(0x40, []byte("event")); err != nil {
		t.Fatal(err)
	}
	protocol, n, err := pair.server.Read(buf)
	if err != nil || protocol != 0x40 || string(buf[:n]) != "event" {
		t.Fatalf("packet read protocol=%x payload=%q err=%v", protocol, buf[:n], err)
	}
	if got := pair.edgeRouter.ActiveChannels(); got != 3 {
		t.Fatalf("active channels after stream close scheduling = %d, want at least session channels", got)
	}
}

func TestDialWaitsForServerApplicationAcceptanceAfterDCEPOpen(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllWebRTCPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(listener.SignalingHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	clientListener, edgePhysical, err := gizwebrtc.Dial(ctx, edgeKey, serverKey.Public, gizwebrtc.DialConfig{
		SignalingURL:   httpServer.URL + gizwebrtc.SignalingPath,
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: allowAllWebRTCPolicy{},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	serverPhysical := accepted.(*gizwebrtc.Conn)
	serverRouter, err := NewRouter(serverPhysical, Config{AcceptSessions: true})
	if err != nil {
		t.Fatal(err)
	}
	edgeRouter, err := NewRouter(edgePhysical, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = edgeRouter.Close()
		_ = serverRouter.Close()
		_ = clientListener.Close()
		_ = edgePhysical.Close()
		_ = serverPhysical.Close()
		_ = listener.Close()
		httpServer.Close()
		cancel()
	})
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	declaration := SessionDeclaration{SessionID: id, ClientPublicKey: clientKey.Public}
	dialDone := make(chan error, 1)
	go func() {
		_, dialErr := edgeRouter.Dial(ctx, declaration)
		dialDone <- dialErr
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		serverRouter.mu.Lock()
		attached := serverRouter.sessions[id] != nil
		serverRouter.mu.Unlock()
		if attached {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native control and packet channels were not attached")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-dialDone:
		t.Fatalf("Dial returned before Router.Accept: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	serverConn, got, err := serverRouter.Accept(ctx)
	if err != nil || got != declaration {
		t.Fatalf("Accept = %+v, %v", got, err)
	}
	defer serverConn.Close()
	select {
	case err := <-dialDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Dial did not return after explicit Server acceptance")
	}
}

func TestUnacceptedSessionExpiresAndReleasesAdmissionSlotAndTombstone(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	config := Config{HandshakeTimeout: 25 * time.Millisecond}.withDefaults()
	router := &Router{
		cfg:               config,
		sessions:          make(map[SessionID]*Conn),
		pending:           make(map[SessionID]*pendingSession),
		retired:           make(map[SessionID]*time.Timer),
		pendingAdmissions: 1,
		closeCh:           make(chan struct{}),
	}
	declaration := SessionDeclaration{SessionID: id, ClientPublicKey: key.Public}
	conn := newConn(router, declaration, &trackedChannel{}, &trackedChannel{}, false, 2)
	conn.admissionSlot.Store(true)
	router.sessions[id] = conn
	conn.armAdmissionTimeout(config.HandshakeTimeout)

	select {
	case <-conn.closeCh:
	case <-time.After(time.Second):
		t.Fatal("unaccepted session did not expire")
	}
	if conn.acceptApplication() {
		t.Fatal("expired session was accepted")
	}
	cleanupDeadline := time.Now().Add(time.Second)
	for {
		router.mu.Lock()
		cleaned := router.pendingAdmissions == 0 && router.sessions[id] == nil
		router.mu.Unlock()
		if cleaned {
			break
		}
		if time.Now().After(cleanupDeadline) {
			t.Fatal("expired session did not release its admission slot and registry entry")
		}
		time.Sleep(time.Millisecond)
	}
	router.mu.Lock()
	if _, ok := router.retired[id]; !ok {
		t.Fatal("expired session has no callback tombstone")
	}
	if err := router.reservePendingLocked(declaration, true); !errors.Is(err, ErrSessionExists) {
		t.Fatalf("reuse during tombstone error = %v", err)
	}
	router.mu.Unlock()

	deadline := time.Now().Add(time.Second)
	for {
		router.mu.Lock()
		_, retired := router.retired[id]
		router.mu.Unlock()
		if !retired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session tombstone did not expire")
		}
		time.Sleep(time.Millisecond)
	}
}

func pairKeyPlaceholder() giznet.PublicKey { return giznet.PublicKey{} }

func TestServerUsesEvenServiceChannelIDs(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	listener := pair.edge.ListenService(9)
	serverStream, err := pair.server.Dial(9)
	if err != nil {
		t.Fatal(err)
	}
	edgeStream, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer serverStream.Close()
	defer edgeStream.Close()
	if _, err := serverStream.Write([]byte("server")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := edgeStream.Read(buf)
	if err != nil || string(buf[:n]) != "server" {
		t.Fatalf("read = %q, %v", buf[:n], err)
	}
}

func TestRemoteServiceChannelIDCannotBeReusedAfterClose(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	listener := pair.server.ListenService(9)
	first, err := pair.edge.Dial(9)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := peer.Close(); err != nil {
		t.Fatal(err)
	}

	label, err := serviceLabel(pair.declaration.SessionID, 9, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	reused, err := pair.edgePhysical.OpenNativeChannel(ctx, label, gizwebrtc.NativeChannelOptions{Ordered: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reused.Close()
	if err := reused.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := reused.Read(make([]byte, 1)); err == nil {
		t.Fatal("reused live-session channel ID remained open")
	}
	deadline := time.Now().Add(5 * time.Second)
	for pair.serverRouter.ActiveChannels() != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("server active channels = %d, want 2", pair.serverRouter.ActiveChannels())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDuplicatePacketChannelDoesNotCreatePendingSession(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	label, err := packetLabel(pair.declaration.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	zero := uint16(0)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	duplicate, err := pair.edgePhysical.OpenNativeChannel(ctx, label, gizwebrtc.NativeChannelOptions{
		Ordered: false, MaxRetransmits: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer duplicate.Close()
	if err := duplicate.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := duplicate.ReadMessage(make([]byte, 1)); err == nil {
		t.Fatal("duplicate packet channel remained open")
	}
	pair.serverRouter.mu.Lock()
	_, pending := pair.serverRouter.pending[pair.declaration.SessionID]
	pair.serverRouter.mu.Unlock()
	if pending {
		t.Fatal("duplicate packet channel created a pending session")
	}
}

func TestPartiallyReliableServiceChannelIsRejected(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	label, err := serviceLabel(pair.declaration.SessionID, 9, 1)
	if err != nil {
		t.Fatal(err)
	}
	lifetime := uint16(250)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	channel, err := pair.edgePhysical.OpenNativeChannel(ctx, label, gizwebrtc.NativeChannelOptions{
		Ordered: true, MaxPacketLifeTime: &lifetime,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	if err := channel.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := channel.Read(make([]byte, 1)); err == nil {
		t.Fatal("partially reliable service channel remained open")
	}
	if got := pair.serverRouter.ActiveChannels(); got != 2 {
		t.Fatalf("server active channels = %d, want 2", got)
	}
}

func TestLateServiceChannelAfterSessionCloseReleasesCapacity(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	pair := newTunnelPair(t, Config{}, Config{
		AllowRemoteService: func(giznet.PublicKey, uint64) bool {
			close(entered)
			<-release
			return true
		},
	})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	label, err := serviceLabel(pair.declaration.SessionID, 9, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	channel, err := pair.edgePhysical.OpenNativeChannel(ctx, label, gizwebrtc.NativeChannelOptions{Ordered: true})
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("service admission did not reach policy callback")
	}
	if err := pair.server.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for pair.serverRouter.ActiveChannels() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("server active channels = %d, want 0", pair.serverRouter.ActiveChannels())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSessionCloseDoesNotClosePhysicalConnection(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	if err := pair.edge.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for pair.server.PeerInfo().State != giznet.PeerStateOffline && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := pair.edgePhysical.Write(0x55, []byte("physical-alive")); err != nil {
		t.Fatalf("physical connection closed with session: %v", err)
	}
}

func TestChannelCapacityIsPerSessionAndAssociation(t *testing.T) {
	pair := newTunnelPair(t,
		Config{MaxChannelsPerSession: 3, MaxChannels: 6},
		Config{MaxChannelsPerSession: 3, MaxChannels: 6},
	)
	stream, err := pair.edge.Dial(1)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = pair.edge.Dial(2)
	if !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("Dial beyond session channel limit error = %v", err)
	}
	capacity, ok := channelCapacityFromError(err)
	if !ok || capacity.scope != "session" || capacity.active != 3 || capacity.limit != 3 {
		t.Fatalf("session capacity error = %#v, %v", capacity, err)
	}
}

func TestAssociationCapacityCarriesExactEstablishedSnapshot(t *testing.T) {
	pair := newTunnelPair(t,
		Config{MaxChannelsPerSession: 32, MaxChannels: 3},
		Config{MaxChannelsPerSession: 32, MaxChannels: 3},
	)
	stream, err := pair.edge.Dial(1)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = pair.edge.Dial(2)
	if !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("Dial beyond association channel limit error = %v", err)
	}
	capacity, ok := channelCapacityFromError(err)
	if !ok || capacity.scope != "association" || capacity.active != 3 || capacity.limit != 3 {
		t.Fatalf("association capacity error = %#v, %v", capacity, err)
	}
}

func TestPendingAndClosedCapacityErrorsHaveNoEstablishedSnapshot(t *testing.T) {
	router := &Router{
		cfg:      Config{MaxChannelsPerSession: 3, MaxChannels: 3},
		pending:  make(map[SessionID]*pendingSession),
		sessions: make(map[SessionID]*Conn),
	}
	id := SessionID{1}
	router.pending[id] = &pendingSession{active: 3}
	router.activeChannels = 3
	_, err := router.reserveChannelLocked(id)
	if !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("pending capacity error = %v", err)
	}
	if _, ok := channelCapacityFromError(err); ok {
		t.Fatalf("pending capacity unexpectedly had established metadata: %v", err)
	}

	router.closed = true
	_, err = router.reserveChannelLocked(id)
	if !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("closed router capacity error = %v", err)
	}
	if _, ok := channelCapacityFromError(err); ok {
		t.Fatalf("closed router error unexpectedly had capacity metadata: %v", err)
	}
}

func TestOpusRemainsOnSharedPhysicalPacketLane(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 64*1024)
		protocol, n, err := pair.serverPhysical.Read(buf)
		if err == nil && protocol == giznet.ProtocolTunnelPacket {
			err = pair.serverRouter.HandlePacket(buf[:n])
		}
		done <- err
	}()
	payload := []byte{1, 2, 3, 4}
	if _, err := pair.edge.Write(giznet.ProtocolOpusPacket, payload); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	protocol, n, err := pair.server.Read(buf)
	if err != nil || protocol != giznet.ProtocolOpusPacket || string(buf[:n]) != string(payload) {
		t.Fatalf("opus read protocol=%x payload=%v err=%v", protocol, buf[:n], err)
	}
}

func TestClosedControlPropagatesToLogicalConnection(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	if err := pair.server.control.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, _, err := pair.edge.Read(make([]byte, 1))
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, giznet.ErrConnClosed) {
				t.Logf("logical close cause = %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("logical connection did not close")
		}
	}
}
