package workspace

import (
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
)

func TestWorkspaceSQLDeletionLifecycle(t *testing.T) {
	db, _ := sqltest.New(t)
	testWorkspaceSQLDeletionLifecycle(t, db)
}
func TestWorkspaceSQLPostgresDeletionLifecycle(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	testWorkspaceSQLDeletionLifecycle(t, workspacePostgresTestDB(t, dsn))
}

func testWorkspaceSQLDeletionLifecycle(t *testing.T, db *sqlx.DB) {
	ctx := t.Context()
	if err := Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	target := sqlWorkspaceFixture("target", "owner", "target")
	foreign := sqlWorkspaceFixture("foreign", "owner", "foreign")
	for _, item := range []struct{ id, name string }{{target.Id, target.Name}, {foreign.Id, foreign.Name}} {
		if err := createSQLWorkspace(ctx, db, sqlWorkspaceFixture(item.id, "owner", item.name)); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	record, err := pendingdeletion.New(pendingdeletion.KindWorkspace, target.Id, target.OwnerPublicKey, pendingdeletion.ReasonResourceDelete, workspaceDeletionDescriptor{ID: target.Id, Name: target.Name, OwnerPublicKey: target.OwnerPublicKey}, now)
	if err != nil {
		t.Fatal(err)
	}
	source := workspaceSQLDeletionSource{DB: db}
	wrong, err := pendingdeletion.New(pendingdeletion.KindWorkspace, target.Id, target.OwnerPublicKey, pendingdeletion.ReasonResourceDelete, workspaceDeletionDescriptor{ID: target.Id, Name: "wrong-name", OwnerPublicKey: target.OwnerPublicKey}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := source.CreateOrGet(ctx, wrong); !errors.Is(err, errWorkspaceSQLConflict) {
		t.Fatalf("mismatched descriptor = %v", err)
	}
	// A caller-owned transaction can roll back the marker and record state together.
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := createWorkspaceDeletionTx(ctx, tx, record); err != nil || !created {
		tx.Rollback()
		t.Fatalf("transactional create=%v,%v", created, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := source.GetByResource(ctx, target.Id); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("rolled back marker=%v", err)
	}
	var pending sql.NullString
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT pending_deletion_id FROM workspaces WHERE id=?`), target.Id).Scan(&pending); err != nil || pending.Valid {
		t.Fatalf("rolled back state=%v,error=%v", pending, err)
	}
	var workers sync.WaitGroup
	createdIDs := make(chan string, 8)
	for range 8 {
		workers.Go(func() {
			stored, created, err := source.CreateOrGet(ctx, record)
			if err != nil {
				t.Error(err)
				return
			}
			if stored.DeletionID != record.DeletionID {
				t.Error("concurrent marker identity changed")
			}
			if created {
				createdIDs <- stored.DeletionID
			}
		})
	}
	workers.Wait()
	close(createdIDs)
	if len(createdIDs) != 1 {
		t.Fatalf("created tasks=%d, want 1", len(createdIDs))
	}
	task, err := source.GetTask(ctx, record.DeletionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := pendingdeletion.ValidateTask(task); err != nil {
		t.Fatal(err)
	}
	if exists, err := source.HasLocator(ctx, pendingdeletion.Locator{Kind: record.Kind, ResourceID: target.Id, OwnerPublicKey: target.OwnerPublicKey}); err != nil || !exists {
		t.Fatalf("locator=%v,%v", exists, err)
	}
	refs, _, err := source.ScanDue(ctx, now, 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("due refs=%v,error=%v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now, time.Second)
	if err != nil || !claimed {
		t.Fatalf("claim=%v,error=%v", claimed, err)
	}
	claim, err = source.Checkpoint(ctx, claim, pendingdeletion.PhaseFinalize, now)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(2 * time.Second)
	if err := source.Renew(ctx, claim, later, time.Second); !errors.Is(err, pendingdeletion.ErrConflict) {
		t.Fatalf("expired renewal=%v", err)
	}
	replacement, claimed, err := source.Claim(ctx, refs[0], later, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("reclaim=%v,error=%v", claimed, err)
	}
	if err := source.Finalize(ctx, claim, later); !errors.Is(err, pendingdeletion.ErrConflict) {
		t.Fatalf("stale finalization=%v", err)
	}
	_, version, err := getSQLWorkspaceByID(ctx, db, target.Id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE workspaces SET incarnation='replacement' WHERE id=?`), target.Id); err != nil {
		t.Fatal(err)
	}
	if err := source.Finalize(ctx, replacement, later); !errors.Is(err, pendingdeletion.ErrConflict) {
		t.Fatalf("replacement finalization=%v", err)
	}
	var labelCount int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT count(*) FROM workspace_labels WHERE workspace_id=?`), target.Id).Scan(&labelCount); err != nil || labelCount != 1 {
		t.Fatalf("replacement labels=%d,error=%v", labelCount, err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE workspaces SET incarnation=? WHERE id=?`), version.incarnation, target.Id); err != nil {
		t.Fatal(err)
	}
	if err := source.Finalize(ctx, replacement, later); err != nil {
		t.Fatal(err)
	}
	if _, _, err := getSQLWorkspaceByID(ctx, db, target.Id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("final record=%v", err)
	}
	if _, err := source.GetTask(ctx, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("final task=%v", err)
	}
	if _, _, err := getSQLWorkspaceByID(ctx, db, foreign.Id); err != nil {
		t.Fatalf("foreign Workspace changed: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM workspace_labels`).Scan(&labelCount); err != nil || labelCount != 1 {
		t.Fatalf("remaining labels=%d,error=%v", labelCount, err)
	}
	// Ownerless Workspaces preserve SQL NULL through task persistence.
	ownerless := sqlWorkspaceFixture("ownerless", "", "ownerless")
	ownerless.System = new(true)
	if err := createSQLWorkspace(ctx, db, ownerless); err != nil {
		t.Fatal(err)
	}
	record, err = pendingdeletion.New(pendingdeletion.KindWorkspace, ownerless.Id, nil, pendingdeletion.ReasonFriendGroupDelete, socialRetirementDescriptor{ID: ownerless.Id, Name: ownerless.Name, WorkspaceKind: socialutil.SFUWorkspaceKindFriendGroup, SocialResourceID: "group"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := source.CreateOrGet(ctx, record); err != nil {
		t.Fatal(err)
	}
	loaded, err := source.GetByResource(ctx, ownerless.Id)
	if err != nil || loaded.OwnerPublicKey != nil {
		t.Fatalf("ownerless record=%+v,error=%v", loaded, err)
	}
}
