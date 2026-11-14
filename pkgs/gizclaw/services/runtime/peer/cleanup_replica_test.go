package peer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type preparedPeerSocial struct {
	*peerDeletionAdapters
	prepared     social.PeerSnapshot
	prepareCalls int
	readCalls    int
}

func (s *preparedPeerSocial) SnapshotPeerSocial(ctx context.Context, owner string) (social.PeerSnapshot, error) {
	s.readCalls++
	return s.peerDeletionAdapters.SnapshotPeerSocial(ctx, owner)
}

func (s *preparedPeerSocial) PreparePeerSocialRetirement(context.Context, string) (social.PeerSnapshot, error) {
	s.prepareCalls++
	return s.prepared, nil
}

func TestPeerRetirementRequiresPreparedSocialCoverage(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_plan", true: "existing_incomplete_plan"}[existing], func(t *testing.T) {
			ctx, key := t.Context(), giznet.PublicKey{98}
			store := kv.NewMemory(nil)
			server := &Server{Store: store, ServerPublicKey: giznet.PublicKey{100}}
			saveTestPeer(t, server, key, apitypes.DeviceInfo{})
			if err := server.DeleteSelf(ctx, key); err != nil {
				t.Fatal(err)
			}
			source := PendingDeletionSource(store)
			claim := claimPeerDeletionFinalPass(t, source, time.Now().Add(time.Second))
			base := &peerDeletionAdapters{publicKey: key.String()}
			created := time.Now().UTC()
			prepared := &preparedPeerSocial{peerDeletionAdapters: base, prepared: social.PeerSnapshot{
				PublicKey: key.String(),
				Groups:    []friendgroup.PeerRetirementGroup{{FriendGroupID: "group-a", OwnerPublicKey: "foreign", PeerPublicKey: key.String(), WorkspaceID: "workspace-a", CreatedAt: &created}},
			}}
			handler := DeletionHandler{Server: server, Source: source, Social: prepared, Workspaces: base}
			if existing {
				current, raw, err := handler.retainedPeer(ctx, claim.Record, key)
				if err != nil {
					t.Fatal(err)
				}
				plan := retirementPlan{Version: retirementPlanVersion, HomeServer: server.ServerPublicKey.String(), PeerFingerprint: peerFingerprint(raw), MarkerFingerprint: claim.MarkerFingerprint, Peer: current, Social: social.PeerSnapshot{PublicKey: key.String()}}
				plan.Workspaces.PublicKey = key.String()
				data, err := json.Marshal(plan)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Set(ctx, peerRetirementPlanKey(claim.Record.DeletionID), data); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := handler.loadOrCreatePlan(ctx, claim, key)
			if err != nil || !plan.Social.Covers(prepared.prepared) {
				t.Fatalf("refreshed Social inventory omitted: %+v, %v", plan, err)
			}
			if _, err := source.GetTask(ctx, claim.Record.DeletionID); err != nil {
				t.Fatalf("refresh consumed the request: %v", err)
			}
			if prepared.prepareCalls != 1 || prepared.readCalls != 0 {
				t.Fatalf("preparation/read calls = %d/%d", prepared.prepareCalls, prepared.readCalls)
			}
			if base.sessionCalls+base.bindingCalls+base.quiesceCalls != 0 {
				t.Fatal("coverage check performed cleanup")
			}
		})
	}
}
