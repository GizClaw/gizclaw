package contextcmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func testPublicKeyText(fill byte) string {
	kp, err := giznet.NewKeyPair(testPrivateKey(fill))
	if err != nil {
		panic(err)
	}
	return kp.Public.String()
}

func testPrivateKey(fill byte) giznet.Key {
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	return key
}

func TestContextCreateStoresDescription(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := NewCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{
		"create",
		"local",
		"--server",
		"127.0.0.1:9820",
		"--public-key",
		testPublicKeyText(0xab),
		"--description",
		"Local dev",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	store, err := clicontext.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore error = %v", err)
	}
	ctx, err := store.LoadByName("local")
	if err != nil {
		t.Fatalf("LoadByName error = %v", err)
	}
	if ctx.Config.Description != "Local dev" {
		t.Fatalf("Description = %q, want Local dev", ctx.Config.Description)
	}
}

func TestContextCommandsManageContexts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	executeContextCmd(t,
		"create", "alpha",
		"--server", "127.0.0.1:9820",
		"--public-key", testPublicKeyText(0xab),
	)
	executeContextCmd(t,
		"create", "beta",
		"--server", "127.0.0.1:9821",
		"--public-key", testPublicKeyText(0xcd),
		"--description", "Beta context",
	)

	listOut := executeContextCmd(t, "list")
	if !strings.Contains(listOut, "* alpha") || !strings.Contains(listOut, "  beta") {
		t.Fatalf("list output = %q", listOut)
	}

	executeContextCmd(t, "use", "alpha")
	infoOut := executeContextCmd(t, "info")
	var info contextInfo
	if err := json.Unmarshal([]byte(infoOut), &info); err != nil {
		t.Fatalf("decode info output %q: %v", infoOut, err)
	}
	if info.Name != "alpha" || !info.Current || info.ServerEndpoint != "127.0.0.1:9820" {
		t.Fatalf("info = %+v", info)
	}

	showOut := executeContextCmd(t, "show", "beta")
	var shown contextInfo
	if err := json.Unmarshal([]byte(showOut), &shown); err != nil {
		t.Fatalf("decode show output %q: %v", showOut, err)
	}
	if shown.Name != "beta" || shown.Current || shown.Description != "Beta context" || shown.ServerEndpoint != "127.0.0.1:9821" {
		t.Fatalf("show = %+v", shown)
	}

	executeContextCmd(t, "delete", "alpha")
	listOut = executeContextCmd(t, "list")
	if strings.Contains(listOut, "alpha") || !strings.Contains(listOut, "beta") {
		t.Fatalf("list after delete output = %q", listOut)
	}
}

func TestContextInfoWithoutActiveContext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := NewCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"info"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no active context") {
		t.Fatalf("info error = %v", err)
	}
}

func TestContextListEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	out := executeContextCmd(t, "list")
	if !strings.Contains(out, "No contexts found.") {
		t.Fatalf("list empty output = %q", out)
	}
}

func executeContextCmd(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := NewCmd()
	cmd.SetOut(&out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("context %v error = %v", args, err)
	}
	return out.String()
}
