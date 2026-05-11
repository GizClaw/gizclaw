package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/agent/recall"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

type starvationEmbedder struct{}

func (starvationEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	switch {
	case text == "query":
		return []float32{1, 0}, nil
	case strings.HasPrefix(text, "foreign-"):
		return []float32{1, 0}, nil
	case strings.HasPrefix(text, "local-"):
		return []float32{0.8, 0.2}, nil
	default:
		return []float32{0, 1}, nil
	}
}

func (s starvationEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v, err := s.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (starvationEmbedder) Dimension() int { return 2 }

func (starvationEmbedder) Model() string { return "starvation-embed-v1" }

func TestRecallPerPersonaVecIsolation(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})

	host, err := NewHost(ctx, HostConfig{
		Store:     store,
		Embedder:  starvationEmbedder{},
		FS:        &testDirFS{root: t.TempDir()},
		Separator: testSep,
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}

	local, err := host.Open("persona_local")
	if err != nil {
		t.Fatalf("open local persona: %v", err)
	}
	foreign, err := host.Open("persona_foreign")
	if err != nil {
		t.Fatalf("open foreign persona: %v", err)
	}

	for i := range 40 {
		if err := foreign.StoreSegment(ctx, SegmentInput{Summary: fmt.Sprintf("foreign-%02d", i)}, recall.Bucket1H); err != nil {
			t.Fatalf("store foreign segment %d: %v", i, err)
		}
	}
	if err := local.StoreSegment(ctx, SegmentInput{Summary: "local-target"}, recall.Bucket1H); err != nil {
		t.Fatalf("store local segment: %v", err)
	}

	res, err := local.Recall(ctx, RecallQuery{Text: "query", Limit: 1})
	if err != nil {
		t.Fatalf("local recall: %v", err)
	}
	if len(res.Segments) == 0 {
		t.Fatal("expected local recall to return local segment with per-persona vec")
	}
	if res.Segments[0].Summary != "local-target" {
		t.Fatalf("expected top result local-target, got %q", res.Segments[0].Summary)
	}
}

func TestHostDeleteClearsPersonaData(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})

	host, err := NewHost(ctx, HostConfig{
		Store:     store,
		Embedder:  starvationEmbedder{},
		FS:        &testDirFS{root: t.TempDir()},
		Separator: testSep,
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}

	a, err := host.Open("persona_a")
	if err != nil {
		t.Fatalf("open persona_a: %v", err)
	}
	b, err := host.Open("persona_b")
	if err != nil {
		t.Fatalf("open persona_b: %v", err)
	}

	if err := a.StoreSegment(ctx, SegmentInput{Summary: "foreign-a"}, recall.Bucket1H); err != nil {
		t.Fatalf("store persona_a segment: %v", err)
	}
	if err := b.StoreSegment(ctx, SegmentInput{Summary: "local-b"}, recall.Bucket1H); err != nil {
		t.Fatalf("store persona_b segment: %v", err)
	}

	if err := host.Delete(ctx, "persona_a"); err != nil {
		t.Fatalf("delete persona_a: %v", err)
	}

	resB, err := b.Recall(ctx, RecallQuery{Text: "query", Limit: 3})
	if err != nil {
		t.Fatalf("persona_b recall: %v", err)
	}
	if len(resB.Segments) == 0 {
		t.Fatal("expected persona_b segment after deleting persona_a")
	}

	a2, err := host.Open("persona_a")
	if err != nil {
		t.Fatalf("reopen persona_a: %v", err)
	}
	resA, err := a2.Recall(ctx, RecallQuery{Text: "query", Limit: 3})
	if err != nil {
		t.Fatalf("persona_a recall after delete: %v", err)
	}
	if len(resA.Segments) != 0 {
		t.Fatalf("expected persona_a empty after delete, got %d segments", len(resA.Segments))
	}
}

func TestConversationRevertRecomputesPendingCounters(t *testing.T) {
	policy := CompressPolicy{MaxMessages: 4, MaxChars: 1 << 20}
	h := newTestHostWithCompactor(t, policy)
	defer h.Close()

	m := mustOpen(t, h, "revert_pending")
	ctx := context.Background()
	conv := mustOpenConversation(t, m, "dev-1", nil)

	if err := conv.Append(ctx, Message{Role: RoleUser, Content: "u1", Timestamp: 1000}); err != nil {
		t.Fatalf("append u1: %v", err)
	}
	if err := conv.Append(ctx, Message{Role: RoleModel, Content: "m1", Timestamp: 2000}); err != nil {
		t.Fatalf("append m1: %v", err)
	}
	if err := conv.Append(ctx, Message{Role: RoleUser, Content: "u2", Timestamp: 3000}); err != nil {
		t.Fatalf("append u2: %v", err)
	}

	if err := conv.Revert(ctx); err != nil {
		t.Fatalf("revert: %v", err)
	}

	if err := conv.Append(ctx, Message{Role: RoleModel, Content: "m2", Timestamp: 4000}); err != nil {
		t.Fatalf("append m2 after revert: %v", err)
	}

	count, err := conv.Count(ctx)
	if err != nil {
		t.Fatalf("count after append m2: %v", err)
	}
	if count != 3 {
		t.Fatalf("conversation should not auto-compress yet, count=%d want 3", count)
	}

	count1h, _, err := m.Index().BucketStats(ctx, recall.Bucket1H)
	if err != nil {
		t.Fatalf("bucket stats after append m2: %v", err)
	}
	if count1h != 0 {
		t.Fatalf("unexpected auto-compress after append m2, 1h segments=%d", count1h)
	}

	if err := conv.Append(ctx, Message{Role: RoleUser, Content: "u3", Timestamp: 5000}); err != nil {
		t.Fatalf("append u3: %v", err)
	}

	count, err = conv.Count(ctx)
	if err != nil {
		t.Fatalf("count after append u3: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected auto-compress at threshold, count=%d want 0", count)
	}

	count1h, _, err = m.Index().BucketStats(ctx, recall.Bucket1H)
	if err != nil {
		t.Fatalf("bucket stats after threshold compress: %v", err)
	}
	if count1h == 0 {
		t.Fatal("expected at least one segment after threshold compress")
	}
}
