package infraflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type measuredKV struct {
	kv.Store
	gets   atomic.Int64
	mu     sync.Mutex
	keys   map[string]int
	writes int
}

type failingKV struct {
	kv.Store
	reads atomic.Int64
}

func (s *failingKV) Get(context.Context, kv.Key) ([]byte, error) {
	s.reads.Add(1)
	return nil, errors.New("temporary KV failure")
}

func TestFeedFailureIsCachedAndCancellationIsHonored(t *testing.T) {
	store := &failingKV{Store: kv.NewMemory(nil)}
	feed, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := feed.Read(t.Context(), infraapi.Prod, ""); err == nil {
			t.Fatal("masked KV failure")
		}
	}
	if store.reads.Load() != 1 {
		t.Fatalf("failure triggered %d backend reads", store.reads.Load())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := feed.Read(ctx, infraapi.Prod, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cache read: %v", err)
	}
}

func (s *measuredKV) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.gets.Add(1)
	return s.Store.Get(ctx, key)
}
func (s *measuredKV) ApplyMutation(ctx context.Context, m kv.Mutation) (bool, error) {
	ok, err := s.Store.ApplyMutation(ctx, m)
	if ok && err == nil {
		s.mu.Lock()
		for _, entry := range m.Entries {
			s.keys[strings.Join(entry.Key, "/")] = len(entry.Value)
			s.writes++
		}
		s.mu.Unlock()
	}
	return ok, err
}

func fixtureFrame(end time.Time, scale int64) infraapi.Frame {
	flows := make([]infraapi.Flow, MaxFlows)
	for i := range flows {
		flows[i] = infraapi.Flow{Source: fmt.Sprintf("edge-%02d", i), Target: "server", Kind: infraapi.Request, Count: scale, Bytes: scale * 100, DurationSumMs: float64(scale) * 20, DurationCount: scale}
	}
	return infraapi.Frame{Id: strconvIDTest(end.UnixMilli()), Environment: infraapi.Prod, WindowStart: end.Add(-Window).UnixMilli(), WindowEnd: end.UnixMilli(), Simulated: true, Flows: flows}
}
func strconvIDTest(n int64) string { return fmt.Sprint(n) }

func TestFeedBoundedSlotsCursorCacheAndOwnership(t *testing.T) {
	store := &measuredKV{Store: kv.NewMemory(nil), keys: map[string]int{}}
	feed, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().Truncate(Window)
	for i := RetentionFrames + 10; i >= 0; i-- {
		frame := fixtureFrame(end.Add(-time.Duration(i)*Window), 1)
		if i%2 == 0 {
			frame = fixtureFrame(end.Add(-time.Duration(i)*Window), 1000000)
		}
		if err := feed.Append(t.Context(), frame); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.keys) != RetentionFrames+1 || store.writes != 2*(RetentionFrames+11) {
		t.Fatalf("keys=%d writes=%d", len(store.keys), store.writes)
	}
	for _, size := range store.keys {
		if size > MaxFrameBytes {
			t.Fatalf("unbounded record: %d", size)
		}
	}
	latest, err := feed.Read(t.Context(), infraapi.Prod, "")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Reset || len(latest.Frames) != 1 || latest.Cursor != strconvIDTest(end.UnixMilli()) {
		t.Fatalf("cold page: %+v", latest)
	}
	latest.Frames[0].Flows[0].Count = -99
	reads := store.gets.Load()
	for range 20 {
		page, err := feed.Read(t.Context(), infraapi.Prod, latest.Cursor)
		if err != nil || len(page.Frames) != 0 {
			t.Fatalf("unchanged page=%+v err=%v", page, err)
		}
	}
	if store.gets.Load() != reads {
		t.Fatal("repeated dashboard reads bypassed fixed cache")
	}
	page, err := feed.Read(t.Context(), infraapi.Prod, strconvIDTest(end.Add(-3*Window).UnixMilli()))
	if err != nil || page.Reset || len(page.Frames) != 3 {
		t.Fatalf("catchup=%+v err=%v", page, err)
	}
	for _, cursor := range []string{strconvIDTest(end.Add(-200 * Window).UnixMilli()), strconvIDTest(end.Add(Window).UnixMilli())} {
		page, err := feed.Read(t.Context(), infraapi.Prod, cursor)
		if err != nil || !page.Reset || len(page.Frames) != 1 || page.Frames[0].Flows[0].Count < 0 {
			t.Fatalf("reset=%+v err=%v", page, err)
		}
	}
	if err := feed.Append(t.Context(), fixtureFrame(end, 1000000)); err != nil {
		t.Fatal("identical retry:", err)
	}
	if err := feed.Append(t.Context(), fixtureFrame(end, 2)); !errors.Is(err, ErrWindowConflict) {
		t.Fatal("conflicting publisher:", err)
	}
	if err := feed.Append(t.Context(), fixtureFrame(end.Add(-Window), 1)); !errors.Is(err, ErrStaleFrame) {
		t.Fatal("stale frame:", err)
	}
	other, err := feed.Read(t.Context(), infraapi.E2e, latest.Cursor)
	if err != nil || !other.Reset || len(other.Frames) != 0 {
		t.Fatalf("cross-environment data leaked: %+v %v", other, err)
	}
}

