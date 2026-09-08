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
	if server.WorkspaceDB == nil {
		db, err := sqlx.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("open test database: %v", err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		server.WorkspaceDB = db
	}
	if server.ProviderTenantDB == nil {
		server.ProviderTenantDB = server.WorkspaceDB
	}
	if server.RuntimeProfileDB == nil {
		server.RuntimeProfileDB = server.WorkspaceDB
	}
	if server.VoiceDB == nil {
		server.VoiceDB = server.WorkspaceDB
	}
	if server.CredentialDB == nil {
		server.CredentialDB = server.WorkspaceDB
	}
	if server.ModelDB == nil {
		server.ModelDB = server.WorkspaceDB
	}
	if server.WorkflowDB == nil {
		server.WorkflowDB = server.WorkspaceDB
	}
	if server.ContactDB == nil {
		server.ContactDB = server.WorkspaceDB
	}
	if server.MemoryLayoutDB == nil {
		server.MemoryLayoutDB = server.WorkspaceDB
	}
	if server.ToolDB == nil {
		server.ToolDB = server.WorkspaceDB
	}
	if server.FirmwareDB == nil {
		server.FirmwareDB = server.WorkspaceDB
	}
	if server.PeerRunDB == nil {
		server.PeerRunDB = server.WorkspaceDB
	}
	if server.WorkspaceHistory == nil {
		store, err := logstore.NewSQLStoreWithDB(server.WorkspaceDB, "workspace_history")
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
