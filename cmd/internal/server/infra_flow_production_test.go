package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/gizedge"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/monitor"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/goccy/go-yaml"
)

// This acceptance test uses the published command wiring, real loopback HTTP,
// WebRTC Edge HTTP/control RPC and shared SQLite KV. No frame is inserted by
// the test; only the production worker may write infrastructure frames.
func TestInfraFlowProductionServerEdgeCollection(t *testing.T) {
	sourceAddr, publisherAddr, edgeAddr := localTCPUDPAddr(t), localTCPUDPAddr(t), localTCPUDPAddr(t)
	sourceKey, publisherKey, edgeKey, ownerKey := testKeyPair(t, 0xc1), testKeyPair(t, 0xc2), testKeyPair(t, 0xc3), testKeyPair(t, 0xc4)
	shared := filepath.Join(t.TempDir(), "shared.sqlite")
	// Seed a normal business API key through its service, not the flow storage.
	physical, err := storage.New(map[string]storage.Config{"shared": storage.SQLiteConfig{DSN: shared}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := stores.New(map[string]stores.Config{"api-keys": {Kind: stores.KindKeyValue, Storage: "shared", Prefix: "api_keys"}}, physical)
	if err != nil {
		t.Fatal(err)
	}
	keyStore, err := registry.KV("api-keys")
	if err != nil {
		t.Fatal(err)
	}
	created, err := apikey.NewServer(keyStore).Create(t.Context(), ownerKey.Public.String(), "infra acceptance", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(registry.Close(), physical.Close()); err != nil {
		t.Fatal(err)
	}
	token := "gizclaw_mk_" + strings.Repeat("t", 32)
	routes := []infraflow.Route{{Source: "edge-local", Target: "server-local"}}
	capture := &infraflow.CaptureConfig{Node: "server-local", Routes: []infraflow.CaptureRoute{
		{Route: routes[0], EdgePublicKey: edgeKey.Public.String()},
	}}
	sourceRoot, publisherRoot := t.TempDir(), t.TempDir()
	writeFlowWorkspace := func(root, addr string, key *giznet.KeyPair, flow infraflow.Config, monitorConfig monitor.Config, edges []giznet.PublicKey) {
		t.Helper()
		base := validWorkspaceConfigData(t, func(cfg *ConfigFile) {
			cfg.Identity.PrivateKey = key.Private
			cfg.WebRTC.Listen, cfg.WebRTC.Endpoint = addr, addr
			cfg.HTTP.Listeners[0].Listen = addr
			cfg.Storage["shared"] = storageFileConfig{Kind: storage.KindSQLite, DSN: shared}
			cfg.Stores["flow-windows"] = storeFileConfig{Kind: stores.KindKeyValue, Storage: "shared", Prefix: "infra_flows"}
			cfg.Stores["api-keys"] = storeFileConfig{Kind: stores.KindKeyValue, Storage: "shared", Prefix: "api_keys"}
		})
		extra, err := yaml.Marshal(struct {
			Edges   []giznet.PublicKey `yaml:"edge-nodes"`
			Monitor monitor.Config     `yaml:"monitor"`
			Flow    infraflow.Config   `yaml:"infra_flow"`
		}{edges, monitorConfig, flow})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, workspaceConfigFile), append(base, extra...), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFlowWorkspace(sourceRoot, sourceAddr, sourceKey, infraflow.Config{Store: "flow-windows", Environment: infraapi.E2e, Capture: capture}, monitor.Config{Token: token}, []giznet.PublicKey{edgeKey.Public})
	writeFlowWorkspace(publisherRoot, publisherAddr, publisherKey, infraflow.Config{Store: "flow-windows", Environment: infraapi.E2e, Producer: &infraflow.ProducerConfig{Sources: []infraflow.SourceConfig{{Endpoint: "http://" + sourceAddr + "/monitor/api/node", PublicKey: sourceKey.Public.String(), Token: token, Routes: routes}}}}, monitor.Config{}, nil)
	start := func(role, root string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestInfraFlowProcess$", "-test.v")
		cmd.Env = append(os.Environ(), "GIZCLAW_TEST_FLOW_PROCESS_ROLE="+role, "GIZCLAW_TEST_FLOW_PROCESS_ROOT="+root)
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		t.Cleanup(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("%s process shutdown: %v\n%s", role, err, output.String())
				}
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Errorf("%s process failed to join\n%s", role, output.String())
			}
		})
	}
	start("server", sourceRoot)
	_ = waitForServerInfo(t, "http://"+sourceAddr+"/server-info")
	start("server", publisherRoot)
	_ = waitForServerInfo(t, "http://"+publisherAddr+"/server-info")
	edgeRoot := t.TempDir()
	edgeConfig := gizedge.ConfigFile{
		Identity:  gizedge.IdentityConfig{PrivateKey: edgeKey.Private},
		WebRTC:    &gizedge.WebRTCConfig{Listen: edgeAddr, Endpoint: edgeAddr},
		HTTP:      &gizedge.HTTPConfig{Endpoint: "http://" + edgeAddr, Listeners: []gizedge.HTTPListenerConfig{{Listen: edgeAddr}}},
		Upstreams: &[]gizedge.UpstreamConfig{{Endpoint: "http://" + sourceAddr, PublicKey: sourceKey.Public}},
	}
	edgeData, err := yaml.Marshal(edgeConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(edgeRoot, "config.yaml"), edgeData, 0o600); err != nil {
		t.Fatal(err)
	}
	start("edge", edgeRoot)
	_ = waitForServerInfoWithClient(t, &http.Client{Timeout: 3 * time.Second}, "http://"+edgeAddr+"/server-info")
	client := &http.Client{Timeout: 3 * time.Second}
	readPage := func(environment infraapi.Environment, cursor string) infraapi.Page {
		t.Helper()
		query := "?environment=" + string(environment)
		if cursor != "" {
			query += "&cursor=" + cursor
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+edgeAddr+infraflow.Path+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var page infraapi.Page
		if response.StatusCode != http.StatusOK {
			t.Fatalf("flow HTTP status: %s", response.Status)
		}
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
			t.Fatal(err)
		}
		if page.Limits.MaxFlows != infraflow.MaxFlows || page.Limits.RetentionFrames != infraflow.RetentionFrames || page.Limits.MaxReadFrames != infraflow.MaxReadFrames || page.Limits.WindowMs != 5000 {
			t.Fatalf("feed limits: %+v", page.Limits)
		}
		if len(page.Frames) > 0 && page.Cursor != page.Frames[len(page.Frames)-1].Id {
			t.Fatal("cursor does not identify latest frame")
		}
		return page
	}
	waitFrame := infraFlowAcceptanceReader(t, func(cursor string) infraapi.Page {
		return readPage(infraapi.E2e, cursor)
	})
	_ = waitFrame(func(frame infraapi.Frame) bool { return len(frame.Flows) == 1 })
	// Observe rotation explicitly: best-effort traffic must not rotate its own
	// sampling budget, and the publisher can legitimately scrape after a boundary.
	time.Sleep(time.Until(time.Now().Truncate(infraflow.Window).Add(infraflow.Window)))
	monitorClient, err := monitorapi.NewClientWithResponses("http://"+sourceAddr, monitorapi.WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := monitorClient.GetNodeMonitorWithResponse(t.Context(), func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
	if err != nil || rotated.JSON200 == nil || rotated.JSON200.InfraFlows == nil || rotated.JSON200.InfraFlows.WindowEnd != time.Now().Truncate(infraflow.Window).UnixMilli() {
		t.Fatalf("capture rotation barrier failed: %+v %v", rotated, err)
	}
	pulseStart := time.Now()
	for range 4 {
		response, err := client.Get("http://" + edgeAddr + "/server-info")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("real Edge forwarding failed")
		}
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+edgeAddr+"/gizclaw/v1/device/status", nil)
	req.Header.Set("Authorization", "Bearer "+created.Secret)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	// The real API-key resolve RPC reads persisted business data and rejects this
	// unassigned owner. Only its peer-surface completion contributes a flow.
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("unassigned owner status: %s", response.Status)
	}
	expectedEnd := pulseStart.Truncate(infraflow.Window).Add(infraflow.Window).UnixMilli()
	if time.Now().UnixMilli() >= expectedEnd {
		t.Fatal("controlled pulse crossed window; exact-count evidence invalid")
	}
	active := waitFrame(func(frame infraapi.Frame) bool { return frame.WindowEnd == expectedEnd })
	if len(active.Flows) != 1 {
		t.Fatalf("missing owned routes: %+v", active)
	}
	var edgeFlow infraapi.Flow
	for _, flow := range active.Flows {
		switch flow.Source {
		case "edge-local":
			edgeFlow = flow
		default:
			t.Fatal("unexpected topology node")
		}
	}
	if edgeFlow.Target != "server-local" || edgeFlow.Count != 5 || edgeFlow.Bytes <= 0 || edgeFlow.DurationCount != 5 || edgeFlow.DurationSumMs <= 0 {
		t.Fatalf("HTTP/RPC completion evidence: %+v", edgeFlow)
	}
	// A best-effort source may miss a window; the next available idle frame must
	// have no replayed activity, regardless of a gap or a batched cursor response.
	idle := waitFrame(func(frame infraapi.Frame) bool { return frame.WindowStart >= active.WindowEnd })
	for _, flow := range idle.Flows {
		if flow.Count != 0 || flow.Bytes != 0 || flow.DurationCount != 0 || flow.DurationSumMs != 0 {
			t.Fatalf("idle replay: %+v", flow)
		}
	}
	if page := readPage(infraapi.Prod, ""); len(page.Frames) != 0 || page.Cursor != "" {
		t.Fatal("production activity leaked environments")
	}
	t.Logf("real peer surface: window=%d..%d cursor=%s edge_count=%d edge_bytes=%d simulated=false idle_window=%d", active.WindowStart, active.WindowEnd, active.Id, edgeFlow.Count, edgeFlow.Bytes, idle.WindowEnd)
}

