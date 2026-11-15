package friend

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type cancellationSQLWorkflow struct{}

func (cancellationSQLWorkflow) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	var value apitypes.Workflow
	if err := json.Unmarshal([]byte(`{"id":"system-sfu","spec":{"driver":"sfu","sfu":{}}}`), &value); err != nil {
		return nil, err
	}
	return adminhttp.GetWorkflow200JSONResponse(value), nil
}

func cancellationSQLWorkspace(t *testing.T) *workspace.Server {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "workspace.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := workspace.Initialize(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return &workspace.Server{DB: db, Workflows: cancellationSQLWorkflow{}}
}

func assertCancellationSQLRows(t *testing.T, workspaces *workspace.Server, name string, want int) {
	t.Helper()
	var count int
	if err := workspaces.DB.GetContext(t.Context(), &count, `SELECT COUNT(*) FROM workspaces WHERE name=?`, name); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("exact SQL Workspace rows = %d, want %d", count, want)
	}
}

// These wire types are the fields written by cab4785b. In particular, neither
// the cancellation handoff, its receipt, nor its decision contains an owner or
// region; the creation intent was the sole shared record containing the owner.
type baseFriendCancellationIntent struct {
	RelationID     string       `json:"relation_id"`
	FirstPeer      string       `json:"first_peer"`
	SecondPeer     string       `json:"second_peer"`
	WorkspaceID    string       `json:"workspace_id"`
	WorkspaceName  string       `json:"workspace_name"`
	Relationship   friendRecord `json:"relationship"`
	DeletedAt      time.Time    `json:"deleted_at"`
	CancelCreation bool         `json:"cancel_creation,omitempty"`
}

type baseFriendCancellationReceipt struct {
	RelationID     string    `json:"relation_id"`
	FirstPeer      string    `json:"first_peer"`
	SecondPeer     string    `json:"second_peer"`
	WorkspaceID    string    `json:"workspace_id"`
	WorkspaceName  string    `json:"workspace_name"`
	DeletedAt      time.Time `json:"deleted_at"`
	CancelCreation bool      `json:"cancel_creation,omitempty"`
}

func baseFriendCancellationBoundary(t *testing.T, s *Server, sqlWorkspaces *workspace.Server, owner, boundary string) (creationIntent, retirementIntent, []byte) {
	t.Helper()
	ctx := t.Context()
	s.Workspaces = sqlWorkspaces
	creation, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ensureCreationWorkspace(ctx, creation); err != nil {
		t.Fatal(err)
	}
	legacy := baseFriendCancellationIntent{RelationID: creation.RelationID, FirstPeer: creation.FirstPeer, SecondPeer: creation.SecondPeer, WorkspaceName: creation.Workspace, DeletedAt: s.now(), CancelCreation: true}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := json.Marshal(struct {
		RelationID    string `json:"relation_id"`
		IncarnationID string `json:"incarnation_id"`
		Workspace     string `json:"workspace_name"`
		State         string `json:"state"`
	}{creation.RelationID, creation.IncarnationID, creation.Workspace, "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Friends.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: creationDecisionKey(creation.RelationID, creation.IncarnationID)}}, Entries: []kv.Entry{{Key: creationDecisionKey(creation.RelationID, creation.IncarnationID), Value: decision}, {Key: retirementIntentKey(creation.RelationID), Value: data}}, AddMembers: (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Add(creation.RelationID)}); err != nil {
		t.Fatal(err)
	}
	// cab4785b deleted this shared identity before entering SQL cleanup.
	if err := deleteCreationIntent(ctx, s.Friends, creation); err != nil {
		t.Fatal(err)
	}
	var pending retirementIntent
	if err := json.Unmarshal(data, &pending); err != nil {
		t.Fatal(err)
	}
	if boundary == "before-sql" {
		return creation, pending, data
	}
	if _, err := sqlWorkspaces.DeleteSystemWorkspace(ownership.WithOwner(ctx, owner), creation.Workspace); err != nil {
		t.Fatal(err)
	}
	var tasks int
	if err := sqlWorkspaces.DB.GetContext(ctx, &tasks, `SELECT COUNT(*) FROM workspace_pending_deletions`); err != nil || tasks != 0 {
		t.Fatalf("base direct cancellation unexpectedly retained SQL tasks=%d err=%v", tasks, err)
	}
	if boundary == "after-sql" {
		return creation, pending, data
	}
	receipt, err := json.Marshal(baseFriendCancellationReceipt{RelationID: creation.RelationID, FirstPeer: creation.FirstPeer, SecondPeer: creation.SecondPeer, WorkspaceName: creation.Workspace, DeletedAt: pending.DeletedAt, CancelCreation: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Friends.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: retirementIntentKey(creation.RelationID), Expected: data}}, Entries: []kv.Entry{{Key: retirementReceiptKey(creation.RelationID), Value: receipt}}, DeleteKeys: []kv.Key{retirementIntentKey(creation.RelationID)}, RemoveMembers: (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Remove(creation.RelationID)}); err != nil {
		t.Fatal(err)
	}
	return creation, pending, receipt
}

