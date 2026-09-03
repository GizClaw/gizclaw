package giztest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunTaskRunsFinalizersAfterClientSetupFailure(t *testing.T) {
	var output bytes.Buffer
	doc := &Document{
		Name: "setup-failure", Path: "setup-failure.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"endpoint": {Direction: "input", Type: "string", Value: "http://invalid"},
			"evidence": {Direction: "input", Type: "string", Value: "cleanup-ran"},
		},
		Clients: map[string]ClientSpec{"peer": {Identity: "ephemeral", Connection: "webrtc", AccessPoint: "${endpoint}"}},
		Finally: []Step{{ID: "emit_cleanup", Output: &OutputOperation{Variable: "evidence"}}},
	}
	driver := &stubDriver{openErr: errors.New("dial peer: connection refused")}
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: &output})
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
	docBefore := &Document{Name: "before", Path: "before.yaml", Repeat: 1, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "review", ReviewOp: &ReviewOperation{Message: "before"}}}}
	docGroup := &Document{Name: "group", Path: "group.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	in := &blockingReviewReader{block: block, started: started, once: &once}
	done := make(chan Report, 1)
	go func() {
		done <- Run(context.Background(), []*Document{docBefore, docGroup}, Options{Driver: &stubDriver{}, Parallel: 2, In: in, Out: io.Discard})
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

func (r *blockingReviewReader) Read(p []byte) (int, error) {
	r.once.Do(func() { r.started <- "started" })
	<-r.block
	copy(p, "pass\n")
	return 5, nil
}

type blockingReviewReader struct {
	block   <-chan struct{}
	started chan<- string
	once    *sync.Once
}

func TestRunDocumentsRejectsInsufficientBarrierCapacityOffline(t *testing.T) {
	doc := &Document{Name: "benchmark.case", Path: "case.yaml", Repeat: 2, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 2}}}}
	report := Run(context.Background(), []*Document{doc}, Options{Driver: &stubDriver{}, Parallel: 1, In: nil, Out: io.Discard})
	if report.Status != "failed" || len(report.Tasks) != 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunDocumentsReportsEveryCancelledTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := &Document{Name: "cancelled", Path: "cancelled.yaml", Repeat: 3, Variables: map[string]VariableSpec{}, Clients: map[string]ClientSpec{}, Steps: []Step{{ID: "sync", Barrier: &BarrierOperation{Participants: 3}}}}
	report := Run(ctx, []*Document{doc}, Options{Driver: &stubDriver{}, Parallel: 3, Out: io.Discard})
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
		"server_time":   "1755763200123",
		"nan_string":    "NaN",
		"inf_string":    "+Inf",
		"nan_float":     math.NaN(),
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
		"bounds on protojson int64":     {"/server_time": {Minimum: minFloat(1), Maximum: minFloat(1e15)}},
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
		"contains missing":           {"/text": {Contains: "恐龙"}},
		"contains_all partial":       {"/text": {ContainsAll: []string{"乌龟", "恐龙"}}},
		"contains_any all missing":   {"/text": {ContainsAny: []string{"恐龙", "大象"}}},
		"not_contains hit":           {"/text": {NotContains: []any{"种子"}}},
		"pattern mismatch":           {"/joined": {Pattern: "^FAIL$"}},
		"min_length short":           {"/joined": {MinLength: minInt(10)}},
		"max_length long":            {"/text": {MaxLength: minInt(3)}},
		"maximum exceeded":           {"/first_text_ms": {Maximum: minFloat(1000)}},
		"minimum unmet":              {"/ratio": {Minimum: minFloat(3)}},
		"string matcher on number":   {"/first_text_ms": {Contains: "1"}},
		"numeric matcher on string":  {"/joined": {Maximum: minFloat(1)}},
		"numeric string above bound": {"/server_time": {Maximum: minFloat(10)}},
		"NaN string never satisfies": {"/nan_string": {Minimum: minFloat(0), Maximum: minFloat(1)}},
		"Inf string never satisfies": {"/inf_string": {Minimum: minFloat(0)}},
		"NaN float never satisfies":  {"/nan_float": {Maximum: minFloat(1)}},
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

