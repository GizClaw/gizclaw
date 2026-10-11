package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/jmoiron/sqlx"
)

// Explicitly opt in with the provider/driver build overlay. Run state, its
// permission deadline/refresh/cache, Eino, Server, WebRTC and Giztest are real.
// The slow-connect control delays new TCP sessions before forwarding TLS and
// SCRAM to PostgreSQL; existing sessions and SQL are never delayed or replaced.
func TestPostgreSQLRunStateGiztest(t *testing.T) {
	if os.Getenv("GIZCLAW_TEST_RUN_STATE") != "1" {
		t.Skip("use the local run-state harness")
	}
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("run-state harness requires a local PostgreSQL URL")
	}
	concurrency := 64
	if value := os.Getenv("GIZCLAW_TEST_RUN_STATE_CONCURRENCY"); value != "" {
		concurrency, err = strconv.Atoi(value)
		if err != nil || concurrency < 2 || concurrency > 128 {
			t.Fatal("concurrency must be 2..128")
		}
	}
	mode := os.Getenv("GIZCLAW_TEST_RUN_STATE_MODE")
	if mode == "" {
		mode = "natural"
	}
	if mode != "natural" && mode != "slow-connect" {
		t.Fatal("mode must be natural or slow-connect")
	}
	root := os.Getenv("GIZCLAW_TEST_RUN_STATE_EVIDENCE")
	if root == "" {
		root = t.TempDir()
	}
	if err := os.MkdirAll(filepath.Join(root, "documents"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	logs, err := os.Create(filepath.Join(root, "permissions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous); _ = logs.Close() })
	bootstrap, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bootstrap.Close() })
	schema := fmt.Sprintf("run_state_%d", time.Now().UnixNano())
	if _, err := bootstrap.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := bootstrap.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	proxy := newRunStatePGProxy(t, parsed.Host)
	parsed.Host = proxy.listener.Addr().String()
	q := parsed.Query()
	q.Set("search_path", schema)
	q.Set("application_name", schema+"_runs")
	parsed.RawQuery = q.Encode()
	cfg := validLayeredConfig(t.TempDir())
	cfg.Storage["peer-runs-db"] = storage.PostgreSQLConfig{DSN: parsed.String()}
	q.Set("application_name", schema+"_business")
	parsed.RawQuery = q.Encode()
	cfg.Storage["business-db"] = storage.PostgreSQLConfig{DSN: parsed.String()}
	srv, endpoint, admin := runStateGiztestServer(t, ctx, cfg)
	if runStateTrace.next.Load() == 0 {
		t.Fatal("the run-state build overlay is required before provider calls")
	}
	runStateSeedVoice(t, ctx, admin)
	db := srv.Manager().PeerRun.DB
	before := db.Stats()
	readyGoroutines := runtime.NumGoroutine()
	phase := atomic.Value{}
	phase.Store("setup")
	var monitor sync.WaitGroup
	monitorCtx, stopMonitor := context.WithCancel(ctx)
	monitor.Go(func() {
		file, err := os.Create(filepath.Join(root, "pool.jsonl"))
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		encoder := json.NewEncoder(file)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-tick.C:
				if err := encoder.Encode(map[string]any{"time": time.Now().UTC(), "phase": phase.Load(), "pool": db.Stats(), "goroutines": runtime.NumGoroutine()}); err != nil {
					t.Error(err)
					return
				}
			}
		}
	})
	monitor.Go(func() {
		file, err := os.Create(filepath.Join(root, "pg-activity.jsonl"))
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-tick.C:
				var sample string
				err := bootstrap.GetContext(monitorCtx, &sample, `SELECT json_build_object('time',clock_timestamp(),'activity',COALESCE(json_agg(a),'[]'::json))::text FROM (
			 SELECT pid,application_name,state,wait_event_type,wait_event,pg_blocking_pids(pid) AS blockers,
			 EXTRACT(EPOCH FROM clock_timestamp()-query_start)*1000 AS query_ms,
			 EXTRACT(EPOCH FROM clock_timestamp()-xact_start)*1000 AS transaction_ms,query
			 FROM pg_stat_activity WHERE application_name LIKE $1 AND state<>'idle') a`, schema+"%")
				if err != nil {
					if monitorCtx.Err() == nil {
						t.Error(err)
					}
					return
				}
				var compact bytes.Buffer
				if err := json.Compact(&compact, []byte(sample)); err != nil {
					t.Error(err)
					return
				}
				if _, err := fmt.Fprintln(file, compact.String()); err != nil {
					t.Error(err)
					return
				}
			}
		}
	})
	t.Cleanup(func() { stopMonitor(); monitor.Wait() })
	barrier := giztest.NewTaskBarrier(concurrency)
	initialBarrier := giztest.NewTaskBarrier(concurrency)
	var once sync.Once
	warmDone := make(chan struct{})
	var warmStats sql.DBStats
	var warmErr error
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/initial" {
			if err := initialBarrier.Wait(r.Context()); err != nil {
				http.Error(w, err.Error(), 408)
				return
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if r.URL.Path != "/refresh" {
			http.NotFound(w, r)
			return
		}
		if err := barrier.Wait(r.Context()); err != nil {
			http.Error(w, err.Error(), 408)
			return
		}
		once.Do(func() {
			defer close(warmDone)
			// Warm exactly the capacity available to the production pool. Hold
			// each session until all are open so tiny queries cannot reuse one.
			count := concurrency
			if max := db.Stats().MaxOpenConnections; max > 0 && max < count {
				count = max
			}
			conns := make([]*sql.Conn, 0, count)
			for range count {
				conn, err := db.Conn(ctx)
				if err != nil {
					warmErr = err
					break
				}
				conns = append(conns, conn)
				if _, err := conn.ExecContext(ctx, "SELECT 1"); err != nil {
					warmErr = err
					break
				}
			}
			for _, conn := range conns {
				_ = conn.Close()
			}
			warmStats = db.Stats()
			if mode == "slow-connect" && warmErr == nil {
				proxy.delay.Store(int64(2500 * time.Millisecond))
			}
			phase.Store("refresh")
		})
		if warmErr != nil {
			http.Error(w, "warm failed", 500)
			return
		}
		// Keep actual Event streams and their 5s refresh workers alive.
		timer := time.NewTimer(12 * time.Second)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(gate.Close)
	tone, err := os.ReadFile("../../../tests/gizclaw-e2e/testdata/audio/sfu-tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_TEST_RUN_STATE_TONE", base64.StdEncoding.EncodeToString(tone))
	for index := range concurrency {
		doc := runStateVoiceDocument(t, endpoint, gate.URL, index)
		writeRunStateEvidence(t, filepath.Join(root, "documents", fmt.Sprintf("%03d.giztest.yaml", index)), doc)
	}
	command := giztestcmd.NewCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	reportPath := filepath.Join(root, "giztest.json")
	command.SetArgs([]string{"run", filepath.Join(root, "documents"), "--parallel", strconv.Itoa(concurrency), "--seed", "1", "--output", reportPath})
	phase.Store("voice")
	runErr := command.ExecuteContext(ctx)
	proxy.delay.Store(0)
	phase.Store("settling")
	postLoad := db.Stats()
	postLoadGoroutines := runtime.NumGoroutine()
	settleTime := 3 * time.Second
	if value := os.Getenv("GIZCLAW_TEST_RUN_STATE_SETTLE"); value != "" {
		settleTime, err = time.ParseDuration(value)
		if err != nil || settleTime < 0 || settleTime > 90*time.Second {
			t.Fatal("settle time must be between 0s and 90s")
		}
	}
	settle := time.NewTimer(settleTime)
	select {
	case <-settle.C:
	case <-ctx.Done():
		settle.Stop()
		t.Fatal(ctx.Err())
	}
	after := db.Stats()
	settledGoroutines := runtime.NumGoroutine()
	var goroutines bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&goroutines, 2); err != nil {
		t.Error(err)
	}
	if err := os.WriteFile(filepath.Join(root, "goroutines-settled.txt"), goroutines.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = admin.Close()
	if err := srv.Close(); err != nil {
		t.Error(err)
	}
	// sql.DB.Close can race a final last-seen write that was already opening
	// its session. Observe bounded convergence rather than one immediate sample.
	closeCtx, closeCancel := context.WithTimeout(ctx, 5*time.Second)
	closeTick := time.NewTicker(10 * time.Millisecond)
	defer closeCancel()
	defer closeTick.Stop()
waitClosed:
	for db.Stats().OpenConnections != 0 {
		select {
		case <-closeCtx.Done():
			break waitClosed
		case <-closeTick.C:
		}
	}
	closed := db.Stats()
	if closed.OpenConnections != 0 {
		t.Errorf("closed Server retained PG sessions: %+v", closed)
	}
	var remainingBackends int
waitBackends:
	for {
		if err := bootstrap.GetContext(closeCtx, &remainingBackends, "SELECT COUNT(*) FROM pg_stat_activity WHERE application_name LIKE $1", schema+"%"); err != nil {
			t.Error(err)
			break
		}
		if remainingBackends == 0 {
			break
		}
		select {
		case <-closeCtx.Done():
			break waitBackends
		case <-closeTick.C:
		}
	}
	if remainingBackends != 0 {
		t.Errorf("closed Server retained %d PG backends", remainingBackends)
	}
	stopMonitor()
	monitor.Wait()
	var warmed sql.DBStats
	appliedDelay := 0
	select {
	case <-warmDone:
		warmed = warmStats
		if mode == "slow-connect" && warmErr == nil {
			appliedDelay = 2500
		}
	default:
	}
	writeRunStateEvidence(t, filepath.Join(root, "summary.json"), map[string]any{"mode": mode, "concurrency": concurrency, "requested_startup_delay_ms": map[bool]int{true: 2500}[mode == "slow-connect"], "startup_delay_ms": appliedDelay, "before": before, "warm": warmed, "post_load": postLoad, "after": after, "closed": closed, "remaining_backends": remainingBackends, "settle_ms": settleTime.Milliseconds(), "goroutines_ready": readyGoroutines, "goroutines_post_load": postLoadGoroutines, "goroutines_settled": settledGoroutines, "goroutines": runtime.NumGoroutine(), "application": schema, "runner_failed": runErr != nil})
	runStateTrace.Lock()
	events := append([]runStateTraceEvent(nil), runStateTrace.events...)
	runStateTrace.Unlock()
	writeRunStateEvidence(t, filepath.Join(root, "driver.json"), events)
	if err := os.WriteFile(filepath.Join(root, "runner.log"), output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var statements string
	if err := bootstrap.GetContext(ctx, &statements, `SELECT COALESCE(json_agg(s),'[]'::json)::text FROM (SELECT query,calls,mean_exec_time,max_exec_time,rows FROM pg_stat_statements WHERE query LIKE '%peer_runs%' ORDER BY total_exec_time DESC) s`); err != nil {
		t.Error(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pg-statements.json"), []byte(statements), 0o600); err != nil {
		t.Fatal(err)
	}
	if after.InUse != 0 {
		t.Errorf("run-state pool still in use: %+v", after)
	}
	if runErr != nil {
		t.Fatalf("Giztest: %v; evidence %s", runErr, root)
	}
	t.Logf("%d voice peers passed refresh and script switch; %s", concurrency, root)
}

func runStateGiztestServer(t *testing.T, ctx context.Context, cfg Config) (*CmdServer, string, *gizcli.Client) {
	t.Helper()
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
		_ = srv.Close()
		httpServer.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve() }()
	t.Cleanup(func() {
		httpServer.Close()
		_ = srv.Close()
		select {
		case err := <-served:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	admin := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, srv.PublicKey(), gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath, SecurityPolicy: policy})
	}}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Dial(srv.PublicKey(), httpServer.URL); err != nil {
		t.Fatal(err)
	}
	go func() { _ = admin.Serve() }()
	return srv, httpServer.URL, admin
}