func TestFriendBaseCancellationCrashAfterSQLRetainsUnprovableWork(t *testing.T) {
	for _, owner := range []string{"owner", "peer"} {
		t.Run(owner, func(t *testing.T) {
			ctx := t.Context()
			s := newTestServer()
			sqlWorkspaces := cancellationSQLWorkspace(t)
			creation, _, raw := baseFriendCancellationBoundary(t, s, sqlWorkspaces, owner, "after-sql")
			// The base retry's DeleteSystemWorkspace also returned raw ErrNoRows;
			// completeFriendRetirement only tolerated kv.ErrNotFound, so this
			// information-loss boundary already prevented base completion.
			if _, err := sqlWorkspaces.DeleteSystemWorkspace(ownership.WithOwner(ctx, owner), creation.Workspace); err != sql.ErrNoRows {
				t.Fatalf("base physical-delete retry sentinel=%v", err)
			}
			beforeDecision, err := s.Friends.Get(ctx, creationDecisionKey(creation.RelationID, creation.IncarnationID))
			if err != nil {
				t.Fatal(err)
			}
			for _, sqlRegion := range []*workspace.Server{sqlWorkspaces, cancellationSQLWorkspace(t)} {
				restarted := newTestServer()
				restarted.Friends, restarted.Workspaces = s.Friends, sqlRegion
				if err := restarted.ReconcileRetirementIntents(ctx); !errors.Is(err, errLegacyCancellationEvidence) || !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("ownerless crash invented regional completion: %v", err)
				}
				stored, err := s.Friends.Get(ctx, retirementIntentKey(creation.RelationID))
				if err != nil || !bytes.Equal(stored, raw) {
					t.Fatalf("unprovable base handoff changed: %q err=%v", stored, err)
				}
				if _, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
					t.Fatalf("absence published a completion receipt: %v", err)
				}
			}
			if _, err := readCreationIntent(ctx, s.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("fixture retained creation evidence: %v", err)
			}
			afterDecision, err := s.Friends.Get(ctx, creationDecisionKey(creation.RelationID, creation.IncarnationID))
			if err != nil || !bytes.Equal(beforeDecision, afterDecision) {
				t.Fatalf("ownerless decision changed=%q err=%v", afterDecision, err)
			}
			var ids []string
			for id, err := range (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).IDs(ctx, s.Friends) {
				if err != nil {
					t.Fatal(err)
				}
				ids = append(ids, id)
			}
			if !slices.Contains(ids, creation.RelationID) {
				t.Fatalf("unprovable work lost its recovery index: %v", ids)
			}
			assertCancellationSQLRows(t, sqlWorkspaces, creation.Workspace, 0)
		})
	}
}

