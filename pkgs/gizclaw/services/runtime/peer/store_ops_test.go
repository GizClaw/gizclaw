package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
)

func TestPeerDeletionAdoptsLegacyRequestWithoutChangingMarkerPlanOrLease(t *testing.T) {
	for _, scenario := range []string{"no_plan", "current_plan", "projection_plan", "unsupported_plan", "legacy_children", "replacement_plan", "invalid_plan", "conflicting_snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, key := t.Context(), giznet.PublicKey{103}
			store := kv.NewMemory(nil)
			server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
			saveTestPeer(t, server, key, apitypes.DeviceInfo{})
			raw, err := store.Get(ctx, peerKey(key.String()))
			if err != nil {
				t.Fatal(err)
			}
			var formatted bytes.Buffer
			if err := json.Indent(&formatted, raw, "", "  "); err != nil {
				t.Fatal(err)
			}
			raw = formatted.Bytes()
			if err := store.Set(ctx, peerKey(key.String()), raw); err != nil {
				t.Fatal(err)
			}
			peer, err := decodePeer(raw)
			if err != nil {
				t.Fatal(err)
			}
			record, err := pendingdeletion.New(pendingdeletion.KindPeer, key.String(), new(key.String()), pendingdeletion.ReasonPeerDelete, peerDeletionDescriptor{PublicKey: key.String()}, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if _, created, err := pendingdeletion.CreateOrGet(ctx, store, record); err != nil || !created {
				t.Fatalf("legacy request = %v, %v", created, err)
			}
			fingerprint, err := pendingdeletion.Fingerprint(record)
			if err != nil {
				t.Fatal(err)
			}
			source := PendingDeletionSource(store)
			now := record.DeletedAt.Add(time.Second)
			claim := claimPeerDeletion(t, source, now)
			if err := source.Fail(ctx, claim, "store_error", "prior attempt", false, now.Add(time.Hour), now, 32); err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Hour)
			claim = claimPeerDeletion(t, source, now)
			before, err := source.GetTask(ctx, record.DeletionID)
			if err != nil {
				t.Fatal(err)
			}
			var planData []byte
			wantSuccess := true
			if scenario != "no_plan" && scenario != "conflicting_snapshot" {
				plan := retirementPlan{Version: retirementPlanVersion, HomeServer: server.ServerPublicKey.String(), PeerFingerprint: peerFingerprint(raw), MarkerFingerprint: fingerprint, Peer: peer, PeerRecord: raw}
				plan.Social.PublicKey, plan.Workspaces.PublicKey = key.String(), key.String()
				switch scenario {
				case "projection_plan":
					plan.Version, plan.PeerRecord = 2, nil
					plan.Peer.FirmwareId = new("projected-firmware")
				case "unsupported_plan":
					plan.Version, wantSuccess = 1, false
				case "legacy_children":
					plan.Version, plan.PeerRecord, plan.WorkspaceIDs, wantSuccess = 2, nil, []string{"unproven-child"}, false
				case "replacement_plan":
					plan.Peer.UpdatedAt = plan.Peer.UpdatedAt.Add(time.Second)
					wantSuccess = false
				case "invalid_plan":
					wantSuccess = false
				}
				planData, err = json.MarshalIndent(plan, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "invalid_plan" {
					planData = []byte(`{"version":`)
				}
				if err := store.Set(ctx, peerRetirementPlanKey(record.DeletionID), planData); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "conflicting_snapshot" {
				wantSuccess = false
				if err := store.Set(ctx, peerDeletionSnapshotKey(record.DeletionID), []byte(`{"marker_fingerprint":"wrong","peer_record":""}`)); err != nil {
					t.Fatal(err)
				}
			}
			err = server.DeleteSelf(ctx, key)
			if (err == nil) != wantSuccess {
				t.Fatalf("legacy admission = %v, success wanted %v", err, wantSuccess)
			}
			after, err := source.GetTask(ctx, record.DeletionID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("admission changed marker, counters or active lease: before=%+v after=%+v error=%v", before, after, err)
			}
			if len(planData) != 0 {
				stored, err := store.Get(ctx, peerRetirementPlanKey(record.DeletionID))
				if err != nil || !bytes.Equal(stored, planData) {
					t.Fatalf("admission rewrote plan evidence: %q, %v", stored, err)
				}
			}
			stored, err := store.Get(ctx, peerKey(key.String()))
			if err != nil {
				t.Fatal(err)
			}
			if !wantSuccess {
				if !bytes.Equal(stored, raw) {
					t.Fatal("unsafe adoption replaced the original Peer")
				}
				return
			}
			if !isPeerTombstone(stored) {
				t.Fatalf("legacy admission retained active Peer: %s", stored)
			}
			data, err := store.Get(ctx, peerDeletionSnapshotKey(record.DeletionID))
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := validatePeerDeletionSnapshot(data, record)
			if err != nil || !bytes.Equal(snapshot, raw) {
				t.Fatalf("original exact Peer bytes lost: %q, %v", snapshot, err)
			}
			base := &peerDeletionAdapters{publicKey: key.String()}
			handler := DeletionHandler{Server: server, Source: source, Assignments: testDeletionAssignments{}, Social: base, Workspaces: base, APIKeys: base, RuntimeProfiles: base, Memory: base, Quiescer: base, WorkspaceLookup: emptyPeerLookup{}, FriendGroupLookup: emptyPeerLookup{}, Now: func() time.Time { return now }}
			if err := handler.CheckTask(ctx, claim.Task); err != nil {
				t.Fatalf("cleanup cannot read adopted snapshot: %v", err)
			}
			if scenario != "no_plan" {
				return
			}
			finished := false
			for range 32 {
				err := handler.Handle(ctx, claim)
				if err == nil {
					finished = true
					break
				}
				var outcome *pendingdeletion.OutcomeError
				if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred || outcome.After != time.Hour {
					t.Fatal(err)
				}
				if err := source.Defer(ctx, claim, outcome.Code, outcome.Message, now.Add(time.Hour), now); err != nil {
					t.Fatal(err)
				}
				now = now.Add(time.Hour)
				claim = claimPeerDeletion(t, source, now)
			}
			if !finished {
				t.Fatal("legacy cleanup did not finish after 32 actual attempts")
			}
			if _, err := store.Get(ctx, peerDeletionSnapshotKey(record.DeletionID)); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("guarded finalization retained adoption metadata: %v", err)
			}
		})
	}
}

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

