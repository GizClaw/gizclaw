package kv

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"testing"
)

func testOrderedCollection(t *testing.T, store interface {
	Get(context.Context, Key) ([]byte, error)
	Set(context.Context, Key, []byte) error
	ListMembers(context.Context, Key) ([]string, error)
	ApplyMutation(context.Context, Mutation) (bool, error)
	RangeOrderedMembers(context.Context, Key, OrderedRange) ([]string, error)
}) {
	t.Helper()
	ctx := t.Context()
	namespace := "ordered-contract-" + rand.Text()
	index, record := Key{namespace, "index"}, Key{namespace, "record"}
	t.Cleanup(func() { _, _ = store.ApplyMutation(context.Background(), Mutation{DeleteKeys: []Key{index, record}}) })
	ok, err := store.ApplyMutation(ctx, Mutation{
		Conditions: []Condition{{Key: record}}, Entries: []Entry{{Key: record, Value: []byte("v1")}},
		AddOrderedMembers: []SetMembers{{Key: index, Members: []string{"z", "a\x00", "a", "b", "é", "", "a"}}},
	})
	if err != nil || !ok {
		t.Fatalf("publish ordered index: %v, %v", ok, err)
	}
	got, err := store.RangeOrderedMembers(ctx, index, OrderedRange{Limit: 20})
	if err != nil || !slices.Equal(got, []string{"", "a", "a\x00", "b", "z", "é"}) {
		t.Fatalf("ordered members = %q, %v", got, err)
	}
	got, err = store.RangeOrderedMembers(ctx, index, OrderedRange{After: new("a"), Before: new("z"), Limit: 1})
	if err != nil || !slices.Equal(got, []string{"a\x00"}) {
		t.Fatalf("first range page = %q, %v", got, err)
	}
	got, err = store.RangeOrderedMembers(ctx, index, OrderedRange{After: new("a\x00"), Before: new("z"), Limit: 1})
	if err != nil || !slices.Equal(got, []string{"b"}) {
		t.Fatalf("second range page = %q, %v", got, err)
	}
	if _, err := store.ListMembers(ctx, index); !errors.Is(err, ErrWrongType) {
		t.Fatalf("ordered index as ordinary Set: %v", err)
	}
	if _, err := store.Get(ctx, index); !errors.Is(err, ErrWrongType) {
		t.Fatalf("ordered index as value: %v", err)
	}
	if _, err := store.RangeOrderedMembers(ctx, index, OrderedRange{}); err == nil {
		t.Fatal("zero range limit accepted")
	}
	ok, err = store.ApplyMutation(ctx, Mutation{
		Conditions:           []Condition{{Key: record, Expected: []byte("wrong")}},
		RemoveOrderedMembers: []SetMembers{{Key: index, Members: []string{"a"}}},
	})
	if err != nil || ok {
		t.Fatalf("failed condition = %v, %v", ok, err)
	}
	if _, err := store.ApplyMutation(ctx, Mutation{
		Entries:    []Entry{{Key: record, Value: []byte("partial")}},
		AddMembers: []SetMembers{{Key: index, Members: []string{"wrong type"}}},
	}); !errors.Is(err, ErrWrongType) {
		t.Fatalf("ordinary mutation on ordered index = %v", err)
	}
	if data, err := store.Get(ctx, record); err != nil || string(data) != "v1" {
		t.Fatalf("partial mutation: %q, %v", data, err)
	}
	ok, err = store.ApplyMutation(ctx, Mutation{
		Conditions:           []Condition{{Key: record, Expected: []byte("v1")}},
		RemoveOrderedMembers: []SetMembers{{Key: index, Members: []string{"", "a", "a\x00", "b", "z", "é"}}},
	})
	if err != nil || !ok {
		t.Fatalf("remove ordered members: %v, %v", ok, err)
	}
	if _, err := store.Get(ctx, index); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty index key remains: %v", err)
	}
	if err := store.Set(ctx, index, []byte("value")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RangeOrderedMembers(ctx, index, OrderedRange{Limit: 1}); !errors.Is(err, ErrWrongType) {
		t.Fatalf("value as ordered index: %v", err)
	}
	if _, err := store.ApplyMutation(ctx, Mutation{DeleteKeys: []Key{index}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMutation(ctx, Mutation{AddMembers: []SetMembers{{Key: index, Members: []string{""}}}}); err != nil {
		t.Fatal(err)
	}
	if members, err := store.ListMembers(ctx, index); err != nil || !slices.Equal(members, []string{""}) {
		t.Fatalf("ordinary Set empty-string member = %q, %v", members, err)
	}

}
