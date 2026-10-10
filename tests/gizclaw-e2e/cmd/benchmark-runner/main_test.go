package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestBenchmarkFailureRetainsConcurrentRepeatsAndRunsNextDocument(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "giztest", "benchmark.doubao-realtime-conversation.concurrency-10.giztest.yaml"),
		filepath.Join("..", "..", "giztest", "benchmark.doubao-realtime-conversation.concurrency-20.giztest.yaml"),
	}
	output := filepath.Join(t.TempDir(), "combined.json")
	var repeats []int
	execute := func(_ context.Context, _ string, doc *giztest.Document, path string) (int, error) {
		repeats = append(repeats, doc.Repeat)
		child := giztest.Report{Version: "v1", Status: "passed"}
		code := 0
		for i := range doc.Repeat {
			task := giztest.TaskReport{Name: doc.Name, Path: doc.Path, RepeatIndex: i, Status: "passed"}
			if len(repeats) == 1 && i == 0 {
				task.Status, task.Error = "failed", "audio boundary assertion"
				child.Status, code = "failed", 4
			}
			child.Tasks = append(child.Tasks, task)
		}
		return code, giztest.WriteReport(path, child)
	}
	err := run(context.Background(), "gizclaw", output, paths, io.Discard, execute)
	if err == nil || !strings.Contains(err.Error(), "1 benchmark tasks failed") {
		t.Fatalf("expected retained benchmark failure, got %v", err)
	}
	if !slices.Equal(repeats, []int{10, 20}) {
		t.Fatalf("document repeats changed or later document skipped: %v", repeats)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report giztest.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || len(report.Tasks) != 30 || report.Tasks[0].Status != "failed" || report.Tasks[29].Status != "passed" {
		t.Fatalf("combined report lost failure or later tasks: %+v", report)
	}
}

func TestBenchmarkRejectsIncompleteReport(t *testing.T) {
	path := filepath.Join("..", "..", "giztest", "benchmark.doubao-realtime-conversation.concurrency-10.giztest.yaml")
	output := filepath.Join(t.TempDir(), "combined.json")
	execute := func(_ context.Context, _ string, doc *giztest.Document, path string) (int, error) {
		child := giztest.Report{Version: "v1", Status: "passed", Tasks: []giztest.TaskReport{
			{Name: doc.Name, Path: doc.Path, RepeatIndex: 0, Status: "passed"},
		}}
		return 0, giztest.WriteReport(path, child)
	}
	err := run(context.Background(), "gizclaw", output, []string{path}, io.Discard, execute)
	if err == nil || !strings.Contains(err.Error(), "task count") {
		t.Fatalf("expected incomplete report rejection, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("incomplete document must not become a combined pass: %v", err)
	}
}

func TestCancelledBenchmarksKeepProgressWithoutPublishingAPass(t *testing.T) {
	path := filepath.Join("..", "..", "giztest", "benchmark.doubao-realtime-conversation.concurrency-10.giztest.yaml")
	output := filepath.Join(t.TempDir(), "combined.json")
	if err := os.WriteFile(output, []byte(`{"status":"passed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	execute := func(_ context.Context, _ string, doc *giztest.Document, path string) (int, error) {
		child := giztest.Report{Version: "v1", Status: "passed"}
		for i := range doc.Repeat {
			child.Tasks = append(child.Tasks, giztest.TaskReport{Name: doc.Name, Path: doc.Path, RepeatIndex: i, Status: "passed"})
		}
		cancel()
		return 0, giztest.WriteReport(path, child)
	}
	err := run(ctx, "gizclaw", output, []string{path, path}, io.Discard, execute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("incomplete run must not publish or reuse a combined pass: %v", err)
	}
	data, err := os.ReadFile(output + ".partial.json")
	if err != nil {
		t.Fatal(err)
	}
	var progress giztest.Report
	if err := json.Unmarshal(data, &progress); err != nil || len(progress.Tasks) != 10 {
		t.Fatalf("completed document progress was lost: tasks=%d error=%v", len(progress.Tasks), err)
	}
}