func runStateSeedVoice(t *testing.T, ctx context.Context, admin *gizcli.Client) {
	t.Helper()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{SpeechAppId: new("local"), SpeechApiKey: new("local")}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateCredential(ctx, admin, adminhttp.CredentialUpsert{Id: "local", Provider: "volc", Body: body}); err != nil {
		t.Fatal(err)
	}
	tenant, err := api.CreateVolcTenantWithResponse(ctx, adminhttp.VolcTenantUpsert{Id: "local", CredentialId: "local", ResourceIds: &[]string{"local-asr", "local-tts"}})
	if err != nil {
		t.Fatal(err)
	}
	if tenant.StatusCode() != 200 {
		t.Fatalf("tenant status %d: %s", tenant.StatusCode(), tenant.Body)
	}
	var modelData apitypes.ModelProviderData
	if err := modelData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{ApiMode: apitypes.VolcTenantModelProviderDataApiModeAsr, ResourceId: new("local-asr")}); err != nil {
		t.Fatal(err)
	}
	model, err := api.CreateModelWithResponse(ctx, adminhttp.ModelUpsert{Id: "asr", Kind: apitypes.ModelKindAsr, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: "local"}, ProviderData: modelData})
	if err != nil {
		t.Fatal(err)
	}
	if model.StatusCode() != 200 {
		t.Fatalf("model status %d: %s", model.StatusCode(), model.Body)
	}
	var voiceData apitypes.VoiceProviderData
	if err := voiceData.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{ResourceId: new("local-tts"), VoiceId: new("local")}); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateVoice(ctx, admin, adminhttp.VoiceUpsert{Id: "narrator", Source: apitypes.VoiceSourceManual, Provider: apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindVolcTenant, Id: "local"}, ProviderData: &voiceData}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../tests/gizclaw-e2e/testdata/rtp-bos/eino.json")
	if err != nil {
		t.Fatal(err)
	}
	var workflow apitypes.WorkflowSpec
	if err := json.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateWorkflow(ctx, admin, apitypes.Workflow{Id: "run-state-voice", Spec: workflow}); err != nil {
		t.Fatal(err)
	}
	i18n := map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Local voice"}, "zh-CN": {DisplayName: "本地语音"}}
	profile := adminhttp.RuntimeProfileUpsert{Id: "run-state", Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &map[string]apitypes.RuntimeProfileBinding{"asr": {ResourceId: "asr", I18n: i18n}}, Voices: &map[string]apitypes.RuntimeProfileBinding{"narrator": {ResourceId: "narrator", I18n: i18n}}}, Workflows: apitypes.RuntimeProfileWorkflows{"voice": {ResourceId: "run-state-voice", I18n: i18n, PttAsrModel: new("asr"), RealtimeAsrModel: new("asr")}}}}
	if _, err := adminapi.CreateRuntimeProfile(ctx, admin, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := adminapi.CreateRegistrationToken(ctx, admin, adminhttp.RegistrationTokenUpsert{Id: "run-state", Token: "local-run-state", RuntimeProfileId: profile.Id}); err != nil {
		t.Fatal(err)
	}
}

