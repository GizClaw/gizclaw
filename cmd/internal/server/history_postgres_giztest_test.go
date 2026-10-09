package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/jmoiron/sqlx"
	"github.com/pion/webrtc/v4"
)

// The Eino script is deterministic, but Server, WebRTC, Giztest, Workspace
// history and internal Eino history all use their production implementations.
// A PostgreSQL trigger gives every history writer the same database cost.
func TestPostgreSQLHistoryGiztest(t *testing.T) {
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN is required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	table := fmt.Sprintf("gzc_history_giztest_%d", time.Now().UnixNano())
	schema := table + "_schema"
	bootstrap, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bootstrap.Close() })
	if _, err := bootstrap.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = bootstrap.Exec(`DROP SCHEMA "` + schema + `" CASCADE`) })
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("GIZCLAW_TEST_POSTGRES_DSN must be a PostgreSQL URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	dsn = parsed.String()
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `" CASCADE`)
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `_keys"`)
		_, _ = db.Exec(`DROP FUNCTION IF EXISTS "` + table + `_delay"() CASCADE`)
	})
	cfg := validLayeredConfig(t.TempDir())
	// Use PostgreSQL for metadata and run state too, matching the deployment
	// and keeping SQLite's single-connection metadata queue out of this load.
	cfg.Storage["business-db"] = storage.PostgreSQLConfig{DSN: dsn}
	cfg.Storage["peer-runs-db"] = storage.PostgreSQLConfig{DSN: dsn}
	cfg.Storage["history-pg"] = storage.PostgreSQLConfig{DSN: dsn}
	cfg.Stores["workspace-history"] = stores.Config{Kind: stores.KindLogMutable, Storage: "history-pg", Table: table, TTL: 30 * 24 * time.Hour}
	cfg.Stores["eino-state"] = stores.Config{Kind: stores.KindSQL, Storage: "business-db"}
	cfg.Services.AgentHost.Persistence = &AgentHostPersistenceConfig{StateStore: "eino-state", HistoryStore: "workspace-history"}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminPublicKey = key.Public
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(srv)
	srv.PublicEndpoint = strings.TrimPrefix(httpServer.URL, "http://")
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
		listener, err := (&gizwebrtc.ListenConfig{ICEAddr: "127.0.0.1:0", SecurityPolicy: opts.SecurityPolicy, PeerEventHandler: opts.PeerEventHandler}).Listen(opts.KeyPair)
		if err == nil {
			srv.WebRTCSignalingHandler = listener.SignalingHandler()
		}
		return listener, err
	}}
	if err := srv.Listen(); err != nil {
		httpServer.Close()
		_ = srv.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	settings := webrtc.SettingEngine{}
	settings.DetachDataChannels()
	settings.SetIncludeLoopbackCandidate(true)
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{API: webrtc.NewAPI(webrtc.WithSettingEngine(settings)), SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	t.Cleanup(func() {
		_ = admin.Close()
		httpServer.Close()
		if err := srv.Close(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	})
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()
	root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "history-postgres")
	data, err := os.ReadFile(filepath.Join(root, "echo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var graph apitypes.EinoWorkflowSpec
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "history-echo", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &graph}}); err != nil {
		t.Fatal(err)
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: "history-postgres", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"history-echo": {ResourceId: "history-echo", I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "History echo"}, "zh-CN": {DisplayName: "历史回声"}}}}}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "history-postgres", Token: "local-history-postgres", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_ENDPOINT", httpServer.URL)
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", "local-history-postgres")
	load, err := logstore.NewSQLStoreWithDBAndTTL(db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = load.Close() })
	if _, err := db.Exec(`CREATE FUNCTION "` + table + `_delay"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.030); RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER history_delay BEFORE INSERT ON "` + table + `" FOR EACH ROW EXECUTE FUNCTION "` + table + `_delay"()`); err != nil {
		t.Fatal(err)
	}
	const writers, batches = 96, 8
	db.SetMaxOpenConns(writers + 4)
	db.SetMaxIdleConns(writers + 4)
	errs := make([]error, writers)
	var workers sync.WaitGroup
	for writer := range writers {
		workers.Go(func() {
			for batch := range batches {
				_, err := load.Append(ctx, []logstore.Record{{Stream: fmt.Sprintf("load-workspace-%d", writer), ID: fmt.Sprintf("%d", batch), Kind: "message", Time: time.Now().UTC()}})
				if err != nil {
					errs[writer] = err
					return
				}
			}
		})
	}
	reportDir := os.Getenv("GIZCLAW_TEST_HISTORY_GIZTEST_EVIDENCE")
	if reportDir == "" {
		reportDir = t.TempDir()
	}
	if err := os.MkdirAll(reportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	command := giztestcmd.NewCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	reportPath := filepath.Join(reportDir, "giztest.json")
	command.SetArgs([]string{"run", "--parallel", "8", "--output", reportPath, filepath.Join(root, "history.giztest.yaml")})
	runErr := command.ExecuteContext(ctx)
	workers.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	var rows int
	if err := db.GetContext(ctx, &rows, `SELECT COUNT(*) FROM "`+table+`" WHERE stream LIKE 'load-workspace-%'`); err != nil {
		t.Fatal(err)
	}
	if rows != writers*batches {
		t.Errorf("load persisted %d rows, want %d", rows, writers*batches)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "runner.log"), output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("same load: %d independent streams, %d persisted rows, 30ms database work per row; report: %s\n%s", writers, rows, reportPath, output.String())
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err = os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Status string `json:"status"`
		Tasks  []struct {
			Status string `json:"status"`
			Steps  []struct {
				Status string `json:"status"`
			} `json:"steps"`
			Cleanup []struct {
				Status string `json:"status"`
			} `json:"cleanup"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "passed" || len(report.Tasks) != 8 {
		t.Fatalf("Giztest status=%s tasks=%d; want 8 passed tasks", report.Status, len(report.Tasks))
	}
	for _, task := range report.Tasks {
		if task.Status != "passed" || len(task.Steps) != 6 || len(task.Cleanup) != 3 {
			t.Fatal("Giztest did not execute every step and cleanup")
		}
		for _, step := range append(task.Steps, task.Cleanup...) {
			if step.Status != "passed" {
				t.Fatal("Giztest skipped or failed a step")
			}
		}
	}
}
