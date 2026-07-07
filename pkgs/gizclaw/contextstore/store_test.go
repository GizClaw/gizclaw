package contextstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestStoreCreateLoadListEndpointContext(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	if err := store.CreateWithOptions("local", "127.0.0.1:9820", CreateOptions{
		Description: "Local dev",
	}); err != nil {
		t.Fatalf("CreateWithOptions() error = %v", err)
	}
	ctx, err := store.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if ctx == nil || ctx.Name != "local" {
		t.Fatalf("Current() = %#v", ctx)
	}
	if ctx.Config.Description != "Local dev" || ctx.Config.Server.Endpoint != "127.0.0.1:9820" {
		t.Fatalf("config = %#v", ctx.Config)
	}
	if ctx.Config.Identity.PrivateKey.IsZero() || ctx.KeyPair == nil {
		t.Fatalf("identity not stored in config: cfg=%#v keyPair=%#v", ctx.Config.Identity, ctx.KeyPair)
	}
	entries, err := os.ReadDir(filepath.Join(store.Root, "local"))
	if err != nil {
		t.Fatalf("ReadDir context error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != ConfigFile {
		t.Fatalf("context files = %#v, want only %s", entries, ConfigFile)
	}
	summaries, err := store.ListSummaries()
	if err != nil {
		t.Fatalf("ListSummaries() error = %v", err)
	}
	if len(summaries) != 1 || !summaries[0].Current || summaries[0].LocalPublicKey.IsZero() {
		t.Fatalf("summaries = %#v", summaries)
	}
}

func TestStoreUseLoadByNameAndDelete(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	for _, name := range []string{"alpha", "beta"} {
		if err := store.Create(name, "127.0.0.1:9820"); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	if err := store.Use("beta"); err != nil {
		t.Fatalf("Use(beta) error = %v", err)
	}
	current, err := store.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if current == nil || current.Name != "beta" {
		t.Fatalf("Current() = %#v", current)
	}
	loaded, err := store.LoadByName("alpha")
	if err != nil {
		t.Fatalf("LoadByName(alpha) error = %v", err)
	}
	if loaded.Name != "alpha" {
		t.Fatalf("LoadByName(alpha).Name = %q", loaded.Name)
	}

	if err := store.Delete("alpha"); err != nil {
		t.Fatalf("Delete(alpha) error = %v", err)
	}
	if _, err := store.LoadByName("alpha"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("LoadByName(alpha) after delete error = %v", err)
	}
	current, err = store.Current()
	if err != nil {
		t.Fatalf("Current() after non-current delete error = %v", err)
	}
	if current == nil || current.Name != "beta" {
		t.Fatalf("Current() after non-current delete = %#v", current)
	}

	if err := store.Delete("beta"); err != nil {
		t.Fatalf("Delete(beta) error = %v", err)
	}
	current, err = store.Current()
	if err != nil {
		t.Fatalf("Current() after current delete error = %v", err)
	}
	if current != nil {
		t.Fatalf("Current() after current delete = %#v", current)
	}
	names, currentName, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(names) != 0 || currentName != "" {
		t.Fatalf("List() after deletes = names %#v current %q", names, currentName)
	}
}

func TestStoreUseLoadByNameAndDeleteRejectInvalidOrMissingNames(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	for _, tc := range []struct {
		name string
		run  func(string) error
	}{
		{name: "Use", run: store.Use},
		{name: "Delete", run: store.Delete},
		{name: "LoadByName", run: func(name string) error {
			_, err := store.LoadByName(name)
			return err
		}},
	} {
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			if err := tc.run("../bad"); err == nil || !strings.Contains(err.Error(), "invalid name") {
				t.Fatalf("%s invalid error = %v", tc.name, err)
			}
		})
		t.Run(tc.name+"/missing", func(t *testing.T) {
			if err := tc.run("missing"); err == nil || !strings.Contains(err.Error(), "does not exist") {
				t.Fatalf("%s missing error = %v", tc.name, err)
			}
		})
	}
}

func TestStoreCreateRejectsDuplicateName(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	if err := store.Create("local", "127.0.0.1:9820"); err != nil {
		t.Fatalf("Create(local) error = %v", err)
	}
	err := store.Create("local", "127.0.0.1:9820")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate Create() error = %v", err)
	}
}

func TestContextHelpersAndSummary(t *testing.T) {
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
description: Manual
identity:
  private-key: `+clientKey.Private.String()+`
server:
  endpoint: 127.0.0.1:9820
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Server.PublicAPIAddr() != "127.0.0.1:9820" {
		t.Fatalf("PublicAPIAddr() = %q", cfg.Server.PublicAPIAddr())
	}

	summary, err := LoadSummary(dir)
	if err != nil {
		t.Fatalf("LoadSummary() error = %v", err)
	}
	if summary.Name != filepath.Base(dir) || summary.Description != "Manual" || summary.LocalPublicKey != clientKey.Public {
		t.Fatalf("LoadSummary() = %#v", summary)
	}
}

func TestLoadConfigNormalizesIdentityPrivateKey(t *testing.T) {
	var rawPrivate giznet.Key
	for i := range rawPrivate {
		rawPrivate[i] = 0xff
	}
	want, err := giznet.NewKeyPair(rawPrivate)
	if err != nil {
		t.Fatalf("NewKeyPair error = %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
identity:
  private-key: `+rawPrivate.String()+`
server:
  endpoint: 127.0.0.1:9820
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Identity.PrivateKey != want.Private {
		t.Fatalf("identity private key = %s, want normalized %s", cfg.Identity.PrivateKey, want.Private)
	}
}

func TestLoadConfigRejectsMissingAndMalformedConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "bad-yaml", body: "server: [", want: "parse config"},
		{name: "missing-identity", body: "server:\n  endpoint: 127.0.0.1:9820\n", want: "missing identity.private-key"},
		{name: "missing-endpoint", body: "identity:\n  private-key: " + testPrivateKey(t, 0xaa).String() + "\nserver: {}\n", want: "missing server.endpoint"},
		{name: "url-endpoint", body: "identity:\n  private-key: " + testPrivateKey(t, 0xaa).String() + "\nserver:\n  endpoint: http://127.0.0.1:9820\n", want: "host:port"},
		{name: "empty-host", body: "identity:\n  private-key: " + testPrivateKey(t, 0xaa).String() + "\nserver:\n  endpoint: :9820\n", want: "host is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadConfig() error = %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("missing-file", func(t *testing.T) {
		if _, err := LoadConfig(t.TempDir()); err == nil || !strings.Contains(err.Error(), "read config") {
			t.Fatalf("LoadConfig() missing file error = %v", err)
		}
	})
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
identity:
  private-key: 11111111111111111111111111111111
server:
  endpoint: 127.0.0.1:9820
  extra: value
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestCreateRejectsEndpointURL(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	err := store.Create("bad", "http://127.0.0.1:9820")
	if err == nil || !strings.Contains(err.Error(), "host:port") {
		t.Fatalf("Create() error = %v", err)
	}
}

func testPrivateKey(t *testing.T, fill byte) giznet.Key {
	t.Helper()
	var key giznet.Key
	for i := range key {
		key[i] = fill
	}
	kp, err := giznet.NewKeyPair(key)
	if err != nil {
		t.Fatalf("NewKeyPair error = %v", err)
	}
	return kp.Private
}