func TestFriendBaseCancellationRecoversAfterExactRegionalSQLBackupRestore(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	originalSQL, foreignSQL := cancellationSQLWorkspace(t), cancellationSQLWorkspace(t)
	creation, pending, raw := baseFriendCancellationBoundary(t, s, originalSQL, "peer", "before-sql")
	// This backup's provenance is the exact original regional SQL connection,
	// captured before physical deletion. Peer ordering/home routing supplies no
	// provenance. Preserve every SQL field, including its separate incarnation.
	const columns = "id,owner_public_key,name,workflow_id,system,created_at,updated_at,last_active_at,parameters_json,toolkit_json,icon_json,labels_json,incarnation,revision,pending_deletion_id"
	backup := make(map[string]any)
	if err := originalSQL.DB.QueryRowxContext(ctx, "SELECT "+columns+" FROM workspaces WHERE id=?", creation.Workspace).MapScan(backup); err != nil {
		t.Fatal(err)
	}
	if backup["id"] != creation.Workspace || backup["name"] != creation.Workspace || backup["owner_public_key"] != "peer" || backup["incarnation"] == "" || backup["pending_deletion_id"] != nil {
		t.Fatalf("backup did not retain the exact original identity: %+v", backup)
	}
	assetRoot, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = assetRoot.Close() })
	assets, err := objectstore.NewRoot(assetRoot)
	if err != nil {
		t.Fatal(err)
	}
	originalSQL.Assets = assets
	artifactName := iconasset.ObjectName(creation.Workspace, iconasset.FormatPNG)
	artifactBackup := []byte("original regional Workspace artifact")
	if err := assets.Put(artifactName, bytes.NewReader(artifactBackup)); err != nil {
		t.Fatal(err)
	}
	if _, err := originalSQL.DeleteSystemWorkspace(ownership.WithOwner(ctx, "peer"), creation.Workspace); err != nil {
		t.Fatal(err)
	}
	assertCancellationSQLRows(t, originalSQL, creation.Workspace, 0)
	if items, err := assets.List(""); err != nil || len(items) != 0 {
		t.Fatalf("base delete retained artifacts=%v err=%v", items, err)
	}
	foreign := newTestServer()
	foreign.Friends, foreign.Workspaces = s.Friends, foreignSQL
	for _, worker := range []*Server{s, foreign} {
		if err := worker.ReconcileRetirementIntents(ctx); !errors.Is(err, errLegacyCancellationEvidence) {
			t.Fatalf("unrestored crash auto-completed: %v", err)
		}
	}
	stored, err := s.Friends.Get(ctx, retirementIntentKey(creation.RelationID))
	if err != nil || !bytes.Equal(stored, raw) {
		t.Fatalf("missing-evidence handoff changed: %q err=%v", stored, err)
	}
	if _, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("unrestored crash published a receipt: %v", err)
	}
	// A normal creation is not a backup restore and may be permanently fenced.
	// Maintenance must preserve the original row instead of minting a new SQL
	// incarnation or clearing this owner admission fence.
	if _, err := originalSQL.DB.ExecContext(ctx, `UPDATE workspace_owner_state SET retired=1 WHERE owner_public_key=?`, "peer"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := originalSQL.CreateSystemWorkspace(ownership.WithOwner(ctx, "peer"), adminhttp.WorkspaceUpsert{Id: creation.Workspace, Name: creation.Workspace, WorkflowId: socialutil.SFUWorkflowID}); !errors.Is(err, ownership.ErrOwnerRetired) {
		t.Fatalf("normal admission bypassed the retired-owner fence: %v", err)
	}
	values := make([]any, 0, len(backup))
	for column := range strings.SplitSeq(columns, ",") {
		values = append(values, backup[column])
	}
	// Exclusive offline maintenance: this test runs no admission or recovery
	// writer for the relation during the checks and restore below. Establish the
	// matching cancelled decision, exact still-pending handoff and absent receipt
	// while those writers are quiesced; these checks are not an online lease.
	decision, err := readCreationDecision(ctx, s.Friends, creation)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCreationDecision(decision, creation); err != nil || decision.State != creationDecisionCancelled {
		t.Fatalf("maintenance lacks the matching cancellation decision: %+v err=%v", decision, err)
	}
	stored, err = s.Friends.Get(ctx, retirementIntentKey(creation.RelationID))
	if err != nil || !bytes.Equal(stored, raw) {
		t.Fatalf("maintenance handoff no longer matches the backup: %q err=%v", stored, err)
	}
	if _, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("maintenance cannot restore after a completion receipt: %v", err)
	}
	// Restore only to the proven original regional DB. Plain INSERT fails on
	// any current ID/name collision and never overwrites an existing identity.
	const restore = "INSERT INTO workspaces(" + columns + ") VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	if _, err := originalSQL.DB.ExecContext(ctx, restore, values...); err != nil {
		t.Fatal(err)
	}
	if _, err := originalSQL.DB.ExecContext(ctx, restore, values...); err == nil {
		t.Fatal("maintenance restore overwrote an existing row")
	}
	restored := make(map[string]any)
	if err := originalSQL.DB.QueryRowxContext(ctx, "SELECT "+columns+" FROM workspaces WHERE id=?", creation.Workspace).MapScan(restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, backup) {
		t.Fatalf("maintenance changed backed-up identity: got=%+v want=%+v", restored, backup)
	}
	if err := assets.Put(artifactName, bytes.NewReader(artifactBackup)); err != nil {
		t.Fatal(err)
	}
	// End exclusive maintenance before resuming either region's reconciler.
	assertCancellationSQLRows(t, foreignSQL, creation.Workspace, 0)
	if err := foreign.ReconcileRetirementIntents(ctx); !errors.Is(err, errLegacyCancellationEvidence) {
		t.Fatalf("foreign absence consumed restored regional work: %v", err)
	}
	assertCancellationSQLRows(t, originalSQL, creation.Workspace, 1)
	if err := s.ReconcileRetirementIntents(ctx); err != nil {
		t.Fatal(err)
	}
	assertCancellationSQLRows(t, originalSQL, creation.Workspace, 0)
	if items, err := assets.List(""); err != nil || len(items) != 0 {
		t.Fatalf("reconciliation retained restored artifacts=%v err=%v", items, err)
	}
	receipt, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID)
	if err != nil || !receipt.CancelCreation || receipt.WorkspaceOwner != backup["owner_public_key"] || receipt.WorkspaceName != backup["name"] || !receipt.DeletedAt.Equal(pending.DeletedAt) {
		t.Fatalf("restored cleanup receipt=%+v err=%v", receipt, err)
	}
	if _, err := readRetirementIntent(ctx, s.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("restored cleanup remained pending: %v", err)
	}
	var retired int
	if err := originalSQL.DB.GetContext(ctx, &retired, `SELECT retired FROM workspace_owner_state WHERE owner_public_key=?`, "peer"); err != nil || retired != 1 {
		t.Fatalf("maintenance cleared the owner fence=%d err=%v", retired, err)
	}
}

