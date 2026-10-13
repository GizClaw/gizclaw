package server

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/graphstate"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestNewAgentHostPersistenceRejectsStoreCapabilities(t *testing.T) {
	for _, test := range []struct {
		name    string
		binding AgentHostPersistenceConfig
		want    string
	}{
		{"missing state", AgentHostPersistenceConfig{StateStore: "missing"}, "services.agent_host.persistence.state_store"},
		{"non-SQL state", AgentHostPersistenceConfig{StateStore: "peers"}, "requires *sqlx.DB"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := validLayeredConfig(t.TempDir())
			cfg.Services.AgentHost.Persistence = &test.binding
			if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewAgentHostPersistencePreservesExistingData(t *testing.T) {
	assertAgentHostPersistencePreservesExistingData(t, validLayeredConfig(t.TempDir()))
}

func TestNewAgentHostPersistencePreservesPostgresData(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is not set")
	}
	cfg := validLayeredConfig(t.TempDir())
	cfg.Storage["business-db"] = storage.PostgreSQLConfig{DSN: dsn}
	assertAgentHostPersistencePreservesExistingData(t, cfg)
}

func assertAgentHostPersistencePreservesExistingData(t *testing.T, cfg Config) {
	t.Helper()
	ctx := t.Context()
	cfg.Stores["shared-state"] = stores.Config{Kind: stores.KindSQL, Storage: "business-db"}
	cfg.Services.AgentHost.Persistence = &AgentHostPersistenceConfig{StateStore: "shared-state"}
	workspaceID := "workspace-" + strings.ToLower(rand.Text())
	scope := agenthost.WorkspaceAgentScope("owner", workspaceID, workspaceID)
	record := memory.HistoryEntry{ConversationID: workspaceID, ID: "existing-message", Role: memory.RoleUser, Name: "user", Text: "retained conversation", CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	var originalState string
	// Seed the existing physical stores independently of service bindings.
	func() {
		registry, physical, err := newStoreRegistry(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer physical.Close()
		defer registry.Close()
		db, err := registry.SQL("shared-state")
		if err != nil {
			t.Fatal(err)
		}
		if err := graphstate.Initialize(ctx, db); err != nil {
			t.Fatal(err)
		}
		state, err := graphstate.OpenScope(ctx, db, "owner", workspaceID, workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.CompareAndSwap(ctx, scope, "", map[string]any{"score": int64(9007199254740993)}); err != nil {
			t.Fatal(err)
		}
		if err := db.GetContext(ctx, &originalState, db.Rebind("SELECT state_json FROM graph_states WHERE owner_id=? AND workspace_id=?"), "owner", workspaceID); err != nil {
			t.Fatal(err)
		}
		if err := graphstate.RetireWorkspace(ctx, db, "owner", workspaceID+"-retired"); err != nil {
			t.Fatal(err)
		}
		history, err := memory.NewSQLHistoryStore(ctx, db, memory.DefaultHistoryTable, 30*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if err := history.Append(ctx, []memory.HistoryEntry{record}); err != nil {
			t.Fatal(err)
		}

	}()

	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	srv.PeerListenerFactories = nil
	srv.PeerListeners = []giznet.Listener{closedPeerListener{}}
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	manager := srv.Manager()
	if srv.GraphStateDB == nil || srv.WorkspaceHistory == nil || manager == nil || manager.GraphStateDB != srv.GraphStateDB {
		t.Fatal("Server did not inject the shared persistence into the Peer Manager")
	}
	var restoredState string
	if err := srv.GraphStateDB.GetContext(ctx, &restoredState, srv.GraphStateDB.Rebind("SELECT state_json FROM graph_states WHERE owner_id=? AND workspace_id=?"), "owner", workspaceID); err != nil || restoredState != originalState {
		t.Fatalf("checkpoint changed: %q, error %v", restoredState, err)
	}
	state, err := graphstate.OpenScope(ctx, srv.GraphStateDB, "owner", workspaceID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.Load(ctx, scope)
	if err != nil || snapshot.Fields["score"] != int64(9007199254740993) {
		t.Fatalf("restored checkpoint = %+v, error %v", snapshot, err)
	}
	if _, err := graphstate.OpenScope(ctx, srv.GraphStateDB, "owner", workspaceID+"-retired", workspaceID+"-retired"); !errors.Is(err, graphstate.ErrRetired) {
		t.Fatalf("retirement fence was not preserved: %v", err)
	}
	got, err := srv.WorkspaceHistory.Get(ctx, record.Key())
	if err != nil || got.ID != record.ID || got.Text != record.Text || !got.CreatedAt.Equal(record.CreatedAt) {
		t.Fatalf("workspace history changed: %+v err=%v", got, err)
	}
}
