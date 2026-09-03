// Package giztest owns the Giztest scenario language: the document schema and
// its semantics, variables, captures and expectations, the task runner, and
// the JSON report. It executes client-facing steps through a Driver, so the
// gizclaw CLI and the cgo end-to-end runner share one implementation of
// everything except how a client is dialed.
package giztest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Options configures one Run.
type Options struct {
	// Driver executes every client-facing step. Required.
	Driver Driver
	// Parallel bounds concurrent tasks across all selected documents.
	Parallel int
	// In is the terminal a review step reads its verdict from.
	In io.Reader
	// Out is where an output step writes and where progress is reported.
	Out io.Writer
}

type task struct {
	doc     *Document
	index   int
	barrier *TaskBarrier
}

func Run(ctx context.Context, docs []*Document, opts Options) Report {
	started := time.Now()
	report := Report{Version: "v1", StartedAt: started}
	var groups [][]task
	taskCount := 0
	for _, doc := range docs {
		var barrier *TaskBarrier
		if documentHasBarrier(doc) {
			barrier = NewTaskBarrier(doc.Repeat)
			if opts.Parallel < doc.Repeat {
				for i := range doc.Repeat {
					report.Tasks = append(report.Tasks, TaskReport{Path: doc.Path, Name: doc.Name, TaskID: fmt.Sprintf("%s-%04d", doc.Name, i), RepeatIndex: i, Status: "failed", Error: fmt.Sprintf("parallelism %d is below barrier group %d", opts.Parallel, doc.Repeat)})
				}
				continue
			}
		}
		group := make([]task, 0, doc.Repeat)
		for i := range doc.Repeat {
			item := task{doc: doc, index: i, barrier: barrier}
			if barrier == nil {
				groups = append(groups, []task{item})
			} else {
				group = append(group, item)
			}
			taskCount++
		}
		if barrier != nil {
			groups = append(groups, group)
		}
	}
	results := make(chan TaskReport, taskCount)
	slots := make(chan struct{}, opts.Parallel)
	var wg sync.WaitGroup
	go func() {
		defer func() {
			wg.Wait()
			close(results)
		}()
		for groupIndex, group := range groups {
			acquired := 0
			for acquired < len(group) {
				if ctx.Err() != nil {
					break
				}
				select {
				case slots <- struct{}{}:
					acquired++
				case <-ctx.Done():
				}
			}
			if acquired != len(group) {
				for range acquired {
					<-slots
				}
				for _, remaining := range groups[groupIndex:] {
					for _, item := range remaining {
						results <- cancelledTaskReport(item, ctx)
					}
				}
				return
			}
			for _, item := range group {
				wg.Go(func() {
					defer func() { <-slots }()
					results <- runTask(ctx, item, opts)
				})
			}
		}
	}()
	for result := range results {
		report.Tasks = append(report.Tasks, result)
	}
	report.finish(started)
	return report
}

func cancelledTaskReport(item task, ctx context.Context) TaskReport {
	err := context.Cause(ctx)
	if err == nil {
		err = ctx.Err()
	}
	return TaskReport{
		Path: item.doc.Path, Name: item.doc.Name,
		TaskID:      fmt.Sprintf("%s-%04d", item.doc.Name, item.index),
		RepeatIndex: item.index, Status: "failed", Error: SafeError(err),
	}
}

// HasBarrier reports whether the document synchronizes its repeated tasks.
func (d *Document) HasBarrier() bool { return documentHasBarrier(d) }

func documentHasBarrier(doc *Document) bool {
	for _, s := range doc.Steps {
		if s.Barrier != nil {
			return true
		}
	}
	return false
}

