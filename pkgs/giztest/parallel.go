package giztest

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	// parallelCancelGrace bounds how long a task waits for the cancelled
	// children of one parallel step to release their streams. Every wait on a
	// child goroutine is bounded by it, so a stream that ignores cancellation
	// cannot hold the step timeout or the task open.
	parallelCancelGrace = 30 * time.Second
	// parallelJoinGrace is the last chance a child that already outlived
	// parallelCancelGrace gets before task teardown. A child still running
	// after it owns shared clients that finalizers must not touch.
	parallelJoinGrace = 5 * time.Second
)

// parallelChild is one running child of a parallel step. The goroutine that
// drives it is the only writer of result, err, and durationMS before done is
// closed; readers wait on done first.
type parallelChild struct {
	step       Step
	done       chan struct{}
	result     StepResult
	err        error
	durationMS int64
}

/*
parallelTracker owns the driver-agnostic half of the parallel contract for one
task: the runner starts every child goroutine, bounds every wait on it, and
refuses to tear the session down while a child that ignored cancellation still
owns a stream. A Session contributes only the prepare and run phases through
ParallelSession.
*/
type parallelTracker struct {
	// unfinished holds the children whose goroutine ignored cancellation for a
	// whole grace period and therefore still own their stream and the task's
	// shared clients.
	unfinished []*parallelChild
	// cancelGrace bounds the wait for one cancelled group; runner tests
	// shorten it.
	cancelGrace time.Duration
	// joinGrace bounds the final wait before task teardown.
	joinGrace time.Duration
}

func newParallelTracker(cancelGrace time.Duration) *parallelTracker {
	if cancelGrace <= 0 {
		cancelGrace = parallelCancelGrace
	}
	return &parallelTracker{cancelGrace: cancelGrace, joinGrace: min(cancelGrace, parallelJoinGrace)}
}

// stopAll waits at most one cancel grace, shared by the whole group, for
// already-cancelled children to release their streams. Children that ignore
// cancellation are retained as unfinished instead of being waited on forever;
// the task joins them before any finalizer touches the shared clients.
func (p *parallelTracker) stopAll(children []*parallelChild) []string {
	deadline := time.Now().Add(p.cancelGrace)
	var unfinished []string
	for _, child := range children {
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-child.done:
		case <-timer.C:
			p.unfinished = append(p.unfinished, child)
			unfinished = append(unfinished, child.step.ID)
		}
		timer.Stop()
	}
	return unfinished
}

// join gives every unfinished child goroutine one last bounded chance to exit
// and returns the IDs of those still running. A non-empty result means a
// stream is still owned by a child goroutine, so the task must not run
// finalizers or close the session underneath it.
func (p *parallelTracker) join() []string {
	if p == nil || len(p.unfinished) == 0 {
		return nil
	}
	deadline := time.Now().Add(p.joinGrace)
	var running []string
	for _, child := range p.unfinished {
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-child.done:
		case <-timer.C:
			running = append(running, child.step.ID)
		}
		timer.Stop()
	}
	p.unfinished = nil
	return running
}

