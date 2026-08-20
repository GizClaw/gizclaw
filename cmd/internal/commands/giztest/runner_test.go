package giztest

import (
	"context"
	"io"
	"testing"
)

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
