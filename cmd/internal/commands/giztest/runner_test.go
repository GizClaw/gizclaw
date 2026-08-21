package giztest

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

func TestRunTaskRunsFinalizersAfterClientSetupFailure(t *testing.T) {
	var output bytes.Buffer
	doc := &Document{
		Name: "setup-failure", Path: "setup-failure.giztest.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"endpoint": {Direction: "input", Type: "string", Value: "http://invalid"},
			"evidence": {Direction: "input", Type: "string", Value: "cleanup-ran"},
		},
		Clients: map[string]ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "${endpoint}"}},
		Finally: []Step{{ID: "emit_cleanup", Output: &OutputOperation{Variable: "evidence"}}},
	}
	result := runTask(context.Background(), task{doc: doc}, runOptions{in: nil, out: &output})
	if result.Status != "failed" || len(result.Cleanup) != 1 || result.Cleanup[0].Status != "passed" {
		t.Fatalf("result = %#v", result)
	}
	if output.String() != "evidence=cleanup-ran\n" {
		t.Fatalf("cleanup output = %q", output.String())
	}
}

func TestRunDocumentsAdmitsBarrierGroupAtomically(t *testing.T) {
	block := make(chan struct{})
	started := make(chan string, 3)
	var once sync.Once
	docBefore := &Document{Name: "before", Path: "before.giztest.yaml", Repeat: 1, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "review", ReviewOp: &ReviewOperation{Message: "before"}}}}
	docGroup := &Document{Name: "group", Path: "group.giztest.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	in := &blockingReviewReader{block: block, started: started, once: &once}
	done := make(chan Report, 1)
	go func() {
		done <- runDocuments(context.Background(), []*Document{docBefore, docGroup}, runOptions{parallel: 2, in: in, out: io.Discard})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("leading task did not start")
	}
	select {
	case report := <-done:
		t.Fatalf("barrier group started before two slots were available: %#v", report)
	case <-time.After(50 * time.Millisecond):
	}
	close(block)
	select {
	case report := <-done:
		if report.Status != "passed" {
			t.Fatalf("report = %#v", report)
		}
		for _, result := range report.Tasks {
			if result.Name == "group" && result.DurationMS >= 25 {
				t.Fatalf("barrier task was partially admitted for %dms", result.DurationMS)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
}

type blockingReviewReader struct {
	block   <-chan struct{}
	started chan<- string
	once    *sync.Once
}

func (r *blockingReviewReader) Read(p []byte) (int, error) {
	r.once.Do(func() { r.started <- "started" })
	<-r.block
	copy(p, "pass\n")
	return 5, nil
}

func TestRunDocumentsRejectsInsufficientBarrierCapacityOffline(t *testing.T) {
	doc := &Document{Name: "benchmark.case", Path: "case.giztest.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	report := runDocuments(context.Background(), []*Document{doc}, runOptions{parallel: 1, in: nil, out: io.Discard})
	if report.Status != "failed" || len(report.Tasks) != 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunDocumentsReportsEveryCancelledTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := &Document{Name: "cancelled", Path: "cancelled.giztest.yaml", Repeat: 3, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 3}}}}
	report := runDocuments(ctx, []*Document{doc}, runOptions{parallel: 3, out: io.Discard})
	if len(report.Tasks) != 3 || report.Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
}
