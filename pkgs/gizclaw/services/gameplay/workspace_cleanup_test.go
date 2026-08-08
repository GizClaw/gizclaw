package gameplay

import (
	"testing"
)

func TestDeleteWorkspaceDataIsExactAndPreservesPetBinding(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"workspace-a", "workspace-b"} {
		if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_workspace_reward_sources
			(workspace_id, scheduled_checkpoint, completed_checkpoint, created_at, updated_at)
			VALUES (?, '', '', ?, ?)`, id, formatTime(*now), formatTime(*now)); err != nil {
			t.Fatal(err)
		}
		tx, err := runtime.DB.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := insertDriveFactOutbox(ctx, tx, driveFactOutbox{
			ObservationID: "observation-" + id, PayloadDigest: "digest-" + id,
			OwnerPublicKey: "peer-a", RuntimeProfile: "profile-a", PetID: "pet-a",
			Target:  DriveFactTarget{WorkspaceID: id, ProfileID: "profile-a", ProfileRevision: "revision-a", BindingName: "memory", BindingIdentity: "binding"},
			Payload: driveFactPayload{ID: "fact-" + id, Text: "fact", ObservedAt: *now},
			State:   driveFactPending, NextAttemptAt: *now, CreatedAt: *now, UpdatedAt: *now,
		}); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_pet_workspace_bindings
		(owner_public_key, pet_id, runtime_profile_id, workspace_id, created_at)
		VALUES (?, ?, ?, ?, ?)`, "peer-a", "pet-a", "profile-a", "workspace-a", formatTime(*now)); err != nil {
		t.Fatal(err)
	}

	if err := runtime.DeleteWorkspaceData(ctx, "workspace-a"); err != nil {
		t.Fatal(err)
	}
	if err := runtime.DeleteWorkspaceData(ctx, "workspace-a"); err != nil {
		t.Fatalf("idempotent retry error = %v", err)
	}
	if absent, err := runtime.WorkspaceDataAbsent(ctx, "workspace-a"); err != nil || !absent {
		t.Fatalf("WorkspaceDataAbsent(target) = %v, %v", absent, err)
	}
	if absent, err := runtime.WorkspaceDataAbsent(ctx, "workspace-b"); err != nil || absent {
		t.Fatalf("WorkspaceDataAbsent(foreign) = %v, %v", absent, err)
	}
	var bindings int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_pet_workspace_bindings WHERE workspace_id = ?`, "workspace-a").Scan(&bindings); err != nil || bindings != 1 {
		t.Fatalf("Pet binding count = %d, %v", bindings, err)
	}
}

func TestDeleteWorkspaceDataRollsBack(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_workspace_reward_sources
		(workspace_id, scheduled_checkpoint, completed_checkpoint, created_at, updated_at)
		VALUES (?, '', '', ?, ?)`, "workspace-a", formatTime(*now), formatTime(*now)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `CREATE TRIGGER fail_workspace_source_delete
		BEFORE DELETE ON gameplay_workspace_reward_sources
		WHEN OLD.workspace_id = 'workspace-a'
		BEGIN SELECT RAISE(ABORT, 'forced cleanup failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := runtime.DeleteWorkspaceData(ctx, "workspace-a"); err == nil {
		t.Fatal("DeleteWorkspaceData() error = nil")
	}
	var sources int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?`, "workspace-a").Scan(&sources); err != nil || sources != 1 {
		t.Fatalf("source count after rollback = %d, %v", sources, err)
	}
}
