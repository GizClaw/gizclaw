package friendgroup

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFriendGroupDeletionHandlerFinalizesCurrentMarker(t *testing.T) {
	s, groupID, source, claim, now := retiredFriendGroupClaim(t)
	var descriptor map[string]any
	if err := json.Unmarshal(claim.Record.Descriptor, &descriptor); err != nil {
		t.Fatal(err)
	}
	if len(descriptor) != 1 || descriptor["friend_group_id"] != groupID {
		t.Fatalf("current descriptor = %#v, want only friend_group_id", descriptor)
	}
	foreign, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace,
		"workspace-foreign",
		nil,
		pendingdeletion.ReasonResourceDelete,
		map[string]string{"workspace_id": "workspace-foreign"},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(t.Context(), s.RelationshipStore, foreign); err != nil {
		t.Fatal(err)
	}

	handler := DeletionHandler{Server: s, Source: source, Now: func() time.Time { return now.Add(time.Second) }}
	if err := handler.Handle(t.Context(), claim); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := source.GetTask(t.Context(), claim.Record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("GetTask() error = %v, want ErrNotFound", err)
	}
	if _, err := pendingdeletion.GetByLocator(t.Context(), s.RelationshipStore, pendingdeletion.KindFriendGroup, groupID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Friend Group locator error = %v, want not found", err)
	}
	if _, err := s.readRetirementReceipt(t.Context(), groupID); err != nil {
		t.Fatalf("retirement receipt removed: %v", err)
	}
	if _, err := pendingdeletion.GetByLocator(t.Context(), s.RelationshipStore, pendingdeletion.KindWorkspace, foreign.ResourceID); err != nil {
		t.Fatalf("foreign Workspace marker changed: %v", err)
	}
	if err := handler.Handle(t.Context(), claim); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("replayed Handle() error = %v, want ErrNotFound", err)
	}
}

