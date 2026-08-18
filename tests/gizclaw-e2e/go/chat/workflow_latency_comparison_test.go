//go:build gizclaw_e2e

package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const defaultWorkflowLatencyComparisonPairs = 5

func TestWorkflowDriverVoiceLatencyComparison(t *testing.T) {
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	pairs := workflowLatencyComparisonPairs(t)
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN",
		createChatRegistrationToken(t, workspaceCasePushToTalkRoundtrip))

	paths := map[string]string{
		"flowcraft": filepath.Join("..", "..", "testdata", "workspaces", "flowcraft-latency-comparison.json"),
		"eino":      filepath.Join("..", "..", "testdata", "workspaces", "eino-latency-comparison.json"),
	}
	results := map[string][]roundStats{"flowcraft": {}, "eino": {}}

	for _, driver := range []string{"flowcraft", "eino"} {
		runWorkflowLatencySample(t, paths[driver], driver, "warmup", 0)
	}
	for pair := 1; pair <= pairs; pair++ {
		order := []string{"flowcraft", "eino"}
		if pair%2 == 0 {
			order[0], order[1] = order[1], order[0]
		}
		for _, driver := range order {
			stat := runWorkflowLatencySample(t, paths[driver], driver, "measure", pair)
			results[driver] = append(results[driver], stat)
		}
	}

	report := map[string]any{
		"pairs": pairs,
		"mode":  "push-to-talk",
		"controls": map[string]any{
			"asr":           "asr",
			"llm":           "llm",
			"tts":           "tts",
			"voice":         "assistant-voice",
			"temperature":   0,
			"utterance":     "请报告当前系统状态",
			"serial_stages": []string{"memory_recall", "capture_input", "planner_prompt", "planner_llm", "answer_prompt", "answer_llm", "memory_observe", "completion_barrier"},
			"llm_calls":     2,
			"max_tokens":    []int{64, 128},
			"memory":        "default-memory",
		},
		"flowcraft": workflowLatencySummary(results["flowcraft"]),
		"eino":      workflowLatencySummary(results["eino"]),
		"samples": map[string][]map[string]float64{
			"flowcraft": workflowLatencySamples(results["flowcraft"]),
			"eino":      workflowLatencySamples(results["eino"]),
		},
	}
	fmt.Printf("workflow_latency_comparison=%s\n", encodeJSONLine(report))
}

func workflowLatencySamples(stats []roundStats) []map[string]float64 {
	samples := make([]map[string]float64, 0, len(stats))
	for _, stat := range stats {
		samples = append(samples, map[string]float64{
			"model_text_first_after_transcript_done_ms":  durationMilliseconds(textAfterTranscriptDone(stat)),
			"text_done_after_transcript_done_ms":         durationMilliseconds(positiveDurationDifference(stat.AssistantTextDone, stat.TranscriptDone)),
			"tts_audio_first_after_text_first_ms":        durationMilliseconds(positiveDurationDifference(stat.FirstAudioChunk, stat.FirstAssistantTextChunk)),
			"response_complete_after_transcript_done_ms": durationMilliseconds(positiveDurationDifference(stat.ResponseTotal, stat.TranscriptDone)),
		})
	}
	return samples
}

func workflowLatencyComparisonPairs(t testing.TB) int {
	t.Helper()
	value := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_WORKFLOW_LATENCY_PAIRS"))
	if value == "" {
		return defaultWorkflowLatencyComparisonPairs
	}
	pairs, err := strconv.Atoi(value)
	if err != nil || pairs <= 0 || pairs > 20 {
		t.Fatalf("GIZCLAW_E2E_WORKFLOW_LATENCY_PAIRS = %q, want integer 1..20", value)
	}
	return pairs
}

func runWorkflowLatencySample(t *testing.T, path, driver, phase string, pair int) roundStats {
	t.Helper()
	cfg, err := loadConfig(path, clientContextConfigPath())
	if err != nil {
		t.Fatalf("load %s latency config: %v", driver, err)
	}
	cfg.Rounds = 1
	// Reuse one Workspace per driver so ensureWorkspaceForRun deletes the
	// previous sample before recreating it. Merely stopping a differently named
	// Workspace leaves its local memory resource open and can retain Badger's
	// directory lock across samples.
	cfg.workspaceSuffix = "benchmark"
	result, err := runLoadedConfigWithResult(cfg, workspaceCasePushToTalkRoundtrip)
	if err != nil {
		t.Fatalf("%s %s pair %d: %v", driver, phase, pair, err)
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("%s %s pair %d rounds = %d, want 1", driver, phase, pair, len(result.Rounds))
	}
	stat := result.Rounds[0]
	if !stat.FirstAudioBeforeTextDone {
		t.Fatalf("%s %s pair %d did not start TTS before text EOS", driver, phase, pair)
	}
	return stat
}

func workflowLatencySummary(stats []roundStats) map[string]timingSummary {
	return map[string]timingSummary{
		"model_text_first_after_transcript_done": summarizeDurations(stats, textAfterTranscriptDone),
		"text_done_after_transcript_done": summarizeDurations(stats, func(stat roundStats) time.Duration {
			return positiveDurationDifference(stat.AssistantTextDone, stat.TranscriptDone)
		}),
		"tts_audio_first_after_text_first": summarizeDurations(stats, func(stat roundStats) time.Duration {
			return positiveDurationDifference(stat.FirstAudioChunk, stat.FirstAssistantTextChunk)
		}),
		"response_complete_after_transcript_done": summarizeDurations(stats, func(stat roundStats) time.Duration {
			return positiveDurationDifference(stat.ResponseTotal, stat.TranscriptDone)
		}),
	}
}

func positiveDurationDifference(end, start time.Duration) time.Duration {
	if end <= start {
		return 0
	}
	return end - start
}