// TestInfraFlowProcess runs a normal command lifecycle in an isolated process;
// the parent acceptance test owns cancellation and joins before deleting Stores.
func TestInfraFlowProcess(t *testing.T) {
	role := os.Getenv("GIZCLAW_TEST_FLOW_PROCESS_ROLE")
	if role == "" {
		return
	}
	root := os.Getenv("GIZCLAW_TEST_FLOW_PROCESS_ROOT")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	if role == "server" {
		err = ServeContext(ctx, root, ServeOptions{Force: true})
	} else {
		err = gizedge.ServeContext(ctx, root)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// infraFlowAcceptanceReader consumes bounded response batches across matches.
// Missing publication windows can legitimately reset a cursor to the latest frame.
func infraFlowAcceptanceReader(t *testing.T, readPage func(string) infraapi.Page) func(func(infraapi.Frame) bool) infraapi.Frame {
	t.Helper()
	cursor := ""
	var pending []infraapi.Frame
	var lastEnd int64
	return func(match func(infraapi.Frame) bool) infraapi.Frame {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if len(pending) == 0 {
				page := readPage(cursor)
				cursor = page.Cursor
				if page.Reset && len(page.Frames) > 1 {
					t.Fatal("reset returned historical frames")
				}
				pending = page.Frames
			}
			for len(pending) > 0 {
				frame := pending[0]
				pending = pending[1:]
				if frame.Simulated || frame.Environment != infraapi.E2e || frame.WindowEnd-frame.WindowStart != 5000 || frame.WindowEnd%5000 != 0 || len(frame.Flows) > 64 {
					t.Fatalf("invalid production frame: %+v", frame)
				}
				if frame.WindowEnd <= lastEnd {
					continue
				}
				lastEnd = frame.WindowEnd
				if match(frame) {
					return frame
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("no matching fresh production frame")
		return infraapi.Frame{}
	}
}

func TestInfraFlowAcceptanceReaderAllowsGapResetAndBatchedFrames(t *testing.T) {
	frame := func(end int64) infraapi.Frame {
		return infraapi.Frame{Id: fmt.Sprint(end), Environment: infraapi.E2e, WindowStart: end - 5000, WindowEnd: end, Flows: []infraapi.Flow{}}
	}
	calls := 0
	read := infraFlowAcceptanceReader(t, func(cursor string) infraapi.Page {
		calls++
		switch calls {
		case 1:
			if cursor != "" {
				t.Fatal("unexpected initial cursor")
			}
			return infraapi.Page{Cursor: "5000", Frames: []infraapi.Frame{frame(5000)}}
		case 2:
			if cursor != "5000" {
				t.Fatal("missing consumed cursor")
			}
			// Window 10000 was not published: the real Feed contract resets to latest.
			return infraapi.Page{Cursor: "15000", Reset: true, Frames: []infraapi.Frame{frame(15000)}}
		case 3:
			if cursor != "15000" {
				t.Fatal("reset cursor was not retained")
			}
			return infraapi.Page{Cursor: "25000", Frames: []infraapi.Frame{frame(20000), frame(25000)}}
		default:
			t.Fatal("reader discarded a pending frame and polled again")
			return infraapi.Page{}
		}
	})
	for _, end := range []int64{5000, 15000, 20000, 25000} {
		if got := read(func(f infraapi.Frame) bool { return f.WindowEnd == end }); got.WindowEnd != end {
			t.Fatalf("frame=%d want=%d", got.WindowEnd, end)
		}
	}
	if calls != 3 {
		t.Fatalf("polls=%d want=3", calls)
	}
}
