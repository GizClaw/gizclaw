package giztest

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunTaskRunsFinalizersAfterClientSetupFailure(t *testing.T) {
	var output bytes.Buffer
	doc := &Document{
		Name: "setup-failure", Path: "setup-failure.giztest.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"endpoint": {Direction: "input", Type: "string", Value: "http://invalid"},
			"evidence": {Direction: "input", Type: "string", Value: "cleanup-ran"},
		},
		Clients: map[string]ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "${endpoint}"}},
		Finally: []Step{{ID: "emit_cleanup", Output: &OutputOperation{Variable: "evidence"}}},
	}
	result := runTask(context.Background(), task{doc: doc}, runOptions{in: nil, out: &output})
	if result.Status != "failed" || len(result.Cleanup) != 1 || result.Cleanup[0].Status != "passed" {
		t.Fatalf("result = %#v", result)
	}
	if output.String() != "evidence=cleanup-ran\n" {
		t.Fatalf("cleanup output = %q", output.String())
	}
}

func TestRunDocumentsAdmitsBarrierGroupAtomically(t *testing.T) {
	block := make(chan struct{})
	started := make(chan string, 3)
	var once sync.Once
	docBefore := &Document{Name: "before", Path: "before.giztest.yaml", Repeat: 1, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "review", ReviewOp: &ReviewOperation{Message: "before"}}}}
	docGroup := &Document{Name: "group", Path: "group.giztest.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	in := &blockingReviewReader{block: block, started: started, once: &once}
	done := make(chan Report, 1)
	go func() {
		done <- runDocuments(context.Background(), []*Document{docBefore, docGroup}, runOptions{parallel: 2, in: in, out: io.Discard})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("leading task did not start")
	}
	select {
	case report := <-done:
		t.Fatalf("barrier group started before two slots were available: %#v", report)
	case <-time.After(50 * time.Millisecond):
	}
	close(block)
	select {
	case report := <-done:
		if report.Status != "passed" {
			t.Fatalf("report = %#v", report)
		}
		for _, result := range report.Tasks {
			if result.Name == "group" && result.DurationMS >= 25 {
				t.Fatalf("barrier task was partially admitted for %dms", result.DurationMS)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
}

type blockingReviewReader struct {
	block   <-chan struct{}
	started chan<- string
	once    *sync.Once
}

func (r *blockingReviewReader) Read(p []byte) (int, error) {
	r.once.Do(func() { r.started <- "started" })
	<-r.block
	copy(p, "pass\n")
	return 5, nil
}

func TestRunDocumentsRejectsInsufficientBarrierCapacityOffline(t *testing.T) {
	doc := &Document{Name: "benchmark.case", Path: "case.giztest.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	report := runDocuments(context.Background(), []*Document{doc}, runOptions{parallel: 1, in: nil, out: io.Discard})
	if report.Status != "failed" || len(report.Tasks) != 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunDocumentsReportsEveryCancelledTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := &Document{Name: "cancelled", Path: "cancelled.giztest.yaml", Repeat: 3, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 3}}}}
	report := runDocuments(ctx, []*Document{doc}, runOptions{parallel: 3, out: io.Discard})
	if len(report.Tasks) != 3 || report.Status != "failed" {
		t.Fatalf("report = %#v", report)
	}
}

func TestAssertValueExtendedMatchers(t *testing.T) {
	value := map[string]any{
		"text":          []string{"乌龟把种", "子交给小鸟，", "小鸟种下了种子。"},
		"joined":        "PASS",
		"fragments_any": []any{"hello ", "world"},
		"first_text_ms": int64(1200),
		"ratio":         2.5,
	}
	minInt := func(v int) *int { return &v }
	minFloat := func(v float64) *float64 { return &v }
	pass := map[string]map[string]Expectation{
		"contains on joined fragments":  {"/text": {Contains: "小鸟种下"}},
		"contains_all across fragments": {"/text": {ContainsAll: []string{"乌龟", "小鸟", "种子"}}},
		"contains_any single hit":       {"/text": {ContainsAny: []string{"发芽", "种子"}}},
		"not_contains clean":            {"/text": {NotContains: []any{"###", "```"}}},
		"not_contains scalar operand":   {"/joined": {NotContains: "FAIL"}},
		"pattern on plain string":       {"/joined": {Pattern: "^PASS$"}},
		"rune window":                   {"/text": {MinLength: minInt(10), MaxLength: minInt(20)}},
		"maximum on int64":              {"/first_text_ms": {Maximum: minFloat(6000)}},
		"minimum on float":              {"/ratio": {Minimum: minFloat(1)}},
		"combined matchers":             {"/joined": {Contains: "PAS", Pattern: "^[A-Z]+$", MaxLength: minInt(4)}},
		"any-typed fragment join":       {"/fragments_any": {Contains: "hello world"}},
	}
	for name, assertions := range pass {
		t.Run("pass "+name, func(t *testing.T) {
			if err := assertValue(assertions, value); err != nil {
				t.Fatalf("assertValue() error = %v", err)
			}
		})
	}
	fail := map[string]map[string]Expectation{
		"contains missing":          {"/text": {Contains: "恐龙"}},
		"contains_all partial":      {"/text": {ContainsAll: []string{"乌龟", "恐龙"}}},
		"contains_any all missing":  {"/text": {ContainsAny: []string{"恐龙", "大象"}}},
		"not_contains hit":          {"/text": {NotContains: []any{"种子"}}},
		"pattern mismatch":          {"/joined": {Pattern: "^FAIL$"}},
		"min_length short":          {"/joined": {MinLength: minInt(10)}},
		"max_length long":           {"/text": {MaxLength: minInt(3)}},
		"maximum exceeded":          {"/first_text_ms": {Maximum: minFloat(1000)}},
		"minimum unmet":             {"/ratio": {Minimum: minFloat(3)}},
		"string matcher on number":  {"/first_text_ms": {Contains: "1"}},
		"numeric matcher on string": {"/joined": {Maximum: minFloat(1)}},
	}
	for name, assertions := range fail {
		t.Run("fail "+name, func(t *testing.T) {
			if err := assertValue(assertions, value); err == nil {
				t.Fatal("assertValue() accepted a failing matcher")
			}
		})
	}
}

func TestAssertValueContentMatcherFailuresAreContentFree(t *testing.T) {
	value := map[string]any{"text": []string{"secret story about 乌龟"}}
	assertions := map[string]Expectation{"/text": {NotContains: []any{"乌龟"}, Contains: "乌龟"}}
	err := assertValue(assertions, value)
	if err == nil {
		t.Fatal("assertValue() accepted a failing matcher")
	}
	for _, banned := range []string{"secret", "乌龟"} {
		if strings.Contains(err.Error(), banned) {
			t.Fatalf("failure message leaks content: %v", err)
		}
	}
}
