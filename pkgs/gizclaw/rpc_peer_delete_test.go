package gizclaw

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var errTestRPCWrite = errors.New("test RPC write failure")
var errTestPeerDelete = errors.New("test peer delete failure")

type peerDeleteWriteConn struct {
	mu      sync.Mutex
	writes  int
	failAt  int
	entered chan struct{}
	release chan struct{}
}

type trackingGiznetConn struct {
	*testGiznetConn
	closed atomic.Bool
}

type blockingBatchMutateStore struct {
	kv.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type failingBatchMutateStore struct {
	kv.Store
}

func (s *failingBatchMutateStore) BatchMutate(context.Context, []kv.Entry, []kv.Key) error {
	return errTestPeerDelete
}

func (s *blockingBatchMutateStore) BatchMutate(ctx context.Context, entries []kv.Entry, keys []kv.Key) error {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.Store.BatchMutate(ctx, entries, keys)
}

func (c *trackingGiznetConn) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *peerDeleteWriteConn) Read([]byte) (int, error)         { return 0, net.ErrClosed }
func (c *peerDeleteWriteConn) Close() error                     { return nil }
func (c *peerDeleteWriteConn) LocalAddr() net.Addr              { return nil }
func (c *peerDeleteWriteConn) RemoteAddr() net.Addr             { return nil }
func (c *peerDeleteWriteConn) SetDeadline(time.Time) error      { return nil }
func (c *peerDeleteWriteConn) SetReadDeadline(time.Time) error  { return nil }
func (c *peerDeleteWriteConn) SetWriteDeadline(time.Time) error { return nil }

func (c *peerDeleteWriteConn) nextWrite() error {
	c.mu.Lock()
	c.writes++
	write := c.writes
	c.mu.Unlock()
	if write == 1 && c.entered != nil {
		close(c.entered)
		<-c.release
	}
	if write == c.failAt {
		return errTestRPCWrite
	}
	return nil
}

func (c *peerDeleteWriteConn) Write(p []byte) (int, error) {
	if err := c.nextWrite(); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *peerDeleteWriteConn) WriteBuffers(buffers net.Buffers) (int64, error) {
	if err := c.nextWrite(); err != nil {
		return 0, err
	}
	var total int64
	for _, buffer := range buffers {
		total += int64(len(buffer))
	}
	return total, nil
}

func TestRPCPeerDeleteAcknowledgesBeforeTerminalAction(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	publicKey := giznet.PublicKey{1}
	if _, err := peers.SavePeer(context.Background(), apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device:    apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatalf("SavePeer: %v", err)
	}
	terminal := make(chan struct{}, 1)
	server := &rpcServer{
		peer:            peers,
		callerPublicKey: publicKey,
		onPeerDeleted: func() {
			terminal <- struct{}{}
		},
	}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Handle(serverSide) }()

	request := newRPCRequest(
		"delete-self",
		rpcapi.RPCMethodServerPeerDelete,
		mustRPCParams(rpcapi.ServerPeerDeleteRequest{}, (*rpcapi.RPCPayload).FromServerPeerDeleteRequest),
	)
	response, err := callRPC(context.Background(), clientSide, request)
	if err != nil {
		t.Fatalf("callRPC: %v", err)
	}
	if response.Error != nil {
		t.Fatalf("response error = %#v", response.Error)
	}
	if _, err := response.Result.AsServerPeerDeleteResponse(); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	select {
	case <-terminal:
	case <-time.After(time.Second):
		t.Fatal("terminal action was not called after response")
	}
	if _, err := peers.LoadPeer(context.Background(), publicKey); err == nil {
		t.Fatal("Peer remains active after self-delete")
	}
	if pending, err := pendingdeletion.HasLocator(context.Background(), store, pendingdeletion.KindPeer, publicKey.String()); err != nil || !pending {
		t.Fatalf("peer pending deletion = %v, error = %v", pending, err)
	}
	_ = clientSide.Close()
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server Handle: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server Handle did not stop")
	}
}

