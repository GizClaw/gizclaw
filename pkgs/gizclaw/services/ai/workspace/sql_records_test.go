package workspace

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
	"github.com/jmoiron/sqlx"
)

func sqlWorkspaceFixture(id, owner, name string) apitypes.Workspace {
	now := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	item := apitypes.Workspace{Id: id, Name: name, WorkflowId: "workflow", CreatedAt: now, UpdatedAt: now, LastActiveAt: now, System: new(false), Labels: new(map[string]string{"kind": "chat"})}
	if owner != "" {
		item.OwnerPublicKey = &owner
	}
	return item
}

func TestWorkspaceSQLIdentityAndConditionalMutation(t *testing.T) {
	db, _ := sqltest.New(t)
	testWorkspaceSQLIdentityAndConditionalMutation(t, db)
}

func testWorkspaceSQLIdentityAndConditionalMutation(t *testing.T, db *sqlx.DB) {
	ctx := t.Context()
	if err := Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	original := sqlWorkspaceFixture("workspace", "owner", "same-name")
	if err := createSQLWorkspace(ctx, db, original); err != nil {
		t.Fatal(err)
	}
	read, version, err := getSQLWorkspaceByName(ctx, db, original.OwnerPublicKey, original.Name)
	if err != nil || !reflect.DeepEqual(read, original) {
		t.Fatalf("read=%+v, error=%v", read, err)
	}
	// The same name is legal in independent owner scopes, including ownerless records.
	for _, item := range []apitypes.Workspace{sqlWorkspaceFixture("other", "other-owner", "same-name"), sqlWorkspaceFixture("ownerless", "", "same-name")} {
		if err := createSQLWorkspace(ctx, db, item); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []apitypes.Workspace{sqlWorkspaceFixture("duplicate", "owner", "same-name"), sqlWorkspaceFixture("duplicate-ownerless", "", "same-name"), sqlWorkspaceFixture("workspace", "another", "different-name")} {
		if err := createSQLWorkspace(ctx, db, item); !errors.Is(err, errWorkspaceSQLConflict) {
			t.Fatalf("duplicate=%v", err)
		}
	}
	changed := read
	changed.Labels = new(map[string]string{"kind": "new", "team": "blue"})
	changed.WorkflowId = "new-workflow"
	if err := updateSQLWorkspace(ctx, db, changed, version); err != nil {
		t.Fatal(err)
	}
	if err := updateSQLWorkspace(ctx, db, original, version); !errors.Is(err, errWorkspaceSQLConflict) {
		t.Fatalf("stale update=%v", err)
	}
	current, currentVersion, err := getSQLWorkspaceByID(ctx, db, original.Id)
	if err != nil || current.WorkflowId != "new-workflow" || !reflect.DeepEqual(current.Labels, changed.Labels) {
		t.Fatalf("updated=%+v, error=%v", current, err)
	}
	var labelCount int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT count(*) FROM workspace_labels WHERE workspace_id=? AND ((label_key='kind' AND label_value='new') OR (label_key='team' AND label_value='blue'))`), original.Id).Scan(&labelCount); err != nil || labelCount != 2 {
		t.Fatalf("labels=%d,error=%v", labelCount, err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE workspaces SET pending_deletion_id='deletion' WHERE id=?`), original.Id); err != nil {
		t.Fatal(err)
	}
	if err := updateSQLWorkspace(ctx, db, current, currentVersion); !errors.Is(err, errWorkspaceSQLConflict) {
		t.Fatalf("retired update=%v", err)
	}
	// Explicit child deletion keeps this fixture valid even when SQLite foreign keys are disabled.
	if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM workspace_labels WHERE workspace_id=?`), original.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM workspaces WHERE id=?`), original.Id); err != nil {
		t.Fatal(err)
	}
	if err := createSQLWorkspace(ctx, db, original); err != nil {
		t.Fatal(err)
	}
	if err := updateSQLWorkspace(ctx, db, changed, currentVersion); !errors.Is(err, errWorkspaceSQLConflict) {
		t.Fatalf("recreated record update=%v", err)
	}
	current, version, err = getSQLWorkspaceByID(ctx, db, original.Id)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	winners := make(chan string, 12)
	for i := range 12 {
		workers.Go(func() {
			update := current
			value := fmt.Sprintf("winner-%d", i)
			update.WorkflowId = value
			update.Labels = new(map[string]string{"winner": value})
			err := updateSQLWorkspace(ctx, db, update, version)
			if err == nil {
				winners <- value
			} else if !errors.Is(err, errWorkspaceSQLConflict) {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	close(winners)
	if len(winners) != 1 {
		t.Fatalf("concurrent winners=%d, want 1", len(winners))
	}
	winner := <-winners
	current, _, err = getSQLWorkspaceByID(ctx, db, original.Id)
	if err != nil || current.WorkflowId != winner || (*current.Labels)["winner"] != winner {
		t.Fatalf("concurrent result=%+v,error=%v", current, err)
	}
	var indexedWinner string
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT label_value FROM workspace_labels WHERE workspace_id=? AND label_key='winner'`), original.Id).Scan(&indexedWinner); err != nil || indexedWinner != winner {
		t.Fatalf("indexed winner=%s,error=%v", indexedWinner, err)
	}

}

func TestWorkspaceSQLFiltersAndPaginationBoundReads(t *testing.T) {
	db, observer := sqltest.New(t)
	ctx := t.Context()
	if err := Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	for i := range 1000 {
		item := sqlWorkspaceFixture(fmt.Sprintf("workspace-%04d", i), "other-owner", fmt.Sprintf("name-%04d", i))
		if i%10 == 0 {
			item.OwnerPublicKey = new("owner")
		}
		if i%20 == 0 {
			item.Labels = new(map[string]string{"kind": "chat", "team": "blue"})
		}
		if i%100 == 0 {
			item.System = new(true)
		}
		if err := createSQLWorkspace(ctx, db, item); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspaces SET pending_deletion_id='deletion' WHERE id='workspace-0020'`); err != nil {
		t.Fatal(err)
	}
	// Unrelated configuration must never be fetched or decoded for this owner query.
	if _, err := db.ExecContext(ctx, `UPDATE workspaces SET parameters_json='invalid' WHERE owner_public_key='other-owner'`); err != nil {
		t.Fatal(err)
	}
	observer.Reset()
	observer.Delay.Store(int64(10 * time.Millisecond))
	filter := workspaceSQLFilter{owner: new("owner"), ordinaryOnly: true, activeOnly: true, labels: map[string]string{"kind": "chat", "team": "blue"}}
	items, more, next, err := listSQLWorkspaces(ctx, db, filter, "", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7 || !more || next == nil || *next != items[6].Id {
		t.Fatalf("page=%d more=%v next=%v", len(items), more, next)
	}
	if observer.Statements.Load() != 1 || observer.Rows.Load() != 8 {
		t.Fatalf("queries=%d rows=%d", observer.Statements.Load(), observer.Rows.Load())
	}
	for _, item := range items {
		if item.Id == "workspace-0020" || *item.OwnerPublicKey != "owner" || *item.System || (*item.Labels)["team"] != "blue" {
			t.Fatalf("foreign filtered record=%+v", item)
		}
	}
	observer.Reset()
	second, _, _, err := listSQLWorkspaces(ctx, db, filter, *next, 7)
	if err != nil || len(second) != 7 || second[0].Id <= *next {
		t.Fatalf("second page=%+v,error=%v", second, err)
	}
	if observer.Statements.Load() != 1 || observer.Rows.Load() != 8 {
		t.Fatalf("second queries=%d rows=%d", observer.Statements.Load(), observer.Rows.Load())
	}
}

func TestWorkspaceSQLLabelFailureRollsBackRecord(t *testing.T) {
	db, _ := sqltest.New(t)
	ctx := t.Context()
	if err := Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	original := sqlWorkspaceFixture("workspace", "owner", "name")
	if err := createSQLWorkspace(ctx, db, original); err != nil {
		t.Fatal(err)
	}
	_, version, err := getSQLWorkspaceByID(ctx, db, original.Id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER reject_label BEFORE INSERT ON workspace_labels WHEN NEW.label_value='reject' BEGIN SELECT RAISE(FAIL,'rejected label'); END`); err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.WorkflowId = "new-workflow"
	changed.Labels = new(map[string]string{"kind": "reject"})
	if err := updateSQLWorkspace(ctx, db, changed, version); err == nil {
		t.Fatal("label failure did not reject update")
	}
	current, currentVersion, err := getSQLWorkspaceByID(ctx, db, original.Id)
	if err != nil || !reflect.DeepEqual(current, original) || currentVersion != version {
		t.Fatalf("failed update changed record=%+v,version=%+v,error=%v", current, currentVersion, err)
	}
	var label string
	if err := db.QueryRowContext(ctx, `SELECT label_value FROM workspace_labels WHERE workspace_id=? AND label_key='kind'`, original.Id).Scan(&label); err != nil || label != "chat" {
		t.Fatalf("failed update changed index=%s,error=%v", label, err)
	}
}

func TestWorkspaceActivityRejectsRetirementAndPreservesMonotonicTime(t *testing.T) {
	db, _ := sqltest.New(t)
	testWorkspaceActivityRetirement(t, db)
}

func testWorkspaceActivityRetirement(t *testing.T, db *sqlx.DB) {
	ctx := t.Context()
	if err := Initialize(ctx, db); err != nil {
		t.Fatal(err)
	}
	item := sqlWorkspaceFixture("activity", "owner", "activity")
	if err := createSQLWorkspace(ctx, db, item); err != nil {
		t.Fatal(err)
	}
	_, original, err := getSQLWorkspaceByID(ctx, db, item.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{item.LastActiveAt, item.LastActiveAt.Add(-time.Hour)} {
		if err := bumpSQLWorkspaceActivity(ctx, db, item.Id, at); err != nil {
			t.Fatal(err)
		}
	}
	read, version, err := getSQLWorkspaceByID(ctx, db, item.Id)
	if err != nil || !read.LastActiveAt.Equal(item.LastActiveAt) || version != original {
		t.Fatalf("no-op activity changed record: %+v %+v %v", read, version, err)
	}
	if _, err := db.Exec(`UPDATE workspaces SET pending_deletion_id='retiring' WHERE id='activity'`); err != nil {
		t.Fatal(err)
	}
	if err := bumpSQLWorkspaceActivity(ctx, db, item.Id, item.LastActiveAt.Add(time.Hour)); !errors.Is(err, errWorkspaceSQLConflict) {
		t.Fatalf("retired activity = %v", err)
	}
	if _, err := db.Exec(`DELETE FROM workspaces WHERE id='activity'`); err != nil {
		t.Fatal(err)
	}
	if err := bumpSQLWorkspaceActivity(ctx, db, item.Id, item.LastActiveAt); !errors.Is(err, errWorkspaceSQLConflict) {
		t.Fatalf("deleted activity = %v", err)
	}
}
