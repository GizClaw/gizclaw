package workspace

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPeerWorkspaceRetirementSnapshotsAndMarksOnlyOwnedUserWorkspaces(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	now := time.Now().UTC()
	ownerA, ownerB := "peer-a", "peer-b"
	itemA := deletionTestWorkspace("workspace-a", "a", &ownerA, false, now)
	itemB := deletionTestWorkspace("workspace-b", "b", &ownerB, false, now)
	petWorkspace := deletionTestWorkspace("workspace-pet", "pet-a", &ownerA, true, now)
	for _, item := range []struct {
		workspaceID string
		name        string
		owner       string
		value       any
	}{{itemA.Id, itemA.Name, ownerA, itemA}, {itemB.Id, itemB.Name, ownerB, itemB}} {
		data, err := json.Marshal(item.value)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.BatchSet(ctx, []kv.Entry{
			{Key: workspaceKey(item.workspaceID), Value: data},
			{Key: workspaceByOwnerKey(item.owner, item.name), Value: []byte(item.workspaceID)},
			{Key: workspaceScopeNameKey(&item.owner, item.name), Value: []byte(item.workspaceID)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	petData, err := json.Marshal(petWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BatchSet(ctx, []kv.Entry{
		{Key: workspaceKey(petWorkspace.Id), Value: petData},
		{Key: workspaceScopeNameKey(petWorkspace.OwnerPublicKey, petWorkspace.Name), Value: []byte(petWorkspace.Id)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.fastDeleteWorkspaceRecord(ctx, store, itemA); err != nil {
		t.Fatalf("preexisting Workspace deletion marker: %v", err)
	}
	snapshot, err := server.SnapshotPeerWorkspaces(ctx, ownerA, []string{petWorkspace.Id})
	if err != nil || len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].ID != itemA.Id ||
		len(snapshot.PetWorkspaces) != 1 || snapshot.PetWorkspaces[0].ID != petWorkspace.Id {
		t.Fatalf("SnapshotPeerWorkspaces() = %#v, %v", snapshot, err)
	}
	ids, err := server.RetirePeerWorkspaces(ctx, snapshot)
	if err != nil || len(ids) != 1 || ids[0] != itemA.Id {
		t.Fatalf("RetirePeerWorkspaces() = %#v, %v", ids, err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindWorkspace, itemA.Id); err != nil || !pending {
		t.Fatalf("owned Workspace marker = %v, %v", pending, err)
	}
	if _, err := store.Get(ctx, workspaceKey(itemB.Id)); err != nil {
		t.Fatalf("foreign Workspace removed: %v", err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindWorkspace, petWorkspace.Id); err != nil || pending {
		t.Fatalf("Pet Workspace marker before Pet completion = %v, %v", pending, err)
	}
	petIDs, err := server.RetirePeerPetWorkspaces(ctx, snapshot)
	if err != nil || len(petIDs) != 1 || petIDs[0] != petWorkspace.Id {
		t.Fatalf("RetirePeerPetWorkspaces() = %#v, %v", petIDs, err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindWorkspace, petWorkspace.Id); err != nil || !pending {
		t.Fatalf("Pet Workspace marker after Pet completion = %v, %v", pending, err)
	}
	if _, err := server.RetirePeerWorkspaces(ctx, snapshot); err != nil {
		t.Fatalf("replay retirement: %v", err)
	}
	if _, err := server.RetirePeerPetWorkspaces(ctx, snapshot); err != nil {
		t.Fatalf("replay Pet Workspace retirement: %v", err)
	}
}