func runStateVoiceDocument(t *testing.T, endpoint, gate string, index int) *giztest.Document {
	t.Helper()
	rpc := func(id, method string, request map[string]any) giztest.Step {
		return giztest.Step{ID: id, Client: "peer", RPC: &giztest.RPCOperation{Method: method, Request: request}}
	}
	voice := func(id string) giztest.Step {
		return giztest.Step{ID: id, Client: "peer", Timeout: "15s", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Input: "${tone}", Pacing: "20ms", HoldBeforeAudio: "100ms", RequireText: new(true), RequireAudio: new(true)}, Expect: map[string]giztest.Expectation{"/text_eos": {Equals: true}, "/audio_eos": {Equals: true}, "/audio_bytes": {Minimum: new(float64(1))}}}
	}
	doc := &giztest.Document{Version: "gizclaw.test/v1alpha1", Name: fmt.Sprintf("run-state.voice.%03d", index), Repeat: 1, Timeout: "90s", Clients: map[string]giztest.ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: endpoint}}, Variables: map[string]giztest.VariableSpec{"workspace": {Direction: "input", Type: "string", Generate: "token"}, "workspace_b": {Direction: "input", Type: "string", Generate: "token"}, "tone": {Direction: "input", Type: "audio", Env: "GIZCLAW_TEST_RUN_STATE_TONE", MediaType: "audio/ogg", Codec: "opus", MaxBytes: 262144}}}
	doc.Steps = append(doc.Steps, rpc("register", "server.register", map[string]any{"token": "local-run-state"}))
	for _, name := range []string{"workspace", "workspace_b"} {
		doc.Steps = append(doc.Steps, rpc("create_"+name, "server.workspace.create", map[string]any{"name": "${" + name + "}", "workflow_name": "voice", "parameters": map[string]any{"eino_workspace_parameters": map[string]any{"agent_type": "EINO_WORKSPACE_PARAMETERS_AGENT_TYPE_EINO", "input": "WORKSPACE_INPUT_MODE_PUSH_TO_TALK"}}}))
	}
	doc.Steps = append(doc.Steps, rpc("select", "server.run.workspace.set", map[string]any{"workspace_name": "${workspace}"}), giztest.Step{ID: "inputs_ready", Client: "peer", Timeout: "40s", HTTP: &giztest.HTTPOperation{Endpoint: gate, Method: "GET", Path: "/initial", Status: 200}}, voice("initial_voice"), giztest.Step{ID: "refresh_window", Client: "peer", Timeout: "40s", HTTP: &giztest.HTTPOperation{Endpoint: gate, Method: "GET", Path: "/refresh", Status: 200}}, voice("refreshed_voice"), rpc("switch", "server.run.workspace.set", map[string]any{"workspace_name": "${workspace_b}"}), voice("switched_voice"))
	doc.Finally = append(doc.Finally, rpc("stop", "server.run.stop", map[string]any{}))
	for _, name := range []string{"workspace", "workspace_b"} {
		doc.Finally = append(doc.Finally, rpc("delete_"+name, "server.workspace.delete", map[string]any{"name": "${" + name + "}"}))
	}
	doc.Finally = append(doc.Finally, rpc("delete_peer", "server.peer.delete", map[string]any{}))
	return doc
}

