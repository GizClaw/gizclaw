package gizlog

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestContextHandlerAddsAndOverridesAttributes(t *testing.T) {
	state := &recordingState{}
	handler := newContextHandler(
		&recordingHandler{min: slog.LevelInfo, state: state},
		[]slog.Attr{slog.String("node_id", "trusted-node")},
	)
	ctx := WithPeerPublicKey(context.Background(), "first")
	ctx = WithPeerPublicKey(ctx, " trusted ")
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
	record.AddAttrs(slog.String("peer_public_key", "caller"))
	record.AddAttrs(slog.String("node_id", "caller"))
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got := state.attrs["peer_public_key"]; got != "trusted" {
		t.Fatalf("peer_public_key = %q", got)
	}
	if got := state.attrs["node_id"]; got != "trusted-node" {
		t.Fatalf("node_id = %q", got)
	}

	siblingState := &recordingState{}
	sibling := newContextHandler(&recordingHandler{min: slog.LevelInfo, state: siblingState}, nil)
	if err := sibling.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "sibling", 0)); err != nil {
		t.Fatalf("sibling Handle() error = %v", err)
	}
	if _, exists := siblingState.attrs["peer_public_key"]; exists {
		t.Fatalf("sibling attrs = %+v", siblingState.attrs)
	}
}

func TestContextHandlerKeepsIdentityOutsideCallerGroup(t *testing.T) {
	state := &recordingState{}
	handler := newContextHandler(
		&recordingHandler{min: slog.LevelInfo, state: state},
		[]slog.Attr{slog.String("node_id", "trusted-node")},
	)
	handler = handler.WithGroup("request").(*contextHandler)
	handler = handler.WithAttrs([]slog.Attr{
		slog.String("peer_public_key", "caller"),
		slog.String("method", "GET"),
	}).(*contextHandler)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
	record.AddAttrs(slog.String("node_id", "caller"), slog.String("path", "/"))
	if err := handler.Handle(WithPeerPublicKey(context.Background(), "trusted-peer"), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if state.attrs["node_id"] != "trusted-node" || state.attrs["peer_public_key"] != "trusted-peer" {
		t.Fatalf("identity attrs = %+v", state.attrs)
	}
	if state.attrs["request.method"] != "GET" || state.attrs["request.path"] != "/" {
		t.Fatalf("request attrs = %+v", state.attrs)
	}
}

func TestWithPeerPublicKeyIgnoresEmptyIdentity(t *testing.T) {
	ctx := context.Background()
	if got := WithPeerPublicKey(ctx, "  "); got != ctx {
		t.Fatal("empty identity changed context")
	}
}