func TestFriendBaseCancellationBeforeSQLRejectsForeignAbsenceAndRecoversLocally(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	localSQL := cancellationSQLWorkspace(t)
	creation, pending, _ := baseFriendCancellationBoundary(t, s, localSQL, "peer", "before-sql")
	foreign := newTestServer()
	foreign.Friends, foreign.Workspaces = s.Friends, cancellationSQLWorkspace(t)
	if err := foreign.completeFriendRetirement(ctx, foreign.Friends, pending); !errors.Is(err, errLegacyCancellationEvidence) {
		t.Fatalf("foreign SQL absence completed a base handoff: %v", err)
	}
	assertCancellationSQLRows(t, localSQL, creation.Workspace, 1)
	if err := s.ReconcileRetirementIntents(ctx); err != nil {
		t.Fatal(err)
	}
	assertCancellationSQLRows(t, localSQL, creation.Workspace, 0)
	receipt, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID)
	if err != nil || receipt.WorkspaceOwner != "peer" {
		t.Fatalf("locally recovered base receipt=%+v err=%v", receipt, err)
	}
}

func TestFriendBaseCompletedCancellationReceiptNeedsNoInventedOwner(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	sqlWorkspaces := cancellationSQLWorkspace(t)
	creation, pending, raw := baseFriendCancellationBoundary(t, s, sqlWorkspaces, "peer", "receipt")
	if err := s.completeFriendRetirement(ctx, s.Friends, pending); err != nil {
		t.Fatalf("old retirement worker could not acknowledge exact base receipt: %v", err)
	}
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, creation.RelationID, "peer", creation.Workspace); err != nil {
		t.Fatalf("old creator could not acknowledge exact base receipt: %v", err)
	}
	stored, err := s.Friends.Get(ctx, retirementReceiptKey(creation.RelationID))
	if err != nil || !bytes.Equal(raw, stored) {
		t.Fatalf("completed base receipt invented ownership=%q err=%v", stored, err)
	}
}

func TestFriendBaseReceiptCannotHideRetainedOtherOwnerWorkspace(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	creation, _, _ := baseFriendCancellationBoundary(t, s, cancellationSQLWorkspace(t), "peer", "receipt")
	copySQL := cancellationSQLWorkspace(t)
	s.Workspaces = copySQL
	if _, _, err := copySQL.CreateSystemWorkspace(ownership.WithOwner(ctx, "peer"), adminhttp.WorkspaceUpsert{Id: creation.Workspace, Name: creation.Workspace, WorkflowId: socialutil.SFUWorkflowID}); err != nil {
		t.Fatal(err)
	}
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, creation.RelationID, "owner", creation.Workspace); err == nil {
		t.Fatal("wrong owner hid an existing second-owner Workspace behind the legacy receipt")
	}
	assertCancellationSQLRows(t, copySQL, creation.Workspace, 1)
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, creation.RelationID, "peer", creation.Workspace); err != nil {
		t.Fatal(err)
	}
	assertCancellationSQLRows(t, copySQL, creation.Workspace, 0)
}

func TestFriendLegacyReceiptProofRefreshesOnlyOnDefinitiveRejection(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = root
	creation, _, _ := baseFriendCancellationBoundary(t, s, cancellationSQLWorkspace(t), "peer", "receipt")
	root.replicate = false
	replacement := retirementReceipt{RelationID: creation.RelationID, FirstPeer: creation.FirstPeer, SecondPeer: creation.SecondPeer, WorkspaceName: "different-cancelled-workspace", DeletedAt: s.now().Add(time.Second), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, root.Store, retirementReceiptKey(creation.RelationID), replacement); err != nil {
		t.Fatal(err)
	}
	root.primary = 0
	before := root.rejected
	if completed, err := s.completedLegacyFriendCancellation(ctx, root, creation.RelationID, creation.Workspace); err != nil || completed {
		t.Fatalf("stale legacy receipt acknowledged another completion: %v, %v", completed, err)
	}
	if root.rejected != before+1 || root.primary != 1 {
		t.Fatalf("receipt proof refresh rejected=%d primary=%d", root.rejected-before, root.primary)
	}
}

