package pendingdeletion

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateTaskStateInvariants(t *testing.T) {
	record, err := New(KindPeer, "peer-task", nil, ReasonPeerDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := Fingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	valid := Task{
		Source: "peer", Record: record, MarkerFingerprint: fingerprint,
		Status: StatusQueued, Phase: PhaseValidate, NextAttemptAt: record.DeletedAt, UpdatedAt: record.DeletedAt,
	}
	if err := ValidateTask(valid); err != nil {
		t.Fatalf("ValidateTask(valid) = %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Task)
	}{
		{name: "source", mutate: func(task *Task) { task.Source = " peer" }},
		{name: "record", mutate: func(task *Task) { task.Record.ResourceID = " " }},
		{name: "fingerprint", mutate: func(task *Task) { task.MarkerFingerprint = "wrong" }},
		{name: "status", mutate: func(task *Task) { task.Status = "unknown" }},
		{name: "phase", mutate: func(task *Task) { task.Phase = "INVALID" }},
		{name: "failure count", mutate: func(task *Task) { task.FailureCount = -1 }},
		{name: "updated time", mutate: func(task *Task) { task.UpdatedAt = time.Time{} }},
		{name: "failure metadata", mutate: func(task *Task) { task.LastErrorMessage = strings.Repeat("x", 257) }},
		{name: "running lease", mutate: func(task *Task) {
			task.Status = StatusRunning
			task.NextAttemptAt = time.Time{}
		}},
		{name: "inactive lease", mutate: func(task *Task) { task.LeaseToken = "token" }},
		{name: "due time", mutate: func(task *Task) { task.NextAttemptAt = time.Time{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			task := valid
			test.mutate(&task)
			if err := ValidateTask(task); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateTask() error = %v, want ErrInvalid", err)
			}
		})
	}
	running := valid
	running.Status = StatusRunning
	running.NextAttemptAt = time.Time{}
	running.LeaseToken = "token"
	running.LeaseDeadline = time.Unix(2, 0)
	if err := ValidateTask(running); err != nil {
		t.Fatalf("ValidateTask(running) = %v", err)
	}
}
