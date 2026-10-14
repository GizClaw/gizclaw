package kvmigratecmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	clientlog "github.com/pingcap/log"
)

func TestCommandRequiresConnectorsAndRejectsConflictingModes(t *testing.T) {
	for _, args := range [][]string{{"workspace"}, {"workspace", "--source", "old"}, {"workspace", "--source", "old", "--target", "new", "--dry-run", "--verify-only"}} {
		command := NewCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("accepted args %v", args)
		}
	}
	command := NewCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--dry-run", "--verify-only", "--source", "--target", "--stores"} {
		if !strings.Contains(output.String(), flag) {
			t.Errorf("help omits %s", flag)
		}
	}
}

func TestClientLogsUseStderrAndRestore(t *testing.T) {
	previous := clientlog.L()
	var output bytes.Buffer
	restore, err := redirectClientLogs(&output)
	if err != nil {
		t.Fatal(err)
	}
	clientlog.L().Info("migration SDK diagnostic")
	var writers sync.WaitGroup
	for range 64 {
		writers.Go(func() { clientlog.L().Info("concurrent SDK diagnostic") })
	}
	writers.Wait()
	restore()
	if !strings.Contains(output.String(), "migration SDK diagnostic") {
		t.Fatal("SDK logger did not use command stderr")
	}
	if clientlog.L() != previous {
		t.Fatal("command retained process logger ownership")
	}
	if strings.Count(output.String(), "concurrent SDK diagnostic") != 64 {
		t.Fatal("concurrent SDK records were lost or interleaved")
	}
}

func TestPreflightErrorReportRetainsDryRunMode(t *testing.T) {
	command := NewCmd()
	var output, diagnostics bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&diagnostics)
	command.SetArgs([]string{t.TempDir(), "--source", "old", "--target", "new", "--dry-run"})
	if err := command.Execute(); err == nil {
		t.Fatal("missing config unexpectedly succeeded")
	}
	var result kv.RedisMigrationResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("error report is not JSON: %v", err)
	}
	if !result.DryRun || result.Imported != 0 {
		t.Fatalf("misleading preflight result: %+v", result)
	}
}