func TestFriendLegacyReceiptProofDoesNotReplayUnknownOutcome(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	root := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	s.Friends = root
	creation, _, _ := baseFriendCancellationBoundary(t, s, cancellationSQLWorkspace(t), "peer", "receipt")
	uncertain := errors.New("legacy cancellation receipt proof outcome unavailable")
	root.writeErr, root.primary = uncertain, 0
	before := root.mutations
	if completed, err := s.completedLegacyFriendCancellation(ctx, root, creation.RelationID, creation.Workspace); !errors.Is(err, uncertain) || completed {
		t.Fatalf("unknown receipt proof was acknowledged: %v, %v", completed, err)
	}
	if root.mutations != before+1 || root.primary != 0 {
		t.Fatalf("unknown receipt proof replayed: writes=%d primary=%d", root.mutations-before, root.primary)
	}
}

func TestFriendLegacyCancellationRecoversOwnerBeforeSQLDeletion(t *testing.T) {
	for _, creationPresent := range []bool{true, false} {
		t.Run(fmt.Sprintf("creation-present-%v", creationPresent), func(t *testing.T) {
			ctx := t.Context()
			s := newTestServer()
			sqlWorkspaces := cancellationSQLWorkspace(t)
			s.Workspaces = sqlWorkspaces
			creation, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", "peer")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ensureCreationWorkspace(ctx, creation); err != nil {
				t.Fatal(err)
			}
			if !creationPresent {
				if err := deleteCreationIntent(ctx, s.Friends, creation); err != nil {
					t.Fatal(err)
				}
			}
			legacy := retirementIntent{RelationID: creation.RelationID, FirstPeer: creation.FirstPeer, SecondPeer: creation.SecondPeer, WorkspaceName: creation.Workspace, DeletedAt: s.now(), CancelCreation: true}
			data, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Friends.ApplyMutation(ctx, kv.Mutation{Entries: []kv.Entry{{Key: retirementIntentKey(creation.RelationID), Value: data}}, AddMembers: (socialutil.RecoveryIndex{Root: retirementIntentsRoot}).Add(creation.RelationID)}); err != nil {
				t.Fatal(err)
			}
			if err := s.ReconcileRetirementIntents(ctx); err != nil {
				t.Fatal(err)
			}
			assertCancellationSQLRows(t, sqlWorkspaces, creation.Workspace, 0)
			receipt, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID)
			if err != nil || receipt.WorkspaceOwner != "peer" || receipt.WorkspaceName != creation.Workspace {
				t.Fatalf("legacy cleanup receipt=%+v err=%v", receipt, err)
			}
			if _, err := readRetirementIntent(ctx, s.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("legacy work remained=%v", err)
			}
		})
	}
}

func TestFriendLegacyCancellationForeignSQLAbsenceStaysPending(t *testing.T) {
	ctx := t.Context()
	local, foreign := newTestServer(), newTestServer()
	localSQL, foreignSQL := cancellationSQLWorkspace(t), cancellationSQLWorkspace(t)
	local.Workspaces, foreign.Workspaces, foreign.Friends = localSQL, foreignSQL, local.Friends
	creation, err := local.getOrCreateCreationIntent(ctx, local.Friends, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.ensureCreationWorkspace(ctx, creation); err != nil {
		t.Fatal(err)
	}
	if err := deleteCreationIntent(ctx, local.Friends, creation); err != nil {
		t.Fatal(err)
	}
	legacy := retirementIntent{RelationID: creation.RelationID, FirstPeer: creation.FirstPeer, SecondPeer: creation.SecondPeer, WorkspaceName: creation.Workspace, DeletedAt: local.now(), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, local.Friends, retirementIntentKey(creation.RelationID), legacy); err != nil {
		t.Fatal(err)
	}
	if err := foreign.completeFriendRetirement(ctx, foreign.Friends, legacy); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign absence completed legacy cleanup=%v", err)
	}
	current, err := readRetirementIntent(ctx, local.Friends, creation.RelationID)
	if err != nil || current.WorkspaceOwner != "" {
		t.Fatalf("foreign absence guessed owner=%+v err=%v", current, err)
	}
	if _, err := readRetirementReceipt(ctx, local.Friends, creation.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("foreign absence published receipt=%v", err)
	}
	assertCancellationSQLRows(t, localSQL, creation.Workspace, 1)
	if err := local.completeFriendRetirement(ctx, local.Friends, legacy); err != nil {
		t.Fatal(err)
	}
	assertCancellationSQLRows(t, localSQL, creation.Workspace, 0)
}

