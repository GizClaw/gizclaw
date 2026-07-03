package contextstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestStoreCreateLoadListEndpointContext(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: t.TempDir()}
	if err := store.CreateWithOptions("local", "127.0.0.1:9820", CreateOptions{
		Description:     "Local dev",
		ServerPublicKey: serverKey.Public.String(),
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
	if ctx.Config.Server.SignalingURL() != "http://127.0.0.1:9820/webrtc/v1/offer" {
		t.Fatalf("SignalingURL() = %q", ctx.Config.Server.SignalingURL())
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
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: t.TempDir()}
	for _, name := range []string{"alpha", "beta"} {
		if err := store.Create(name, "127.0.0.1:9820", serverKey.Public.String()); err != nil {
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

func TestStoreCreateRejectsInvalidServerPublicKey(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	for _, tc := range []struct {
		name      string
		publicKey string
		want      string
	}{
		{name: "missing", publicKey: "", want: "missing server public key"},
		{name: "invalid", publicKey: "not-a-public-key", want: "invalid server public key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Create("ctx-"+tc.name, "127.0.0.1:9820", tc.publicKey)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Create() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStoreCreateRejectsDuplicateName(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: t.TempDir()}
	if err := store.Create("local", "127.0.0.1:9820", serverKey.Public.String()); err != nil {
		t.Fatalf("Create(local) error = %v", err)
	}
	err = store.Create("local", "127.0.0.1:9820", serverKey.Public.String())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate Create() error = %v", err)
	}
}

func TestContextHelpersAndSummaryWithoutIdentity(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
description: Manual
server:
  endpoint: 127.0.0.1:9820
  public-key: `+serverKey.Public.String()+`
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
	ctx := &Context{Config: cfg}
	gotKey, err := ctx.ServerPublicKey()
	if err != nil {
		t.Fatalf("ServerPublicKey() error = %v", err)
	}
	if gotKey != serverKey.Public {
		t.Fatalf("ServerPublicKey() = %v, want %v", gotKey, serverKey.Public)
	}

	summary, err := LoadSummary(dir)
	if err != nil {
		t.Fatalf("LoadSummary() without identity error = %v", err)
	}
	if summary.Name != filepath.Base(dir) || summary.Description != "Manual" || !summary.LocalPublicKey.IsZero() {
		t.Fatalf("LoadSummary() = %#v", summary)
	}
}

func TestLoadConfigRejectsMissingAndMalformedConfig(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "bad-yaml", body: "server: [", want: "parse config"},
		{name: "missing-endpoint", body: "server:\n  public-key: " + serverKey.Public.String() + "\n", want: "missing server.endpoint"},
		{name: "url-endpoint", body: "server:\n  endpoint: http://127.0.0.1:9820\n  public-key: " + serverKey.Public.String() + "\n", want: "host:port"},
		{name: "missing-public-key", body: "server:\n  endpoint: 127.0.0.1:9820\n", want: "missing server.public-key"},
		{name: "empty-host", body: "server:\n  endpoint: :9820\n  public-key: " + serverKey.Public.String() + "\n", want: "host is empty"},
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

func TestLoadIdentityRejectsInvalidKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), IdentityFile)
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIdentity(path); err == nil || !strings.Contains(err.Error(), "invalid key file") {
		t.Fatalf("LoadIdentity() error = %v", err)
	}
}

func TestLoadRejectsOldNoiseFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`
server:
  host: 127.0.0.1
  public-api-port: 9820
  public-key: 11111111111111111111111111111111
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil || !strings.Contains(err.Error(), "server.host is not supported") {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestCreateRejectsEndpointURL(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{Root: t.TempDir()}
	err = store.Create("bad", "http://127.0.0.1:9820", serverKey.Public.String())
	if err == nil || !strings.Contains(err.Error(), "host:port") {
		t.Fatalf("Create() error = %v", err)
	}
}