func TestAggregatorTrafficVolumeDoesNotAllocateMoreRoutes(t *testing.T) {
	end := time.Now().Truncate(Window)
	routes := fixtureFrame(end, 0).Flows
	for _, scale := range []int64{1, 1000000} {
		aggregator, err := NewAggregator(infraapi.Prod, routes, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range routes {
			if err := aggregator.Add(i, scale, 100*scale, float64(scale)*20, scale); err != nil {
				t.Fatal(err)
			}
		}
		frame := aggregator.Drain(end)
		if len(frame.Flows) != MaxFlows || frame.Flows[0].Count != scale {
			t.Fatalf("aggregate capacity changed: %+v", frame)
		}
		data, _ := json.Marshal(frame)
		if len(data) > MaxFrameBytes {
			t.Fatalf("frame exceeded byte cap: %d", len(data))
		}
		if err := validateFrame(frame); err != nil {
			t.Fatal(err)
		}
		if err := aggregator.Add(MaxFlows, 1, 0, 0, 0); err == nil {
			t.Fatal("accepted unconfigured route")
		}
		if err := aggregator.Add(0, MaxCounter+1, 0, 0, 0); err == nil {
			t.Fatal("accepted unsafe counter")
		}
		if frame := aggregator.Drain(end.Add(Window)); frame.Flows[0].Count != 0 {
			t.Fatal("retained raw observations after drain")
		}
	}
	if _, err := NewAggregator(infraapi.Prod, append(routes, routes[0]), false); err == nil {
		t.Fatal("accepted route count above hard cap")
	}
}

func TestFeedConcurrentWritersCannotOverwriteOneWindow(t *testing.T) {
	store := kv.NewMemory(nil)
	a, _ := New(store)
	b, _ := New(store)
	end := time.Now().Truncate(Window)
	var group sync.WaitGroup
	results := make(chan error, 2)
	group.Go(func() { results <- a.Append(t.Context(), fixtureFrame(end, 1)) })
	group.Go(func() { results <- b.Append(t.Context(), fixtureFrame(end, 2)) })
	group.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrWindowConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestGeneratedReadAPIAuthenticatesBeforeKVAndRejectsUnboundedQueries(t *testing.T) {
	store := &measuredKV{Store: kv.NewMemory(nil), keys: map[string]int{}}
	feed, _ := New(store)
	end := time.Now().Truncate(Window)
	if err := feed.Append(t.Context(), fixtureFrame(end, 1)); err != nil {
		t.Fatal(err)
	}
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	server := httptest.NewServer(Handler(Config{Store: "flows", ReadToken: token}, feed, nil))
	defer server.Close()
	reads := store.gets.Load()
	for _, test := range []struct {
		method, query, token string
		status               int
	}{
		{"GET", "?environment=prod", "", 401},
		{"POST", "?environment=prod", token, 405},
		{"GET", "?environment=prod&environment=dev", token, 400},
		{"GET", "?environment=user-123", token, 400},
		{"GET", "?environment=prod&limit=999999", token, 400},
		{"GET", "?environment=prod&cursor=-1", token, 400},
	} {
		req, _ := http.NewRequestWithContext(t.Context(), test.method, server.URL+Path+test.query, nil)
		req.Header.Set("Authorization", "Bearer "+test.token)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatalf("%s -> %d want %d", test.query, response.StatusCode, test.status)
		}
	}
	if store.gets.Load() != reads {
		t.Fatal("rejected HTTP request accessed KV")
	}
	client, err := infraapi.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ReadInfraFlowsWithResponse(t.Context(), &infraapi.ReadInfraFlowsParams{Environment: infraapi.Prod}, func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
	if err != nil || response.JSON200 == nil || len(response.JSON200.Frames) != 1 {
		t.Fatalf("generated client: %+v %v", response, err)
	}
	if response.HTTPResponse.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("cache policy changed")
	}
}
