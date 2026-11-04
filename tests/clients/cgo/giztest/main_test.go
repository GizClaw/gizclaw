package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

const barrierDocument = `# User Story:
# As a Giztest operator,
# I want CLI zero timing overrides to permit barrier runs,
# So that I can select a lockstep control without editing the document.
version: gizclaw.test/v1alpha1
name: barrier-timing
clients:
  peer: {identity: ephemeral, connection: webrtc, access_point: localhost:9820}
variables: {}
steps:
  - id: sync
    barrier: {}
`

func TestRunBarrierTimingZeroOverrides(t *testing.T) {
	for _, field := range []string{"start_jitter", "stagger", "step_jitter"} {
		t.Run(field, func(t *testing.T) {
			document := barrierDocument + field + ": 1s\n"
			path := filepath.Join(t.TempDir(), "barrier.giztest.yaml")
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := newRunCmd()
			cmd.SetArgs([]string{path})
			err := cmd.Execute()
			if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitValidation {
				t.Fatalf("unmodified document error = %#v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			output := filepath.Join(t.TempDir(), "report.json")
			cmd = newRunCmd()
			cmd.SetContext(ctx)
			cmd.SetArgs([]string{path, "--" + strings.ReplaceAll(field, "_", "-") + "=0", "--output", output})
			err = cmd.Execute()
			if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitExecution {
				t.Fatalf("zero override did not reach execution: %#v", err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var report giztest.Report
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Tasks) != 1 || report.Tasks[0].Error != "context canceled" || report.Tasks[0].StartJitter != "0s" || report.Tasks[0].Stagger != "0s" || report.Tasks[0].StepJitter != "0s" {
				t.Fatalf("report = %+v", report)
			}
		})
	}
}