func TestCheckActiveRoleReadsLiveAuthorityWithoutFirmwareProjection(t *testing.T) {
	server := &Server{Store: kv.NewMemory(nil)}
	key := giznet.PublicKey{61}
	for _, status := range []apitypes.PeerRegistrationStatus{apitypes.PeerRegistrationStatusActive, apitypes.PeerRegistrationStatusBlocked, apitypes.PeerRegistrationStatusActive} {
		server.RegistrationFirmware = nil
		if _, err := server.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleEdgeNode, Status: status}); err != nil {
			t.Fatal(err)
		}
		server.RegistrationFirmware = func(context.Context, string) (*string, error) {
			t.Error("role authority queried firmware projection")
			return nil, errors.New("projection unavailable")
		}
		allowed, err := server.CheckActiveRole(t.Context(), key, apitypes.PeerRoleEdgeNode)
		if err != nil || allowed != (status == apitypes.PeerRegistrationStatusActive) {
			t.Fatalf("status %s: allowed=%v error=%v", status, allowed, err)
		}
		if allowed, err := server.CheckActiveRole(t.Context(), key, apitypes.PeerRoleAdmin); err != nil || allowed {
			t.Fatalf("mismatched role: allowed=%v error=%v", allowed, err)
		}
	}
}

func TestCheckActiveRoleRejectsMissingDeletionAndCorruptAuthority(t *testing.T) {
	key := giznet.PublicKey{62}
	for _, state := range []string{"missing", "pending", "deleted", "corrupt"} {
		t.Run(state, func(t *testing.T) {
			store := kv.NewMemory(nil)
			server := &Server{Store: store}
			if state != "missing" {
				if _, err := server.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleEdgeNode, Status: apitypes.PeerRegistrationStatusActive}); err != nil {
					t.Fatal(err)
				}
			}
			switch state {
			case "pending":
				if err := server.DeleteSelf(t.Context(), key); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := store.Set(t.Context(), peerKey(key.String()), encodedPeerTombstone); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := store.Set(t.Context(), peerKey(key.String()), []byte("invalid JSON")); err != nil {
					t.Fatal(err)
				}
			}
			if allowed, err := server.CheckActiveRole(t.Context(), key, apitypes.PeerRoleEdgeNode); allowed || err == nil {
				t.Fatalf("%s authority: allowed=%v error=%v", state, allowed, err)
			}
		})
	}
	if allowed, err := (&Server{}).CheckActiveRole(t.Context(), key, apitypes.PeerRoleEdgeNode); allowed || err == nil {
		t.Fatalf("missing store: allowed=%v error=%v", allowed, err)
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

func TestEnsureConnectedPeerRejectsBlockedAndPreservesRecord(t *testing.T) {
	s := &Server{Store: kv.NewMemory(nil)}
	key := giznet.PublicKey{81}
	if _, err := s.EnsureConnectedPeer(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	_, err := s.block(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := s.EnsureConnectedPeer(t.Context(), key); !errors.Is(err, ErrPeerBlocked) {
			t.Fatalf("blocked activation error = %v", err)
		}
	}
	after, err := s.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("blocked durable record changed: %v", err)
	}
	if _, err := s.approve(t.Context(), key, apitypes.PeerRoleClient); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureConnectedPeer(t.Context(), key); err != nil {
		t.Fatalf("approved activation: %v", err)
	}
}

func TestEnsureConnectedPeerRejectsConcurrentBlockedCreate(t *testing.T) {
	base := kv.NewMemory(nil)
	other := &Server{Store: base}
	key := giznet.PublicKey{82}
	store := &bootstrapConflictStore{Store: base, before: func() {
		if _, err := other.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusBlocked}); err != nil {
			t.Fatal(err)
		}
	}}
	s := &Server{Store: store}
	if _, err := s.EnsureConnectedPeer(t.Context(), key); !errors.Is(err, ErrPeerBlocked) {
		t.Fatalf("concurrent blocked create: %v", err)
	}
}

