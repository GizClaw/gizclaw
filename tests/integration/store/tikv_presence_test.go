//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

// testTiKVPresence exercises the production presence reader against the PD-
// verified Learner. It measures presence decoration only, without an HTTP,
// membership, profile or WebRTC workload.
func testTiKVPresence(t *testing.T, pd string, addresses map[string]string) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	proxies := make(map[string]*pausedTiKVResponseProxy)
	redirects := make(map[string]string)
	for _, address := range addresses {
		proxy := newPausedTiKVResponseProxy(t, address)
		proxies[address], redirects[address] = proxy, proxy.address()
	}
	config := storage.TiKVConfig{PDAddresses: []string{pd}}
	physical, err := storage.New(map[string]storage.Config{"writer": config, "reader": config})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	writerClient, _, err := physical.TiKV("writer")
	if err != nil {
		t.Fatal(err)
	}
	readerClient, _, err := physical.TiKV("reader")
	if err != nil {
		t.Fatal(err)
	}
	transport := &redirectedTiKVClient{Client: readerClient.GetTiKVClient(), addresses: redirects}
	observer := &observedTiKVClient{Client: transport}
	readerClient.SetTiKVClient(observer)
	options := kv.TiKVReadOptions{StaleReadDelay: 5 * time.Second, Labels: map[string]string{"region": "local"}}
	writeRoot, err := kv.NewTiKVWithClient(writerClient, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	readRoot, err := kv.NewTiKVWithClient(readerClient, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := kv.Key{"presence-" + rand.Text()}
	writer := peerpresence.New(kv.Prefixed(writeRoot, prefix), giznet.PublicKey{31})
	reader := peerpresence.New(kv.Prefixed(readRoot, prefix), giznet.PublicKey{32})
	if err := writer.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
	peers := make([]giznet.PublicKey, 10)
	seen := time.Now().UTC()
	publishers := make([]*peerpresence.Publisher, len(peers))
	for i := range peers {
		peers[i] = giznet.PublicKey{byte(i + 50)}
		publishers[i] = writer.NewPublisher(peers[i])
		publishers[i].Publish(peerpresence.Snapshot{Online: true, LastSeenAt: seen, WorkspaceName: "room"})
	}
	assertOnline := func(results []peerpresence.ReadResult) error {
		if len(results) != len(peers) {
			return fmt.Errorf("presence results = %d", len(results))
		}
		for i, result := range results {
			if result.Err != nil || !result.Snapshot.Online || result.Snapshot.WorkspaceName != "room" || !result.Snapshot.LastSeenAt.Equal(seen) {
				return fmt.Errorf("Peer %d presence = %+v", i, result)
			}
		}
		return nil
	}
	await := func(t *testing.T, check func() error) {
		t.Helper()
		deadline := time.NewTimer(40 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			err := check()
			if err == nil {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-deadline.C:
				t.Fatal(err)
			case <-ticker.C:
			}
		}
	}
	await(t, func() error {
		start := len(observer.snapshot())
		if err := assertOnline(reader.ReadBatch(ctx, peers)); err != nil {
			return err
		}
		for _, read := range observer.snapshot()[start:] {
			if read.Address != addresses["local"] || !read.Stale {
				return fmt.Errorf("waiting for Learner to serve the historical snapshot: %+v", read)
			}
		}
		return nil
	})

	t.Run("learner-batch-and-timestamp", func(t *testing.T) {
		start := len(observer.snapshot())
		if err := assertOnline(reader.ReadBatch(ctx, peers)); err != nil {
			t.Fatal(err)
		}
		reads := observer.snapshot()[start:]
		keys := 0
		for _, read := range reads {
			keys += read.Keys
			if read.Address != addresses["local"] || !read.Stale {
				t.Fatalf("presence did not use regional reads: %+v", reads)
			}
		}
		if keys != 11 || len(reads) != 2 || reads[0].Keys != 10 || reads[1].Keys != 1 {
			t.Fatalf("want 10 Peer keys on one snapshot plus one unique lease, got %+v", reads)
		}
		t.Logf("10 online Peers: %d Learner RPCs, %d keys; one Peer batch on snapshot %d", len(reads), keys, reads[0].Version)
	})

	t.Run("local-latency-and-concurrent-pages", func(t *testing.T) {
		measure := func(name string, read func() error) {
			t.Helper()
			latencies := make([]time.Duration, 50)
			for i := range latencies {
				started := time.Now()
				if err := read(); err != nil {
					t.Fatal(err)
				}
				latencies[i] = time.Since(started)
			}
			slices.Sort(latencies)
			t.Logf("%s: 50 ten-Peer pages, p50=%s p95=%s", name, latencies[25], latencies[47])
		}
		measure("serial", func() error {
			for _, peer := range peers {
				snapshot, err := reader.Read(ctx, peer)
				if err != nil || !snapshot.Online {
					return fmt.Errorf("serial presence = %+v, %v", snapshot, err)
				}
			}
			return nil
		})
		measure("batch", func() error { return assertOnline(reader.ReadBatch(ctx, peers)) })
		var pages sync.WaitGroup
		for range 16 {
			pages.Go(func() {
				for range 10 {
					if err := assertOnline(reader.ReadBatch(ctx, peers)); err != nil {
						t.Error(err)
						return
					}
				}
			})
		}
		pages.Wait()
		t.Log("16 concurrent callers completed 160 ten-Peer presence pages")
	})

	t.Run("frozen-learner-retains-old-presence", func(t *testing.T) {
		proxyURL := requiredEnvironment(t, "GIZCLAW_TEST_TIKV_PROXY")
		if err := tiKVHTTP(ctx, http.MethodPost, proxyURL+"/freeze", nil, nil); err != nil {
			t.Fatal(err)
		}
		defer func() {
			restoreCtx, restoreCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer restoreCancel()
			if err := tiKVHTTP(restoreCtx, http.MethodPost, proxyURL+"/resume", nil, nil); err != nil {
				t.Error(err)
			}
		}()
		publishers[0].Publish(peerpresence.Snapshot{LastSeenAt: seen})
		await(t, func() error {
			snapshot, err := writer.Read(kv.WithPrimaryReads(ctx), peers[0])
			if err != nil || snapshot.Online {
				return fmt.Errorf("primary presence = %+v, %v", snapshot, err)
			}
			return nil
		})
		start := len(observer.snapshot())
		if err := assertOnline(reader.ReadBatch(ctx, peers)); err != nil {
			t.Fatal(err)
		}
		for _, read := range observer.snapshot()[start:] {
			if !read.Stale || read.Address != addresses["local"] {
				t.Fatalf("frozen presence escaped Learner: %+v", read)
			}
		}
		t.Log("real Raft replication paused: center is offline, Learner still returns the old online/room snapshot")
	})
	await(t, func() error {
		result := reader.ReadBatch(ctx, peers)[0]
		if result.Err != nil || result.Snapshot.Online || result.Snapshot.WorkspaceName != "" || !result.Snapshot.LastSeenAt.Equal(seen) {
			return fmt.Errorf("replicated offline presence = %+v", result)
		}
		return nil
	})

	t.Run("real-response-stall-shares-page-deadline", func(t *testing.T) {
		transport.enabled.Store(true)
		// Warm both redirected connections before withholding real responses.
		for _, readCtx := range []context.Context{ctx, kv.WithPrimaryReads(ctx)} {
			if snapshot, err := reader.Read(readCtx, peers[1]); err != nil || !snapshot.Online {
				t.Fatalf("warm TCP proxy: %+v, %v", snapshot, err)
			}
		}
		for _, proxy := range proxies {
			proxy.paused.Store(true)
		}
		t.Cleanup(func() {
			for _, proxy := range proxies {
				close(proxy.resume)
				proxy.paused.Store(false)
			}
		})
		for _, budget := range []time.Duration{2 * time.Second, 75 * time.Millisecond} {
			callCtx := ctx
			cancel := func() {}
			if budget < 2*time.Second {
				callCtx, cancel = context.WithTimeout(ctx, budget)
			}
			started := time.Now()
			results := reader.ReadBatch(callCtx, peers)
			elapsed := time.Since(started)
			cancel()
			if elapsed < budget-10*time.Millisecond || elapsed > budget+750*time.Millisecond {
				t.Fatalf("ten-Peer page took %s with budget %s", elapsed, budget)
			}
			for _, result := range results {
				if result.Err == nil || result.Snapshot.Online || result.Snapshot.WorkspaceName != "" {
					t.Fatalf("stalled read fabricated presence: %+v", result)
				}
			}
			t.Logf("real TiKV TCP responses stalled: ten-Peer page budget=%s elapsed=%s", budget, elapsed)
		}
		held := int64(0)
		for _, proxy := range proxies {
			held += proxy.held.Load()
		}
		if held == 0 {
			t.Fatal("the TCP proxies did not intercept any real TiKV response")
		}
	})

	t.Run("bulk-read-record-semantics", func(t *testing.T) {
		store := kv.Prefixed(writeRoot, append(slices.Clone(prefix), "contract"))
		if err := store.BatchSet(ctx, []kv.Entry{{Key: kv.Key{"value"}, Value: []byte("value")}, {Key: kv.Key{"empty"}, Value: []byte{}}}); err != nil {
			t.Fatal(err)
		}
		if err := store.AddMembers(ctx, kv.Key{"set"}, "member"); err != nil {
			t.Fatal(err)
		}
		results := store.BatchGet(kv.WithPrimaryReads(ctx), []kv.Key{{"value"}, {"missing"}, {"value"}, {"empty"}, {"set"}})
		if string(results[0].Value) != "value" || results[0].Err != nil || !errors.Is(results[1].Err, kv.ErrNotFound) || string(results[2].Value) != "value" || results[3].Err != nil || len(results[3].Value) != 0 || !errors.Is(results[4].Err, kv.ErrNotFound) {
			t.Fatalf("TiKV mixed batch = %+v", results)
		}
		results[0].Value[0] = 'X'
		if string(results[2].Value) != "value" {
			t.Fatal("TiKV duplicate results alias mutable bytes")
		}
	})
}
