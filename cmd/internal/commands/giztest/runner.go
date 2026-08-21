package giztest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type runOptions struct {
	parallel    int
	in          io.Reader
	out         io.Writer
	speechCache *speechFixtureCache
}
type task struct {
	doc     *Document
	index   int
	barrier *taskBarrier
}

func runDocuments(ctx context.Context, docs []*Document, opts runOptions) Report {
	started := time.Now()
	report := Report{Version: "v1", StartedAt: started}
	if opts.speechCache == nil {
		opts.speechCache = newSpeechFixtureCache()
	}
	var groups [][]task
	taskCount := 0
	for _, doc := range docs {
		var barrier *taskBarrier
		if documentHasBarrier(doc) {
			barrier = newTaskBarrier(doc.Repeat)
			if opts.parallel < doc.Repeat {
				for i := range doc.Repeat {
					report.Tasks = append(report.Tasks, TaskReport{Path: doc.Path, Name: doc.Name, TaskID: fmt.Sprintf("%s-%04d", doc.Name, i), RepeatIndex: i, Status: "failed", Error: fmt.Sprintf("parallelism %d is below barrier group %d", opts.parallel, doc.Repeat)})
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
	slots := make(chan struct{}, opts.parallel)
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
		RepeatIndex: item.index, Status: "failed", Error: safeError(err),
	}
}

func documentHasBarrier(doc *Document) bool {
	for _, s := range doc.Steps {
		if s.Barrier != nil {
			return true
		}
	}
	return false
}

func runTask(parent context.Context, item task, opts runOptions) TaskReport {
	started := time.Now()
	result := TaskReport{Path: item.doc.Path, Name: item.doc.Name, TaskID: fmt.Sprintf("%s-%04d", item.doc.Name, item.index), RepeatIndex: item.index, Status: "failed"}
	timeout, _ := item.doc.taskTimeout()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	vars, err := newVariables(item.doc.Variables)
	if err != nil {
		item.barrier.Abort(err)
		result.Error = safeError(err)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	defer vars.release()
	redactions := vars.redactions(item.doc.Report.Redact)
	clients, err := connectClients(ctx, item.doc.Clients, item.doc.Steps, vars)
	if clients != nil {
		result.Clients = clients.fingerprints()
		defer clients.Close()
	}
	if err != nil {
		item.barrier.Abort(err)
		result.Error = safeError(err, redactions...)
		result.Cleanup, _ = runFinalizers(parent, item.doc.Path, item.doc.Finally, clients, vars, item.barrier, opts, redactions)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	failed := false
	for _, step := range item.doc.Steps {
		stepResult, err := runStep(ctx, item.doc.Path, step, clients, vars, item.barrier, opts, redactions)
		result.Steps = append(result.Steps, stepResult)
		if err != nil {
			item.barrier.Abort(err)
			result.Error = safeError(err, redactions...)
			failed = true
			break
		}
	}
	var cleanupErr error
	result.Cleanup, cleanupErr = runFinalizers(parent, item.doc.Path, item.doc.Finally, clients, vars, item.barrier, opts, redactions)
	if cleanupErr != nil {
		if result.Error == "" {
			result.Error = safeError(cleanupErr, redactions...)
		}
		failed = true
	}
	if !failed {
		result.Status = "passed"
	}
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func runFinalizers(parent context.Context, documentPath string, steps []Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string) ([]StepReport, error) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cleanupCancel()
	results := make([]StepReport, 0, len(steps))
	var firstErr error
	for _, step := range steps {
		stepResult, err := runStep(cleanupCtx, documentPath, step, clients, vars, barrier, opts, redactions)
		stepResult.Stage = "cleanup"
		results = append(results, stepResult)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return results, firstErr
}

func runStep(ctx context.Context, documentPath string, step Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string) (StepReport, error) {
	started := time.Now()
	op := step.operation()
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
	case "rpc":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		params := step.RPC.Request
		if params == nil {
			params = map[string]any{}
		}
		params, err = vars.resolve(params)
		if err == nil {
			value, err = invokeUnary(stepCtx, client, step, params)
			saved = value
		}
	case "rpc_stream":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		request, resolveErr := vars.resolve(step.RPCStream.Request)
		if resolveErr != nil {
			err = resolveErr
			break
		}
		streamResult, invokeErr := invokeRPCStream(stepCtx, client, step, request)
		err = invokeErr
		value, saved, evidence = streamResult.assertion, streamResult.saved, streamResult.evidence
	case "speech":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		request, resolveErr := vars.resolve(step.Speech.Request)
		if resolveErr != nil {
			err = resolveErr
			break
		}
		input, resolveErr := vars.resolve(step.Speech.Input)
		if resolveErr != nil && step.Speech.Input != nil {
			err = resolveErr
			break
		}
		var outputSpec VariableSpec
		if step.SaveAs != "" {
			outputSpec = vars.values[step.SaveAs].spec
		}
		inputSpec, _ := vars.referencedSpec(step.Speech.Input)
		invoke := func() (operationResult, error) {
			return invokeSpeech(stepCtx, client, step, request, input, inputSpec, outputSpec)
		}
		var speechResult operationResult
		var invokeErr error
		if step.Speech.Cache == "run" {
			key, keyErr := speechFixtureKey(documentPath, step, request, outputSpec)
			if keyErr != nil {
				err = keyErr
				break
			}
			var hit bool
			speechResult, hit, invokeErr = opts.speechCache.Do(stepCtx, key, invoke)
			speechResult.evidence = maps.Clone(speechResult.evidence)
			if speechResult.evidence == nil {
				speechResult.evidence = make(map[string]any)
			}
			if hit {
				speechResult.evidence["cache"] = "hit"
			} else {
				speechResult.evidence["cache"] = "miss"
			}
		} else {
			speechResult, invokeErr = invoke()
		}
		err = invokeErr
		value, saved, evidence = speechResult.assertion, speechResult.saved, speechResult.evidence
	case "peer_stream":
		client, getErr := clients.get(step.Client)
		if getErr != nil {
			err = getErr
			break
		}
		input, resolveErr := vars.resolve(step.PeerStream.Input)
		if resolveErr != nil && step.PeerStream.Input != nil {
			err = resolveErr
			break
		}
		if spec, ok := vars.referencedSpec(step.PeerStream.Input); ok && step.PeerStream.Mode != "text" {
			if spec.Type != "audio" || spec.Codec != "opus" || (spec.MediaType != "audio/ogg" && spec.MediaType != "audio/opus") {
				err = fmt.Errorf("peer_stream audio input must declare audio/ogg or audio/opus with opus codec")
				break
			}
		}
		audioCaptureMaxBytes, captureErr := peerStreamAudioCaptureMaxBytes(step, vars)
		if captureErr != nil {
			err = captureErr
			break
		}
		streamResult, invokeErr := invokePeerStream(stepCtx, client, step, input, audioCaptureMaxBytes)
		err = invokeErr
		value, saved, evidence = streamResult.assertion, streamResult.saved, streamResult.evidence
	case "barrier":
		if barrier == nil {
			err = fmt.Errorf("barrier not initialized")
		} else {
			err = barrier.Wait(stepCtx)
			evidence = map[string]any{"participants": barrier.total}
		}
	case "output":
		evidence, err = emitOutput(opts.out, vars, step.Output.Variable)
	case "review":
		err = runReview(opts.in, opts.out, step.ReviewOp.Message)
	case "client_rpc":
		key := step.Client + ":" + step.ClientRPC.Method
		counter := clients.inbound[key]
		if counter == nil {
			err = fmt.Errorf("client RPC %s was not installed", step.ClientRPC.Method)
			break
		}
		expected := int64(step.ClientRPC.ExpectCalls)
		if expected == 0 {
			expected = 1
		}
		calls := counter.Load()
		ticker := time.NewTicker(10 * time.Millisecond)
		for calls < expected && err == nil {
			select {
			case <-ticker.C:
				calls = counter.Load()
			case <-stepCtx.Done():
				err = fmt.Errorf("client RPC %s calls = %d, want at least %d: %w", step.ClientRPC.Method, calls, expected, context.Cause(stepCtx))
			}
		}
		ticker.Stop()
		if err != nil {
			break
		}
		evidence = map[string]any{"method": step.ClientRPC.Method, "calls": calls}
		value, saved = evidence, evidence
	default:
		err = fmt.Errorf("unsupported operation %q", op)
	}
	if step.ExpectError != nil {
		code, message, matched := structuredRPCError(err)
		if !matched {
			if err == nil {
				err = fmt.Errorf("expected RPC error code %d, got success", step.ExpectError.Code)
			}
		} else if code != step.ExpectError.Code {
			err = fmt.Errorf("expected RPC error code %d, got %d", step.ExpectError.Code, code)
		} else if step.ExpectError.MessageContains != "" && !strings.Contains(message, step.ExpectError.MessageContains) {
			err = fmt.Errorf("RPC error message does not contain expected text")
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
			err = assertValue(step.Expect, value)
		}
		if evidence == nil {
			evidence = map[string]any{"result": "captured"}
		}
	}
	if err == nil {
		report.Status = "passed"
		report.Evidence = evidence
	} else {
		report.Error = safeError(err, redactions...)
	}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, err
}

func peerStreamAudioCaptureMaxBytes(step Step, vars *variables) (int, error) {
	limit := 0
	for name, pointer := range step.Capture {
		if pointer != "/audio" {
			continue
		}
		item, ok := vars.values[name]
		if !ok {
			return 0, fmt.Errorf("capture references unknown variable %q", name)
		}
		if item.spec.Type != "audio" || item.spec.MaxBytes <= 0 {
			return 0, fmt.Errorf("peer_stream /audio capture variable %q must be audio with max_bytes", name)
		}
		if limit == 0 || item.spec.MaxBytes < limit {
			limit = item.spec.MaxBytes
		}
	}
	return limit, nil
}

func structuredRPCError(err error) (int32, string, bool) {
	var failure *rpcFailure
	if errors.As(err, &failure) {
		return failure.code, failure.message, true
	}
	var apiError rpcapi.Error
	if errors.As(err, &apiError) {
		return int32(apiError.Code), apiError.Message, true
	}
	return 0, "", false
}

func applyCaptures(vars *variables, captures map[string]string, input any) error {
	for name, pointer := range captures {
		value, ok := jsonPointer(input, pointer)
		if !ok {
			return fmt.Errorf("capture pointer %q not found", pointer)
		}
		if err := vars.assign(name, value); err != nil {
			return err
		}
	}
	return nil
}
func assertValue(assertions map[string]Expectation, input any) error {
	for path, a := range assertions {
		value, ok := jsonPointer(input, path)
		if a.Present != nil && ok != *a.Present {
			return fmt.Errorf("assert %s presence = %v", path, ok)
		}
		if !ok {
			if a.Present != nil && !*a.Present {
				continue
			}
			return fmt.Errorf("assert path %q not found", path)
		}
		if a.Equals != nil && !jsonEqual(value, a.Equals) {
			return fmt.Errorf("assert %s equals failed", path)
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
	text, ok := stringTarget(value)
	if !ok {
		return fmt.Errorf("assert %s requires a string or text-fragment array target", path)
	}
	if a.Contains != "" && !strings.Contains(text, a.Contains) {
		return fmt.Errorf("assert %s contains failed", path)
	}
	for _, needle := range a.ContainsAll {
		if !strings.Contains(text, needle) {
			return fmt.Errorf("assert %s contains_all failed", path)
		}
	}
	if len(a.ContainsAny) > 0 {
		matched := false
		for _, needle := range a.ContainsAny {
			if strings.Contains(text, needle) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("assert %s contains_any failed", path)
		}
	}
	for _, needle := range notContains {
		if strings.Contains(text, needle) {
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

func assertNumericBounds(path string, a Expectation, value any) error {
	if a.Minimum == nil && a.Maximum == nil {
		return nil
	}
	number, ok := numericTarget(value)
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

func stringTarget(value any) (string, bool) {
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

func numericTarget(value any) (float64, bool) {
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
func jsonPointer(input any, pointer string) (any, bool) {
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
func safeError(err error, redactions ...string) string {
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
func loadDocuments(paths []string) ([]*Document, error) {
	docs := make([]*Document, 0, len(paths))
	names := map[string]string{}
	for _, path := range paths {
		doc, err := loadDocument(path)
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
