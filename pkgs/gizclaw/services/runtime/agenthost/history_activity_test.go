package agenthost

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

func TestHistoryAgentAdvancesWorkspaceLastActiveAt(t *testing.T) {
	ctx := context.Background()
	objects := newTestObjectStore(t)
	workspaces := workspacetest.New(t)
	workspaces.RuntimeStore = workspace.NewObjectRuntimeStore(objects, newTestHistoryLogStore(t), objects)
	seeded := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	workspacetest.Seed(t, workspaces, apitypes.Workspace{
		Id: "save-1", Name: "save-1", WorkflowId: "workflow",
		CreatedAt: seeded, UpdatedAt: seeded, LastActiveAt: seeded,
	})
	// The Resolver hands the Agent host exactly this runtime.
	runtime, err := workspaces.GetWorkspaceRuntimeByID(ctx, "save-1")
	if err != nil {
		t.Fatalf("GetWorkspaceRuntimeByID() error = %v", err)
	}
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	)}, runtime.History)

	out, err := agent.Transform(withHistoryGearID(ctx, "gear-a"), historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		if _, err := out.Next(); err != nil {
			if !IsStreamDone(err) {
				t.Fatalf("Next() error = %v", err)
			}
			break
		}
	}
	latest, found, err := runtime.History.LatestEntry(ctx)
	if err != nil || !found {
		t.Fatalf("LatestEntry() = %+v, %v, %v", latest, found, err)
	}
	after, err := workspaces.GetAvailableWorkspaceByID(ctx, "save-1")
	if err != nil {
		t.Fatalf("GetAvailableWorkspaceByID() error = %v", err)
	}
	if !after.LastActiveAt.Equal(latest.CreatedAt) {
		t.Fatalf("last_active_at = %s, want latest entry %s", after.LastActiveAt, latest.CreatedAt)
	}
	if !after.CreatedAt.Equal(seeded) || !after.UpdatedAt.Equal(seeded) {
		t.Fatalf("created_at = %s updated_at = %s, want unchanged %s", after.CreatedAt, after.UpdatedAt, seeded)
	}

	if _, err := runtime.History.Append(ctx, workspace.AppendHistoryRequest{
		Type: "agent", Name: "assistant", Text: "older", CreatedAt: seeded.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Append(older) error = %v", err)
	}
	final, err := workspaces.GetAvailableWorkspaceByID(ctx, "save-1")
	if err != nil {
		t.Fatalf("GetAvailableWorkspaceByID(final) error = %v", err)
	}
	if !final.LastActiveAt.Equal(latest.CreatedAt) {
		t.Fatalf("last_active_at after older entry = %s, want unchanged %s", final.LastActiveAt, latest.CreatedAt)
	}
}
