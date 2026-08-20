package giztest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type runOptions struct {
	parallel int
	in       io.Reader
	out      io.Writer
}
type task struct {
	doc     *Document
	index   int
	barrier *taskBarrier
}

func runDocuments(ctx context.Context, docs []*Document, opts runOptions) Report {
	started := time.Now()
	report := Report{Version: "v1", StartedAt: started}
	var tasks []task
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
		for i := range doc.Repeat {
			tasks = append(tasks, task{doc: doc, index: i, barrier: barrier})
		}
	}
	queue := make(chan task)
	results := make(chan TaskReport, len(tasks))
	var wg sync.WaitGroup
	for range opts.parallel {
		wg.Go(func() {
			for item := range queue {
				results <- runTask(ctx, item, opts)
			}
		})
	}
	go func() {
		defer close(queue)
		for position, item := range tasks {
			select {
			case queue <- item:
			case <-ctx.Done():
				for _, skipped := range tasks[position:] {
					results <- cancelledTaskReport(skipped, ctx)
				}
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
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
		result.Cleanup, _ = runFinalizers(parent, item.doc.Finally, clients, vars, item.barrier, opts, redactions)
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	failed := false
	for _, step := range item.doc.Steps {
		stepResult, err := runStep(ctx, step, clients, vars, item.barrier, opts, redactions)
		result.Steps = append(result.Steps, stepResult)
		if err != nil {
			item.barrier.Abort(err)
			result.Error = safeError(err, redactions...)
			failed = true
			break
		}
	}
	var cleanupErr error
	result.Cleanup, cleanupErr = runFinalizers(parent, item.doc.Finally, clients, vars, item.barrier, opts, redactions)
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

func runFinalizers(parent context.Context, steps []Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string) ([]StepReport, error) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cleanupCancel()
	results := make([]StepReport, 0, len(steps))
	var firstErr error
	for _, step := range steps {
		stepResult, err := runStep(cleanupCtx, step, clients, vars, barrier, opts, redactions)
		stepResult.Stage = "cleanup"
		results = append(results, stepResult)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return results, firstErr
}

func runStep(ctx context.Context, step Step, clients *clientSet, vars *variables, barrier *taskBarrier, opts runOptions, redactions []string) (StepReport, error) {
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
		speechResult, invokeErr := invokeSpeech(stepCtx, client, step, request, input, inputSpec, outputSpec)
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
		streamResult, invokeErr := invokePeerStream(stepCtx, client, step, input)
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
	}
	return nil
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
