package friend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFriendReAddAtFullQuotaProvesReaderAbsence(t *testing.T) {
	for _, lifecycle := range []string{"new", "replaced"} {
		for _, operation := range []string{"admin", "invitation"} {
			t.Run(lifecycle+"/"+operation, func(t *testing.T) {
				ctx := t.Context()
				s := newPeerLimitTestServer()
				root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
				s.Friends, s.InviteTokens = root, root
				token, err := s.CreateFriendInviteToken(ctx, "target", rpcapi.FriendInviteTokenCreateRequest{})
				if err != nil {
					t.Fatal(err)
				}
				for i := range socialutil.PeerFriendLimit {
					if _, err := s.AdminCreateFriend(ctx, "owner", fmt.Sprintf("previous-%02d", i)); err != nil {
						t.Fatal(err)
					}
				}
				root.replicate = false
				primary := kv.WithPrimaryReads(ctx)
				if _, err := s.DeleteFriend(primary, "owner", rpcapi.FriendDeleteRequest{Name: "previous-00"}); err != nil {
					t.Fatal(err)
				}
				current, err := s.AdminCreateFriend(primary, "owner", "target")
				if err != nil {
					t.Fatal(err)
				}
				if lifecycle == "replaced" {
					if _, err := s.DeleteFriend(primary, "owner", rpcapi.FriendDeleteRequest{Name: "target"}); err != nil {
						t.Fatal(err)
					}
					current, err = s.AdminCreateFriend(primary, "owner", "target")
					if err != nil {
						t.Fatal(err)
					}
				}
				root.primary = 0
				if _, err := s.GetFriendRelation(ctx, "owner", "target"); !errors.Is(err, kv.ErrNotFound) || root.primary != 0 {
					t.Fatalf("ordinary absent read changed policy: primary=%d err=%v", root.primary, err)
				}
				workspaces := s.Workspaces.(*recordingWorkspaceService)
				created, rejected := len(workspaces.created), root.rejected
				var item rpcapi.FriendObject
				if operation == "admin" {
					item, err = s.AdminCreateFriend(ctx, "owner", "target")
				} else {
					var existing bool
					item, existing, err = s.AddFriendReportingExisting(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken})
					if !existing {
						t.Fatal("writer's active pair was not reported existing")
					}
				}
				if err != nil || socialutil.StringValue(item.WorkspaceName) != socialutil.StringValue(current.WorkspaceName) {
					t.Fatalf("full-quota re-add=%+v current=%+v err=%v", item, current, err)
				}
				if root.rejected != rejected+1 || root.primary == 0 || root.primary > 6 {
					t.Fatalf("absence retry rejected=%d primary=%d", root.rejected-rejected, root.primary)
				}
				if len(workspaces.created) != created {
					t.Fatal("idempotent re-add created another Workspace")
				}
			})
		}
	}
}

func TestFriendInactiveMutationProofAvoidsPrimaryAndPropagatesUnknownOutcome(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil)}
	s.Friends = root
	if _, active, err := s.readActiveRelationshipForMutation(ctx, root, "owner", "peer", nil); err != nil || active || root.primary != 0 || root.mutations != 1 {
		t.Fatalf("inactive proof: active=%v primary=%d mutations=%d err=%v", active, root.primary, root.mutations, err)
	}
	uncertain := errors.New("inactive proof outcome unavailable")
	root.writeErr = uncertain
	before := root.mutations
	if _, _, err := s.readActiveRelationshipForMutation(ctx, root, "owner", "peer", nil); !errors.Is(err, uncertain) || root.mutations != before+1 || root.primary != 0 {
		t.Fatalf("unknown inactive proof replayed: mutations=%d primary=%d err=%v", root.mutations-before, root.primary, err)
	}
}