/*
runParallel resolves every child's inputs on the task goroutine, starts all of
them at once, and returns once every child finished or the step context
expired.

The value is an object keyed by child id, so the step's capture and expect
declarations address a child result by JSON pointer. One failing child fails
the step, and every child's own outcome is returned as its own step report.
*/
func runParallel(ctx context.Context, documentPath string, step Step, session Session, vars *Variables, tracker *parallelTracker, redactions []string) (map[string]any, []StepReport, map[string]any, error) {
	if session == nil {
		return nil, nil, nil, fmt.Errorf("step %s parallel requires a connected session", step.ID)
	}
	parallel, ok := session.(ParallelSession)
	if !ok {
		return nil, nil, nil, fmt.Errorf("step %s: this runner cannot run parallel steps", step.ID)
	}
	if tracker == nil {
		tracker = newParallelTracker(0)
	}
	// Prepare resolves variables, which the task goroutine owns exclusively,
	// so it must complete for every child before any child starts.
	runs := make([]ParallelChild, 0, len(step.Parallel))
	for _, child := range step.Parallel {
		run, err := parallel.PrepareParallel(StepRequest{DocumentPath: documentPath, Step: child, Vars: vars, Parent: &step})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("step %s parallel child %s: %w", step.ID, child.ID, err)
		}
		runs = append(runs, run)
	}

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// start releases every child goroutine at the same moment, so the children
	// speak and listen simultaneously rather than in declaration order. A child
	// that declares a delay waits that long after the release, which is how a
	// scenario places one child's window inside another child's activity
	// instead of at the same instant.
	start := make(chan struct{})
	children := make([]*parallelChild, len(runs))
	finished := make(chan struct{})
	for i, run := range runs {
		child := &parallelChild{step: step.Parallel[i], done: make(chan struct{})}
		children[i] = child
		delay := childDelay(child.step)
		go func() {
			defer close(child.done)
			<-start
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-childCtx.Done():
					timer.Stop()
					child.err = context.Cause(childCtx)
					return
				}
			}
			started := time.Now()
			child.result, child.err = run.Run(childCtx)
			child.durationMS = time.Since(started).Milliseconds()
		}()
	}
	go func() {
		defer close(finished)
		for _, child := range children {
			<-child.done
		}
	}()
	close(start)

	var unfinished []string
	var deadline error
	select {
	case <-finished:
	case <-childCtx.Done():
		deadline = context.Cause(childCtx)
		cancel()
		unfinished = tracker.stopAll(children)
	}
	value, reports, failures := parallelReports(children, tracker.cancelGrace, redactions)
	evidence := map[string]any{"parallel": len(children), "children": childIDs(children)}
	if len(unfinished) != 0 {
		evidence["unfinished"] = unfinished
	}
	return value, reports, evidence, parallelError(step, deadline, unfinished, failures)
}

// parallelReports turns the finished children into the step value and one
// report per child. A child is read only after its done channel is closed.
func parallelReports(children []*parallelChild, cancelGrace time.Duration, redactions []string) (map[string]any, []StepReport, []string) {
	value := make(map[string]any, len(children))
	reports := make([]StepReport, 0, len(children))
	var failures []string
	for _, child := range children {
		report := StepReport{ID: child.step.ID, Operation: child.step.Operation(), Client: child.step.Client, Status: "passed", Stage: "parallel"}
		select {
		case <-child.done:
			report.DurationMS = child.durationMS
			report.Evidence = child.result.Evidence
			if child.err != nil {
				report.Status = "failed"
				report.Error = SafeError(child.err, redactions...)
				failures = append(failures, fmt.Sprintf("%s: %s", child.step.ID, report.Error))
			} else {
				value[child.step.ID] = child.result.Value
			}
		default:
			report.Status = "failed"
			report.Evidence = map[string]any{"unfinished": true}
			report.Error = fmt.Sprintf("parallel child %s did not stop within %s after cancellation", child.step.ID, cancelGrace)
		}
		reports = append(reports, report)
	}
	return value, reports, failures
}

// parallelError reports the step outcome: an expired step timeout, children
// that ignored cancellation, or the individual child failures.
func parallelError(step Step, deadline error, unfinished, failures []string) error {
	switch {
	case len(unfinished) != 0:
		return fmt.Errorf("parallel step %s: children %s did not stop after cancellation", step.ID, strings.Join(unfinished, ", "))
	case deadline != nil:
		return fmt.Errorf("parallel step %s cancelled its children before they finished: %w", step.ID, deadline)
	case len(failures) != 0:
		return fmt.Errorf("parallel step %s failed: %s", step.ID, strings.Join(failures, "; "))
	}
	return nil
}

func childIDs(children []*parallelChild) []string {
	ids := make([]string, 0, len(children))
	for _, child := range children {
		ids = append(ids, child.step.ID)
	}
	return ids
}

// ParallelChildCaptures returns the entries of a parallel step's capture map
// that address childID, with the child's own pointer prefix removed. A driver
// uses it to apply a bound the parent declared, such as the /audio capture
// limit, while preparing that child.
func ParallelChildCaptures(parent Step, childID string) map[string]string {
	captures := make(map[string]string)
	prefix := "/" + childID
	for name, pointer := range parent.Capture {
		if pointer == prefix {
			captures[name] = ""
			continue
		}
		if rest, ok := strings.CutPrefix(pointer, prefix+"/"); ok {
			captures[name] = "/" + rest
		}
	}
	return captures
}

// childDelay is the stagger a parallel child declared. The document validated
// it, so an unparsable value here can only mean the step was built in code and
// is treated as no delay rather than failing the group.
func childDelay(step Step) time.Duration {
	if step.Delay == "" {
		return 0
	}
	delay, err := time.ParseDuration(step.Delay)
	if err != nil || delay <= 0 {
		return 0
	}
	return delay
}
