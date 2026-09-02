package giztestcmd

import (
	"bytes"
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
