//go:build store_e2e

package store_e2e_test

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
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

type observedPresenceMutations struct {
	kv.Store
	obsoleteAt time.Time
	calls      atomic.Int64
	rejected   sync.Map
	count      atomic.Int64
}

func (s *observedPresenceMutations) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.calls.Add(1)
	matched, err := s.Store.ApplyMutation(ctx, mutation)
	if err == nil && !matched {
		for _, entry := range mutation.Entries {
			var snapshot peerpresence.Snapshot
			if json.Unmarshal(entry.Value, &snapshot) == nil && snapshot.LastSeenAt.Equal(s.obsoleteAt) {
				if _, loaded := s.rejected.LoadOrStore(entry.Key.String(), struct{}{}); !loaded {
					s.count.Add(1)
				}
			}
		}
	}
	return matched, err
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
	store := &observedPresenceMutations{Store: kv.Prefixed(root, kv.Key{"presence-transitions-" + rand.Text()}), obsoleteAt: time.Now().UTC().Add(time.Hour)}
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

	// Hand every Peer to the other Server, then let all old connections try
	// to publish again alongside one valid connection on the original Server.
	handedOff := peerpresence.Snapshot{Online: true, LastSeenAt: seen.Add(2 * time.Second), WorkspaceName: "handoff-room"}
	for _, peer := range peers {
		b.NewPublisher(peer).Publish(handedOff)
	}
	await(a, peers, handedOff)
	before := store.calls.Load()
	for _, publisher := range publishers {
		publisher.Publish(peerpresence.Snapshot{Online: true, LastSeenAt: store.obsoleteAt, WorkspaceName: "obsolete-room"})
	}
	valid := giznet.PublicKey{98}
	a.NewPublisher(valid).Publish(handedOff)
	await(b, []giznet.PublicKey{valid}, handedOff)
	checked, checkedCancel := context.WithTimeout(t.Context(), 6*time.Second)
	for store.count.Load() < count {
		select {
		case <-checked.Done():
			checkedCancel()
			t.Fatalf("only %d/%d old publishers reached rejected mutations", store.count.Load(), count)
		case <-time.After(10 * time.Millisecond):
		}
	}
	checkedCancel()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	await(b, peers, handedOff)
	if calls := store.calls.Load() - before; calls > 32 {
		t.Fatalf("bulk handoff amplified to %d conditional mutations", calls)
	} else {
		t.Logf("256 stale publishers and one valid Peer: %d conditional mutations (20ms/TiKV RPC)", calls)
	}
	a = start(1)

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
