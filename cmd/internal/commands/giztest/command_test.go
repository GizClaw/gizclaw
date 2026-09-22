package giztestcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"strings"
	"testing"
)

func TestValidateCommandHasNoRuntimeSideEffects(t *testing.T) {
	path := writeTestDocument(t, validDocument)
	cmd := NewCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"validate", "-f", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() == "" {
		t.Fatal("missing validation summary")
	}
}

func TestRunCommandRejectsNonPositiveParallelismBeforeConnect(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"run", "--parallel", "0", writeTestDocument(t, validDocument)})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("zero parallelism accepted")
	}
	if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitValidation {
		t.Fatalf("error = %#v", err)
	}
}

func TestValidateRunOptionsEvidenceModes(t *testing.T) {
	for name, tc := range map[string]struct {
		parallel int
		output   string
		evidence string
		wantErr  string
	}{
		"redacted default": {parallel: 1, evidence: "redacted"},
		"full with output": {parallel: 1, output: "report.json", evidence: "full"},
		"unknown mode":     {parallel: 1, evidence: "verbose", wantErr: "redacted or full"},
		"full without output": {
			parallel: 1, evidence: "full", wantErr: "requires --output",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateRunOptions(tc.parallel, tc.output, tc.evidence)
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestPlayCommandRequiresOneFile(t *testing.T) {
	for name, args := range map[string][]string{
		"no file":        {"play", "-o", "record"},
		"multiple files": {"play", "-o", "record", "a.giztest.yaml", "b.giztest.yaml"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := NewCmd()
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("invalid play command accepted")
			}
			coded, ok := err.(interface{ ExitCode() int })
			if !ok || coded.ExitCode() != exitValidation {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestTimingFlagPresence(t *testing.T) {
	for _, args := range [][]string{nil, {"--start-jitter=0", "--stagger=0", "--step-jitter=0", "--seed=0"}} {
		cmd := newRunCmd()
		if err := cmd.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		timing := commandTiming(cmd)
		if args == nil {
			if timing.StartJitter != nil || timing.Stagger != nil || timing.StepJitter != nil || timing.Seed != nil {
				t.Fatalf("defaults became overrides: %+v", timing)
			}
		} else if timing.StartJitter == nil || *timing.StartJitter != "0" || timing.Stagger == nil || *timing.Stagger != "0" || timing.StepJitter == nil || *timing.StepJitter != "0" || timing.Seed == nil || *timing.Seed != 0 {
			t.Fatalf("explicit zeros lost: %+v", timing)
		}
	}
}

func TestRunRejectsInvalidTimingBeforeConnect(t *testing.T) {
	for _, flag := range []string{"--start-jitter=-1s", "--stagger=later", "--step-jitter=0..3s", "--seed=-1"} {
		cmd := NewCmd()
		cmd.SetArgs([]string{"run", flag, writeTestDocument(t, validDocument)})
		err := cmd.Execute()
		if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitValidation {
			t.Fatalf("%s error = %#v", flag, err)
		}
	}
}

func TestRunCanceledContextWritesUnstartedTaskReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := writeTestDocument(t, strings.Replace(validDocument, "clients:\n", "start_jitter: 1h\nclients:\n", 1))
	output := filepath.Join(t.TempDir(), "report.json")
	cmd := NewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"run", path, "--seed=0", "--output", output})
	err := cmd.Execute()
	if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitExecution {
		t.Fatalf("error = %#v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report giztest.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || report.Seed != 0 || len(report.Tasks) != 1 || report.Tasks[0].ActualStartOffsetMS != nil || report.Tasks[0].Error != "context canceled" {
		t.Fatalf("report = %+v", report)
	}
}