func TestRPCPeerDeleteClosesAfterWriteFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		failAt int
	}{
		{name: "response", failAt: 1},
		{name: "eos", failAt: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := kv.NewMemory(nil)
			peers := &peer.Server{Store: store}
			publicKey := giznet.PublicKey{2, byte(test.failAt)}
			if _, err := peers.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
				t.Fatalf("EnsureConnectedPeer: %v", err)
			}
			manager := NewManager(peers)
			transport := &trackingGiznetConn{testGiznetConn: &testGiznetConn{publicKey: publicKey}}
			manager.SetPeerUp(publicKey, transport)
			peerConn := &PeerConn{Conn: transport, Service: &PeerService{manager: manager}}
			peerConn.initRPC()
			stream, err := newRPCStream(context.Background(), &peerDeleteWriteConn{failAt: test.failAt})
			if err != nil {
				t.Fatalf("newRPCStream: %v", err)
			}
			defer stream.Close()
			stream.requestEOSAlreadyConsumed = true
			request := newRPCRequest("delete-self", rpcapi.RPCMethodServerPeerDelete, mustRPCParams(rpcapi.ServerPeerDeleteRequest{}, (*rpcapi.RPCPayload).FromServerPeerDeleteRequest))
			if err := peerConn.rpc.handlePeerDelete(context.Background(), stream, request); !errors.Is(err, errTestRPCWrite) {
				t.Fatalf("handlePeerDelete error = %v, want write failure", err)
			}
			if !peerConn.isRetiring() {
				t.Fatal("peer was not marked retiring after durable delete")
			}
			if !peerConn.isClosed() || !transport.closed.Load() {
				t.Fatal("full Giznet connection was not closed after write failure")
			}
			if _, ok := manager.Peer(publicKey); ok {
				t.Fatal("retiring connection remained registered in Manager")
			}
		})
	}
}