type runStatePGProxy struct {
	listener net.Listener
	delay    atomic.Int64
	workers  sync.WaitGroup
}

func newRunStatePGProxy(t *testing.T, target string) *runStatePGProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	p := &runStatePGProxy{listener: listener}
	p.workers.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			p.workers.Go(func() {
				defer conn.Close()
				stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
				defer stop()
				var header [8]byte
				if _, err := io.ReadFull(conn, header[:]); err != nil {
					return
				}
				// CancelRequest bypasses the startup delay, preserving driver cancel.
				if binary.BigEndian.Uint32(header[4:]) != 80877102 {
					timer := time.NewTimer(time.Duration(p.delay.Load()))
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
				upstream, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
				if err != nil {
					return
				}
				defer upstream.Close()
				if _, err := upstream.Write(header[:]); err != nil {
					return
				}
				done := make(chan struct{})
				go func() { _, _ = io.Copy(upstream, conn); _ = upstream.Close(); close(done) }()
				_, _ = io.Copy(conn, upstream)
				_ = conn.Close()
				<-done
			})
		}
	})
	t.Cleanup(func() { cancel(); _ = listener.Close(); p.workers.Wait() })
	return p
}

func writeRunStateEvidence(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(path, ".giztest.yaml") {
		data = append([]byte("# User Story:\n# As a voice user sharing PostgreSQL with concurrent peers,\n# I want ordinary audio, permission refresh and script switching to complete,\n# So that connection churn cannot deny valid input.\n"), data...)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
