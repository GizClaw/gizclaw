package gizclaw

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func addressTestConn(key giznet.PublicKey, ip string, port int, seen time.Time) *testGiznetConn {
	return &testGiznetConn{publicKey: key, peerInfo: &giznet.PeerInfo{
		Endpoint: &net.TCPAddr{IP: net.ParseIP(ip), Port: port}, LastSeen: seen,
	}}
}

func TestPeerRuntimeAddressSurvivesDisconnectAndManagerRestart(t *testing.T) {
	for _, ip := range []string{"198.51.100.7", "2001:db8::7"} {
		t.Run(ip, func(t *testing.T) {
			manager := &Manager{PeerRun: peerruntest.New(t)}
			key := giznet.PublicKey{1}
			conn := addressTestConn(key, ip, 4242, time.Now().UTC())
			manager.SetPeerUp(key, conn)
			want := conn.peerInfo.Endpoint.String()
			if stored, err := manager.PeerRun.GetLastAddr(t.Context(), key); err != nil || stored != want {
				t.Fatalf("online persisted address = %q, %v, want %q", stored, err, want)
			}
			manager.SetPeerDown(key, conn)
			for _, reader := range []*Manager{manager, {PeerRun: manager.PeerRun}} {
				runtime := reader.PeerRuntime(t.Context(), key)
				if runtime.Online || runtime.LastAddr == nil || *runtime.LastAddr != want {
					t.Fatalf("offline runtime = %+v, want address %q", runtime, want)
				}
			}
		})
	}
}

func TestOldPeerConnectionCannotOverwriteReplacementAddress(t *testing.T) {
	manager := &Manager{PeerRun: peerruntest.New(t)}
	key := giznet.PublicKey{2}
	seen := time.Now().UTC()
	old := addressTestConn(key, "198.51.100.1", 1000, seen.Add(time.Hour))
	current := addressTestConn(key, "2001:db8::2", 2000, seen)
	manager.SetPeerUp(key, old)
	manager.SetPeerUp(key, current)
	manager.SetPeerDown(key, old)
	if got, ok := manager.Peer(key); !ok || got != current {
		t.Fatal("old disconnect removed the replacement")
	}
	manager.ForcePeerDown(key)
	manager.SetPeerDown(key, old)
	runtime := (&Manager{PeerRun: manager.PeerRun}).PeerRuntime(t.Context(), key)
	if runtime.Online || runtime.LastAddr == nil || *runtime.LastAddr != "[2001:db8::2]:2000" {
		t.Fatalf("runtime after late old disconnect = %+v", runtime)
	}
}

func TestPeerAddressWriteFailurePreservesActiveGeneration(t *testing.T) {
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	manager.PeerRun = peerruntest.New(t)
	key := giznet.PublicKey{3}
	old := addressTestConn(key, "198.51.100.1", 1000, time.Now())
	manager.SetPeerUp(key, old)
	if err := manager.PeerRun.DB.Close(); err != nil {
		t.Fatal(err)
	}
	current := addressTestConn(key, "198.51.100.2", 2000, time.Now())
	if _, err := manager.activatePeer(t.Context(), current); err == nil {
		t.Fatal("activation succeeded without persisting its address")
	}
	if got, ok := manager.Peer(key); !ok || got != old {
		t.Fatal("failed address write replaced the active connection")
	}
	if manager.connectionUpdates.ActiveKeys() != 0 {
		t.Fatal("failed address write retained the connection lock")
	}
}

type observedAddressConn struct {
	*testGiznetConn
	once     sync.Once
	observed chan struct{}
}

func (c *observedAddressConn) PeerInfo() *giznet.PeerInfo {
	c.once.Do(func() { close(c.observed) })
	return c.testGiznetConn.PeerInfo()
}

func TestPeerAddressPersistenceSerializesOnlyTheSamePeer(t *testing.T) {
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	manager.PeerRun = peerruntest.New(t)
	key, otherKey := giznet.PublicKey{4}, giznet.PublicKey{5}
	old := addressTestConn(key, "198.51.100.1", 1000, time.Now())
	other := &testGiznetConn{publicKey: otherKey}
	manager.SetPeerUp(key, old)
	manager.SetPeerUp(otherKey, other)

	// Holding the pool's only connection deterministically stalls address SQL.
	blocked, err := manager.PeerRun.DB.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Close()
	first := &observedAddressConn{
		testGiznetConn: addressTestConn(key, "198.51.100.2", 2000, time.Now()),
		observed:       make(chan struct{}),
	}
	firstDone := make(chan error, 1)
	go func() { _, err := manager.activatePeer(t.Context(), first); firstDone <- err }()
	select {
	case <-first.observed:
	case <-time.After(time.Second):
		t.Fatal("replacement did not reach address persistence")
	}
	progress := make(chan bool, 1)
	go func() {
		got, ok := manager.Peer(otherKey)
		progress <- ok && got == other
	}()
	select {
	case ok := <-progress:
		if !ok {
			t.Fatal("unrelated Peer disappeared during address persistence")
		}
	case <-time.After(time.Second):
		t.Fatal("address SQL held the Manager state lock")
	}
	if got, ok := manager.Peer(key); !ok || got != old {
		t.Fatal("replacement published before its address was persisted")
	}
	second := &observedAddressConn{
		testGiznetConn: addressTestConn(key, "2001:db8::3", 3000, time.Now()),
		observed:       make(chan struct{}),
	}
	secondDone := make(chan struct{})
	go func() { manager.SetPeerUp(key, second); close(secondDone) }()
	select {
	case <-second.observed:
		t.Fatal("same-Peer publication bypassed the stalled address write")
	case <-time.After(25 * time.Millisecond):
	}
	if err := blocked.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement did not resume after address persistence")
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("same-Peer publication did not resume")
	}
	manager.SetPeerDown(key, first)
	manager.SetPeerDown(key, second)
	runtime := manager.PeerRuntime(t.Context(), key)
	if runtime.LastAddr == nil || *runtime.LastAddr != "[2001:db8::3]:3000" {
		t.Fatalf("serialized replacement address = %+v", runtime)
	}
	if manager.connectionUpdates.ActiveKeys() != 0 {
		t.Fatal("connection publication retained idle Peer locks")
	}
}

func TestPeerRuntimeAddressUnknownEndpointPreservesLastObservation(t *testing.T) {
	manager := &Manager{PeerRun: peerruntest.New(t)}
	key := giznet.PublicKey{6}
	if runtime := manager.PeerRuntime(t.Context(), key); runtime.LastAddr != nil {
		t.Fatalf("never-seen address = %v", runtime.LastAddr)
	}
	known := addressTestConn(key, "198.51.100.6", 6000, time.Now())
	manager.SetPeerUp(key, known)
	unknown := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, unknown)
	manager.SetPeerDown(key, unknown)
	if runtime := manager.PeerRuntime(t.Context(), key); runtime.LastAddr == nil || *runtime.LastAddr != "198.51.100.6:6000" {
		t.Fatalf("unknown endpoint erased the last observation = %+v", runtime)
	}
}
