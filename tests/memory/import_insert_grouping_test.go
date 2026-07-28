//go:build gizclaw_memory_e2e

package memoryintegration_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/memory"
)

// 场景 3：导入(import)与插入(insert)测试，重点校验分组(label group)正确性。
func TestImportAndInsertGrouping(t *testing.T) {
	runtime := requireRealModelRuntime(t)
	compressor := newRealModelCompressor(runtime)
	importMessages := loadScenarioMessages(t, "m10_comprehensive", 2400)
	insertMessages := loadScenarioMessages(t, "m05_family_week", 800)

	host := newIntegrationHost(t, nil)
	mem, err := host.Open("grouping-case")
	if err != nil {
		t.Fatalf("open grouping-case memory: %v", err)
	}

	ctx := context.Background()

	for j, chunk := range chunkMessages(importMessages, 120) {
		err := ingestMessagesWithGroup(ctx, mem, compressor, chunk, "group:import")
		requireNoLiveError(t, fmt.Sprintf("ingest import chunk=%d", j+1), err)
	}

	for j, chunk := range chunkMessages(insertMessages, 80) {
		err := ingestMessagesWithGroup(ctx, mem, compressor, chunk, "group:insert")
		requireNoLiveError(t, fmt.Sprintf("ingest insert chunk=%d", j+1), err)
	}

	importResult, err := mem.Recall(ctx, memory.RecallQuery{
		Labels: []string{"group:import"},
		Text:   "恐龙",
		Hops:   1,
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("recall import group: %v", err)
	}
	if len(importResult.Segments) == 0 {
		t.Fatal("expected recall segments for group:import")
	}
	if !allSegmentsHaveLabel(importResult.Segments, "group:import") {
		t.Fatalf("import recall contains ungrouped segment: %+v", importResult.Segments)
	}
	if anySegmentHasLabel(importResult.Segments, "group:insert") {
		t.Fatalf("import recall leaked insert group: %+v", importResult.Segments)
	}

	insertResult, err := mem.Recall(ctx, memory.RecallQuery{
		Labels: []string{"group:insert"},
		Text:   "恐龙",
		Hops:   1,
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("recall insert group: %v", err)
	}
	if len(insertResult.Segments) == 0 {
		t.Fatal("expected recall segments for group:insert")
	}
	if !allSegmentsHaveLabel(insertResult.Segments, "group:insert") {
		t.Fatalf("insert recall contains ungrouped segment: %+v", insertResult.Segments)
	}
	if anySegmentHasLabel(insertResult.Segments, "group:import") {
		t.Fatalf("insert recall leaked import group: %+v", insertResult.Segments)
	}
}
