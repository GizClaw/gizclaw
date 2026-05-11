package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/agent/recall"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

type multiOutputCompressor struct{}

type singleOutputCompressor struct{}

func (multiOutputCompressor) CompressMessages(_ context.Context, _ []Message) (*CompressResult, error) {
	return &CompressResult{}, nil
}

func (multiOutputCompressor) ExtractEntities(_ context.Context, _ []Message) (*EntityUpdate, error) {
	return &EntityUpdate{}, nil
}

func (multiOutputCompressor) CompactSegments(_ context.Context, _ []string) (*CompressResult, error) {
	return &CompressResult{
		Segments: []SegmentInput{
			{Summary: "compact-A", Keywords: []string{"a"}, Labels: []string{"person:test"}},
			{Summary: "compact-B", Keywords: []string{"b"}, Labels: []string{"person:test"}},
		},
		Summary: "compact-A|compact-B",
	}, nil
}

func (singleOutputCompressor) CompressMessages(_ context.Context, _ []Message) (*CompressResult, error) {
	return &CompressResult{}, nil
}

func (singleOutputCompressor) ExtractEntities(_ context.Context, _ []Message) (*EntityUpdate, error) {
	return &EntityUpdate{}, nil
}

func (singleOutputCompressor) CompactSegments(_ context.Context, _ []string) (*CompressResult, error) {
	return &CompressResult{
		Segments: []SegmentInput{{
			Summary:  "compact-single",
			Keywords: []string{"single"},
			Labels:   []string{"person:test"},
		}},
		Summary: "compact-single",
	}, nil
}

func TestCompactBucketMultiOutputKeepsUniqueSegments(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})
	host, err := NewHost(ctx, HostConfig{
		Store:          store,
		Embedder:       newMockEmbedder(),
		FS:             &testDirFS{root: t.TempDir()},
		Compressor:     multiOutputCompressor{},
		CompressPolicy: CompressPolicy{MaxMessages: 2, MaxChars: 1 << 20},
		Separator:      testSep,
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	mem, err := host.Open("compact-regression")
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}

	origNow := nowNano
	tick := int64(1_000_000_000)
	nowNano = func() int64 {
		tick++
		return tick
	}
	defer func() { nowNano = origNow }()

	for i := range 4 {
		if err := mem.StoreSegment(ctx, SegmentInput{Summary: "src", Labels: []string{"person:test"}}, recall.Bucket1H); err != nil {
			t.Fatalf("store source segment #%d: %v", i+1, err)
		}
	}

	sourceBefore, err := mem.Index().BucketSegments(ctx, recall.Bucket1H)
	if err != nil {
		t.Fatalf("load source bucket before compact: %v", err)
	}
	compactCount := len(sourceBefore) / 2
	if compactCount < 1 {
		compactCount = 1
	}
	if policyMax := 2; len(sourceBefore) > policyMax {
		need := len(sourceBefore) - policyMax + 1
		if need > compactCount {
			compactCount = need
		}
	}
	if compactCount > len(sourceBefore) {
		compactCount = len(sourceBefore)
	}
	expectedLastTS := sourceBefore[compactCount-1].Timestamp

	if err := mem.CompactBucket(ctx, recall.Bucket1H); err != nil {
		t.Fatalf("compact bucket: %v", err)
	}

	coarse, err := mem.Index().BucketSegments(ctx, recall.Bucket1D)
	if err != nil {
		t.Fatalf("load compacted bucket: %v", err)
	}
	if len(coarse) != 2 {
		t.Fatalf("expected 2 compacted segments, got %d", len(coarse))
	}

	if coarse[0].Timestamp == coarse[1].Timestamp {
		t.Fatalf("expected unique timestamps for compact outputs, both are %d", coarse[0].Timestamp)
	}
	for _, seg := range coarse {
		if seg.Timestamp > tick {
			t.Fatalf("compacted segment timestamp drifted to now: ts=%d now=%d", seg.Timestamp, tick)
		}
		if seg.Timestamp < expectedLastTS {
			t.Fatalf("compacted segment timestamp older than compacted last event: got=%d expectedLastTS=%d", seg.Timestamp, expectedLastTS)
		}
	}

	summarySet := map[string]bool{}
	for _, seg := range coarse {
		summarySet[seg.Summary] = true
		got, err := mem.Index().GetSegment(ctx, seg.ID)
		if err != nil {
			t.Fatalf("GetSegment(%s): %v", seg.ID, err)
		}
		if got == nil {
			t.Fatalf("GetSegment(%s) returned nil", seg.ID)
		}
		if got.Summary != seg.Summary {
			t.Fatalf("sid mapping mismatch for %s: got summary %q want %q", seg.ID, got.Summary, seg.Summary)
		}
	}
	if !summarySet["compact-A"] || !summarySet["compact-B"] {
		t.Fatalf("expected summaries compact-A and compact-B, got %v", summarySet)
	}
}

func TestCompactBucketPreservesHistoricalLastTimestamp(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})
	host, err := NewHost(ctx, HostConfig{
		Store:          store,
		Embedder:       newMockEmbedder(),
		FS:             &testDirFS{root: t.TempDir()},
		Compressor:     singleOutputCompressor{},
		CompressPolicy: CompressPolicy{MaxMessages: 2, MaxChars: 1 << 20},
		Separator:      testSep,
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	mem, err := host.Open("compact-timestamp")
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}

	origNow := nowNano
	nowNano = func() int64 { return 9_999_999_999_999 }
	defer func() { nowNano = origNow }()

	for i, ts := range []int64{1000, 2000, 3000, 4000} {
		if err := mem.Index().StoreSegment(ctx, recall.Segment{
			ID:        fmt.Sprintf("src-fixed-%d", i),
			Summary:   "src",
			Timestamp: ts,
			Bucket:    recall.Bucket1H,
		}); err != nil {
			t.Fatalf("store fixed segment ts=%d: %v", ts, err)
		}
	}

	if err := mem.CompactBucket(ctx, recall.Bucket1H); err != nil {
		t.Fatalf("compact bucket: %v", err)
	}

	coarse, err := mem.Index().BucketSegments(ctx, recall.Bucket1D)
	if err != nil {
		t.Fatalf("load compacted bucket: %v", err)
	}
	if len(coarse) != 1 {
		t.Fatalf("expected one compacted segment, got %d", len(coarse))
	}
	if coarse[0].Timestamp != 3000 {
		t.Fatalf("compacted timestamp = %d, want historical lastTS=3000", coarse[0].Timestamp)
	}
}
