package gizclaw

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
)

func mustBadgerInMemory(t testing.TB, opts *kv.Options) kv.Store {
	t.Helper()
	store, err := kv.NewBadgerInMemory(opts)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func completeTestServer(t testing.TB, server *Server) *Server {
	t.Helper()
	base := mustBadgerInMemory(t, nil)
	set := func(target *kv.Store, prefix string) {
		if *target == nil {
			*target = kv.Prefixed(base, kv.Key{prefix})
		}
	}
	set(&server.PeerStore, "peers")
	set(&server.PublicLoginStore, "public-login")
	set(&server.CredentialStore, "credentials")
	set(&server.FirmwareStore, "firmwares")
	set(&server.RuntimeProfileStore, "runtime-profiles")
	set(&server.ModelStore, "models")
	set(&server.VoiceStore, "voices")
	set(&server.MemoryLayoutStore, "memory-layouts")
	set(&server.ProviderTenantStore, "provider-tenants")
	set(&server.WorkflowStore, "workflows")
	set(&server.WorkspaceStore, "workspaces")
	set(&server.ToolStore, "tools")
	set(&server.ContactStore, "contacts")
	set(&server.FriendStore, "friends")
	set(&server.FriendGroupStore, "friend-groups")
	set(&server.GameplayStore, "gameplay")
	if server.WorkspaceAssets == nil {
		server.WorkspaceAssets = objectstore.Dir(t.TempDir())
	}
	if server.GameplayAssets == nil {
		server.GameplayAssets = objectstore.Dir(t.TempDir())
	}
	if server.GameplayDB == nil {
		db, err := sqlx.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("open test gameplay database: %v", err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		server.GameplayDB = db
	}
	return server
}

func mustOpenAIModelProviderData(t testing.TB, upstreamModel string) apitypes.ModelProviderData {
	t.Helper()
	falseValue := false
	var data apitypes.ModelProviderData
	if err := data.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{
		UpstreamModel:      upstreamModel,
		SupportJsonOutput:  &falseValue,
		SupportToolCalls:   &falseValue,
		SupportTextOnly:    &falseValue,
		UseSystemRole:      &falseValue,
		SupportTemperature: &falseValue,
		SupportThinking:    &falseValue,
	}); err != nil {
		t.Fatalf("FromOpenAITenantModelProviderData() error = %v", err)
	}
	return data
}