// stalePeerStore routes ordinary reads to an unchanged snapshot while conditional
// mutations execute against a real in-memory authority.
type stalePeerStore struct {
	kv.Store
	authority kv.Store
	ordinary  atomic.Int32
	primary   atomic.Int32
	attempts  atomic.Int32
	rejected  atomic.Int32
	apply     func(context.Context, kv.Mutation) (bool, error)
}

func newStalePeerStore(t *testing.T, authority, replica kv.Store) *stalePeerStore {
	t.Helper()
	s := &stalePeerStore{authority: authority}
	s.Store = kvfixture.ReadView(authority, replica, &s.ordinary, &s.primary)
	return s
}
func (s *stalePeerStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.attempts.Add(1)
	var matched bool
	var err error
	if s.apply != nil {
		matched, err = s.apply(ctx, mutation)
	} else {
		matched, err = s.authority.ApplyMutation(ctx, mutation)
	}
	if !matched && err == nil {
		s.rejected.Add(1)
	}
	return matched, err
}
func TestPeerReadsPreserveStaleValuesAndMissingRecords(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{90}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	initial := apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive}
	stored, err := (&Server{Store: authority}).SavePeer(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}
	data, err := authority.Get(ctx, peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, peerKey(key.String()), data); err != nil {
		t.Fatal(err)
	}
	stored.Status = apitypes.PeerRegistrationStatusBlocked
	if _, err := (&Server{Store: authority}).SavePeer(ctx, stored); err != nil {
		t.Fatal(err)
	}
	store := newStalePeerStore(t, authority, replica)
	server := &Server{Store: store}
	allowed, err := server.CheckActiveRole(ctx, key, apitypes.PeerRoleClient)
	if err != nil || !allowed || store.ordinary.Load() == 0 || store.primary.Load() != 0 {
		t.Fatalf("ordinary admission: allowed=%v err=%v reads=%d/%d", allowed, err, store.ordinary.Load(), store.primary.Load())
	}
	if err := replica.Delete(ctx, peerKey(key.String())); err != nil {
		t.Fatal(err)
	}
	got, err := server.LoadPeer(ctx, key)
	if !errors.Is(err, ErrPeerNotFound) || store.primary.Load() != 0 {
		t.Fatalf("missing record was refreshed: peer=%+v err=%v primary=%d", got, err, store.primary.Load())
	}
}

