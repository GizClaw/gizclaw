//go:build gizclaw_e2e

package delete_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/google/uuid"
)

func TestPetDeletionDuringContinuousRPCUse(t *testing.T) {
	env := newDeletionHarness(t)
	peer := env.newPeer(t, "delete-pet-peer")
	petName := "delete-pet-active"
	adopted, err := peer.client.AdoptPet(env.ctx, "delete.pet.adopt", rpcapi.RuntimeAdoptRequest{
		Name: petName, DisplayName: "Delete While Active",
	})
	if err != nil {
		t.Fatalf("adopt active Pet: %v", err)
	}
	storedPet := findPeerPet(t, env, peer.publicKey, petName)
	workspaceBefore, err := peer.client.GetWorkspace(env.ctx, "delete.pet.workspace.before", rpcapi.WorkspaceGetRequest{Name: adopted.Pet.WorkspaceName})
	if err != nil {
		t.Fatalf("get retained Pet Workspace before deletion: %v", err)
	}

	firstDrive := drivePet(t, env.ctx, peer, petName, "before-delete")
	if firstDrive.GameResult == nil || len(firstDrive.Transactions) == 0 || len(firstDrive.RewardGrants) == 0 {
		t.Fatalf("active Pet use did not create retained history: %#v", firstDrive)
	}
	pointsBefore, err := peer.client.GetPoints(env.ctx, "delete.pet.points.before", rpcapi.ServerPointsGetRequest{})
	if err != nil {
		t.Fatalf("get retained Points before deletion: %v", err)
	}

	useCtx, cancelUse := context.WithCancel(context.Background())
	defer cancelUse()
	active := make(chan struct{})
	done := make(chan struct{})
	var activeOnce sync.Once
	var deleteReturned atomic.Bool
	var attemptsAfterDelete atomic.Int64
	var successesAfterDelete atomic.Int64
	go func() {
		defer close(done)
		for attempt := 0; ; attempt++ {
			select {
			case <-useCtx.Done():
				return
			default:
			}
			score := int64(attempt + 1)
			key := fmt.Sprintf("delete-pet-active-%d", attempt)
			_, err := peer.client.DrivePet(useCtx, "delete.pet.drive.concurrent", rpcapi.ServerPetDriveRequest{
				PetName: petName,
				GameResult: &rpcapi.PetDriveGameResultInput{
					GameName: "starter-game", Score: &score, IdempotencyKey: &key,
				},
			})
			if err == nil {
				activeOnce.Do(func() { close(active) })
			}
			if deleteReturned.Load() {
				attemptsAfterDelete.Add(1)
				if err == nil {
					successesAfterDelete.Add(1)
				}
			}
			select {
			case <-useCtx.Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	select {
	case <-active:
	case <-time.After(10 * time.Second):
		t.Fatal("continuous Pet use never became active")
	}

	deleted, err := peer.client.DeletePet(env.ctx, "delete.pet.submit", rpcapi.ServerPetDeleteRequest{Name: petName})
	if err != nil {
		t.Fatalf("delete active Pet: %v", err)
	}
	deleteReturned.Store(true)
	if deleted.Name != petName || deleted.WorkspaceName != adopted.Pet.WorkspaceName {
		t.Fatalf("Pet delete response = %#v", deleted)
	}
	if _, err := peer.client.DeletePet(env.ctx, "delete.pet.repeat", rpcapi.ServerPetDeleteRequest{Name: petName}); err == nil {
		t.Fatal("repeat Pet delete unexpectedly bypassed the deletion fence")
	}
	time.Sleep(300 * time.Millisecond)
	cancelUse()
	<-done
	if attemptsAfterDelete.Load() == 0 {
		t.Fatal("continuous Pet use made no attempt after delete response")
	}
	if successesAfterDelete.Load() != 0 {
		t.Fatalf("Pet mutations succeeded after delete response: %d", successesAfterDelete.Load())
	}

	deletionID, err := pendingdeletion.DeterministicDeletionID(pendingdeletion.Locator{
		Kind: pendingdeletion.KindPet, ResourceID: storedPet.Id, OwnerPublicKey: &peer.publicKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForPetAndTaskAbsent(t, env, peer.publicKey, storedPet.Id, uuid.MustParse(deletionID))
	if _, err := peer.client.GetPet(env.ctx, "delete.pet.get.after", rpcapi.ServerPetGetRequest{Name: petName}); err == nil {
		t.Fatal("deleted Pet is still readable")
	} else {
		var rpcErr rpcapi.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.StatusCodeNotFound {
			t.Fatalf("deleted Pet error = %v, want typed not found", err)
		}
	}
	workspaceAfter, err := peer.client.GetWorkspace(env.ctx, "delete.pet.workspace.after", rpcapi.WorkspaceGetRequest{Name: adopted.Pet.WorkspaceName})
	if err != nil {
		t.Fatalf("ordinary Pet deletion stopped or removed retained Workspace: %v", err)
	}
	if workspaceAfter.Value.Name != workspaceBefore.Value.Name || workspaceAfter.Value.WorkflowName != workspaceBefore.Value.WorkflowName {
		t.Fatalf("retained Pet Workspace changed: before=%#v after=%#v", workspaceBefore.Value, workspaceAfter.Value)
	}
	results, err := peer.client.ListGameResults(env.ctx, "delete.pet.results.after", rpcapi.ServerGameResultListRequest{})
	if err != nil {
		t.Fatalf("list retained Pet results: %v", err)
	}
	if !hasGameResult(results.Items, firstDrive.GameResult.Name) {
		t.Fatalf("ordinary Pet deletion removed retained GameResult %q", firstDrive.GameResult.Name)
	}
	pointsAfter, err := peer.client.GetPoints(env.ctx, "delete.pet.points.after", rpcapi.ServerPointsGetRequest{})
	if err != nil {
		t.Fatalf("get retained Points after deletion: %v", err)
	}
	if pointsAfter.OwnerPublicKey != pointsBefore.OwnerPublicKey {
		t.Fatalf("Points owner changed across Pet deletion: before=%#v after=%#v", pointsBefore, pointsAfter)
	}
	transactions, err := peer.client.ListPointsTransactions(env.ctx, "delete.pet.transactions.after", rpcapi.ServerPointsTransactionListRequest{})
	if err != nil {
		t.Fatalf("list retained Points transactions: %v", err)
	}
	for _, expected := range []string{adopted.Transaction.Name, firstDrive.Transactions[0].Name} {
		transaction, found := findPointsTransaction(transactions.Items, expected)
		if !found || transaction.PetName == nil || *transaction.PetName != petName {
			t.Fatalf("retained Points transaction %q = %#v, found=%v", expected, transaction, found)
		}
	}
	grants, err := peer.client.ListRewardGrants(env.ctx, "delete.pet.grants.after", rpcapi.ServerRewardGrantListRequest{})
	if err != nil {
		t.Fatalf("list retained RewardGrants: %v", err)
	}
	grant, found := findRewardGrant(grants.Items, firstDrive.RewardGrants[0].Name)
	if !found || grant.PetName == nil || *grant.PetName != petName {
		t.Fatalf("retained RewardGrant %q = %#v, found=%v", firstDrive.RewardGrants[0].Name, grant, found)
	}
	if got := findPeerPets(t, env, peer.publicKey, petName); len(got) != 0 {
		t.Fatalf("deleted Pet name resolved to replacement generations: %#v", got)
	}
}

func drivePet(t *testing.T, ctx context.Context, peer deletionPeer, petName, suffix string) *rpcapi.ServerPetDriveResponse {
	t.Helper()
	score := int64(42)
	key := "delete-pet-" + suffix
	result, err := peer.client.DrivePet(ctx, "delete.pet.drive."+suffix, rpcapi.ServerPetDriveRequest{
		PetName: petName,
		GameResult: &rpcapi.PetDriveGameResultInput{
			GameName: "starter-game", Score: &score, IdempotencyKey: &key,
		},
	})
	if err != nil {
		t.Fatalf("drive Pet %q: %v", suffix, err)
	}
	return result
}

func findPeerPet(t *testing.T, env *deletionHarness, owner, name string) apitypes.Pet {
	t.Helper()
	items := findPeerPets(t, env, owner, name)
	if len(items) != 1 {
		t.Fatalf("Pet owner=%q name=%q generations=%d, want 1", owner, name, len(items))
	}
	return items[0]
}

func findPeerPets(t *testing.T, env *deletionHarness, owner, name string) []apitypes.Pet {
	t.Helper()
	response, err := env.api.ListPeerPetsWithResponse(env.ctx, owner, nil)
	if err != nil || response.JSON200 == nil {
		t.Fatalf("list Peer Pets: status=%d body=%s error=%v", response.StatusCode(), response.Body, err)
	}
	var matches []apitypes.Pet
	for _, pet := range response.JSON200.Items {
		if pet.Name == name {
			matches = append(matches, pet)
		}
	}
	return matches
}

func waitForPetAndTaskAbsent(t *testing.T, env *deletionHarness, owner, petID string, deletionID uuid.UUID) {
	t.Helper()
	params := &adminhttp.GetPendingDeletionParams{Source: "gameplay"}
	waitUntil(t, env.ctx, "Pet deletion", func() (bool, string) {
		pet, petErr := env.api.GetPeerPetWithResponse(env.ctx, owner, petID)
		task, taskErr := env.api.GetPendingDeletionWithResponse(env.ctx, deletionID, params)
		if petErr != nil || taskErr != nil {
			return false, fmt.Sprintf("pet_error=%v task_error=%v", petErr, taskErr)
		}
		return pet.StatusCode() == http.StatusNotFound && task.StatusCode() == http.StatusNotFound,
			fmt.Sprintf("pet_status=%d task_status=%d", pet.StatusCode(), task.StatusCode())
	})
}

func hasGameResult(items []rpcapi.GameResult, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func findPointsTransaction(items []rpcapi.PointsTransaction, name string) (rpcapi.PointsTransaction, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return rpcapi.PointsTransaction{}, false
}

func findRewardGrant(items []rpcapi.RewardGrant, name string) (rpcapi.RewardGrant, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return rpcapi.RewardGrant{}, false
}
