package infraflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func captureFixture(t testing.TB, env infraapi.Environment) *Counters {
	t.Helper()
	counters, err := NewCounters(env, CaptureConfig{Node: "server", Routes: []CaptureRoute{{Route: Route{"edge", "server"}, EdgePublicKey: "edge-key"}}})
	if err != nil {
		t.Fatal(err)
	}
	return counters
}

func recordTestActivity(c *Counters, key string, bytes int64, elapsed time.Duration) {
	observer := c.EdgeObserver(key)
	if _, ok := observer.Begin(); ok {
		c.add(observer.route, bytes, elapsed)
	}
}

func TestCaptureCompletedWindowsRestartIdleBoundsAndConcurrentProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := captureFixture(t, infraapi.E2e)
		recordTestActivity(c, "edge-key", 10, time.Millisecond) // startup's partial window is omitted
		time.Sleep(time.Until(time.Now().Truncate(Window).Add(Window)))
		if c.Snapshot() != nil {
			t.Fatal("published partial startup window")
		}
		var work sync.WaitGroup
		for range 8 {
			work.Go(func() {
				for range 1000 {
					recordTestActivity(c, "edge-key", 2, 3*time.Millisecond)
					recordTestActivity(c, "unconfigured", 999, time.Hour)
				}
			})
		}
		work.Wait()
		time.Sleep(Window)
		frame := c.Snapshot()
		if frame == nil || frame.Flows[0].Count < 1 || frame.Flows[0].Count > MaxSamplesPerWindow || frame.Flows[0].Bytes != frame.Flows[0].Count*2 || frame.Flows[0].DurationCount != frame.Flows[0].Count || frame.Flows[0].DurationSumMs != float64(frame.Flows[0].Count)*3 || frame.WindowEnd-frame.WindowStart != 5000 {
			t.Fatalf("window: %+v", frame)
		}
		recorded := frame.Flows[0].Count
		frame.Flows[0].Count = -1
		if c.Snapshot().Flows[0].Count != recorded {
			t.Fatal("snapshot shared mutable counters")
		}
		time.Sleep(Window)
		if c.Snapshot().Flows[0].Count != 0 {
			t.Fatal("idle window replayed activity")
		}
		recordTestActivity(c, "edge-key", MaxCounter+1, time.Second)
		time.Sleep(Window)
		if snapshot := c.Snapshot(); snapshot == nil || snapshot.Flows[0].Count != 0 {
			t.Fatal("overflow became manufactured or invalid counters")
		}
		time.Sleep(Window)
		if c.Snapshot() == nil || c.Snapshot().Flows[0].Count != 0 {
			t.Fatal("overflow did not recover next window")
		}
		restarted := captureFixture(t, infraapi.E2e)
		if restarted.Snapshot() != nil || restarted.generation == c.generation {
			t.Fatal("restart reused counters or generation")
		}
		// A delayed activity writer must still make progress while snapshots run.
		for range 10 {
			work.Go(func() {
				for range 50 {
					c.Snapshot()
					recordTestActivity(c, "edge-key", 1, time.Millisecond)
				}
			})
		}
		work.Wait()
	})
}

func TestProducerRealRunUsesFixedSlotsIdleCursorAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &measuredKV{Store: kv.NewMemory(nil), keys: map[string]int{}}
		feed, _ := New(store)
		c := captureFixture(t, infraapi.E2e)
		producer, err := NewProducer(feed, infraapi.E2e, c, ProducerConfig{})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { producer.Run(ctx); close(done) }()
		synctest.Wait()
		time.Sleep(time.Until(time.Now().Truncate(Window).Add(Window)))
		synctest.Wait()
		recordTestActivity(c, "edge-key", 123, 2*time.Millisecond)
		time.Sleep(Window)
		synctest.Wait()
		page, err := feed.Read(t.Context(), infraapi.E2e, "")
		if err != nil || len(page.Frames) != 1 || page.Frames[0].Simulated || page.Frames[0].Flows[0].Count != 1 {
			t.Fatalf("live page: %+v %v", page, err)
		}
		cursor := page.Cursor
		time.Sleep(Window)
		synctest.Wait()
		idle, err := feed.Read(t.Context(), infraapi.E2e, cursor)
		if err != nil || len(idle.Frames) != 1 || idle.Reset || idle.Frames[0].WindowStart != page.Frames[0].WindowEnd || idle.Frames[0].Flows[0].Count != 0 {
			t.Fatalf("idle continuation: %+v %v", idle, err)
		}
		other, err := feed.Read(t.Context(), infraapi.Prod, "")
		if err != nil || len(other.Frames) != 0 {
			t.Fatalf("environment leak: %+v %v", other, err)
		}
		time.Sleep((RetentionFrames + 2) * Window)
		synctest.Wait()
		if len(store.keys) != RetentionFrames+1 {
			t.Fatalf("retained keys: %d", len(store.keys))
		}
		for _, size := range store.keys {
			if size > MaxFrameBytes {
				t.Fatal("oversized slot")
			}
		}
		cancel()
		synctest.Wait()
		<-done
		writes := store.writes
		time.Sleep(2 * Window)
		synctest.Wait()
		if store.writes != writes {
			t.Fatal("write after cancellation")
		}
		if err := store.Set(t.Context(), kv.Key{"still-borrowed"}, []byte("ok")); err != nil {
			t.Fatal("producer closed borrowed Store")
		}
	})
}