func TestRPCPeerDeleteRejectsNewWorkWhileAcknowledgementIsPending(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	publicKey := giznet.PublicKey{3}
	if _, err := peers.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	var retiring atomic.Bool
	retired := make(chan struct{})
	closed := make(chan struct{}, 1)
	server := &rpcServer{
		peer:            peers,
		callerPublicKey: publicKey,
		isPeerRetiring:  retiring.Load,
		onPeerRetiring:  func() { retiring.Store(true); close(retired) },
		onPeerDeleted:   func() { closed <- struct{}{} },
	}
	conn := &peerDeleteWriteConn{entered: make(chan struct{}), release: make(chan struct{})}
	stream, err := newRPCStream(context.Background(), conn)
	if err != nil {
		t.Fatalf("newRPCStream: %v", err)
	}
	defer stream.Close()
	stream.requestEOSAlreadyConsumed = true
	deleteRequest := newRPCRequest("delete-self", rpcapi.RPCMethodServerPeerDelete, mustRPCParams(rpcapi.ServerPeerDeleteRequest{}, (*rpcapi.RPCPayload).FromServerPeerDeleteRequest))
	deleteErr := make(chan error, 1)
	go func() { deleteErr <- server.handlePeerDelete(context.Background(), stream, deleteRequest) }()
	<-retired
	<-conn.entered
	response, err := server.dispatch(context.Background(), newRPCRequest("ping", rpcapi.RPCMethodAllPing, nil))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeConflict {
		t.Fatalf("retiring response = %#v, want conflict", response)
	}
	close(conn.release)
	if err := <-deleteErr; err != nil {
		t.Fatalf("handlePeerDelete: %v", err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("terminal close was not called")
	}
}

func TestRPCPeerDeleteRejectsNewWorkBeforeDurableDeleteCommits(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	publicKey := giznet.PublicKey{3, 1}
	if _, err := peers.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	blockingStore := &blockingBatchMutateStore{
		Store:   store,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	peers.Store = blockingStore
	manager := NewManager(peers)
	transport := &testGiznetConn{publicKey: publicKey}
	manager.SetPeerUp(publicKey, transport)
	peerConn := &PeerConn{Conn: transport, Service: &PeerService{manager: manager}}
	peerConn.initRPC()
	stream, err := newRPCStream(context.Background(), &peerDeleteWriteConn{})
	if err != nil {
		t.Fatalf("newRPCStream: %v", err)
	}
	defer stream.Close()
	stream.requestEOSAlreadyConsumed = true
	request := newRPCRequest("delete-self", rpcapi.RPCMethodServerPeerDelete, mustRPCParams(rpcapi.ServerPeerDeleteRequest{}, (*rpcapi.RPCPayload).FromServerPeerDeleteRequest))
	deleteErr := make(chan error, 1)
	go func() { deleteErr <- peerConn.rpc.handlePeerDelete(context.Background(), stream, request) }()
	<-blockingStore.entered
	if !peerConn.isRetiring() {
		t.Fatal("peer accepted new work while durable delete was committing")
	}
	response, err := peerConn.rpc.dispatch(context.Background(), newRPCRequest("ping", rpcapi.RPCMethodAllPing, nil))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeConflict {
		t.Fatalf("retiring response = %#v, want conflict", response)
	}
	close(blockingStore.release)
	if err := <-deleteErr; err != nil {
		t.Fatalf("handlePeerDelete: %v", err)
	}
	if _, ok := manager.Peer(publicKey); ok {
		t.Fatal("deleted connection remained registered in Manager")
	}
}

func TestManagerDeleteActivePeerRestoresConnectionAfterDeleteFailure(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	publicKey := giznet.PublicKey{3, 2}
	if _, err := peers.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	peers.Store = &failingBatchMutateStore{Store: store}
	manager := NewManager(peers)
	transport := &testGiznetConn{publicKey: publicKey}
	manager.SetPeerUp(publicKey, transport)
	peerConn := &PeerConn{Conn: transport, Service: &PeerService{manager: manager}}
	registration := &runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Name: "profile"}}
	peerConn.registration.Store(registration)

	err := manager.deleteActivePeer(context.Background(), publicKey, transport, peerConn.beginRetiring)
	if !errors.Is(err, errTestPeerDelete) {
		t.Fatalf("deleteActivePeer error = %v, want %v", err, errTestPeerDelete)
	}
	if peerConn.isRetiring() {
		t.Fatal("failed delete left connection retiring")
	}
	if got := peerConn.registration.Load(); got != registration {
		t.Fatalf("failed delete registration = %p, want %p", got, registration)
	}
	if got, ok := manager.Peer(publicKey); !ok || got != transport {
		t.Fatalf("failed delete active connection = %v, %v", got, ok)
	}
}

func TestRPCPeerDeleteRejectsSupersededConnection(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	manager := NewManager(peers)
	publicKey := giznet.PublicKey{4}
	if _, err := peers.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	oldConn := &testGiznetConn{publicKey: publicKey}
	replacement := &testGiznetConn{publicKey: publicKey}
	manager.SetPeerUp(publicKey, oldConn)
	manager.SetPeerUp(publicKey, replacement)
	oldPeer := &PeerConn{Conn: oldConn, Service: &PeerService{manager: manager}}
	oldPeer.initRPC()

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	serverErr := make(chan error, 1)
	go func() { serverErr <- oldPeer.rpc.Handle(serverSide) }()
	request := newRPCRequest("stale-delete", rpcapi.RPCMethodServerPeerDelete, mustRPCParams(rpcapi.ServerPeerDeleteRequest{}, (*rpcapi.RPCPayload).FromServerPeerDeleteRequest))
	response, err := callRPC(context.Background(), clientSide, request)
	if err != nil {
		t.Fatalf("callRPC: %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeConflict {
		t.Fatalf("response = %#v, want conflict", response)
	}
	if _, err := peers.LoadPeer(context.Background(), publicKey); err != nil {
		t.Fatalf("replacement active Peer was deleted: %v", err)
	}
	if got, ok := manager.Peer(publicKey); !ok || got != replacement {
		t.Fatalf("replacement connection = %v, %v", got, ok)
	}
	_ = clientSide.Close()
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server Handle: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server Handle did not stop")
	}
}
