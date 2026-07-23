package gizclaw

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type blockingGetStore struct {
	kv.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type failingGetStore struct {
	kv.Store
	err error
}

func (s *failingGetStore) Get(context.Context, kv.Key) ([]byte, error) {
	return nil, s.err
}

func (s *blockingGetStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.Store.Get(ctx, key)
}

func TestManagerActivatePeerMakesRegistrationReady(t *testing.T) {
	peers := &peer.Server{Store: kv.NewMemory(nil)}
	manager := NewManager(peers)
	key := giznet.PublicKey{7}
	conn := &testGiznetConn{publicKey: key}
	oldConn, err := manager.activatePeer(context.Background(), conn)
	if err != nil {
		t.Fatalf("activatePeer: %v", err)
	}
	if oldConn != nil {
		t.Fatalf("activatePeer oldConn = %v, want nil", oldConn)
	}
	if _, err := peers.LoadPeer(context.Background(), key); err != nil {
		t.Fatalf("LoadPeer after activation: %v", err)
	}
	registration := runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Name: "profile-early"}}
	if !manager.SetPeerRegistration(key, conn, registration) {
		t.Fatal("registration immediately after activation was rejected")
	}
}

func TestManagerActivatePeerDoesNotBlockUnrelatedPeer(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	manager := NewManager(peers)
	otherKey := giznet.PublicKey{7, 1}
	otherConn := &testGiznetConn{publicKey: otherKey}
	manager.SetPeerUp(otherKey, otherConn)
	if !manager.SetPeerRegistration(otherKey, otherConn, runtimeprofile.Registration{}) {
		t.Fatal("SetPeerRegistration(other) rejected active connection")
	}
	targetKey := giznet.PublicKey{7, 2}
	if _, err := peers.EnsureConnectedPeer(context.Background(), targetKey); err != nil {
		t.Fatalf("EnsureConnectedPeer(target): %v", err)
	}
	oldTargetConn := &testGiznetConn{publicKey: targetKey}
	manager.SetPeerUp(targetKey, oldTargetConn)
	if !manager.SetPeerRegistration(targetKey, oldTargetConn, runtimeprofile.Registration{}) {
		t.Fatal("SetPeerRegistration(target) rejected active connection")
	}
	blockingStore := &blockingGetStore{
		Store:   store,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	peers.Store = blockingStore
	targetConn := &testGiznetConn{publicKey: targetKey}
	activation := make(chan error, 1)
	go func() {
		_, err := manager.activatePeer(context.Background(), targetConn)
		activation <- err
	}()
	<-blockingStore.entered
	if got, ok := manager.Peer(targetKey); !ok || got != oldTargetConn {
		t.Fatalf("target during replacement activation = %v, %v, want old connection", got, ok)
	}
	if _, ok := manager.PeerRegistration(targetKey); !ok {
		t.Fatal("replacement activation hid the active generation registration")
	}

	unrelatedReady := make(chan bool, 1)
	go func() {
		got, ok := manager.Peer(otherKey)
		unrelatedReady <- ok && got == otherConn && manager.SetPeerRegistration(otherKey, otherConn, runtimeprofile.Registration{})
	}()
	select {
	case ready := <-unrelatedReady:
		if !ready {
			t.Fatal("unrelated Peer was not available during activation")
		}
	case <-time.After(time.Second):
		t.Fatal("slow activation blocked unrelated Manager operations")
	}
	if _, err := manager.activatePeer(context.Background(), &testGiznetConn{publicKey: targetKey}); !errors.Is(err, errPeerConnActivating) {
		t.Fatalf("concurrent activatePeer error = %v, want %v", err, errPeerConnActivating)
	}
	close(blockingStore.release)
	if err := <-activation; err != nil {
		t.Fatalf("activatePeer(target): %v", err)
	}
	if got, ok := manager.Peer(targetKey); !ok || got != targetConn {
		t.Fatalf("activated target = %v, %v", got, ok)
	}
}

func TestManagerPeerRuntimeStaysOfflineUntilFirstActivationPublishes(t *testing.T) {
	store := kv.NewMemory(nil)
	blockingStore := &blockingGetStore{
		Store:   store,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(&peer.Server{Store: blockingStore})
	key := giznet.PublicKey{7, 3}
	conn := &testGiznetConn{publicKey: key}
	activation := make(chan error, 1)
	go func() {
		_, err := manager.activatePeer(context.Background(), conn)
		activation <- err
	}()
	<-blockingStore.entered
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online {
		t.Fatalf("runtime during first activation = %+v, want offline", runtime)
	}
	close(blockingStore.release)
	if err := <-activation; err != nil {
		t.Fatalf("activatePeer: %v", err)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online {
		t.Fatalf("runtime after activation = %+v, want online", runtime)
	}
}

func TestManagerForcePeerDownPreservesReplacementActivation(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	manager := NewManager(peers)
	key := giznet.PublicKey{7, 5}
	if _, err := peers.EnsureConnectedPeer(context.Background(), key); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	oldConn := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, oldConn)
	if !manager.SetPeerRegistration(key, oldConn, runtimeprofile.Registration{}) {
		t.Fatal("SetPeerRegistration rejected active connection")
	}
	blockingStore := &blockingGetStore{
		Store:   store,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	peers.Store = blockingStore
	newConn := &testGiznetConn{publicKey: key}
	activation := make(chan error, 1)
	go func() {
		_, err := manager.activatePeer(context.Background(), newConn)
		activation <- err
	}()
	<-blockingStore.entered

	manager.ForcePeerDown(key)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("forced-down generation remained online")
	}
	close(blockingStore.release)
	if err := <-activation; err != nil {
		t.Fatalf("replacement activatePeer: %v", err)
	}
	if got, ok := manager.Peer(key); !ok || got != newConn {
		t.Fatalf("replacement after force down = %v, %v, want new connection", got, ok)
	}
}

func TestManagerPeerDownPreservesDeletingReservation(t *testing.T) {
	for _, test := range []struct {
		name string
		down func(*Manager, giznet.PublicKey, giznet.Conn)
	}{
		{
			name: "connection down",
			down: func(manager *Manager, key giznet.PublicKey, conn giznet.Conn) {
				manager.SetPeerDown(key, conn)
			},
		},
		{
			name: "forced down",
			down: func(manager *Manager, key giznet.PublicKey, _ giznet.Conn) {
				manager.ForcePeerDown(key)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := kv.NewMemory(nil)
			peers := &peer.Server{Store: store}
			key := giznet.PublicKey{7, 6}
			if _, err := peers.EnsureConnectedPeer(context.Background(), key); err != nil {
				t.Fatalf("EnsureConnectedPeer: %v", err)
			}
			blockingStore := &blockingBatchMutateStore{
				Store:   store,
				entered: make(chan struct{}),
				release: make(chan struct{}),
			}
			peers.Store = blockingStore
			manager := NewManager(peers)
			conn := &testGiznetConn{publicKey: key}
			manager.SetPeerUp(key, conn)
			deleteErr := make(chan error, 1)
			go func() {
				deleteErr <- manager.deleteActivePeer(context.Background(), key, conn, nil)
			}()
			<-blockingStore.entered

			test.down(manager, key, conn)
			if _, ok := manager.Peer(key); ok {
				t.Fatal("disconnected deleting Peer remained online")
			}
			if _, err := manager.activatePeer(context.Background(), &testGiznetConn{publicKey: key}); !errors.Is(err, ErrPeerConnRetiring) {
				t.Fatalf("activatePeer during disconnected delete = %v, want %v", err, ErrPeerConnRetiring)
			}

			close(blockingStore.release)
			if err := <-deleteErr; err != nil {
				t.Fatalf("deleteActivePeer: %v", err)
			}
		})
	}
}

func TestManagerDeletingReservationRejectsBlockedReplacementEnsure(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	key := giznet.PublicKey{7, 7}
	if _, err := peers.EnsureConnectedPeer(context.Background(), key); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	blockingStore := &blockingBatchMutateStore{
		Store:   store,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	peers.Store = blockingStore
	manager := NewManager(peers)
	oldConn := &testGiznetConn{publicKey: key}
	newConn := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, oldConn)
	manager.mu.Lock()
	state := manager.peers[key]
	state.activating = newConn
	manager.mu.Unlock()

	deleteErr := make(chan error, 1)
	go func() {
		deleteErr <- manager.deleteActivePeer(context.Background(), key, oldConn, nil)
	}()
	<-blockingStore.entered
	ensureErr := make(chan error, 1)
	go func() {
		ensureErr <- manager.ensureActivatingPeer(context.Background(), key, state, newConn)
	}()
	select {
	case err := <-ensureErr:
		t.Fatalf("replacement ensure returned before delete released record lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(blockingStore.release)
	if err := <-deleteErr; err != nil {
		t.Fatalf("deleteActivePeer: %v", err)
	}
	if err := <-ensureErr; !errors.Is(err, ErrPeerConnRetiring) && !errors.Is(err, ErrPeerConnNotActive) {
		t.Fatalf("blocked replacement ensure error = %v, want retiring or inactive", err)
	}
	if _, err := peers.LoadPeer(context.Background(), key); !errors.Is(err, peer.ErrPeerNotFound) {
		t.Fatalf("LoadPeer after delete and rejected replacement = %v, want %v", err, peer.ErrPeerNotFound)
	}
	if pending, err := pendingdeletion.HasLocator(context.Background(), store, pendingdeletion.KindPeer, key.String()); err != nil || !pending {
		t.Fatalf("pending deletion after rejected replacement = %v, error = %v", pending, err)
	}
}

func TestManagerActivatePeerRollsBackReservationOnEnsureFailure(t *testing.T) {
	store := kv.NewMemory(nil)
	peers := &peer.Server{Store: store}
	manager := NewManager(peers)
	key := giznet.PublicKey{7, 4}
	if _, err := peers.EnsureConnectedPeer(context.Background(), key); err != nil {
		t.Fatalf("EnsureConnectedPeer: %v", err)
	}
	oldConn := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, oldConn)
	registration := runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Name: "profile"}}
	if !manager.SetPeerRegistration(key, oldConn, registration) {
		t.Fatal("SetPeerRegistration rejected active connection")
	}
	ensureErr := errors.New("test ensure failure")
	peers.Store = &failingGetStore{Store: store, err: ensureErr}
	if _, err := manager.activatePeer(context.Background(), &testGiznetConn{publicKey: key}); !errors.Is(err, ensureErr) {
		t.Fatalf("activatePeer error = %v, want %v", err, ensureErr)
	}
	if got, ok := manager.Peer(key); !ok || got != oldConn {
		t.Fatalf("active connection after failed replacement = %v, %v", got, ok)
	}
	if got, ok := manager.PeerRegistration(key); !ok || got.RuntimeProfile.Name != "profile" {
		t.Fatalf("registration after failed replacement = %#v, %v", got, ok)
	}
	peers.Store = store
	if _, err := manager.activatePeer(context.Background(), &testGiznetConn{publicKey: key}); err != nil {
		t.Fatalf("activatePeer after rollback: %v", err)
	}
}

func TestManagerSetPeerDownDeletesMatchingPeer(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	if oldConn := manager.SetPeerUp(key, conn); oldConn != nil {
		t.Fatalf("SetPeerUp first oldConn = %v, want nil", oldConn)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online {
		t.Fatalf("peer should be online before removal: %+v", runtime)
	}

	manager.SetPeerDown(key, conn)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("peer should be removed")
	}
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after removal = %+v", runtime)
	}
}