func TestAssertValueNormalizedMatchers(t *testing.T) {
	value := map[string]any{
		"text":    []any{"下午 四点，", "G"},
		"route":   "今天的观点？",
		"verdict": "ＰＡＳＳ。",
	}
	all := []string{"whitespace", "punctuation", "case", "digits"}
	pass := map[string]Expectation{
		"/text":    {ContainsAll: []string{"四点", "g"}, ContainsAny: []string{"missing", "四 点"}, NotContains: []any{"五点"}, Normalize: all},
		"/route":   {Contains: "今天的观点?", Normalize: []string{"punctuation"}, MinLength: new(6)},
		"/verdict": {Equals: "ｐａｓｓ", Normalize: []string{"case", "punctuation"}},
	}
	if err := assertValue(pass, value); err != nil {
		t.Fatalf("normalized assertion failed: %v", err)
	}
	if err := assertValue(map[string]Expectation{"/route": {Equals: "今天的观点?"}}, value); err == nil {
		t.Fatal("byte-exact equals unexpectedly normalized punctuation")
	}
	if err := assertValue(map[string]Expectation{"/text": {MaxLength: new(5), Contains: "四点", Normalize: []string{"whitespace", "punctuation"}}}, value); err == nil || !strings.Contains(err.Error(), "max_length") {
		t.Fatalf("raw length matcher did not observe original text: %v", err)
	}
	for name, test := range map[string]struct {
		option string
		want   string
	}{
		"whitespace only":  {option: "whitespace", want: "A？１"},
		"punctuation only": {option: "punctuation", want: " A１"},
		"case only":        {option: "case", want: " a？１"},
		"digits only":      {option: "digits", want: " A？1"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := normalizeString(" A？１", []string{test.option}); got != test.want {
				t.Fatalf("normalizeString() = %q, want %q", got, test.want)
			}
		})
	}
	for _, options := range [][]string{{"digits", "case"}, {"case", "digits"}} {
		if got := normalizeString("Ａ１２三", options); got != "ａ123" {
			t.Fatalf("normalizeString() = %q", got)
		}
	}
}

