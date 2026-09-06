package peer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestAdminPeerDirectoryIsLocalToServer(t *testing.T) {
	shared := kv.NewMemory(nil)
	defer shared.Close()
	first := &Server{Store: shared, LocalRuns: peerruntest.New(t)}
	second := &Server{Store: shared, LocalRuns: peerruntest.New(t)}
	a, b := giznet.PublicKey{1}, giznet.PublicKey{2}
	saveTestPeer(t, first, a, apitypes.DeviceInfo{})
	saveTestPeer(t, second, b, apitypes.DeviceInfo{})
	check := func(server *Server, want string) {
		t.Helper()
		items, more, _, err := server.listAdminPage(t.Context(), "", 50)
		if err != nil || more || len(items) != 1 {
			t.Fatalf("local page: %+v %v %v", items, more, err)
		}
		if got := registrationResultForTest(t, items[0]); got.PublicKey != want {
			t.Fatalf("local peer=%s want=%s", got.PublicKey, want)
		}
	}
	check(first, a.String())
	check(second, b.String())
	if _, err := second.EnsureConnectedPeer(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	response, err := second.ListPeers(t.Context(), adminhttp.ListPeersRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	page, ok := response.(adminhttp.ListPeers200JSONResponse)
	if !ok || len(page.Items) != 2 {
		t.Fatalf("reconnected peer absent: %#v", response)
	}
}

type blockedRegistrationReads struct {
	kv.Store
	active, maximum, calls atomic.Int32
	entered                chan struct{}
	release                chan struct{}
}

func (store *blockedRegistrationReads) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	active := store.active.Add(1)
	defer store.active.Add(-1)
	store.calls.Add(1)
	for previous := store.maximum.Load(); active > previous; previous = store.maximum.Load() {
		if store.maximum.CompareAndSwap(previous, active) {
			break
		}
	}
	store.entered <- struct{}{}
	select {
	case <-store.release:
		return store.Store.Get(ctx, key)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestPeerListsOverlapRemoteReadsWithinBound(t *testing.T) {
	base := kv.NewMemory(nil)
	server := &Server{Store: base, LocalRuns: peerruntest.New(t)}
	var expected []string
	sn := "shared-serial"
	for i := range 32 {
		key := giznet.PublicKey{byte(i + 1)}
		saveTestPeer(t, server, key, apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn}})
		expected = append(expected, key.String())
	}
	for _, query := range []string{"directory", "serial"} {
		t.Run(query, func(t *testing.T) {
			store := &blockedRegistrationReads{Store: base, entered: make(chan struct{}, 32), release: make(chan struct{})}
			server.Store = store
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			type result struct {
				items  []adminhttp.PeerRegistrationResult
				err    error
				more   bool
				cursor *string
			}
			done := make(chan result, 1)
			go func() {
				var out result
				if query == "directory" {
					out.items, out.more, out.cursor, out.err = server.listAdminPage(ctx, "", 16)
				} else {
					out.items, out.err = server.listBySN(ctx, sn)
				}
				done <- out
			}()
			for range 8 {
				select {
				case <-store.entered:
				case <-ctx.Done():
					close(store.release)
					t.Fatal("remote reads remained serial")
				}
			}
			close(store.release)
			var out result
			select {
			case out = <-done:
			case <-ctx.Done():
				t.Fatal("list did not finish after releasing storage")
			}
			want := 32
			if query == "directory" {
				want = 16
				if !out.more || out.cursor == nil || *out.cursor != expected[15] {
					t.Fatalf("page cursor changed: %+v", out)
				}
			}
			if out.err != nil || len(out.items) != want || store.calls.Load() != int32(want) || store.maximum.Load() != 8 {
				t.Fatalf("items=%d reads=%d maximum=%d error=%v", len(out.items), store.calls.Load(), store.maximum.Load(), out.err)
			}
			for i, item := range out.items {
				if actual := registrationResultForTest(t, item).PublicKey; actual != expected[i] {
					t.Fatalf("item %d public key=%s want=%s", i, actual, expected[i])
				}
			}
		})
	}
}
