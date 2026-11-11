package friend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
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

func TestFriendLegacyReceiptWithoutIdentitySupportsHistoricalDeleteOnly(t *testing.T) {
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
	if err := s.completeFriendCreationCancellation(ctx, s.Friends, legacy.RelationID, "owner", legacy.WorkspaceName); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("historical receipt guessed local cleanup owner=%v", err)
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
	for _, owner := range []string{"peer", ""} {
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
