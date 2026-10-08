package friend

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRetirementRecoveryContinuesPastMalformedIntent(t *testing.T) {
	s := newTestServer()
	workspaces := s.Workspaces.(*recordingWorkspaceService)
	interrupted := errors.New("interrupted Workspace handoff")
	for _, peers := range [][2]string{{"peer-a", "peer-b"}, {"peer-c", "peer-d"}} {
		workspaces.retireErr = nil
		if _, err := s.AdminCreateFriend(t.Context(), peers[0], peers[1]); err != nil {
			t.Fatal(err)
		}
		workspaces.retireErr = interrupted
		if _, err := s.DeleteFriend(t.Context(), peers[0], rpcapi.FriendDeleteRequest{Name: peers[1]}); !errors.Is(err, interrupted) {
			t.Fatalf("create interrupted retirement: %v", err)
		}
	}
	var ids []string
	for id, err := range (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).IDs(t.Context(), s.Friends) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 {
		t.Fatalf("indexed retirements = %v", ids)
	}
	if err := s.Friends.Set(t.Context(), retirementIntentKey(ids[0]), []byte("{")); err != nil {
		t.Fatal(err)
	}
	workspaces.retireErr = nil
	if err := s.ReconcileRetirementIntents(t.Context()); err == nil {
		t.Fatal("malformed intent was not reported")
	}
	if data, err := s.Friends.Get(t.Context(), retirementIntentKey(ids[0])); err != nil || string(data) != "{" {
		t.Fatalf("failed intent changed: %q, %v", data, err)
	}
	if _, err := s.Friends.Get(t.Context(), retirementIntentKey(ids[1])); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("later valid intent was not completed: %v", err)
	}
	if _, err := readRetirementReceipt(t.Context(), s.Friends, ids[1]); err != nil {
		t.Fatalf("valid retirement proof not committed: %v", err)
	}
}

func TestRelationMutationLockCancellationReleasesPeerGates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &Server{}
		release, err := s.lockRelation(t.Context(), "held-relation")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		waiting := make(chan error, 1)
		go func() {
			unlock, err := s.lockRelationMutation(ctx, "held-relation", "peer-a", "peer-b")
			if err == nil {
				unlock()
			}
			waiting <- err
		}()
		synctest.Wait()
		select {
		case err := <-waiting:
			t.Fatalf("same relationship was not serialized: %v", err)
		default:
		}
		other, err := s.lockRelation(t.Context(), "independent-relation")
		if err != nil {
			t.Fatal(err)
		}
		other()
		cancel()
		if err := <-waiting; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled relationship waiter: %v", err)
		}
		peers, err := s.lockPeers(t.Context(), "peer-a", "peer-b")
		if err != nil {
			t.Fatal(err)
		}
		peers()
	})
}