func TestManagerSetPeerUpReplacesConnection(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	oldConn := &testGiznetConn{}
	newConn := &testGiznetConn{}

	if replaced := manager.SetPeerUp(key, oldConn); replaced != nil {
		t.Fatalf("first SetPeerUp replaced = %v, want nil", replaced)
	}
	if replaced := manager.SetPeerUp(key, newConn); replaced != oldConn {
		t.Fatalf("second SetPeerUp replaced = %v, want old conn", replaced)
	}

	got, ok := manager.Peer(key)
	if !ok || got != newConn {
		t.Fatalf("ActivePeer after replacement = %v, %v", got, ok)
	}
	manager.SetPeerDown(key, oldConn)
	got, ok = manager.Peer(key)
	if !ok || got != newConn {
		t.Fatalf("stale SetPeerDown removed active peer: %v, %v", got, ok)
	}
	manager.SetPeerDown(key, newConn)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("matching SetPeerDown should remove active peer")
	}
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after matching down = %+v", runtime)
	}
}

func TestManagerSetPeerUpSameConnectionDoesNotReplace(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	if replaced := manager.SetPeerUp(key, conn); replaced != nil {
		t.Fatalf("first SetPeerUp replaced = %v, want nil", replaced)
	}
	if replaced := manager.SetPeerUp(key, conn); replaced != nil {
		t.Fatalf("same-conn SetPeerUp replaced = %v, want nil", replaced)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after same-conn replacement = %+v", runtime)
	}
}

