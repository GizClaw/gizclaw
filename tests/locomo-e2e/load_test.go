//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mem0sdk "github.com/GizClaw/gizclaw-go/sdk/go/mem0"
)

type loadSample struct {
	Write       bool      `json:"write"`
	CompletedAt time.Time `json:"completed_at"`
	LatencyMS   float64   `json:"latency_ms"`
	Facts       int       `json:"facts,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type loadMetrics struct {
	Completed int     `json:"completed"`
	Failed    int     `json:"failed"`
	Rate      float64 `json:"completed_per_second"`
	P95MS     float64 `json:"p95_ms"`
}

// TestMem0SDKConcurrentLoad measures successful real extraction/persistence and
// semantic reads. Warmup/drain are excluded from the completion-rate window.
func TestMem0SDKConcurrentLoad(t *testing.T) {
	if os.Getenv("GIZCLAW_MEM0_LOAD") != "1" {
		t.Skip("set GIZCLAW_MEM0_LOAD=1 for credential-backed load validation")
	}
	endpoint := os.Getenv("GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL")
	if endpoint == "" {
		t.Fatal("GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL is required")
	}
	health := requirePGVectorHealth(t, endpoint)
	writeRate := envInt(t, "GIZCLAW_MEM0_LOAD_WRITE_RPS", 32)
	readRate := envInt(t, "GIZCLAW_MEM0_LOAD_READ_RPS", 12)
	seconds := envInt(t, "GIZCLAW_MEM0_LOAD_SECONDS", 60)
	warmup := envInt(t, "GIZCLAW_MEM0_LOAD_WARMUP_SECONDS", 10)
	minimumWrites := envInt(t, "GIZCLAW_MEM0_LOAD_MIN_WRITE_RPS", 30)
	minimumReads := envInt(t, "GIZCLAW_MEM0_LOAD_MIN_READ_RPS", 10)
	if writeRate < 1 || writeRate > 100 || readRate < 1 || readRate > 100 || seconds < 5 || seconds > 300 || warmup < 1 || warmup > 60 {
		t.Fatal("load rates/duration outside bounded test limits")
	}
	client, err := mem0sdk.NewClientWithResponses(endpoint, mem0sdk.WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
		if key := os.Getenv("GIZCLAW_MEM0_LOAD_API_KEY"); key != "" {
			request.Header.Set("X-API-Key", key)
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("gizclaw-load-%d-", time.Now().UnixNano())
	policy := "Extract only the fictional participant's durable preferred drink. Ignore temporary conversational filler."
	var scopes []string
	var scopeMu sync.Mutex
	var reportPath string
	cleanup := func() {
		jobs := make(chan string)
		var cleaners sync.WaitGroup
		var verified atomic.Int64
		for range 16 {
			cleaners.Go(func() {
				for scope := range jobs {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					deleted, deleteErr := client.DeleteMemoriesWithResponse(ctx, &mem0sdk.DeleteMemoriesParams{UserId: &scope})
					listed, listErr := client.ListMemoriesWithResponse(ctx, &mem0sdk.ListMemoriesParams{UserId: &scope, TopK: new(1)})
					cancel()
					if deleteErr != nil || deleted.StatusCode() != 200 || listErr != nil || listed.JSON200 == nil || len(listed.JSON200.Results) != 0 {
						t.Errorf("load scope cleanup failed for generated scope %q", scope)
					} else {
						verified.Add(1)
					}
				}
			})
		}
		for _, scope := range scopes {
			jobs <- scope
		}
		close(jobs)
		cleaners.Wait()
		if reportPath != "" {
			raw, err := os.ReadFile(reportPath)
			var report map[string]any
			if err == nil {
				err = json.Unmarshal(raw, &report)
			}
			if err == nil {
				report["cleanup"] = map[string]any{"scopes": len(scopes), "verified_empty": verified.Load()}
				raw, err = json.MarshalIndent(report, "", "  ")
			}
			if err == nil {
				err = os.WriteFile(reportPath, raw, 0o600)
			}
			if err != nil {
				t.Errorf("write load cleanup receipt: %v", err)
			}
		}
	}
	t.Cleanup(cleanup)
	write := func(scope string, index int) loadSample {
		ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
		defer cancel()
		started := time.Now()
		content := fmt.Sprintf("My name is FictionalParticipant%d. My preferred drink is jasmine tea. ", index) + strings.Repeat("We discussed a temporary detail, then continued the conversation without adding a lasting preference. ", 60)
		response, err := client.AddMemoryWithResponse(ctx, mem0sdk.MemoryCreate{
			UserId: &scope, Prompt: &policy, Messages: []mem0sdk.Message{{Role: "user", Content: content}},
		})
		sample := loadSample{Write: true, CompletedAt: time.Now(), LatencyMS: float64(time.Since(started)) / float64(time.Millisecond)}
		if err != nil {
			sample.Error = fmt.Sprintf("transport_%T", err)
		} else if response.JSON200 == nil {
			sample.Error = fmt.Sprintf("HTTP_%d", response.StatusCode())
		} else {
			for _, fact := range response.JSON200.Results {
				if fact.Event != nil && *fact.Event == "ADD" {
					sample.Facts++
				}
			}
			if sample.Facts == 0 {
				sample.Error = "no_persisted_fact"
			}
		}
		return sample
	}
	seed := prefix + "read-seed"
	scopes = append(scopes, seed)
	if sample := write(seed, 0); sample.Error != "" {
		t.Fatalf("load seed failed: %s", sample.Error)
	}
	var samples []loadSample
	var sampleMu sync.Mutex
	var workers sync.WaitGroup
	var inFlight atomic.Int64
	var peak atomic.Int64
	start := time.Now().Add(time.Duration(warmup) * time.Second)
	end := start.Add(time.Duration(seconds) * time.Second)
	launch := func(isWrite bool, index int) {
		workers.Go(func() {
			active := inFlight.Add(1)
			defer inFlight.Add(-1)
			for before := peak.Load(); active > before && !peak.CompareAndSwap(before, active); before = peak.Load() {
			}
			var sample loadSample
			if isWrite {
				scope := fmt.Sprintf("%swrite-%d", prefix, index)
				scopeMu.Lock()
				scopes = append(scopes, scope)
				scopeMu.Unlock()
				sample = write(scope, index+1)
			} else {
				ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
				began := time.Now()
				response, err := client.SearchMemoriesWithResponse(ctx, mem0sdk.SearchRequest{Query: "What drink does the participant prefer?", Filters: map[string]any{"user_id": seed}, TopK: new(5)})
				cancel()
				sample = loadSample{CompletedAt: time.Now(), LatencyMS: float64(time.Since(began)) / float64(time.Millisecond)}
				if err != nil {
					sample.Error = fmt.Sprintf("transport_%T", err)
				} else if response.JSON200 == nil {
					sample.Error = fmt.Sprintf("HTTP_%d", response.StatusCode())
				} else if len(response.JSON200.Results) == 0 {
					sample.Error = "empty_recall"
				}
			}
			sampleMu.Lock()
			samples = append(samples, sample)
			sampleMu.Unlock()
		})
	}
	var schedulers sync.WaitGroup
	for _, setting := range []struct {
		write bool
		rate  int
	}{{true, writeRate}, {false, readRate}} {
		schedulers.Go(func() {
			ticker := time.NewTicker(time.Second / time.Duration(setting.rate))
			defer ticker.Stop()
			for index := 0; ; index++ {
				select {
				case now := <-ticker.C:
					if !now.Before(end) {
						return
					}
					launch(setting.write, index)
				case <-t.Context().Done():
					return
				}
			}
		})
	}
	schedulers.Wait()
	workers.Wait()
	writeMetrics, readMetrics := summarizeLoad(samples, start, end, true), summarizeLoad(samples, start, end, false)
	allErrors := 0
	for _, sample := range samples {
		if sample.Error != "" {
			allErrors++
		}
	}
	report := map[string]any{"sdk_version": health.SdkVersion, "requested_service_tier": health.LlmServiceTier, "extraction_model": health.LlmModel, "embedding_model": health.EmbeddingModel, "embedding_dimensions": health.EmbeddingDimensions, "measurement_seconds": seconds, "minimum_write_rps": minimumWrites, "minimum_read_rps": minimumReads, "all_errors": allErrors, "warmup_seconds": warmup, "offered_write_rps": writeRate, "offered_read_rps": readRate, "peak_in_flight": peak.Load(), "writes": writeMetrics, "reads": readMetrics, "samples": samples, "scope_count": len(scopes), "workload_note": "One independent synthetic workspace per write; approximately 1000 input tokens. Requires at least one persisted fact per successful write. One preseeded scope for semantic reads."}
	directory := envOr("GIZCLAW_LOCOMO_E2E_REPORT_DIR", "tests/locomo-e2e/reports")
	if err := os.MkdirAll(repoPath(directory), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repoPath(directory), fmt.Sprintf("mem0-sdk-load-%d.json", time.Now().UnixNano()))
	reportPath = path
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("SDK load report: %s; writes %.2f/s p95 %.0fms; reads %.2f/s p95 %.0fms; peak %d", path, writeMetrics.Rate, writeMetrics.P95MS, readMetrics.Rate, readMetrics.P95MS, peak.Load())
	if allErrors > 0 || writeMetrics.Rate < float64(minimumWrites) || readMetrics.Rate < float64(minimumReads) {
		t.Errorf("Mem0 did not sustain %d completed persisted writes/s and %d nonempty semantic reads/s in the measurement window", minimumWrites, minimumReads)
	}
}

func summarizeLoad(samples []loadSample, start, end time.Time, write bool) loadMetrics {
	var result loadMetrics
	var latency []float64
	for _, sample := range samples {
		if sample.Write != write || sample.CompletedAt.Before(start) || !sample.CompletedAt.Before(end) {
			continue
		}
		if sample.Error != "" {
			result.Failed++
			continue
		}
		result.Completed++
		latency = append(latency, sample.LatencyMS)
	}
	result.Rate = float64(result.Completed) / end.Sub(start).Seconds()
	slices.Sort(latency)
	if len(latency) > 0 {
		result.P95MS = latency[(len(latency)*95+99)/100-1]
	}
	return result
}

func TestSDKLoadMetricsCountOnlySuccessfulCompletionsInsideWindow(t *testing.T) {
	start := time.Unix(100, 0)
	end := start.Add(10 * time.Second)
	metrics := summarizeLoad([]loadSample{{Write: true, CompletedAt: start, LatencyMS: 5, Facts: 1}, {Write: true, CompletedAt: start.Add(time.Second), Error: "failed"}, {Write: true, CompletedAt: end, LatencyMS: 1}, {Write: false, CompletedAt: start}}, start, end, true)
	if metrics.Completed != 1 || metrics.Failed != 1 || metrics.Rate != 0.1 || metrics.P95MS != 5 {
		t.Fatalf("invalid throughput accounting: %+v", metrics)
	}
}
