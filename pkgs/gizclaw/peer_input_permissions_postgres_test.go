package gizclaw

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

// The social binding resolver is deterministic; run state and the permission
// worker/deadline/admission path use their real PostgreSQL implementations.
func TestPostgreSQLInputPermissionRevocationAndFailure(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("requires local PostgreSQL URL")
	}
	bootstrap, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bootstrap.Close() })
	schema := fmt.Sprintf("input_permission_%d", time.Now().UnixNano())
	if _, err := bootstrap.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := bootstrap.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	q := parsed.Query()
	q.Set("search_path", schema)
	q.Set("application_name", schema)
	parsed.RawQuery = q.Encode()
	physical, err := storage.New(map[string]storage.Config{"run": storage.PostgreSQLConfig{DSN: parsed.String()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	db, err := physical.SQL("run")
	if err != nil {
		t.Fatal(err)
	}
	runs := &peerrun.Server{DB: db}
	if err := runs.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	peer, bindings := permissionTestPeer(t)
	unsubscribe, err := peer.events.Subscribe(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unsubscribe)
	peer.Service.manager.PeerRun = runs
	selectWorkspace := func(name string) {
		t.Helper()
		selection := apitypes.AgentSelection{WorkspaceName: name}
		if _, err := runs.SetRunAgent(t.Context(), peer.Conn.PublicKey(), selection); err != nil {
			t.Fatal(err)
		}
		if _, err := runs.ActivateRunAgent(t.Context(), peer.Conn.PublicKey(), selection); err != nil {
			t.Fatal(err)
		}
	}
	selectWorkspace(testSFUWorkspaceName)
	if allowed, err := peer.authorizeInputEvent(t.Context(), audioBOS("postgres-sfu")); err != nil || !allowed {
		t.Fatalf("SFU BOS: %v %v", allowed, err)
	}
	if allowed, err := peer.authorizeAudioPacket(t.Context()); err != nil || !allowed {
		t.Fatalf("SFU packet: %v %v", allowed, err)
	}
	peer.startInputAccessRefresh()
	bindings.set(sfu.ErrNotMember)
	deadline := time.NewTimer(peerPermissionMaxAge + time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for peer.permissionAllowsAudio(0) {
		select {
		case <-deadline.C:
			t.Fatal("background PG refresh did not revoke audio")
		case <-tick.C:
		}
	}
	value := peer.inputPermission(t.Context(), false)
	if value.denial == nil || value.denial.Code != sfuAccessRevokedCode {
		t.Fatalf("revocation: %+v", value)
	}
	if allowed, err := peer.authorizeAudioPacket(t.Context()); err != nil || allowed {
		t.Fatalf("revoked packet: %v %v", allowed, err)
	}
	if _, err := peer.authorizeInputEvent(t.Context(), audioEndEvent("postgres-sfu")); err != nil {
		t.Fatal(err)
	}
	// Ordinary workflows must not depend on a successful SFU binding lookup.
	selectWorkspace(testWorkflowWorkspaceName)
	value = peer.inputPermission(t.Context(), true)
	if value.denial != nil || value.isSFU || value.run.workspaceName != testWorkflowWorkspaceName {
		t.Fatalf("ordinary workflow selection: %+v", value)
	}
	if allowed, err := peer.authorizeInputEvent(t.Context(), audioBOS("postgres-ordinary")); err != nil || !allowed {
		t.Fatalf("ordinary BOS: %v %v", allowed, err)
	}
	if allowed, err := peer.authorizeAudioPacket(t.Context()); err != nil || !allowed {
		t.Fatalf("ordinary packet: %v %v", allowed, err)
	}
	// A genuine PG lock is a negative control: pooling must not turn database
	// failure into permission success or extend the existing two-second budget.
	blocker, err := bootstrap.BeginTxx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.ExecContext(t.Context(), "LOCK TABLE "+schema+".peer_runs IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	value = peer.inputPermission(t.Context(), true)
	if value.denial == nil || value.denial.Code != sfuAccessCheckFailedCode {
		t.Fatalf("PG timeout admitted input: %+v", value)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("permission exceeded its budget: %s", elapsed)
	}
	if peer.permissionAllowsAudio(0) {
		t.Fatal("failed refresh admitted cached audio")
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value := peer.inputPermission(t.Context(), true); value.denial != nil {
		t.Fatalf("pool did not recover: %+v", value)
	}
	peer.stopInputAccessRefresh()
	if stats := db.Stats(); stats.InUse != 0 {
		t.Fatalf("permission queries leaked connections: %+v", stats)
	}
}
