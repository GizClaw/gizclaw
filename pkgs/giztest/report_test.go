package giztest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReportIsRedactedAndAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	r := Report{Version: "v1", Status: "passed", Tasks: []TaskReport{{Name: "case", Status: "passed"}}}
	if err := WriteReport(path, r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || !strings.Contains(string(data), `"status": "passed"`) {
		t.Fatalf("report = %s", data)
	}
	if strings.Contains(string(data), `"attempts"`) {
		t.Fatalf("single-attempt report changed shape: %s", data)
	}
}

func TestWriteReportIncludesExplicitRetryAttempts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	r := Report{
		Version: "v1", Status: "passed",
		Tasks: []TaskReport{{
			Name: "case", Status: "passed",
			Steps: []StepReport{{
				ID: "turn", Operation: "peer_stream", Status: "passed", Stage: "peer_stream",
				Attempts: []AttemptReport{{Attempt: 1, Status: "failed", FailureKind: "timeout", Error: "deadline exceeded"}, {Attempt: 2, Status: "passed"}},
			}},
		}},
	}
	if err := WriteReport(path, r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"attempts"`, `"attempt": 1`, `"failure_kind": "timeout"`, `"attempt": 2`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("report lacks %s: %s", want, data)
		}
	}
}

func TestSafeErrorRedactsExactValues(t *testing.T) {
	got := SafeError(errors.New("request failed for opaque-value"), "opaque-value")
	if got != "request failed for [REDACTED]" {
		t.Fatalf("SafeError() = %q", got)
	}
}
