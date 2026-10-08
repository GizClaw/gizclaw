package friendgroup

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func interruptedGroupRetirement(t *testing.T, s *Server, owner, name string, members ...string) string {
	t.Helper()
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	workspaces.retireErr = nil
	group, err := s.CreateFriendGroup(t.Context(), owner, rpcapi.FriendGroupCreateRequest{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	id := mustGroupID(t, s, owner, name)
	for _, member := range members {
		if _, err := s.AddFriendGroupMember(t.Context(), owner, rpcapi.FriendGroupMemberAddRequest{
			FriendGroupName: name,
			PeerPublicKey:   member,
			MemberName:      name + "-" + member,
			Role:            rpcapi.FriendGroupMemberMutableRole("member"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	interrupted := errors.New("interrupted Workspace handoff")
	workspaces.retireErr = interrupted
	if _, err := s.DeleteFriendGroup(t.Context(), owner, rpcapi.FriendGroupDeleteRequest{Name: group.Name}); !errors.Is(err, interrupted) {
		t.Fatalf("create interrupted retirement: %v", err)
	}
	return id
}

func TestGroupMutationLockAllowsIndependentProgressAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &Server{}
		release, err := s.lockGroup(t.Context(), "held-group")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		waiting := make(chan error, 1)
		go func() {
			unlock, err := s.lockGroup(ctx, "held-group")
			if err == nil {
				unlock()
			}
			waiting <- err
		}()
		synctest.Wait()
		select {
		case err := <-waiting:
			t.Fatalf("same Group was not serialized: %v", err)
		default:
		}
		other, err := s.lockGroup(t.Context(), "independent-group")
		if err != nil {
			t.Fatal(err)
		}
		other()
		cancel()
		if err := <-waiting; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Group waiter: %v", err)
		}
		release()
		again, err := s.lockGroup(t.Context(), "held-group")
		if err != nil {
			t.Fatal(err)
		}
		again()
	})
}

func TestRetirementRecoveryAllowsUnavailablePeersAndPreservesAdmissionFence(t *testing.T) {
	for _, unavailablePeer := range []string{"peer-a", "peer-b"} {
		for _, state := range []string{"pending deletion", "completed deletion"} {
			t.Run(unavailablePeer+"/"+state, func(t *testing.T) {
				unavailableErr := errors.New("peer: " + state)
				s := newTestServer(t)
				id := interruptedGroupRetirement(t, s, "peer-a", "room", "peer-b")
				workspaces := s.Workspaces.(*recordingWorkspaceService)
				workspaces.retireErr = nil
				s.PeerAvailability = func(_ context.Context, peer string) error {
					if peer == unavailablePeer {
						return unavailableErr
					}
					return nil
				}
				if _, err := s.CreateFriendGroup(t.Context(), unavailablePeer, rpcapi.FriendGroupCreateRequest{Name: "new-room"}); !errors.Is(err, unavailableErr) {
					t.Fatalf("normal admission allowed a deleting Peer: %v", err)
				}
				intent, err := s.readRetirementIntent(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				// The admission helper used by the old recovery path must still reject
				// unavailable Peers. Only committed-retirement recovery bypasses it.
				_, release, err := s.lockRetirementPeers(t.Context(), intent)
				if release != nil {
					release()
				}
				if !errors.Is(err, unavailableErr) {
					t.Fatalf("ordinary retirement admission no longer rejects the Peer: %v", err)
				}
				if err := s.ReconcileRetirementIntents(t.Context()); err != nil {
					t.Fatalf("recovery rejected a deleting Peer: %v", err)
				}
				if _, err := s.readRetirementIntent(t.Context(), id); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("retirement intent was not completed: %v", err)
				}
				if _, err := s.AdminGetFriendGroup(t.Context(), id); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("retired Group was revived: %v", err)
				}
				receipt, err := s.readRetirementReceipt(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				if err := validateFriendGroupRetirementReceipt(receipt, id); err != nil {
					t.Fatalf("retirement proof: %v", err)
				}
				if len(workspaces.retired) != 2 || workspaces.retired[0] != workspaces.retired[1] {
					t.Fatalf("recovery changed the retired Workspace: %v", workspaces.retired)
				}
				if _, err := pendingdeletion.GetByLocator(t.Context(), s.RelationshipStore, pendingdeletion.KindFriendGroup, id); err != nil {
					t.Fatalf("data cleanup was not handed to PendingDeletion: %v", err)
				}
				if err := s.ReconcileRetirementIntents(t.Context()); err != nil || len(workspaces.retired) != 2 {
					t.Fatalf("completed recovery was not idempotent: %v", err)
				}
			})
		}
	}
}

func TestRetirementRecoveryContinuesPastMalformedIntent(t *testing.T) {
	s := newTestServer(t)
	interruptedGroupRetirement(t, s, "peer-a", "room-a")
	interruptedGroupRetirement(t, s, "peer-b", "room-b")
	var ids []string
	for id, err := range (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).IDs(t.Context(), s.RelationshipStore) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 {
		t.Fatalf("indexed retirements = %v", ids)
	}
	if err := s.RelationshipStore.Set(t.Context(), groupRetirementIntentKey(ids[0]), []byte("{")); err != nil {
		t.Fatal(err)
	}
	s.Workspaces.(*recordingWorkspaceService).retireErr = nil
	if err := s.ReconcileRetirementIntents(t.Context()); err == nil {
		t.Fatal("malformed intent was not reported")
	}
	if data, err := s.RelationshipStore.Get(t.Context(), groupRetirementIntentKey(ids[0])); err != nil || string(data) != "{" {
		t.Fatalf("failed intent changed: %q, %v", data, err)
	}
	if _, err := s.readRetirementIntent(t.Context(), ids[1]); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("later valid intent was not completed: %v", err)
	}
	if _, err := s.readRetirementReceipt(t.Context(), ids[1]); err != nil {
		t.Fatalf("valid retirement proof not committed: %v", err)
	}
}

func TestRetirementRecoveryRejectsInvalidSnapshotBeforeHandoff(t *testing.T) {
	s := newTestServer(t)
	id := interruptedGroupRetirement(t, s, "peer-a", "room")
	intent, err := s.readRetirementIntent(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	intent.Members = nil
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RelationshipStore.Set(t.Context(), groupRetirementIntentKey(id), data); err != nil {
		t.Fatal(err)
	}
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	workspaces.retireErr = nil
	if err := s.ReconcileRetirementIntents(t.Context()); err == nil {
		t.Fatal("invalid persisted snapshot accepted")
	}
	if len(workspaces.retired) != 1 {
		t.Fatal("invalid snapshot caused an additional Workspace handoff")
	}
	if _, err := s.readRetirementIntent(t.Context(), id); err != nil {
		t.Fatalf("invalid intent was lost: %v", err)
	}
}