func TestCollectionConfigurationRejectsDuplicateOwnershipAndCapacity(t *testing.T) {
	c := captureFixture(t, infraapi.E2e)
	source := SourceConfig{Endpoint: "https://server.example/monitor/api/node", PublicKey: "source", Token: "gizclaw_mk_" + strings.Repeat("a", 32), Routes: []Route{{"edge", "server"}}}
	if _, err := producerSources(infraapi.E2e, c, ProducerConfig{Sources: []SourceConfig{source}}); err == nil {
		t.Fatal("duplicate local/remote owner accepted")
	}
	for _, endpoint := range []string{"http://server.example/monitor/api/node", "https://user:password@server.example/monitor/api/node", "https://server.example/monitor/api/node?token=bad"} {
		invalid := source
		invalid.Endpoint = endpoint
		if _, err := producerSources(infraapi.E2e, nil, ProducerConfig{Sources: []SourceConfig{invalid}}); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	source.Routes = nil
	for i := range MaxFlows + 1 {
		source.Routes = append(source.Routes, Route{fmt.Sprintf("edge-%d", i), "server"})
	}
	if _, err := producerSources(infraapi.E2e, nil, ProducerConfig{Sources: []SourceConfig{source}}); err == nil {
		t.Fatal("unbounded route set accepted")
	}
	for _, cfg := range []CaptureConfig{
		{Node: "server", Routes: []CaptureRoute{{Route: Route{"edge", "server"}, EdgePublicKey: "key", Store: "kv"}}},
		{Node: "server", Routes: []CaptureRoute{{Route: Route{"edge", "elsewhere"}, EdgePublicKey: "key"}}},
		{Node: "server", Routes: []CaptureRoute{{Route: Route{"elsewhere", "kv"}, Store: "kv"}}},
		{Node: "server", Routes: []CaptureRoute{{Route: Route{"edge", "server"}, EdgePublicKey: "key"}, {Route: Route{"other", "server"}, EdgePublicKey: "key"}}},
	} {
		if _, err := NewCounters(infraapi.E2e, cfg); err == nil {
			t.Fatalf("invalid capture accepted: %+v", cfg)
		}
	}
}

func TestProducerRemoteValidationAndFailureIsolation(t *testing.T) {
	c := captureFixture(t, infraapi.E2e)
	end := time.Now().Truncate(Window)
	// Unit fault injection checks wire validation. Production activity acceptance
	// is separately exercised by TestInfraFlowProductionServerEdgeCollection.
	sourceSnapshot := monitorapi.NodeSnapshot{PublicKey: "remote", Role: "server", Time: time.Now(), InfraFlows: &monitorapi.NodeInfraFlows{Environment: monitorapi.E2e, Generation: "boot", WindowStart: end.Add(-Window).UnixMilli(), WindowEnd: end.UnixMilli(), Flows: []infraapi.Flow{{Source: "remote", Target: "kv", Kind: infraapi.Request, Count: 2, Bytes: 9, DurationCount: 2, DurationSumMs: 4}}}}
	token := "gizclaw_mk_" + strings.Repeat("a", 32)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing Monitor credential")
		}
		_ = json.NewEncoder(w).Encode(sourceSnapshot)
	}))
	defer remote.Close()
	feed, _ := New(kv.NewMemory(nil))
	producer, err := NewProducer(feed, infraapi.E2e, c, ProducerConfig{Sources: []SourceConfig{{Endpoint: remote.URL + "/monitor/api/node", PublicKey: "remote", Token: token, Routes: []Route{{"remote", "kv"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := producer.collect(t.Context(), end); err == nil {
		t.Fatal("incomplete local window was silently treated as zero")
	}
	page, err := feed.Read(t.Context(), infraapi.E2e, "")
	if err != nil || len(page.Frames) != 1 || len(page.Frames[0].Flows) != 1 || page.Frames[0].Flows[0].Count != 2 {
		t.Fatalf("healthy source lost: %+v %v", page, err)
	}
	for _, mutate := range []func(){
		func() { sourceSnapshot.InfraFlows.Environment = monitorapi.Prod },
		func() { sourceSnapshot.InfraFlows.Environment = monitorapi.E2e; sourceSnapshot.PublicKey = "wrong" },
		func() { sourceSnapshot.PublicKey = "remote"; sourceSnapshot.Time = time.Now().Add(-time.Minute) },
		func() { sourceSnapshot.Time = time.Now(); sourceSnapshot.InfraFlows.WindowEnd -= 5000 },
		func() {
			sourceSnapshot.InfraFlows.WindowEnd = end.UnixMilli()
			sourceSnapshot.InfraFlows.Flows[0].Count = -1
		},
	} {
		mutate()
		if _, err := producer.snapshot(t.Context(), &producer.sources[1], end); err == nil {
			t.Fatal("untrusted snapshot accepted")
		}
	}
	producer.client.CloseIdleConnections()
}

type blockingAppendKV struct{ kv.Store }

func (s blockingAppendKV) ApplyMutation(ctx context.Context, _ kv.Mutation) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

type snapshotTransport func(*http.Request) (*http.Response, error)

func (f snapshotTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProducerSlowSourceCannotBlockHealthySourceOrShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := captureFixture(t, infraapi.E2e)
		feed, _ := New(kv.NewMemory(nil))
		producer, err := NewProducer(feed, infraapi.E2e, c, ProducerConfig{Sources: []SourceConfig{{Endpoint: "https://unavailable.example/monitor/api/node", PublicKey: "missing", Token: "gizclaw_mk_" + strings.Repeat("a", 32), Routes: []Route{{"remote", "storage"}}}}})
		if err != nil {
			t.Fatal(err)
		}
		producer.client.Transport = snapshotTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
		time.Sleep(time.Until(time.Now().Truncate(Window).Add(Window)))
		c.Snapshot()
		recordTestActivity(c, "edge-key", 7, time.Millisecond)
		time.Sleep(Window)
		end := time.Now()
		if err := producer.collect(t.Context(), end); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("source error: %v", err)
		}
		if time.Since(end) != time.Second {
			t.Fatal("source blocked collection beyond budget")
		}
		page, err := feed.Read(t.Context(), infraapi.E2e, "")
		if err != nil || len(page.Frames) != 1 || len(page.Frames[0].Flows) != 1 || page.Frames[0].Flows[0].Count != 1 {
			t.Fatalf("healthy source lost behind failure: %+v %v", page, err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { producer.Run(ctx); close(done) }()
		time.Sleep(4 * time.Second)
		synctest.Wait()
		cancel()
		synctest.Wait()
		<-done // cancel interrupts the in-flight remote read, and joins it
	})
}

func TestProducerRejectsRedirectAndOversizedSnapshot(t *testing.T) {
	c := captureFixture(t, infraapi.E2e)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/monitor/api/node" {
			http.Redirect(w, r, "/unexpected", http.StatusTemporaryRedirect)
			return
		}
		t.Error("followed authenticated redirect")
	}))
	defer remote.Close()
	feed, _ := New(kv.NewMemory(nil))
	p, err := NewProducer(feed, infraapi.E2e, c, ProducerConfig{Sources: []SourceConfig{{Endpoint: remote.URL + "/monitor/api/node", PublicKey: "remote", Token: "gizclaw_mk_" + strings.Repeat("a", 32), Routes: []Route{{"remote", "storage"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.client.CloseIdleConnections()
	end := time.Now().Truncate(Window)
	if _, err := p.snapshot(t.Context(), &p.sources[1], end); err == nil {
		t.Fatal("redirect accepted")
	}
	p.client.CloseIdleConnections()
	p.client.Transport = snapshotTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxSnapshotBytes+1)))}, nil
	})
	if _, err := p.snapshot(t.Context(), &p.sources[1], end); err == nil {
		t.Fatal("oversized snapshot accepted")
	}
}

func TestProducerWriteFailureIsBoundedAndDoesNotBacklog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := captureFixture(t, infraapi.E2e)
		feed, _ := New(blockingAppendKV{kv.NewMemory(nil)})
		p, err := NewProducer(feed, infraapi.E2e, c, ProducerConfig{})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Until(time.Now().Truncate(Window).Add(Window)))
		c.Snapshot()
		recordTestActivity(c, "edge-key", 5, time.Millisecond)
		time.Sleep(Window)
		started := time.Now()
		if err := p.collect(t.Context(), time.Now()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("append failure: %v", err)
		}
		if time.Since(started) != time.Second {
			t.Fatal("unbounded append")
		}
		healthy, _ := New(kv.NewMemory(nil))
		p.feed = healthy
		time.Sleep(4 * time.Second)
		if err := p.collect(t.Context(), time.Now()); err != nil {
			t.Fatal(err)
		}
		page, _ := healthy.Read(t.Context(), infraapi.E2e, "")
		if len(page.Frames) != 1 || page.Frames[0].Flows[0].Count != 0 {
			t.Fatalf("failed window replayed: %+v", page)
		}
	})
}
