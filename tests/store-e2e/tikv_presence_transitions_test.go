//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
)

// Add a bounded round trip to real TiKV operations, without replacing their
// responses. Count actual prewrites, including transaction conflict retries.
type presenceRoundTripClient struct {
	tikv.Client
	prewrites atomic.Int64
}

func (c *presenceRoundTripClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	if req.Type == tikvrpc.CmdPrewrite {
		c.prewrites.Add(1)
	}
	timer := time.NewTimer(20 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	return c.Client.SendRequest(ctx, address, req, timeout)
}

func testTiKVPresenceTransitions(t *testing.T, pd string) {
	physical, err := storage.New(map[string]storage.Config{"presence": storage.TiKVConfig{PDAddresses: []string{pd}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	client, _, err := physical.TiKV("presence")
	if err != nil {
		t.Fatal(err)
	}
	observed := &presenceRoundTripClient{Client: client.GetTiKVClient()}
	defer func() { t.Logf("total real TiKV prewrites: %d", observed.prewrites.Load()) }()
	client.SetTiKVClient(observed)
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := kv.Prefixed(root, kv.Key{"presence-transitions-" + rand.Text()})
	start := func(server byte) *peerpresence.Service {
		s := peerpresence.New(store, giznet.PublicKey{server})
		if err := s.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		})
		return s
	}
	a, b := start(1), start(2)
	const count = 256
	peers := make([]giznet.PublicKey, count)
	publishers := make([]*peerpresence.Publisher, count)
	for i := range peers {
		binary.BigEndian.PutUint64(peers[i][:], uint64(i+100))
		publishers[i] = a.NewPublisher(peers[i])
	}
	await := func(service *peerpresence.Service, keys []giznet.PublicKey, want peerpresence.Snapshot) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
		defer cancel()
		for {
			missing := 0
			for _, result := range service.ReadBatch(ctx, keys) {
				if result.Err != nil || result.Snapshot != want {
					missing++
				}
			}
			if missing == 0 {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("%d/%d transitions missing: %v", missing, len(keys), ctx.Err())
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	seen := time.Now().UTC()
	for _, state := range []peerpresence.Snapshot{
		{Online: true, LastSeenAt: seen},
		{Online: true, LastSeenAt: seen, WorkspaceName: "room"},
		{LastSeenAt: seen.Add(time.Second)},
	} {
		before := observed.prewrites.Load()
		began := time.Now()
		for _, publisher := range publishers {
			publisher.Publish(state)
		}
		await(b, peers, state)
		prewrites := observed.prewrites.Load() - before
		t.Logf("%d transitions online=%v room=%q: prewrites=%d elapsed=%s (20ms/RPC)", count, state.Online, state.WorkspaceName, prewrites, time.Since(began))
		if prewrites > count*2 {
			t.Fatalf("independent Peer transitions amplified to %d prewrites for %d transitions", prewrites, count)
		}
	}

	// Reconnect, hand off to another Server, then reject the old connection's
	// delayed disconnect. A new boot must fence the old process.
	want := peerpresence.Snapshot{Online: true, LastSeenAt: seen.Add(2 * time.Second), WorkspaceName: "new-room"}
	publisher := a.NewPublisher(peers[0])
	publisher.Publish(want)
	await(b, peers[:1], want)
	replacement := b.NewPublisher(peers[0])
	want.WorkspaceName = "other-server"
	replacement.Publish(want)
	await(a, peers[:1], want)
	handoff := want
	publisher.Publish(peerpresence.Snapshot{LastSeenAt: seen.Add(3 * time.Second)})
	// A marker on the same Server proves its writers continued to progress.
	marker := giznet.PublicKey{99}
	a.NewPublisher(marker).Publish(want)
	await(b, []giznet.PublicKey{marker}, want)
	await(a, peers[:1], want)
	next := start(1)
	want.WorkspaceName = "next-boot"
	next.NewPublisher(marker).Publish(want)
	await(b, []giznet.PublicKey{marker}, want)
	if err := a.Close(); err != nil {
		t.Fatal(fmt.Errorf("close fenced generation: %w", err))
	}
	// Close joins every old writer: the delayed disconnect can no longer
	// arrive after this assertion and hide a broken ownership guard.
	await(b, peers[:1], handoff)
	await(b, []giznet.PublicKey{marker}, want)
}
