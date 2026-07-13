package edgecmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestEdgeCommandIncludesServe(t *testing.T) {
	cmd := NewCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "serve") {
		t.Fatalf("edge help missing serve: %s", out)
	}
}

func TestEdgeServeRequiresWorkspaceArg(t *testing.T) {
	cmd := newServeCmd()
	if err := cmd.Args(cmd, []string{"workspace-dir"}); err != nil {
		t.Fatalf("Args(valid) error = %v", err)
	}
	if err := cmd.Args(cmd, nil); err == nil {
		t.Fatal("Args(nil) should fail")
	}
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Fatal("Args(two args) should fail")
	}
}
