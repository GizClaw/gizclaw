package gizclaw

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Unlike a once-gated whole Store, this gate blocks only one Peer and honors
// the admission context, allowing the test to isolate accept-loop scheduling.
type edgeAdmissionGateStore struct {
	kv.Store
	peer    string
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *edgeAdmissionGateStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	if len(key) > 0 && key[len(key)-1] == s.peer {
		s.once.Do(func() { close(s.entered) })
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.Store.Get(ctx, key)
}

func newEdgeTunnelAdmissionFixture(t *testing.T, store kv.Store) (*PeerConn, *giztunnel.Router, <-chan struct{}) {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeConn, serverConn := newTestWebRTCConnPair(t, serverKey, edgeKey, testGiznetSecurityPolicy{}, testGiznetSecurityPolicy{})
	edgeRouter, err := giztunnel.NewRouter(edgeConn.(*gizwebrtc.Conn), giztunnel.Config{})
	if err != nil {
		t.Fatal(err)
	}
	host := &PeerConn{Conn: serverConn, Service: &PeerService{manager: NewManager(&peer.Server{Store: store})}}
	if _, err := host.initEdgeTunnelRouter(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = host.serveEdgeTunnel()
		close(done)
	}()
	t.Cleanup(func() {
		_ = host.tunnelRouter.Close()
		_ = edgeRouter.Close()
		_ = edgeConn.Close()
		_ = serverConn.Close()
		select {
		case <-done:
			if serveErr != nil {
				t.Errorf("serveEdgeTunnel: %v", serveErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("tunnel admission workers did not stop")
		}
	})
	return host, edgeRouter, done
}

func TestServeEdgeTunnelAdmitsSiblingDuringBlockedPeerRead(t *testing.T) {
	blockedKey, siblingKey := giznet.PublicKey{91}, giznet.PublicKey{92}
	gate := &edgeAdmissionGateStore{Store: kv.NewMemory(nil), peer: blockedKey.String(), entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(gate.release) })
	t.Cleanup(release)
	_, router, _ := newEdgeTunnelAdmissionFixture(t, gate)
	firstID, err := giztunnel.NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	firstCtx, cancelFirst := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancelFirst()
	firstDone := make(chan error, 1)
	go func() {
		conn, err := router.Dial(firstCtx, giztunnel.SessionDeclaration{SessionID: firstID, ClientPublicKey: blockedKey})
		if conn != nil {
			_ = conn.Close()
		}
		firstDone <- err
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first session did not enter Peer storage")
	}
	secondID, err := giztunnel.NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	sibling, err := router.Dial(ctx, giztunnel.SessionDeclaration{SessionID: secondID, ClientPublicKey: siblingKey})
	if err != nil {
		t.Fatalf("unrelated session waited behind blocked Peer admission: %v", err)
	}
	_ = sibling.Close()
	select {
	case err := <-firstDone:
		t.Fatalf("blocked admission completed before storage release: %v", err)
	default:
	}
	release()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first admission after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first admission did not resume")
	}
}

func TestEdgeTunnelAdmissionCancelsRecordLockWait(t *testing.T) {
	key := giznet.PublicKey{93}
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	host := &PeerConn{Service: &PeerService{manager: NewManager(peers)}}
	unlock := peers.IconLocks.LockRecord(key.String())
	defer unlock()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- host.admitEdgeTunnelSession(ctx, giztunnel.SessionDeclaration{ClientPublicKey: key}) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("admission lock cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled admission remained blocked on record lock")
	}
	if _, err := peers.LoadPeer(t.Context(), key); !errors.Is(err, peer.ErrPeerNotFound) {
		t.Fatalf("canceled admission created a Peer: %v", err)
	}
}