func TestManagerPeerRegistrationFollowsActiveConnection(t *testing.T) {
	profiles, _ := registrationServerAndToken(t, "profile-old")
	manager := &Manager{RuntimeProfiles: profiles}
	key := giznet.PublicKey{1}
	oldConn := &testGiznetConn{}
	newConn := &testGiznetConn{}
	oldRegistration := runtimeprofile.Registration{
		RuntimeProfile: apitypes.RuntimeProfile{
			Name: "profile-old",
		},
	}

	manager.SetPeerUp(key, oldConn)
	if !manager.SetPeerRegistration(key, oldConn, oldRegistration) {
		t.Fatal("SetPeerRegistration() rejected active connection")
	}
	if err := profiles.BindOwnerProfile(t.Context(), key.String(), oldRegistration.RuntimeProfile.Name); err != nil {
		t.Fatalf("BindOwnerProfile(old) error = %v", err)
	}
	if profile, err := manager.runtimeProfileForOwner(t.Context(), key.String()); err != nil || profile.Name != "profile-old" {
		t.Fatalf("runtimeProfileForOwner() = %#v, %v", profile, err)
	}
	resources := (&PeerService{manager: manager}).peerResources(key)
	if profile := resources.RuntimeProfile(); profile == nil || profile.Name != "profile-old" {
		t.Fatalf("active RuntimeProfile = %#v", profile)
	}

	manager.SetPeerUp(key, newConn)
	if _, ok := manager.PeerRegistration(key); ok {
		t.Fatal("replacement connection inherited stale registration")
	}
	if manager.SetPeerRegistration(key, oldConn, oldRegistration) {
		t.Fatal("SetPeerRegistration() accepted stale connection")
	}
	newRegistration := runtimeprofile.Registration{
		RuntimeProfile: apitypes.RuntimeProfile{
			Name: "profile-new",
		},
	}
	createRegistrationToken(t, profiles, "profile-new")
	if !manager.SetPeerRegistration(key, newConn, newRegistration) {
		t.Fatal("SetPeerRegistration() rejected replacement connection")
	}
	if err := profiles.BindOwnerProfile(t.Context(), key.String(), newRegistration.RuntimeProfile.Name); err != nil {
		t.Fatalf("BindOwnerProfile(new) error = %v", err)
	}
	manager.SetPeerDown(key, oldConn)
	if registration, ok := manager.PeerRegistration(key); !ok || registration.RuntimeProfile.Name != "profile-new" {
		t.Fatalf("stale disconnect changed registration = %#v, %v", registration, ok)
	}
	manager.SetPeerDown(key, newConn)
	if _, ok := manager.PeerRegistration(key); ok {
		t.Fatal("disconnected peer retained registration")
	}
	if profile, err := manager.runtimeProfileForOwner(t.Context(), key.String()); err != nil || profile.Name != "profile-new" {
		t.Fatalf("runtimeProfileForOwner(disconnected) = %#v, %v", profile, err)
	}
	if manager.SetPeerRegistration(key, newConn, newRegistration) {
		t.Fatal("SetPeerRegistration recreated a disconnected peer")
	}
	if _, ok := manager.Peer(key); ok {
		t.Fatal("registration recreated a disconnected peer entry")
	}
}

