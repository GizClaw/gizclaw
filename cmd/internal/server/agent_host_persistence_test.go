package server

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/graphstate"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
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
		{"missing history", AgentHostPersistenceConfig{HistoryStore: "missing"}, "services.agent_host.persistence.history_store"},
		{"non-log history", AgentHostPersistenceConfig{HistoryStore: "peers"}, "requires logstore.MutableStore"},
		{"immutable history", AgentHostPersistenceConfig{HistoryStore: "immutable"}, "requires logstore.MutableStore"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := validLayeredConfig(t.TempDir())
			cfg.Services.AgentHost.Persistence = &test.binding
			cfg.Stores["immutable"] = stores.Config{Kind: stores.KindLogImmutable, Storage: "business-db", Table: "immutable_history"}
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
	cfg.Stores["shared-history"] = stores.Config{Kind: stores.KindLogMutable, Storage: "business-db", Table: "gizclaw_flowcraft_history"}
	cfg.Services.AgentHost.Persistence = &AgentHostPersistenceConfig{StateStore: "shared-state", HistoryStore: "shared-history"}
	scope := agenthost.WorkspaceAgentScope("owner", "workspace", "workspace")
	record := logstore.Record{
		ID: "existing-message", Time: time.Now().UTC().Truncate(time.Millisecond), Stream: "eino.history", Kind: "message",
		Attributes: map[string]string{"scope": scope, "agent_id": "workspace", "context_id": scope, "schema_version": "1"},
		Payload:    []byte(`{"version":1,"message":{"role":"user","content":"retained conversation"}}`),
	}
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
		state, err := graphstate.OpenScope(ctx, db, "owner", "workspace", "workspace")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.CompareAndSwap(ctx, scope, "", map[string]any{"score": int64(9007199254740993)}); err != nil {
			t.Fatal(err)
		}
		if err := db.GetContext(ctx, &originalState, "SELECT state_json FROM graph_states"); err != nil {
			t.Fatal(err)
		}
		if err := graphstate.RetireWorkspace(ctx, db, "owner", "retired-workspace"); err != nil {
			t.Fatal(err)
		}
		history, err := registry.MutableLog("shared-history")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := history.Append(ctx, []logstore.Record{record}); err != nil {
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
	if srv.GraphStateDB == nil || srv.AgentHistory == nil || manager == nil || manager.GraphStateDB != srv.GraphStateDB || manager.AgentHistory != srv.AgentHistory {
		t.Fatal("Server did not inject the shared persistence into the Peer Manager")
	}
	var restoredState string
	if err := srv.GraphStateDB.GetContext(ctx, &restoredState, "SELECT state_json FROM graph_states"); err != nil || restoredState != originalState {
		t.Fatalf("checkpoint changed: %q, error %v", restoredState, err)
	}
	state, err := graphstate.OpenScope(ctx, srv.GraphStateDB, "owner", "workspace", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.Load(ctx, scope)
	if err != nil || snapshot.Fields["score"] != int64(9007199254740993) {
		t.Fatalf("restored checkpoint = %+v, error %v", snapshot, err)
	}
	if _, err := graphstate.OpenScope(ctx, srv.GraphStateDB, "owner", "retired-workspace", "retired-workspace"); !errors.Is(err, graphstate.ErrRetired) {
		t.Fatalf("retirement fence was not preserved: %v", err)
	}
	page, err := srv.AgentHistory.Query(ctx, logstore.Query{
		Streams: []string{"eino.history"}, Kinds: []string{"message"}, Limit: 10, Order: logstore.OrderAsc,
		Start: record.Time.Add(-time.Second), End: record.Time.Add(time.Second),
		Matchers: []logstore.AttributeMatcher{{Name: "scope", Op: logstore.MatchEqual, Value: scope}},
	})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("existing history = %+v, error %v", page, err)
	}
	got := page.Records[0]
	if got.ID != record.ID || !got.Time.Equal(record.Time) || string(got.Payload) != string(record.Payload) || !reflect.DeepEqual(got.Attributes, record.Attributes) {
		t.Fatalf("history changed: %+v", got)
	}
}
