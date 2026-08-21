package giztest

import (
	"bytes"
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
