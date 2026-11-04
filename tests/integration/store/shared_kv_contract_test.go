//go:build store_e2e

package store_e2e_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func assertSharedKV(t *testing.T, first, second kv.Store) {
	t.Helper()
	t.Run("resource-limits", func(t *testing.T) { assertSharedResourceLimits(t, first, second) })
	t.Cleanup(func() {
		_ = first.BatchDelete(context.Background(), []kv.Key{
			{"empty"}, {"ordered"}, {"ordered-other"},
			{"mutation", "set"}, {"mutation", "delete-wins"},
			{"deadline"}, {"expired"}, {"invalid-deadline"}, {"cancelled"},
			{"race", "create"}, {"race", "all-a"}, {"race", "all-b"}, {"race", "compare"},
		})
	})

	ctx := t.Context()
	if err := first.Set(ctx, kv.Key{"empty"}, nil); err != nil {
		t.Fatal(err)
	}
	if value, err := second.Get(ctx, kv.Key{"empty"}); err != nil || value == nil || len(value) != 0 {
		t.Fatalf("cross-client zero value = %#v, %v", value, err)
	}
	if err := first.Set(ctx, kv.Key{"ordered-other"}, []byte("other")); err != nil {
		t.Fatal(err)
	}
	if ok, err := first.ApplyMutation(ctx, kv.Mutation{AddOrderedMembers: []kv.SetMembers{{Key: kv.Key{"ordered"}, Members: []string{"c", "a", "b"}}}}); err != nil || !ok {
		t.Fatalf("publish ordered collection: %v, %v", ok, err)
	}
	page, err := second.RangeOrderedMembers(ctx, kv.Key{"ordered"}, kv.OrderedRange{After: new("a"), Limit: 1})
	if err != nil || !slices.Equal(page, []string{"b"}) {
		t.Fatalf("cross-client ordered page = %v, %v", page, err)
	}

	if err := first.BatchMutate(ctx,
		[]kv.Entry{{Key: kv.Key{"mutation", "set"}, Value: []byte("value")}, {Key: kv.Key{"mutation", "delete-wins"}, Value: []byte("value")}},
		[]kv.Key{{"mutation", "delete-wins"}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(ctx, kv.Key{"mutation", "delete-wins"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("delete-wins error = %v", err)
	}

	deadlineKey := kv.Key{"deadline"}
	if err := first.BatchSet(ctx, []kv.Entry{{Key: deadlineKey, Value: []byte("value"), Deadline: time.Now().Add(150 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	if err := second.Set(ctx, deadlineKey, []byte("persistent")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if value, err := first.Get(ctx, deadlineKey); err != nil || string(value) != "persistent" {
		t.Fatalf("expiration removal Get() = %q, %v", value, err)
	}
	if err := first.BatchSet(ctx, []kv.Entry{{Key: kv.Key{"expired"}, Value: []byte("gone"), Deadline: time.Now().Add(100 * time.Millisecond)}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := second.Get(ctx, kv.Key{"expired"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("expired Get() error = %v", err)
	}
	if err := first.BatchSet(ctx, []kv.Entry{{Key: kv.Key{"invalid-deadline"}, Deadline: time.Now().Add(-time.Second)}}); !errors.Is(err, kv.ErrInvalidDeadline) {
		t.Fatalf("past deadline error = %v", err)
	}

	assertKVCreateWinner(t, first, second)
	assertKVCreateAllWinner(t, first, second)
	assertKVCompareWinner(t, first, second)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := first.Set(cancelled, kv.Key{"cancelled"}, []byte("value")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Set() error = %v", err)
	}
	if _, err := first.Get(ctx, kv.Key{"cancelled"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("cancelled Set() wrote value: %v", err)
	}
}

func assertKVCreateWinner(t *testing.T, stores ...kv.Store) {
	t.Helper()
	results := runKVRace(t, len(stores), func(index int) (bool, error) {
		_, created, err := kv.CreateIfAbsent(t.Context(), stores[index], kv.Entry{Key: kv.Key{"race", "create"}, Value: fmt.Appendf(nil, "value-%d", index)}, nil)
		return created, err
	})
	assertOneKVWinner(t, results)
}

func assertKVCreateAllWinner(t *testing.T, stores ...kv.Store) {
	t.Helper()
	results := runKVRace(t, len(stores), func(index int) (bool, error) {
		_, _, created, err := kv.CreateIfAllAbsent(t.Context(), stores[index], []kv.Entry{
			{Key: kv.Key{"race", "all-a"}, Value: fmt.Appendf(nil, "a-%d", index)},
			{Key: kv.Key{"race", "all-b"}, Value: fmt.Appendf(nil, "b-%d", index)},
		}, nil)
		return created, err
	})
	assertOneKVWinner(t, results)
}

func assertKVCompareWinner(t *testing.T, stores ...kv.Store) {
	t.Helper()
	guard := kv.Key{"race", "compare"}
	if err := stores[0].Set(t.Context(), guard, []byte("before")); err != nil {
		t.Fatal(err)
	}
	results := runKVRace(t, len(stores), func(index int) (bool, error) {
		return kv.CompareAndMutate(t.Context(), stores[index], guard, []byte("before"), []kv.Entry{{Key: guard, Value: fmt.Appendf(nil, "after-%d", index)}}, nil)
	})
	assertOneKVWinner(t, results)
}

type kvRaceResult struct {
	won bool
	err error
}

func runKVRace(t *testing.T, count int, operation func(int) (bool, error)) []kvRaceResult {
	t.Helper()
	start := make(chan struct{})
	results := make([]kvRaceResult, count)
	var workers sync.WaitGroup
	for index := range count {
		workers.Go(func() {
			<-start
			results[index].won, results[index].err = operation(index)
		})
	}
	close(start)
	workers.Wait()
	return results
}

func assertOneKVWinner(t *testing.T, results []kvRaceResult) {
	t.Helper()
	winners := 0
	for _, result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("race winners = %d, want 1", winners)
	}
}

func assertSharedResourceLimits(t *testing.T, first, second kv.Store) {
	keyStores := []kv.Store{kv.Prefixed(first, kv.Key{"limits", "api-keys"}), kv.Prefixed(second, kv.Key{"limits", "api-keys"})}
	keyServers := []*apikey.Server{apikey.NewServer(keyStores[0]), apikey.NewServer(keyStores[1])}
	owner := giznet.PublicKey{1}.String()
	t.Cleanup(func() {
		_ = keyServers[0].CleanupPeer(context.Background(), owner)
		_ = keyStores[0].Delete(context.Background(), kv.Key{"retired", owner})
	})
	for range apikey.PeerAPIKeyLimit - 1 {
		if _, err := keyServers[0].Create(t.Context(), owner, "existing", false); err != nil {
			t.Fatal(err)
		}
	}
	assertOneKVWinner(t, runKVRace(t, 2, func(i int) (bool, error) {
		_, err := keyServers[i].Create(t.Context(), owner, "concurrent", false)
		if errors.Is(err, apikey.ErrPeerAPIKeyLimit) {
			return false, nil
		}
		return err == nil, err
	}))
	page, err := keyServers[1].ListOwner(t.Context(), owner, "", 100)
	if err != nil || len(page.Items) != apikey.PeerAPIKeyLimit {
		t.Fatalf("shared API key count=%d err=%v", len(page.Items), err)
	}

	peerStores := []kv.Store{kv.Prefixed(first, kv.Key{"limits", "peers"}), kv.Prefixed(second, kv.Key{"limits", "peers"})}
	peerServers := []*peer.Server{{Store: peerStores[0]}, {Store: peerStores[1]}}
	sn := "shared-sn"
	imeis := []apitypes.PeerIMEI{{Tac: "12345678", Serial: "shared"}}
	cleanup := []kv.Key{{"by-sn", sn}, {"by-imei", imeis[0].Tac, imeis[0].Serial}}
	for i := 1; i <= peer.IdentifierIndexPeerLimit+1; i++ {
		cleanup = append(cleanup, kv.Key{"by-pubkey", giznet.PublicKey{byte(i)}.String()})
	}
	t.Cleanup(func() { _ = peerStores[0].BatchDelete(context.Background(), cleanup) })
	registration := func(i int) apitypes.Peer {
		return apitypes.Peer{PublicKey: giznet.PublicKey{byte(i)}.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
			Device: apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn, Imeis: &imeis}}}
	}
	for i := 1; i < peer.IdentifierIndexPeerLimit; i++ {
		if _, err := peerServers[0].SavePeer(t.Context(), registration(i)); err != nil {
			t.Fatal(err)
		}
	}
	assertOneKVWinner(t, runKVRace(t, 2, func(i int) (bool, error) {
		_, err := peerServers[i].SavePeer(t.Context(), registration(peer.IdentifierIndexPeerLimit+i))
		if errors.Is(err, peer.ErrIdentifierIndexPeerLimit) {
			return false, nil
		}
		return err == nil, err
	}))
	for _, index := range cleanup[:2] {
		members, err := peerStores[1].ListMembers(t.Context(), index)
		if err != nil || len(members) != peer.IdentifierIndexPeerLimit {
			t.Fatalf("shared identifier count=%d err=%v", len(members), err)
		}
	}
}
