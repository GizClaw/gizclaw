//go:build gizclaw_e2e

package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type doubaoRealtimeQualityRound struct {
	ContainsAll []string `json:"contains_all,omitempty"`
	ContainsAny []string `json:"contains_any,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
	MaxRunes    int      `json:"max_runes,omitempty"`
}

type doubaoRealtimeQualityFixture struct {
	Quality struct {
		Rounds []doubaoRealtimeQualityRound `json:"rounds"`
	} `json:"quality"`
}

func TestDoubaoRealtimeResponseQuality(t *testing.T) {
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	path := selectedWorkspaceConfigPaths(t, "doubao-realtime-quality.json")[0]
	spec, err := loadDoubaoRealtimeQualityFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN", createChatRegistrationToken(t, workspaceCaseDoubaoRealtimeQuality))
	result, err := runDoubaoRealtimeQualityWithRetry(path, clientContextConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDoubaoRealtimeQuality(result.Rounds, spec.Quality.Rounds); err != nil {
		t.Fatal(err)
	}
}

func loadDoubaoRealtimeQualityFixture(path string) (doubaoRealtimeQualityFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return doubaoRealtimeQualityFixture{}, fmt.Errorf("read quality fixture: %w", err)
	}
	var fixture doubaoRealtimeQualityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return doubaoRealtimeQualityFixture{}, fmt.Errorf("decode quality fixture: %w", err)
	}
	if len(fixture.Quality.Rounds) != 8 {
		return doubaoRealtimeQualityFixture{}, fmt.Errorf("quality fixture rounds = %d, want 8", len(fixture.Quality.Rounds))
	}
	return fixture, nil
}

func runDoubaoRealtimeQualityWithRetry(path, contextConfigPath string) (workspaceCaseResult, error) {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		started := time.Now()
		fmt.Printf("workspace_case_attempt case=%s config=%s attempt=%d\n", workspaceCaseDoubaoRealtimeQuality, filepath.Base(path), attempt)
		cfg, err := loadConfig(path, contextConfigPath)
		if err != nil {
			return workspaceCaseResult{}, err
		}
		cfg.workspaceSuffix = fmt.Sprintf("run-%d", os.Getpid())
		if attempt > 1 {
			cfg.workspaceSuffix += fmt.Sprintf("-retry-%d", attempt)
		}
		result, err := runLoadedConfigWithResult(cfg, workspaceCaseDoubaoRealtimeQuality)
		lastErr = err
		retryable := isRetryableLiveWorkspaceError(err)
		outcome := "pass"
		if err != nil {
			outcome = "fail"
		}
		fmt.Printf("workspace_case_attempt_done case=%s config=%s attempt=%d result=%s retryable=%t elapsed=%s\n",
			workspaceCaseDoubaoRealtimeQuality, filepath.Base(path), attempt, outcome, retryable, time.Since(started).Truncate(time.Millisecond))
		if err == nil || !retryable {
			return result, err
		}
		if attempt < 5 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return workspaceCaseResult{}, lastErr
}

func validateDoubaoRealtimeQuality(stats []roundStats, expected []doubaoRealtimeQualityRound) error {
	if len(stats) != len(expected) {
		return fmt.Errorf("doubao realtime quality rounds = %d, want %d", len(stats), len(expected))
	}
	for i, constraint := range expected {
		text := normalizeTranscript(stats[i].AssistantText)
		if text == "" {
			return fmt.Errorf("doubao realtime quality round %d has empty Assistant text", i+1)
		}
		for _, value := range constraint.ContainsAll {
			if normalized := normalizeTranscript(value); normalized == "" || !strings.Contains(text, normalized) {
				return fmt.Errorf("doubao realtime quality round %d response %q does not contain %q", i+1, stats[i].AssistantText, value)
			}
		}
		if len(constraint.ContainsAny) > 0 {
			matched := false
			for _, value := range constraint.ContainsAny {
				if normalized := normalizeTranscript(value); normalized != "" && strings.Contains(text, normalized) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("doubao realtime quality round %d response %q contains none of %v", i+1, stats[i].AssistantText, constraint.ContainsAny)
			}
		}
		for _, value := range constraint.Excludes {
			if normalized := normalizeTranscript(value); normalized != "" && strings.Contains(text, normalized) {
				return fmt.Errorf("doubao realtime quality round %d response %q contains superseded value %q", i+1, stats[i].AssistantText, value)
			}
		}
		if constraint.MaxRunes > 0 && runeCount(text) > constraint.MaxRunes {
			return fmt.Errorf("doubao realtime quality round %d response length = %d, want <= %d: %q",
				i+1, runeCount(text), constraint.MaxRunes, stats[i].AssistantText)
		}
	}
	return nil
}
