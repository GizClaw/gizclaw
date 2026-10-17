package gizclaw

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestDeviceMetricsCountReplacementFencesAndLateDisconnect(t *testing.T) {
	srv := &Server{}
	m := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	m.deviceCount = &srv.deviceCount
	key := giznet.PublicKey{1}
	old, next := &testGiznetConn{publicKey: key}, &testGiznetConn{publicKey: key}
	assertCount := func(want float64) {
		t.Helper()
		if got := srv.DeviceMetricsSnapshot(time.Now())[0].Value; got != want {
			t.Fatalf("device count = %v, want %v", got, want)
		}
	}
	assertCount(0)
	if _, err := m.activatePeer(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	assertCount(1)
	if _, err := m.activatePeer(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	m.SetPeerDown(key, old)
	assertCount(1)
	cleanup := m.DetachPeerConnections(key)
	assertCount(0)
	cleanup()
	if _, err := m.activatePeer(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	m.SetPeerDown(key, next)
	assertCount(1)
	if err := m.QuiescePeer(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	assertCount(0)
	m.SetPeerDown(key, old)
	assertCount(0)
}

func TestDeviceMetricsExcludesActivationReservationAndCountsForceDown(t *testing.T) {
	store := &blockingGetStore{Store: kv.NewMemory(nil), entered: make(chan struct{}), release: make(chan struct{})}
	unblock := sync.OnceFunc(func() { close(store.release) })
	t.Cleanup(unblock)
	m := NewManager(&peer.Server{Store: store})
	key := giznet.PublicKey{2}
	conn := &testGiznetConn{publicKey: key}
	done := make(chan error, 1)
	go func() { _, err := m.activatePeer(context.Background(), conn); done <- err }()
	<-store.entered
	if got := m.deviceCount.Load(); got != 0 {
		t.Fatalf("pending activation count = %d", got)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := m.deviceCount.Load(); got != 1 {
		t.Fatalf("activated count = %d", got)
	}
	var group sync.WaitGroup
	group.Go(func() { m.SetPeerDown(key, conn) })
	group.Go(func() { m.ForcePeerDown(key) })
	group.Wait()
	if got := m.deviceCount.Load(); got != 0 {
		t.Fatalf("concurrent disconnect count = %d", got)
	}
}

func TestDeviceMetricsRestoresCountAfterRejectedSelfDeletion(t *testing.T) {
	store := kv.NewMemory(nil)
	m := NewManager(&peer.Server{Store: store})
	key := giznet.PublicKey{9}
	conn := &testGiznetConn{publicKey: key}
	if _, err := m.activatePeer(t.Context(), conn); err != nil {
		t.Fatal(err)
	}
	m.Peers.Store = &failingGetStore{Store: store, err: errors.New("storage unavailable")}
	if err := m.deleteActivePeer(t.Context(), key, conn, nil); err == nil {
		t.Fatal("expected self-deletion failure")
	}
	if got := m.deviceCount.Load(); got != 1 {
		t.Fatalf("failed deletion lost online device: %d", got)
	}
	m.SetPeerDown(key, conn)
	if got := m.deviceCount.Load(); got != 0 {
		t.Fatalf("disconnect after deletion rollback = %d", got)
	}
}

func TestDeviceMetricsExcludesAdminAndEdgePeers(t *testing.T) {
	m := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	for i, role := range []apitypes.PeerRole{apitypes.PeerRoleAdmin, apitypes.PeerRoleEdgeNode} {
		key := giznet.PublicKey{byte(i + 3)}
		if _, err := m.Peers.EnsureConnectedPeer(t.Context(), key); err != nil {
			t.Fatal(err)
		}
		record, err := m.Peers.LoadPeer(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		record.Role = role
		if _, err := m.Peers.SavePeer(t.Context(), record); err != nil {
			t.Fatal(err)
		}
		conn := &testGiznetConn{publicKey: key}
		if role == apitypes.PeerRoleEdgeNode {
			if err := m.activateEdgeTransport(t.Context(), conn); err != nil {
				t.Fatal(err)
			}
			m.setEdgeTransportDown(key, conn)
		} else {
			if _, err := m.activatePeer(t.Context(), conn); err != nil {
				t.Fatal(err)
			}
			m.SetPeerDown(key, conn)
		}
		if got := m.deviceCount.Load(); got != 0 {
			t.Fatalf("%s counted as device: %d", role, got)
		}
	}
}
