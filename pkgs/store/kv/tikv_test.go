package kv

import (
	"bytes"
	"context"
	"errors"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	tikverr "github.com/tikv/client-go/v2/error"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestTiKVValueEncoding(t *testing.T) {
	for _, payload := range [][]byte{nil, {}, []byte("opaque\x00bytes")} {
		v, err := decodeTiKVValue((tiKVValue{payload: payload}).encode(), time.Now())
		if err != nil || v.payload == nil || !bytes.Equal(payload, v.payload) {
			t.Fatalf("payload roundtrip=%#v %v", v, err)
		}
	}
	if _, err := decodeTiKVValue((tiKVValue{deadline: time.Now().Add(-time.Second).UnixNano()}).encode(), time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expiry=%v", err)
	}
	for _, raw := range [][]byte{nil, {2, 0}, (tiKVValue{kind: 3}).encode(), (tiKVValue{kind: 1, payload: []byte("bad counter")}).encode()} {
		if _, err := decodeTiKVValue(raw, time.Now()); err == nil {
			t.Fatalf("accepted malformed header %x", raw)
		}
	}
}

func TestTiKVCollectionContracts(t *testing.T) {
	address := os.Getenv("GIZCLAW_TEST_TIKV_PD")
	if address == "" {
		t.Skip("real TiKV cluster is exercised by the Docker gate")
	}
	physical, err := storage.New(map[string]storage.Config{"cluster": storage.TiKVConfig{PDAddresses: []string{address}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	client, _, err := physical.TiKV("cluster")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewTiKVWithClient(client, TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("ordered", func(t *testing.T) { testOrderedCollection(t, store) })
	t.Run("member-limit", func(t *testing.T) { testCollectionMemberLimit(t, store) })
	t.Run("member-limit-race", func(t *testing.T) { testCollectionMemberLimitRace(t, store) })
	t.Run("expiry-reclamation", func(t *testing.T) {
		prefix := Key{"expiry", time.Now().Format("150405.000000000")}
		first, second, persistent := append(cloneKey(prefix), "first"), append(cloneKey(prefix), "second"), append(cloneKey(prefix), "persistent")
		t.Cleanup(func() { _ = store.BatchDelete(context.Background(), []Key{first, second, persistent}) })
		deadline := time.Now().Add(150 * time.Millisecond)
		if err := store.BatchSet(t.Context(), []Entry{{Key: first, Value: []byte("gone"), Deadline: deadline}, {Key: second, Value: []byte("gone"), Deadline: deadline}, {Key: persistent, Value: []byte("old"), Deadline: deadline}}); err != nil {
			t.Fatal(err)
		}
		if err := store.Set(t.Context(), persistent, []byte("kept")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(175 * time.Millisecond)
		for range 2 {
			if count, err := store.PurgeExpired(t.Context(), 1); err != nil || count != 1 {
				t.Fatalf("bounded expiry batch=%d %v", count, err)
			}
		}
		ts, err := store.timestamp(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []Key{first, second} {
			if _, err := client.GetSnapshot(ts).Get(t.Context(), store.recordKey(key)); !tikverr.IsErrNotFound(err) {
				t.Fatalf("expired physical record remains: %v", err)
			}
		}
		if value, err := store.Get(t.Context(), persistent); err != nil || string(value) != "kept" {
			t.Fatalf("refreshed record=%q %v", value, err)
		}
	})
	t.Run("expiry-concurrent-refresh", func(t *testing.T) { testTiKVExpiryRefresh(t, store) })
	t.Run("guard-write-skew", func(t *testing.T) {
		prefix := Key{"write-skew", time.Now().Format("150405.000000000")}
		a, b := append(cloneKey(prefix), "a"), append(cloneKey(prefix), "b")
		t.Cleanup(func() { _ = store.BatchDelete(context.Background(), []Key{a, b}) })
		original := store.client.GetTiKVClient()
		barrier := &writeSkewPrewriteBarrier{Client: original, keys: [][]byte{store.recordKey(a), store.recordKey(b)}, both: make(chan struct{}), release: make(chan struct{})}
		store.client.SetTiKVClient(barrier)
		var release sync.Once
		t.Cleanup(func() { release.Do(func() { close(barrier.release) }); store.client.SetTiKVClient(original) })
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		start := make(chan struct{})
		results := make(chan bool, 2)
		errs := make(chan error, 2)
		for i := range 2 {
			go func() {
				<-start
				guard, target := a, b
				if i == 1 {
					guard, target = b, a
				}
				ok, err := store.ApplyMutation(ctx, Mutation{Conditions: []Condition{{Key: guard}}, Entries: []Entry{{Key: target, Value: []byte("created")}}})
				results <- ok
				errs <- err
			}()
		}
		close(start)
		select {
		case <-ctx.Done():
			t.Fatal("both conditional transactions did not reach prewrite")
		case <-barrier.both:
		}
		release.Do(func() { close(barrier.release) })
		winners := 0
		for range 2 {
			if <-results {
				winners++
			}
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		if winners != 1 {
			t.Fatalf("snapshot-isolation write-skew winners=%d, want 1", winners)
		}
	})
}

// Hold the reclaimer's first prewrite so a refresh commits after its snapshot
// read. A real TiKV write conflict must force the reclaimer to preserve it.
type expiryPrewriteBarrier struct {
	tikv.Client
	key     []byte
	reached chan struct{}
	release chan struct{}
	used    atomic.Bool
}

func (c *expiryPrewriteBarrier) SendRequest(ctx context.Context, addr string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	if req.Type == tikvrpc.CmdPrewrite {
		for _, mutation := range req.Prewrite().Mutations {
			if mutation.Op == kvrpcpb.Op_Del && bytes.Equal(mutation.Key, c.key) && c.used.CompareAndSwap(false, true) {
				close(c.reached)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-c.release:
				}
			}
		}
	}
	return c.Client.SendRequest(ctx, addr, req, timeout)
}
func testTiKVExpiryRefresh(t *testing.T, store *TiKV) {
	key := Key{"expiry-refresh", time.Now().Format("150405.000000000")}
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })
	if err := store.BatchSet(t.Context(), []Entry{{Key: key, Value: []byte("expired"), Deadline: time.Now().Add(100 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(125 * time.Millisecond)
	original := store.client.GetTiKVClient()
	barrier := &expiryPrewriteBarrier{Client: original, key: store.recordKey(key), reached: make(chan struct{}), release: make(chan struct{})}
	store.client.SetTiKVClient(barrier)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(barrier.release) }); store.client.SetTiKVClient(original) })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := store.PurgeExpired(ctx, 128); result <- err }()
	select {
	case <-ctx.Done():
		t.Fatal("expiry prewrite barrier was not reached")
	case <-barrier.reached:
	}
	if err := store.Set(ctx, key, []byte("refreshed")); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(barrier.release) })
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if value, err := store.Get(ctx, key); err != nil || string(value) != "refreshed" {
		t.Fatalf("reclaimer removed concurrent refresh: %q %v", value, err)
	}
}

// Both transactions must read the initial absent guards before either commits.
type writeSkewPrewriteBarrier struct {
	tikv.Client
	keys     [][]byte
	arrivals atomic.Int32
	both     chan struct{}
	release  chan struct{}
}

func (c *writeSkewPrewriteBarrier) SendRequest(ctx context.Context, addr string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	if req.Type == tikvrpc.CmdPrewrite {
		matched := false
		for _, mutation := range req.Prewrite().Mutations {
			for _, key := range c.keys {
				matched = matched || bytes.Equal(mutation.Key, key)
			}
		}
		if matched {
			count := c.arrivals.Add(1)
			if count == 2 {
				close(c.both)
			}
			if count <= 2 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-c.release:
				}
			}
		}
	}
	return c.Client.SendRequest(ctx, addr, req, timeout)
}
