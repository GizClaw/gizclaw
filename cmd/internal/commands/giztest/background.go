package giztest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

// backgroundCancelGrace bounds how long an aborted task waits for a cancelled
// background step to release its PeerStream before finalizers run.
const backgroundCancelGrace = 30 * time.Second

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
}

func newBackgroundSteps(steps []Step) *backgroundSteps {
	awaiters := make(map[string]Step)
	for _, step := range steps {
		if step.Await != "" {
			awaiters[step.Await] = step
		}
	}
	return &backgroundSteps{awaiters: awaiters, items: make(map[string]*backgroundStep)}
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
			item.cancel()
			<-item.done
			report.Error = safeError(parseErr, redactions...)
			return report, parseErr
		}
		waitCtx, cancel = context.WithTimeout(ctx, duration)
	}
	defer cancel()
	select {
	case <-item.done:
	case <-waitCtx.Done():
		item.cancel()
		<-item.done
		cause := context.Cause(waitCtx)
		err := fmt.Errorf("await %s: background step cancelled before it finished: %w", step.Await, cause)
		report.Evidence = backgroundEvidence(item)
		report.Evidence["deadline"] = "timeout"
		if errors.Is(cause, context.Canceled) {
			report.Evidence["deadline"] = "cancelled"
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
		item.cancel()
		report := StepReport{ID: id, Operation: item.step.operation(), Client: item.step.Client, Status: "cancelled", Stage: "background"}
		grace := time.NewTimer(backgroundCancelGrace)
		select {
		case <-item.done:
			grace.Stop()
			report.Evidence = backgroundEvidence(item)
			report.Error = fmt.Sprintf("background step %s was cancelled before await step %s ran", id, item.awaitID)
			if item.err != nil {
				report.Error = safeError(fmt.Errorf("background step %s was cancelled before await step %s ran: %w", id, item.awaitID, item.err), redactions...)
			}
		case <-grace.C:
			report.Error = fmt.Sprintf("background step %s did not stop within %s after cancellation", id, backgroundCancelGrace)
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
