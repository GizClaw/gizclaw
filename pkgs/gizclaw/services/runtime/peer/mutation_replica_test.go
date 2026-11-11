package peer

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
)

func TestAdminPeerDirectorySkipsHistoricalHolesAndAdvancesCursor(t *testing.T) {
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	s := &Server{Store: authority, LocalRuns: peerruntest.New(t)}
	for _, key := range []giznet.PublicKey{{1}, {2}, {3}} {
		saveTestPeer(t, s, key, apitypes.DeviceInfo{})
	}
	keys, _, err := s.LocalRuns.ListPeerPublicKeys(ctx, "", 10)
	if err != nil || len(keys) != 3 {
		t.Fatalf("local keys=%v, %v", keys, err)
	}
	visible, err := authority.Get(ctx, peerKey(keys[1]))
	if err != nil {
		t.Fatal(err)
	}
	if err := replica.Set(ctx, peerKey(keys[1]), visible); err != nil {
		t.Fatal(err)
	}
	var ordinary, primary atomic.Int32
	s.Store = kvfixture.ReadView(authority, replica, &ordinary, &primary)
	cursor := ""
	for i := range 3 {
		items, more, next, err := s.listAdminPage(ctx, cursor, 1)
		if err != nil || more != (i < 2) {
			t.Fatalf("page %d: items=%v more=%t cursor=%v error=%v", i, items, more, next, err)
		}
		if i == 1 {
			if len(items) != 1 || registrationResultForTest(t, items[0]).PublicKey != keys[1] {
				t.Fatalf("visible Peer omitted: %v", items)
			}
		} else if len(items) != 0 {
			t.Fatalf("historically absent Peer fabricated: %v", items)
		}
		if more {
			if next == nil || *next != keys[i] || *next <= cursor {
				t.Fatalf("hole did not advance local cursor: %v", next)
			}
			cursor = *next
		} else if next != nil {
			t.Fatalf("last page retained cursor: %v", next)
		}
	}
	if primary.Load() != 0 || ordinary.Load() != 3 {
		t.Fatalf("ordinary list upgraded reads: ordinary=%d primary=%d", ordinary.Load(), primary.Load())
	}
}

func TestSQLMutationPeerInputRefreshesOnlyMissingHistoricalPeer(t *testing.T) {
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	key := giznet.PublicKey{89}
	s := &Server{Store: authority}
	saveTestPeer(t, s, key, apitypes.DeviceInfo{})
	var ordinary, primary atomic.Int32
	s.Store = kvfixture.ReadView(authority, replica, &ordinary, &primary)
	if err := s.EnsureAvailable(ctx, key); !errors.Is(err, ErrPeerNotFound) || primary.Load() != 0 {
		t.Fatalf("ordinary absence=%v primary=%d", err, primary.Load())
	}
	if err := s.EnsureAvailableForMutation(ctx, key); err != nil || primary.Load() == 0 {
		t.Fatalf("newly registered mutation input=%v primary=%d", err, primary.Load())
	}
	if err := s.DeleteSelf(kv.WithPrimaryReads(ctx), key); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAvailableForMutation(ctx, key); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("missing reader accepted retired writer input: %v", err)
	}
	if err := s.EnsureAvailableForMutation(ctx, giznet.PublicKey{90}); !errors.Is(err, ErrPeerNotFound) {
		t.Fatalf("unknown owner accepted: %v", err)
	}
}

type preparedPeerWorkspaces struct {
	*peerDeletionAdapters
	prepared     workspace.PeerRetirementSnapshot
	prepareCalls int
	readCalls    int
}

func (s *preparedPeerWorkspaces) SnapshotPeerWorkspaces(ctx context.Context, owner string) (workspace.PeerRetirementSnapshot, error) {
	s.readCalls++
	return s.peerDeletionAdapters.SnapshotPeerWorkspaces(ctx, owner)
}

func (s *preparedPeerWorkspaces) PreparePeerWorkspacesRetirement(context.Context, string) (workspace.PeerRetirementSnapshot, error) {
	s.prepareCalls++
	return s.prepared, nil
}

func TestPeerRetirementRequiresFencedWorkspaceInventory(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_plan", true: "existing_incomplete_plan"}[existing], func(t *testing.T) {
			ctx, key := t.Context(), giznet.PublicKey{91}
			store := kv.NewMemory(nil)
			s := &Server{Store: store, ServerPublicKey: giznet.PublicKey{92}}
			saveTestPeer(t, s, key, apitypes.DeviceInfo{})
			if err := s.DeleteSelf(ctx, key); err != nil {
				t.Fatal(err)
			}
			source := PendingDeletionSource(store)
			claim := claimPeerDeletion(t, source, time.Now().Add(time.Second))
			base := &peerDeletionAdapters{publicKey: key.String()}
			prepared := &preparedPeerWorkspaces{peerDeletionAdapters: base, prepared: workspace.PeerRetirementSnapshot{
				PublicKey: key.String(), Workspaces: []workspace.PeerRetirementWorkspace{{ID: "owned", Name: "owned"}},
			}}
			handler := DeletionHandler{Server: s, Source: source, Social: base, Workspaces: prepared}
			if existing {
				current, raw, err := handler.retainedPeer(ctx, key)
				if err != nil {
					t.Fatal(err)
				}
				plan := retirementPlan{Version: retirementPlanVersion, HomeServer: s.ServerPublicKey.String(), PeerFingerprint: peerFingerprint(raw), MarkerFingerprint: claim.MarkerFingerprint, Peer: current,
					Social: social.PeerSnapshot{PublicKey: key.String()}, Workspaces: workspace.PeerRetirementSnapshot{PublicKey: key.String()}}
				data, err := json.Marshal(plan)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Set(ctx, peerRetirementPlanKey(claim.Record.DeletionID), data); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := handler.loadOrCreatePlan(ctx, claim, key)
			if existing {
				var outcome *pendingdeletion.OutcomeError
				if !errors.As(err, &outcome) || outcome.Code != "workspace_snapshot_incomplete" {
					t.Fatalf("incomplete SQL plan accepted: %+v, %v", plan, err)
				}
				if _, err := source.GetTask(ctx, claim.Record.DeletionID); err != nil {
					t.Fatalf("incomplete task consumed: %v", err)
				}
			} else if err != nil || !plan.Workspaces.Covers(prepared.prepared) {
				t.Fatalf("fenced SQL inventory omitted: %+v, %v", plan, err)
			}
			if prepared.prepareCalls != 1 || prepared.readCalls != 0 {
				t.Fatalf("prepare/probe=%d/%d", prepared.prepareCalls, prepared.readCalls)
			}
		})
	}
}
