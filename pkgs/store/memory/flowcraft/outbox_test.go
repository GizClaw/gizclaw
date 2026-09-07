package flowcraft

import (
	"context"
	"testing"

	"github.com/GizClaw/flowcraft/memory/recall"
)

type recordingSideEffectOutbox struct {
	recall.SideEffectOutbox
	jobs []recall.SideEffectJob
}

func (outbox *recordingSideEffectOutbox) Enqueue(_ context.Context, job recall.SideEffectJob) error {
	outbox.jobs = append(outbox.jobs, job)
	return nil
}

func TestScopedSideEffectOutboxQualifiesJobIDByScope(t *testing.T) {
	recording := &recordingSideEffectOutbox{}
	outbox := scopedSideEffectOutbox{SideEffectOutbox: recording}
	first := recall.SideEffectJob{
		RequestID: "save-1", Kind: recall.SideEffectProjectRequired,
		Scope: recall.Scope{RuntimeID: "workspace-a"},
	}
	second := first
	second.Scope = recall.Scope{RuntimeID: "workspace-b", UserID: "user", AgentID: "agent"}
	preset := first
	preset.ID = "explicit"
	for _, job := range []recall.SideEffectJob{first, second, first, preset} {
		if err := outbox.Enqueue(t.Context(), job); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"workspace-a/global|save-1|" + string(recall.SideEffectProjectRequired),
		"workspace-b/u:user/a:agent|save-1|" + string(recall.SideEffectProjectRequired),
		"workspace-a/global|save-1|" + string(recall.SideEffectProjectRequired),
		"explicit",
	}
	if len(recording.jobs) != len(want) {
		t.Fatalf("enqueued %d jobs, want %d", len(recording.jobs), len(want))
	}
	for index, job := range recording.jobs {
		if job.ID != want[index] {
			t.Fatalf("job %d ID = %q, want %q", index, job.ID, want[index])
		}
	}
}

func TestScopedSideEffectOutboxSkipsRequestlessJobs(t *testing.T) {
	recording := &recordingSideEffectOutbox{}
	outbox := scopedSideEffectOutbox{SideEffectOutbox: recording}
	if err := outbox.Enqueue(t.Context(), recall.SideEffectJob{Scope: recall.Scope{RuntimeID: "workspace"}}); err != nil {
		t.Fatal(err)
	}
	if len(recording.jobs) != 1 || recording.jobs[0].ID != "" {
		t.Fatalf("jobs = %#v, want one job with an empty ID", recording.jobs)
	}
}
