package runtimeprofile

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestSQLBindingRollsBackFailedRegistration(t *testing.T) {
	db := profileSQLTestDB(t)
	s := &Server{DB: db}
	ctx := t.Context()
	for _, id := range []string{"old", "new"} {
		if _, err := insertRuntimeProfileSQL(ctx, db, apitypes.RuntimeProfile{Id: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BindOwnerProfile(ctx, "owner", "old"); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("registration commit failed")
	if err := s.BindOwnerProfileAndCommit(ctx, "owner", "new", func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("bind = %v", err)
	}
	got, err := s.ResolveOwnerProfile(ctx, "owner")
	if err != nil || got.Id != "old" {
		t.Fatalf("restored = %#v, %v", got, err)
	}
	if err := s.BindOwnerProfileAndCommit(ctx, "other", "new", func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := s.ResolveOwnerProfile(ctx, "other"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("new owner after rollback = %v", err)
	}
}

func TestSQLBindingCancellationRetainsCommittedOwner(t *testing.T) {
	db := profileSQLTestDB(t)
	s := &Server{DB: db}
	for _, id := range []string{"old", "new"} {
		if _, err := insertRuntimeProfileSQL(t.Context(), db, apitypes.RuntimeProfile{Id: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BindOwnerProfile(t.Context(), "owner", "old"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	failure := errors.New("registration canceled")
	if err := s.BindOwnerProfileAndCommit(ctx, "owner", "new", func() error { cancel(); return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	got, err := s.ResolveOwnerProfile(t.Context(), "owner")
	if err != nil || got.Id != "old" {
		t.Fatalf("owner after cancellation = %#v, %v", got, err)
	}
}