func TestFriendLegacyCancellationReceiptRecoversExactLocalOwner(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	sqlWorkspaces := cancellationSQLWorkspace(t)
	s.Workspaces = sqlWorkspaces
	creation, err := s.getOrCreateCreationIntent(ctx, s.Friends, "owner", "peer", "peer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ensureCreationWorkspace(ctx, creation); err != nil {
		t.Fatal(err)
	}
	if err := deleteCreationIntent(ctx, s.Friends, creation); err != nil {
		t.Fatal(err)
	}
	legacy := retirementReceipt{RelationID: creation.RelationID, FirstPeer: creation.FirstPeer, SecondPeer: creation.SecondPeer, WorkspaceName: creation.Workspace, DeletedAt: s.now(), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, s.Friends, retirementReceiptKey(creation.RelationID), legacy); err != nil {
		t.Fatal(err)
	}
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, creation.RelationID, "peer", creation.Workspace); err != nil {
		t.Fatal(err)
	}
	assertCancellationSQLRows(t, sqlWorkspaces, creation.Workspace, 0)
	receipt, err := readRetirementReceipt(ctx, s.Friends, creation.RelationID)
	if err != nil || receipt.WorkspaceOwner != "peer" {
		t.Fatalf("upgraded legacy receipt=%+v err=%v", receipt, err)
	}
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, creation.RelationID, "owner", creation.Workspace); err == nil {
		t.Fatal("receipt acknowledged the other pair owner's scope")
	}
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, creation.RelationID, "peer", creation.Workspace); err != nil {
		t.Fatal(err)
	}
}

func TestFriendLegacyReceiptWithoutIdentityAcknowledgesCompletedCancellation(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	s.Workspaces = cancellationSQLWorkspace(t)
	legacy := retirementReceipt{RelationID: "owner:peer", FirstPeer: "owner", SecondPeer: "peer", WorkspaceName: "old-cancelled-workspace", DeletedAt: s.now(), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, s.Friends, retirementReceiptKey(legacy.RelationID), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteFriend(ctx, "owner", rpcapi.FriendDeleteRequest{Name: "peer"}); err != nil {
		t.Fatalf("historical receipt rejected=%v", err)
	}
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, legacy.RelationID, "owner", legacy.WorkspaceName); err != nil {
		t.Fatalf("historical receipt did not acknowledge completed cancellation=%v", err)
	}
	current, err := readRetirementReceipt(ctx, s.Friends, legacy.RelationID)
	if err != nil || current.WorkspaceOwner != "" {
		t.Fatalf("historical receipt invented an owner=%+v err=%v", current, err)
	}
}

func TestFriendCancelledCreationFinalizesSuccessfulLocalSQLDelete(t *testing.T) {
	ctx := t.Context()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	local, foreign := newTestServer(), newTestServer()
	localSQL, foreignSQL := cancellationSQLWorkspace(t), cancellationSQLWorkspace(t)
	local.Friends, local.Workspaces = store, localSQL
	foreign.Friends, foreign.Workspaces = store, foreignSQL
	intent, err := local.getOrCreateCreationIntent(ctx, store, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	localWorkspace, err := local.ensureCreationWorkspace(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	_, err = foreign.cancelFriendCreation(kv.WithPrimaryReads(ctx), store, "peer", intent.RelationID, intent)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign cancellation without a local copy = %v", err)
	}
	pending, err := readRetirementIntent(kv.WithPrimaryReads(ctx), store, intent.RelationID)
	if err != nil || !pending.CancelCreation || pending.WorkspaceName != intent.Workspace {
		t.Fatalf("foreign SQL absence consumed cancellation work: %+v, %v", pending, err)
	}
	if _, err := readRetirementReceipt(kv.WithPrimaryReads(ctx), store, intent.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("foreign SQL absence published a receipt: %v", err)
	}
	assertCancellationSQLRows(t, localSQL, intent.Workspace, 1)
	assertCancellationSQLRows(t, foreignSQL, intent.Workspace, 0)
	_, err = local.commitFriendCreation(ctx, store, "owner", "peer", intent, localWorkspace)
	if !errors.Is(err, errFriendCreationCancelled) {
		t.Fatalf("resumed creator = %v", err)
	}
	assertCancellationSQLRows(t, localSQL, intent.Workspace, 0)
	receipt, err := readRetirementReceipt(kv.WithPrimaryReads(ctx), store, intent.RelationID)
	if err != nil || !receipt.CancelCreation || receipt.WorkspaceName != intent.Workspace {
		t.Fatalf("successful local cleanup receipt = %+v, %v", receipt, err)
	}
	if _, err := readRetirementIntent(kv.WithPrimaryReads(ctx), store, intent.RelationID); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("successful local cleanup left duplicate deletion work: %v", err)
	}
	if _, err := readCreationIntent(ctx, store, intent.RelationID); err != nil {
		t.Fatalf("fixture did not retain the genuinely stale creation intent: %v", err)
	}
	if err := local.ReconcileCreationIntents(ctx); err != nil {
		t.Fatalf("stale creation recovery repeated a completed SQL delete: %v", err)
	}
	// An old retirement worker also observes no receipt on its frozen reader.
	// It must prove the exact completed cancellation on the writer first.
	before := store.rejected
	if err := local.completeFriendRetirement(ctx, store, pending); err != nil {
		t.Fatalf("repeated local cancellation cleanup = %v", err)
	}
	if store.rejected <= before {
		t.Fatal("stale missing receipt was accepted without a writer comparison")
	}
	assertCancellationSQLRows(t, localSQL, intent.Workspace, 0)
}

