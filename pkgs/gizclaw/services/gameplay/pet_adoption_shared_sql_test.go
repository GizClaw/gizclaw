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
)

type adoptionSQLWorkspace struct {
	*workspace.Server
	spendBalance bool
	createdID    string
}

func (s *adoptionSQLWorkspace) CreateSystemWorkspace(ctx context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	item, created, err := s.Server.CreateSystemWorkspace(ctx, body)
	if err != nil {
		return item, created, err
	}
	s.createdID = item.Id
	if s.spendBalance {
		_, err = s.DB.ExecContext(ctx, `UPDATE gameplay_points_accounts SET balance=0`)
	}
	return item, created, err
}

func TestPetAdoptionSharesSingleSQLConnection(t *testing.T) {
	for _, spent := range []bool{false, true} {
		name := "funded"
		if spent {
			name = "balance-spent-during-workspace-creation"
		}
		t.Run(name, func(t *testing.T) {
			server := workspacetest.New(t)
			server.Workflows = petWorkflowService{}
			workspaces := &adoptionSQLWorkspace{Server: server, spendBalance: spent}
			now := time.Now().UTC()
			catalog := testCatalog(t, now)
			profile := seedGameplayCatalog(t, t.Context(), catalog)
			ctx, cancel := context.WithTimeout(WithRuntimeProfile(t.Context(), profile), 5*time.Second)
			defer cancel()
			runtime := &Runtime{DB: server.DB, Catalog: catalog, Workspaces: workspaces, Workflows: petWorkflowService{}, Now: func() time.Time { return now }, PickWeight: func(int64) int64 { return 0 }}
			if err := runtime.Migration(ctx); err != nil {
				t.Fatal(err)
			}
			server.DeletionFencer = runtime
			result, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Name: "shared-sql-pet", DisplayName: "Pet"})
			if spent {
				if !errors.Is(err, errInsufficientPoints) {
					t.Fatalf("adoption = %v, want insufficient points", err)
				}
				if _, err := server.GetAvailableWorkspaceByID(ctx, workspaces.createdID); !errors.Is(err, workspace.ErrWorkspaceDeleted) {
					t.Fatalf("failed adoption workspace = %v", err)
				}
				var pets, transactions int
				if err := server.DB.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM gameplay_pets), (SELECT COUNT(*) FROM gameplay_points_transactions WHERE reason='pet.adopt')`).Scan(&pets, &transactions); err != nil {
					t.Fatal(err)
				}
				if pets != 0 || transactions != 0 {
					t.Fatalf("failed adoption left pets=%d transactions=%d", pets, transactions)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				retried, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Name: "shared-sql-pet", DisplayName: "Pet"})
				if err != nil {
					t.Fatal(err)
				}
				if retried.Pet.Id != result.Pet.Id || retried.Points.Balance != result.Points.Balance || retried.Transaction.Id != result.Transaction.Id {
					t.Fatal("adoption retry changed the Pet, balance, or transaction")
				}
				if result.Pet.WorkspaceId != workspaces.createdID {
					t.Fatal("adoption did not bind its SQL Workspace")
				}
				if _, err := server.GetAvailableWorkspaceByID(ctx, result.Pet.WorkspaceId); err != nil {
					t.Fatal(err)
				}
			}
			if err := server.DB.PingContext(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
