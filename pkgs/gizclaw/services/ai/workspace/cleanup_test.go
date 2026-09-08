package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/flowstate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
)

func TestWorkspaceDeletionHandlerRemovesOwnedDataAndPreservesForeignData(t *testing.T) {
	ctx := t.Context()
	srv := newTestServer(t)
	runtimeObjects := newTestObjectStore(t)
	assetObjects := newTestObjectStore(t)
	srv.RuntimeStore = newTestRuntimeStore(t, runtimeObjects)
	srv.Assets = assetObjects
	now := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	owner := "peer-a"
	iconName := iconasset.ObjectName("workspace-a", iconasset.FormatPNG)
	item := deletionTestWorkspace("workspace-a", "room-a", &owner, false, now)
	item.Icon = &apitypes.Icon{Png: &iconName}
	if err := seedWorkspaceRecord(ctx, srv.DB, item); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RuntimeStore.PrepareWorkspace(ctx, item.Id); err != nil {
		t.Fatal(err)
	}
	if err := runtimeObjects.Put(ObjectPrefix(item.Id)+"/history/entry.json", strings.NewReader(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := assetObjects.Put(iconName, strings.NewReader("png")); err != nil {
		t.Fatal(err)
	}
	foreign := deletionTestWorkspace("workspace-b", "room-b", &owner, false, now)
	if err := seedWorkspaceRecord(ctx, srv.DB, foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RuntimeStore.PrepareWorkspace(ctx, foreign.Id); err != nil {
		t.Fatal(err)
	}
	if err := runtimeObjects.Put(ObjectPrefix(foreign.Id)+"/history/entry.json", strings.NewReader(`{}`)); err != nil {
		t.Fatal(err)
	}

	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace,
		item.Id,
		item.OwnerPublicKey,
		pendingdeletion.ReasonResourceDelete,
		workspaceDeletionDescriptor{ID: item.Id, Name: item.Name, OwnerPublicKey: item.OwnerPublicKey, HasIcon: true},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewPendingDeletionSource(srv.DB).CreateOrGet(ctx, record); err != nil {
		t.Fatal(err)
	}

	stateDB, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	stateDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := stateDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := flowstate.Initialize(ctx, stateDB); err != nil {
		t.Fatal(err)
	}
	targetState, err := flowstate.OpenScope(ctx, stateDB, owner, item.Id, "agent")
	if err != nil {
		t.Fatal(err)
	}
	foreignState, err := flowstate.OpenScope(ctx, stateDB, owner, foreign.Id, "agent")
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []*flowstate.Store{targetState, foreignState} {
		if err := state.SaveState(ctx, "checkpoint", []byte(`{"kept":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	source := NewPendingDeletionSource(srv.DB)
	claim := claimWorkspaceTask(t, source, now.Add(time.Second))
	quiescer := &recordingWorkspaceQuiescer{}
	handler := DeletionHandler{
		Server: srv, Source: source, Quiescer: quiescer, Flowcraft: flowstate.WorkspaceCleanup{DB: stateDB},
		Now: func() time.Time { return now.Add(time.Second) },
	}
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := getWorkspaceByID(ctx, srv.DB, item.Id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("target Workspace error = %v, want not found", err)
	}
	if _, err := getWorkspaceByID(ctx, srv.DB, foreign.Id); err != nil {
		t.Fatalf("foreign Workspace removed: %v", err)
	}
	if absent, err := srv.RuntimeStore.(RuntimeCleanupStore).WorkspaceRuntimeAbsent(ctx, item.Id); err != nil || !absent {
		t.Fatalf("target runtime absent = %v, %v", absent, err)
	}
	if absent, err := srv.RuntimeStore.(RuntimeCleanupStore).WorkspaceRuntimeAbsent(ctx, foreign.Id); err != nil || absent {
		t.Fatalf("foreign runtime absent = %v, %v", absent, err)
	}
	if len(quiescer.ids) != 2 || quiescer.ids[0] != item.Id || quiescer.ids[1] != item.Id {
		t.Fatalf("quiesced Workspaces = %#v", quiescer.ids)
	}

	if err := targetState.SaveState(ctx, "checkpoint", []byte(`{}`)); !errors.Is(err, flowstate.ErrRetired) {
		t.Fatalf("stale state write = %v", err)
	}
	if value, err := targetState.LoadState(ctx, "checkpoint"); err != nil || value != nil {
		t.Fatalf("retired state = %s, %v", value, err)
	}
	if value, err := foreignState.LoadState(ctx, "checkpoint"); err != nil || string(value) != `{"kept":true}` {
		t.Fatalf("foreign state = %s, %v", value, err)
	}
	if _, err := source.GetTask(ctx, record.DeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("GetTask() error = %v, want ErrNotFound", err)
	}
}

func TestWorkspaceDeletionHandlerRejectsReplacement(t *testing.T) {
	srv := newTestServer(t)
	now := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	owner := "peer-a"
	item := deletionTestWorkspace("workspace-a", "room-a", &owner, false, now)
	if err := seedWorkspaceRecord(t.Context(), srv.DB, item); err != nil {
		t.Fatal(err)
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace, item.Id, item.OwnerPublicKey, pendingdeletion.ReasonResourceDelete,
		workspaceDeletionDescriptor{ID: item.Id, Name: item.Name, OwnerPublicKey: item.OwnerPublicKey}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewPendingDeletionSource(srv.DB).CreateOrGet(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	replacementOwner := "peer-b"
	item.OwnerPublicKey = &replacementOwner
	if _, err := srv.DB.ExecContext(t.Context(), `UPDATE workspaces SET owner_public_key=? WHERE id=?`, replacementOwner, item.Id); err != nil {
		t.Fatal(err)
	}

	source := NewPendingDeletionSource(srv.DB)
	claim := claimWorkspaceTask(t, source, now.Add(time.Second))
	err = (DeletionHandler{Server: srv, Source: source, Now: func() time.Time { return now.Add(time.Second) }}).Handle(t.Context(), claim)
	var outcome *pendingdeletion.OutcomeError
	if !errors.As(err, &outcome) || outcome.Class != pendingdeletion.OutcomeTerminal || outcome.Code != "replacement_ambiguous" {
		t.Fatalf("Handle() error = %#v", err)
	}
	if _, err := source.GetTask(t.Context(), record.DeletionID); err != nil {
		t.Fatalf("task removed after replacement conflict: %v", err)
	}
}

func deletionTestWorkspace(id, name string, owner *string, system bool, now time.Time) apitypes.Workspace {
	labels := map[string]string{}
	return apitypes.Workspace{
		Id: id, Name: name, WorkflowId: "workflow-a", OwnerPublicKey: owner, System: &system,
		CreatedAt: now, UpdatedAt: now, LastActiveAt: now, Labels: &labels,
	}
}

func jsonMarshalWorkspace(item apitypes.Workspace) ([]byte, error) {
	return json.Marshal(item)
}

func claimWorkspaceTask(t *testing.T, source workspaceSQLDeletionSource, now time.Time) pendingdeletion.Claim {
	t.Helper()
	refs, _, err := source.ScanDue(t.Context(), now, 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(t.Context(), refs[0], now, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	return claim
}

type recordingWorkspaceQuiescer struct {
	ids []string
}

func (q *recordingWorkspaceQuiescer) QuiesceWorkspace(_ context.Context, id string) error {
	q.ids = append(q.ids, id)
	return nil
}

func TestWorkspaceDeletionClaimOwnerScope(t *testing.T) {
	owner, other := "peer-a", "peer-b"
	for _, tc := range []struct {
		name                   string
		owner, descriptorOwner *string
		wantError              bool
	}{
		{name: "ownerless"},
		{name: "owned", owner: &owner, descriptorOwner: &owner},
		{name: "different owner", owner: &owner, descriptorOwner: &other, wantError: true},
		{name: "missing descriptor owner", owner: &owner, wantError: true},
		{name: "missing record owner", descriptorOwner: &owner, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record, err := pendingdeletion.New(pendingdeletion.KindWorkspace, "workspace-a", tc.owner,
				pendingdeletion.ReasonResourceDelete, workspaceDeletionDescriptor{
					ID: "workspace-a", Name: "room-a", OwnerPublicKey: tc.descriptorOwner,
				}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			fingerprint, err := pendingdeletion.Fingerprint(record)
			if err != nil {
				t.Fatal(err)
			}
			_, err = validateWorkspaceDeletionClaim(pendingdeletion.Claim{Task: pendingdeletion.Task{
				Source: pendingDeletionSourceName, Record: record, MarkerFingerprint: fingerprint,
			}})
			if (err != nil) != tc.wantError {
				t.Fatalf("validateWorkspaceDeletionClaim() = %v, want error %v", err, tc.wantError)
			}
		})
	}
}
