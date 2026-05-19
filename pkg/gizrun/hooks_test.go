package gizrun

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/configfile"
	"github.com/gofiber/fiber/v2"
)

func TestServeParsesCommandFlagsOnly(t *testing.T) {
	resetInitHooksForTest(t)
	resetDefaultCmdHandlerForTest(t)
	resetFlagSetForTest(t)
	configFile := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(configFile, nil, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	previousArgs := os.Args
	os.Args = []string{"gizrun", "-config", configFile, "chat", "-model=gpt"}
	t.Cleanup(func() {
		os.Args = previousArgs
	})

	var value string
	var gotArgs []string
	var gotFlags []string
	InitAt(0, func() error {
		flag.StringVar(&value, "config", "", "config path")
		return nil
	})
	if err := HandleCmd("chat", CmdHandleFunc(func(_ context.Context, args []string, flags []string) error {
		gotArgs = append([]string(nil), args...)
		gotFlags = append([]string(nil), flags...)
		return nil
	})); err != nil {
		t.Fatalf("HandleCmd failed: %v", err)
	}

	if err := Serve(); err != nil {
		t.Fatalf("Serve failed: %v", err)
	}

	if value != configFile {
		t.Fatalf("config flag = %q, want %q", value, configFile)
	}
	if want := []string{"chat"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}
	if len(gotFlags) != 1 || gotFlags[0] != "-model=gpt" {
		t.Fatalf("flags = %#v, want model flag", gotFlags)
	}
}

func TestServePassesFullCommandArgs(t *testing.T) {
	resetRuntimeForTest(t)
	resetInitHooksForTest(t)
	resetDefaultCmdHandlerForTest(t)
	resetFlagSetForTest(t)

	previousArgs := os.Args
	os.Args = []string{"gizrun", "chat", "hello", "-model=gpt"}
	t.Cleanup(func() {
		os.Args = previousArgs
	})

	var gotArgs []string
	var gotFlags []string
	if err := HandleCmd("chat/hello", CmdHandleFunc(func(_ context.Context, args []string, flags []string) error {
		gotArgs = append([]string(nil), args...)
		gotFlags = append([]string(nil), flags...)
		return nil
	})); err != nil {
		t.Fatalf("HandleCmd failed: %v", err)
	}

	if err := Serve(); err != nil {
		t.Fatalf("Serve failed: %v", err)
	}

	if want := []string{"chat", "hello"}; !slices.Equal(gotArgs, want) {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}
	if want := []string{"-model=gpt"}; !slices.Equal(gotFlags, want) {
		t.Fatalf("flags = %#v, want %#v", gotFlags, want)
	}
}

func TestRuntimeHooksInitializeHTTPAndPprof(t *testing.T) {
	initHookValues := append([]initHook(nil), initHooks.hooks...)
	postInitHookValues := append([]postInitHook(nil), postInitHooks.hooks...)
	exitHookValues := append([]exitHook(nil), exitHooks.hooks...)
	resetRuntimeForTest(t)
	resetFlagSetForTest(t)

	runInitHooks(initHookValues)
	if err := flag.CommandLine.Set("pprof", "true"); err != nil {
		t.Fatalf("set pprof flag: %v", err)
	}
	if err := runCtx.loadConfig(flag.CommandLine); err != nil {
		t.Fatalf("load config: %v", err)
	}
	runPostInitHooks(runCtx, postInitHookValues)
	t.Cleanup(func() {
		runExitHooks(runCtx, exitHookValues)
	})

	metricsResp, err := runCtx.debugApp.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if err != nil {
		t.Fatal(err)
	}
	if metricsResp.StatusCode != fiber.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", metricsResp.StatusCode)
	}
	pprofResp, err := runCtx.debugApp.Test(httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if pprofResp.StatusCode != fiber.StatusOK {
		t.Fatalf("/debug/pprof/ status = %d, want 200", pprofResp.StatusCode)
	}
}

func TestConfigFilePostInitAppliesRuntimeConfig(t *testing.T) {
	postInitHookValues := append([]postInitHook(nil), postInitHooks.hooks...)
	resetRuntimeForTest(t)

	pprof := true
	debugPort := 0
	runCtx.configFile = configfile.ConfigFile{
		"gizrun": gizrunConfig{
			DebugPort: &debugPort,
			Addr:      "127.0.0.1:0",
			Pprof:     &pprof,
			Metrics: &struct {
				Push *metricsPushConfig `yaml:"push"`
			}{
				Push: &metricsPushConfig{
					Enabled: true,
					URL:     "http://push.example.test",
				},
			},
		},
	}

	runPostInitHooksBySeq(t, runCtx, postInitHookValues, 0x01)

	if runCtx.debugPort != 0 {
		t.Fatalf("debug port = %d, want 0", runCtx.debugPort)
	}
	if runCtx.addr != "127.0.0.1:0" {
		t.Fatalf("addr = %q, want configured addr", runCtx.addr)
	}
	if !runCtx.enablePprof {
		t.Fatal("pprof = false, want true")
	}
	if !runCtx.metricsPush.Enabled || runCtx.metricsPush.URL != "http://push.example.test" {
		t.Fatalf("metrics push = %#v, want enabled config", runCtx.metricsPush)
	}
}

func TestLogLevelPostInitAppliesVerboseFlag(t *testing.T) {
	postInitHookValues := append([]postInitHook(nil), postInitHooks.hooks...)
	resetRuntimeForTest(t)

	if err := runCtx.verbose.Set("debug"); err != nil {
		t.Fatalf("set verbose: %v", err)
	}
	runPostInitHooksBySeq(t, runCtx, postInitHookValues, 0x10)

	if got := runCtx.logLevel.Level(); got != slog.LevelDebug {
		t.Fatalf("log level = %v, want debug", got)
	}
}

func TestRuntimeHooksServePublicHTTP(t *testing.T) {
	postInitHookValues := append([]postInitHook(nil), postInitHooks.hooks...)
	exitHookValues := append([]exitHook(nil), exitHooks.hooks...)
	resetRuntimeForTest(t)
	runCtx.addr = "127.0.0.1:0"

	runPostInitHooksBySeq(t, runCtx, postInitHookValues, 0x31)
	t.Cleanup(func() {
		runExitHooksBySeq(t, runCtx, exitHookValues, 0x00)
	})

	if !runCtx.publicListening {
		t.Fatal("publicListening = false, want true")
	}
	if runCtx.addr == "" || runCtx.addr == "127.0.0.1:0" {
		t.Fatalf("addr = %q, want bound listener addr", runCtx.addr)
	}
}

func TestPostInitAt(t *testing.T) {
	resetInitHooksForTest(t)
	postInitHooks.next = 0
	postInitHooks.hooks = nil

	var got []int
	PostInitAt(0x20, func(*RunContext) error { got = append(got, 2); return nil })
	PostInitAt(0x10, func(*RunContext) error { got = append(got, 1); return nil })
	PostInitAt(0x20, func(*RunContext) error { got = append(got, 3); return nil })
	PostInitAt(0x30, nil)

	runInitHooks(initHooks.hooks)
	runPostInitHooks(runCtx, postInitHooks.hooks)

	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("post-init hook count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("post-init hook order = %#v, want %#v", got, want)
		}
	}
}

