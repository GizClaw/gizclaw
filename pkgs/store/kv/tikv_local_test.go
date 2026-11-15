package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tikv/client-go/v2/oracle"
)

func TestTiKVLocalReadVersionAndRetirement(t *testing.T) {
	cache := new(tiKVLocalWrites)
	key := tiKVLocalKey{key: "record"}
	cache.publish(120, map[tiKVLocalKey][]byte{key: []byte("new")})
	pinned := cache.capture([][]byte{[]byte(key.key)}, nil, false)
	view := &tiKVReadView{ts: 110, writes: pinned}
	if value, err := view.resolve([]byte(key.key), []byte("old"), nil); err != nil || string(value) != "new" {
		t.Fatalf("lagging read = %q, %v", value, err)
	}
	// Equal payloads cannot prove coverage: an intervening writer may have
	// changed the value and changed it back (ABA).
	if value, err := view.resolve([]byte(key.key), []byte("new"), nil); err != nil || string(value) != "new" {
		t.Fatalf("ABA read = %q, %v", value, err)
	}
	cache.retire(119)
	if len(cache.capture([][]byte{[]byte(key.key)}, nil, false)) != 1 {
		t.Fatal("retired a commit the snapshot did not cover")
	}
	cache.retire(150)
	if len(cache.capture([][]byte{[]byte(key.key)}, nil, false)) != 0 {
		t.Fatal("covered commit was not reclaimed")
	}
	if value, err := view.resolve([]byte(key.key), []byte("old"), nil); err != nil || string(value) != "new" {
		t.Fatalf("in-flight old snapshot lost its pinned postimage: %q, %v", value, err)
	}
	view.ts = 150
	if value, err := view.resolve([]byte(key.key), []byte("external"), nil); err != nil || string(value) != "external" {
		t.Fatalf("covered read masked an external update: %q, %v", value, err)
	}
	cache.publish(120, map[tiKVLocalKey][]byte{key: []byte("late")})
	if len(cache.capture([][]byte{[]byte(key.key)}, nil, false)) != 0 {
		t.Fatal("late publication resurrected a covered commit")
	}
	cache.publish(180, map[tiKVLocalKey][]byte{key: []byte("latest")})
	cache.publish(160, map[tiKVLocalKey][]byte{key: []byte("out-of-order")})
	if got := cache.capture([][]byte{[]byte(key.key)}, nil, false)[key]; got.ts != 180 || string(got.value) != "latest" {
		t.Fatalf("completion order replaced commit order: %+v", got)
	}
}