func TestPeerPartialUpdateRetriesStaleSnapshotWithoutLosingIdentifiers(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{91}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	writer := &Server{Store: authority}
	initial, err := writer.SavePeer(ctx, apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive})
	if err != nil {
		t.Fatal(err)
	}
	data, err := authority.Get(ctx, peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, peerKey(key.String()), data); err != nil {
		t.Fatal(err)
	}
	sn := "newer-sn"
	initial.Device.Identifiers = &apitypes.DeviceIdentifiers{Sn: &sn}
	if _, err := writer.SavePeer(ctx, initial); err != nil {
		t.Fatal(err)
	}
	store := newStalePeerStore(t, authority, replica)
	store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		if store.attempts.Load() == 1 && (store.ordinary.Load() == 0 || store.primary.Load() != 0) {
			t.Errorf("first write bypassed ordinary reader: %d/%d", store.ordinary.Load(), store.primary.Load())
		}
		return authority.ApplyMutation(ctx, mutation)
	}
	name := "updated-name"
	got, err := (&Server{Store: store}).putInfo(ctx, key, apitypes.DeviceInfo{Name: &name})
	if err != nil || got.Device.Name == nil || *got.Device.Name != name || got.Device.Identifiers == nil || *got.Device.Identifiers.Sn != sn {
		t.Fatalf("partial update lost newer data: %+v %v", got, err)
	}
	if store.attempts.Load() != 2 || store.rejected.Load() != 1 || store.primary.Load() == 0 {
		t.Fatalf("fallback attempts=%d rejected=%d primary=%d", store.attempts.Load(), store.rejected.Load(), store.primary.Load())
	}
	if found, err := authority.HasMember(ctx, snPrefix(sn), key.String()); err != nil || !found {
		t.Fatalf("newer index lost: %v %v", found, err)
	}
}

func TestPeerFullWriteDoesNotReplayStaleOrUncertainMutation(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale", true: "uncertain"}[uncertain], func(t *testing.T) {
			ctx, key := t.Context(), giznet.PublicKey{92}
			authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			writer := &Server{Store: authority}
			initial, err := writer.SavePeer(ctx, apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive})
			if err != nil {
				t.Fatal(err)
			}
			data, err := authority.Get(ctx, peerKey(key.String()))
			if err != nil {
				t.Fatal(err)
			}
			if err := replica.Set(ctx, peerKey(key.String()), data); err != nil {
				t.Fatal(err)
			}
			winner := initial
			name := "winner"
			winner.Device.Name = &name
			if _, err := writer.SavePeer(ctx, winner); err != nil {
				t.Fatal(err)
			}
			store := newStalePeerStore(t, authority, replica)
			failure := errors.New("uncertain commit")
			if uncertain {
				store.apply = func(context.Context, kv.Mutation) (bool, error) { return false, failure }
			}
			_, err = (&Server{Store: store}).SavePeer(ctx, initial)
			if uncertain && !errors.Is(err, failure) || !uncertain && !errors.Is(err, ErrPeerConcurrentUpdate) {
				t.Fatalf("write error=%v", err)
			}
			if store.attempts.Load() != 1 || uncertain && store.primary.Load() != 0 {
				t.Fatalf("write replayed: attempts=%d primary=%d", store.attempts.Load(), store.primary.Load())
			}
			got, err := writer.LoadPeer(ctx, key)
			if err != nil || got.Device.Name == nil || *got.Device.Name != name {
				t.Fatalf("winner changed: %+v %v", got, err)
			}
		})
	}
}

func TestBootstrapFallbackDoesNotPinUnrelatedPeersToPrimary(t *testing.T) {
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	writer := &Server{Store: authority}
	keys := []giznet.PublicKey{{101}, {102}}
	for _, key := range keys {
		initial, err := writer.SavePeer(ctx, apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive})
		if err != nil {
			t.Fatal(err)
		}
		data, err := authority.Get(ctx, peerKey(key.String()))
		if err != nil {
			t.Fatal(err)
		}
		if err := replica.Set(ctx, peerKey(key.String()), data); err != nil {
			t.Fatal(err)
		}
		name := "newer-" + key.String()
		initial.Device.Name = &name
		if _, err := writer.SavePeer(ctx, initial); err != nil {
			t.Fatal(err)
		}
	}
	store := newStalePeerStore(t, authority, replica)
	if err := (&Server{Store: store}).BootstrapEdgeNodes(ctx, keys); err != nil {
		t.Fatal(err)
	}
	if store.attempts.Load() != 4 || store.rejected.Load() != 2 {
		t.Fatalf("fallback leaked across Peer keys: attempts=%d rejected=%d", store.attempts.Load(), store.rejected.Load())
	}
	for _, key := range keys {
		got, err := writer.LoadPeer(ctx, key)
		if err != nil || got.Role != apitypes.PeerRoleEdgeNode || got.Device.Name == nil || *got.Device.Name != "newer-"+key.String() {
			t.Fatalf("bootstrap lost record: %+v %v", got, err)
		}
	}
}