func TestInitAt(t *testing.T) {
	resetInitHooksForTest(t)

	var got []int
	InitAt(0x20, func() error { got = append(got, 2); return nil })
	InitAt(0x10, func() error { got = append(got, 1); return nil })
	InitAt(0x20, func() error { got = append(got, 3); return nil })
	InitAt(0x30, nil)

	runInitHooks(initHooks.hooks)

	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("init hook count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("init hook order = %#v, want %#v", got, want)
		}
	}
}

func TestExitAt(t *testing.T) {
	resetInitHooksForTest(t)
	exitHooks.next = 0
	exitHooks.hooks = nil

	var got []int
	ExitAt(0x20, func(*RunContext) error { got = append(got, 2); return nil })
	ExitAt(0x10, func(*RunContext) error { got = append(got, 1); return nil })
	ExitAt(0x20, func(*RunContext) error { got = append(got, 3); return nil })
	ExitAt(0x30, nil)

	runExitHooks(runCtx, exitHooks.hooks)

	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("exit hook count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("exit hook order = %#v, want %#v", got, want)
		}
	}
}

func runPostInitHooksBySeq(t *testing.T, ctx *RunContext, hooks []postInitHook, seq int) {
	t.Helper()
	ran := false
	for _, hook := range hooks {
		if hook.seq != seq {
			continue
		}
		ran = true
		if err := hook.fn(ctx); err != nil {
			t.Fatalf("post-init hook 0x%x failed: %v", seq, err)
		}
	}
	if !ran {
		t.Fatalf("post-init hook 0x%x was not found", seq)
	}
}

func runExitHooksBySeq(t *testing.T, ctx *RunContext, hooks []exitHook, seq int) {
	t.Helper()
	ran := false
	for _, hook := range hooks {
		if hook.seq != seq {
			continue
		}
		ran = true
		if err := hook.fn(ctx); err != nil {
			t.Fatalf("exit hook 0x%x failed: %v", seq, err)
		}
	}
	if !ran {
		t.Fatalf("exit hook 0x%x was not found", seq)
	}
}

func resetInitHooksForTest(t *testing.T) {
	t.Helper()
	previousInit := initHooks
	previousPostInit := postInitHooks
	previousExit := exitHooks
	t.Cleanup(func() {
		initHooks = previousInit
		postInitHooks = previousPostInit
		exitHooks = previousExit
	})
	initHooks.next = 1
	initHooks.hooks = []initHook{{
		seq: 0,
		fn: func() error {
			return nil
		},
	}}
	postInitHooks.next = 1
	postInitHooks.hooks = []postInitHook{{
		seq: 0,
		fn:  func(*RunContext) error { return nil },
	}}
	exitHooks.next = 1
	exitHooks.hooks = []exitHook{{
		seq: 0,
		fn:  func(*RunContext) error { return nil },
	}}
}

func resetRuntimeForTest(t *testing.T) {
	t.Helper()
	previous := runCtx
	runCtx = newRunContext()
	t.Cleanup(func() {
		runCtx = previous
	})
}

func resetFlagSetForTest(t *testing.T) {
	t.Helper()
	previous := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("gizrun-test", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	t.Cleanup(func() {
		flag.CommandLine = previous
	})
}
