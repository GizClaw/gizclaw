package acl

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestViewCRUD(t *testing.T) {
	server := migratedTestServer(t)
	ctx := context.Background()
	description := "Content for children under 12."

	view, err := server.CreateView(ctx, "under-12", apitypes.ACLViewSpec{
		Description: &description,
	})
	if err != nil {
		t.Fatalf("CreateView() error = %v", err)
	}
	if view.Name != "under-12" || view.Description == nil || *view.Description != description {
		t.Fatalf("CreateView() = %+v", view)
	}
	if _, err := server.CreateView(ctx, "under-12", apitypes.ACLViewSpec{}); !errors.Is(err, ErrViewAlreadyExists) {
		t.Fatalf("CreateView(duplicate) error = %v, want %v", err, ErrViewAlreadyExists)
	}

	if _, err := server.PutView(ctx, "under-6", apitypes.ACLViewSpec{}); err != nil {
		t.Fatalf("PutView(new) error = %v", err)
	}
	views, hasNext, nextCursor, err := server.ListViews(ctx, ListViewsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListViews() error = %v", err)
	}
	if len(views) != 1 || views[0].Name != "under-12" || !hasNext || nextCursor == nil {
		t.Fatalf("ListViews() = views:%#v hasNext:%v next:%v", views, hasNext, nextCursor)
	}
	views, hasNext, nextCursor, err = server.ListViews(ctx, ListViewsRequest{Cursor: *nextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("ListViews(next) error = %v", err)
	}
	if len(views) != 1 || views[0].Name != "under-6" || hasNext || nextCursor != nil {
		t.Fatalf("ListViews(next) = views:%#v hasNext:%v next:%v", views, hasNext, nextCursor)
	}

	updatedDescription := "Updated"
	updated, err := server.PutView(ctx, "under-12", apitypes.ACLViewSpec{Description: &updatedDescription})
	if err != nil {
		t.Fatalf("PutView(existing) error = %v", err)
	}
	if updated.Description == nil || *updated.Description != updatedDescription {
		t.Fatalf("PutView(existing) = %+v", updated)
	}
	if _, err := server.DeleteView(ctx, "under-12"); err != nil {
		t.Fatalf("DeleteView() error = %v", err)
	}
	if _, err := server.GetView(ctx, "under-12"); !errors.Is(err, ErrViewNotFound) {
		t.Fatalf("GetView(deleted) error = %v, want %v", err, ErrViewNotFound)
	}
}

func TestListViewsEmptyPage(t *testing.T) {
	server := migratedTestServer(t)
	views, hasNext, nextCursor, err := server.ListViews(context.Background(), ListViewsRequest{})
	if err != nil {
		t.Fatalf("ListViews(empty) error = %v", err)
	}
	if len(views) != 0 || hasNext || nextCursor != nil {
		t.Fatalf("ListViews(empty) = views:%#v hasNext:%v next:%v", views, hasNext, nextCursor)
	}
	if _, err := server.DeleteView(context.Background(), "missing"); !errors.Is(err, ErrViewNotFound) {
		t.Fatalf("DeleteView(missing) error = %v, want %v", err, ErrViewNotFound)
	}
}
