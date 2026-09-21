//go:build gizclaw_sdk_e2e

package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The admission lane builds all native runners before selecting this test. A
// missing SDK or a native process failure is an error, never a skipped test.
func TestAdmissionSDKGiztests(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []struct {
		name    string
		command []string
	}{
		{"JavaScript", []string{"node", "--experimental-strip-types", filepath.Join(root, "tests/gizclaw-e2e/js/giztest/index.ts")}},
		{"C", []string{os.Getenv("GIZCLAW_ADMISSION_C_RUNNER"), "test"}},
		{"Flutter", []string{os.Getenv("GIZCLAW_ADMISSION_FLUTTER_RUNNER")}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			if runner.command[0] == "" {
				t.Fatal("admission runner executable is required")
			}
			var after func(context.Context)
			if runner.name == "JavaScript" {
				after = func(ctx context.Context) {
					command := exec.CommandContext(ctx, "go", "test", "-tags=gizclaw_e2e", "./tests/gizclaw-e2e/go/admission", "-count=1")
					command.Dir = root
					if output, err := command.CombinedOutput(); err != nil {
						t.Fatalf("Go SDK admission: %v\n%s", err, output)
					}
				}
			}
			runAdmissionGiztests(t, func(ctx context.Context, file, report string) ([]byte, error) {
				args := append(append([]string{}, runner.command[1:]...), "run", file, "--parallel", "1", "--output", report)
				command := exec.CommandContext(ctx, runner.command[0], args...)
				command.Dir = root
				return command.CombinedOutput()
			}, after)
		})
	}
}
