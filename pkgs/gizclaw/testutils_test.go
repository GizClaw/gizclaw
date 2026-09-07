package gizclaw

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
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
	if server.PublicEndpoint == "" {
		server.PublicEndpoint = "server.test:9820"
	}
	base := mustBadgerInMemory(t, nil)
	set := func(target *kv.Store, prefix string) {
		if *target == nil {
			*target = kv.Prefixed(base, kv.Key{prefix})
		}
	}
	set(&server.PeerStore, "peers")
	set(&server.APIKeyStore, "api-keys")
	set(&server.FriendStore, "friends")
	set(&server.FriendGroupStore, "friend-groups")
	if server.WorkspaceAssets == nil {
		server.WorkspaceAssets = newTestObjectStore(t)
	}
	if server.GameplayAssets == nil {
		server.GameplayAssets = newTestObjectStore(t)
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
	if server.WorkspaceDB == nil {
		server.WorkspaceDB = server.GameplayDB
	}
	if server.ProviderTenantDB == nil {
		server.ProviderTenantDB = server.GameplayDB
	}
	if server.GameplayCatalogDB == nil {
		server.GameplayCatalogDB = server.GameplayDB
	}
	if server.RuntimeProfileDB == nil {
		server.RuntimeProfileDB = server.GameplayDB
	}
	if server.VoiceDB == nil {
		server.VoiceDB = server.GameplayDB
	}
	if server.CredentialDB == nil {
		server.CredentialDB = server.GameplayDB
	}
	if server.ModelDB == nil {
		server.ModelDB = server.GameplayDB
	}
	if server.WorkflowDB == nil {
		server.WorkflowDB = server.GameplayDB
	}
	if server.ContactDB == nil {
		server.ContactDB = server.GameplayDB
	}
	if server.MemoryLayoutDB == nil {
		server.MemoryLayoutDB = server.GameplayDB
	}
	if server.ToolDB == nil {
		server.ToolDB = server.GameplayDB
	}
	if server.FirmwareDB == nil {
		server.FirmwareDB = server.GameplayDB
	}
	if server.PeerRunDB == nil {
		server.PeerRunDB = server.GameplayDB
	}
	if server.WorkspaceHistory == nil {
		store, err := logstore.NewSQLStoreWithDB(server.GameplayDB, "workspace_history")
		if err != nil {
			t.Fatalf("open test workspace history: %v", err)
		}
		server.WorkspaceHistory = store
	}
	if server.WorkspaceHistoryAssets == nil {
		server.WorkspaceHistoryAssets = newTestObjectStore(t)
	}
	// Registered after every store cleanup above, so cleanup order (last in,
	// first out) always stops the Server, and with it the pending deletion
	// processor, before the stores it scans are closed.
	t.Cleanup(func() { _ = server.Close() })
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
