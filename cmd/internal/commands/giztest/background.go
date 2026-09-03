package giztest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

const (
	// backgroundCancelGrace bounds how long a task waits for a cancelled
	// background step to release its PeerStream. Every wait on a background
	// goroutine is bounded by it, so a PeerStream that ignores cancellation
	// cannot hold the step timeout or the task open.
	backgroundCancelGrace = 30 * time.Second
	// backgroundJoinGrace is the last chance a step that already outlived
	// backgroundCancelGrace gets before task teardown. A step still running
	// after it owns shared clients that finalizers must not touch.
	backgroundJoinGrace = 5 * time.Second
)

// backgroundStep is one running background peer_stream. The goroutine that
// drives the stream is the only writer of result, err, and durationMS before
// done is closed; readers wait on done first.
type backgroundStep struct {
	step       Step
	awaitID    string
	cancel     context.CancelFunc
	done       chan struct{}
	result     operationResult
	err        error
	durationMS int64
}

// backgroundSteps tracks the background steps of one task in declaration
// order. Validation guarantees every background step has exactly one later
// await step; awaiters maps a background step ID to that await step so the
// stream can honor the await step's /audio capture bound from the start.
type backgroundSteps struct {
	awaiters map[string]Step
	items    map[string]*backgroundStep
	order    []string
	// unfinished holds the steps whose goroutine ignored cancellation for a
	// whole grace period and therefore still owns its PeerStream and the
	// task's shared clients.
	unfinished []*backgroundStep
	// cancelGrace bounds each individual wait; runner tests shorten it.
	cancelGrace time.Duration
	// joinGrace bounds the final wait before task teardown.
	joinGrace time.Duration
}

func newBackgroundSteps(steps []Step, cancelGrace time.Duration) *backgroundSteps {
	awaiters := make(map[string]Step)
	for _, step := range steps {
		if step.Await != "" {
			awaiters[step.Await] = step
		}
	}
	if cancelGrace <= 0 {
		cancelGrace = backgroundCancelGrace
	}
	return &backgroundSteps{
		awaiters:    awaiters,
		items:       make(map[string]*backgroundStep),
		cancelGrace: cancelGrace,
		joinGrace:   min(cancelGrace, backgroundJoinGrace),
	}
}

// stop cancels one background step and waits at most the cancel grace for its
// goroutine to release the PeerStream. A step that ignores cancellation is
// retained as unfinished instead of being waited on forever; the task joins it
// before any finalizer touches the shared clients.
func (b *backgroundSteps) stop(item *backgroundStep) bool {
	item.cancel()
	timer := time.NewTimer(b.cancelGrace)
	defer timer.Stop()
	select {
	case <-item.done:
		return true
	case <-timer.C:
		b.unfinished = append(b.unfinished, item)
		return false
	}
}

// join gives every unfinished background goroutine one last bounded chance to
// exit and returns the IDs of those still running. A non-empty result means a
// PeerStream is still owned by a background goroutine, so the task must not
// run finalizers or close the shared clients underneath it.
func (b *backgroundSteps) join() []string {
	if len(b.unfinished) == 0 {
		return nil
	}
	deadline := time.Now().Add(b.joinGrace)
	var running []string
	for _, item := range b.unfinished {
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-item.done:
		case <-timer.C:
			running = append(running, item.step.ID)
		}
		timer.Stop()
	}
	b.unfinished = nil
	return running
}

// start resolves the step on the task goroutine, then drives its PeerStream in
// a goroutine bounded by the step timeout and the task context. The returned
// report records only that the step started; its outcome belongs to the await
// step.
func (b *backgroundSteps) start(ctx context.Context, step Step, clients *clientSet, vars *variables, opts runOptions) (StepReport, error) {
	started := time.Now()
	report := StepReport{ID: step.ID, Operation: step.operation(), Client: step.Client, Status: "failed", Stage: step.operation()}
	awaitStep, ok := b.awaiters[step.ID]
	if !ok {
		report.Error = fmt.Sprintf("background step %s has no await step", step.ID)
		return report, errors.New(report.Error)
	}
	if _, exists := b.items[step.ID]; exists {
		report.Error = fmt.Sprintf("background step %s is already running", step.ID)
		return report, errors.New(report.Error)
	}
	if step.PeerStream == nil {
		report.Error = fmt.Sprintf("background step %s requires peer_stream", step.ID)
		return report, errors.New(report.Error)
	}
	if opts.audioObserver != nil {
		report.Error = "background steps cannot play audio interactively"
		return report, errors.New(report.Error)
	}
	invocation, err := preparePeerStream(step, awaitStep, clients, vars, opts)
	if err != nil {
		report.Error = safeError(err)
		report.DurationMS = time.Since(started).Milliseconds()
		return report, err
	}
	stepCtx, cancel := context.WithCancel(ctx)
	if step.Timeout != "" {
		duration, parseErr := time.ParseDuration(step.Timeout)
		if parseErr != nil {
			cancel()
			report.Error = safeError(parseErr)
			return report, parseErr
		}
		stepCtx, cancel = context.WithTimeout(ctx, duration)
	}
	item := &backgroundStep{step: step, awaitID: awaitStep.ID, cancel: cancel, done: make(chan struct{})}
	b.items[step.ID] = item
	b.order = append(b.order, step.ID)
	go func() {
		defer close(item.done)
		defer cancel()
		runStarted := time.Now()
		item.result, item.err = invocation.run(stepCtx, nil)
		item.durationMS = time.Since(runStarted).Milliseconds()
	}()
	report.Status = "started"
	report.Evidence = map[string]any{"background": true, "await": awaitStep.ID}
	report.DurationMS = time.Since(started).Milliseconds()
	return report, nil
}