func TestTiKVLocalDeletionAndExpiry(t *testing.T) {
	key := tiKVLocalKey{key: "record"}
	view := &tiKVReadView{ts: 10, writes: map[tiKVLocalKey]tiKVLocalWrite{key: {ts: 20}}}
	if _, err := view.resolve([]byte(key.key), []byte("deleted-old-value"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete resurrected historical value: %v", err)
	}
	expired := (tiKVValue{deadline: time.Now().Add(-time.Second).UnixNano(), payload: []byte("expired")}).encode()
	view.writes[key] = tiKVLocalWrite{ts: 30, value: expired}
	value, err := view.resolve([]byte(key.key), (tiKVValue{payload: []byte("persistent-old")}).encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeTiKVValue(value, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expiry fell back to historical persistent value: %v", err)
	}
}

type tiKVTestKeys struct {
	keys []string
}

func (it *tiKVTestKeys) Valid() bool { return len(it.keys) != 0 }
func (it *tiKVTestKeys) Key() []byte { return []byte(it.keys[0]) }
func (it *tiKVTestKeys) Next() error {
	it.keys = it.keys[1:]
	return nil
}

func TestTiKVLocalOrderedMerge(t *testing.T) {
	view := &tiKVReadView{ts: 10, prefix: "m/", writes: map[tiKVLocalKey]tiKVLocalWrite{
		{key: "m/a"}:     {ts: 20},
		{key: "m/b"}:     {ts: 20},
		{key: "m/c"}:     {ts: 20, value: []byte{1}},
		{key: "m/e"}:     {ts: 20, value: []byte{1}},
		{key: "other/a"}: {ts: 20, value: []byte{1}},
	}}
	// Removing the first two historical entries must not underfill LIMIT.
	members, err := view.mergeMembers(t.Context(), &tiKVTestKeys{keys: []string{"m/a", "m/b", "m/d", "m/f"}}, []byte("m/"), []byte("m0"), 2)
	if err != nil || !reflect.DeepEqual(members, []string{"c", "d"}) {
		t.Fatalf("bounded merged range = %q, %v", members, err)
	}
	members, err = view.mergeMembers(t.Context(), &tiKVTestKeys{keys: []string{"m/d"}}, []byte("m/d"), []byte("m/f"), 10)
	if err != nil || !reflect.DeepEqual(members, []string{"d", "e"}) {
		t.Fatalf("merged range bounds = %q, %v", members, err)
	}
	view.writes[tiKVLocalKey{key: "m/", clear: true}] = tiKVLocalWrite{ts: 20}
	members, err = view.mergeMembers(t.Context(), &tiKVTestKeys{keys: []string{"m/ancient", "m/d"}}, []byte("m/"), []byte("m0"), 10)
	if err != nil || !reflect.DeepEqual(members, []string{"c", "e"}) {
		t.Fatalf("recreated collection leaked an old incarnation: %q, %v", members, err)
	}
	if _, err := view.resolve([]byte("m/ancient"), []byte{1}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old member remained visible after recreation: %v", err)
	}
}

func TestTiKVLocalCollectionBucketsAndBoundedCapture(t *testing.T) {
	store := &TiKV{opts: &Options{Separator: '|'}}
	cache := new(tiKVLocalWrites)
	logical := Key{"tenant-a", "collection"}
	prefix := string(store.memberPrefix(logical))
	record := tiKVLocalKey{key: string(store.recordKey(logical))}
	clear := tiKVLocalKey{key: prefix, clear: true}
	writes := map[tiKVLocalKey][]byte{
		record: (tiKVValue{kind: 2, payload: binary.BigEndian.AppendUint64(nil, 7)}).encode(),
		clear:  nil,
	}
	members := []string{"", "\x00", "a", "b", "b\x00", "c", "d", "\xff"}
	for _, member := range members {
		writes[tiKVLocalKey{key: prefix + member}] = []byte{1}
	}
	writes[tiKVLocalKey{key: prefix + "c"}] = nil
	for i := range 1000 {
		other := Key{"unrelated", strconv.Itoa(i)}
		writes[tiKVLocalKey{key: string(store.recordKey(other))}] = []byte("record")
		writes[tiKVLocalKey{key: string(store.memberPrefix(other)) + "member"}] = []byte{1}
	}
	cache.publish(20, writes)
	if len(cache.memberKeys[prefix]) != len(members) || len(cache.memberKeys) != 1001 {
		t.Fatalf("collection buckets=%d target members=%d", len(cache.memberKeys), len(cache.memberKeys[prefix]))
	}
	bounds := tiKVLocalRange{start: []byte(prefix + "b\x00"), end: []byte(prefix + "\xff")}
	pinned := cache.capture([][]byte{[]byte(record.key)}, []byte(prefix), true, bounds)
	if len(pinned) != 5 || pinned[record].ts != 20 || pinned[clear].ts != 20 {
		t.Fatalf("bounded capture lost metadata/clear or pinned unrelated keys: %v", pinned)
	}
	for _, member := range []string{"b\x00", "c", "d"} {
		if _, ok := pinned[tiKVLocalKey{key: prefix + member}]; !ok {
			t.Fatalf("bounded capture omitted member %q", member)
		}
	}
	if pinned[tiKVLocalKey{key: prefix + "c"}].value != nil {
		t.Fatal("member removal was not retained")
	}
	view := &tiKVReadView{ts: 10, prefix: prefix, writes: pinned}
	got, err := view.mergeMembers(t.Context(), &tiKVTestKeys{keys: []string{prefix + "ancient"}}, bounds.start, bounds.end, 2)
	if err != nil || !slices.Equal(got, []string{"b\x00", "d"}) {
		t.Fatalf("bounded reset/removal merge=%q %v", got, err)
	}
	point := cache.capture([][]byte{[]byte(record.key), []byte(prefix + "b")}, []byte(prefix), false)
	if len(point) != 3 || point[clear].ts != 20 {
		t.Fatalf("point capture scanned members or lost the clear marker: %v", point)
	}
}

func TestTiKVLocalCollectionEncodingIsolation(t *testing.T) {
	store := &TiKV{opts: &Options{Separator: '|'}}
	cache := new(tiKVLocalWrites)
	collections := []Key{nil, {"tenant"}, {"tenant", ""}, {"tenant-a", "collection"}, {"tenant-b", "collection"}, {"tenant-a", "collection\x00\xff"}}
	writes := make(map[tiKVLocalKey][]byte)
	for _, key := range collections {
		prefix := string(store.memberPrefix(key))
		for _, member := range []string{"", "\x00\xff|member"} {
			writes[tiKVLocalKey{key: prefix + member}] = []byte{1}
		}
	}
	// Invalid physical member headers remain exact flat postimages, but cannot
	// enter a collection bucket by pretending to be a valid member prefix.
	malformed := store.memberPrefix(Key{"invalid"})
	binary.BigEndian.PutUint64(malformed[len(tiKVNamespace)+1:], ^uint64(0))
	writes[tiKVLocalKey{key: string(malformed)}] = []byte{1}
	cache.publish(20, writes)
	if len(cache.memberKeys) != len(collections) {
		t.Fatalf("encoded collection buckets=%d", len(cache.memberKeys))
	}
	for _, key := range collections {
		prefix := store.memberPrefix(key)
		pinned := cache.capture(nil, prefix, true)
		if len(pinned) != 2 {
			t.Fatalf("collection %q capture size=%d", key, len(pinned))
		}
		for _, member := range []string{"", "\x00\xff|member"} {
			if pinned[tiKVLocalKey{key: string(prefix) + member}].ts != 20 {
				t.Fatalf("collection %q omitted binary/empty member %q", key, member)
			}
		}
	}
	if pinned := cache.capture([][]byte{malformed}, nil, false); len(pinned) != 1 {
		t.Fatal("index parsing changed the authoritative flat postimage")
	}
}

func TestTiKVLocalCollectionIndexRetirement(t *testing.T) {
	store := new(TiKV)
	cache := new(tiKVLocalWrites)
	prefix := string(store.memberPrefix(Key{"collection"}))
	first, second := tiKVLocalKey{key: prefix + "first"}, tiKVLocalKey{key: prefix + "second"}
	cache.publish(100, map[tiKVLocalKey][]byte{first: {1}})
	cache.publish(200, map[tiKVLocalKey][]byte{first: nil})
	cache.publish(150, map[tiKVLocalKey][]byte{first: {1}, second: {1}})
	cache.retire(100)
	if len(cache.memberKeys[prefix]) != 2 || cache.writes[first].ts != 200 {
		t.Fatal("stale heap entry or old publication removed a current index entry")
	}
	cache.retire(150)
	if len(cache.memberKeys[prefix]) != 1 || cache.writes[first].value != nil {
		t.Fatal("retirement lost the current removal postimage")
	}
	cache.retire(200)
	if len(cache.memberKeys) != 0 || len(cache.writes) != 0 {
		t.Fatal("last-member retirement retained an empty bucket")
	}
	cache.publish(150, map[tiKVLocalKey][]byte{first: {1}})
	if len(cache.memberKeys) != 0 || len(cache.writes) != 0 {
		t.Fatal("late covered publication resurrected an indexed member")
	}
}

func TestTiKVLocalBoundedCollectionPinsBeforeTimestamp(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	clock := &tiKVSnapshotOracle{cachedTS: 100, afterRead: func() {
		close(entered)
		<-release
	}}
	store := newTiKVSnapshotTestStore(t, clock, true)
	logical := Key{"collection"}
	prefix := store.memberPrefix(logical)
	record := store.recordKey(logical)
	clear := tiKVLocalKey{key: string(prefix), clear: true}
	metadata := (tiKVValue{kind: 2, payload: binary.BigEndian.AppendUint64(nil, 2)}).encode()
	store.local.publish(101, map[tiKVLocalKey][]byte{
		{key: string(record)}:       metadata,
		clear:                       nil,
		{key: string(prefix) + "a"}: {1},
		{key: string(prefix) + "b"}: nil,
		{key: string(prefix) + "z"}: {1},
	})
	bounds := tiKVLocalRange{start: append(bytes.Clone(prefix), 'a'), end: append(bytes.Clone(prefix), 'c')}
	result := make(chan *tiKVReadView, 1)
	done := make(chan error, 1)
	go func() {
		view, err := store.readView(t.Context(), [][]byte{record}, prefix, true, bounds)
		result <- view
		done <- err
	}()
	<-entered
	retired := make(chan struct{})
	go func() {
		store.local.retire(101)
		close(retired)
	}()
	select {
	case <-retired:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("timestamp selection held the collection index mutex")
	}
	close(release)
	view := <-result
	defer view.close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(view.writes) != 4 || view.writes[clear].ts != 101 || len(store.local.memberKeys) != 0 {
		t.Fatalf("bounded capture was not pinned before retirement: %v", view.writes)
	}
	if value, err := view.resolve(record, nil, nil); err != nil || !bytes.Equal(value, metadata) {
		t.Fatalf("pinned collection metadata=%x %v", value, err)
	}
	members, err := view.mergeMembers(t.Context(), &tiKVTestKeys{keys: []string{string(prefix) + "b"}}, bounds.start, bounds.end, 2)
	if err != nil || !slices.Equal(members, []string{"a"}) {
		t.Fatalf("pinned reset/removal merge=%q %v", members, err)
	}
}

func TestTiKVLocalEmptyCollectionRangeStillSelectsSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock := &tiKVSnapshotOracle{cachedTS: 100, afterRead: cancel}
	store := newTiKVSnapshotTestStore(t, clock, true)
	_, err := store.RangeOrderedMembers(ctx, Key{"collection"}, OrderedRange{After: new("z"), Before: new("a"), Limit: 1})
	if !errors.Is(err, context.Canceled) || clock.cachedCalls != 1 {
		t.Fatalf("empty range bypassed snapshot/cancellation validation: calls=%d err=%v", clock.cachedCalls, err)
	}
}

func TestTiKVLocalReadPinsBeforeTimestamp(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	clock := &tiKVSnapshotOracle{cachedTS: oracle.GoTimeToTS(time.Now()), afterRead: func() {
		close(entered)
		<-release
	}}
	store := newTiKVSnapshotTestStore(t, clock, true)
	key := store.recordKey(Key{"peer"})
	cachedKey := tiKVLocalKey{key: string(key)}
	committedTS := clock.cachedTS + 1
	store.local.publish(committedTS, map[tiKVLocalKey][]byte{cachedKey: []byte("committed")})
	result := make(chan *tiKVReadView, 1)
	done := make(chan error, 1)
	go func() {
		view, err := store.readView(t.Context(), [][]byte{key}, nil, false)
		result <- view
		done <- err
	}()
	<-entered
	progress := make(chan struct{})
	go func() {
		store.local.retire(committedTS)
		close(progress)
	}()
	select {
	case <-progress:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("timestamp lookup held the cache state mutex")
	}
	close(release)
	view := <-result
	defer view.close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if value, err := view.resolve(key, []byte("historical"), nil); err != nil || string(value) != "committed" {
		t.Fatalf("older in-flight read lost local commit: %q, %v", value, err)
	}
}

func TestTiKVLocalAtomicPublication(t *testing.T) {
	store := new(TiKV)
	cache := new(tiKVLocalWrites)
	logical := Key{"collection"}
	record := store.recordKey(logical)
	prefix := store.memberPrefix(logical)
	member := tiKVLocalKey{key: string(prefix) + "member"}
	var writers sync.WaitGroup
	for i := range 4 {
		writers.Go(func() {
			for j := range 100 {
				ts := uint64(i*100 + j + 1)
				cache.publish(ts, map[tiKVLocalKey][]byte{{key: string(record)}: {1}, member: {1}})
			}
		})
	}
	for range 400 {
		captured := cache.capture([][]byte{record}, prefix, true)
		if len(captured) != 0 && (len(captured) != 2 || captured[tiKVLocalKey{key: string(record)}].ts != captured[member].ts) {
			t.Fatalf("partially published transaction or collection index: %v", captured)
		}
	}
	writers.Wait()
	cache.retire(400)
	if len(cache.writes) != 0 || len(cache.expiry) != 0 || len(cache.memberKeys) != 0 {
		t.Fatal("covered local writes, index buckets or stale heap entries remain")
	}
}

func TestTiKVLocalOverwritesBoundRetirementQueue(t *testing.T) {
	store := new(TiKV)
	cache := new(tiKVLocalWrites)
	logical := Key{"collection"}
	prefix := store.memberPrefix(logical)
	record := tiKVLocalKey{key: string(store.recordKey(logical))}
	reset := tiKVLocalKey{key: string(prefix), clear: true}
	member := tiKVLocalKey{key: string(prefix) + "member"}
	cache.publish(1, map[tiKVLocalKey][]byte{record: []byte("pinned"), reset: nil, member: {1}})
	pinned := cache.capture([][]byte{[]byte(record.key)}, prefix, true)
	for ts := uint64(2); ts <= 4096; ts++ {
		cache.publish(ts, map[tiKVLocalKey][]byte{record: []byte("latest"), reset: nil, member: {1}})
	}
	if len(cache.writes) != 3 || len(cache.expiry) > 2*len(cache.writes)+64 || len(cache.memberKeys[string(prefix)]) != 1 {
		t.Fatalf("overwrites grew retirement state: writes=%d queue=%d buckets=%v", len(cache.writes), len(cache.expiry), cache.memberKeys)
	}
	cache.retire(4095)
	if len(cache.writes) != 3 || cache.writes[record].ts != 4096 || cache.writes[reset].ts != 4096 || cache.writes[member].ts != 4096 || len(cache.memberKeys[string(prefix)]) != 1 {
		t.Fatal("queue compaction retired uncovered postimages or index entries")
	}
	cache.retire(4096)
	if len(cache.writes) != 0 || len(cache.expiry) != 0 || len(cache.memberKeys) != 0 {
		t.Fatal("covered postimages, index buckets or scheduling entries remain")
	}
	if string(pinned[record].value) != "pinned" || pinned[reset].ts != 1 || pinned[member].ts != 1 {
		t.Fatal("queue compaction changed an in-flight read view")
	}
}

func TestTiKVLocalStrongReadBypassesPostimages(t *testing.T) {
	clock := &tiKVSnapshotOracle{freshTS: 100, cachedTS: 50}
	store := newTiKVSnapshotTestStore(t, clock, true)
	key := store.recordKey(Key{"peer"})
	store.local.publish(90, map[tiKVLocalKey][]byte{{key: string(key)}: []byte("local")})
	view, err := store.readView(WithPrimaryReads(context.Background()), [][]byte{key}, nil, false)
	if err != nil || len(view.writes) != 0 || view.cache != nil {
		t.Fatalf("primary read used local postimages: %+v, %v", view, err)
	}
	if value, err := view.resolve(key, []byte("primary"), nil); err != nil || !slices.Equal(value, []byte("primary")) {
		t.Fatalf("primary result = %q, %v", value, err)
	}
	ordinary := newTiKVSnapshotTestStore(t, &tiKVSnapshotOracle{}, false)
	if ordinary.local != nil {
		t.Fatal("strong-only TiKV enabled the local historical-read layer")
	}
}
