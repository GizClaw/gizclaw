package giztest

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

func TestTaskIDAndOutputObserverSurviveSetupFailureCleanup(t *testing.T) {
	if TaskID(context.Background()) != "" {
		t.Fatal("task ID leaked outside Run")
	}
	doc := &Document{Name: "observer", Path: "observer.giztest.yaml", Repeat: 3, Variables: map[string]VariableSpec{
		"secret": {Direction: "input", Type: "string", Value: "hidden-value", Secret: true},
		"note":   {Direction: "input", Type: "string", Value: "note hidden-value"},
	}, Finally: []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "note"}}}}
	var mu sync.Mutex
	observed := make(map[string]string)
	report := Run(context.Background(), []*Document{doc}, Options{Driver: &stubDriver{openErr: errors.New("fixture setup failure")}, Parallel: 3, Out: io.Discard, ObserveOutput: func(ctx context.Context, req StepRequest, text string) {
		if req.DocumentPath != doc.Path || !req.Cleanup {
			t.Error("observer lost cleanup metadata")
		}
		mu.Lock()
		observed[TaskID(ctx)] = text
		mu.Unlock()
	}})
	if len(observed) != 3 {
		t.Fatalf("observed tasks=%v", observed)
	}
	for _, task := range report.Tasks {
		if observed[task.TaskID] != "note [REDACTED]" {
			t.Fatalf("%s output=%q", task.TaskID, observed[task.TaskID])
		}
	}
}
