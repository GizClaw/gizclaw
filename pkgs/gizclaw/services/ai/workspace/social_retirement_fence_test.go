package workspace

import (
	"context"
	"github.com/jmoiron/sqlx"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

// recordingFencer runs the fenced callback and records which Workspaces took
// the reward deletion fence.
type recordingFencer struct {
	fenced []string
}

func (f *recordingFencer) WithWorkspaceDeletionFence(ctx context.Context, resourceID string, fenced func(context.Context, *sqlx.DB, *sqlx.Tx) error) error {
	f.fenced = append(f.fenced, resourceID)
	return fenced(ctx, nil, nil)
}

// TestRetireSystemWorkspaceFencesEveryNonSFUSystemWorkspace pins which
// retirements may skip the reward settlement fence. Only the exact Social SFU
// shape may: a stale or malformed Social retirement record naming another
// system Workspace must not carry it past the fence.
func TestRetireSystemWorkspaceFencesEveryNonSFUSystemWorkspace(t *testing.T) {
	now := time.Now().UTC()
	owner := "peer-a"
	sfuItem := deletionTestWorkspace("ws-sfu", "sfu-1", &owner, true, now)
	sfuItem.WorkflowId = socialutil.SFUWorkflowID
	parameterized := sfuItem
	parameterized.Id, parameterized.Name = "ws-sfu-params", "sfu-2"
	parameterized.Parameters = &apitypes.WorkspaceParameters{}
	petItem := deletionTestWorkspace("ws-pet", "pet-1", &owner, true, now)
	notSystem := deletionTestWorkspace("ws-user", "user-1", &owner, false, now)
	notSystem.System = nil

	for name, tc := range map[string]struct {
		item       apitypes.Workspace
		wantFenced bool
		wantErr    bool
	}{
		"social sfu workspace skips the fence":       {item: sfuItem},
		"other system workspace keeps the fence":     {item: petItem, wantFenced: true},
		"parameterized sfu keeps the fence":          {item: parameterized, wantFenced: true},
		"non system workspace is not retired at all": {item: notSystem, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			store := newTestServer(t).DB
			fencer := &recordingFencer{}
			server := &Server{DB: store, DeletionFencer: fencer}
			if tc.item.System != nil {
				if err := createSQLWorkspace(ctx, store, tc.item); err != nil {
					t.Fatal(err)
				}
			}
			retired, err := server.retireSystemWorkspace(ctx, store, tc.item, socialutil.SFUWorkspaceKindFriend, "relation-1")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("retireSystemWorkspace() = %#v, want an error", retired)
				}
				if len(fencer.fenced) != 0 {
					t.Fatalf("rejected Workspace took the fence: %v", fencer.fenced)
				}
				return
			}
			if err != nil {
				t.Fatalf("retireSystemWorkspace() error = %v", err)
			}
			pending, err := NewPendingDeletionSource(store).HasLocator(ctx, pendingdeletion.Locator{Kind: pendingdeletion.KindWorkspace, ResourceID: tc.item.Id})
			if err != nil || !pending {
				t.Fatalf("pending deletion marker = %v, %v; want present", pending, err)
			}
			fenced := len(fencer.fenced) == 1 && fencer.fenced[0] == tc.item.Id
			if fenced != tc.wantFenced {
				t.Fatalf("fenced = %v (%v), want %v", fenced, fencer.fenced, tc.wantFenced)
			}
		})
	}
}
