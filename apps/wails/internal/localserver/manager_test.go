package localserver

import (
	"context"
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
