package gameplay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/jmoiron/sqlx"
)

type sharedSQLRewardEnvironment struct {
	workspaceRewardTestEnvironment
	workspaces *workspace.Server
}

func (e *sharedSQLRewardEnvironment) EnsureWorkspaceAvailable(ctx context.Context, id string) error {
	_, err := e.workspaces.GetAvailableWorkspaceByID(ctx, id)
	return err
}

func (e *sharedSQLRewardEnvironment) EnsureWorkspaceAvailableInTransaction(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, id string) error {
	_, err := e.workspaces.GetAvailableWorkspaceInTransaction(ctx, db, tx, id)
	return err
}

func TestWorkspaceRewardSettlementSharesSingleSQLConnection(t *testing.T) {
	for _, state := range []string{"available", "deleting", "missing"} {
		t.Run(state, func(t *testing.T) {
			server := workspacetest.New(t)
			now := time.Now().UTC()
			if state != "missing" {
				workspacetest.Seed(t, server, apitypes.Workspace{
					Id: "workspace-reward", Name: "reward", WorkflowId: "workflow",
					CreatedAt: now, UpdatedAt: now, LastActiveAt: now,
				})
			}
			runtime := &Runtime{DB: server.DB, WorkspaceRewards: &sharedSQLRewardEnvironment{workspaces: server}, Now: func() time.Time { return now }}
			if err := runtime.Migration(t.Context()); err != nil {
				t.Fatal(err)
			}
			server.DeletionFencer = runtime
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if state == "deleting" {
				response, err := server.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: "workspace-reward"})
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := response.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
					t.Fatalf("delete: %T", response)
				}
			}
			policy := workspaceRewardTestPolicy(t)
			source := workspaceRewardSource{WorkspaceID: "workspace-reward", ScheduledCheckpoint: "002", CreatedAt: now, UpdatedAt: now}
			if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
				t.Fatal(err)
			}
			window := workspaceRewardWindow{
				ID: "window-reward", WorkspaceID: source.WorkspaceID,
				WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-a",
				RuntimeProfileId: "profile-a", RuntimeProfileRevision: "revision-a",
				Policy: policy, PolicyDigest: policy.Digest, StartHistoryID: "001", HighWaterHistoryID: "002",
				StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
				EvaluateAfter: now, State: workspaceRewardClaimed, AttemptCount: 1,
				ClaimToken: "claim-reward", ClaimUntil: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
			}
			if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
				t.Fatal(err)
			}
			_, _, err := runtime.settleWorkspaceReward(ctx, window, "transcript", workspaceRewardEvaluation{Score: 0, Reason: "Not qualified."})
			switch state {
			case "available":
				if err != nil {
					t.Fatal(err)
				}
			case "deleting":
				if !errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
					t.Fatalf("settlement error = %v", err)
				}
			case "missing":
				if !errors.Is(err, workspace.ErrWorkspaceDeleted) {
					t.Fatalf("settlement error = %v", err)
				}
			}
			// The connection must be returned after both commit and rejection paths.
			if err := server.DB.PingContext(ctx); err != nil {
				t.Fatal(err)
			}
			if server.DB.Stats().InUse != 0 {
				t.Fatal("settlement retained SQL connection")
			}
		})
	}
}
