package gizclaw

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Hold transport cleanup to exercise the exact interval after the durable
// block and Manager detachment, but before any callback or Close can run.
type deferredPeerCleanupManager struct {
	*Manager
	cleanup func()
}

func (m *deferredPeerCleanupManager) DetachPeerConnections(key giznet.PublicKey) func() {
	m.cleanup = m.Manager.DetachPeerConnections(key)
	return func() {}
}

func TestPeerHTTPStreamRejectsDetachedGenerationBeforeCleanup(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	base := kv.NewMemory(nil)
	peers := &peer.Server{Store: base}
	manager := &deferredPeerCleanupManager{Manager: NewManager(peers)}
	peers.PeerManager = manager
	policy := (*ServerSecurityPolicy)(&Server{manager: manager.Manager})
	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey, policy, testGiznetSecurityPolicy{})
	if _, err := manager.activatePeer(t.Context(), serverConn); err != nil {
		t.Fatal(err)
	}
	host := &PeerConn{Conn: serverConn}
	cleaned := false
	if !manager.RegisterPeerRetirer(clientKey.Public, serverConn, &host.retiring, func() { cleaned = true }) {
		t.Fatal("could not register the active generation")
	}
	t.Cleanup(func() {
		if manager.cleanup != nil {
			manager.cleanup()
		}
	})
	var handled atomic.Int64
	server := gizhttp.NewServer(serverConn, ServicePeerHTTP, rejectRetiringHTTP(host.isRetiring, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})))
	done := make(chan error, 1)
	go func() { done <- server.Serve() }()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		if err := <-done; err != nil && !isPeerServiceClosed(err) {
			t.Error(err)
		}
	})
	request := func() int {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://peer.test/", nil)
		if err != nil {
			t.Fatal(err)
		}
		// Each client opens a fresh real WebRTC service stream.
		client := gizhttp.NewClient(clientConn, ServicePeerHTTP)
		defer client.CloseIdleConnections()
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	// A stream on an already active connection needs no storage query.
	slow := &blockingGetStore{Store: base, entered: make(chan struct{}), release: make(chan struct{})}
	defer close(slow.release)
	peers.Store = slow
	if status := request(); status != http.StatusNoContent {
		t.Fatalf("ordinary stream with slow store = %d", status)
	}
	select {
	case <-slow.entered:
		t.Fatal("ordinary service queried storage")
	default:
	}
	peers.Store = base
	response, err := peers.BlockPeer(t.Context(), adminhttp.BlockPeerRequestObject{PublicKey: clientKey.Public.String()})
	if _, ok := response.(adminhttp.BlockPeer200JSONResponse); err != nil || !ok {
		t.Fatalf("block = %T, %v", response, err)
	}
	if cleaned || manager.cleanup == nil {
		t.Fatal("test did not defer the captured cleanup")
	}
	if status := request(); status != http.StatusConflict {
		t.Fatalf("stream before cleanup = %d, want rejection", status)
	}
	if handled.Load() != 1 {
		t.Fatal("detached generation ran the business handler")
	}
}