func TestFriendGroupDeletionHandlerPreservesLegacyLocatorTargets(t *testing.T) {
	ctx := t.Context()
	s := newTestServer(t)
	s.NewID = func() string { return "group-legacy" }
	group, err := s.CreateFriendGroup(ctx, "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	legacyKey := kv.Key{"friend-group-messages", socialutil.EscapeStoreSegment(groupID), "message-a"}
	legacyValue := []byte(`{"body":"retained"}`)
	if err := s.Groups.Set(ctx, legacyKey, legacyValue); err != nil {
		t.Fatal(err)
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindFriendGroup,
		groupID,
		nil,
		pendingdeletion.ReasonFriendGroupDelete,
		retiredFriendGroupDataDescriptor{
			FriendGroupID:      groupID,
			MessageStorePrefix: []string{"friend-group-messages", socialutil.EscapeStoreSegment(groupID)},
			MessageAssetPrefix: socialutil.EscapeStoreSegment(groupID) + "/",
		},
		s.now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(ctx, s.RelationshipStore, record); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriendGroup(ctx, "peer-a", rpcapi.FriendGroupDeleteRequest{Name: group.Name}); err != nil {
		t.Fatal(err)
	}
	source := NewPendingDeletionSource(s.RelationshipStore)
	claim := claimFriendGroupTask(t, source, s.now().Add(time.Second))
	if err := (DeletionHandler{Server: s, Source: source, Now: func() time.Time { return s.now().Add(time.Second) }}).Handle(ctx, claim); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	got, err := s.Groups.Get(ctx, legacyKey)
	if err != nil || string(got) != string(legacyValue) {
		t.Fatalf("legacy message target = %q, %v; want %q", got, err, legacyValue)
	}
}

func TestFriendGroupDeletionHandlerDefersRetirementIntent(t *testing.T) {
	s, groupID, source, claim, now := retiredFriendGroupClaim(t)
	if err := s.RelationshipStore.Set(t.Context(), groupRetirementIntentKey(groupID), []byte(`{"pending":true}`)); err != nil {
		t.Fatal(err)
	}
	err := (DeletionHandler{Server: s, Source: source, Now: func() time.Time { return now.Add(time.Second) }}).Handle(t.Context(), claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred {
		t.Fatalf("Handle() error = %#v, want deferred", err)
	}
	task, err := source.GetTask(t.Context(), claim.Record.DeletionID)
	if err != nil || task.FailureCount != 0 || task.Status != pendingdeletion.StatusRunning {
		t.Fatalf("task after defer = %#v, %v", task, err)
	}
}

func TestFriendGroupDeletionHandlerDefersEveryResidualControlPlane(t *testing.T) {
	tests := map[string]func(*testing.T, *Server, string){
		"group": func(t *testing.T, s *Server, groupID string) {
			t.Helper()
			if err := s.Groups.Set(t.Context(), socialutil.GroupKey(groupID), []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		},
		"invite": func(t *testing.T, s *Server, groupID string) {
			t.Helper()
			if err := s.InviteTokens.Set(t.Context(), socialutil.GroupInviteTokenKey(groupID), []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		},
		"workspace binding": func(t *testing.T, s *Server, groupID string) {
			t.Helper()
			if err := s.RelationshipStore.Set(t.Context(), workspaceBindingKey(groupID), []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		},
		"member": func(t *testing.T, s *Server, groupID string) {
			t.Helper()
			if err := s.Members.Set(t.Context(), socialutil.GroupMemberKey(groupID, "peer-a"), []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		},
		"belongs": func(t *testing.T, s *Server, groupID string) {
			t.Helper()
			value, err := json.Marshal(friendGroupMemberRecord{FriendGroupID: groupID, PeerPublicKey: "peer-a"})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Belongs.Set(t.Context(), socialutil.GroupBelongKey("peer-a", groupID), value); err != nil {
				t.Fatal(err)
			}
		},
		"membership name": func(t *testing.T, s *Server, groupID string) {
			t.Helper()
			if err := s.Belongs.Set(t.Context(), socialutil.GroupNameKey("peer-a", "room"), []byte(groupID)); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, seed := range tests {
		t.Run(name, func(t *testing.T) {
			s, groupID, source, claim, now := retiredFriendGroupClaim(t)
			seed(t, s, groupID)
			err := (DeletionHandler{Server: s, Source: source, Now: func() time.Time { return now.Add(time.Second) }}).Handle(t.Context(), claim)
			var outcome *pendingdeletion.OutcomeError
			if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred || outcome.Code != "social_cleanup_incomplete" {
				t.Fatalf("Handle() error = %#v, want deferred social_cleanup_incomplete", err)
			}
			if _, err := source.GetTask(t.Context(), claim.Record.DeletionID); err != nil {
				t.Fatalf("task removed after rejected residual: %v", err)
			}
		})
	}
}

func TestFriendGroupDeletionHandlerDefersMissingReceipt(t *testing.T) {
	s, groupID, source, claim, now := retiredFriendGroupClaim(t)
	if err := s.RelationshipStore.Delete(t.Context(), groupRetirementReceiptKey(groupID)); err != nil {
		t.Fatal(err)
	}
	err := (DeletionHandler{Server: s, Source: source, Now: func() time.Time { return now.Add(time.Second) }}).Handle(t.Context(), claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeDeferred || outcome.Code != "retirement_receipt_pending" {
		t.Fatalf("Handle() error = %#v, want deferred retirement_receipt_pending", err)
	}
}

func TestFriendGroupDeletionHandlerRejectsReceiptWithoutImmutableName(t *testing.T) {
	s, groupID, source, claim, now := retiredFriendGroupClaim(t)
	receipt, err := s.readRetirementReceipt(t.Context(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Name = ""
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RelationshipStore.Set(t.Context(), groupRetirementReceiptKey(groupID), data); err != nil {
		t.Fatal(err)
	}
	err = (DeletionHandler{Server: s, Source: source, Now: func() time.Time { return now.Add(time.Second) }}).Handle(t.Context(), claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeTerminal || outcome.Code != "retirement_receipt_invalid" {
		t.Fatalf("Handle() error = %#v, want terminal retirement_receipt_invalid", err)
	}
}

func retiredFriendGroupClaim(t *testing.T) (*Server, string, pendingdeletion.KVSource, pendingdeletion.Claim, time.Time) {
	t.Helper()
	s := newTestServer(t)
	s.NewID = func() string { return "group-cleanup" }
	group, err := s.CreateFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupCreateRequest{Name: "room"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := mustGroupID(t, s, "peer-a", group.Name)
	if _, err := s.DeleteFriendGroup(t.Context(), "peer-a", rpcapi.FriendGroupDeleteRequest{Name: group.Name}); err != nil {
		t.Fatal(err)
	}
	source := NewPendingDeletionSource(s.RelationshipStore)
	now := s.now()
	return s, groupID, source, claimFriendGroupTask(t, source, now.Add(time.Second)), now
}

func claimFriendGroupTask(t *testing.T, source pendingdeletion.KVSource, now time.Time) pendingdeletion.Claim {
	t.Helper()
	refs, _, err := source.ScanDue(t.Context(), now, 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(t.Context(), refs[0], now, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	return claim
}