func TestFriendCancelledSQLDeletePreservesReplacementHandoff(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	workspaces := cancellationSQLWorkspace(t)
	s.Friends, s.Workspaces = store, workspaces
	intent, err := s.getOrCreateCreationIntent(ctx, store, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ensureCreationWorkspace(ctx, intent); err != nil {
		t.Fatal(err)
	}
	pending := retirementIntent{RelationID: intent.RelationID, FirstPeer: "owner", SecondPeer: "peer", WorkspaceName: intent.Workspace, WorkspaceOwner: intent.WorkspaceOwner, DeletedAt: s.now(), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, store, retirementIntentKey(intent.RelationID), pending); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	replacement := pending
	replacement.WorkspaceName = "new-incarnation-workspace"
	replacement.DeletedAt = replacement.DeletedAt.Add(time.Second)
	replacementReceipt := retirementReceipt{RelationID: replacement.RelationID, FirstPeer: replacement.FirstPeer, SecondPeer: replacement.SecondPeer, WorkspaceName: replacement.WorkspaceName, WorkspaceOwner: replacement.WorkspaceOwner, DeletedAt: replacement.DeletedAt, CancelCreation: true}
	store.beforeWrite = func(ctx context.Context, mutation kv.Mutation) {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, retirementReceiptKey(intent.RelationID)) {
				store.beforeWrite = nil
				if err := socialutil.WriteJSON(ctx, store.Store, retirementIntentKey(intent.RelationID), replacement); err != nil {
					t.Fatal(err)
				}
				if err := socialutil.WriteJSON(ctx, store.Store, retirementReceiptKey(intent.RelationID), replacementReceipt); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); err != nil {
		t.Fatal(err)
	}
	current, err := readRetirementIntent(kv.WithPrimaryReads(ctx), store, intent.RelationID)
	if err != nil || current.WorkspaceName != replacement.WorkspaceName {
		t.Fatalf("replacement handoff consumed: %+v, %v", current, err)
	}
	assertCancellationSQLRows(t, workspaces, intent.Workspace, 0)
	if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("other incarnation's receipt acknowledged local absence: %v", err)
	}
	currentReceipt, err := readRetirementReceipt(kv.WithPrimaryReads(ctx), store, intent.RelationID)
	if err != nil || currentReceipt.WorkspaceName != replacement.WorkspaceName {
		t.Fatalf("replacement receipt changed: %+v, %v", currentReceipt, err)
	}
}

