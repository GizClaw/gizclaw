package peer

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestStoreOpsHelpers(t *testing.T) {
	server := &Server{}
	if _, err := server.store(); err == nil {
		t.Fatal("store should fail when store is nil")
	}
	if (&Server{}).peerRuntime(context.Background(), giznet.PublicKey{1}).Online {
		t.Fatal("zero peerRuntime should be offline")
	}
	if optionalPeer(apitypes.Peer{PublicKey: giznet.PublicKey{1}.String()}, nil) == nil {
		t.Fatal("optionalPeer should keep value")
	}
	if optionalPeer(apitypes.Peer{}, errors.New("boom")) != nil {
		t.Fatal("optionalPeer should drop error case")
	}
}

func TestStoreOpsEnsureConnectedPeerValidation(t *testing.T) {
	server := &Server{
		Store: mustBadgerInMemory(t, nil),
	}

	_, err := server.EnsureConnectedPeer(context.Background(), giznet.PublicKey{})
	if err == nil || !strings.Contains(err.Error(), "empty public key") {
		t.Fatalf("empty public key err = %v", err)
	}
}

func TestStoreOpsEnsureConnectedPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	ctx := context.Background()
	publicKey := giznet.PublicKey{1}

	connected, err := server.EnsureConnectedPeer(ctx, publicKey)
	if err != nil {
		t.Fatalf("EnsureConnectedPeer error = %v", err)
	}
	if connected.Role != apitypes.PeerRoleClient || connected.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("connected peer = %+v", connected)
	}
	if connected.AutoRegistered == nil || !*connected.AutoRegistered {
		t.Fatalf("connected auto_registered = %+v", connected.AutoRegistered)
	}
	if connected.Configuration.View != nil {
		t.Fatalf("connected view = %v, want nil", connected.Configuration.View)
	}
}

func TestStoreOpsEnsureConnectedPeerUsesDefaultView(t *testing.T) {
	server := &Server{
		Store:           mustBadgerInMemory(t, nil),
		DefaultPeerView: " default-client ",
	}

	connected, err := server.EnsureConnectedPeer(context.Background(), giznet.PublicKey{1})
	if err != nil {
		t.Fatalf("EnsureConnectedPeer error = %v", err)
	}
	if connected.Configuration.View == nil || *connected.Configuration.View != "default-client" {
		t.Fatalf("connected view = %v, want default-client", connected.Configuration.View)
	}
	stored, err := server.LoadPeer(context.Background(), giznet.PublicKey{1})
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if stored.Configuration.View == nil || *stored.Configuration.View != "default-client" {
		t.Fatalf("stored view = %v, want default-client", stored.Configuration.View)
	}
}

func TestStoreOpsEnsureConnectedPeerPreservesExisting(t *testing.T) {
	view := "existing-view"
	tests := []struct {
		name          string
		configuration apitypes.Configuration
	}{
		{name: "nil view", configuration: apitypes.Configuration{}},
		{name: "different view", configuration: apitypes.Configuration{View: &view}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{
				Store:           mustBadgerInMemory(t, nil),
				DefaultPeerView: "default-client",
			}
			ctx := context.Background()
			publicKey := giznet.PublicKey{1}
			saved, err := server.SavePeer(ctx, apitypes.Peer{
				PublicKey:     publicKey.String(),
				Role:          apitypes.PeerRoleAdmin,
				Status:        apitypes.PeerRegistrationStatusBlocked,
				Configuration: tc.configuration,
			})
			if err != nil {
				t.Fatalf("SavePeer error = %v", err)
			}

			got, err := server.EnsureConnectedPeer(ctx, publicKey)
			if err != nil {
				t.Fatalf("EnsureConnectedPeer error = %v", err)
			}
			if !reflect.DeepEqual(got, saved) {
				t.Fatalf("EnsureConnectedPeer changed existing peer: got %+v, want %+v", got, saved)
			}
		})
	}
}