func TestPeerConnRetireSerializesConcurrentRegistration(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{8}
	conn := &testGiznetConn{publicKey: key}
	manager.SetPeerUp(key, conn)
	peerConn := &PeerConn{Conn: conn, Service: &PeerService{manager: manager}}
	registration := runtimeprofile.Registration{RuntimeProfile: apitypes.RuntimeProfile{Name: "profile-race"}}
	registrationEntered := make(chan struct{})
	releaseRegistration := make(chan struct{})
	registrationDone := make(chan bool, 1)
	go func() {
		registrationDone <- manager.setPeerRegistrationIfActive(key, conn, registration, func() bool {
			close(registrationEntered)
			<-releaseRegistration
			peerConn.registration.Store(&registration)
			return !peerConn.isRetiring()
		})
	}()
	<-registrationEntered
	retireDone := make(chan struct{})
	go func() {
		peerConn.retire()
		close(retireDone)
	}()
	select {
	case <-retireDone:
		t.Fatal("retire crossed the registration Manager critical section")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseRegistration)
	if accepted := <-registrationDone; !accepted {
		t.Fatal("active registration was unexpectedly rejected before retire")
	}
	<-retireDone
	if !peerConn.isRetiring() {
		t.Fatal("PeerConn was not marked retiring")
	}
	if peerConn.registration.Load() != nil {
		t.Fatal("retiring PeerConn retained local registration")
	}
	if _, ok := manager.Peer(key); ok {
		t.Fatal("concurrent registration restored retiring PeerConn in Manager")
	}
}

