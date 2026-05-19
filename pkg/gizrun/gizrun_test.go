package gizrun

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/cmdhandler"
)

func TestHandleCmd(t *testing.T) {
	resetDefaultCmdHandlerForTest(t)

	handler := CmdHandleFunc(func(context.Context, []string, []string) error { return nil })
	if err := HandleCmd("admin/play", handler); err != nil {
		t.Fatalf("HandleCmd failed: %v", err)
	}

	got, ok := runCtx.cmdHandler.Lookup("admin/play")
	if !ok {
		t.Fatal("Lookup ok = false, want true")
	}
	if got == nil {
		t.Fatal("handler = nil")
	}
}

func TestServeMissingCommandDoesNotRunPostInit(t *testing.T) {
	resetRuntimeForTest(t)
	resetInitHooksForTest(t)
	resetDefaultCmdHandlerForTest(t)
	resetFlagSetForTest(t)

	previousArgs := os.Args
	os.Args = []string{"gizrun", "missing"}
	t.Cleanup(func() {
		os.Args = previousArgs
	})

	postInitRan := false
	exitRan := false
	PostInitAt(0, func(*RunContext) error {
		postInitRan = true
		return nil
	})
	ExitAt(0, func(*RunContext) error {
		exitRan = true
		return nil
	})

	err := Serve()
	if err == nil || !strings.Contains(err.Error(), "command handler not found") {
		t.Fatalf("Serve error = %v, want command handler not found", err)
	}
	if postInitRan {
		t.Fatal("post-init hook ran for missing command")
	}
	if exitRan {
		t.Fatal("exit hook ran without post-init startup")
	}
}

func resetDefaultCmdHandlerForTest(t *testing.T) {
	t.Helper()
	previous := runCtx.cmdHandler
	runCtx.cmdHandler = cmdhandler.New()
	t.Cleanup(func() {
		runCtx.cmdHandler = previous
	})
}