func TestFriendCancelledSQLDeleteDoesNotReplayUncertainReceiptCommit(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &socialReadSplitStore{Store: kv.NewMemory(nil), reader: kv.NewMemory(nil), replicate: true}
	workspaces := cancellationSQLWorkspace(t)
	s.Friends, s.Workspaces = store, workspaces
	intent, err := s.getOrCreateCreationIntent(ctx, store, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ensureCreationWorkspace(ctx, intent); err != nil {
		t.Fatal(err)
	}
	pending := retirementIntent{RelationID: intent.RelationID, FirstPeer: "owner", SecondPeer: "peer", WorkspaceName: intent.Workspace, WorkspaceOwner: intent.WorkspaceOwner, DeletedAt: s.now(), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, store, retirementIntentKey(intent.RelationID), pending); err != nil {
		t.Fatal(err)
	}
	store.replicate = false
	uncertain := errors.New("uncertain cancellation receipt commit")
	receiptWrites := 0
	store.beforeWrite = func(_ context.Context, mutation kv.Mutation) {
		for _, entry := range mutation.Entries {
			if slices.Equal(entry.Key, retirementReceiptKey(intent.RelationID)) {
				receiptWrites++
				store.writeErr = uncertain
			}
		}
	}
	if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); !errors.Is(err, uncertain) || receiptWrites != 1 {
		t.Fatalf("uncertain receipt commit err=%v attempts=%d", err, receiptWrites)
	}
	store.writeErr, store.beforeWrite = nil, nil
	assertCancellationSQLRows(t, workspaces, intent.Workspace, 0)
	if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); err != nil {
		t.Fatalf("known committed receipt did not allow an explicit retry: %v", err)
	}
	if receiptWrites != 1 {
		t.Fatalf("completed cancellation was republished %d times", receiptWrites)
	}
}

func TestFriendCancellationReceiptRequiresExactWorkspaceOwner(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	s.Workspaces = cancellationSQLWorkspace(t)
	for _, owner := range []string{"peer"} {
		receipt := retirementReceipt{RelationID: "owner:peer", FirstPeer: "owner", SecondPeer: "peer", WorkspaceName: "cancelled-workspace", WorkspaceOwner: owner, DeletedAt: s.now(), CancelCreation: true}
		if err := socialutil.WriteJSON(ctx, s.Friends, retirementReceiptKey(receipt.RelationID), receipt); err != nil {
			t.Fatal(err)
		}
		if err := s.completeFriendCreationCancellation(ctx, s.Friends, receipt.RelationID, "owner", receipt.WorkspaceName); err == nil {
			t.Fatalf("receipt with Workspace owner %q acknowledged a different local scope", owner)
		}
	}
	// Old durable work without immutable owner identity remains explicit work.
	pending := retirementIntent{RelationID: "owner:peer", FirstPeer: "owner", SecondPeer: "peer", WorkspaceName: "cancelled-workspace", DeletedAt: s.now(), CancelCreation: true}
	if err := s.completeFriendRetirement(ctx, s.Friends, pending); err == nil {
		t.Fatal("ownerless cancellation was guessed without durable or local SQL identity")
	}
}

type cancellationReceiptFailureStore struct {
	kv.Store
	err      error
	attempts int
}

func (s *cancellationReceiptFailureStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	for _, entry := range mutation.Entries {
		if len(entry.Key) > 0 && entry.Key[0] == retirementReceiptsRoot[0] {
			s.attempts++
			if s.err != nil {
				return false, s.err
			}
		}
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestFriendCancelledSQLDeleteRetainsUnprovenHandoffAfterCommitError(t *testing.T) {
	ctx := t.Context()
	s := newTestServer()
	store := &cancellationReceiptFailureStore{Store: kv.NewMemory(nil)}
	workspaces := cancellationSQLWorkspace(t)
	s.Friends, s.Workspaces = store, workspaces
	intent, err := s.getOrCreateCreationIntent(ctx, store, "owner", "peer", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ensureCreationWorkspace(ctx, intent); err != nil {
		t.Fatal(err)
	}
	pending := retirementIntent{RelationID: intent.RelationID, FirstPeer: "owner", SecondPeer: "peer", WorkspaceName: intent.Workspace, WorkspaceOwner: intent.WorkspaceOwner, DeletedAt: s.now(), CancelCreation: true}
	if err := socialutil.WriteJSON(ctx, store, retirementIntentKey(intent.RelationID), pending); err != nil {
		t.Fatal(err)
	}
	uncertain := errors.New("cancellation receipt outcome unavailable")
	store.err = uncertain
	if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); !errors.Is(err, uncertain) || store.attempts != 1 {
		t.Fatalf("uncertain receipt err=%v attempts=%d", err, store.attempts)
	}
	assertCancellationSQLRows(t, workspaces, intent.Workspace, 0)
	store.err = nil
	if err := s.completeFriendCreationCancellation(ctx, store, intent.RelationID, intent.WorkspaceOwner, intent.Workspace); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing SQL without a committed receipt silently completed: %v", err)
	}
	if current, err := readRetirementIntent(ctx, store, intent.RelationID); err != nil || current.WorkspaceName != intent.Workspace || store.attempts != 1 {
		t.Fatalf("unproven handoff lost: %+v, %v attempts=%d", current, err, store.attempts)
	}
}