func TestPeerResourcesForHTTPSessionDoesNotInheritActiveConnectionRegistration(t *testing.T) {
	profiles, _ := registrationServerAndToken(t, "profile-connection")
	manager := &Manager{RuntimeProfiles: profiles}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}
	activeRegistration := runtimeprofile.Registration{
		RuntimeProfile: apitypes.RuntimeProfile{
			Name: "profile-connection",
		},
	}
	manager.SetPeerUp(key, conn)
	if !manager.SetPeerRegistration(key, conn, activeRegistration) {
		t.Fatal("SetPeerRegistration() rejected active connection")
	}
	service := &PeerService{manager: manager}

	unregistered := service.peerResourcesForHTTPSession(key, nil)
	if profile := unregistered.RuntimeProfile(); profile != nil {
		t.Fatalf("unregistered HTTP session inherited RuntimeProfile = %#v", profile)
	}

	sessionRegistration := runtimeprofile.Registration{
		RuntimeProfile: apitypes.RuntimeProfile{
			Name: "profile-session",
		},
	}
	createRegistrationToken(t, profiles, "profile-session")
	registered := service.peerResourcesForHTTPSession(key, &sessionRegistration)
	if profile := registered.RuntimeProfile(); profile == nil || profile.Name != "profile-session" {
		t.Fatalf("registered HTTP RuntimeProfile = %#v", profile)
	}
}

func TestManagerSetPeerUpAndDownUpdatesRuntime(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	manager.SetPeerUp(key, conn)
	if got, ok := manager.Peer(key); !ok || got != conn {
		t.Fatalf("active peer after set = %v, %v", got, ok)
	}
	if runtime := manager.PeerRuntime(context.Background(), key); !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after set = %+v, want online with no peer info", runtime)
	}

	manager.SetPeerDown(key, conn)
	if runtime := manager.PeerRuntime(context.Background(), key); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after remove = %+v", runtime)
	}
}

func TestManagerForcePeerDownRemovesActivePeer(t *testing.T) {
	manager := &Manager{}
	key := giznet.PublicKey{1}
	conn := &testGiznetConn{}

	manager.SetPeerUp(key, conn)
	manager.ForcePeerDown(key)
	if _, ok := manager.Peer(key); ok {
		t.Fatal("ForcePeerDown should remove active peer")
	}
}

