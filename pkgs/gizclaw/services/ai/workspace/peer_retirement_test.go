package workspace

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

func TestPeerWorkspaceRetirementSnapshotsAndMarksOnlyOwnedUserWorkspaces(t *testing.T) {
	ctx := t.Context()
	server := newTestServer(t)
	store := server.DB
	now := time.Now().UTC()
	ownerA, ownerB := "peer-a", "peer-b"
	itemA := deletionTestWorkspace("workspace-a", "a", &ownerA, false, now)
	itemB := deletionTestWorkspace("workspace-b", "b", &ownerB, false, now)
	petWorkspace := deletionTestWorkspace("workspace-pet", "pet-a", &ownerA, true, now)
	for _, item := range []apitypes.Workspace{itemA, itemB, petWorkspace} {
		if err := createSQLWorkspace(ctx, store, item); err != nil {
			t.Fatal(err)
		}
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
	if pending, err := NewPendingDeletionSource(store).HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindWorkspace, ResourceID: itemA.Id}); err != nil || !pending {
		t.Fatalf("owned Workspace marker = %v, %v", pending, err)
	}
	if _, err := getWorkspaceByID(ctx, store, itemB.Id); err != nil {
		t.Fatalf("foreign Workspace removed: %v", err)
	}
	if pending, err := NewPendingDeletionSource(store).HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindWorkspace, ResourceID: petWorkspace.Id}); err != nil || pending {
		t.Fatalf("Pet Workspace marker before Pet completion = %v, %v", pending, err)
	}
	petIDs, err := server.RetirePeerPetWorkspaces(ctx, snapshot)
	if err != nil || len(petIDs) != 1 || petIDs[0] != petWorkspace.Id {
		t.Fatalf("RetirePeerPetWorkspaces() = %#v, %v", petIDs, err)
	}
	if pending, err := NewPendingDeletionSource(store).HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindWorkspace, ResourceID: petWorkspace.Id}); err != nil || !pending {
		t.Fatalf("Pet Workspace marker after Pet completion = %v, %v", pending, err)
	}
	if _, err := server.RetirePeerWorkspaces(ctx, snapshot); err != nil {
		t.Fatalf("replay retirement: %v", err)
	}
	if _, err := server.RetirePeerPetWorkspaces(ctx, snapshot); err != nil {
		t.Fatalf("replay Pet Workspace retirement: %v", err)
	}
}