func TestStoreOpsBootstrapEdgeNodesCreatesAndUpdatesPeers(t *testing.T) {
	server := &Server{
		Store:           mustBadgerInMemory(t, nil),
		DefaultPeerView: "default-client",
	}
	ctx := context.Background()
	existingKey := giznet.PublicKey{1}
	newKey := giznet.PublicKey{2}
	createdAt := time.Unix(100, 0).UTC()
	deviceName := "existing"
	if _, err := server.SavePeer(ctx, apitypes.Peer{
		PublicKey: existingKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusBlocked,
		Device: apitypes.DeviceInfo{
			Name: &deviceName,
		},
		Configuration: apitypes.Configuration{},
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	if err := server.BootstrapEdgeNodes(ctx, []giznet.PublicKey{existingKey, newKey}); err != nil {
		t.Fatalf("BootstrapEdgeNodes error = %v", err)
	}

	existing, err := server.LoadPeer(ctx, existingKey)
	if err != nil {
		t.Fatalf("LoadPeer(existing) error = %v", err)
	}
	if existing.Role != apitypes.PeerRoleEdgeNode || existing.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("existing peer role/status = %+v", existing)
	}
	if existing.CreatedAt != createdAt || existing.Device.Name == nil || *existing.Device.Name != deviceName {
		t.Fatalf("existing peer metadata not preserved: %+v", existing)
	}
	if existing.Configuration.View != nil {
		t.Fatalf("existing edge view = %v, want nil", existing.Configuration.View)
	}

	created, err := server.LoadPeer(ctx, newKey)
	if err != nil {
		t.Fatalf("LoadPeer(new) error = %v", err)
	}
	if created.Role != apitypes.PeerRoleEdgeNode || created.Status != apitypes.PeerRegistrationStatusActive || created.PublicKey != newKey.String() {
		t.Fatalf("new edge peer = %+v", created)
	}
	if created.Configuration.View != nil {
		t.Fatalf("new edge view = %v, want nil", created.Configuration.View)
	}
}

func TestStoreOpsBootstrapEdgeNodesRejectsZeroKey(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	if err := server.BootstrapEdgeNodes(context.Background(), []giznet.PublicKey{{}}); err == nil || !strings.Contains(err.Error(), "empty edge-node public key") {
		t.Fatalf("BootstrapEdgeNodes zero key error = %v", err)
	}
}

func TestStoreOpsLoadAndSavePeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{1}
	want := apitypes.Peer{
		PublicKey: publicKey.String(),
		Role:      apitypes.PeerRoleClient,
		Status:    apitypes.PeerRegistrationStatusActive,
		Device: apitypes.DeviceInfo{
			Name: func() *string {
				value := "demo"
				return &value
			}(),
		},
		Configuration: apitypes.Configuration{},
	}

	got, err := server.SavePeer(context.Background(), want)
	if err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}
	if got.PublicKey != want.PublicKey {
		t.Fatalf("SavePeer public key = %q, want %q", got.PublicKey, want.PublicKey)
	}

	loaded, err := server.LoadPeer(context.Background(), publicKey)
	if err != nil {
		t.Fatalf("LoadPeer error = %v", err)
	}
	if loaded.PublicKey != want.PublicKey || loaded.Role != want.Role || loaded.Status != want.Status {
		t.Fatalf("LoadPeer = %+v", loaded)
	}
	if loaded.Device.Name == nil || *loaded.Device.Name != "demo" {
		t.Fatalf("LoadPeer device name = %+v", loaded.Device.Name)
	}
}

func TestStoreOpsRecordMutationsUseIconRecordLock(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Server, giznet.PublicKey) error
	}{
		{
			name: "device info",
			call: func(ctx context.Context, server *Server, publicKey giznet.PublicKey) error {
				_, err := server.putInfo(ctx, publicKey, apitypes.DeviceInfo{})
				return err
			},
		},
		{
			name: "configuration",
			call: func(ctx context.Context, server *Server, publicKey giznet.PublicKey) error {
				_, err := server.putConfig(ctx, publicKey, apitypes.Configuration{})
				return err
			},
		},
		{
			name: "approve",
			call: func(ctx context.Context, server *Server, publicKey giznet.PublicKey) error {
				_, err := server.approve(ctx, publicKey, apitypes.PeerRoleClient)
				return err
			},
		},
		{
			name: "block",
			call: func(ctx context.Context, server *Server, publicKey giznet.PublicKey) error {
				_, err := server.block(ctx, publicKey)
				return err
			},
		},
		{
			name: "save full peer",
			call: func(ctx context.Context, server *Server, publicKey giznet.PublicKey) error {
				peer, err := server.LoadPeer(ctx, publicKey)
				if err != nil {
					return err
				}
				_, err = server.SavePeer(ctx, peer)
				return err
			},
		},
		{
			name: "edge bootstrap",
			call: func(ctx context.Context, server *Server, publicKey giznet.PublicKey) error {
				return server.BootstrapEdgeNodes(ctx, []giznet.PublicKey{publicKey})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{Store: mustBadgerInMemory(t, nil)}
			publicKey := giznet.PublicKey{1}
			saveTestPeer(t, server, publicKey, apitypes.DeviceInfo{})
			unlock := server.IconLocks.LockRecord(publicKey.String())
			done := make(chan error, 1)
			go func() {
				done <- tt.call(context.Background(), server, publicKey)
			}()

			select {
			case err := <-done:
				unlock()
				t.Fatalf("mutation completed without record lock: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			unlock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("mutation error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("mutation did not complete after record lock release")
			}
		})
	}
}

func TestStoreOpsLoadPeerMissing(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.LoadPeer(context.Background(), giznet.PublicKey{1})
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("LoadPeer missing err = %v", err)
	}
}

func TestStoreOpsSavePeerRejectsInvalidPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.SavePeer(context.Background(), apitypes.Peer{})
	if err == nil || !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("SavePeer invalid err = %v", err)
	}

}

func TestStoreOpsExists(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{1}

	if exists, err := server.exists(context.Background(), publicKey); err != nil || exists {
		t.Fatalf("exists(missing) = %v, %v", exists, err)
	}

	if _, err := server.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     publicKey.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer error = %v", err)
	}

	if exists, err := server.exists(context.Background(), publicKey); err != nil || !exists {
		t.Fatalf("exists(peer) = %v, %v", exists, err)
	}
}
