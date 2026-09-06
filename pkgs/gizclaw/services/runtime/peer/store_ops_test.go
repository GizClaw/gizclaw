package peer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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

func TestStoreOpsBootstrapEdgeNodesRejectsZeroKey(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	if err := server.BootstrapEdgeNodes(context.Background(), []giznet.PublicKey{{}}); err == nil || !strings.Contains(err.Error(), "empty edge-node public key") {
		t.Fatalf("BootstrapEdgeNodes zero key error = %v", err)
	}
}

func TestStoreOpsLoadPeerMissing(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.LoadPeer(context.Background(), giznet.PublicKey{1})
	if !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("LoadPeer missing err = %v", err)
	}
}

func TestBindFirmwarePersistsReleaseLine(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}
	publicKey := giznet.PublicKey{1}
	if _, err := server.EnsureConnectedPeer(context.Background(), publicKey); err != nil {
		t.Fatal(err)
	}
	bound, err := server.BindFirmware(context.Background(), publicKey, "h106")
	if err != nil {
		t.Fatal(err)
	}
	if bound.FirmwareId == nil || *bound.FirmwareId != "h106" {
		t.Fatalf("BindFirmware() = %#v, want h106", bound)
	}
	loaded, err := server.LoadPeer(context.Background(), publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FirmwareId == nil || *loaded.FirmwareId != "h106" {
		t.Fatalf("LoadPeer() = %#v, want h106", loaded)
	}
	projected := toAdminRegistration(loaded)
	if projected.FirmwareId == nil || *projected.FirmwareId != "h106" {
		t.Fatalf("toAdminRegistration() = %#v, want h106", projected)
	}
	if _, err := server.BindFirmware(context.Background(), publicKey, " "); err == nil {
		t.Fatal("BindFirmware(empty) error = nil")
	}
	if _, err := server.BindFirmware(context.Background(), publicKey, " h106 "); err == nil {
		t.Fatal("BindFirmware(padded id) error = nil")
	}
}

func TestStoreOpsSavePeerRejectsInvalidPeer(t *testing.T) {
	server := &Server{Store: mustBadgerInMemory(t, nil)}

	_, err := server.SavePeer(context.Background(), apitypes.Peer{})
	if err == nil || !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("SavePeer invalid err = %v", err)
	}

}

// Another Server can create or edit the shared Edge record after bootstrap
// reads it. A retry must merge from that record, not overwrite its metadata.
func TestBootstrapEdgeNodesPreservesConcurrentMetadata(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "update"}[exists], func(t *testing.T) {
			base := mustBadgerInMemory(t, nil)
			key := giznet.PublicKey{43}
			other := &Server{Store: base}
			record := apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive}
			if exists {
				if _, err := other.SavePeer(t.Context(), record); err != nil {
					t.Fatal(err)
				}
			}
			store := &bootstrapConflictStore{Store: base}
			store.before = func() {
				sn := "concurrent-sn"
				record.Device.Identifiers = &apitypes.DeviceIdentifiers{Sn: &sn}
				if _, err := other.SavePeer(t.Context(), record); err != nil {
					t.Fatal(err)
				}
			}
			server := &Server{Store: store}
			if err := server.BootstrapEdgeNodes(t.Context(), []giznet.PublicKey{key}); err != nil {
				t.Fatal(err)
			}
			got, err := other.LoadPeer(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			if got.Role != apitypes.PeerRoleEdgeNode || got.Device.Identifiers == nil || got.Device.Identifiers.Sn == nil || *got.Device.Identifiers.Sn != "concurrent-sn" {
				t.Fatalf("bootstrap lost concurrent metadata: %#v", got)
			}
			keys, err := base.ListMembers(t.Context(), snPrefix("concurrent-sn"))
			if err != nil || len(keys) != 1 || keys[0] != key.String() {
				t.Fatalf("identifier index: %v, %v", keys, err)
			}
		})
	}
}

type bootstrapConflictStore struct {
	kv.Store
	before func()
}

func (s *bootstrapConflictStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if before := s.before; before != nil {
		s.before = nil
		before()
	}
	return s.Store.ApplyMutation(ctx, mutation)
}
