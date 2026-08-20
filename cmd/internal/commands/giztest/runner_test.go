package giztest

import (
	"bytes"
	"context"
	"io"
	"testing"
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