func TestManagerEnsurePeerCreatesDefaultPeer(t *testing.T) {
	service := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	manager := NewManager(service)
	ctx := context.Background()
	key := giznet.PublicKey{1}

	created, err := manager.EnsurePeer(ctx, key)
	if err != nil {
		t.Fatalf("EnsurePeer error = %v", err)
	}
	if created.PublicKey != key.String() {
		t.Fatalf("PublicKey = %q, want %q", created.PublicKey, key.String())
	}
	if created.Role != apitypes.PeerRoleClient {
		t.Fatalf("Role = %q, want client", created.Role)
	}
	if created.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("Status = %q, want active", created.Status)
	}
	if created.AutoRegistered == nil || !*created.AutoRegistered {
		t.Fatalf("AutoRegistered = %v, want true", created.AutoRegistered)
	}

	loaded, err := service.LoadPeer(ctx, key)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if loaded.Role != apitypes.PeerRoleClient || loaded.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("loaded peer = %+v", loaded)
	}
}

func TestApplyPeerRefreshIdentifiersSkipsUnchangedCollections(t *testing.T) {
	name := "primary"
	sn := "sn-1"
	peer := apitypes.Peer{
		Device: apitypes.DeviceInfo{
			Identifiers: &apitypes.DeviceIdentifiers{
				Sn: &sn,
				Imeis: &[]apitypes.PeerIMEI{{
					Name:   &name,
					Tac:    "12345678",
					Serial: "0000001",
				}},
				Labels: &[]apitypes.PeerLabel{{
					Key:   "batch",
					Value: "cn-east",
				}},
			},
		},
	}
	identifiers := apitypes.DeviceIdentifiers{
		Sn: &sn,
		Imeis: &[]apitypes.PeerIMEI{{
			Name:   &name,
			Tac:    "12345678",
			Serial: "0000001",
		}},
		Labels: &[]apitypes.PeerLabel{{
			Key:   "batch",
			Value: "cn-east",
		}},
	}

	var updatedFields []string
	applyPeerRefreshIdentifiers(&peer, identifiers, &updatedFields)

	if len(updatedFields) != 0 {
		t.Fatalf("applyPeerRefreshIdentifiers() updatedFields = %v, want none", updatedFields)
	}
}

func TestApplyPeerRefreshIdentifiersUpdatesChangedCollections(t *testing.T) {
	name := "primary"
	nextName := "secondary"
	peer := apitypes.Peer{
		Device: apitypes.DeviceInfo{
			Identifiers: &apitypes.DeviceIdentifiers{
				Imeis: &[]apitypes.PeerIMEI{{
					Name:   &name,
					Tac:    "12345678",
					Serial: "0000001",
				}},
				Labels: &[]apitypes.PeerLabel{{
					Key:   "batch",
					Value: "cn-east",
				}},
			},
		},
	}
	identifiers := apitypes.DeviceIdentifiers{
		Imeis: &[]apitypes.PeerIMEI{{
			Name:   &nextName,
			Tac:    "87654321",
			Serial: "0000009",
		}},
		Labels: &[]apitypes.PeerLabel{{
			Key:   "batch",
			Value: "cn-west",
		}},
	}

	var updatedFields []string
	applyPeerRefreshIdentifiers(&peer, identifiers, &updatedFields)

	if len(updatedFields) != 2 {
		t.Fatalf("applyPeerRefreshIdentifiers() updatedFields = %v, want 2 entries", updatedFields)
	}
	if peer.Device.Identifiers == nil || peer.Device.Identifiers.Imeis == nil || (*peer.Device.Identifiers.Imeis)[0].Tac != "87654321" {
		t.Fatalf("IMEIs not updated: %+v", peer.Device.Identifiers)
	}
	if peer.Device.Identifiers.Labels == nil || (*peer.Device.Identifiers.Labels)[0].Value != "cn-west" {
		t.Fatalf("labels not updated: %+v", peer.Device.Identifiers)
	}
}

func TestIsPeerDisconnectedError(t *testing.T) {
	t.Run("closed connection errors are offline", func(t *testing.T) {
		if !isPeerDisconnectedError(errors.New("gizhttp: read response: giznet: conn closed")) {
			t.Fatal("conn closed error should be treated as disconnected")
		}
	})

	t.Run("generic read response errors stay online", func(t *testing.T) {
		if isPeerDisconnectedError(errors.New("gizhttp: read response: malformed HTTP response")) {
			t.Fatal("generic read response error should not be treated as disconnected")
		}
	})
}