type peerReadErrorStore struct {
	kv.Store
	err error
}

func (s peerReadErrorStore) Get(context.Context, kv.Key) ([]byte, error) {
	return nil, s.err
}

func TestPeerReadFailuresDoNotRefreshPrimary(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{103}
	base := kv.NewMemory(nil)
	server := &Server{Store: base, LocalRuns: peerruntest.New(t)}
	saveTestPeer(t, server, key, apitypes.DeviceInfo{})
	for _, failure := range []error{kv.ErrNotFound, errors.New("reader unavailable")} {
		for _, primary := range []bool{false, true} {
			ctx := ctx
			if primary {
				ctx = kv.WithPrimaryReads(ctx)
			}
			var ordinaryReads, primaryReads atomic.Int32
			authority := peerReadErrorStore{Store: base, err: errors.New("authority unavailable")}
			replica := peerReadErrorStore{Store: base, err: failure}
			server.Store = kvfixture.ReadView(authority, replica, &ordinaryReads, &primaryReads)
			for _, operation := range []struct {
				name string
				run  func() error
			}{
				{"load", func() error { _, err := server.LoadPeer(ctx, key); return err }},
				{"availability", func() error { return server.EnsureAvailable(ctx, key) }},
				{"role", func() error { _, err := server.CheckActiveRole(ctx, key, apitypes.PeerRoleClient); return err }},
				{"admin-list", func() error { _, _, _, err := server.listAdminPage(ctx, "", 10); return err }},
			} {
				ordinaryReads.Store(0)
				primaryReads.Store(0)
				wantError := primary || operation.name != "admin-list" || !errors.Is(failure, kv.ErrNotFound)
				if err := operation.run(); (err != nil) != wantError {
					t.Fatalf("%s error=%v, want error=%t", operation.name, err, wantError)
				}
				wantOrdinary, wantPrimary := int32(1), int32(0)
				if primary {
					wantOrdinary, wantPrimary = 0, 1
				}
				if ordinaryReads.Load() != wantOrdinary || primaryReads.Load() != wantPrimary {
					t.Fatalf("%s retried failed read (primary=%v): %d/%d", operation.name, primary, ordinaryReads.Load(), primaryReads.Load())
				}
			}
		}
	}
}

func TestPeerPartialUpdateRefreshesMissingInputAtPrimary(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{104}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	saveTestPeer(t, &Server{Store: authority}, key, apitypes.DeviceInfo{})
	store := newStalePeerStore(t, authority, replica)
	name := "updated"
	updated, err := (&Server{Store: store}).putInfo(ctx, key, apitypes.DeviceInfo{Name: &name})
	if err != nil || updated.Device.Name == nil || *updated.Device.Name != name || store.attempts.Load() != 1 || store.primary.Load() == 0 {
		t.Fatalf("missing update input: %+v %v writes=%d primary=%d", updated, err, store.attempts.Load(), store.primary.Load())
	}
}

func TestPeerDeleteDoesNotRecreateTaskAfterTombstone(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "concurrent-finalize"}[concurrent], func(t *testing.T) {
			ctx, key := t.Context(), giznet.PublicKey{105}
			authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			saveTestPeer(t, &Server{Store: authority}, key, apitypes.DeviceInfo{})
			raw, err := authority.Get(ctx, peerKey(key.String()))
			if err != nil {
				t.Fatal(err)
			}
			if err := replica.Set(ctx, peerKey(key.String()), raw); err != nil {
				t.Fatal(err)
			}
			store := newStalePeerStore(t, authority, replica)
			finalize := func() {
				if err := authority.Set(ctx, peerKey(key.String()), encodedPeerTombstone); err != nil {
					t.Fatal(err)
				}
			}
			if concurrent {
				store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
					finalize()
					return authority.ApplyMutation(ctx, mutation)
				}
			} else {
				finalize()
			}
			server := &Server{Store: store}
			for range 2 {
				response, err := server.DeletePeer(ctx, adminhttp.DeletePeerRequestObject{PublicKey: key.String()})
				if err != nil {
					t.Fatal(err)
				}
				ok, success := response.(adminhttp.DeletePeer200JSONResponse)
				if !success {
					t.Fatalf("repeated delete response=%T", response)
				}
				tombstone, err := adminhttp.PeerRegistrationResult(ok).AsExternalRef0RegistrationTombstone()
				if err != nil || tombstone.Status != apitypes.RegistrationTombstoneStatusDeleted {
					t.Fatalf("delete did not classify tombstone: %+v %v", tombstone, err)
				}
			}
			if exists, err := pendingdeletion.HasLocator(ctx, authority, pendingdeletion.KindPeer, key.String()); err != nil || exists {
				t.Fatalf("tombstone recreated deletion work: %v %v", exists, err)
			}
			refs, _, err := PendingDeletionSource(authority).ScanDue(ctx, time.Now().Add(time.Minute), 10, "")
			if err != nil || len(refs) != 0 || store.attempts.Load() != 2 || store.rejected.Load() != 2 {
				t.Fatalf("tombstone work=%+v %v attempts=%d rejected=%d", refs, err, store.attempts.Load(), store.rejected.Load())
			}
		})
	}
}

