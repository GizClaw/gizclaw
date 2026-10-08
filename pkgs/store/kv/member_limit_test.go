package kv

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"testing"
)

func testCollectionMemberLimit(t *testing.T, store collectionStore) {
	ctx := t.Context()
	root := "member-limit-" + rand.Text()
	index, ordered, record, other, legacy, wrong := Key{root, "index"}, Key{root, "ordered"}, Key{root, "record"}, Key{root, "other"}, Key{root, "legacy"}, Key{root, "wrong"}
	mixed, mixedOrdered := Key{root, "mixed"}, Key{root, "mixed-ordered"}
	t.Cleanup(func() {
		_, _ = store.ApplyMutation(context.Background(), Mutation{DeleteKeys: []Key{index, ordered, record, other, legacy, wrong, mixed, mixedOrdered}})
	})
	if err := store.Set(ctx, record, []byte("before")); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, wrong, []byte("string")); err != nil {
		t.Fatal(err)
	}
	seed := Mutation{
		AddMembers:        []SetMembers{{Key: index, Members: []string{"a", "b", "a"}, MaxMembers: 2}, {Key: legacy, Members: []string{"a", "b", "c"}}},
		AddOrderedMembers: []SetMembers{{Key: ordered, Members: []string{"a", "b"}, MaxMembers: 2}},
	}
	if ok, err := store.ApplyMutation(ctx, seed); err != nil || !ok {
		t.Fatalf("seed = %v, %v", ok, err)
	}
	for _, mutation := range []Mutation{
		{Entries: []Entry{{Key: record, Value: []byte("changed")}}, AddMembers: []SetMembers{{Key: other, Members: []string{"x"}, MaxMembers: 1}, {Key: index, Members: []string{"c"}, MaxMembers: 2}}},
		{DeleteKeys: []Key{record}, AddOrderedMembers: []SetMembers{{Key: ordered, Members: []string{"c"}, MaxMembers: 2}}},
		{AddMembers: []SetMembers{{Key: index, Members: []string{"c"}, MaxMembers: 3}, {Key: index, Members: []string{"d"}, MaxMembers: 3}}},
		{AddMembers: []SetMembers{{Key: legacy, Members: []string{"d"}, MaxMembers: 2}}},
		{Entries: []Entry{{Key: record, Value: []byte("changed")}}, AddMembers: []SetMembers{{Key: mixed, Members: []string{"a"}}, {Key: mixed, Members: []string{"b"}, MaxMembers: 1}}},
		{DeleteKeys: []Key{record}, AddOrderedMembers: []SetMembers{{Key: mixedOrdered, Members: []string{"a"}}, {Key: mixedOrdered, Members: []string{"b"}, MaxMembers: 1}}},
	} {
		if ok, err := store.ApplyMutation(ctx, mutation); ok || !errors.Is(err, ErrMemberLimit) {
			t.Fatalf("overflow = %v, %v", ok, err)
		}
		value, err := store.Get(ctx, record)
		if err != nil || string(value) != "before" {
			t.Fatalf("rejected mutation changed record = %q, %v", value, err)
		}
		members, err := store.ListMembers(ctx, index)
		slices.Sort(members)
		if err != nil || !slices.Equal(members, []string{"a", "b"}) {
			t.Fatalf("rejected mutation changed members = %v, %v", members, err)
		}
		members, err = store.ListMembers(ctx, other)
		if err != nil || len(members) != 0 {
			t.Fatalf("rejected mutation created other index = %v, %v", members, err)
		}
		members, err = store.ListMembers(ctx, mixed)
		if err != nil || len(members) != 0 {
			t.Fatalf("rejected mixed additions created index = %v, %v", members, err)
		}
		members, err = store.RangeOrderedMembers(ctx, mixedOrdered, OrderedRange{Limit: 2})
		if err != nil || len(members) != 0 {
			t.Fatalf("rejected mixed additions created ordered index = %v, %v", members, err)
		}
	}
	duplicate := Mutation{
		AddMembers:        []SetMembers{{Key: index, Members: []string{"a", "a"}, MaxMembers: 2}, {Key: legacy, Members: []string{"b"}, MaxMembers: 2}, {Key: mixed, Members: []string{"a"}}, {Key: mixed, Members: []string{"a"}, MaxMembers: 1}},
		AddOrderedMembers: []SetMembers{{Key: ordered, Members: []string{"a"}, MaxMembers: 2}, {Key: mixedOrdered, Members: []string{"a"}}, {Key: mixedOrdered, Members: []string{"a"}, MaxMembers: 1}},
	}
	if ok, err := store.ApplyMutation(ctx, duplicate); err != nil || !ok {
		t.Fatalf("idempotent addition = %v, %v", ok, err)
	}
	if ok, err := store.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: wrong, Members: []string{"x"}, MaxMembers: 2}}}); ok || !errors.Is(err, ErrWrongType) {
		t.Fatalf("wrong type = %v, %v", ok, err)
	}
	if ok, err := store.ApplyMutation(ctx, Mutation{RemoveMembers: []SetMembers{{Key: index, Members: []string{"a"}}}}); err != nil || !ok {
		t.Fatalf("remove = %v, %v", ok, err)
	}
	if ok, err := store.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: index, Members: []string{"c"}, MaxMembers: 2}}}); err != nil || !ok {
		t.Fatalf("released capacity = %v, %v", ok, err)
	}
}

func testCollectionMemberLimitRace(t *testing.T, store collectionStore) {
	ctx := t.Context()
	root := "member-limit-race-" + rand.Text()
	index := Key{root, "index"}
	const attempts, maximum = 24, 10
	keys := []Key{index}
	for i := range attempts {
		keys = append(keys, Key{root, fmt.Sprint(i)})
	}
	t.Cleanup(func() { _, _ = store.ApplyMutation(context.Background(), Mutation{DeleteKeys: keys}) })
	start, results := make(chan struct{}), make(chan error, attempts)
	for i := range attempts {
		go func() {
			<-start
			ok, err := store.ApplyMutation(ctx, Mutation{Entries: []Entry{{Key: keys[i+1], Value: []byte("created")}}, AddMembers: []SetMembers{{Key: index, Members: []string{fmt.Sprint(i)}, MaxMembers: maximum}}})
			if err == nil && !ok {
				err = errors.New("unexpected condition failure")
			}
			results <- err
		}()
	}
	close(start)
	winners := 0
	for range attempts {
		err := <-results
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrMemberLimit) {
			t.Errorf("concurrent addition: %v", err)
		}
	}
	if winners != maximum {
		t.Fatalf("successful additions = %d, want %d", winners, maximum)
	}
	members, err := store.ListMembers(ctx, index)
	if err != nil || len(members) != maximum {
		t.Fatalf("members = %v, %v", members, err)
	}
	for i := range attempts {
		value, err := store.Get(ctx, keys[i+1])
		if slices.Contains(members, fmt.Sprint(i)) {
			if err != nil || string(value) != "created" {
				t.Errorf("winner record = %q, %v", value, err)
			}
		} else if !errors.Is(err, ErrNotFound) {
			t.Errorf("rejected record persisted: %q, %v", value, err)
		}
	}
}
