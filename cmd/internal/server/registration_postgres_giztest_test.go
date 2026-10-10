package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// This opt-in load test uses the production Server, WebRTC, Go Giztest driver,
// PostgreSQL and Redis. Only the HTTP rendezvous and optional database delay
// belong to the fixture. Each generated document connects a distinct public key.
func TestPostgreSQLRegistrationGiztest(t *testing.T) {
	dsn, redisURL := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN"), os.Getenv("GIZCLAW_TEST_REDIS_URL")
	if dsn == "" || redisURL == "" {
		t.Skip("GIZCLAW_TEST_POSTGRES_DSN and GIZCLAW_TEST_REDIS_URL are required")
	}
	concurrency := 64
	if value := os.Getenv("GIZCLAW_TEST_REGISTRATION_CONCURRENCY"); value != "" {
		var err error
		concurrency, err = strconv.Atoi(value)
		if err != nil || concurrency < 2 || concurrency > 128 {
			t.Fatal("registration concurrency must be between 2 and 128")
		}
	}
	evidence := os.Getenv("GIZCLAW_TEST_REGISTRATION_EVIDENCE")
	if evidence == "" {
		evidence = t.TempDir()
	}
	for _, scenario := range []struct {
		name        string
		independent bool
		prefill     int
		delay       bool
		limited     bool
	}{
		{name: "shared"},
		{name: "independent", independent: true},
		{name: "shared-populated", prefill: 20000},
		{name: "shared-delay", delay: true},
		{name: "independent-delay", independent: true, delay: true},
		{name: "shared-limited", limited: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			root := filepath.Join(evidence, scenario.name)
			documents := filepath.Join(root, "documents")
			if err := os.MkdirAll(documents, 0o700); err != nil {
				t.Fatal(err)
			}
			db, srv, endpoint, admin, application := registrationLoadServer(t, ctx, dsn, redisURL)
			if _, err := adminapi.CreateRuntimeProfile(ctx, admin, adminhttp.RuntimeProfileUpsert{Id: "registration-load", Spec: apitypes.RuntimeProfileSpec{}}); err != nil {
				t.Fatal(err)
			}
			tokens := 1
			if scenario.independent {
				tokens = concurrency
			}
			for i := range tokens {
				id := fmt.Sprintf("registration-load-%d", i)
				body := adminhttp.RegistrationTokenUpsert{Id: id, Token: id, RuntimeProfileId: "registration-load"}
				if scenario.limited {
					body.MaxActivations = new(int64(concurrency))
				}
				if _, err := adminapi.CreateRegistrationToken(ctx, admin, body); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.prefill > 0 {
				if _, err := db.ExecContext(ctx, `INSERT INTO registration_token_activations(token_id,peer_public_key,activated_at)
					SELECT 'registration-load-0','fixture-' || n, '2026-01-01T00:00:00Z' FROM generate_series(1,$1) n`, scenario.prefill); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.delay {
				// A labelled diagnostic control, not a production latency estimate.
				// Delay a per-owner write, after the activation has been recorded.
				if _, err := db.ExecContext(ctx, `CREATE FUNCTION registration_delay() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.020); RETURN NEW; END $$;
					CREATE TRIGGER registration_delay BEFORE INSERT ON runtime_profile_owners FOR EACH ROW EXECUTE FUNCTION registration_delay()`); err != nil {
					t.Fatal(err)
				}
			}
			// Separate documents allow a different token per client in the control.
			// This rendezvous excludes connection setup from registration timing.
			barriers := map[string]*giztest.TaskBarrier{
				"/register": giztest.NewTaskBarrier(concurrency),
				"/retry":    giztest.NewTaskBarrier(concurrency),
			}
			gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				barrier := barriers[r.URL.Path]
				if barrier == nil {
					http.NotFound(w, r)
					return
				}
				if err := barrier.Wait(r.Context()); err != nil {
					http.Error(w, err.Error(), http.StatusRequestTimeout)
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(gate.Close)
			for i := range concurrency {
				token := fmt.Sprintf("registration-load-%d", i%tokens)
				doc := registrationLoadDocument(t, endpoint, gate.URL, token, i)
				writeRegistrationEvidence(t, filepath.Join(documents, fmt.Sprintf("%03d.giztest.yaml", i)), doc)
			}
			stopMonitor := monitorRegistrationPG(t, ctx, db, application, filepath.Join(root, "pg-waits.jsonl"))
			command := giztestcmd.NewCmd()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			reportPath := filepath.Join(root, "giztest.json")
			command.SetArgs([]string{"run", documents, "--parallel", strconv.Itoa(concurrency), "--seed", "1", "--output", reportPath})
			runErr := command.ExecuteContext(ctx)
			stopMonitor()
			if err := os.WriteFile(filepath.Join(root, "runner.log"), output.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			if runErr != nil {
				t.Fatalf("Giztest: %v; report: %s\n%s", runErr, reportPath, output.String())
			}
			var activations, owners int
			if err := db.GetContext(ctx, &activations, "SELECT COUNT(*) FROM registration_token_activations"); err != nil {
				t.Fatal(err)
			}
			if err := db.GetContext(ctx, &owners, "SELECT COUNT(*) FROM runtime_profile_owners"); err != nil {
				t.Fatal(err)
			}
			if activations != concurrency+scenario.prefill || owners != concurrency {
				t.Fatalf("activations=%d owners=%d, want %d/%d", activations, owners, concurrency+scenario.prefill, concurrency)
			}
			var report giztest.Report
			data, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if report.Status != "passed" || len(report.Tasks) != concurrency {
				t.Fatalf("status=%s tasks=%d", report.Status, len(report.Tasks))
			}
			for _, task := range report.Tasks {
				if task.Status != "passed" || len(task.Steps) != 7 {
					t.Fatalf("incomplete task: %s", task.TaskID)
				}
				for _, step := range task.Steps {
					if step.Status != "passed" {
						t.Fatalf("incomplete step: %s/%s", task.TaskID, step.ID)
					}
				}
			}
			writeRegistrationEvidence(t, filepath.Join(root, "context.json"), map[string]any{
				"concurrency": concurrency, "tokens": tokens, "prefill": scenario.prefill,
				"limited":               scenario.limited,
				"owner_insert_delay_ms": map[bool]int{true: 20}[scenario.delay],
				"activations":           activations, "owners": owners, "server_public_key": srv.PublicKey().String(),
			})
			t.Logf("%s: %d devices, %d tokens, %d activations; %s", scenario.name, concurrency, tokens, activations, reportPath)
		})
	}
}

func registrationLoadDocument(t *testing.T, endpoint, gate, token string, index int) *giztest.Document {
	t.Helper()
	credential, err := gizcli.RegistrationTokenCredential("${token}")
	if err != nil {
		t.Fatal(err)
	}
	rpc := func(id string) giztest.Step {
		return giztest.Step{ID: id, Client: "peer", RPC: &giztest.RPCOperation{Method: "server.register", Request: map[string]any{"token": "${token}"}}, Expect: map[string]giztest.Expectation{"/runtime_profile_name": {Equals: "registration-load"}}}
	}
	ready := func(id, path string) giztest.Step {
		return giztest.Step{ID: id, Client: "peer", HTTP: &giztest.HTTPOperation{Endpoint: gate, Method: "GET", Path: path, Status: 200}}
	}
	return &giztest.Document{
		Version: "gizclaw.test/v1alpha1", Name: fmt.Sprintf("registration-load.%03d", index), Repeat: 1, Timeout: "90s",
		Clients:   map[string]giztest.ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: endpoint, AdmissionCredential: credential}},
		Variables: map[string]giztest.VariableSpec{"token": {Direction: "input", Type: "string", Value: token, Secret: true}},
		Steps:     []giztest.Step{ready("ready", "/register"), rpc("register"), ready("retry_ready", "/retry"), rpc("retry_0"), rpc("retry_1"), rpc("retry_2"), rpc("retry_3")},
	}
}

func writeRegistrationEvidence(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(path, ".giztest.yaml") {
		data = append([]byte("# User Story:\n# As a device sharing a product RegistrationToken,\n# I want concurrent registration and retries through real WebRTC,\n# So that activation limits, owner bindings and database contention are observable.\n"), data...)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func monitorRegistrationPG(t *testing.T, ctx context.Context, db *sqlx.DB, application, path string) func() {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Go(func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		defer file.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var sample string
				err := db.GetContext(ctx, &sample, `SELECT json_build_object('time',clock_timestamp(),'activity',COALESCE(json_agg(a),'[]'::json))::text
				FROM (SELECT pid,state,wait_event_type,wait_event,pg_blocking_pids(pid) AS blockers,
					EXTRACT(EPOCH FROM clock_timestamp()-query_start)*1000 AS query_ms,
					EXTRACT(EPOCH FROM clock_timestamp()-xact_start)*1000 AS transaction_ms,query
					FROM pg_stat_activity WHERE application_name=$1 AND pid<>pg_backend_pid() AND state<>'idle') a`, application)
				if err != nil {
					if ctx.Err() == nil {
						t.Errorf("PG monitor: %v", err)
					}
					return
				}
				var compact bytes.Buffer
				if err := json.Compact(&compact, []byte(sample)); err != nil {
					t.Errorf("PG sample: %v", err)
					return
				}
				if _, err := fmt.Fprintln(file, compact.String()); err != nil {
					t.Errorf("PG evidence: %v", err)
					return
				}
			}
		}
	})
	return func() { cancel(); workers.Wait() }
}