func TestFriendInvitationRevocationGuardsPublicationAndExistingProof(t *testing.T) {
	for _, phase := range []string{"before-workspace", "publication", "existing"} {
		for _, change := range []string{"record", "index"} {
			t.Run(phase+"/"+change, func(t *testing.T) {
				ctx := t.Context()
				s := newTestServer()
				root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
				s.Friends = kv.Prefixed(root, kv.Key{"relations"})
				s.InviteTokens = kv.Prefixed(root, kv.Key{"invitations"})
				token, err := s.CreateFriendInviteToken(ctx, "inviter", rpcapi.FriendInviteTokenCreateRequest{})
				if err != nil {
					t.Fatal(err)
				}
				if phase == "existing" {
					if _, err := s.AddFriend(ctx, "caller", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil {
						t.Fatal(err)
					}
				}
				root.replicate = false
				authorityInvites := kv.Prefixed(root.Store, kv.Key{"invitations"})
				revoke := func() {
					if change == "index" {
						if err := authorityInvites.Delete(ctx, socialutil.InviteTokenIndexKey(socialutil.FriendInviteTokensRoot, token.InviteToken)); err != nil {
							t.Fatal(err)
						}
					} else {
						record := inviteTokenRecord{PeerPublicKey: "inviter", InviteToken: "rotated", CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Hour)}
						// Keep the original digest locator to isolate the record guard.
						if err := socialutil.WriteJSON(ctx, authorityInvites, socialutil.FriendInviteTokenKey("inviter"), record); err != nil {
							t.Fatal(err)
						}
					}
				}
				if phase == "publication" {
					root.beforeWrite = func(_ context.Context, mutation kv.Mutation) {
						for _, entry := range mutation.Entries {
							if slices.Equal(entry.Key, append(kv.Key{"relations"}, workspaceBindingKey(socialutil.RelationID("caller", "inviter"))...)) {
								root.beforeWrite = nil
								revoke()
								return
							}
						}
					}
				} else {
					revoke()
				}
				_, _, err = s.AddFriendReportingExisting(ctx, "caller", rpcapi.FriendAddRequest{InviteToken: token.InviteToken})
				if !errors.Is(err, ErrInviteTokenUnavailable) {
					t.Fatalf("revoked invitation acknowledged: %v", err)
				}
				_, active, err := readActiveRelationship(kv.WithPrimaryReads(ctx), s.Friends, "caller", "inviter")
				if err != nil || active != (phase == "existing") {
					t.Fatalf("active=%v err=%v", active, err)
				}
				workspaces := s.Workspaces.(*recordingWorkspaceService)
				if phase == "before-workspace" && len(workspaces.created) != 0 {
					t.Fatal("revoked stale invitation materialized SQL")
				}
				if phase == "publication" && !slices.Equal(workspaces.deleted, []string{workspaces.created[0].Name}) {
					t.Fatalf("cancelled Workspace cleanup=%v", workspaces.deleted)
				}
				if err := s.ReconcileCreationIntents(kv.WithPrimaryReads(ctx)); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestFriendInvitationRenewalKeepsPendingIncarnation(t *testing.T) {
	for _, phase := range []string{"recovery", "publication"} {
		t.Run(phase, func(t *testing.T) {
			ctx := t.Context()
			s := newTestServer()
			root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
			s.Friends, s.InviteTokens = root, root
			token, err := s.CreateFriendInviteToken(ctx, "peer", rpcapi.FriendInviteTokenCreateRequest{})
			if err != nil {
				t.Fatal(err)
			}
			var pending creationIntent
			if phase == "recovery" {
				_, admission, err := s.observeFriendInvitation(ctx, token.InviteToken)
				if err != nil {
					t.Fatal(err)
				}
				pending, err = s.getOrCreateCreationIntent(ctx, root, "owner", "peer", "peer", admission)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.CreateFriendInviteTokenWithTTL(ctx, "peer", time.Hour); err != nil {
					t.Fatal(err)
				}
				if err := s.ReconcileCreationIntents(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				root.beforeWrite = func(_ context.Context, mutation kv.Mutation) {
					for _, entry := range mutation.Entries {
						if slices.Equal(entry.Key, workspaceBindingKey(socialutil.RelationID("owner", "peer"))) {
							root.beforeWrite, root.replicate = nil, false
							var err error
							pending, err = readCreationIntent(kv.WithPrimaryReads(ctx), root, socialutil.RelationID("owner", "peer"))
							if err != nil {
								t.Fatal(err)
							}
							record := inviteTokenRecord{PeerPublicKey: "peer", InviteToken: token.InviteToken, CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Hour)}
							if err := socialutil.WriteInviteToken(ctx, root.Store, socialutil.FriendInviteTokenKey("peer"), record); err != nil {
								t.Fatal(err)
							}
							return
						}
					}
				}
				if _, err := s.AddFriend(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil {
					t.Fatal(err)
				}
			}
			item, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer")
			if err != nil || socialutil.StringValue(item.WorkspaceName) != pending.Workspace {
				t.Fatalf("renewal changed incarnation: item=%+v pending=%+v err=%v", item, pending, err)
			}
			workspaces := s.Workspaces.(*recordingWorkspaceService)
			if len(workspaces.created) != 1 || len(workspaces.deleted) != 0 {
				t.Fatalf("renewal Workspace lifecycle created=%v deleted=%v", workspaces.created, workspaces.deleted)
			}
		})
	}
}

func TestFriendInvitationExistingProofUsesNoPrimaryAndDoesNotReplayErrors(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends, s.InviteTokens = root, root
	token, err := s.CreateFriendInviteToken(ctx, "peer", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFriend(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil {
		t.Fatal(err)
	}
	root.primary = 0
	if _, existing, err := s.AddFriendReportingExisting(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); err != nil || !existing || root.primary != 0 {
		t.Fatalf("idempotent proof existing=%v primary=%d err=%v", existing, root.primary, err)
	}
	uncertain := errors.New("idempotent proof outcome unavailable")
	root.writeErr = uncertain
	before := root.mutations
	if _, _, err := s.AddFriendReportingExisting(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); !errors.Is(err, uncertain) || root.mutations != before+1 {
		t.Fatalf("proof error replayed: writes=%d err=%v", root.mutations-before, err)
	}
}

func TestFriendInvitationUnknownPublicationIsNotReplayedOrCancelled(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends, s.InviteTokens = root, root
	token, err := s.CreateFriendInviteToken(ctx, "peer", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	uncertain := errors.New("relationship commit outcome unavailable")
	attempts := 0
	root.beforeWrite = func(_ context.Context, mutation kv.Mutation) {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, workspaceBindingKey(socialutil.RelationID("owner", "peer"))) {
				attempts++
				root.writeErr = uncertain
			}
		}
	}
	if _, err := s.AddFriend(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken}); !errors.Is(err, uncertain) || attempts != 1 {
		t.Fatalf("unknown commit replayed attempts=%d err=%v", attempts, err)
	}
	root.writeErr, root.beforeWrite = nil, nil
	if _, active, err := readActiveRelationship(kv.WithPrimaryReads(ctx), root, "owner", "peer"); err != nil || !active {
		t.Fatalf("known committed relationship cancelled=%v, %v", active, err)
	}
	if len(s.Workspaces.(*recordingWorkspaceService).deleted) != 0 {
		t.Fatal("unknown commit compensated blindly")
	}
}

func TestFriendInvitationRecoveryRetainsExactGuard(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	token, err := s.CreateFriendInviteToken(ctx, "inviter", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, invitation, err := s.observeFriendInvitation(ctx, token.InviteToken)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := s.getOrCreateCreationIntent(ctx, s.Friends, "caller", "inviter", "inviter", invitation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClearFriendInviteToken(ctx, "inviter", rpcapi.FriendInviteTokenClearRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileCreationIntents(ctx); err != nil {
		t.Fatal(err)
	}
	if _, active, err := readActiveRelationship(ctx, s.Friends, "caller", "inviter"); err != nil || active {
		t.Fatalf("recovery bypassed revocation: %v, %v", active, err)
	}
	if len(s.Workspaces.(*recordingWorkspaceService).created) != 0 {
		t.Fatal("recovery materialized a revoked invitation")
	}
	decision, err := readCreationDecision(ctx, s.Friends, intent)
	if err != nil || decision.State != creationDecisionCancelled {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestFriendMutationProvesExistingPairAfterStaleDeletion(t *testing.T) {
	for _, operation := range []string{"admin", "invitation"} {
		t.Run(operation, func(t *testing.T) {
			ctx := t.Context()
			s := newTestServer()
			root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
			s.Friends, s.InviteTokens = root, root
			token, err := s.CreateFriendInviteToken(ctx, "peer", rpcapi.FriendInviteTokenCreateRequest{})
			if err != nil {
				t.Fatal(err)
			}
			old, err := s.AdminCreateFriend(ctx, "owner", "peer")
			if err != nil {
				t.Fatal(err)
			}
			root.replicate = false
			if _, err := s.DeleteFriend(kv.WithPrimaryReads(ctx), "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
				t.Fatal(err)
			}
			root.primary = 0
			stale, err := s.GetFriendRelation(ctx, "owner", "peer")
			if err != nil || socialutil.StringValue(stale.WorkspaceName) != socialutil.StringValue(old.WorkspaceName) || root.primary != 0 {
				t.Fatalf("ordinary read=%+v primary=%d err=%v", stale, root.primary, err)
			}
			var created rpcapi.FriendObject
			if operation == "admin" {
				created, err = s.AdminCreateFriend(ctx, "owner", "peer")
			} else {
				var existing bool
				created, existing, err = s.AddFriendReportingExisting(ctx, "owner", rpcapi.FriendAddRequest{InviteToken: token.InviteToken})
				if existing {
					t.Fatal("deleted pair acknowledged as existing")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			current, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer")
			if err != nil || socialutil.StringValue(current.WorkspaceName) != socialutil.StringValue(created.WorkspaceName) || socialutil.StringValue(created.WorkspaceName) == socialutil.StringValue(old.WorkspaceName) {
				t.Fatalf("created=%+v current=%+v err=%v", created, current, err)
			}
		})
	}
}

func TestFriendFullReplicaRefreshesBeforeAdmissionOrPendingCancellation(t *testing.T) {
	for _, phase := range []string{"precheck", "commit"} {
		t.Run(phase, func(t *testing.T) {
			ctx := t.Context()
			s := newPeerLimitTestServer()
			root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
			s.Friends = root
			var pending creationIntent
			if phase == "commit" {
				var err error
				pending, err = s.getOrCreateCreationIntent(ctx, root, "owner", "new-peer", "owner")
				if err != nil {
					t.Fatal(err)
				}
			}
			for i := range socialutil.PeerFriendLimit {
				if _, err := s.AdminCreateFriend(ctx, "owner", fmt.Sprintf("peer-%02d", i)); err != nil {
					t.Fatal(err)
				}
			}
			root.replicate = false
			revision, err := root.Store.Get(ctx, peerFriendRevisionKey("owner"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DeleteFriend(kv.WithPrimaryReads(ctx), "owner", rpcapi.FriendDeleteRequest{Name: "peer-00"}); err != nil {
				t.Fatal(err)
			}
			after, err := root.Store.Get(ctx, peerFriendRevisionKey("owner"))
			if err != nil || !bytes.Equal(revision, after) {
				t.Fatalf("deletion changed admission revision=%q -> %q err=%v", revision, after, err)
			}
			var item rpcapi.FriendObject
			if phase == "commit" {
				workspace, err := s.ensureCreationWorkspace(ctx, pending)
				if err != nil {
					t.Fatal(err)
				}
				item, err = s.commitFriendCreation(ctx, root, "owner", "new-peer", pending, workspace)
				if err != nil {
					t.Fatal(err)
				}
				if socialutil.StringValue(item.WorkspaceName) != pending.Workspace {
					t.Fatal("capacity refresh changed pending incarnation")
				}
			} else {
				item, err = s.AdminCreateFriend(ctx, "owner", "new-peer")
				if err != nil {
					t.Fatal(err)
				}
			}
			if slices.Contains(s.Workspaces.(*recordingWorkspaceService).deleted, socialutil.StringValue(item.WorkspaceName)) {
				t.Fatal("stale full quota cancelled admitted incarnation")
			}
			members, err := root.Store.ListMembers(ctx, friendCollectionKey("owner"))
			if err != nil || len(members) != socialutil.PeerFriendLimit {
				t.Fatalf("authority count=%d err=%v", len(members), err)
			}
			latest, err := root.Store.Get(ctx, peerFriendRevisionKey("owner"))
			if err != nil || bytes.Equal(revision, latest) {
				t.Fatalf("admission revision did not advance=%q err=%v", latest, err)
			}
		})
	}
}

func TestPrepareFriendRetirementDoesNotSealLaggedEmptyInventory(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil)}
	s.Friends = store
	created, err := s.AdminCreateFriend(kv.WithPrimaryReads(ctx), "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	primary, mutations := store.primary, store.mutations
	if snapshot, err := s.SnapshotPeerFriends(ctx, "owner"); err != nil || len(snapshot) != 0 {
		t.Fatalf("ordinary snapshot = %+v, %v", snapshot, err)
	}
	if store.primary != primary || store.mutations != mutations {
		t.Fatal("ordinary snapshot changed read policy or wrote a fence")
	}
	snapshot, err := s.PreparePeerFriendsRetirement(ctx, "owner")
	if err != nil || len(snapshot) != 1 || snapshot[0].WorkspaceName != socialutil.StringValue(created.WorkspaceName) {
		t.Fatalf("prepared snapshot = %+v, %v", snapshot, err)
	}
	if _, err := s.CreateFriendInviteToken(ctx, "owner", rpcapi.FriendInviteTokenCreateRequest{}); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("invitation after retirement fence = %v", err)
	}
	for _, pair := range [][2]string{{"owner", "new-first"}, {"new-second", "owner"}} {
		if _, err := s.AdminCreateFriend(ctx, pair[0], pair[1]); !errors.Is(err, ownership.ErrOwnerRetired) {
			t.Fatalf("admission for %v after fence = %v", pair, err)
		}
	}
}

func TestFriendInFlightCreationCannotPublishAfterRetirementFence(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	intent, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := s.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot, err := s.PreparePeerFriendsRetirement(ctx, "peer"); err != nil || len(snapshot) != 0 {
		t.Fatalf("prepare pending relation = %+v, %v", snapshot, err)
	}
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("late commit = %v", err)
	}
	if _, err := s.GetFriendRelation(ctx, "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("late relationship published = %v", err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if !slices.Equal(workspaces.deleted, []string{intent.Workspace}) {
		t.Fatalf("cancelled Workspace cleanup = %v", workspaces.deleted)
	}
}

func TestFriendInviteRenewalRecomputesAfterCASRejection(t *testing.T) {
	for _, change := range []string{"rotated", "extended"} {
		t.Run(change, func(t *testing.T) {
			ctx := t.Context()
			s := newTestServer()
			store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
			s.InviteTokens = store
			old := inviteTokenRecord{PeerPublicKey: "owner", InviteToken: "old", CreatedAt: s.now(), ExpiresAt: s.now().Add(time.Hour)}
			if err := socialutil.WriteInviteToken(ctx, store, socialutil.FriendInviteTokenKey("owner"), old); err != nil {
				t.Fatal(err)
			}
			store.replicate = false
			current := old
			current.ExpiresAt = s.now().Add(6 * time.Hour)
			if change == "rotated" {
				current.InviteToken = "rotated"
			}
			store.beforeWrite = func(ctx context.Context, mutation kv.Mutation) {
				store.beforeWrite = nil
				if err := socialutil.WriteInviteToken(ctx, store.Store, socialutil.FriendInviteTokenKey("owner"), current); err != nil {
					t.Fatal(err)
				}
			}
			result, err := s.CreateFriendInviteTokenWithTTL(ctx, "owner", 2*time.Hour)
			if err != nil || result.InviteToken != current.InviteToken || !result.ExpiresAt.Equal(current.ExpiresAt) {
				t.Fatalf("renewal = %+v, %v; want token %q expiry %v", result, err, current.InviteToken, current.ExpiresAt)
			}
			if store.rejected != 1 || store.primary == 0 {
				t.Fatalf("renewal retry rejected=%d primary=%d", store.rejected, store.primary)
			}
			for _, token := range []string{"old", "rotated"} {
				_, err := socialutil.ReadInviteToken(kv.WithPrimaryReads(ctx), store, socialutil.FriendInviteTokensRoot, token)
				if (token == current.InviteToken) != (err == nil) {
					t.Fatalf("token %q index = %v", token, err)
				}
			}
		})
	}
}

func TestFriendStaleCreationRecoveryDoesNotRecreateRetiredWorkspace(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	intent, err := s.getOrCreateCreationIntent(ctx, store, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	workspace, err := s.ensureCreationWorkspace(kv.WithPrimaryReads(ctx), intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitFriendCreation(kv.WithPrimaryReads(ctx), store, "owner", "peer", intent, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(kv.WithPrimaryReads(ctx), "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	before := len(workspaces.owners)
	if err := s.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("stale creation recovery = %v", err)
	}
	if len(workspaces.owners) != before {
		t.Fatal("stale intent recreated its retired SQL Workspace")
	}
	if _, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("retired relationship restored = %v", err)
	}
}

func TestFriendRejectedCreatorCompensatesOnlyItsRetiredIncarnation(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	intent, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := s.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	newRelation, err := s.AdminCreateFriend(ctx, "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); !errors.Is(err, errFriendCreationCancelled) {
		t.Fatalf("delayed old commit = %v", err)
	}
	active, err := s.GetFriendRelation(ctx, "owner", "peer")
	if err != nil || socialutil.StringValue(active.WorkspaceName) != socialutil.StringValue(newRelation.WorkspaceName) {
		t.Fatalf("new lifecycle changed = %+v, %v", active, err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if workspaces.retired[len(workspaces.retired)-1] != workspace.Id || len(workspaces.deleted) != 0 {
		t.Fatalf("compensation retired=%v deleted=%v", workspaces.retired, workspaces.deleted)
	}
	// A primary availability rejection uses cancellation cleanup too. Its old
	// intent must still leave the replacement binding and relationship intact.
	s.PeerAvailability = func(context.Context, string) error { return ownership.ErrOwnerRetired }
	if _, err := s.commitFriendCreation(ctx, s.Friends, "owner", "peer", intent, workspace); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("unavailable stale creator = %v", err)
	}
	active, err = s.GetFriendRelation(ctx, "owner", "peer")
	if err != nil || socialutil.StringValue(active.WorkspaceName) != socialutil.StringValue(newRelation.WorkspaceName) ||
		workspaces.retired[len(workspaces.retired)-1] != workspace.Id {
		t.Fatalf("old cancellation touched replacement: active=%+v retired=%v err=%v", active, workspaces.retired, err)
	}
}

func TestFriendStaleReceiptCannotAcknowledgeNewActiveIncarnation(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	old, err := s.AdminCreateFriend(ctx, "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	current, err := s.AdminCreateFriend(kv.WithPrimaryReads(ctx), "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"})
	if err != nil || socialutil.StringValue(deleted.WorkspaceName) != socialutil.StringValue(current.WorkspaceName) ||
		socialutil.StringValue(deleted.WorkspaceName) == socialutil.StringValue(old.WorkspaceName) {
		t.Fatalf("delete acknowledged stale receipt = %+v, %v", deleted, err)
	}
	if _, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("new incarnation left active = %v", err)
	}
}

func TestFriendAdminCreateProjectionMatchesCommittedLifecycle(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	if _, err := s.AdminCreateFriend(ctx, "owner", "peer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	id := socialutil.RelationID("owner", "peer")
	created, err := s.AdminCreateFriendResource(ctx, id, "owner", "peer")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := readWorkspaceBinding(kv.WithPrimaryReads(ctx), store, id)
	if err != nil || created.WorkspaceId != binding.WorkspaceID {
		t.Fatalf("Admin creation = %+v binding=%+v err=%v", created, binding, err)
	}
}

func TestFriendCapacityRetryRechecksBothPeersOnAuthority(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = store
	retired := false
	var refreshed []string
	s.PeerAvailability = func(ctx context.Context, peer string) error {
		if kv.WithPrimaryReads(ctx) == ctx && retired {
			refreshed = append(refreshed, peer)
			if peer == "peer" {
				return ownership.ErrOwnerRetired
			}
		}
		return nil
	}
	store.beforeWrite = func(ctx context.Context, mutation kv.Mutation) {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, workspaceBindingKey(socialutil.RelationID("owner", "peer"))) {
				store.beforeWrite = nil
				store.replicate = false
				retired = true
				if err := store.Store.Set(ctx, peerFriendRevisionKey("owner"), []byte("concurrent-capacity")); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	if _, err := s.AdminCreateFriend(ctx, "owner", "peer"); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("capacity retry bypassed availability = %v", err)
	}
	if !slices.Equal(refreshed, []string{"owner", "peer"}) {
		t.Fatalf("fallback availability peers = %v", refreshed)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	if len(workspaces.created) != 1 || !slices.Equal(workspaces.deleted, []string{workspaces.created[0].Name}) {
		t.Fatalf("retry changed incarnation or leaked Workspace: created=%v deleted=%v", workspaces.created, workspaces.deleted)
	}
	if _, err := s.GetFriendRelation(kv.WithPrimaryReads(ctx), "owner", "peer"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("unavailable peer was admitted = %v", err)
	}
}
