//go:build gizclaw_memory_e2e

package memoryintegration_test

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/memory"
)

// 场景 1：加载不同 size 数据文件，且对每一档都运行真实模型的
// segmentation + profile（ExtractEntities）链路。
func TestLoadDifferentSizeFilesWithRealModelSegmentationAndProfile(t *testing.T) {
	runtime := requireRealModelRuntime(t)
	segCorpus := loadSegtestCorpus(t)
	scenarioTotal := sumScenarioMessagesByMeta(t)
	totalExamples := segCorpus.TotalMessages + scenarioTotal

	if segCorpus.TotalCases < 50 {
		t.Fatalf("expected >= 50 seg cases, got %d", segCorpus.TotalCases)
	}
	if totalExamples < 20_000 {
		t.Fatalf("expected >= 20000 examples across corpora, got %d (seg_cases=%d scenarios=%d)", totalExamples, segCorpus.TotalMessages, scenarioTotal)
	}

	selectedNames := []string{"m01_single_person", "m05_family_week", "m10_comprehensive"}
	expectedMins := []int{10, 100, 10000}
	selected := make([][]memory.Message, 0, len(selectedNames))
	for _, name := range selectedNames {
		selected = append(selected, loadScenarioMessages(t, name, 0))
	}

	compressor := newRealModelCompressor(runtime)
	prevMessages := 0
	for idx, messages := range selected {
		if len(messages) < expectedMins[idx] {
			t.Fatalf("scenario %s expected at least %d messages, got %d", selectedNames[idx], expectedMins[idx], len(messages))
		}
		if len(messages) <= prevMessages {
			t.Fatalf("selected size order invalid: case=%s messages=%d prev=%d", selectedNames[idx], len(messages), prevMessages)
		}
		prevMessages = len(messages)

		ctx, cancel := context.WithTimeout(context.Background(), runtime.Timeout*2)
		result, err := compressor.CompressMessages(ctx, messages)
		cancel()
		requireNoLiveError(t, "compress "+selectedNames[idx], err)

		if result == nil || len(result.Segments) == 0 {
			t.Fatalf("compress %s returned empty segments", selectedNames[idx])
		}

		ctx, cancel = context.WithTimeout(context.Background(), runtime.Timeout*2)
		update, err := compressor.ExtractEntities(ctx, messages)
		cancel()
		requireNoLiveError(t, "profile "+selectedNames[idx], err)

		if update == nil {
			t.Fatalf("profile %s returned nil update", selectedNames[idx])
		}
		if len(update.Entities) == 0 && len(update.Relations) == 0 {
			t.Fatalf("profile %s returned empty entities and relations", selectedNames[idx])
		}
	}
}
