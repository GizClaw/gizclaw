package gizclaw

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestDriveFactTargetUsesOpaqueBindingIdentity(t *testing.T) {
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileMem0Connection(apitypes.RuntimeProfileMem0Connection{
		ApiKey: "top-secret-api-key", Endpoint: "https://memory.example", ProjectId: "project",
		Type: apitypes.RuntimeProfileMem0ConnectionTypeMem0,
	}); err != nil {
		t.Fatal(err)
	}
	spec := agenthost.Spec{
		Workspace:  apitypes.Workspace{Id: "workspace-id", Name: "pet-workspace"},
		MemoryName: "long-term", MemoryProfileID: "profile-id",
		MemoryProfileRevision: "revision",
		MemoryBinding: &apitypes.RuntimeProfileMemoryBinding{
			Driver:   apitypes.RuntimeProfileMemoryDriverMem0,
			LayoutId: "layout", Connection: connection,
		},
		MemoryLayout: &apitypes.MemoryLayout{Name: "layout"},
	}
	target, err := driveFactTarget(spec)
	if err != nil {
		t.Fatal(err)
	}
	if target.WorkspaceID != "workspace-id" || target.BindingName != "long-term" ||
		target.BindingIdentity == "" || strings.Contains(target.BindingIdentity, "top-secret") {
		t.Fatalf("target = %#v", target)
	}
	changed := spec
	changedConnection := apitypes.RuntimeProfileMemoryConnection{}
	if err := changedConnection.FromRuntimeProfileMem0Connection(apitypes.RuntimeProfileMem0Connection{
		ApiKey: "rotated-secret", Endpoint: "https://memory.example", ProjectId: "project",
		Type: apitypes.RuntimeProfileMem0ConnectionTypeMem0,
	}); err != nil {
		t.Fatal(err)
	}
	changedBinding := *spec.MemoryBinding
	changedBinding.Connection = changedConnection
	changed.MemoryBinding = &changedBinding
	changedTarget, err := driveFactTarget(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedTarget.BindingIdentity == target.BindingIdentity {
		t.Fatal("binding identity did not change with physical connection")
	}
	revisionOnly := target
	revisionOnly.ProfileRevision = "new-revision"
	if !sameDriveFactTarget(revisionOnly, target) {
		t.Fatal("RuntimeProfile revision-only change was treated as a physical binding change")
	}
	if sameDriveFactTarget(changedTarget, target) {
		t.Fatal("physical connection change was not detected")
	}
}

type serverDriveFactMemory struct{}

func (serverDriveFactMemory) Snapshot(context.Context, string) (gameplay.DriveFactTarget, error) {
	return gameplay.DriveFactTarget{}, nil
}

func (serverDriveFactMemory) Observe(context.Context, gameplay.DriveFactTarget, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func (serverDriveFactMemory) Wait(context.Context, gameplay.DriveFactTarget, memory.OperationRequest) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func TestServerCloseJoinsDriveFactDispatcher(t *testing.T) {
	db, err := sqlx.Open("sqlite", "file:drive-fact-dispatcher?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runtime := &gameplay.Runtime{DB: db, DriveFacts: serverDriveFactMemory{}}
	server := &Server{manager: &Manager{Gameplay: runtime}}
	server.startDriveFactDispatcher()
	if server.driveFactStop == nil || server.driveFactDone == nil {
		t.Fatal("Drive Fact dispatcher did not start")
	}
	done := make(chan error, 1)
	go func() { done <- server.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Server.Close did not join Drive Fact dispatcher")
	}
	if server.driveFactStop != nil || server.driveFactDone != nil {
		t.Fatal("Server.Close did not clear Drive Fact dispatcher state")
	}
}
