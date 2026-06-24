package agenthost

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

func TestPushSourceLifecycle(t *testing.T) {
	ctx := context.Background()
	source := NewPushSource(0)

	if err := source.Push(ctx, &genx.MessageChunk{Part: genx.Text("before")}); !errors.Is(err, ErrNoActiveInput) {
		t.Fatalf("Push before OpenAgentInput error = %v, want ErrNoActiveInput", err)
	}

	first, err := source.OpenAgentInput(ctx)
	if err != nil {
		t.Fatalf("OpenAgentInput() error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{Part: genx.Text("hello")}); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	chunk, err := first.Next()
	if err != nil {
		t.Fatalf("first.Next() error = %v", err)
	}
	if got := string(chunk.Part.(genx.Text)); got != "hello" {
		t.Fatalf("first.Next() text = %q, want hello", got)
	}

	second, err := source.OpenAgentInput(ctx)
	if err != nil {
		t.Fatalf("OpenAgentInput(second) error = %v", err)
	}
	if _, err := first.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("old input Next() error = %v, want EOF", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{Part: genx.Text("next")}); err != nil {
		t.Fatalf("Push(second) error = %v", err)
	}
	chunk, err = second.Next()
	if err != nil {
		t.Fatalf("second.Next() error = %v", err)
	}
	if got := string(chunk.Part.(genx.Text)); got != "next" {
		t.Fatalf("second.Next() text = %q, want next", got)
	}

	if err := source.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := source.Push(ctx, &genx.MessageChunk{Part: genx.Text("after")}); !errors.Is(err, ErrNoActiveInput) {
		t.Fatalf("Push after Close error = %v, want ErrNoActiveInput", err)
	}
}

func TestPushSourceNil(t *testing.T) {
	var source *PushSource
	if _, err := source.OpenAgentInput(context.Background()); !errors.Is(err, ErrMissingSource) {
		t.Fatalf("nil OpenAgentInput error = %v, want ErrMissingSource", err)
	}
	if err := source.Push(context.Background(), nil); !errors.Is(err, ErrNoActiveInput) {
		t.Fatalf("nil Push error = %v, want ErrNoActiveInput", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
}
