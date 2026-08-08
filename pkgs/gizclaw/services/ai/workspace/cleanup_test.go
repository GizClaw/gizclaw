package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestWorkspaceDeletionHandlerRemovesOwnedDataAndPreservesForeignData(t *testing.T) {
	ctx := t.Context()
	srv := newTestServer(t)
	runtimeObjects := newTestObjectStore(t)
	assetObjects := newTestObjectStore(t)
	srv.RuntimeStore = NewObjectRuntimeStore(runtimeObjects)
	srv.Assets = assetObjects
	now := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	owner := "peer-a"
	iconName := iconasset.ObjectName("workspace-a", iconasset.FormatPNG)
	item := deletionTestWorkspace("workspace-a", "room-a", &owner, false, now)
	item.Icon = &apitypes.Icon{Png: &iconName}
	if err := writeWorkspace(ctx, srv.Store, item); err != nil {
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
	if err := writeWorkspace(ctx, srv.Store, foreign); err != nil {
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
	if _, _, err := pendingdeletion.CreateOrGet(ctx, srv.Store, record); err != nil {
		t.Fatal(err)
	}
	source := NewPendingDeletionSource(srv.Store)
	claim := claimWorkspaceTask(t, source, now.Add(time.Second))
	gameplay := &recordingWorkspaceCleanup{rows: map[string]bool{item.Id: true, foreign.Id: true}}
	quiescer := &recordingWorkspaceQuiescer{}
	handler := DeletionHandler{
		Server: srv, Source: source, Quiescer: quiescer, Gameplay: gameplay,
		Now: func() time.Time { return now.Add(time.Second) },
	}
	if err := handler.Handle(ctx, claim); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := getWorkspaceByID(ctx, srv.Store, item.Id); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("target Workspace error = %v, want not found", err)
	}
	if _, err := getWorkspaceByID(ctx, srv.Store, foreign.Id); err != nil {
		t.Fatalf("foreign Workspace removed: %v", err)
	}
	if absent, err := srv.RuntimeStore.(RuntimeCleanupStore).WorkspaceRuntimeAbsent(ctx, item.Id); err != nil || !absent {
		t.Fatalf("target runtime absent = %v, %v", absent, err)
	}
	if absent, err := srv.RuntimeStore.(RuntimeCleanupStore).WorkspaceRuntimeAbsent(ctx, foreign.Id); err != nil || absent {
		t.Fatalf("foreign runtime absent = %v, %v", absent, err)
	}
	if gameplay.rows[item.Id] || !gameplay.rows[foreign.Id] {
		t.Fatalf("Gameplay rows = %#v", gameplay.rows)
	}
	if len(quiescer.ids) != 2 || quiescer.ids[0] != item.Id || quiescer.ids[1] != item.Id {
		t.Fatalf("quiesced Workspaces = %#v", quiescer.ids)
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
	if err := writeWorkspace(t.Context(), srv.Store, item); err != nil {
		t.Fatal(err)
	}
	record, err := pendingdeletion.New(
		pendingdeletion.KindWorkspace, item.Id, item.OwnerPublicKey, pendingdeletion.ReasonResourceDelete,
		workspaceDeletionDescriptor{ID: item.Id, Name: item.Name, OwnerPublicKey: item.OwnerPublicKey}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(t.Context(), srv.Store, record); err != nil {
		t.Fatal(err)
	}
	replacementOwner := "peer-b"
	item.OwnerPublicKey = &replacementOwner
	data, err := jsonMarshalWorkspace(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.Set(t.Context(), workspaceKey(item.Id), data); err != nil {
		t.Fatal(err)
	}
	source := NewPendingDeletionSource(srv.Store)
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

func claimWorkspaceTask(t *testing.T, source pendingdeletion.KVSource, now time.Time) pendingdeletion.Claim {
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

type recordingWorkspaceCleanup struct {
	rows map[string]bool
}

func (c *recordingWorkspaceCleanup) DeleteWorkspaceData(_ context.Context, id string) error {
	delete(c.rows, id)
	return nil
}

func (c *recordingWorkspaceCleanup) WorkspaceDataAbsent(_ context.Context, id string) (bool, error) {
	return !c.rows[id], nil
}

type recordingWorkspaceQuiescer struct {
	ids []string
}

func (q *recordingWorkspaceQuiescer) QuiesceWorkspace(_ context.Context, id string) error {
	q.ids = append(q.ids, id)
	return nil
}
