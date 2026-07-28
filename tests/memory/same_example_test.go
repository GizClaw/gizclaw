//go:build gizclaw_memory_e2e

package memoryintegration_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/memory"
)

// 场景 2：同一个 example（同一源 case）按 small/medium/large 三档运行，
// 对比 recall 输出与图谱结果。
func TestRunSameExampleAcrossDatasetSizes(t *testing.T) {
	runtime := requireRealModelRuntime(t)
	// 使用同一个大场景 example：m10_comprehensive。
	base := loadScenarioMessages(t, "m10_comprehensive", 0)
	if len(base) < 3000 {
		t.Fatalf("base example m10_comprehensive too short: %d", len(base))
	}

	type sizedInput struct {
		name     string
		messages []memory.Message
	}

	variants := []sizedInput{
		{name: "small", messages: base[:120]},
		{name: "medium", messages: base[:1200]},
		{name: "large", messages: base[:3000]},
	}

	compressor := newRealModelCompressor(runtime)
	segmentCounts := make(map[string]int, len(variants))

	for _, variant := range variants {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			ctx := context.Background()

			host := newIntegrationHost(t, nil)
			mem, err := host.Open("same-example-" + variant.name)
			if err != nil {
				t.Fatalf("open memory %s: %v", variant.name, err)
			}

			chunks := chunkMessages(variant.messages, 80)
			for idx, chunk := range chunks {
				err := ingestMessagesWithGroup(ctx, mem, compressor, chunk, "group:same-example")
				requireNoLiveError(t, fmt.Sprintf("ingest %s chunk %d", variant.name, idx+1), err)
			}

			result, err := mem.Recall(ctx, memory.RecallQuery{
				Labels: []string{"group:same-example"},
				Text:   "小明 喜欢 什么",
				Hops:   2,
				Limit:  10,
			})
			if err != nil {
				t.Fatalf("recall %s: %v", variant.name, err)
			}
			if len(result.Segments) == 0 {
				t.Fatalf("expected recall segments for %s", variant.name)
			}

			containsPerson := false
			containsDino := false
			for _, seg := range result.Segments {
				if strings.Contains(seg.Summary, "小明") {
					containsPerson = true
				}
				if strings.Contains(seg.Summary, "恐龙") {
					containsDino = true
				}
			}
			if !containsPerson {
				t.Fatalf("recall %s lacks person context: %+v", variant.name, result.Segments)
			}
			if !containsDino {
				t.Fatalf("recall %s lacks dinosaur context: %+v", variant.name, result.Segments)
			}

			entityCount, err := graphEntityCount(ctx, mem)
			if err != nil {
				t.Fatalf("list entities: %v", err)
			}
			if entityCount == 0 {
				t.Fatalf("expected graph entities for %s", variant.name)
			}

			recent, err := mem.Index().RecentSegments(ctx, 1000)
			if err != nil {
				t.Fatalf("recent segments %s: %v", variant.name, err)
			}
			segmentCounts[variant.name] = len(recent)
		})
	}

	if segmentCounts["small"] > segmentCounts["medium"] {
		t.Fatalf("segment count regression: small=%d medium=%d", segmentCounts["small"], segmentCounts["medium"])
	}
	if segmentCounts["medium"] > segmentCounts["large"] {
		t.Fatalf("segment count regression: medium=%d large=%d", segmentCounts["medium"], segmentCounts["large"])
	}
}
