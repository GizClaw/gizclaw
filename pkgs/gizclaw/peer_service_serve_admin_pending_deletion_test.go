package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestAdminPendingDeletionListGetAndRetry(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Open("sqlite", "file:admin-pending-deletion?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlx.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	runtime := &gameplay.Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	descriptor := map[string]string{
		"owner_public_key": "owner-secret", "pet_id": "pet-a",
		"runtime_profile_id": "profile-a", "pet_def_id": "petdef-a", "workspace_id": "workspace-a",
	}
	record, err := pendingdeletion.New(pendingdeletion.KindPet, "pet-a", new("owner-secret"), pendingdeletion.ReasonResourceDelete, descriptor, now)
	if err != nil {
		t.Fatalf("pendingdeletion.New() error = %v", err)
	}
	fingerprint, err := pendingdeletion.Fingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gameplay_pending_deletions (
		deletion_id, kind, owner_public_key, resource_id, reason, deleted_at, descriptor_version,
		descriptor_json, marker_fingerprint, task_status, task_phase, failure_count,
		next_attempt_at, lease_token, lease_deadline, last_error_code, last_error_message, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, '', '', '', '', ?)`,
		record.DeletionID, record.Kind, "owner-secret", record.ResourceID, record.Reason,
		now.Format(time.RFC3339Nano), record.DescriptorVersion, string(record.Descriptor), fingerprint,
		pendingdeletion.StatusQueued, pendingdeletion.PhaseValidate, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	source := gameplay.PendingDeletionSource{DB: db}
	registry := pendingdeletion.NewRegistry()
	if err := registry.Register(source, gameplay.PetDeletionHandler{DB: db}); err != nil {
		t.Fatal(err)
	}
	service := &adminService{PendingDeletions: pendingdeletion.NewAdmin(registry, nil)}
	listResponse, err := service.ListPendingDeletions(ctx, adminhttp.ListPendingDeletionsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := listResponse.(adminhttp.ListPendingDeletions200JSONResponse)
	if !ok || len(list.Items) != 1 || list.Items[0].ResourceId != "pet-a" {
		t.Fatalf("ListPendingDeletions() = %#v", listResponse)
	}
	encoded, err := json.Marshal(list.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("owner-secret")) || bytes.Contains(encoded, record.Descriptor) || bytes.Contains(encoded, []byte(fingerprint)) {
		t.Fatalf("pending deletion projection leaked private marker fields: %s", encoded)
	}
	getResponse, err := service.GetPendingDeletion(ctx, adminhttp.GetPendingDeletionRequestObject{
		DeletionId: uuid.MustParse(record.DeletionID), Params: adminhttp.GetPendingDeletionParams{Source: "gameplay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := getResponse.(adminhttp.GetPendingDeletion200JSONResponse); !ok {
		t.Fatalf("GetPendingDeletion() = %#v", getResponse)
	}
	retryResponse, err := service.RetryPendingDeletion(ctx, adminhttp.RetryPendingDeletionRequestObject{
		DeletionId: uuid.MustParse(record.DeletionID), Params: adminhttp.RetryPendingDeletionParams{Source: "gameplay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := retryResponse.(adminhttp.RetryPendingDeletion409JSONResponse); !ok {
		t.Fatalf("RetryPendingDeletion(queued) = %#v", retryResponse)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gameplay_pending_deletions SET task_status = 'failed' WHERE deletion_id = ?`, record.DeletionID); err != nil {
		t.Fatal(err)
	}
	retryResponse, err = service.RetryPendingDeletion(ctx, adminhttp.RetryPendingDeletionRequestObject{
		DeletionId: uuid.MustParse(record.DeletionID), Params: adminhttp.RetryPendingDeletionParams{Source: "gameplay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, ok := retryResponse.(adminhttp.RetryPendingDeletion200JSONResponse)
	if !ok || queued.Status != apitypes.PendingDeletionStatus(pendingdeletion.StatusQueued) {
		t.Fatalf("RetryPendingDeletion(failed) = %#v", retryResponse)
	}
}
