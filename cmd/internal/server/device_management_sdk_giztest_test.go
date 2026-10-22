//go:build gizclaw_sdk_e2e

package server

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// A missing runner, skipped document/step, failed assertion or failed cleanup
// fails the lane. Flutter has no telemetry sender; Go/JS/C execute KV packets.
func TestDeviceManagementSDKGiztests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name      string
		command   []string
		telemetry bool
	}{
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}, true},
		{"C", []string{os.Getenv("GIZCLAW_MANAGEMENT_C_RUNNER"), "test"}, true},
		{"Flutter", []string{os.Getenv("GIZCLAW_MANAGEMENT_FLUTTER_RUNNER")}, false},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("native SDK runner executable is required")
			}
			runDeviceManagementGiztests(t, false, runner.telemetry, func(ctx context.Context, file, report string) ([]byte, error) {
				args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
				cmd := exec.CommandContext(ctx, runner.command[0], args...)
				cmd.Dir = root
				if runner.name == "Flutter" && runtime.GOOS == "linux" {
					return runManagementNativeDiagnostic(ctx, cmd)
				}
				return cmd.CombinedOutput()
			})
		})
	}
}

func runManagementNativeDiagnostic(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return output.Bytes(), err
	}
	pid := cmd.Process.Pid
	var diagnostics []byte
	done := make(chan struct{})
	timer := time.AfterFunc(20*time.Second, func() {
		defer close(done)
		if ctx.Err() != nil {
			return
		}
		diagnosticCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer cancel()
		stack := exec.CommandContext(diagnosticCtx, "sudo", "-n", "timeout", "--kill-after=1s", "5s",
			"gdb", "--batch", "--nx", "--quiet", "-iex", "set auto-load off", "-iex", "set debuginfod enabled off",
			"-ex", "set print frame-arguments none", "-ex", "set print entry-values no",
			"-ex", "thread apply all bt 24", "-p", strconv.Itoa(pid))
		trace, err := stack.CombinedOutput()
		diagnostics = append(fmt.Appendf(nil, "\nNATIVE_FLUTTER_THREAD_STACK pid=%d error_type=%T\n", pid, err), trace...)
	})
	err := cmd.Wait()
	if timer.Stop() {
		close(done)
	}
	<-done
	return append(output.Bytes(), diagnostics...), err
}