func TestPeerDeleteUsesExactRawGuardBeforeFirmwareProjection(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{106}
	authority := kv.NewMemory(nil)
	saveTestPeer(t, &Server{Store: authority}, key, apitypes.DeviceInfo{})
	raw, err := authority.Get(ctx, peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	raw = append([]byte(" \n"), raw...)
	if err := authority.Set(ctx, peerKey(key.String()), raw); err != nil {
		t.Fatal(err)
	}
	store := newStalePeerStore(t, authority, authority)
	server := &Server{Store: store, RegistrationFirmware: func(context.Context, string) (*string, error) { return new("sql-firmware"), nil }}
	projected, err := server.delete(ctx, key, pendingdeletion.ReasonAdminDelete)
	if err != nil || projected.FirmwareId == nil || *projected.FirmwareId != "sql-firmware" || store.attempts.Load() != 1 || store.primary.Load() != 0 {
		t.Fatalf("raw deletion guard: %+v %v writes=%d primary=%d", projected, err, store.attempts.Load(), store.primary.Load())
	}
	if current, err := authority.Get(ctx, peerKey(key.String())); err != nil || !isPeerTombstone(current) {
		t.Fatalf("deletion did not publish the tombstone: %v", err)
	}
	record, err := pendingdeletion.GetByLocator(ctx, authority, pendingdeletion.KindPeer, key.String())
	if err != nil {
		t.Fatal(err)
	}
	var descriptor peerDeletionDescriptor
	if err := json.Unmarshal(record.Descriptor, &descriptor); err != nil || !bytes.Equal(descriptor.PeerRecord, raw) {
		t.Fatalf("deletion did not retain exact unprojected bytes: %+v, %v", descriptor, err)
	}
}

func TestPeerDeletionConflictRetryIsBounded(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{108}
	authority := kv.NewMemory(nil)
	saveTestPeer(t, &Server{Store: authority}, key, apitypes.DeviceInfo{})
	store := newStalePeerStore(t, authority, authority)
	store.apply = func(context.Context, kv.Mutation) (bool, error) { return false, nil }
	_, err := (&Server{Store: store}).delete(ctx, key, pendingdeletion.ReasonAdminDelete)
	if !errors.Is(err, ErrPeerConcurrentUpdate) || store.attempts.Load() != 8 {
		t.Fatalf("deletion retry=%v attempts=%d", err, store.attempts.Load())
	}
}

func TestPeerDeleteDoesNotReplayUncertainCommit(t *testing.T) {
	ctx, key := t.Context(), giznet.PublicKey{107}
	authority := kv.NewMemory(nil)
	saveTestPeer(t, &Server{Store: authority}, key, apitypes.DeviceInfo{})
	store := newStalePeerStore(t, authority, authority)
	failure := errors.New("lost commit acknowledgement")
	store.apply = func(ctx context.Context, mutation kv.Mutation) (bool, error) {
		if applied, err := authority.ApplyMutation(ctx, mutation); err != nil || !applied {
			t.Fatalf("commit = %v %v", applied, err)
		}
		return false, failure
	}
	_, err := (&Server{Store: store}).delete(ctx, key, pendingdeletion.ReasonAdminDelete)
	if !errors.Is(err, failure) || store.attempts.Load() != 1 || store.primary.Load() != 0 {
		t.Fatalf("uncertain deletion replayed: %v attempts=%d primary=%d", err, store.attempts.Load(), store.primary.Load())
	}
	if exists, err := pendingdeletion.HasLocator(ctx, authority, pendingdeletion.KindPeer, key.String()); err != nil || !exists {
		t.Fatalf("committed deletion missing: %v %v", exists, err)
	}
}