func runTask(parent context.Context, item task, opts Options) TaskReport {
	started := time.Now()
	result := TaskReport{Path: item.doc.Path, Name: item.doc.Name, TaskID: fmt.Sprintf("%s-%04d", item.doc.Name, item.index), RepeatIndex: item.index, Status: "failed"}
	timeout, _ := item.doc.taskTimeout()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	vars, err := NewVariables(item.doc.Variables)
	if err != nil {
		item.barrier.Abort(err)
		result.Error = SafeError(err)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	defer vars.release()
	redactions := vars.redactions(item.doc.Report.Redact)
	session, err := opts.Driver.Open(ctx, item.doc, vars)
	if session != nil {
		result.Clients = session.Fingerprints()
		defer session.Close()
	}
	if err != nil {
		item.barrier.Abort(err)
		result.Error = SafeError(err, redactions...)
		result.Cleanup, _ = runFinalizers(parent, item.doc.Path, item.doc.Finally, session, vars, item.barrier, opts, redactions)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	failed := false
	for _, step := range item.doc.Steps {
		stepResult, err := RunStep(ctx, item.doc.Path, step, session, vars, item.barrier, opts, redactions)
		result.Steps = append(result.Steps, stepResult)
		if err != nil {
			item.barrier.Abort(err)
			result.Error = SafeError(err, redactions...)
			failed = true
			break
		}
	}
	// Streams the document held open across steps close before the
	// finalizers so cleanup observes a settled session.
	if err := session.CloseStreams(); err != nil {
		if result.Error == "" {
			result.Error = SafeError(err, redactions...)
		}
		failed = true
	}
	var cleanupErr error
	result.Cleanup, cleanupErr = runFinalizers(parent, item.doc.Path, item.doc.Finally, session, vars, item.barrier, opts, redactions)
	if cleanupErr != nil {
		if result.Error == "" {
			result.Error = SafeError(cleanupErr, redactions...)
		}
		failed = true
	}
	if !failed {
		result.Status = "passed"
	}
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func runFinalizers(parent context.Context, documentPath string, steps []Step, session Session, vars *Variables, barrier *TaskBarrier, opts Options, redactions []string) ([]StepReport, error) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cleanupCancel()
	results := make([]StepReport, 0, len(steps))
	var firstErr error
	for _, step := range steps {
		stepResult, err := runStepStage(cleanupCtx, documentPath, step, session, vars, barrier, opts, redactions, true)
		stepResult.Stage = "cleanup"
		results = append(results, stepResult)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return results, firstErr
}

// AssertionError marks a failure where the operation itself succeeded but an
// expectation did not hold. Retry policies distinguish it from an operation
// failure, so drivers wrap expectation failures in it.
type AssertionError struct{ err error }

// NewAssertionError wraps err as an expectation failure.
func NewAssertionError(err error) *AssertionError { return &AssertionError{err: err} }

func (e *AssertionError) Error() string { return e.err.Error() }
func (e *AssertionError) Unwrap() error { return e.err }

// RunStep executes one step against session and reports it. Drivers use it to
// exercise a single operation without building a document; Run calls it for
// every step and finalizer.
func RunStep(ctx context.Context, documentPath string, step Step, session Session, vars *Variables, barrier *TaskBarrier, opts Options, redactions []string) (StepReport, error) {
	return runStepStage(ctx, documentPath, step, session, vars, barrier, opts, redactions, false)
}

func runStepStage(ctx context.Context, documentPath string, step Step, session Session, vars *Variables, barrier *TaskBarrier, opts Options, redactions []string, cleanup bool) (StepReport, error) {
	if step.Retry == nil {
		return runStepOnce(ctx, documentPath, step, session, vars, barrier, opts, redactions, cleanup)
	}
	return runStepWithRetry(ctx, documentPath, step, session, vars, barrier, opts, redactions, cleanup)
}

func runStepWithRetry(ctx context.Context, documentPath string, step Step, session Session, vars *Variables, barrier *TaskBarrier, opts Options, redactions []string, cleanup bool) (StepReport, error) {
	started := time.Now()
	retryOn := step.Retry.On
	if retryOn == nil {
		retryOn = []string{"timeout"}
	}
	var delay time.Duration
	if step.Retry.Delay != "" {
		delay, _ = time.ParseDuration(step.Retry.Delay)
	}
	attempts := make([]AttemptReport, 0, step.Retry.Attempts)
	var report StepReport
	var finalErr error
	for attempt := 1; attempt <= step.Retry.Attempts; attempt++ {
		attemptVars := cloneVariables(vars)
		attemptReport, err := runStepOnce(ctx, documentPath, step, session, attemptVars, barrier, opts, redactions, cleanup)
		if err == nil {
			if commitErr := commitAttemptVariables(vars, attemptVars); commitErr != nil {
				err = commitErr
				attemptReport.Status = "failed"
				attemptReport.Error = SafeError(err, redactions...)
			}
		}
		kind := failureKind(err)
		attempts = append(attempts, AttemptReport{
			Attempt: attempt, Status: attemptReport.Status, FailureKind: kind,
			DurationMS: attemptReport.DurationMS, Error: attemptReport.Error,
			Evidence: maps.Clone(attemptReport.Evidence),
		})
		releaseAttemptVariables(vars, attemptVars)
		report, finalErr = attemptReport, err
		if err == nil || attempt == step.Retry.Attempts || !slices.Contains(retryOn, kind) || ctx.Err() != nil {
			break
		}
		if delay > 0 {
			if delayErr := waitRetryDelay(ctx, delay); delayErr != nil {
				finalErr = delayErr
				break
			}
		}
	}
	report.Attempts = attempts
	report.DurationMS = time.Since(started).Milliseconds()
	return report, finalErr
}

func waitRetryDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func cloneVariables(input *Variables) *Variables {
	return &Variables{values: maps.Clone(input.values)}
}

func releaseAttemptVariables(base, attempt *Variables) {
	protected := make(map[variableDataIdentity]struct{})
	for _, current := range base.values {
		collectVariableDataIdentities(current.data, protected)
	}
	seen := make(map[variableDataIdentity]struct{})
	for name, original := range base.values {
		candidate := attempt.values[name]
		if original.data != nil || candidate.data == nil {
			continue
		}
		clearDiscardedVariableData(candidate.data, protected, seen)
		candidate.data = nil
		attempt.values[name] = candidate
	}
}

type variableDataIdentity struct {
	typeOf  reflect.Type
	pointer uintptr
}

func collectVariableDataIdentities(input any, identities map[variableDataIdentity]struct{}) {
	switch value := input.(type) {
	case []byte:
		identities[variableDataIdentity{typeOf: reflect.TypeFor[[]byte](), pointer: reflect.ValueOf(value).Pointer()}] = struct{}{}
	case map[string]any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[map[string]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := identities[identity]; ok {
			return
		}
		identities[identity] = struct{}{}
		for _, item := range value {
			collectVariableDataIdentities(item, identities)
		}
	case []any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[[]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := identities[identity]; ok {
			return
		}
		identities[identity] = struct{}{}
		for _, item := range value {
			collectVariableDataIdentities(item, identities)
		}
	}
}

func clearDiscardedVariableData(input any, protected, seen map[variableDataIdentity]struct{}) {
	switch value := input.(type) {
	case []byte:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[[]byte](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := protected[identity]; !ok {
			clear(value)
		}
	case map[string]any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[map[string]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := protected[identity]; ok {
			return
		}
		if _, ok := seen[identity]; ok {
			return
		}
		seen[identity] = struct{}{}
		for key, item := range value {
			clearDiscardedVariableData(item, protected, seen)
			value[key] = nil
		}
	case []any:
		identity := variableDataIdentity{typeOf: reflect.TypeFor[[]any](), pointer: reflect.ValueOf(value).Pointer()}
		if _, ok := protected[identity]; ok {
			return
		}
		if _, ok := seen[identity]; ok {
			return
		}
		seen[identity] = struct{}{}
		for index, item := range value {
			clearDiscardedVariableData(item, protected, seen)
			value[index] = nil
		}
	}
}

func commitAttemptVariables(dst, src *Variables) error {
	pending := make(map[string]any)
	for name, current := range dst.values {
		candidate := src.values[name]
		if current.data != nil || candidate.data == nil {
			continue
		}
		if err := CheckValueType(current.spec, candidate.data); err != nil {
			return fmt.Errorf("variable %q: %w", name, err)
		}
		pending[name] = candidate.data
	}
	for name, data := range pending {
		current := dst.values[name]
		current.data = data
		dst.values[name] = current
	}
	return nil
}

func failureKind(err error) string {
	if err == nil {
		return ""
	}
	var assertion *AssertionError
	if errors.As(err, &assertion) {
		return "assertion"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "operation"
}

func runStepOnce(ctx context.Context, documentPath string, step Step, session Session, vars *Variables, barrier *TaskBarrier, opts Options, redactions []string, cleanup bool) (StepReport, error) {
	started := time.Now()
	op := step.Operation()
	report := StepReport{ID: step.ID, Operation: op, Client: step.Client, Status: "failed", Stage: op}
	stepCtx, cancel := context.WithCancel(ctx)
	if step.Timeout != "" {
		duration, err := time.ParseDuration(step.Timeout)
		if err != nil {
			return report, err
		}
		stepCtx, cancel = context.WithTimeout(ctx, duration)
	}
	defer cancel()
	var evidence map[string]any
	var value any
	var saved any
	var err error
	switch op {
	case "barrier":
		if barrier == nil {
			err = fmt.Errorf("barrier not initialized")
		} else {
			err = barrier.Wait(stepCtx)
			evidence = map[string]any{"participants": barrier.total}
		}
	case "output":
		evidence, err = emitOutput(opts.Out, vars, step.Output.Variable)
	case "review":
		err = runReview(opts.In, opts.Out, step.ReviewOp.Message)
	default:
		if session == nil {
			err = fmt.Errorf("operation %s requires a connected session", op)
			break
		}
		var result StepResult
		result, err = session.Execute(stepCtx, StepRequest{
			DocumentPath: documentPath, Step: step, Vars: vars, Cleanup: cleanup,
		})
		value, saved, evidence = result.Value, result.Saved, result.Evidence
	}
	if step.ExpectError != nil {
		code, message, matched := opts.Driver.FailureCode(err)
		if !matched {
			if err == nil {
				err = &AssertionError{err: fmt.Errorf("expected RPC error code %d, got success", step.ExpectError.Code)}
			}
		} else if code != step.ExpectError.Code {
			err = &AssertionError{err: fmt.Errorf("expected RPC error code %d, got %d", step.ExpectError.Code, code)}
		} else if step.ExpectError.MessageContains != "" && !strings.Contains(message, step.ExpectError.MessageContains) {
			err = &AssertionError{err: fmt.Errorf("RPC error message does not contain expected text")}
		} else {
			err = nil
			evidence = map[string]any{"rpc_error_code": code}
		}
	}
	if err == nil && value != nil {
		if step.SaveAs != "" {
			if saved == nil {
				saved = value
			}
			err = vars.assign(step.SaveAs, saved)
		}
		if err == nil {
			err = applyCaptures(vars, step.Capture, value)
		}
		if err == nil {
			expectations, resolveErr := resolveExpectations(vars, step.Expect)
			if resolveErr != nil {
				err = resolveErr
			} else if assertionErr := assertValue(expectations, value); assertionErr != nil {
				err = &AssertionError{err: assertionErr}
			}
		}
		if evidence == nil {
			evidence = map[string]any{"result": "captured"}
		}
	}
	if err == nil {
		report.Status = "passed"
		report.Evidence = evidence
	} else {
		report.Error = SafeError(err, redactions...)
		if len(evidence) != 0 {
			report.Evidence = evidence
		}
	}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, err
}

func applyCaptures(vars *Variables, captures map[string]string, input any) error {
	for name, pointer := range captures {
		value, ok := JSONPointer(input, pointer)
		if !ok {
			return fmt.Errorf("capture pointer %q not found", pointer)
		}
		if err := vars.assign(name, value); err != nil {
			return err
		}
	}
	return nil
}

/*
resolveExpectations substitutes ${name} references in the text operands of
every expectation.

Document validation already treats a reference inside expect as a real
variable reference and rejects one whose producing step has not run, so
comparing the operand literally would contradict the document model.

Pattern is resolved too, so a generated value can anchor a regular expression.
*/
func resolveExpectations(vars *Variables, assertions map[string]Expectation) (map[string]Expectation, error) {
	if len(assertions) == 0 {
		return assertions, nil
	}
	resolved := make(map[string]Expectation, len(assertions))
	for path, expectation := range assertions {
		if expectation.Equals != nil {
			value, err := vars.Resolve(expectation.Equals)
			if err != nil {
				return nil, fmt.Errorf("expect %s equals: %w", path, err)
			}
			expectation.Equals = value
		}
		for name, field := range map[string]*string{
			"contains": &expectation.Contains,
			"pattern":  &expectation.Pattern,
		} {
			if *field == "" {
				continue
			}
			text, err := resolveText(vars, *field)
			if err != nil {
				return nil, fmt.Errorf("expect %s %s: %w", path, name, err)
			}
			*field = text
		}
		for name, needles := range map[string][]string{
			"contains_all": expectation.ContainsAll,
			"contains_any": expectation.ContainsAny,
		} {
			for i, needle := range needles {
				text, err := resolveText(vars, needle)
				if err != nil {
					return nil, fmt.Errorf("expect %s %s: %w", path, name, err)
				}
				needles[i] = text
			}
		}
		if expectation.NotContains != nil {
			value, err := vars.Resolve(expectation.NotContains)
			if err != nil {
				return nil, fmt.Errorf("expect %s not_contains: %w", path, err)
			}
			expectation.NotContains = value
		}
		resolved[path] = expectation
	}
	return resolved, nil
}

// resolveText resolves one operand that must stay a string after substitution.
func resolveText(vars *Variables, input string) (string, error) {
	value, err := vars.Resolve(input)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("must resolve to a string")
	}
	return text, nil
}

func assertValue(assertions map[string]Expectation, input any) error {
	for path, a := range assertions {
		value, ok := JSONPointer(input, path)
		if a.Present != nil && ok != *a.Present {
			return fmt.Errorf("assert %s presence = %v", path, ok)
		}
		if !ok {
			if a.Present != nil && !*a.Present {
				continue
			}
			return fmt.Errorf("assert path %q not found", path)
		}
		if a.Equals != nil {
			matched := jsonEqual(value, a.Equals)
			if a.Normalize != nil {
				text, ok := StringTarget(value)
				operand, operandOK := a.Equals.(string)
				matched = ok && operandOK && normalizeString(text, a.Normalize) == normalizeString(operand, a.Normalize)
			}
			if !matched {
				return fmt.Errorf("assert %s equals failed", path)
			}
		}
		if a.Count != nil {
			array, ok := value.([]any)
			if !ok || len(array) != *a.Count {
				return fmt.Errorf("assert %s count failed", path)
			}
		}
		if a.NonEmpty != nil {
			empty := value == nil
			switch x := value.(type) {
			case string:
				empty = x == ""
			case []any:
				empty = len(x) == 0
			case map[string]any:
				empty = len(x) == 0
			}
			if empty == *a.NonEmpty {
				return fmt.Errorf("assert %s non_empty failed", path)
			}
		}
		if err := assertStringMatchers(path, a, value); err != nil {
			return err
		}
		if err := assertNumericBounds(path, a, value); err != nil {
			return err
		}
	}
	return nil
}

// assertStringMatchers evaluates the content, pattern, and rune-length
// matchers against a string value or the empty-separator join of a
// text-fragment array. Failure messages never include the asserted content.
func assertStringMatchers(path string, a Expectation, value any) error {
	notContains, err := a.notContainsList()
	if err != nil {
		return fmt.Errorf("assert %s: %w", path, err)
	}
	hasStringMatcher := a.Contains != "" || len(a.ContainsAll) > 0 || len(a.ContainsAny) > 0 ||
		len(notContains) > 0 || a.Pattern != "" || a.MinLength != nil || a.MaxLength != nil
	if !hasStringMatcher {
		return nil
	}
	text, ok := StringTarget(value)
	if !ok {
		return fmt.Errorf("assert %s requires a string or text-fragment array target", path)
	}
	matchText := normalizeString(text, a.Normalize)
	if a.Contains != "" && !strings.Contains(matchText, normalizeString(a.Contains, a.Normalize)) {
		return fmt.Errorf("assert %s contains failed", path)
	}
	for _, needle := range a.ContainsAll {
		if !strings.Contains(matchText, normalizeString(needle, a.Normalize)) {
			return fmt.Errorf("assert %s contains_all failed", path)
		}
	}
	if len(a.ContainsAny) > 0 {
		matched := false
		for _, needle := range a.ContainsAny {
			if strings.Contains(matchText, normalizeString(needle, a.Normalize)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("assert %s contains_any failed", path)
		}
	}
	for _, needle := range notContains {
		if strings.Contains(matchText, normalizeString(needle, a.Normalize)) {
			return fmt.Errorf("assert %s not_contains failed", path)
		}
	}
	if a.Pattern != "" {
		matcher, err := regexp.Compile(a.Pattern)
		if err != nil {
			return fmt.Errorf("assert %s pattern does not compile", path)
		}
		if !matcher.MatchString(text) {
			return fmt.Errorf("assert %s pattern failed", path)
		}
	}
	if a.MinLength != nil || a.MaxLength != nil {
		runes := utf8.RuneCountInString(text)
		if a.MinLength != nil && runes < *a.MinLength {
			return fmt.Errorf("assert %s min_length failed: %d runes < %d", path, runes, *a.MinLength)
		}
		if a.MaxLength != nil && runes > *a.MaxLength {
			return fmt.Errorf("assert %s max_length failed: %d runes > %d", path, runes, *a.MaxLength)
		}
	}
	return nil
}

func normalizeString(input string, options []string) string {
	if len(options) == 0 {
		return input
	}
	whitespace := slices.Contains(options, "whitespace")
	punctuation := slices.Contains(options, "punctuation")
	digits := slices.Contains(options, "digits")
	caseFold := slices.Contains(options, "case")
	return strings.Map(func(r rune) rune {
		if whitespace && unicode.IsSpace(r) {
			return -1
		}
		if punctuation && unicode.IsPunct(r) {
			return -1
		}
		if digits {
			r = normalizeDigit(r)
		}
		if caseFold {
			r = unicode.ToLower(r)
		}
		return r
	}, input)
}

func normalizeDigit(r rune) rune {
	const asciiDigits = "0123456789"
	for _, digits := range []string{"０１２３４５６７８９", "零一二三四五六七八九"} {
		index := 0
		for _, candidate := range digits {
			if r == candidate {
				return rune(asciiDigits[index])
			}
			index++
		}
	}
	return r
}

func assertNumericBounds(path string, a Expectation, value any) error {
	if a.Minimum == nil && a.Maximum == nil {
		return nil
	}
	number, ok := NumericTarget(value)
	if !ok {
		return fmt.Errorf("assert %s requires a numeric target", path)
	}
	if a.Minimum != nil && number < *a.Minimum {
		return fmt.Errorf("assert %s minimum failed: %v < %v", path, number, *a.Minimum)
	}
	if a.Maximum != nil && number > *a.Maximum {
		return fmt.Errorf("assert %s maximum failed: %v > %v", path, number, *a.Maximum)
	}
	return nil
}

// StringTarget interprets a decoded JSON value as text, joining a
// text-fragment array with no separator. Drivers use it to assert streamed
// text in tests.
func StringTarget(value any) (string, bool) {
	switch x := value.(type) {
	case string:
		return x, true
	case []string:
		return strings.Join(x, ""), true
	case []any:
		var joined strings.Builder
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return "", false
			}
			joined.WriteString(s)
		}
		return joined.String(), true
	}
	return "", false
}

// NumericTarget interprets a decoded JSON value as a finite number. protojson
// encodes 64-bit integer fields as decimal strings, so numeric RPC fields
// arrive as strings and are accepted here.
func NumericTarget(value any) (float64, bool) {
	number, ok := rawNumericTarget(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func rawNumericTarget(value any) (float64, bool) {
	switch x := value.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case json.Number:
		number, err := x.Float64()
		return number, err == nil
	case string:
		// protojson encodes int64/uint64 fields as decimal strings, so numeric
		// RPC fields such as timestamps and byte counts arrive as strings.
		number, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return number, err == nil
	}
	return 0, false
}

// JSONPointer resolves an RFC 6901 pointer against a decoded JSON value. It
// backs both capture and expect, and drivers use it to assert their own step
// values in tests.
func JSONPointer(input any, pointer string) (any, bool) {
	if pointer == "" {
		return input, true
	}
	current := input
	for part := range strings.SplitSeq(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[part]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil, false
			}
			current = value[index]
		default:
			return nil, false
		}
	}
	return current, true
}
func jsonEqual(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}
func SafeError(err error, redactions ...string) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, secret := range redactions {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	for _, word := range []string{"token", "credential", "authorization", "private_key"} {
		if strings.Contains(strings.ToLower(text), word) {
			return "redacted execution error"
		}
	}
	if len(text) > 512 {
		text = text[:512]
	}
	return text
}

// LoadDocuments reads every path, rejects duplicate document names, and
// returns the documents sorted by path. driver is passed through to
// LoadDocument.
func LoadDocuments(paths []string, driver Driver) ([]*Document, error) {
	docs := make([]*Document, 0, len(paths))
	names := map[string]string{}
	for _, path := range paths {
		doc, err := LoadDocument(path, driver)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if other, ok := names[doc.Name]; ok {
			return nil, fmt.Errorf("duplicate document name %q in %s and %s", doc.Name, other, path)
		}
		names[doc.Name] = path
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}
