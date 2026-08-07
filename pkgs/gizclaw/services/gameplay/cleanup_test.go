package gameplay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/google/uuid"
)

func TestPendingDeletionProcessorPhysicallyDeletesPet(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-cleanup", apitypes.PetAdoptRequest{Name: "pet-main", DisplayName: "Pet"})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	if _, err := runtime.DeletePet(ctx, "peer-cleanup", adopted.Pet.Id); err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}

	source := PendingDeletionSource{DB: runtime.DB}
	registry := pendingdeletion.NewRegistry()
	if err := registry.Register(source, PetDeletionHandler{DB: runtime.DB, Now: func() time.Time { return now.UTC() }}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	config := pendingdeletion.Config{
		ScanInterval: 10 * time.Millisecond, PageSize: 10, DispatchCapacity: 4, Workers: 1,
		LeaseDuration: time.Second, AttemptTimeout: 500 * time.Millisecond,
		RetryInitial: 10 * time.Millisecond, RetryMax: 100 * time.Millisecond, MaxAttempts: 3,
	}
	processor, err := pendingdeletion.NewProcessor(registry, config, nil)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}
	processor.Start(context.Background())
	t.Cleanup(processor.Close)
	processor.Wake()

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err = runtime.GetPet(ctx, "peer-cleanup", adopted.Pet.Id)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			t.Fatalf("GetPet() error = %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("Pet still exists after processor deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}

	var bindings, markers, locators int
	if err := runtime.DB.QueryRow(`SELECT COUNT(*) FROM gameplay_pet_workspace_bindings WHERE owner_public_key = ? AND pet_id = ?`, "peer-cleanup", adopted.Pet.Id).Scan(&bindings); err != nil {
		t.Fatalf("count binding: %v", err)
	}
	if err := runtime.DB.QueryRow(`SELECT COUNT(*) FROM gameplay_pending_deletions`).Scan(&markers); err != nil {
		t.Fatalf("count markers: %v", err)
	}
	if err := runtime.DB.QueryRow(`SELECT COUNT(*) FROM gameplay_pending_deletion_locators`).Scan(&locators); err != nil {
		t.Fatalf("count locators: %v", err)
	}
	if bindings != 1 || markers != 0 || locators != 0 {
		t.Fatalf("bindings=%d markers=%d locators=%d, want 1, 0, 0", bindings, markers, locators)
	}
	admin := pendingdeletion.NewAdmin(registry, nil)
	record, err := pendingdeletion.New(pendingdeletion.KindPet, adopted.Pet.Id, &adopted.Pet.OwnerPublicKey, pendingdeletion.ReasonResourceDelete, petDeletionDescriptor{
		OwnerPublicKey: adopted.Pet.OwnerPublicKey, PetID: adopted.Pet.Id,
		RuntimeProfile: adopted.Pet.RuntimeProfileId, PetDefID: adopted.Pet.PetDefId,
		WorkspaceID: adopted.Pet.WorkspaceId,
	}, now.UTC())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := admin.Get(ctx, gameplayPendingDeletionSource, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("Admin.Get() error = %v, want ErrNotFound", err)
	}
	if _, err := admin.Retry(ctx, gameplayPendingDeletionSource, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("Admin.Retry() error = %v, want ErrNotFound", err)
	}
}

func TestPendingDeletionSourceAllowsOnlyOneLiveClaim(t *testing.T) {
	ctx, runtime, _ := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-claim", apitypes.PetAdoptRequest{Name: "pet-main", DisplayName: "Pet"})
	if err != nil {
		t.Fatalf("AdoptPet() error = %v", err)
	}
	if _, err := runtime.DeletePet(ctx, "peer-claim", adopted.Pet.Id); err != nil {
		t.Fatalf("DeletePet() error = %v", err)
	}
	source := PendingDeletionSource{DB: runtime.DB}
	refs, _, err := source.ScanDue(ctx, time.Now().Add(time.Hour), 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], time.Now().UTC(), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim(first) = %#v, %v, %v", claim, claimed, err)
	}
	if _, claimed, err := source.Claim(ctx, refs[0], time.Now().UTC(), time.Minute); err != nil || claimed {
		t.Fatalf("Claim(second) = %v, %v, want false, nil", claimed, err)
	}
	stale := claim
	stale.LeaseToken = "stale"
	if err := source.Renew(ctx, stale, time.Now().UTC(), time.Minute); !errors.Is(err, pendingdeletion.ErrConflict) {
		t.Fatalf("Renew(stale) error = %v, want ErrConflict", err)
	}
}