// await blocks until the named background step finishes or the await step's
// timeout expires, then applies the await step's save_as, capture, and expect
// declarations to the background result.
func (b *backgroundSteps) await(ctx context.Context, step Step, vars *variables, redactions []string) (StepReport, error) {
	started := time.Now()
	report := StepReport{ID: step.ID, Operation: "await", Status: "failed", Stage: "await"}
	item, ok := b.items[step.Await]
	if !ok {
		err := fmt.Errorf("await references background step %q that is not running", step.Await)
		report.Error = safeError(err, redactions...)
		return report, err
	}
	delete(b.items, step.Await)
	report.Client = item.step.Client
	waitCtx, cancel := context.WithCancel(ctx)
	if step.Timeout != "" {
		duration, parseErr := time.ParseDuration(step.Timeout)
		if parseErr != nil {
			cancel()
			b.stop(item)
			report.Error = safeError(parseErr, redactions...)
			return report, parseErr
		}
		waitCtx, cancel = context.WithTimeout(ctx, duration)
	}
	defer cancel()
	select {
	case <-item.done:
	case <-waitCtx.Done():
		stopped := b.stop(item)
		cause := context.Cause(waitCtx)
		err := fmt.Errorf("await %s: background step cancelled before it finished: %w", step.Await, cause)
		if !stopped {
			err = fmt.Errorf("await %s: background step did not stop within %s after cancellation", step.Await, b.cancelGrace)
		}
		report.Evidence = backgroundEvidence(item)
		report.Evidence["deadline"] = "timeout"
		if errors.Is(cause, context.Canceled) {
			report.Evidence["deadline"] = "cancelled"
		}
		if !stopped {
			report.Evidence["unfinished"] = true
		}
		report.Error = safeError(err, redactions...)
		report.DurationMS = time.Since(started).Milliseconds()
		return report, err
	}
	var value, saved any
	if item.err == nil {
		value, saved = item.result.assertion, item.result.saved
	}
	return completeStepReport(report, step, vars, value, saved, backgroundEvidence(item), item.err, started, redactions)
}

// cancelRemaining stops every background step that no await step consumed,
// which only happens when the task aborts before reaching those await steps,
// and reports each one as cancelled.
func (b *backgroundSteps) cancelRemaining(redactions []string) []StepReport {
	var reports []StepReport
	for _, id := range b.order {
		item, ok := b.items[id]
		if !ok {
			continue
		}
		delete(b.items, id)
		report := StepReport{ID: id, Operation: item.step.operation(), Client: item.step.Client, Status: "cancelled", Stage: "background"}
		if b.stop(item) {
			report.Evidence = backgroundEvidence(item)
			report.Error = fmt.Sprintf("background step %s was cancelled before await step %s ran", id, item.awaitID)
			if item.err != nil {
				report.Error = safeError(fmt.Errorf("background step %s was cancelled before await step %s ran: %w", id, item.awaitID, item.err), redactions...)
			}
		} else {
			report.Evidence = map[string]any{"step": id, "background": true, "unfinished": true}
			report.Error = fmt.Sprintf("background step %s did not stop within %s after cancellation", id, b.cancelGrace)
		}
		reports = append(reports, report)
	}
	return reports
}

// backgroundEvidence is safe to call only after item.done is closed.
func backgroundEvidence(item *backgroundStep) map[string]any {
	select {
	case <-item.done:
	default:
		return map[string]any{"step": item.step.ID, "background": true}
	}
	evidence := maps.Clone(item.result.evidence)
	if evidence == nil {
		evidence = make(map[string]any)
	}
	evidence["step"] = item.step.ID
	evidence["background"] = true
	evidence["background_duration_ms"] = item.durationMS
	return evidence
}
