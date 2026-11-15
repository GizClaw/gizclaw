package workspace

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/jmoiron/sqlx"
)

func TestWorkspaceRetirementPreparationFencesOwnedSQLAdmissions(t *testing.T) {
	testWorkspaceRetirementAdmission(t, newTestServer(t).DB)
}

func testWorkspaceRetirementAdmission(t *testing.T, db *sqlx.DB) {
	t.Helper()
	s := &Server{DB: db}
	ctx, now, owner := t.Context(), time.Now().UTC(), "peer-a"
	first := deletionTestWorkspace("first", "first", &owner, false, now)
	if err := createSQLWorkspace(ctx, s.DB, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SnapshotPeerWorkspaces(ctx, owner); err != nil {
		t.Fatal(err)
	}
	second := deletionTestWorkspace("second", "second", &owner, false, now)
	if err := createSQLWorkspace(ctx, s.DB, second); err != nil {
		t.Fatalf("read-only probe fenced admissions: %v", err)
	}
	other := &Server{DB: s.DB}
	snapshot, err := other.PreparePeerWorkspacesRetirement(ctx, owner)
	if err != nil || len(snapshot.Workspaces) != 2 {
		t.Fatalf("fenced inventory=%+v, %v", snapshot, err)
	}
	for _, system := range []bool{false, true} {
		late := deletionTestWorkspace("late", "late", &owner, system, now)
		if err := createSQLWorkspace(ctx, s.DB, late); !errors.Is(err, ErrPeerPendingDeletion) || !errors.Is(err, ownership.ErrOwnerRetired) {
			t.Fatalf("late owned admission system=%t: %v", system, err)
		}
		if _, _, err := getSQLWorkspaceByID(ctx, s.DB, late.Id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("late row persisted: %v", err)
		}
	}
	foreign := "peer-b"
	if err := createSQLWorkspace(ctx, s.DB, deletionTestWorkspace("foreign", "foreign", &foreign, false, now)); err != nil {
		t.Fatalf("foreign owner was fenced: %v", err)
	}
	if err := createSQLWorkspace(ctx, s.DB, deletionTestWorkspace("unowned", "unowned", nil, false, now)); err != nil {
		t.Fatalf("unowned Workspace was fenced: %v", err)
	}
	if replay, err := other.PreparePeerWorkspacesRetirement(ctx, owner); err != nil || !snapshot.Covers(replay) {
		t.Fatalf("replayed preparation=%+v, %v", replay, err)
	}
}

func TestWorkspaceRetirementRejectsOtherInstanceAdmittedInitializer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	s := newTestServer(t)
	seedWorkflow(t, s, "workflow-1")
	runtime := &recordingRuntimeStore{}
	s.RuntimeStore = runtime
	// Simulate a retained historical Peer observation on every availability call.
	s.PeerAvailability = func(context.Context, string) error { return nil }
	entered, released := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(released) })
	defer release()
	done := make(chan error, 1)
	go func() {
		_, err := s.CreatePeerWorkspace(ownership.WithOwner(ctx, "peer-a"), PeerWorkspaceCreateRequest{
			Name: "admitted", WorkflowID: "workflow-1",
			Initialize: func(ctx context.Context, _ Runtime) error {
				close(entered)
				select {
				case <-released:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	other := &Server{DB: s.DB}
	inventory, err := other.PreparePeerWorkspacesRetirement(ctx, "peer-a")
	if err != nil || len(inventory.Workspaces) != 0 {
		t.Fatalf("other Server retirement=%+v, %v", inventory, err)
	}
	release()
	select {
	case err := <-done:
		var createErr *PeerWorkspaceCreateError
		if !errors.Is(err, ErrPeerPendingDeletion) || !errors.As(err, &createErr) || createErr.Kind != PeerWorkspaceCreateConflict {
			t.Fatalf("late creator=%v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if len(runtime.prepared) != 1 || len(runtime.deleted) != 1 || runtime.prepared[0] != runtime.deleted[0] {
		t.Fatalf("prepared runtime leaked: prepared=%v deleted=%v", runtime.prepared, runtime.deleted)
	}
	if rows, _, _, err := listSQLWorkspaces(ctx, s.DB, workspaceSQLFilter{owner: new("peer-a")}, "", 10); err != nil || len(rows) != 0 {
		t.Fatalf("late Workspace survived fenced snapshot: %v, %v", rows, err)
	}
}

func TestWorkspaceMutationAvailabilityDoesNotRefreshOrdinaryReads(t *testing.T) {
	s := newTestServer(t)
	seedWorkflow(t, s, "workflow-1")
	missing := errors.New("owner absent from historical reader")
	ordinaryChecks, mutationChecks := 0, 0
	s.PeerAvailability = func(context.Context, string) error {
		ordinaryChecks++
		return missing
	}
	s.PeerMutationAvailability = func(context.Context, string) error {
		mutationChecks++
		return nil
	}
	ctx := ownership.WithOwner(t.Context(), "peer-a")
	created, err := s.CreatePeerWorkspace(ctx, PeerWorkspaceCreateRequest{Name: "new-owner", WorkflowID: "workflow-1"})
	if err != nil || mutationChecks == 0 || ordinaryChecks != 0 {
		t.Fatalf("mutation input used ordinary gate: %+v, %v, checks=%d/%d", created, err, ordinaryChecks, mutationChecks)
	}
	before := mutationChecks
	if _, err := s.GetWorkspaceByName(ctx, created.Name); !errors.Is(err, missing) || mutationChecks != before || ordinaryChecks != 1 {
		t.Fatalf("ordinary read upgraded gate: %v, checks=%d/%d", err, ordinaryChecks, mutationChecks)
	}
}

func TestSystemWorkspaceCleanupLookupRetainsExactOwnerDuringRetirement(t *testing.T) {
	s := newTestServer(t)
	ctx, now, owner := t.Context(), time.Now().UTC(), "peer-a"
	system := deletionTestWorkspace("system", "system", &owner, true, now)
	ordinary := deletionTestWorkspace("ordinary", "ordinary", &owner, false, now)
	for _, item := range []apitypes.Workspace{system, ordinary} {
		if err := createSQLWorkspace(ctx, s.DB, item); err != nil {
			t.Fatal(err)
		}
	}
	s.PeerAvailability = func(context.Context, string) error { return ErrPeerPendingDeletion }
	if _, err := s.GetWorkspaceByName(ownership.WithOwner(ctx, owner), system.Name); !errors.Is(err, ErrPeerPendingDeletion) {
		t.Fatalf("ordinary lookup bypassed retirement: %v", err)
	}
	got, err := s.GetSystemWorkspaceForCleanup(ownership.WithOwner(ctx, owner), system.Name)
	if err != nil || got.Id != system.Id || got.OwnerPublicKey == nil || *got.OwnerPublicKey != owner {
		t.Fatalf("cleanup identity=%+v, %v", got, err)
	}
	if _, err := s.GetSystemWorkspaceForCleanup(ownership.WithOwner(ctx, "peer-b"), system.Name); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign row was exposed: %v", err)
	}
	if _, err := s.GetSystemWorkspaceForCleanup(ctx, system.Name); err == nil {
		t.Fatal("cleanup accepted no owner scope")
	}
	if _, err := s.GetSystemWorkspaceForCleanup(ownership.WithOwner(ctx, owner), ordinary.Name); err == nil {
		t.Fatal("ordinary Workspace accepted as Social cleanup proof")
	}
}
