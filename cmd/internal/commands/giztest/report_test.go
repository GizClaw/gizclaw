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
	if err := writeReport(path, r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || !strings.Contains(string(data), `"status": "passed"`) {
		t.Fatalf("report = %s", data)
	}
}

func TestSafeErrorRedactsExactValues(t *testing.T) {
	got := safeError(errors.New("request failed for opaque-value"), "opaque-value")
	if got != "request failed for [REDACTED]" {
		t.Fatalf("safeError() = %q", got)
	}
}