func TestPetDeletionFinalizationRollsBackWhenLocatorChanged(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-rollback", apitypes.PetAdoptRequest{Name: "pet-main", DisplayName: "Pet"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DeletePet(ctx, "peer-rollback", adopted.Pet.Id); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource{DB: runtime.DB}
	refs, _, err := source.ScanDue(ctx, now.Add(time.Second), 1, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now.Add(time.Second), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `DELETE FROM gameplay_pending_deletion_locators WHERE deletion_id = ?`, claim.Record.DeletionID); err != nil {
		t.Fatal(err)
	}
	err = (PetDeletionHandler{DB: runtime.DB, Now: func() time.Time { return now.Add(time.Second) }}).Handle(ctx, claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeTerminal {
		t.Fatalf("Handle() error = %#v, want terminal locator failure", err)
	}
	if _, err := runtime.GetPet(ctx, "peer-rollback", adopted.Pet.Id); err != nil {
		t.Fatalf("GetPet() after rollback error = %v", err)
	}
	if _, err := source.GetTask(ctx, claim.Record.DeletionID); err != nil {
		t.Fatalf("GetTask() after rollback error = %v", err)
	}
}

func TestPetDeletionExpiredLeaseDeletesNothing(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-expired", apitypes.PetAdoptRequest{Name: "pet-main", DisplayName: "Pet"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DeletePet(ctx, "peer-expired", adopted.Pet.Id); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource{DB: runtime.DB}
	refs, _, err := source.ScanDue(ctx, now.Add(time.Second), 1, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now.Add(time.Second), time.Second)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	err = (PetDeletionHandler{DB: runtime.DB, Now: func() time.Time { return now.Add(3 * time.Second) }}).Handle(ctx, claim)
	if !errors.Is(err, pendingdeletion.ErrConflict) {
		t.Fatalf("Handle() error = %v, want ErrConflict", err)
	}
	if _, err := runtime.GetPet(ctx, "peer-expired", adopted.Pet.Id); err != nil {
		t.Fatalf("GetPet() after expired attempt error = %v", err)
	}
}

func TestCompatibleLegacyMissingPetFinalizesWithoutReceipt(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	owner := "peer-legacy"
	descriptor := petDeletionDescriptor{
		OwnerPublicKey: owner, PetID: "pet-legacy", RuntimeProfile: "profile-legacy",
		PetDefID: "petdef-legacy", WorkspaceID: "workspace-legacy",
	}
	record, err := pendingdeletion.New(pendingdeletion.KindPet, descriptor.PetID, &owner, pendingdeletion.ReasonResourceDelete, descriptor, *now)
	if err != nil {
		t.Fatal(err)
	}
	record.DeletionID = uuid.NewString()
	fingerprint, err := pendingdeletion.Fingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	descriptorJSON, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_pending_deletions (
		deletion_id, kind, owner_public_key, resource_id, reason, deleted_at,
		descriptor_version, descriptor_json, marker_fingerprint, task_status,
		task_phase, failure_count, next_attempt_at, lease_token, lease_deadline,
		last_error_code, last_error_message, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, '', '', '', '', ?)`,
		record.DeletionID, record.Kind, owner, record.ResourceID, record.Reason, formatTime(record.DeletedAt),
		record.DescriptorVersion, string(descriptorJSON), fingerprint, pendingdeletion.StatusQueued,
		pendingdeletion.PhaseValidate, formatTime(record.DeletedAt), formatTime(record.DeletedAt)); err != nil {
		t.Fatal(err)
	}
	source := PendingDeletionSource{DB: runtime.DB}
	refs, _, err := source.ScanDue(ctx, now.Add(time.Second), 1, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now.Add(time.Second), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	if err := (PetDeletionHandler{DB: runtime.DB, Now: func() time.Time { return now.Add(time.Second) }}).Handle(ctx, claim); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := source.GetTask(ctx, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("GetTask() error = %v, want ErrNotFound", err)
	}
}

func TestMalformedStoredMarkerBecomesTerminalInsteadOfBlockingSource(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	owner := "peer-malformed"
	record := pendingdeletion.Record{
		DeletionID: uuid.NewString(), Kind: pendingdeletion.KindPet, ResourceID: "pet-malformed",
		Reason: pendingdeletion.ReasonResourceDelete, DeletedAt: *now, OwnerPublicKey: &owner,
		DescriptorVersion: pendingdeletion.DescriptorVersion, Descriptor: []byte(`{`),
	}
	fingerprint, err := pendingdeletion.StoredFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_pending_deletions (
		deletion_id, kind, owner_public_key, resource_id, reason, deleted_at,
		descriptor_version, descriptor_json, marker_fingerprint, task_status,
		task_phase, failure_count, next_attempt_at, lease_token, lease_deadline,
		last_error_code, last_error_message, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, '', '', '', '', ?)`,
		record.DeletionID, record.Kind, owner, record.ResourceID, record.Reason, formatTime(record.DeletedAt),
		record.DescriptorVersion, string(record.Descriptor), fingerprint, pendingdeletion.StatusQueued,
		pendingdeletion.PhaseValidate, formatTime(record.DeletedAt), formatTime(record.DeletedAt)); err != nil {
		t.Fatal(err)
	}
	registry := pendingdeletion.NewRegistry()
	source := PendingDeletionSource{DB: runtime.DB}
	if err := registry.Register(source, PetDeletionHandler{DB: runtime.DB}); err != nil {
		t.Fatal(err)
	}
	processor, err := pendingdeletion.NewProcessor(registry, pendingdeletion.Config{
		ScanInterval: 5 * time.Millisecond, PageSize: 1, DispatchCapacity: 1, Workers: 1,
		LeaseDuration: time.Second, AttemptTimeout: 500 * time.Millisecond,
		RetryInitial: time.Millisecond, RetryMax: time.Second, MaxAttempts: 3,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	processor.Start(ctx)
	t.Cleanup(processor.Close)
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := source.GetTask(ctx, record.DeletionID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == pendingdeletion.StatusFailed {
			if task.LastErrorCode != "invalid_task_state" {
				t.Fatalf("failed task = %#v", task)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("malformed marker remained in status %q", task.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
