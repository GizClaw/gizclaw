package localserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestManagerStartsCapturesBoundedLogsAndStops(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "gizclaw")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\ni=0\nwhile :; do\n  echo line-$i\n  i=$((i + 1))\n  sleep 0.01\ndone\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	manager := New()
	manager.Executable = executable
	manager.MaxLogLines = 5
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		manager.Shutdown(ctx)
	})

	status, err := manager.Start("local-lab", filepath.Join(dir, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "running" || status.PID == 0 {
		t.Fatalf("Start() = %+v", status)
	}
	duplicate, err := manager.Start("local-lab", filepath.Join(dir, "workspace"))
	if err != nil || duplicate.PID != status.PID {
		t.Fatalf("duplicate Start() = %+v, %v", duplicate, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for len(manager.Status("local-lab").Logs) < manager.MaxLogLines && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	status = manager.Status("local-lab")
	if len(status.Logs) != manager.MaxLogLines {
		t.Fatalf("logs = %d, want %d: %v", len(status.Logs), manager.MaxLogLines, status.Logs)
	}
	if got := status.Logs[len(status.Logs)-1]; got == "" {
		t.Fatal("last log line is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err = manager.Stop(ctx, "local-lab")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.PID != 0 {
		t.Fatalf("Stop() = %+v", status)
	}
	if len(status.Logs) > manager.MaxLogLines {
		t.Fatalf("Stop() logs = %d, want <= %d", len(status.Logs), manager.MaxLogLines)
	}
	if status.Error != "" {
		t.Fatalf("Stop() error state = %q", status.Error)
	}
}

func TestManagerReportsUnexpectedExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "gizclaw")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\necho failed-to-start >&2\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := New()
	manager.Executable = executable
	if _, err := manager.Start("broken", filepath.Join(dir, "workspace")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	status := manager.Status("broken")
	for status.State == "running" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		status = manager.Status("broken")
	}
	if status.State != "failed" || status.Error == "" {
		t.Fatalf("Status() = %+v", status)
	}
}

func TestManagerRecoversExistingProcessFromWorkspacePID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	executable := filepath.Join(dir, "gizclaw")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	original := New()
	original.Executable = executable
	started, err := original.Start("local-lab", workspace)
	if err != nil {
		t.Fatal(err)
	}
	pidData, err := os.ReadFile(filepath.Join(workspace, PIDFile))
	if err != nil {
		t.Fatal(err)
	}
	pidInfo, err := os.Stat(filepath.Join(workspace, PIDFile))
	if err != nil {
		t.Fatal(err)
	}
	if pidInfo.Mode().Perm() != 0o600 {
		t.Fatalf("PID file mode = %o", pidInfo.Mode().Perm())
	}
	if string(pidData) != fmt.Sprintf("%d\n", started.PID) {
		t.Fatalf("PID file = %q, want %d", pidData, started.PID)
	}

	restartedDesktop := New()
	recovered, err := restartedDesktop.Recover("local-lab", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != "running" || recovered.PID != started.PID {
		t.Fatalf("Recover() = %+v, want PID %d", recovered, started.PID)
	}
	duplicate, err := restartedDesktop.Start("local-lab", workspace)
	if err != nil || duplicate.PID != started.PID {
		t.Fatalf("Start() after recovery = %+v, %v", duplicate, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopped, err := restartedDesktop.Stop(ctx, "local-lab")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != "stopped" || stopped.PID != 0 {
		t.Fatalf("Stop() recovered process = %+v", stopped)
	}
	if _, err := os.Stat(filepath.Join(workspace, PIDFile)); !os.IsNotExist(err) {
		t.Fatalf("PID file after Stop() error = %v", err)
	}
}

func TestManagerRemovesStaleWorkspacePID(t *testing.T) {
	workspace := t.TempDir()
	pidPath := filepath.Join(workspace, PIDFile)
	if err := os.WriteFile(pidPath, []byte("not-a-pid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := New().Recover("local-lab", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" {
		t.Fatalf("Recover() = %+v", status)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("stale PID file error = %v", err)
	}
}

func TestManagerShutdownStopsAllProcessesAndRejectsNewStarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "gizclaw")
	script := "#!/bin/sh\ntrap 'exit 0' INT TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := New()
	manager.Executable = executable
	for _, id := range []string{"first", "second"} {
		if _, err := manager.Start(id, filepath.Join(dir, id)); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	manager.Shutdown(ctx)
	for _, id := range []string{"first", "second"} {
		if status := manager.Status(id); status.State != "stopped" || status.PID != 0 {
			t.Fatalf("Status(%q) after Shutdown() = %+v", id, status)
		}
	}
	if _, err := manager.Start("late", filepath.Join(dir, "late")); err == nil {
		t.Fatal("Start() during shutdown error = nil")
	}
}

func TestManagerShutdownKillsProcessAfterTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "gizclaw")
	script := "#!/bin/sh\ntrap '' INT TERM\nwhile :; do :; done\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := New()
	manager.Executable = executable
	if _, err := manager.Start("stubborn", filepath.Join(dir, "workspace")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	manager.Shutdown(ctx)
	if status := manager.Status("stubborn"); status.State != "stopped" || status.PID != 0 {
		t.Fatalf("Status() after forced Shutdown() = %+v", status)
	}
}
