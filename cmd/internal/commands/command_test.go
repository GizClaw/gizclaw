package commands

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestNormalizeLegacyLongFlags(t *testing.T) {
	got := normalizeLegacyLongFlags([]string{"context", "create", "demo", "-server=127.0.0.1:9820", "-description", "demo", "--help", "-h"})
	want := []string{"context", "create", "demo", "--server=127.0.0.1:9820", "--description", "demo", "--help", "-h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLegacyLongFlags() = %#v, want %#v", got, want)
	}
}

func TestRootHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "serve") {
		t.Fatalf("help missing 'serve': %s", out)
	}
	if !strings.Contains(out, "service") {
		t.Fatalf("help missing 'service': %s", out)
	}
	if !strings.Contains(out, "context") {
		t.Fatalf("help missing 'context': %s", out)
	}
	if !strings.Contains(out, "gen-key") {
		t.Fatalf("help missing 'gen-key': %s", out)
	}
	if !strings.Contains(out, "migrate") {
		t.Fatalf("help missing 'migrate': %s", out)
	}
	if !strings.Contains(out, "connect") {
		t.Fatalf("help missing 'connect': %s", out)
	}
	if !strings.Contains(out, "admin") {
		t.Fatalf("help missing 'admin': %s", out)
	}
	if !strings.Contains(out, "edge") {
		t.Fatalf("help missing 'edge': %s", out)
	}
	if strings.Contains(out, "play") {
		t.Fatalf("help should not include old Play UI command: %s", out)
	}
}

func TestEdgeHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"edge", "serve", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "serve <dir>") {
		t.Fatalf("edge serve help missing '<dir>': %s", out)
	}
}

func TestGenKey(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"gen-key"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	value := strings.TrimSpace(buf.String())
	var key giznet.Key
	if err := key.UnmarshalText([]byte(value)); err != nil {
		t.Fatalf("gen-key output is not a GizClaw key: %v, output=%q", err, value)
	}
	if _, err := giznet.NewKeyPair(key); err != nil {
		t.Fatalf("gen-key output cannot derive a key pair: %v", err)
	}
}

func TestGenKeyRejectsArgs(t *testing.T) {
	root := New()
	root.SetArgs([]string{"gen-key", "extra"})
	if err := root.Execute(); err == nil {
		t.Fatal("gen-key with args should fail")
	}
}

func TestServeHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"serve", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "serve <dir>") {
		t.Fatalf("serve help missing '<dir>': %s", out)
	}
	if strings.Contains(out, "--data-dir") || strings.Contains(out, "--listen") || strings.Contains(out, "--config") {
		t.Fatalf("serve help should not mention removed flags: %s", out)
	}
	for _, want := range []string{"--force"} {
		if !strings.Contains(out, want) {
			t.Fatalf("serve help missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "--bg") {
		t.Fatalf("serve help should not mention '--bg': %s", out)
	}
}

func TestServiceHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"service", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"install", "start", "stop", "uninstall"} {
		if !strings.Contains(out, want) {
			t.Fatalf("service help missing %q: %s", want, out)
		}
	}
}

func TestContextHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"context", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "create") {
		t.Fatalf("context help missing 'create': %s", out)
	}
	if !strings.Contains(out, "use") {
		t.Fatalf("context help missing 'use': %s", out)
	}
	if !strings.Contains(out, "list") {
		t.Fatalf("context help missing 'list': %s", out)
	}
	if !strings.Contains(out, "info") {
		t.Fatalf("context help missing 'info': %s", out)
	}
	if !strings.Contains(out, "show") {
		t.Fatalf("context help missing 'show': %s", out)
	}
}

func TestSetNameHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"connect", "set-name", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"set-name <name>", "--context"} {
		if !strings.Contains(out, want) {
			t.Fatalf("set-name help missing %q: %s", want, out)
		}
	}
}

func TestSetNameRejectsEmptyName(t *testing.T) {
	root := New()
	root.SetArgs([]string{"connect", "set-name", "   "})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "device name must not be empty") {
		t.Fatalf("set-name empty err=%v", err)
	}
}

func TestConnectHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"connect", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"ping", "server-info", "set-name", "say", "test-speed", "contact", "firmware", "friend", "friend-group"} {
		if !strings.Contains(out, want) {
			t.Fatalf("connect help missing %q: %s", want, out)
		}
	}
}

func TestPingHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"connect", "ping", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "--context") {
		t.Fatalf("ping help missing '--context': %s", out)
	}
}

func TestSayHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"connect", "say", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"say --voice <voice-alias> <text>", "--context", "--registration-token", "--voice", "--timeout"} {
		if !strings.Contains(out, want) {
			t.Fatalf("say help missing %q: %s", want, out)
		}
	}
}

func TestSayRejectsEmptyText(t *testing.T) {
	root := New()
	root.SetArgs([]string{"connect", "say", "--voice", "voice-1", "   "})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "text must not be empty") {
		t.Fatalf("say empty err=%v", err)
	}
}

func TestTestSpeedHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"connect", "test-speed", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"--context", "--up-content-length", "--down-content-length", "--timeout"} {
		if !strings.Contains(out, want) {
			t.Fatalf("test-speed help missing %q: %s", want, out)
		}
	}
}

func TestAdminHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"admin", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"apply", "delete", "show", "peers", "credentials", "minimax-tenants", "volc-tenants", "voices", "workflows", "workspaces"} {
		if !strings.Contains(out, want) {
			t.Fatalf("admin help missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "--listen") {
		t.Fatalf("admin help should not include old UI listen flag: %s", out)
	}
}

func TestAdminResourceHelp(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		wants    []string
		notWants []string
	}{
		{[]string{"admin", "credentials", "--help"}, []string{"list", "get"}, []string{"create", "put", "delete"}},
		{[]string{"admin", "minimax-tenants", "--help"}, []string{"list", "get", "sync-voices"}, []string{"create", "put", "delete"}},
		{[]string{"admin", "volc-tenants", "--help"}, []string{"list", "get", "sync-voices"}, []string{"create", "put", "delete"}},
		{[]string{"admin", "voices", "--help"}, []string{"list", "get"}, []string{"create", "put", "delete"}},
		{[]string{"admin", "workflows", "--help"}, []string{"list", "get"}, []string{"create", "put", "delete", "upload-icon", "download-icon", "delete-icon"}},
		{[]string{"admin", "workspaces", "--help"}, []string{"list", "get"}, []string{"create", "put", "delete"}},
	} {
		root := New()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetArgs(tc.args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		for _, want := range tc.wants {
			if !strings.Contains(out, want) {
				t.Fatalf("%v help missing %q: %s", tc.args, want, out)
			}
		}
		for _, notWant := range tc.notWants {
			if strings.Contains(out, "\n  "+notWant+" ") || strings.Contains(out, "\n    "+notWant+" ") {
				t.Fatalf("%v help should not include write command %q: %s", tc.args, notWant, out)
			}
		}
	}
}

func TestAdminHelpDoesNotExposeUIListen(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"admin", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "peers") || !strings.Contains(out, "credentials") {
		t.Fatalf("admin help missing subcommands: %s", out)
	}
	if strings.Contains(out, "--listen") {
		t.Fatalf("admin help should not include old UI listen flag: %s", out)
	}
}

func TestPlayCommandRemoved(t *testing.T) {
	root := New()
	root.SetArgs([]string{"play", "--help"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("play command err = %v, want unknown command", err)
	}
}

func TestAdminPeersHelp(t *testing.T) {
	root := New()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"admin", "peers", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"resolve-sn",
		"resolve-imei",
		"info",
		"runtime",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("admin peers help missing %q: %s", want, out)
		}
	}
}

func TestContextCreateMissingFlags(t *testing.T) {
	root := New()
	root.SetArgs([]string{"context", "create", "test"})
	err := root.Execute()
	if err == nil {
		t.Fatal("context create without required flags should fail")
	}
}

func TestServeRequiresWorkspaceArg(t *testing.T) {
	root := New()
	root.SetArgs([]string{"serve"})
	err := root.Execute()
	if err == nil {
		t.Fatal("serve without workspace arg should fail")
	}
}