func registrationLoadServer(t *testing.T, ctx context.Context, dsn, redisURL string) (*sqlx.DB, *CmdServer, string, *gizcli.Client, string) {
	t.Helper()
	application := fmt.Sprintf("registration_load_%d", time.Now().UnixNano())
	bootstrap, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bootstrap.Close() })
	if _, err := bootstrap.ExecContext(ctx, "CREATE SCHEMA "+application); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := bootstrap.Exec("DROP SCHEMA " + application + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("GIZCLAW_TEST_POSTGRES_DSN must be a PostgreSQL URL")
	}
	query := parsed.Query()
	query.Set("search_path", application)
	query.Set("application_name", application)
	parsed.RawQuery = query.Encode()
	dsn = parsed.String()
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := validLayeredConfig(t.TempDir())
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal("invalid GIZCLAW_TEST_REDIS_URL")
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client := redis.NewClient(redisOptions)
		defer client.Close()
		var cursor uint64
		for {
			keys, next, err := client.Scan(cleanupCtx, cursor, application+"/*", 256).Result()
			if err != nil {
				t.Errorf("scan fixture Redis keys: %v", err)
				return
			}
			if len(keys) > 0 {
				if err := client.Del(cleanupCtx, keys...).Err(); err != nil {
					t.Errorf("remove fixture Redis keys: %v", err)
					return
				}
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	})
	cfg.PeerAdmission = "registration-token"
	cfg.Storage["business-db"] = storage.PostgreSQLConfig{DSN: dsn}
	cfg.Storage["peer-runs-db"] = storage.PostgreSQLConfig{DSN: dsn}
	cfg.Storage["redis"] = storage.RedisConfig{URL: redisURL}
	for _, name := range []string{"peers", "api-keys", "friends", "friend-groups"} {
		cfg.Stores[name] = stores.Config{Kind: stores.KindKeyValue, Storage: "redis", Prefix: application + "/" + name}
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminPublicKey = key.Public
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Admission deliberately rejects simultaneous lookups of the same token.
	// Pace only signaling HTTP requests; the documents rendezvous after all
	// genuine credential checks and WebRTC handshakes, then race their RPCs.
	signaling := make(chan struct{}, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == gizwebrtc.SignalingPath {
			select {
			case signaling <- struct{}{}:
				defer func() { <-signaling }()
			case <-r.Context().Done():
				return
			}
		}
		srv.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	srv.PublicEndpoint = strings.TrimPrefix(httpServer.URL, "http://")
	srv.PeerListenerFactories = []gizclaw.PeerListenerFactory{func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
		listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: opts.SecurityPolicy, PeerEventHandler: opts.PeerEventHandler}).Listen(opts.KeyPair)
		if err == nil {
			srv.WebRTCSignalingHandler = listener.SignalingHandler()
		}
		return listener, err
	}}
	if err := srv.Listen(); err != nil {
		_ = srv.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.Close()
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Server did not stop")
		}
	})
	if _, err := srv.Manager().Peers.EnsureConnectedPeer(ctx, key.Public); err != nil {
		t.Fatal(err)
	}
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()
	return db, srv, httpServer.URL, admin, application
}