func TestFailureKind(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"none":              {want: ""},
		"assertion":         {err: &AssertionError{err: context.DeadlineExceeded}, want: "assertion"},
		"wrapped timeout":   {err: fmt.Errorf("step timed out: %w", context.DeadlineExceeded), want: "timeout"},
		"wrapped cancelled": {err: fmt.Errorf("step cancelled: %w", context.Canceled), want: "cancelled"},
		"operation":         {err: errors.New("provider rejected request"), want: "operation"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := failureKind(test.err); got != test.want {
				t.Fatalf("failureKind() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAttemptVariableCopiesClearFailedBytes(t *testing.T) {
	input := []byte{9, 8}
	vars, _ := NewVariables(map[string]VariableSpec{
		"input": {Direction: "input", Type: "audio", Value: input, MaxBytes: 4, MediaType: "audio/ogg", Codec: "opus"},
		"audio": {Direction: "output", Type: "audio", MaxBytes: 4, MediaType: "audio/ogg", Codec: "opus"},
	})
	attempt := cloneVariables(vars)
	buffer := []byte{1, 2, 3}
	if err := attempt.assign("audio", buffer); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 0 || vars.values["audio"].data != nil || input[0] != 9 {
		t.Fatalf("failed buffer = %v, committed = %#v, input = %v", buffer, vars.values["audio"].data, input)
	}
}

func TestAttemptVariableCopiesClearNestedFailedBytes(t *testing.T) {
	vars, _ := NewVariables(map[string]VariableSpec{
		"result": {Direction: "output", Type: "object"},
	})
	attempt := cloneVariables(vars)
	buffer := []byte{1, 2, 3}
	object := map[string]any{"parts": []any{buffer}}
	object["cycle"] = object
	if err := attempt.assign("result", object); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 0 || object["parts"] != nil || object["cycle"] != nil || vars.values["result"].data != nil {
		t.Fatalf("discarded object was not cleared: buffer = %v, object = %#v", buffer, object)
	}
}

func TestAttemptVariableCopiesPreserveAliasedInputBytes(t *testing.T) {
	buffer := []byte{1, 2, 3}
	input := map[string]any{"parts": []any{buffer}}
	vars, _ := NewVariables(map[string]VariableSpec{
		"input":  {Direction: "input", Type: "object", Value: input},
		"result": {Direction: "output", Type: "object"},
	})
	attempt := cloneVariables(vars)
	if err := attempt.assign("result", input); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 1 || input["parts"] == nil || vars.values["result"].data != nil {
		t.Fatalf("input alias was cleared: buffer = %v, input = %#v", buffer, input)
	}
}

func TestAttemptVariableCommitTransfersNestedBytes(t *testing.T) {
	vars, _ := NewVariables(map[string]VariableSpec{
		"result": {Direction: "output", Type: "object"},
	})
	attempt := cloneVariables(vars)
	buffer := []byte{1, 2, 3}
	object := map[string]any{"parts": []any{buffer}}
	if err := attempt.assign("result", object); err != nil {
		t.Fatal(err)
	}
	if err := commitAttemptVariables(vars, attempt); err != nil {
		t.Fatal(err)
	}
	releaseAttemptVariables(vars, attempt)
	if buffer[0] != 1 || vars.values["result"].data == nil {
		t.Fatalf("committed object was cleared: buffer = %v, committed = %#v", buffer, vars.values["result"].data)
	}
}

func TestCommitAttemptVariablesValidatesBeforePublishing(t *testing.T) {
	vars, _ := NewVariables(map[string]VariableSpec{
		"first":  {Direction: "output", Type: "string"},
		"second": {Direction: "output", Type: "integer"},
	})
	attempt := cloneVariables(vars)
	first := attempt.values["first"]
	first.data = "ready"
	attempt.values["first"] = first
	second := attempt.values["second"]
	second.data = "wrong type"
	attempt.values["second"] = second
	if err := commitAttemptVariables(vars, attempt); err == nil {
		t.Fatal("commitAttemptVariables() accepted an invalid pending value")
	}
	if vars.values["first"].data != nil || vars.values["second"].data != nil {
		t.Fatalf("partial outputs published: %#v", vars.values)
	}
}

func TestResolveExpectationsSubstitutesReferences(t *testing.T) {
	vars, err := NewVariables(map[string]VariableSpec{
		"name":  {Direction: "input", Type: "string", Value: "contact-7f3a"},
		"count": {Direction: "input", Type: "integer", Value: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveExpectations(vars, map[string]Expectation{
		"/name":    {Equals: "${name}"},
		"/summary": {Contains: "id ${name}", ContainsAll: []string{"${name}"}},
		"/total":   {Equals: "${count}"},
		"/note":    {NotContains: []any{"${name}"}},
		"/pattern": {Pattern: "^${name}$"},
	})
	if err != nil {
		t.Fatalf("resolveExpectations() error = %v", err)
	}
	if got := resolved["/name"].Equals; got != "contact-7f3a" {
		t.Fatalf("equals = %#v, want the variable value", got)
	}
	if got := resolved["/summary"].Contains; got != "id contact-7f3a" {
		t.Fatalf("contains = %q, want the embedded reference substituted", got)
	}
	if got := resolved["/summary"].ContainsAll[0]; got != "contact-7f3a" {
		t.Fatalf("contains_all = %q", got)
	}
	if got := resolved["/total"].Equals; got != 3 {
		t.Fatalf("equals = %#v, want the typed variable value", got)
	}
	notContains, err := resolved["/note"].notContainsList()
	if err != nil || len(notContains) != 1 || notContains[0] != "contact-7f3a" {
		t.Fatalf("not_contains = %#v, %v", notContains, err)
	}
	if got := resolved["/pattern"].Pattern; got != "^contact-7f3a$" {
		t.Fatalf("pattern = %q, want the reference substituted", got)
	}

	// The resolved expectation is what the assertion sees.
	if err := assertValue(resolved, map[string]any{
		"/name": "contact-7f3a",
	}); err == nil {
		t.Fatal("assertValue must still evaluate the resolved expectations")
	}
	if err := assertValue(map[string]Expectation{"/name": resolved["/name"]}, map[string]any{
		"name": "contact-7f3a",
	}); err != nil {
		t.Fatalf("resolved equals did not match: %v", err)
	}
}

func TestResolveExpectationsRejectsUnavailableVariable(t *testing.T) {
	vars, err := NewVariables(map[string]VariableSpec{
		"result": {Direction: "output", Type: "string"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExpectations(vars, map[string]Expectation{
		"/name": {Equals: "${result}"},
	}); err == nil {
		t.Fatal("an unproduced output variable must not silently compare literally")
	}
}

func TestLoadSupportedDocumentsSeparatesUnsupportedFromBroken(t *testing.T) {
	supportedPath := writeTestDocument(t, validDocument)
	// The same document with its rpc step replaced by one no stub driver runs.
	unsupported := strings.Replace(validDocument, `    rpc:
      method: all.ping
      request: {}`, `    speech:
      method: server.speech.synthesize
      request: {}`, 1)
	unsupported = strings.Replace(unsupported, "name: ping-connectivity", "name: speech-synthesis", 1)
	unsupportedPath := writeTestDocument(t, unsupported)

	documents, skipped, err := LoadSupportedDocuments(
		[]string{supportedPath, unsupportedPath}, &stubDriver{operations: []string{"rpc"}})
	if err != nil {
		t.Fatalf("LoadSupportedDocuments() error = %v", err)
	}
	if len(documents) != 1 || documents[0].Path != supportedPath {
		t.Fatalf("documents = %#v", documents)
	}
	if len(skipped) != 1 || skipped[0].Path != unsupportedPath {
		t.Fatalf("skipped = %#v", skipped)
	}
	if !strings.Contains(skipped[0].Reason, "speech") {
		t.Fatalf("skip reason must name the operation, got %q", skipped[0].Reason)
	}

	// A document that does not parse stays an error: broken is not skipped.
	broken := writeTestDocument(t, "version: gizclaw.test/v1alpha1\n")
	if _, _, err := LoadSupportedDocuments([]string{broken}, &stubDriver{}); err == nil {
		t.Fatal("a malformed document must not be reported as skipped")
	}
}
